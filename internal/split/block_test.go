package split

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// blockTexts ist der Textinhalt aller Blöcke - bequem für Contains-Prüfungen.
func blockTexts(bs []Block) []string {
	out := make([]string, len(bs))
	for i, b := range bs {
		out[i] = b.Text
	}
	return out
}

// findBlock liefert den ersten Block, dessen Text alle Teilstrings enthält.
func findBlock(bs []Block, parts ...string) (Block, bool) {
	for _, b := range bs {
		ok := true
		for _, p := range parts {
			if !strings.Contains(b.Text, p) {
				ok = false
				break
			}
		}
		if ok {
			return b, true
		}
	}
	return Block{}, false
}

func TestExtractBlocks_CodeFence(t *testing.T) {
	src := "Intro text.\n\n```bash\necho a\n```\n\nOutro."
	blocks := ExtractBlocks(src)
	b, ok := findBlock(blocks, "```bash", "echo a")
	if !ok {
		t.Fatalf("kein atomarer Code-Block in %+v", blockTexts(blocks))
	}
	if b.Kind != Code {
		t.Errorf("Kind = %v, erwartet Code", b.Kind)
	}
	if !strings.HasSuffix(b.Text, "```") {
		t.Errorf("Zaun nicht geschlossen: %q", b.Text)
	}
}

// TestExtractBlocks_FenceInfoString deckt den Bug ab, an dem Zäune mit
// Attributen oder Leerzeichen im Info-String nicht als Zaun erkannt wurden -
// der Split lief dann mitten durch den Code.
func TestExtractBlocks_FenceInfoString(t *testing.T) {
	cases := []string{
		"```js title=\"a.js\"",
		"``` go",
		"```python {highlight=1-3}",
		"~~~~yaml",
		"````md",
	}
	for _, open := range cases {
		t.Run(open, func(t *testing.T) {
			marker, ok := fenceOpen(open)
			if !ok {
				t.Fatalf("%q nicht als Zaun-Öffner erkannt", open)
			}
			src := "Vorher.\n\n" + open + "\ncode\n\nnoch code\n" + marker + "\n\nNachher."
			blocks := ExtractBlocks(src)
			b, found := findBlock(blocks, open)
			if !found {
				t.Fatalf("Zaun-Block fehlt in %+v", blockTexts(blocks))
			}
			if b.Kind != Code {
				t.Errorf("Kind = %v, erwartet Code", b.Kind)
			}
			if !strings.Contains(b.Text, "noch code") {
				t.Errorf("Zaun über Leerzeile hinweg geteilt: %q", b.Text)
			}
			// Der Prosatext darf nicht im Code-Block gelandet sein.
			if strings.Contains(b.Text, "Nachher") {
				t.Errorf("Parser lief über den Zaun hinaus: %q", b.Text)
			}
		})
	}
}

// TestFenceClose: nur gleiches Zeichen und mindestens gleiche Länge schließen.
func TestFenceClose(t *testing.T) {
	cases := []struct {
		line, marker string
		want         bool
	}{
		{"```", "```", true},
		{"  ```  ", "```", true},
		{"````", "```", true},
		{"``", "```", false},
		{"~~~", "```", false},
		{"```go", "```", false},
		{"```", "````", false},
	}
	for _, c := range cases {
		if got := fenceClose(c.line, c.marker); got != c.want {
			t.Errorf("fenceClose(%q, %q) = %v, erwartet %v", c.line, c.marker, got, c.want)
		}
	}
}

// TestExtractBlocks_IndentationPreserved: Einrückung ist in Markdown
// bedeutungstragend und darf den Split nicht überleben-verlieren.
func TestExtractBlocks_IndentationPreserved(t *testing.T) {
	src := "- Punkt:\n\n  ```go\n  func a(){}\n  ```\n\n- Zweiter\n"
	blocks := ExtractBlocks(src)
	b, ok := findBlock(blocks, "func a(){}")
	if !ok {
		t.Fatalf("Zaun im Listenpunkt fehlt: %+v", blockTexts(blocks))
	}
	if !strings.Contains(b.Text, "  ```go") {
		t.Errorf("Einrückung des Zauns verloren: %q", b.Text)
	}
}

func TestExtractBlocks_Table(t *testing.T) {
	src := "Intro\n\n| A | B |\n|---|---|\n| 1 | 2 |\n\nAfter"
	blocks := ExtractBlocks(src)
	b, ok := findBlock(blocks, "| A | B |", "| 1 | 2 |")
	if !ok {
		t.Fatalf("Tabelle ist nicht als Block zusammen: %+v", blockTexts(blocks))
	}
	if b.Kind != Table {
		t.Errorf("Kind = %v, erwartet Table", b.Kind)
	}
}

func TestExtractBlocks_List(t *testing.T) {
	src := "Intro\n\n- item one\n  continuation line\n- item two\n\nAfter"
	blocks := ExtractBlocks(src)
	if _, ok := findBlock(blocks, "- item one", "continuation line", "- item two"); !ok {
		t.Fatalf("Liste ist nicht als Block zusammen: %+v", blockTexts(blocks))
	}
}

// TestExtractBlocks_LooseListItem: ein Fortsetzungsabsatz hinter einer
// Leerzeile gehört zum Item und behält seine Einrückung.
func TestExtractBlocks_LooseListItem(t *testing.T) {
	src := "1. Eins\n\n   Fortsetzung von eins.\n\n2. Zwei\n"
	blocks := ExtractBlocks(src)
	b, ok := findBlock(blocks, "1. Eins")
	if !ok {
		t.Fatalf("Listenblock fehlt: %+v", blockTexts(blocks))
	}
	if !strings.Contains(b.Text, "   Fortsetzung von eins.") {
		t.Errorf("Fortsetzungsabsatz fehlt oder ist entrückt: %q", b.Text)
	}
	if strings.Contains(b.Text, "2. Zwei") {
		t.Errorf("Geschwister-Item sollte trennbar bleiben: %q", b.Text)
	}
}

func TestExtractBlocks_HTMLDivStaysTogether(t *testing.T) {
	src := "Intro.\n\n<div class=\"box\">\n  Zelle eins\n  Zelle zwei\n</div>\n\nOutro."
	blocks := ExtractBlocks(src)
	b, ok := findBlock(blocks, "<div", "</div>", "Zelle eins", "Zelle zwei")
	if !ok {
		t.Fatalf("Multi-Zeilen-<div> wurde aufgeteilt: %+v", blockTexts(blocks))
	}
	if b.Kind != HTML {
		t.Errorf("Kind = %v, erwartet HTML", b.Kind)
	}
	if strings.Contains(b.Text, "Outro") {
		t.Errorf("HTML-Block frisst den Folgetext: %q", b.Text)
	}
}

// TestExtractBlocks_UnclosedHTMLStops: ein vergessener schließender Tag darf
// nicht den Rest des Dokuments verschlucken.
func TestExtractBlocks_UnclosedHTMLStops(t *testing.T) {
	t.Run("Überschrift bricht ab", func(t *testing.T) {
		src := "<div>\nInhalt\n# Überschrift\n\nText danach.\n"
		blocks := ExtractBlocks(src)
		b, ok := findBlock(blocks, "<div>")
		if !ok {
			t.Fatalf("HTML-Block fehlt: %+v", blockTexts(blocks))
		}
		if strings.Contains(b.Text, "Überschrift") {
			t.Errorf("Abbruch an der Überschrift hat nicht gegriffen: %q", b.Text)
		}
		if _, ok := findBlock(blocks, "# Überschrift"); !ok {
			t.Errorf("Überschrift wurde nicht als eigener Block erkannt: %+v", blockTexts(blocks))
		}
	})
	t.Run("zwei Leerzeilen brechen ab", func(t *testing.T) {
		src := "<div>\nInhalt\n\n\nText danach.\n"
		blocks := ExtractBlocks(src)
		b, ok := findBlock(blocks, "<div>")
		if !ok {
			t.Fatalf("HTML-Block fehlt: %+v", blockTexts(blocks))
		}
		if strings.Contains(b.Text, "Text danach") {
			t.Errorf("Abbruch am Leerzeilenlauf hat nicht gegriffen: %q", b.Text)
		}
	})
	t.Run("pre schluckt Rautenzeilen", func(t *testing.T) {
		src := "<pre>\n# kein Heading, ein Shell-Kommentar\necho a\n</pre>\n"
		blocks := ExtractBlocks(src)
		b, ok := findBlock(blocks, "<pre>")
		if !ok {
			t.Fatalf("pre-Block fehlt: %+v", blockTexts(blocks))
		}
		if !strings.Contains(b.Text, "echo a") {
			t.Errorf("pre wurde an der Rautenzeile zerschnitten: %q", b.Text)
		}
	})
}

// TestExtractBlocks_InlineTagIsNotHTML: ein `<div>` in Backticks oder mitten
// im Satz eröffnet keinen HTML-Block - sonst verschluckt ein Satz über HTML
// den Rest des Dokuments.
func TestExtractBlocks_InlineTagIsNotHTML(t *testing.T) {
	cases := []string{
		"Der Text wird in ein `<pre>` gerendert und dann gedruckt.",
		"Wir bauen das in ein <div>, das reicht.",
	}
	for _, line := range cases {
		t.Run(line[:20], func(t *testing.T) {
			src := line + "\n\n# Danach\n\nNoch ein Absatz.\n"
			blocks := ExtractBlocks(src)
			if blocks[0].Kind == HTML {
				t.Errorf("Inline-Tag als HTML-Block gewertet: %q", blocks[0].Text)
			}
			if strings.Contains(blocks[0].Text, "Danach") {
				t.Errorf("Block frisst den Rest des Dokuments: %q", blocks[0].Text)
			}
		})
	}
}

func TestExtractBlocks_HeadingOwnBlock(t *testing.T) {
	src := "Some prose.\n\n### Titel\n\nMore prose."
	blocks := ExtractBlocks(src)
	b, ok := findBlock(blocks, "### Titel")
	if !ok {
		t.Fatalf("Überschrift ist kein eigener Block: %+v", blockTexts(blocks))
	}
	if b.Kind != Heading || b.Level != 3 {
		t.Errorf("Kind/Level = %v/%d, erwartet Heading/3", b.Kind, b.Level)
	}
}

func TestExtractBlocks_Empty(t *testing.T) {
	if blocks := ExtractBlocks(""); len(blocks) != 0 {
		t.Fatalf("erwartet 0 Blöcke, bekommen %d: %+v", len(blocks), blockTexts(blocks))
	}
	if blocks := ExtractBlocks("\n\n\n   \n"); len(blocks) != 0 {
		t.Fatalf("Leergang soll 0 Blöcke ergeben, bekommen %d", len(blocks))
	}
}

// TestExtractBlocks_GapRecorded: der Abstand zwischen Blöcken wird gemerkt.
func TestExtractBlocks_GapRecorded(t *testing.T) {
	blocks := ExtractBlocks("# Titel\nDirekt darunter.\n\nAbsatz.\n\n\nWeit weg.\n")
	if len(blocks) != 4 {
		t.Fatalf("erwartet 4 Blöcke, bekommen %d: %+v", len(blocks), blockTexts(blocks))
	}
	want := []int{0, 1, 2}
	for i, w := range want {
		if blocks[i].Gap != w {
			t.Errorf("Block %d: Gap = %d, erwartet %d", i, blocks[i].Gap, w)
		}
	}
}

// --- Packing -----------------------------------------------------------

// assertNoSplitBlocks prüft, dass jeder Block genau einmal vorkommt.
func assertNoSplitBlocks(t *testing.T, chunks []string, blocks []Block) {
	t.Helper()
	for _, blk := range blocks {
		hits := 0
		for _, c := range chunks {
			if strings.Contains(c, blk.Text) {
				hits++
			}
		}
		if hits != 1 {
			t.Fatalf("Block in %d Chunks statt genau 1: %.60q", hits, blk.Text)
		}
	}
}

func TestPackChunks_SizeLimit(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&b, "## Abschnitt %02d\n\n%s %02d\n\n", i, strings.Repeat("x", 100), i)
	}
	blocks := ExtractBlocks(b.String())
	const maxSize = 300
	chunks := PackChunks(blocks, maxSize)
	if len(chunks) < 2 {
		t.Fatalf("erwartet > 1 Chunk, bekommen %d", len(chunks))
	}
	for i, c := range chunks {
		if len(c) > maxSize {
			t.Errorf("Chunk %d: %d Zeichen über Budget %d ohne unteilbaren Block", i+1, len(c), maxSize)
		}
	}
	assertNoSplitBlocks(t, chunks, blocks)
}

// TestPackChunks_AtomicOversized: Code bleibt komplett, auch weit über Budget.
func TestPackChunks_AtomicOversized(t *testing.T) {
	big := "```bash\n" + strings.Repeat("echo sehr langes zeichen\n", 50) + "```"
	src := "kleiner einstieg\n\n" + big + "\n\nklarer ausgang\n"
	blocks := ExtractBlocks(src)
	chunks := PackChunks(blocks, 100)

	found := false
	for _, c := range chunks {
		if strings.Contains(c, "```bash") {
			found = true
			if strings.Count(c, "echo sehr langes zeichen") != 50 {
				t.Errorf("Code-Block unvollständig: %d von 50 Zeilen", strings.Count(c, "echo sehr langes zeichen"))
			}
		}
	}
	if !found {
		t.Fatal("großer Code-Block wurde nirgends gefunden")
	}
	assertNoSplitBlocks(t, chunks, blocks)
}

// TestPackChunks_NoOrphanHeading: eine Überschrift steht nie allein am Ende.
func TestPackChunks_NoOrphanHeading(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 12; i++ {
		fmt.Fprintf(&b, "## Abschnitt %02d\n\n%s %02d\n\n", i, strings.Repeat("y", 120), i)
	}
	blocks := ExtractBlocks(b.String())
	chunks := PackChunks(blocks, 400)
	if len(chunks) < 2 {
		t.Fatalf("zu wenige Chunks für den Test: %d", len(chunks))
	}
	for i, c := range chunks {
		lines := strings.Split(strings.TrimRight(c, "\n"), "\n")
		if headingLevel(lines[len(lines)-1]) > 0 {
			t.Errorf("Chunk %d endet auf einer verwaisten Überschrift:\n%s", i+1, c)
		}
	}
}

// TestPackChunks_CutsAtHeadings: Chunks beginnen bevorzugt an Abschnitten.
func TestPackChunks_CutsAtHeadings(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 10; i++ {
		fmt.Fprintf(&b, "## Abschnitt %02d\n\n%s %02d\n\n", i, strings.Repeat("z", 200), i)
	}
	chunks := Split(b.String(), 600)
	if len(chunks) < 3 {
		t.Fatalf("zu wenige Chunks: %d", len(chunks))
	}
	for i, c := range chunks {
		if i == 0 {
			continue
		}
		if headingLevel(strings.SplitN(c, "\n", 2)[0]) == 0 {
			t.Errorf("Chunk %d beginnt nicht an einer Überschrift:\n%.80s", i+1, c)
		}
	}
}

func TestPackChunks_Empty(t *testing.T) {
	if chunks := PackChunks(nil, 100); len(chunks) != 0 {
		t.Fatalf("erwartet 0 Chunks, bekommen %d", len(chunks))
	}
}

func TestSplitFile_MissingFile(t *testing.T) {
	if _, err := SplitFile("/tmp/nicht-existierende-datei-xyz.md", 100); err == nil {
		t.Fatal("erwartete Fehlermeldung bei fehlender Datei")
	}
}

func TestSplitFile_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	src := "## Abschnitt\n\nProsa 1.\n\n```go\nfunc f() {}\n```\n\n- a\n- b\n\n| H1 | H2 |\n|---|---|\n| 1 | 2 |\n\nFazit.\n"
	p := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(p, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	doc, err := SplitFileDoc(p, 80)
	if err != nil {
		t.Fatalf("SplitFileDoc: %v", err)
	}
	assertExactRoundtrip(t, src, doc)
}
