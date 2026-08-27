package split

import (
	"fmt"
	"os"
)

// group bündelt Blöcke, zwischen denen nicht getrennt werden darf.
type group struct {
	from, to int // Blockbereich [from,to)
	size     int
	heading  bool // Gruppe beginnt mit einer Überschrift
	level    int  // deren Ebene (1..6), sonst 0
	atomic   bool // enthält Code oder HTML: nie teilen, egal wie groß
}

// groupBlocks bildet die unteilbaren Einheiten. Zusammengebunden wird,
//   - wenn zwischen zwei Blöcken keine Leerzeile stand (sie kleben im Original
//     aneinander, etwa Tabelle direkt unter ihrem Einleitungssatz), und
//   - hinter einer Überschrift, damit nie eine Überschrift allein am Chunk-Ende
//     stehenbleibt und ihr Abschnitt im nächsten Chunk beginnt.
func groupBlocks(blocks []Block) []group {
	var gs []group
	i := 0
	for i < len(blocks) {
		j := i + 1
		for j < len(blocks) && (blocks[j-1].Gap == 0 || blocks[j-1].Kind == Heading) {
			j++
		}
		g := group{from: i, to: j, size: rangeSize(blocks, i, j)}
		if blocks[i].Kind == Heading {
			g.heading, g.level = true, blocks[i].Level
		}
		for k := i; k < j; k++ {
			if blocks[k].Kind == Code || blocks[k].Kind == HTML {
				g.atomic = true
			}
		}
		gs = append(gs, g)
		i = j
	}
	return gs
}

// minFill ist die Füllmenge, ab der eine Überschrift als Schnittpunkt taugt.
// Die Zielgröße ist ein weiches Budget: lieber ein kleinerer Chunk, der an
// einer Abschnittsgrenze endet, als ein voller, der mitten im Abschnitt bricht.
// Obere Ebenen (h1/h2) schneiden früher als tiefe.
func minFill(maxSize, level int) int {
	if level <= 2 {
		return maxSize / 4
	}
	return maxSize / 2
}

// packRanges verteilt die Gruppen auf Chunks. Eine Gruppe landet immer
// vollständig in genau einem Chunk - auch dann, wenn sie allein schon größer
// als maxSize ist (Code- und HTML-Blöcke bleiben komplett).
func packRanges(gs []group, maxSize int) [][2]int {
	var out [][2]int
	start := 0
	for start < len(gs) {
		curLen := 0
		lastHeading, lenAtHeading := -1, 0
		i := start
		for i < len(gs) {
			sep := 2 // Leerzeile zwischen zwei Gruppen
			if i == start {
				sep = 0
			}
			if i > start {
				// Bevorzugter Schnitt: vor einer Überschrift, sobald der
				// laufende Chunk genug Substanz hat.
				if gs[i].heading && curLen >= minFill(maxSize, gs[i].level) {
					break
				}
				if curLen+sep+gs[i].size > maxSize {
					break
				}
			}
			if gs[i].heading && i > start {
				lastHeading, lenAtHeading = i, curLen
			}
			curLen += sep + gs[i].size
			i++
		}
		if i == start {
			i = start + 1 // eine einzelne zu große Gruppe bekommt ihren Chunk
		}
		// Wenn wegen der Größe gebrochen wurde und weiter vorn eine
		// Überschrift lag, dorthin zurückziehen - der Abschnitt bleibt ganz.
		if i < len(gs) && !gs[i].heading &&
			lastHeading > start && lastHeading < i && lenAtHeading >= maxSize/4 {
			i = lastHeading
		}
		out = append(out, [2]int{start, i})
		start = i
	}
	return out
}

// PackChunks gruppiert Blöcke zu Chunks von höchstens maxSize Zeichen.
// Ein Block wandert nie in zwei Chunks; unteilbare Blöcke (Code, HTML) dürfen
// das Budget überschreiten und werden dann auf stderr gemeldet.
func PackChunks(blocks []Block, maxSize int) []string {
	return SplitBlocks(blocks, maxSize).Chunks
}

// Doc ist das Ergebnis eines Splits: die Chunks und die Leerzeilen-Abstände,
// die im Original an den Chunk-Grenzen standen. Gaps hat len(Chunks)-1
// Einträge und macht den Rückweg byte-genau.
type Doc struct {
	Chunks []string
	Gaps   []int
}

// SplitBlocks packt bereits extrahierte Blöcke.
func SplitBlocks(blocks []Block, maxSize int) Doc {
	if maxSize < 1 {
		maxSize = 1
	}
	gs := groupBlocks(blocks)
	ranges := packRanges(gs, maxSize)

	var doc Doc
	for n, r := range ranges {
		from, to := gs[r[0]].from, gs[r[1]-1].to
		doc.Chunks = append(doc.Chunks, render(blocks, from, to))
		if n > 0 {
			prevEnd := gs[ranges[n-1][1]-1].to
			doc.Gaps = append(doc.Gaps, blocks[prevEnd-1].Gap)
		}
	}

	for i, c := range doc.Chunks {
		if len(c) > maxSize {
			fmt.Fprintf(os.Stderr, "ℹ️  Chunk %d: %d Zeichen (Budget %d) - unteilbarer Block\n", i+1, len(c), maxSize)
		}
	}
	return doc
}

// SplitDoc zerlegt Inhalt in Chunks samt Grenz-Abständen.
func SplitDoc(content string, maxSize int) Doc {
	return SplitBlocks(ExtractBlocks(content), maxSize)
}

// Split liefert nur die Chunks.
func Split(content string, maxSize int) []string {
	return SplitDoc(content, maxSize).Chunks
}

// SplitFile liest eine Markdown-Datei und liefert deren Chunks.
func SplitFile(path string, maxSize int) ([]string, error) {
	doc, err := SplitFileDoc(path, maxSize)
	if err != nil {
		return nil, err
	}
	return doc.Chunks, nil
}

// SplitFileDoc liest eine Markdown-Datei und liefert Chunks samt Abständen.
func SplitFileDoc(path string, maxSize int) (Doc, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Doc{}, fmt.Errorf("datei lesen: %w", err)
	}
	return SplitDoc(string(data), maxSize), nil
}
