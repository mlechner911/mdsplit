package main

import (
	"fmt"
	"os"
	"path/filepath"

	"mcp-md-splitter/internal/job"
	"mcp-md-splitter/internal/split"
)

// runCLIMode exportiert Chunks + Manifest neben der Quelldatei.
func runCLIMode(path string, size int, target string) {
	if path == "" {
		fmt.Println("❌ Fehler: Bitte einen Dateipfad via -file angeben.")
		os.Exit(1)
	}
	if st, err := os.Stat(path); err != nil || st.IsDir() {
		fmt.Printf("❌ Datei nicht gefunden: %s\n", path)
		os.Exit(1)
	}

	fmt.Printf("📖 Verarbeite: %s (Budget %d Zeichen pro Chunk)\n", path, size)

	doc, err := split.SplitFileDoc(path, size)
	if err != nil {
		fmt.Printf("❌ Fehler beim Verarbeiten: %v\n", err)
		os.Exit(1)
	}
	if len(doc.Chunks) == 0 {
		fmt.Println("❌ Keine Chunks erstellt - Datei ist leer?")
		os.Exit(1)
	}

	outputDir := filepath.Join(filepath.Dir(path), "chunks")
	m := job.New(path, size, target, doc)
	if err := m.Write(outputDir, doc.Chunks); err != nil {
		fmt.Printf("❌ %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("📦 %d Teile geschrieben nach %s\n", m.TotalParts, outputDir)
	total, mx, mn := 0, 0, int(^uint(0)>>1)
	for _, p := range m.Parts {
		title := p.Heading
		if title == "" {
			title = "(ohne Überschrift)"
		}
		fmt.Printf("  ✅ %s (%d Zeichen) %s\n", p.File, p.Chars, title)
		total += p.Chars
		if p.Chars > mx {
			mx = p.Chars
		}
		if p.Chars < mn {
			mn = p.Chars
		}
	}

	fmt.Printf("\n📝 Manifest: %s\n🔑 jobId: %s\n", filepath.Join(outputDir, job.IndexName), m.ID)
	fmt.Printf("\n📊 Statistik:\n  Total: %d Zeichen\n  Durchschnitt: %d Zeichen\n  Min: %d Zeichen\n  Max: %d Zeichen\n",
		total, total/m.TotalParts, mn, mx)
}
