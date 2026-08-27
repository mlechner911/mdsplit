package glossary

import (
	"strings"
	"testing"
)

// TestIsTerm hält fest, was ein Glossareintrag sein darf. Der Wert wird als
// Regel "term = value" in jeden Übersetzungsprompt injiziert, der den Begriff
// enthält - ein ganzer Satz an dieser Stelle steuert die Übersetzung, statt
// sie zu präzisieren.
func TestIsTerm(t *testing.T) {
	ok := map[string]string{
		"code fences":      "delimitadores de código",
		"manifest":         "manifiesto",
		"blank line":       "línea en blanco",
		"round-trip check": "comprobación de ida y vuelta",
	}
	for k, v := range ok {
		if !isTerm(k, v) {
			t.Errorf("isTerm(%q, %q) = false, erwartet true", k, v)
		}
	}
	// Alle vier stammen aus einem echten Lauf gegen qwen2.5-7b.
	bad := map[string]string{
		"chunk starts":   "un bloque que comienza en mitad de una fence es dañino.",
		"whether":        "siempre mantiene el mismo tamaño, independientemente del tamaño original.",
		"byte-identical": "byte-identico / solo espacios en blanco / divergente",
		"atomic blocks":  "bloques atómicos (fences, tablas, listas, HTML multi-line)",
	}
	for k, v := range bad {
		if isTerm(k, v) {
			t.Errorf("isTerm(%q, %q) = true, erwartet false", k, v)
		}
	}
}

func TestClean_DropsUnaskedAndUnchanged(t *testing.T) {
	cands := []Candidate{{Term: "manifest"}, {Term: "code fences"}}
	got := clean(map[string]string{
		"manifest":    "manifiesto",
		"code fences": "code fences", // unverändert: keine Entscheidung
		"invented":    "inventado",   // nicht gefragt
	}, cands)
	if len(got) != 1 || got["manifest"] != "manifiesto" {
		t.Errorf("clean = %v", got)
	}
}

// TestCandidates_RejectsIdentifiers: put_chunk ist ein Werkzeugname. Ein 7B hat
// daraus "Chunk hinzufügen" gemacht - ein Glossareintrag, der den Aufruf
// unbrauchbar machen würde.
func TestCandidates_RejectsIdentifiers(t *testing.T) {
	doc := "# Doc\n\nCall put chunk to store a part. The put chunk step is safe.\n" +
		"Later, put chunk again.\n\n```go\nput_chunk(jobId, part)\nput_chunk(jobId, 2)\n```\n\n" +
		"A code fence is atomic. Every code fence stays whole. The code fence rule holds.\n"
	for _, c := range Candidates(doc, 40) {
		if c.Term == "put chunk" {
			t.Errorf("Bezeichner put_chunk als Begriff vorgeschlagen: %+v", c)
		}
	}
}

// TestCandidates_MergesHyphenVariants: "blank line" und "blank-line" sind
// derselbe Begriff und dürfen nicht zweimal zur Entscheidung stehen.
func TestCandidates_MergesHyphenVariants(t *testing.T) {
	doc := "# Doc\n\nA blank line ends a block. The blank-line gap is recorded.\n\n" +
		"Every blank line matters. The blank-line rule is simple.\n\nOne more blank line here.\n"
	seen := 0
	for _, c := range Candidates(doc, 40) {
		if c.Term == "blank line" || c.Term == "blank-line" {
			seen++
		}
	}
	if seen > 1 {
		t.Errorf("Hyphen-Variante nicht zusammengeführt: %d Einträge", seen)
	}
}

// TestCandidates_NoPairsAcrossPunctuation: "code fences, tables and lists"
// darf nicht den Scheinbegriff "fences tables" erzeugen.
func TestCandidates_NoPairsAcrossPunctuation(t *testing.T) {
	doc := "# Doc\n\nCode fences, tables and list items stay whole.\n\n" +
		"Again: code fences, tables and lists are atomic.\n\n" +
		"And once more, code fences, tables and lists.\n"
	for _, c := range Candidates(doc, 40) {
		if c.Term == "fences tables" {
			t.Errorf("Paar über Satzzeichen hinweg gebildet: %+v", c)
		}
	}
}

// TestSupport benennt, was die Kandidatenauswahl je Quellsprache leisten kann.
// Sie ist englisch: englische Stoppwortliste, Trennung an Leerzeichen. Das
// still hinzunehmen hieße, dem Nutzer eine Liste aus Artikeln und
// Präpositionen als Terminologie zu verkaufen.
func TestSupport(t *testing.T) {
	cases := map[string]SourceSupport{
		"":        SupportGood,
		"en":      SupportGood,
		"en-GB":   SupportGood,
		"English": SupportGood,
		"de":      SupportNoisy,
		"es":      SupportNoisy,
		"fr":      SupportNoisy,
		"zh":      SupportNone,
		"zh-CN":   SupportNone,
		"ja":      SupportNone,
		"th":      SupportNone,
	}
	for code, want := range cases {
		got, note := Support(code)
		if got != want {
			t.Errorf("Support(%q) = %v, erwartet %v", code, got, want)
		}
		if want != SupportGood && note == "" {
			t.Errorf("Support(%q) meldet ein Problem ohne Begründung", code)
		}
		if want == SupportGood && note != "" {
			t.Errorf("Support(%q) warnt ohne Anlass: %q", code, note)
		}
	}
}

// TestPrompt_UsesTheSourceLanguage: der Prompt sagte fest "For each English
// term", auch wenn die Quelle deutsch war.
func TestPrompt_UsesTheSourceLanguage(t *testing.T) {
	system, _ := prompt("German", "Spanish", []Candidate{{Term: "Leerzeile"}})
	if !strings.Contains(system, "from German into Spanish") {
		t.Errorf("Sprachpaar fehlt im Prompt:\n%s", system)
	}
	if strings.Contains(system, "English") {
		t.Errorf("Prompt behauptet weiterhin Englisch:\n%s", system)
	}
}
