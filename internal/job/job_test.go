package job

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mlechner911/mdsplit/internal/split"
)

const doc = "# Titel\n\nProsa eins.\n\n## Kapitel\n\n```go\nfunc f() {}\n```\n\n- eins\n- zwei\n\nFazit.\n"

// newJob legt einen Split in einem Temp-Ordner an.
func newJob(t *testing.T, src string, size int, target string) (*Manifest, string, string) {
	t.Helper()
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(srcPath, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	d := split.SplitDoc(src, size)
	m := New(srcPath, size, target, d)
	if err := m.Write(filepath.Join(dir, "chunks"), d.Chunks); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return m, srcPath, dir
}

func TestID_StableAndSizeDependent(t *testing.T) {
	a := ID("/tmp/a.md", 2000)
	if a != ID("/tmp/a.md", 2000) {
		t.Error("ID ist nicht stabil")
	}
	if a == ID("/tmp/a.md", 4000) {
		t.Error("ID ignoriert die Zielgröße")
	}
	if a == ID("/tmp/b.md", 2000) {
		t.Error("ID ignoriert die Quelldatei")
	}
}

func TestWriteAndLoad(t *testing.T) {
	m, _, _ := newJob(t, doc, 60, "de")
	if m.TotalParts < 2 {
		t.Fatalf("zu wenige Teile für den Test: %d", m.TotalParts)
	}
	got, err := Load(m.Dir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.ID != m.ID || got.TotalParts != m.TotalParts || got.Target != "de" {
		t.Errorf("Manifest weicht ab: %+v", got)
	}
	if len(got.Gaps) != got.TotalParts-1 {
		t.Errorf("Gaps = %d, erwartet %d", len(got.Gaps), got.TotalParts-1)
	}
	for _, p := range got.Parts {
		if p.Chars == 0 {
			t.Errorf("Teil %d ohne Länge", p.Part)
		}
		sp, err := got.SourcePath(p.Part)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(sp); err != nil {
			t.Errorf("Chunk-Datei fehlt: %s", sp)
		}
	}
	if got.Parts[0].Heading != "# Titel" {
		t.Errorf("Heading = %q, erwartet \"# Titel\"", got.Parts[0].Heading)
	}
}

// TestSplitReturnsNoContent: das Manifest darf den Dokumentinhalt nicht
// mitschleppen - genau darum geht es beim Job-Workflow.
func TestSplitReturnsNoContent(t *testing.T) {
	m, _, _ := newJob(t, doc, 60, "")
	raw, err := os.ReadFile(filepath.Join(m.Dir(), IndexName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "func f() {}") {
		t.Error("Manifest enthält Dokumentinhalt")
	}
	if strings.Contains(string(raw), "Prosa eins") {
		t.Error("Manifest enthält Dokumentinhalt")
	}
}

func TestPutChunk_RoundTrip(t *testing.T) {
	m, srcPath, _ := newJob(t, doc, 60, "de")

	if done, missing := m.Progress(); done != 0 || len(missing) != m.TotalParts {
		t.Fatalf("Startfortschritt falsch: done=%d missing=%d", done, len(missing))
	}

	// Ohne Bearbeitung ergibt der Merge byte-genau das Original.
	paths, translated, err := m.MergePaths()
	if err != nil {
		t.Fatal(err)
	}
	if translated != 0 {
		t.Errorf("translated = %d, erwartet 0", translated)
	}
	merged, err := split.MergeFilesGaps(paths, m.Gaps)
	if err != nil {
		t.Fatal(err)
	}
	orig, _ := os.ReadFile(srcPath)
	if want := split.Canonical(string(orig)); merged != want {
		t.Fatalf("Merge ohne Bearbeitung nicht byte-genau:\nWANT %q\nGOT  %q", want, merged)
	}

	// Einen Teil bearbeiten: Original bleibt liegen, Merge nimmt die Fassung.
	if err := m.WriteChunk(1, "ERSETZT"); err != nil {
		t.Fatal(err)
	}
	sp, _ := m.SourcePath(1)
	if data, _ := os.ReadFile(sp); strings.Contains(string(data), "ERSETZT") {
		t.Error("put_chunk hat das Original überschrieben")
	}
	text, edited, err := m.ReadChunk(1)
	if err != nil {
		t.Fatal(err)
	}
	if !edited || text != "ERSETZT" {
		t.Errorf("ReadChunk = %q/%v, erwartet \"ERSETZT\"/true", text, edited)
	}
	done, missing := m.Progress()
	if done != 1 || len(missing) != m.TotalParts-1 || missing[0] != 2 {
		t.Errorf("Fortschritt falsch: done=%d missing=%v", done, missing)
	}
	paths, translated, err = m.MergePaths()
	if err != nil {
		t.Fatal(err)
	}
	if translated != 1 {
		t.Errorf("translated = %d, erwartet 1", translated)
	}
	merged, err = split.MergeFilesGaps(paths, m.Gaps)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(merged, "ERSETZT") {
		t.Errorf("bearbeiteter Teil fehlt im Merge: %.40q", merged)
	}
}

func TestTargetPath_Naming(t *testing.T) {
	m, _, _ := newJob(t, doc, 60, "de")
	tp, err := m.TargetPath(1)
	if err != nil {
		t.Fatal(err)
	}
	if base := filepath.Base(tp); base != "doc-part-01.de.md" {
		t.Errorf("TargetPath = %q, erwartet doc-part-01.de.md", base)
	}

	m2, _, _ := newJob(t, doc, 60, "")
	tp2, _ := m2.TargetPath(1)
	if base := filepath.Base(tp2); base != "doc-part-01.out.md" {
		t.Errorf("TargetPath ohne target = %q, erwartet doc-part-01.out.md", base)
	}
}

func TestPartOutOfRange(t *testing.T) {
	m, _, _ := newJob(t, doc, 60, "")
	for _, n := range []int{0, -1, m.TotalParts + 1} {
		if _, err := m.SourcePath(n); err == nil {
			t.Errorf("Teil %d hätte einen Fehler liefern müssen", n)
		}
		if _, _, err := m.ReadChunk(n); err == nil {
			t.Errorf("ReadChunk(%d) hätte einen Fehler liefern müssen", n)
		}
	}
}

func TestLoadByID(t *testing.T) {
	m, _, _ := newJob(t, doc, 60, "")
	got, err := LoadByID(m.ID)
	if err != nil {
		t.Skipf("kein Cache-Verzeichnis verfügbar: %v", err)
	}
	if got.Dir() != m.Dir() {
		t.Errorf("Dir = %q, erwartet %q", got.Dir(), m.Dir())
	}
	if _, err := LoadByID("nichtvorhanden"); err == nil {
		t.Error("unbekannte jobId hätte einen Fehler liefern müssen")
	}
}

// TestArtifacts_RoundTrip prüft die echten Artefakte im Projekt: die Chunks
// aus testdata/chunks/ müssen zusammen wieder die Fixture ergeben. Deckt damit
// ab, dass CLI-Schreibweise und Manifest-Leser zueinander passen.
func TestArtifacts_RoundTrip(t *testing.T) {
	root := "../.."
	orig, err := os.ReadFile(filepath.Join(root, "testdata", "sample.md"))
	if err != nil {
		t.Skip("testdata/sample.md nicht vorhanden; laufe nur in Projektroot")
	}
	m, err := Load(filepath.Join(root, "testdata", "chunks"))
	if err != nil {
		t.Skip("testdata/chunks/ fehlt - erst `task roundtrip` ausführen")
	}
	if filepath.Base(m.SourceFile) != "sample.md" {
		t.Skipf("Manifest verweist auf %s statt auf die Fixture", m.SourceFile)
	}
	// Bewusst die Originale, nicht MergePaths: bearbeitete Teile wären
	// Übersetzungen und dürften vom Original abweichen.
	paths := make([]string, 0, m.TotalParts)
	for i := 1; i <= m.TotalParts; i++ {
		p, err := m.SourcePath(i)
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	merged, err := split.MergeFilesGaps(paths, m.Gaps)
	if err != nil {
		t.Fatalf("MergeFilesGaps: %v", err)
	}
	if want := split.Canonical(string(orig)); merged != want {
		t.Fatalf("Artefakt-Roundtrip verletzt: WANT %d / GOT %d Zeichen", len(want), len(merged))
	}
}

// TestProvenance_RecordedOnSplit: die Herkunft wird immer festgehalten, auch
// ohne Übersetzung - sie kostet nichts und fehlt sonst genau dann, wenn man
// sie braucht.
func TestProvenance_RecordedOnSplit(t *testing.T) {
	m, _, _ := newJob(t, doc, 60, "de")
	got, err := Load(m.Dir())
	if err != nil {
		t.Fatal(err)
	}
	p := got.Provenance
	if p.Tool == "" || p.URL == "" {
		t.Errorf("Tool-Identität fehlt: %+v", p)
	}
	if p.SourceSHA == "" || p.SourceChars != len(doc) {
		t.Errorf("Quell-Hash oder -Größe fehlen: %+v", p)
	}
	if p.Machine {
		t.Error("machine_translation gesetzt, obwohl nichts übersetzt wurde")
	}
}

// TestSourceChanged ist der Grund, warum der Hash überhaupt gespeichert wird:
// eine still veraltete Übersetzung ist schlimmer als eine offensichtlich
// fehlende.
func TestSourceChanged(t *testing.T) {
	m, srcPath, _ := newJob(t, doc, 60, "de")

	changed, known := m.SourceChanged()
	if !known {
		t.Fatal("Hash wurde nicht gespeichert")
	}
	if changed {
		t.Error("unveränderte Quelle gilt als geändert")
	}

	if err := os.WriteFile(srcPath, []byte(doc+"\nEin neuer Satz.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if changed, _ := m.SourceChanged(); !changed {
		t.Error("geänderte Quelle wurde nicht erkannt")
	}

	// Ein Manifest ohne Hash darf nicht raten.
	m.Provenance.SourceSHA = ""
	if _, known := m.SourceChanged(); known {
		t.Error("Manifest ohne Hash behauptet, Bescheid zu wissen")
	}
}

func TestRecordRun_PersistsAndMarksMachine(t *testing.T) {
	m, _, _ := newJob(t, doc, 60, "de")
	if err := m.RecordRun("de", "some-model", "block", "2026-08-27T14:23:11Z"); err != nil {
		t.Fatalf("RecordRun: %v", err)
	}
	got, err := Load(m.Dir())
	if err != nil {
		t.Fatal(err)
	}
	p := got.Provenance
	if p.Model != "some-model" || p.Mode != "block" || p.TargetLang != "de" {
		t.Errorf("Lauf nicht festgehalten: %+v", p)
	}
	if !p.Machine {
		t.Error("machine_translation nicht gesetzt - genau das soll ehrlich dabeistehen")
	}
	// Der Hash der Quelle darf ein Lauf nicht überschreiben.
	if p.SourceSHA == "" {
		t.Error("Quell-Hash beim Aufzeichnen verloren")
	}
}

// TestProvenance_NoEndpointLeak: die URL des Endpoints hat in einem Dokument
// nichts zu suchen, das später öffentlich werden kann.
func TestProvenance_NoEndpointLeak(t *testing.T) {
	m, _, _ := newJob(t, doc, 60, "de")
	if err := m.RecordRun("de", "some-model", "block", "2026-08-27T14:23:11Z"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(m.Dir(), IndexName))
	if err != nil {
		t.Fatal(err)
	}
	y := m.Provenance.YAML(m.PartsSummary())
	for _, forbidden := range []string{"11434", "192.168", "Bearer", "localhost:"} {
		if strings.Contains(string(raw), forbidden) || strings.Contains(y, forbidden) {
			t.Errorf("Endpoint-Detail %q ist in die Provenienz gelangt", forbidden)
		}
	}
}
