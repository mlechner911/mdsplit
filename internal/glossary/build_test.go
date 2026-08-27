package glossary

import "testing"

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
