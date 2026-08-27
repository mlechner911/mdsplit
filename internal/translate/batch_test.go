package translate

import (
	"strings"
	"testing"
)

func TestBatchPrompt_NumbersEveryItem(t *testing.T) {
	_, user := batchPrompt(Options{Language: "de"}, []string{"What is sent", "never transmitted", "one"})
	for _, want := range []string{"1. What is sent", "2. never transmitted", "3. one"} {
		if !strings.Contains(user, want) {
			t.Errorf("Anfrage ohne %q:\n%s", want, user)
		}
	}
}

// TestNumberedLineRx deckt die Schreibweisen ab, in denen Modelle eine
// nummerierte Liste zurückgeben.
func TestNumberedLineRx(t *testing.T) {
	cases := map[string][2]string{
		"1. was gesendet wird":    {"1", "was gesendet wird"},
		"2) nie übertragen":       {"2", "nie übertragen"},
		"  3 - eine Anfrage":      {"3", "eine Anfrage"},
		"12: Struktur garantiert": {"12", "Struktur garantiert"},
	}
	for line, want := range cases {
		m := numberedLineRx.FindStringSubmatch(line)
		if m == nil {
			t.Errorf("%q nicht erkannt", line)
			continue
		}
		if m[1] != want[0] || m[2] != want[1] {
			t.Errorf("%q -> %q/%q, erwartet %q/%q", line, m[1], m[2], want[0], want[1])
		}
	}
	// Eine gewöhnliche Prosazeile ist keine nummerierte Antwort.
	if numberedLineRx.MatchString("Blockmodus ist der Standard.") {
		t.Error("Prosazeile fälschlich als nummerierte Antwort gelesen")
	}
}

// TestBatchShort_SkipsSingletons: ein einzelnes Fragment gewinnt nichts durch
// eine Liste und soll den Einzelweg nehmen.
func TestBatchShort_SkipsSingletons(t *testing.T) {
	pieces := []piece{{text: "only one", translate: true}, {text: " | ", translate: false}}
	memo := map[string]string{}
	st := newStats()
	// Ein nil-Client würde beim Aufruf panisch werden - dass der Test
	// durchläuft, belegt den frühen Ausstieg.
	batchShort(nil, nil, pieces, Options{Language: "de"}, memo, &st)
	if len(memo) != 0 || st.requests != 0 {
		t.Errorf("Einzelfragment wurde gebündelt: memo=%d requests=%d", len(memo), st.requests)
	}
}

// TestBatchShort_OnlyShortOnes: lange Prosa bleibt beim Einzelweg, wo sie
// genug eigenen Kontext mitbringt.
func TestBatchShort_OnlyShortOnes(t *testing.T) {
	long := strings.Repeat("ein langer Satz mit viel Kontext ", 4)
	pieces := []piece{
		{text: "never transmitted", translate: true},
		{text: long, translate: true},
	}
	var items []string
	for _, p := range pieces {
		if p.translate && len([]rune(p.text)) <= shortFragment {
			items = append(items, p.text)
		}
	}
	if len(items) != 1 || items[0] != "never transmitted" {
		t.Errorf("Auswahl der kurzen Fragmente falsch: %v", items)
	}
}
