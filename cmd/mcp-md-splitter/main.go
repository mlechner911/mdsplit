package main

import (
	"flag"
)

// version ist die Server-Version für MCP-Clients; wird bei Bedarf via ldflags gesetzt.
var version = "1.1.0"

func main() {
	cliMode := flag.Bool("cli", false, "run the standalone CLI export instead of the MCP server")
	filePath := flag.String("file", "", "path to the Markdown file (CLI mode)")
	chunkSize := flag.Int("size", 8000, "soft character budget per chunk; indivisible blocks may exceed it")
	mergeMode := flag.Bool("merge", false, "reassemble a split back into one document")
	chunksDir := flag.String("dir", "", "chunk directory containing index.json (merge mode)")
	outFile := flag.String("out", "", "merge output path (default: <source>.merged)")
	target := flag.String("target", "", "suffix for edited parts, e.g. de (default: out)")
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
