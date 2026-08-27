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
		fmt.Println("error: pass a file path with -file")
		os.Exit(1)
	}
	if st, err := os.Stat(path); err != nil || st.IsDir() {
		fmt.Printf("error: file not found: %s\n", path)
		os.Exit(1)
	}

	fmt.Printf("reading %s (budget %d chars per chunk)\n", path, size)

	doc, err := split.SplitFileDoc(path, size)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
	if len(doc.Chunks) == 0 {
		fmt.Println("error: no chunks produced - is the file empty?")
		os.Exit(1)
	}

	outputDir := filepath.Join(filepath.Dir(path), "chunks")
	m := job.New(path, size, target, doc)
	if err := m.Write(outputDir, doc.Chunks); err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("wrote %d parts to %s\n", m.TotalParts, outputDir)
	total, mx, mn := 0, 0, int(^uint(0)>>1)
	for _, p := range m.Parts {
		title := p.Heading
		if title == "" {
			title = "(no heading)"
		}
		fmt.Printf("  %s  %6d chars  %s\n", p.File, p.Chars, title)
		total += p.Chars
		if p.Chars > mx {
			mx = p.Chars
		}
		if p.Chars < mn {
			mn = p.Chars
		}
	}

	fmt.Printf("\nmanifest: %s\njobId:    %s\n", filepath.Join(outputDir, job.IndexName), m.ID)
	fmt.Printf("\nstats: %d chars total, %d average, %d min, %d max\n",
		total, total/m.TotalParts, mn, mx)
}
