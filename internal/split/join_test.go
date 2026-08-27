package split

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// assertExactRoundtrip ist der Kernvertrag: Split + JoinGaps ergibt exakt
// Canonical(Original) - Byte für Byte, inklusive Einrückung.
func assertExactRoundtrip(t *testing.T, src string, doc Doc) {
	t.Helper()
	if len(doc.Chunks) == 0 {
		t.Fatal("keine Chunks")
	}
	if len(doc.Gaps) != len(doc.Chunks)-1 {
		t.Fatalf("Gaps = %d, erwartet %d", len(doc.Gaps), len(doc.Chunks)-1)
	}
	want := Canonical(src)
	got := JoinGaps(doc.Chunks, doc.Gaps)
	if got == want {
		return
	}
	wl, gl := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(wl) || i < len(gl); i++ {
		var w, g string
		if i < len(wl) {
			w = wl[i]
		}
		if i < len(gl) {
			g = gl[i]
		}
		if w != g {
			t.Fatalf("Roundtrip weicht ab bei Zeile %d (%d Zeilen erwartet, %d bekommen):\n  WANT %q\n  GOT  %q",
				i+1, len(wl), len(gl), w, g)
		}
	}
	t.Fatalf("Roundtrip weicht ab: %d vs %d Bytes", len(want), len(got))
}

func TestCanonical(t *testing.T) {
	cases := map[string]string{
		"  \n\n\t  ":               "",
		"\nhallo\n\n\n   \nwelt\n": "hallo\n\n\n\nwelt\n",
		"a  \nb":                   "a  \nb\n", // harter Zeilenumbruch bleibt
		"x":                        "x\n",
		"  eingerückt\n":           "  eingerückt\n",
	}
	for in, want := range cases {
		if got := Canonical(in); got != want {
			t.Errorf("Canonical(%q) = %q, erwartet %q", in, got, want)
		}
	}
}

func TestNormalize_Basics(t *testing.T) {
	cases := map[string]string{
		"  \n\n\t  ":               "",
		"\nhallo\n\n\n   \nwelt\n": "hallo\n\nwelt\n",
		"a   \nb":                  "a\nb\n",
		"x":                        "x\n",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, erwartet %q", in, got, want)
		}
	}
}

func TestJoin_OrderAndBlankLines(t *testing.T) {
	got := Join([]string{"Teil eins.", "", "\nTeil zwei.\n\n"})
	want := "Teil eins.\n\nTeil zwei.\n"
	if got != want {
		t.Errorf("Join = %q, erwartet %q", got, want)
	}
	if Join(nil) != "" {
		t.Error("Join(nil) soll leer sein")
	}
}

func TestJoinGaps_UsesRecordedGaps(t *testing.T) {
	got := JoinGaps([]string{"a", "b", "c"}, []int{2, 1})
	if want := "a\n\n\nb\n\nc\n"; got != want {
		t.Errorf("JoinGaps = %q, erwartet %q", got, want)
	}
}

func TestMergeFiles_Missing(t *testing.T) {
	if _, err := MergeFiles([]string{"/tmp/nicht-da-xyz.md"}); err == nil {
		t.Fatal("Fehler bei fehlender Datei erwartet")
	}
}

// TestRoundtrip_Cases deckt die Konstruktionen ab, an denen der Splitter
// vorher Zeichen verloren hat.
func TestRoundtrip_Cases(t *testing.T) {
	cases := map[string]string{
		"Zaun mit Info-String":   "Text A\n\n```js title=\"a.js\"\nconst x = 1;\n\nconst y = 2;\n```\n\nText B\n",
		"Zaun mit Leerzeichen":   "Text A\n\n``` go\nfunc a(){}\n```\n\nText B\n",
		"Tilde-Zaun":             "Text A\n\n~~~\ncode ``` hier\n~~~\n\nText B\n",
		"Zaun im Listenpunkt":    "- Punkt:\n\n  ```go\n  func a(){}\n  ```\n\n- Zweiter\n",
		"Heading ohne Leerzeile": "# Titel\nSofort Text ohne Leerzeile\n",
		"YAML-Frontmatter":       "---\ntitle: Hallo\ntags: [a,b]\n---\n\n# Doc\n\nText\n",
		"offener div":            "<div>\n\nSehr viel Text\n\n# Heading\n\nnoch mehr\n",
		"lose Liste":             "1. Eins\n\n   Fortsetzung von eins.\n\n2. Zwei\n",
		"Tabelle direkt an Text": "Vorher\n| a | b |\n|---|---|\n| 1 | 2 |\nNachher\n",
		"eingerücktes Zitat":     "- Punkt\n\n  > **User:** Zitat im Listenpunkt.\n\n- Zwei\n",
		"harter Umbruch":         "Zeile eins  \nZeile zwei\n",
		"Leerzeilenlauf":         "A\n\n\n\nB\n",
	}
	for name, src := range cases {
		for _, size := range []int{40, 200, 8000} {
			t.Run(name, func(t *testing.T) {
				assertExactRoundtrip(t, src, SplitDoc(src, size))
			})
		}
	}
}

func TestRoundtrip_testMd(t *testing.T) {
	data, err := os.ReadFile("../../testdata/sample.md")
	if err != nil {
		t.Skip("testdata/sample.md nicht vorhanden; laufe nur in Projektroot")
	}
	for _, size := range []int{500, 2000, 4000} {
		assertExactRoundtrip(t, string(data), SplitDoc(string(data), size))
	}
}

// TestRoundtrip_ProjectDocs fährt den Splitter über die Markdown-Dateien des
// Projekts - der billigste echte Korpus, den wir hermetisch haben.
func TestRoundtrip_ProjectDocs(t *testing.T) {
	matches, _ := filepath.Glob("../../*.md")
	fixtures, _ := filepath.Glob("../../testdata/*.md")
	matches = append(matches, fixtures...)
	if len(matches) == 0 {
		t.Skip("keine Projekt-Markdown gefunden")
	}
	for _, p := range matches {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, size := range []int{500, 2000, 8000} {
			t.Run(filepath.Base(p), func(t *testing.T) {
				assertExactRoundtrip(t, string(data), SplitDoc(string(data), size))
			})
		}
	}
}

// TestRoundtrip_MergeFiles deckt den Dateipfad ab.
func TestRoundtrip_MergeFiles(t *testing.T) {
	dir := t.TempDir()
	src := "# Titel\n\nProsa.\n\n```go\nfunc f() {}\n```\n\n- eins\n- zwei\n\nFazit.\n"
	doc := SplitDoc(src, 60)

	paths := make([]string, 0, len(doc.Chunks))
	for i, content := range doc.Chunks {
		p := filepath.Join(dir, "doc-part-"+string(rune('1'+i))+".md")
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	got, err := MergeFilesGaps(paths, doc.Gaps)
	if err != nil {
		t.Fatalf("MergeFilesGaps: %v", err)
	}
	if want := Canonical(src); got != want {
		t.Fatalf("Roundtrip:\nWANT %q\nGOT  %q", want, got)
	}
}
