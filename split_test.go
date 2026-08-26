package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// normalize entfernt führende/nachführende Leerzeilen und drückt Mehrfach-
// Leerzeilen auf eine zusammen, damit Vergleiche robust bleiben.
func normalize(s string) []string {
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(s), "\n") {
		if len(out) > 0 && out[len(out)-1] == "" && strings.TrimSpace(l) == "" {
			continue
		}
		out = append(out, l)
	}
	return out
}

func joinNorm(s string) string {
	return strings.Join(normalize(s), "\n")
}

func TestExtractBlocks_CodeFence(t *testing.T) {
	src := "Intro text.\n\n```bash\necho a\n```\n\nOutro."
	blocks := extractBlocks(src)
	found := false
	for _, b := range blocks {
		if strings.Contains(b, "```bash") && strings.HasSuffix(b, "```") {
			found = true
			if !strings.Contains(b, "echo a") {
				t.Errorf("Code-Zaun fehlt Inhalt: %q", b)
			}
		}
	}
	if !found {
		t.Fatalf("kein atomarer Code-Block in %+v", blocks)
	}
}

func TestExtractBlocks_MultiLineFenceStaysTogether(t *testing.T) {
	var b strings.Builder
	b.WriteString("Prose before.\n\n```\n")
	for i := 0; i < 50; i++ {
		b.WriteString(strings.Repeat("x", 80) + "\n")
	}
	b.WriteString("```\n\nProse after.")
	blocks := extractBlocks(b.String())

	fences := 0
	for _, blk := range blocks {
		if strings.Contains(blk, "```") && strings.Count(blk, "```") >= 2 {
			fences++
			if !strings.Contains(blk, strings.Repeat("x", 80)) {
				t.Fatalf("Code-Block ist aufgeteilt: %q", blk)
			}
		}
	}
	if fences != 1 {
		t.Fatalf("erwartet genau 1 Zaun-Block, gefunden %d in %+v", fences, blocks)
	}
}

func TestExtractBlocks_Table(t *testing.T) {
	src := "Intro\n\n| A | B |\n|---|---|\n| 1 | 2 |\n\nAfter"
	blocks := extractBlocks(src)
	found := false
	for _, b := range blocks {
		if strings.HasPrefix(b, "| A | B |") && strings.Contains(b, "| 1 | 2 |") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Tabelle ist nicht als Block zusammen: %+v", blocks)
	}
}

func TestExtractBlocks_List(t *testing.T) {
	src := "Intro\n\n- item one\n  continuation line\n- item two\n\nAfter"
	blocks := extractBlocks(src)
	found := false
	for _, b := range blocks {
		if strings.Contains(b, "- item one") &&
			strings.Contains(b, "continuation line") &&
			strings.Contains(b, "- item two") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Liste ist nicht als Block zusammen: %+v", blocks)
	}
}

func TestExtractBlocks_HeadingOwnBlock(t *testing.T) {
	src := "Some prose.\n\n# Titel\n\nMore prose."
	blocks := extractBlocks(src)
	found := false
	for _, b := range blocks {
		if strings.TrimSpace(b) == "# Titel" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Überschrift ist kein eigener Block: %+v", blocks)
	}
}

// allBlocksInChunks prüft, dass jeder Eingangsblock komplett in genau einem
// Output-Chunk vorkommt.
func assertNoSplitBlocks(t *testing.T, chunks, blocks []string) {
	t.Helper()
	for _, blk := range blocks {
		hits := 0
		for _, c := range chunks {
			if strings.Contains(c, blk) {
				hits++
			}
		}
		if hits != 1 {
			t.Fatalf("Block in %d Chunks statt genau 1: %q", hits, blk)
		}
	}
}

func TestPackChunks_SizeLimit(t *testing.T) {
	var blocks []string
	for i := 0; i < 20; i++ {
		blocks = append(blocks, fmt.Sprintf("# B%02d ", i)+strings.Repeat("x", 100)) // ~110 Zeichen je Block
	}
	const maxSize = 300
	chunks := packChunks(blocks, maxSize)

	if len(chunks) < 2 {
		t.Fatalf("erwartet > 1 Chunk, bekommen %d", len(chunks))
	}
	for i, c := range chunks {
		if len(c) > maxSize*2 {
			t.Errorf("Chunk %d: %d Zeichen weit über Ziel %d", i+1, len(c), maxSize)
		}
	}
	assertNoSplitBlocks(t, chunks, blocks)
}

func TestPackChunks_SingleBlockLargerThanMax(t *testing.T) {
	big := "```bash\n" + strings.Repeat("echo sehr langes zeichen\n", 50) + "```"
	blocks := []string{"kleiner einstieg", big, "klarer ausgang"}
	chunks := packChunks(blocks, 100)

	foundBig := false
	for i, c := range chunks {
		if len(c) > 100*2 {
			if !strings.Contains(c, "echo sehr langes") {
				t.Errorf("Chunk %d zu groß ohne großen Block: %d Zeichen", i+1, len(c))
			}
			foundBig = true
		}
	}
	if !foundBig {
		t.Fatal("großer Code-Block wurde nirgends gefunden")
	}
	for _, blk := range blocks {
		hits := 0
		for _, c := range chunks {
			if strings.Contains(c, blk) {
				hits++
			}
		}
		if hits != 1 {
			t.Fatalf("Block %d-mal in Chunks: %.40q...", hits, blk)
		}
	}
}

func TestPackChunks_Empty(t *testing.T) {
	chunks := packChunks(nil, 100)
	if len(chunks) != 0 {
		t.Fatalf("erwartet 0 Chunks, bekommen %d", len(chunks))
	}
}

func TestExtractBlocks_Empty(t *testing.T) {
	blocks := extractBlocks("")
	if len(blocks) != 0 {
		t.Fatalf("erwartet 0 Blöcke, bekommen %d: %+v", len(blocks), blocks)
	}
	blocks = extractBlocks("\n\n\n   \n")
	if len(blocks) != 0 {
		t.Fatalf("Leergang soll 0 Blöcke ergeben, bekommen %d", len(blocks))
	}
}

func TestGetMarkdownChunks_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	src := "## Abschnitt\n\nProsa 1.\n\n```go\nfunc f() {}\n```\n\n- a\n- b\n\n| H1 | H2 |\n|---|---|\n| 1 | 2 |\n\nFazit."
	p := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(p, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	chunks, err := getMarkdownChunks(p, 80)
	if err != nil {
		t.Fatalf("getMarkdownChunks: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("keine Chunks zurückgeliefert")
	}
	// Größen-Budget prüfen (außer ein einziger unteilbarer Block überschreitet es)
	for i, c := range chunks {
		if len(c) > 80*2+10 {
			t.Errorf("Chunk %d: %d Zeichen deutlich über Budget", i+1, len(c))
		}
	}

	want := joinNorm(src)
	got := joinNorm(strings.Join(chunks, "\n\n"))
	if got != want {
		t.Errorf("Inhalt wurde nicht verlustfrei reproduziert:\nWANT:\n%s\n\nGOT:\n%s", want, got)
	}
}

func TestGetMarkdownChunks_MissingFile(t *testing.T) {
	_, err := getMarkdownChunks("/tmp/nicht-existierende-datei-xyz.md", 100)
	if err == nil {
		t.Fatal("erwartete Fehlermeldung bei fehlender Datei")
	}
}

// TestFullSplit_testMd nutzt die Fixture aus dem Projekt: Chunks müssen die
// Zielgröße respektieren (bis hin zu doppeltem Budget) und gemeinsam die
// Originalzeilen reproduzieren.
func TestFullSplit_testMd(t *testing.T) {
	data, err := os.ReadFile("test.md")
	if err != nil {
		t.Skip("test.md nicht vorhanden; laufe nur in Projektroot")
	}
	const maxSize = 4000
	blocks := extractBlocks(string(data))
	chunks := packChunks(blocks, maxSize)
	if len(chunks) == 0 {
		t.Fatal("keine Chunks erzeugt")
	}

	// Chunks mit der gleichen Trennung rekonstruieren, die auch innerhalb von
	// Chunks gilt (leere Zeile zwischen Blöcken) -> muss Quelle wiedergeben.
	got := joinNorm(strings.Join(chunks, "\n\n"))
	want := joinNorm(string(data))
	if got != want {
		t.Fatalf("Inhalt nicht verlustfrei reproduziert:\nWANT (%d):\n%s\n\nGOT (%d):\n%s", len(want), want, len(got), got)
	}
	for i, c := range chunks {
		if len(c) > maxSize*2+10 {
			t.Errorf("Chunk %d: %d Zeichen - weit über 2x Ziel", i+1, len(c))
		}
	}
}
