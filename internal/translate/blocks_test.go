package translate

import (
	"strings"
	"testing"
)

const chunk = "## Install the tool\n\n" +
	"Run `go install` and the binary lands in `$GOBIN`, see [the docs](https://example.com/a).\n\n" +
	"```bash\ngo install github.com/example/tool@latest\n```\n\n" +
	"| Flag | Meaning |\n|---|---|\n| -v | verbose output |\n\n" +
	"- first item with `code`\n- second item\n\n" +
	"<div class=\"note\">\n  Some boxed text\n</div>\n"

// sent liefert alles, was tatsächlich ans Modell ginge.
func sent(pieces []piece) []string {
	var out []string
	for _, p := range pieces {
		if p.translate {
			out = append(out, p.text)
		}
	}
	return out
}

// literal liefert das, was byte-genau reproduziert wird.
func literal(pieces []piece) string {
	var b strings.Builder
	for _, p := range pieces {
		if !p.translate {
			b.WriteString(p.text)
		}
	}
	return b.String()
}

// TestPlan_Reassembles: ohne Übersetzung muss der Plan den Chunk exakt ergeben.
func TestPlan_Reassembles(t *testing.T) {
	var b strings.Builder
	for _, p := range planChunk(chunk) {
		b.WriteString(p.text)
	}
	if got := b.String(); got != strings.TrimRight(chunk, "\n") {
		t.Fatalf("Plan reproduziert den Chunk nicht:\nWANT %q\nGOT  %q", chunk, got)
	}
}

// TestPlan_CodeIsNeverSent ist der Kern von block mode: Code verlässt den
// Rechner nicht, also kann ihn kein Modell beschädigen.
func TestPlan_CodeIsNeverSent(t *testing.T) {
	for _, s := range sent(planChunk(chunk)) {
		for _, forbidden := range []string{"go install github.com", "```", "<div", "</div>", "Some boxed text"} {
			if strings.Contains(s, forbidden) {
				t.Errorf("Code oder HTML würde gesendet: %q enthält %q", s, forbidden)
			}
		}
	}
	lit := literal(planChunk(chunk))
	for _, required := range []string{"```bash", "go install github.com/example/tool@latest", "<div class=\"note\">", "</div>"} {
		if !strings.Contains(lit, required) {
			t.Errorf("literal fehlt %q", required)
		}
	}
}

// TestPlan_StructureIsLiteral: Marker, Pipes und Trennzeilen sind literal, eine
// Tabelle kann also keine Spalte verlieren.
func TestPlan_StructureIsLiteral(t *testing.T) {
	lit := literal(planChunk(chunk))
	for _, required := range []string{"## ", "|", "|---|---|", "- "} {
		if !strings.Contains(lit, required) {
			t.Errorf("Strukturmarker %q ist nicht literal", required)
		}
	}
	for _, s := range sent(planChunk(chunk)) {
		if strings.HasPrefix(s, "#") || strings.HasPrefix(s, "- ") || strings.Contains(s, "|") {
			t.Errorf("Marker landet im gesendeten Fragment: %q", s)
		}
	}
}

// TestPlan_SendsTheProse: die Prosa muss ankommen, sonst übersetzt nichts.
func TestPlan_SendsTheProse(t *testing.T) {
	joined := strings.Join(sent(planChunk(chunk)), "\n")
	for _, required := range []string{"Install the tool", "verbose output", "first item", "second item"} {
		if !strings.Contains(joined, required) {
			t.Errorf("Prosa %q wird nicht gesendet; gesendet wurde: %q", required, joined)
		}
	}
}

func TestTranslatable(t *testing.T) {
	cases := map[string]bool{
		"verbose output":               true,
		"-v":                           false,
		"`$GOBIN`":                     false,
		"1.2.0":                        false,
		"":                             false,
		"| ":                           false,
		"see `go install` for details": true,
		"https://example.com":          false,
	}
	for in, want := range cases {
		if got := translatable(in); got != want {
			t.Errorf("translatable(%q) = %v, erwartet %v", in, got, want)
		}
	}
}

// TestProtectRestore: Inline-Code, URLs und Link-Ziele werden maskiert, der
// Satz drumherum geht aber am Stück raus - fragmentweise Übersetzung würde an
// der deutschen Wortstellung scheitern.
func TestProtectRestore(t *testing.T) {
	in := "Run `go install` and see [the docs](https://example.com/a) or https://x.dev now."
	masked, tokens := protect(in)
	if len(tokens) != 3 {
		t.Fatalf("erwartet 3 geschützte Spans, bekommen %d: %v", len(tokens), tokens)
	}
	for _, bad := range []string{"go install", "example.com", "x.dev"} {
		if strings.Contains(masked, bad) {
			t.Errorf("geschützter Inhalt steht noch im Prompt: %q", masked)
		}
	}
	if !strings.Contains(masked, "Run ") || !strings.Contains(masked, " now.") {
		t.Errorf("Satzkontext ging verloren: %q", masked)
	}
	back, err := restore(masked, tokens)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if back != in {
		t.Errorf("Roundtrip:\nWANT %q\nGOT  %q", in, back)
	}
}

func TestRestore_RejectsLostPlaceholder(t *testing.T) {
	_, tokens := protect("Run `go install` now.")
	if _, err := restore("Führe jetzt aus.", tokens); err == nil {
		t.Fatal("verlorener Platzhalter wurde nicht erkannt")
	}
	if _, err := restore("⟦0⟧ und nochmal ⟦0⟧", tokens); err == nil {
		t.Fatal("doppelter Platzhalter wurde nicht erkannt")
	}
}

// TestPlanList_FenceInItem: ein Zaun im Listenpunkt bleibt literal.
func TestPlanList_FenceInItem(t *testing.T) {
	src := "- Step one:\n\n  ```go\n  func main() {}\n  ```\n\n- Step two"
	for _, s := range sent(planChunk(src)) {
		if strings.Contains(s, "func main") || strings.Contains(s, "```") {
			t.Errorf("Zaun im Listenpunkt würde gesendet: %q", s)
		}
	}
}

// TestProtect_ExtendedSpans deckt die Ergänzungen ab: Bilder, Referenz-Links,
// Fußnoten und Inline-HTML.
func TestProtect_ExtendedSpans(t *testing.T) {
	cases := map[string][]string{
		"See ![Screenshot of the app](img/a.png) here.": {"](img/a.png)"},
		"As shown in [the guide][guide] later.":         {"][guide]"},
		"A claim with a source[^1] attached.":           {"[^1]"},
		"Logo <img src=\"a.png\" alt=\"x\"> inline.":    {"<img src=\"a.png\" alt=\"x\">"},
		"Use <br> to break.":                            {"<br>"},
	}
	for in, want := range cases {
		t.Run(in[:12], func(t *testing.T) {
			masked, tokens := protect(in)
			if len(tokens) != len(want) {
				t.Fatalf("erwartet %d geschützte Spans, bekommen %d: %v", len(want), len(tokens), tokens)
			}
			for i, w := range want {
				if tokens[i] != w {
					t.Errorf("Span %d = %q, erwartet %q", i, tokens[i], w)
				}
			}
			back, err := restore(masked, tokens)
			if err != nil || back != in {
				t.Errorf("Roundtrip: %q / %v", back, err)
			}
		})
	}
}

// TestProtect_AltTextStaysTranslatable: der Alt-Text ist sichtbare Prosa und
// muss übersetzt werden, der Pfad darf es nicht.
func TestProtect_AltTextStaysTranslatable(t *testing.T) {
	masked, _ := protect("See ![Screenshot of the app](img/a.png) here.")
	if !strings.Contains(masked, "Screenshot of the app") {
		t.Errorf("Alt-Text wurde mitgeschützt: %q", masked)
	}
	if strings.Contains(masked, "img/a.png") {
		t.Errorf("Bildpfad ist ungeschützt: %q", masked)
	}
}

// TestFitFragment schließt das Loch, durch das ein Modell im Block-Modus doch
// Struktur einschleusen konnte: eine Antwort mit Zeilenumbruch oder mit einem
// führenden Marker verschiebt beim Neuparsen alle folgenden Blöcke.
func TestFitFragment(t *testing.T) {
	t.Run("Umbruch in Einzeiler wird geglättet", func(t *testing.T) {
		got, err := fitFragment("verbose output", "ausführliche\nAusgabe")
		if err != nil {
			t.Fatalf("unerwarteter Fehler: %v", err)
		}
		if got != "ausführliche Ausgabe" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("Umbruch bleibt, wenn die Quelle mehrzeilig war", func(t *testing.T) {
		got, err := fitFragment("a\nb", "eins\nzwei")
		if err != nil || got != "eins\nzwei" {
			t.Errorf("got %q / %v", got, err)
		}
	})
	t.Run("neuer Listenmarker wird abgelehnt", func(t *testing.T) {
		if _, err := fitFragment("some prose", "- ein Punkt"); err == nil {
			t.Error("führender Listenmarker wurde nicht erkannt")
		}
	})
	t.Run("neue Überschrift wird abgelehnt", func(t *testing.T) {
		if _, err := fitFragment("some prose", "## Titel"); err == nil {
			t.Error("führende Überschrift wurde nicht erkannt")
		}
	})
	t.Run("neuer Zaun wird abgelehnt", func(t *testing.T) {
		if _, err := fitFragment("some prose", "```go"); err == nil {
			t.Error("führender Zaun wurde nicht erkannt")
		}
	})
	t.Run("Quelle war schon strukturell", func(t *testing.T) {
		if _, err := fitFragment("| a |", "| b |"); err != nil {
			t.Errorf("unerwarteter Fehler: %v", err)
		}
	})
}

// TestTranslatable_IdentifiersAreNotProse deckt den Fall ab, an dem
// TranslateGemma das Flag "-out" zu "– aus" gemacht hat: nach Buchstabenzahl
// sieht ein Flag wie ein Wort aus, ist aber ein Bezeichner.
func TestTranslatable_IdentifiersAreNotProse(t *testing.T) {
	notProse := []string{
		"-out", "-v", "--verbose", "--dry-run", "-size",
		"./cmd/tool", "internal/split", "config.yaml", "os.ReadFile",
		"~/go/bin", "cmd/mcp-md-splitter/",
	}
	for _, s := range notProse {
		if translatable(s) {
			t.Errorf("translatable(%q) = true, erwartet false", s)
		}
	}
	prose := []string{
		"verbose output", "output path", "the tool never writes outside",
		"Run go install for details", "Flag",
	}
	for _, s := range prose {
		if !translatable(s) {
			t.Errorf("translatable(%q) = false, erwartet true", s)
		}
	}
}

// TestRestore_TolerantOfSpacing: ein Modell, das ⟦ 0 ⟧ schreibt, hat den
// Platzhalter nicht verloren - das soll die Antwort nicht kosten.
func TestRestore_TolerantOfSpacing(t *testing.T) {
	in := "Run `go install` now."
	masked, tokens := protect(in)
	spaced := strings.ReplaceAll(masked, "⟦0⟧", "⟦ 0 ⟧")
	back, err := restore(spaced, tokens)
	if err != nil {
		t.Fatalf("Leerzeichen im Platzhalter wurden nicht toleriert: %v", err)
	}
	if !strings.Contains(back, "`go install`") {
		t.Errorf("Inhalt nicht wiederhergestellt: %q", back)
	}
}

// TestRestore_ErrorNamesTheDamage: die Meldung muss sagen, wie viele
// Platzhalter überlebt haben - sonst ist der Fehlschlag nicht diagnostizierbar.
func TestRestore_ErrorNamesTheDamage(t *testing.T) {
	_, tokens := protect("Run `a` and `b` and `c`.")
	_, err := restore("Nur ⟦0⟧ blieb übrig.", tokens)
	if err == nil {
		t.Fatal("Fehler erwartet")
	}
	if !strings.Contains(err.Error(), "of 3 sentinels") {
		t.Errorf("Meldung nennt die Bilanz nicht: %v", err)
	}
}
