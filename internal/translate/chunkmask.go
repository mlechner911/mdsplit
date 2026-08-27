package translate

import (
	"fmt"
	"strings"

	"mcp-md-splitter/internal/split"
)

// maskChunk prepares a whole chunk for chunk mode: code fences and HTML blocks
// are replaced by a sentinel, and the protected inline spans are masked too.
//
// This is the middle ground between the two modes. The model still sees the
// chunk's prose as one connected text - which is what translation quality
// actually depends on, because word order is decided across a clause, not
// within it - while the code it could damage is not in front of it at all.
// It also roughly halves the input: on typical technical Markdown, a quarter to
// a third of the bytes are code, which matters when the context window is small.
func maskChunk(chunk string) (string, []string) {
	blocks := split.ExtractBlocks(chunk)
	var tokens []string
	var b strings.Builder

	for i, blk := range blocks {
		switch blk.Kind {
		case split.Code, split.HTML:
			tokens = append(tokens, blk.Text)
			fmt.Fprintf(&b, "⟦%d⟧", len(tokens)-1)
		case split.List:
			// Ein Zaun im Listenpunkt ist ebenso Code, steckt aber in einem
			// Listenblock - er muss genauso verschwinden.
			b.WriteString(protectInto(maskFences(blk.Text, &tokens), &tokens))
		default:
			b.WriteString(protectInto(blk.Text, &tokens))
		}
		if i+1 < len(blocks) {
			b.WriteString(strings.Repeat("\n", blk.Gap+1))
		}
	}
	return b.String(), tokens
}

// maskFences replaces every fenced run inside a text with a sentinel.
func maskFences(text string, tokens *[]string) string {
	lines := strings.Split(text, "\n")
	var out []string
	for i := 0; i < len(lines); i++ {
		marker, ok := fenceMarker(lines[i])
		if !ok {
			out = append(out, lines[i])
			continue
		}
		start := i
		for i++; i < len(lines); i++ {
			if isClosing(lines[i], marker) {
				break
			}
		}
		end := i
		if end >= len(lines) {
			end = len(lines) - 1
		}
		*tokens = append(*tokens, strings.Join(lines[start:end+1], "\n"))
		out = append(out, fmt.Sprintf("⟦%d⟧", len(*tokens)-1))
	}
	return strings.Join(out, "\n")
}
