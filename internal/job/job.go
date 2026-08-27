// Package job verwaltet Split-Aufträge: das Manifest neben den Chunks, die
// Ablage der übersetzten Teile und die Wiederauffindbarkeit über eine ID.
//
// Der MCP-Server gibt einem Modell nie das ganze Dokument zurück, sondern nur
// das Manifest; Inhalt wandert einzeln über get_chunk/put_chunk. Damit bleibt
// der Kontextbedarf konstant, egal wie groß die Quelle ist.
package job

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mlechner911/mdsplit/internal/meta"
	"github.com/mlechner911/mdsplit/internal/split"
)

// IndexName ist der Dateiname des Manifests im Chunk-Ordner.
const IndexName = "index.json"

// DefaultTarget ist die Endung für zurückgeschriebene Teile, wenn beim Split
// keine Zielsprache angegeben wurde.
const DefaultTarget = "out"

// Provenance records how a document was produced. It is written to the
// manifest on every run - it costs nothing and is there when someone asks
// where a translation came from - and can optionally be stamped into the
// merged document itself.
//
// SourceSHA256 is the field that earns its keep: with it, a later run can tell
// that the source has changed since the translation was made. Size and date are
// informative; the hash is what does work.
type Provenance struct {
	Tool        string `json:"tool"`
	Version     string `json:"version"`
	URL         string `json:"url"`
	Source      string `json:"source"`
	SourceSHA   string `json:"source_sha256"`
	SourceChars int    `json:"source_chars"`
	// The rest is filled in once a part has actually been translated.
	TargetLang string `json:"target_lang,omitempty"`
	Model      string `json:"model,omitempty"`
	Mode       string `json:"mode,omitempty"`
	Translated string `json:"translated,omitempty"` // RFC3339
	Machine    bool   `json:"machine_translation,omitempty"`
}

// Part beschreibt einen Chunk im Manifest.
type Part struct {
	Part    int    `json:"part"`
	File    string `json:"file"` // Dateiname im Job-Ordner
	Chars   int    `json:"chars"`
	Heading string `json:"heading,omitempty"`
}

// Manifest ist der Auftrag: Quelle, Chunk-Liste und die Leerzeilen-Abstände,
// die den byte-genauen Rückweg möglich machen.
type Manifest struct {
	ID         string `json:"id"`
	SourceFile string `json:"source_file"`
	TotalParts int    `json:"total_parts"`
	Size       int    `json:"size"`
	Target     string `json:"target,omitempty"`
	Language   string `json:"language,omitempty"`
	// Glossary pins terminology across parts. Because each part is translated
	// in isolation, nothing else stops the model from rendering the same term
	// two ways in two chunks.
	Glossary map[string]string `json:"glossary,omitempty"`
	Gaps     []int             `json:"gaps"`
	// Provenance records what produced this split and, once translated, what
	// produced the translation.
	Provenance Provenance `json:"provenance"`
	Parts      []Part     `json:"parts"`
	Chunks     []string   `json:"chunks"` // Dateinamen, für ältere Leser

	dir string // Laufzeit: Ordner, aus dem geladen wurde
}

// HashSource returns the SHA-256 of a file, shortened to 16 hex characters -
// enough to notice a change, short enough to read in a manifest.
func HashSource(path string) (string, int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 0, fmt.Errorf("read source: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:16], len(data), nil
}

// SourceChanged reports whether the source file differs from the one this
// manifest was built from. A missing hash means the manifest predates
// provenance, so nothing can be said.
func (m *Manifest) SourceChanged() (changed bool, known bool) {
	if m.Provenance.SourceSHA == "" {
		return false, false
	}
	sum, _, err := HashSource(m.SourceFile)
	if err != nil {
		return false, false
	}
	return sum != m.Provenance.SourceSHA, true
}

// ID leitet eine stabile Auftrags-ID aus Quellpfad und Zielgröße ab. Derselbe
// Split ergibt dieselbe ID, ein erneutes Splitten überschreibt also sauber.
func ID(source string, size int) string {
	abs, err := filepath.Abs(source)
	if err != nil {
		abs = source
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d", abs, size)))
	return hex.EncodeToString(sum[:])[:10]
}

// New baut das Manifest zu einem fertigen Split.
func New(source string, size int, target string, doc split.Doc) *Manifest {
	ext := filepath.Ext(source)
	if ext == "" {
		ext = ".md"
	}
	base := strings.TrimSuffix(filepath.Base(source), ext)

	m := &Manifest{
		ID:         ID(source, size),
		SourceFile: source,
		TotalParts: len(doc.Chunks),
		Size:       size,
		Target:     target,
		Gaps:       doc.Gaps,
		Provenance: Provenance{
			Tool:    meta.Name,
			Version: meta.Version,
			URL:     meta.URL,
			Source:  filepath.Base(source),
		},
	}
	if sum, n, err := HashSource(source); err == nil {
		m.Provenance.SourceSHA, m.Provenance.SourceChars = sum, n
	}
	for i, c := range doc.Chunks {
		name := fmt.Sprintf("%s-part-%02d%s", base, i+1, ext)
		m.Parts = append(m.Parts, Part{
			Part:    i + 1,
			File:    name,
			Chars:   len(c),
			Heading: split.FirstHeading(c),
		})
		m.Chunks = append(m.Chunks, name)
	}
	return m
}

// Dir liefert den Ordner, in dem die Chunks liegen.
func (m *Manifest) Dir() string { return m.dir }

// SetDir setzt den Ordner (vor dem ersten Save).
func (m *Manifest) SetDir(dir string) { m.dir = dir }

// Write legt Chunks und Manifest im Ordner ab und registriert die ID.
func (m *Manifest) Write(dir string, chunks []string) error {
	if len(chunks) != len(m.Parts) {
		return fmt.Errorf("manifest has %d parts but %d chunks were passed", len(m.Parts), len(chunks))
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	m.dir = dir
	for i, p := range m.Parts {
		if err := os.WriteFile(filepath.Join(dir, p.File), []byte(chunks[i]), 0o644); err != nil {
			return fmt.Errorf("write chunk: %w", err)
		}
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, IndexName), data, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return registerID(m.ID, dir)
}

// Load liest das Manifest aus einem Chunk-Ordner.
func Load(dir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, IndexName))
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	m.dir = dir
	// Ältere Manifeste kennen nur chunks[]; Parts daraus nachziehen.
	if len(m.Parts) == 0 {
		for i, c := range m.Chunks {
			m.Parts = append(m.Parts, Part{Part: i + 1, File: filepath.Base(c)})
		}
		m.TotalParts = len(m.Parts)
	}
	return &m, nil
}

// LoadByID findet einen Auftrag über die im Split vergebene ID.
func LoadByID(id string) (*Manifest, error) {
	dir, err := lookupID(id)
	if err != nil {
		return nil, err
	}
	return Load(dir)
}

// part liefert den Eintrag zu einer 1-basierten Teilnummer.
func (m *Manifest) part(n int) (Part, error) {
	if n < 1 || n > len(m.Parts) {
		return Part{}, fmt.Errorf("part %d is out of range 1..%d", n, len(m.Parts))
	}
	return m.Parts[n-1], nil
}

// SourcePath liefert den Pfad des Original-Chunks.
func (m *Manifest) SourcePath(n int) (string, error) {
	p, err := m.part(n)
	if err != nil {
		return "", err
	}
	return filepath.Join(m.dir, p.File), nil
}

// TargetPath liefert den Pfad, unter dem der bearbeitete Teil abgelegt wird:
// <name>-part-NN.<target>.md neben dem Original.
func (m *Manifest) TargetPath(n int) (string, error) {
	p, err := m.part(n)
	if err != nil {
		return "", err
	}
	target := m.Target
	if target == "" {
		target = DefaultTarget
	}
	ext := filepath.Ext(p.File)
	return filepath.Join(m.dir, strings.TrimSuffix(p.File, ext)+"."+target+ext), nil
}

// ReadChunk liefert den Text eines Teils - den bearbeiteten, falls vorhanden.
func (m *Manifest) ReadChunk(n int) (string, bool, error) {
	if tp, err := m.TargetPath(n); err == nil {
		if data, err := os.ReadFile(tp); err == nil {
			return string(data), true, nil
		}
	}
	sp, err := m.SourcePath(n)
	if err != nil {
		return "", false, err
	}
	data, err := os.ReadFile(sp)
	if err != nil {
		return "", false, fmt.Errorf("read chunk: %w", err)
	}
	return string(data), false, nil
}

// WriteChunk legt den bearbeiteten Teil ab.
func (m *Manifest) WriteChunk(n int, text string) error {
	tp, err := m.TargetPath(n)
	if err != nil {
		return err
	}
	return os.WriteFile(tp, []byte(text), 0o644)
}

// RecordRun stores what produced the current translations and saves the
// manifest. Called after a translating run so the provenance reflects it.
func (m *Manifest) RecordRun(lang, model, mode, when string) error {
	m.Provenance.TargetLang = lang
	m.Provenance.Model = model
	m.Provenance.Mode = mode
	m.Provenance.Translated = when
	m.Provenance.Machine = true
	return m.save()
}

// save rewrites the manifest in place.
func (m *Manifest) save() error {
	if m.dir == "" {
		return fmt.Errorf("manifest has no directory")
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	return os.WriteFile(filepath.Join(m.dir, IndexName), data, 0o644)
}

// PartsSummary renders "7/11" for a stamp.
func (m *Manifest) PartsSummary() string {
	done, _ := m.Progress()
	return fmt.Sprintf("%d/%d", done, m.TotalParts)
}

// Progress zählt, wie viele Teile bereits zurückgeschrieben wurden.
func (m *Manifest) Progress() (done int, missing []int) {
	for i := range m.Parts {
		tp, err := m.TargetPath(i + 1)
		if err != nil {
			continue
		}
		if _, err := os.Stat(tp); err == nil {
			done++
		} else {
			missing = append(missing, i+1)
		}
	}
	return done, missing
}

// MergePaths liefert die zu verschmelzenden Dateien: je Teil den bearbeiteten,
// sonst das Original. translated sagt, wie viele davon bearbeitet waren.
func (m *Manifest) MergePaths() (paths []string, translated int, err error) {
	for i := range m.Parts {
		tp, terr := m.TargetPath(i + 1)
		if terr == nil {
			if _, serr := os.Stat(tp); serr == nil {
				paths = append(paths, tp)
				translated++
				continue
			}
		}
		sp, serr := m.SourcePath(i + 1)
		if serr != nil {
			return nil, 0, serr
		}
		if _, statErr := os.Stat(sp); statErr != nil {
			return nil, 0, fmt.Errorf("chunk file is missing: %s", sp)
		}
		paths = append(paths, sp)
	}
	return paths, translated, nil
}

// --- ID-Registrierung ---------------------------------------------------
//
// Damit get_chunk/put_chunk nur die ID brauchen, merkt sich der Server, wo der
// Ordner zu einer ID liegt. Ein Zeiger je Auftrag im Cache-Verzeichnis.

func pointerDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locate cache directory: %w", err)
	}
	return filepath.Join(base, "mcp-md-splitter", "jobs"), nil
}

func registerID(id, dir string) error {
	pd, err := pointerDir()
	if err != nil {
		return nil // ohne Cache-Ordner bleibt der Ordnerpfad der Zugang
	}
	if err := os.MkdirAll(pd, 0o755); err != nil {
		return nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	return os.WriteFile(filepath.Join(pd, id+".json"),
		[]byte(fmt.Sprintf("{\n  \"dir\": %q\n}\n", abs)), 0o644)
}

func lookupID(id string) (string, error) {
	pd, err := pointerDir()
	if err != nil {
		return "", fmt.Errorf("cannot locate job %s: %w", id, err)
	}
	data, err := os.ReadFile(filepath.Join(pd, id+".json"))
	if err != nil {
		return "", fmt.Errorf("unknown jobId %q - run split_markdown again to create one", id)
	}
	var p struct {
		Dir string `json:"dir"`
	}
	if err := json.Unmarshal(data, &p); err != nil || p.Dir == "" {
		return "", fmt.Errorf("job pointer %q is unreadable", id)
	}
	return p.Dir, nil
}
