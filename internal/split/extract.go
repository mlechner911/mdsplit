package split

import (
	"regexp"
	"strings"
)

var (
	// headingRx: bis zu 3 Leerzeichen Einrückung, 1-6 '#', dann Whitespace
	// oder Zeilenende. Mehr Einrückung wäre in CommonMark kein Heading mehr.
	headingRx  = regexp.MustCompile(`^ {0,3}(#{1,6})(\s|$)`)
	tableRowRx = regexp.MustCompile(`^[ \t]*\|.*\|[ \t]*$`)
	listRx     = regexp.MustCompile(`^[ \t]*(?:[-*+]|\d{1,9}[.)])(\s|$)`)
	// fenceRx erlaubt beliebige Einrückung (Zäune stehen oft in Listenpunkten)
	// und eine beliebige Info-String-Zeile: ```js title="a.js", ``` go, ~~~~.
	fenceRx = regexp.MustCompile("^([ \t]*)(`{3,}|~{3,})(.*)$")
)

// fenceOpen prüft, ob die Zeile einen Code-Zaun öffnet, und liefert dessen
// Marker (die Zaun-Zeichen selbst) zurück. Bei Backtick-Zäunen darf der
// Info-String kein Backtick enthalten (CommonMark).
func fenceOpen(line string) (string, bool) {
	m := fenceRx.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	marker, info := m[2], m[3]
	if marker[0] == '`' && strings.Contains(info, "`") {
		return "", false
	}
	return marker, true
}

// fenceClose prüft, ob die Zeile den mit marker geöffneten Zaun schließt:
// gleiches Zeichen, mindestens gleiche Länge, danach nur noch Whitespace.
func fenceClose(line, marker string) bool {
	t := strings.TrimSpace(line)
	c := marker[0]
	n := 0
	for n < len(t) && t[n] == c {
		n++
	}
	return n >= len(marker) && strings.TrimSpace(t[n:]) == ""
}

// headingLevel liefert die Ebene einer Überschrift (1..6), sonst 0.
func headingLevel(line string) int {
	m := headingRx.FindStringSubmatch(line)
	if m == nil {
		return 0
	}
	return len(m[1])
}

// isIndentedCont erkennt eine eingerückte Fortsetzungszeile.
func isIndentedCont(line string) bool {
	return len(line) > 0 && (line[0] == ' ' || line[0] == '\t')
}

// ExtractBlocks zerlegt Markdown in atomare Blöcke. Code-Zäune, HTML-Blöcke,
// Tabellen und Listenpunkte bleiben ungeteilt; die Einrückung jeder Zeile
// bleibt erhalten. Die Leerzeilen zwischen zwei Blöcken landen als Gap im
// jeweils vorangehenden Block, damit der Rückweg exakt ist.
func ExtractBlocks(content string) []Block {
	lines := strings.Split(content, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1] // abschließendes "\n" erzeugt keine echte Zeile
	}
	lines = blankOut(lines)

	var blocks []Block
	i := 0
	for i < len(lines) {
		blank := 0
		for i < len(lines) && lines[i] == "" {
			blank++
			i++
		}
		if i >= len(lines) {
			break
		}
		if len(blocks) > 0 {
			blocks[len(blocks)-1].Gap = blank
		}

		start := i
		kind := Prose
		level := 0
		switch {
		case isFence(lines[i]):
			kind = Code
			i = consumeFence(lines, i)
		case htmlOpensBlock(lines[i]):
			kind = HTML
			i = consumeHTML(lines, i)
		case headingLevel(lines[i]) > 0:
			kind, level = Heading, headingLevel(lines[i])
			i++
		case tableRowRx.MatchString(lines[i]):
			kind = Table
			i = consumeTable(lines, i)
		case listRx.MatchString(lines[i]):
			kind = List
			i = consumeList(lines, i)
		default:
			i = consumeProse(lines, i)
		}

		end := i
		if kind != Code {
			// Der HTML-Abbruch kann Leerzeilen am Blockende hinterlassen;
			// die gehören als Gap in die nächste Runde.
			for end > start && lines[end-1] == "" {
				end--
			}
			i = end
		}
		blocks = append(blocks, Block{
			Text:  strings.Join(lines[start:end], "\n"),
			Kind:  kind,
			Level: level,
		})
	}
	return blocks
}

func isFence(line string) bool {
	_, ok := fenceOpen(line)
	return ok
}

// consumeFence schluckt den Zaun bis zur schließenden Zeile (einschließlich).
// Ein nie geschlossener Zaun reicht bis Dateiende - genau wie in CommonMark.
func consumeFence(lines []string, i int) int {
	marker, _ := fenceOpen(lines[i])
	for j := i + 1; j < len(lines); j++ {
		if fenceClose(lines[j], marker) {
			return j + 1
		}
	}
	return len(lines)
}

// consumeTable fasst aufeinanderfolgende Tabellenzeilen zusammen.
func consumeTable(lines []string, i int) int {
	j := i + 1
	for j < len(lines) && tableRowRx.MatchString(lines[j]) {
		j++
	}
	return j
}

// opensVerbatim meldet <pre>/<code>: darin sind '#'-Zeilen Inhalt
// (Shell-Kommentare) und dürfen den HTML-Block nicht beenden.
func opensVerbatim(line string) bool {
	l := strings.ToLower(stripInline(line))
	return strings.Contains(l, "<pre") || strings.Contains(l, "<code")
}

// consumeHTML liest einen HTML-Block über einen Tag-Zähler ein. Damit ein
// vergessener schließender Tag nicht den Rest des Dokuments verschluckt,
// bricht der Block ab, sobald eine Überschriftenzeile oder zwei aufeinander
// folgende Leerzeilen auftauchen.
func consumeHTML(lines []string, i int) int {
	bal := htmlLineDelta(lines[i])
	verbatim := opensVerbatim(lines[i])
	j := i + 1
	for j < len(lines) && bal > 0 {
		if lines[j] == "" {
			k := j
			for k < len(lines) && lines[k] == "" {
				k++
			}
			if k-j >= 2 {
				return j // Abbruch: Leerzeilenlauf beendet den Block
			}
			j = k
			continue
		}
		if !verbatim && headingLevel(lines[j]) > 0 {
			return j // Abbruch: Überschrift beendet den Block
		}
		if opensVerbatim(lines[j]) {
			verbatim = true
		}
		bal += htmlLineDelta(lines[j])
		j++
	}
	return j
}

// consumeList liest einen Listenpunkt samt Fortsetzungen. Eingerückte
// Fortsetzungsabsätze (auch über eine Leerzeile hinweg) und Code-Zäune im
// Punkt gehören dazu; ein Geschwister-Item nach einer Leerzeile beendet den
// Block, damit lange Listen an Item-Grenzen teilbar bleiben.
func consumeList(lines []string, i int) int {
	j := i + 1
	for j < len(lines) {
		l := lines[j]
		if l == "" {
			k := j
			for k < len(lines) && lines[k] == "" {
				k++
			}
			if k < len(lines) && isIndentedCont(lines[k]) && !listRx.MatchString(lines[k]) {
				j = k
				continue
			}
			return j
		}
		if isFence(l) {
			j = consumeFence(lines, j)
			continue
		}
		if headingLevel(l) > 0 || tableRowRx.MatchString(l) || htmlOpensBlock(l) {
			return j
		}
		j++ // Listenzeile, Einrückung oder Lazy Continuation
	}
	return j
}

// consumeProse liest einen Absatz bis zur nächsten Leerzeile oder bis eine
// Konstruktion beginnt, die einen eigenen Block bildet.
func consumeProse(lines []string, i int) int {
	j := i + 1
	for j < len(lines) {
		l := lines[j]
		if l == "" || headingLevel(l) > 0 || isFence(l) ||
			tableRowRx.MatchString(l) || listRx.MatchString(l) || htmlOpensBlock(l) {
			return j
		}
		j++
	}
	return j
}

// FirstHeading liefert die erste Überschriftenzeile eines Chunks (getrimmt),
// sonst "". Nützlich, um einem Chunk im Manifest einen Namen zu geben.
func FirstHeading(chunk string) string {
	for _, l := range strings.Split(chunk, "\n") {
		if headingLevel(l) > 0 {
			return strings.TrimSpace(l)
		}
	}
	return ""
}
