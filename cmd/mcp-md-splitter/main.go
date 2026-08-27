package main

import (
	"flag"
)

// version ist die Server-Version für MCP-Clients; wird bei Bedarf via ldflags gesetzt.
var version = "1.1.0"

func main() {
	cliMode := flag.Bool("cli", false, "Aktiviert den eigenständigen CLI-Export")
	filePath := flag.String("file", "", "Pfad zur Markdown-Datei (CLI)")
	chunkSize := flag.Int("size", 8000, "Maximale Zeichenanzahl pro Chunk")
	mergeMode := flag.Bool("merge", false, "Setzt Chunks wieder zusammen (Rückweg)")
	chunksDir := flag.String("dir", "", "Chunks-Ordner mit index.json (Merge)")
	outFile := flag.String("out", "", "Zielpfad der Merge-Ausgabe (Standard: <Quelle>.merged)")
	target := flag.String("target", "", "Kürzel der bearbeiteten Fassung, z. B. de (Standard: out)")
	flag.Parse()

	switch {
	case *mergeMode:
		runMergeMode(*chunksDir, *outFile)
	case *cliMode:
		runCLIMode(*filePath, *chunkSize, *target)
	default:
		runMCPMode()
	}
}
