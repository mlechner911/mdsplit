package split

import "strings"

// Kind klassifiziert einen atomaren Markdown-Block.
type Kind uint8

const (
	Prose Kind = iota
	Heading
	Code
	Table
	List
	HTML
)

// Block ist ein atomarer Markdown-Abschnitt. Text enthält die Quellzeilen
// unverändert – insbesondere bleibt die Einrückung erhalten, weil sie in
// Markdown bedeutungstragend ist (Code-Zaun im Listenpunkt, Fortsetzungs-
// absatz, eingerücktes Blockquote). Gap zählt die Leerzeilen, die im
// Original zwischen diesem und dem nächsten Block standen; nur so lässt
// sich der Rückweg byte-genau rekonstruieren.
type Block struct {
	Text  string
	Gap   int
	Kind  Kind
	Level int // nur bei Kind == Heading: 1..6
}

// isBlank erkennt Zeilen, die nur aus Whitespace bestehen.
func isBlank(line string) bool {
	return strings.TrimSpace(line) == ""
}

// blankOut ersetzt reine Whitespace-Zeilen durch die leere Zeile. Das ist die
// einzige Normalisierung, die der Splitter am Inhalt vornimmt (siehe Canonical).
func blankOut(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		if isBlank(l) {
			out[i] = ""
			continue
		}
		out[i] = l
	}
	return out
}

// render setzt die Blöcke [from,to) mit ihren originalen Leerzeilen-Abständen
// wieder zusammen.
func render(blocks []Block, from, to int) string {
	var b strings.Builder
	for k := from; k < to; k++ {
		b.WriteString(blocks[k].Text)
		if k+1 < to {
			b.WriteString(strings.Repeat("\n", blocks[k].Gap+1))
		}
	}
	return b.String()
}

// rangeSize liefert die Zeichenlänge, die render(blocks, from, to) ergäbe.
func rangeSize(blocks []Block, from, to int) int {
	n := 0
	for k := from; k < to; k++ {
		n += len(blocks[k].Text)
		if k+1 < to {
			n += blocks[k].Gap + 1
		}
	}
	return n
}
