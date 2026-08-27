package split

import (
	"regexp"
	"strings"
)

// tagRx extrahiert HTML-Tags samt Tag-Namen aus einer Zeile.
var tagRx = regexp.MustCompile(`<\/?([a-zA-Z][a-zA-Z0-9]*)[^>]*>`)

// codeSpanRx trifft Inline-Code. Ein `<div>` in Backticks ist Text über HTML,
// kein HTML - sonst reißt jede Doku, die über Tags schreibt, einen Block auf.
var codeSpanRx = regexp.MustCompile("`+[^`]*`+")

// commentRx trifft HTML-Kommentare, die keine Blockbilanz verändern.
var commentRx = regexp.MustCompile(`(?s)<!--.*?-->`)

// htmlBlockStartRx: ein HTML-Block beginnt am Zeilenanfang (bis zu drei
// Leerzeichen Einrückung), nicht mitten in einem Satz.
var htmlBlockStartRx = regexp.MustCompile(`^ {0,3}<[a-zA-Z/]`)

// stripInline entfernt Inline-Code und Kommentare vor der Tag-Analyse.
func stripInline(line string) string {
	return commentRx.ReplaceAllString(codeSpanRx.ReplaceAllString(line, ""), "")
}

// htmlOpensBlock meldet, ob die Zeile einen mehrzeiligen HTML-Block eröffnet.
func htmlOpensBlock(line string) bool {
	return htmlBlockStartRx.MatchString(line) && htmlLineDelta(line) > 0
}

// htmlBlocks sind tags, die einen Block öffnen/abschließen (Stack-Parser).
var htmlBlocks = map[string]bool{
	"div": true, "p": true, "table": true, "section": true, "article": true,
	"main": true, "aside": true, "header": true, "footer": true, "nav": true,
	"details": true, "figure": true, "form": true, "ul": true, "ol": true,
	"blockquote": true, "pre": true, "h1": true, "h2": true, "h3": true,
	"h4": true, "h5": true, "h6": true,
}

// voidTags sind selbsterfüllend (einzeilige HTML-Blöcke).
var voidTags = map[string]bool{
	"hr": true, "br": true, "img": true, "iframe": true, "meta": true,
	"link": true, "input": true, "source": true, "audio": true, "video": true,
}

// htmlLineDelta liefert die Netto-Öffnungsbilanz von HTML-Block-Tags in einer
// Zeile: +1 pro öffnendem Block-Tag, -1 pro schließendem; Void-Tags und
// Self-Closing zählen 0. Ergebnis > 0 triggert die atomare Block-Aufnahme.
func htmlLineDelta(line string) int {
	line = stripInline(line)
	bal := 0
	for _, m := range tagRx.FindAllStringSubmatchIndex(line, -1) {
		full := line[m[0]:m[1]]
		tname := strings.ToLower(line[m[2]:m[3]])
		if !htmlBlocks[tname] || voidTags[tname] {
			continue
		}
		switch {
		case strings.HasPrefix(full, "</"):
			bal--
		case !strings.HasSuffix(full, "/>"):
			bal++
		}
	}
	return bal
}
