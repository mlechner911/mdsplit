package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mlechner911/mdsplit/internal/job"
	"github.com/mlechner911/mdsplit/internal/split"
)

// runMergeMode setzt die Teile eines Splits wieder zusammen. Der Ordner muss
// eine index.json enthalten; je Teil wird die bearbeitete Fassung genommen,
// falls vorhanden, sonst das Original.
func runMergeMode(dir string, out string, stamp bool) {
	if dir == "" {
		fmt.Println("error: pass the chunk directory with -dir (e.g. chunks/)")
		os.Exit(1)
	}
	abs, err := filepath.Abs(dir)
	if err != nil || !dirExists(abs) {
		fmt.Printf("error: directory not found: %s\n", dir)
		os.Exit(1)
	}

	m, err := job.Load(abs)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
	paths, translated, err := m.MergePaths()
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}

	merged, err := split.MergeFilesGaps(paths, m.Gaps)
	if err != nil {
		fmt.Printf("error: merge failed: %v\n", err)
		os.Exit(1)
	}

	if out == "" {
		out = defaultMergeTarget(m)
	}

	// Der Roundtrip-Check läuft gegen den ungestempelten Text: sonst meldet er
	// ab dem ersten Stempel dauerhaft "weicht ab" und wird wertlos.
	verdict := ""
	if translated == 0 {
		if data, err := os.ReadFile(m.SourceFile); err == nil {
			verdict = roundtripVerdict(merged, string(data))
		}
	}
	if stamp {
		merged = job.Stamp(merged, m.Provenance.YAML(m.PartsSummary()))
	}
	if err := os.WriteFile(out, []byte(merged), 0o644); err != nil {
		fmt.Printf("error: cannot write %s: %v\n", out, err)
		os.Exit(1)
	}

	fmt.Printf("merged %d parts into %s (%d chars)\n", len(paths), out, len(merged))
	if stamp {
		fmt.Println("  provenance stamped into the front matter")
	}
	if translated > 0 {
		fmt.Printf("  edited parts used: %d/%d\n", translated, m.TotalParts)
		return
	}
	if verdict != "" {
		fmt.Println(verdict)
	}
}

// runCheckMode reports whether a split is still current. This is the reason the
// source hash is recorded at all: a translation that silently went stale is
// worse than one that is obviously missing.
func runCheckMode(dir string) {
	if dir == "" {
		fmt.Println("error: pass the chunk directory with -dir (e.g. chunks/)")
		os.Exit(1)
	}
	abs, err := filepath.Abs(dir)
	if err != nil || !dirExists(abs) {
		fmt.Printf("error: directory not found: %s\n", dir)
		os.Exit(1)
	}
	m, err := job.Load(abs)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}

	done, missing := m.Progress()
	fmt.Printf("job %s - %s\n", m.ID, m.SourceFile)
	if p := m.Provenance; p.Tool != "" {
		fmt.Printf("split by %s %s at budget %d\n", p.Tool, p.Version, m.Size)
		if p.Model != "" {
			fmt.Printf("translated into %s by %s (%s mode)", p.TargetLang, p.Model, p.Mode)
			if p.Translated != "" {
				fmt.Printf(" on %s", p.Translated)
			}
			fmt.Println()
		}
	}
	fmt.Printf("parts: %d/%d translated", done, m.TotalParts)
	if len(missing) > 0 {
		fmt.Printf("; open: %v", missing)
	}
	fmt.Println()

	changed, known := m.SourceChanged()
	switch {
	case !known:
		fmt.Println("source: cannot tell - this manifest carries no source hash")
	case changed:
		fmt.Printf("source: CHANGED since the split - re-split and retranslate\n")
		os.Exit(1)
	default:
		fmt.Println("source: unchanged since the split")
	}
}

// roundtripVerdict vergleicht erst byte-genau (Canonical) und fällt nur dann
// auf den toleranten Vergleich zurück - so verschleiert die Normalisierung
// keine echte Abweichung mehr.
func roundtripVerdict(merged, orig string) string {
	switch {
	case split.Canonical(merged) == split.Canonical(orig):
		return "round-trip: byte-identical to the source"
	case split.Normalize(merged) == split.Normalize(orig):
		return "round-trip: identical apart from whitespace"
	default:
		return "round-trip: DIFFERS from the source - please check"
	}
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
