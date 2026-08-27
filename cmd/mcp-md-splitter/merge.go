package main

import (
	"fmt"
	"os"
	"path/filepath"

	"mcp-md-splitter/internal/job"
	"mcp-md-splitter/internal/split"
)

// runMergeMode setzt die Teile eines Splits wieder zusammen. Der Ordner muss
// eine index.json enthalten; je Teil wird die bearbeitete Fassung genommen,
// falls vorhanden, sonst das Original.
func runMergeMode(dir string, out string) {
	if dir == "" {
		fmt.Println("❌ Fehler: Bitte den Chunks-Ordner via -dir angeben (z. B. chunks/).")
		os.Exit(1)
	}
	abs, err := filepath.Abs(dir)
	if err != nil || !dirExists(abs) {
		fmt.Printf("❌ Ordner nicht gefunden: %s\n", dir)
		os.Exit(1)
	}

	m, err := job.Load(abs)
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		os.Exit(1)
	}
	paths, translated, err := m.MergePaths()
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		os.Exit(1)
	}

	merged, err := split.MergeFilesGaps(paths, m.Gaps)
	if err != nil {
		fmt.Printf("❌ Fehler beim Zusammenfügen: %v\n", err)
		os.Exit(1)
	}

	if out == "" {
		out = defaultMergeTarget(m)
	}
	if err := os.WriteFile(out, []byte(merged), 0o644); err != nil {
		fmt.Printf("❌ Fehler beim Schreiben von %s: %v\n", out, err)
		os.Exit(1)
	}

	fmt.Printf("🔗 %d Teile zusammengefügt → %s (%d Zeichen)\n", len(paths), out, len(merged))
	if translated > 0 {
		fmt.Printf("   davon bearbeitet: %d/%d\n", translated, m.TotalParts)
		return
	}
	if data, err := os.ReadFile(m.SourceFile); err == nil {
		fmt.Println(roundtripVerdict(merged, string(data)))
	}
}

// roundtripVerdict vergleicht erst byte-genau (Canonical) und fällt nur dann
// auf den toleranten Vergleich zurück - so verschleiert die Normalisierung
// keine echte Abweichung mehr.
func roundtripVerdict(merged, orig string) string {
	switch {
	case split.Canonical(merged) == split.Canonical(orig):
		return "✅ Roundtrip-Check: byte-genau identisch zum Original"
	case split.Normalize(merged) == split.Normalize(orig):
		return "✅ Roundtrip-Check: inhaltlich identisch (nur Whitespace weicht ab)"
	default:
		return "⚠️  Roundtrip-Check: weicht vom Original ab - bitte prüfen"
	}
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
