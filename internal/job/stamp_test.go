package job

import (
	"strings"
	"testing"
)

// countTopLevelKey zählt Zeilen, die genau mit dem Schlüssel beginnen.
func countTopLevelKey(doc, key string) int {
	n := 0
	for _, l := range strings.Split(doc, "\n") {
		if strings.HasPrefix(l, key) {
			n++
		}
	}
	return n
}

func sampleProv() Provenance {
	return Provenance{
		Tool: "mdsplit", Version: "1.3.0", URL: "https://github.com/mlechner911/mdsplit",
		Source: "doc.md", SourceSHA: "3f2a9c1e4b7d0011", SourceChars: 10236,
		TargetLang: "de", Model: "translategemma-4b-it", Mode: "block",
		Translated: "2026-08-27T14:23:11Z", Machine: true,
	}
}

func TestProvenanceYAML(t *testing.T) {
	got := sampleProv().YAML("11/11")
	for _, want := range []string{
		"translation:\n", "  tool: mdsplit\n", "  version: 1.3.0\n",
		"  url: \"https://github.com/mlechner911/mdsplit\"\n",
		"  source_sha256: 3f2a9c1e4b7d0011\n", "  source_chars: 10236\n",
		"  model: translategemma-4b-it\n", "  parts: 11/11\n",
		"  machine_translation: true\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("YAML ohne %q:\n%s", want, got)
		}
	}
	// Die URL enthält ":" und muss deshalb gequotet sein, sonst ist es kein YAML.
	if strings.Contains(got, "url: https://") {
		t.Errorf("URL ist ungequotet und damit ungültiges YAML:\n%s", got)
	}
}

// TestStamp_NoFrontMatter: ohne vorhandenen Block wird einer angelegt.
func TestStamp_NoFrontMatter(t *testing.T) {
	doc := "# Titel\n\nText.\n"
	got := Stamp(doc, sampleProv().YAML("2/2"))
	if !strings.HasPrefix(got, "---\ntranslation:\n") {
		t.Fatalf("kein Front Matter angelegt:\n%s", got)
	}
	if !strings.Contains(got, "---\n\n# Titel") {
		t.Errorf("Dokument nicht sauber angehängt:\n%s", got)
	}
	if Unstamp(got) != doc {
		t.Errorf("Unstamp stellt das Original nicht her:\n%q", Unstamp(got))
	}
}

// TestStamp_MergesIntoExisting ist der Fall, an dem naives Voranstellen das
// Dokument zerstört: der vorhandene Block hörte auf, Metadaten zu sein.
func TestStamp_MergesIntoExisting(t *testing.T) {
	doc := "---\ntitle: Handbuch\ntags:\n  - a\n  - b\n---\n\n# Titel\n\nText.\n"
	got := Stamp(doc, sampleProv().YAML("2/2"))

	if strings.Count(got, "---\n") != 2 {
		t.Fatalf("erwartet genau einen Front-Matter-Block, bekommen:\n%s", got)
	}
	for _, want := range []string{"title: Handbuch", "tags:", "  - a", "  - b", "translation:", "  tool: mdsplit"} {
		if !strings.Contains(got, want) {
			t.Errorf("fehlt %q:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "# Titel") {
		t.Errorf("Dokumentinhalt verloren:\n%s", got)
	}
	if Unstamp(got) != doc {
		t.Errorf("Unstamp:\nWANT %q\nGOT  %q", doc, Unstamp(got))
	}
}

// TestStamp_ReplacesOwnKey: zweimal stempeln darf nicht zwei Blöcke ergeben.
func TestStamp_ReplacesOwnKey(t *testing.T) {
	doc := "---\ntitle: X\n---\n\nText.\n"
	once := Stamp(doc, sampleProv().YAML("2/2"))
	p := sampleProv()
	p.Model = "qwen2.5-7b-instruct"
	twice := Stamp(once, p.YAML("2/2"))

	// Zeilenweise zählen: "machine_translation:" enthält den Teilstring.
	if n := countTopLevelKey(twice, "translation:"); n != 1 {
		t.Fatalf("erwartet 1 translation-Block, bekommen %d:\n%s", n, twice)
	}
	if strings.Contains(twice, "translategemma") {
		t.Errorf("alter Modellwert überlebt:\n%s", twice)
	}
	if !strings.Contains(twice, "title: X") {
		t.Errorf("fremder Schlüssel verloren:\n%s", twice)
	}
}

// TestSplitFrontMatter_ThematicBreak: ein unabgeschlossenes "---" ist eine
// Trennlinie, kein Front Matter - da darf nichts hineingeschrieben werden.
func TestSplitFrontMatter_ThematicBreak(t *testing.T) {
	doc := "---\n\nDas war eine Trennlinie.\n"
	if _, _, ok := splitFrontMatter(doc); ok {
		t.Error("Trennlinie wurde als Front Matter gelesen")
	}
	got := Stamp(doc, sampleProv().YAML("1/1"))
	if !strings.Contains(got, "Das war eine Trennlinie.") {
		t.Errorf("Inhalt verloren:\n%s", got)
	}
}

func TestUnstamp_LeavesForeignDocsAlone(t *testing.T) {
	for _, doc := range []string{
		"# Nur Text\n",
		"---\ntitle: X\n---\n\nText.\n",
		"---\n\nTrennlinie.\n",
	} {
		if got := Unstamp(doc); got != doc {
			t.Errorf("Unstamp hat ein fremdes Dokument verändert:\nWANT %q\nGOT  %q", doc, got)
		}
	}
}
