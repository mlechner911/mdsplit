package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/mlechner911/mdsplit/internal/split"
)

// runOutlineMode prints a document's headings, or one section of it.
//
// Deliberately jobless: no chunks, no manifest, no registry. That apparatus
// exists for processing a whole document. Looking something up wants none of
// it - you read one section and leave.
func runOutlineMode(path, section string) {
	if path == "" {
		fmt.Println("error: pass a file path with -file")
		os.Exit(1)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}

	if section != "" {
		text, err := split.Section(string(data), section)
		if err != nil {
			fmt.Printf("error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(text)
		return
	}

	topics := split.Outline(string(data))
	if len(topics) == 0 {
		fmt.Println("no headings in this document")
		return
	}
	for _, t := range topics {
		fmt.Printf("%6d  %5d  %s%s\n", t.Line, t.Chars,
			strings.Repeat("  ", t.Level-1), t.Title)
	}
	fmt.Printf("\n%d headings. Read one with -section \"<title or path>\".\n", len(topics))
}
