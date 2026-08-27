package split

import (
	"errors"
	"strings"
	"testing"
)

const manual = `# Handbook

Intro prose.

## Usage

How to use it.

### CLI

Run it like this:

` + "```bash\ntool -v\n```" + `

### Two modes

Fast and slow.

## Translation

About translating.

### Two modes

Block and chunk.

## License

MIT.
`

func TestOutline_LevelsAndPaths(t *testing.T) {
	got := Outline(manual)
	want := []struct {
		level int
		path  string
	}{
		{1, "Handbook"},
		{2, "Handbook > Usage"},
		{3, "Handbook > Usage > CLI"},
		{3, "Handbook > Usage > Two modes"},
		{2, "Handbook > Translation"},
		{3, "Handbook > Translation > Two modes"},
		{2, "Handbook > License"},
	}
	if len(got) != len(want) {
		t.Fatalf("erwartet %d Überschriften, bekommen %d: %+v", len(want), len(got), got)
	}
	for i, w := range want {
		if got[i].Level != w.level || got[i].Path != w.path {
			t.Errorf("%d: %d/%q, erwartet %d/%q", i, got[i].Level, got[i].Path, w.level, w.path)
		}
	}
}

// TestOutline_CarriesNoText ist der Kern: die Gliederung erlaubt zu wählen,
// ohne zu lesen. Käme Inhalt mit, wäre der Zweck verfehlt.
func TestOutline_CarriesNoText(t *testing.T) {
	for _, h := range Outline(manual) {
		for _, forbidden := range []string{"tool -v", "Fast and slow", "Intro prose", "```"} {
			if strings.Contains(h.Title, forbidden) || strings.Contains(h.Path, forbidden) {
				t.Errorf("Gliederung trägt Inhalt: %+v", h)
			}
		}
	}
}

// TestOutline_Chars misst den ganzen Abschnitt samt Unterabschnitten - genau
// die Zahl, mit der ein Aufrufer entscheidet, ob Lesen bezahlbar ist.
func TestOutline_Chars(t *testing.T) {
	byPath := map[string]Topic{}
	for _, h := range Outline(manual) {
		byPath[h.Path] = h
	}
	usage := byPath["Handbook > Usage"]
	cli := byPath["Handbook > Usage > CLI"]
	if usage.Chars <= cli.Chars {
		t.Errorf("Abschnitt (%d) muss größer sein als sein Unterabschnitt (%d)", usage.Chars, cli.Chars)
	}
	if byPath["Handbook"].Chars < usage.Chars {
		t.Error("die oberste Ebene muss alles darunter enthalten")
	}
}

func TestOutline_Lines(t *testing.T) {
	for _, h := range Outline(manual) {
		lines := strings.Split(manual, "\n")
		if h.Line < 1 || h.Line > len(lines) {
			t.Fatalf("Zeile %d außerhalb des Dokuments: %+v", h.Line, h)
		}
		if !strings.Contains(lines[h.Line-1], h.Title) {
			t.Errorf("Zeile %d ist %q, erwartet die Überschrift %q", h.Line, lines[h.Line-1], h.Title)
		}
	}
}

// TestSection_StopsAtSameLevel: ein Abschnitt endet an der nächsten gleich-
// oder höherrangigen Überschrift, nicht an der nächsten überhaupt.
func TestSection_StopsAtSameLevel(t *testing.T) {
	got, err := Section(manual, "Handbook > Usage")
	if err != nil {
		t.Fatalf("Section: %v", err)
	}
	for _, want := range []string{"## Usage", "How to use it.", "### CLI", "tool -v", "### Two modes", "Fast and slow."} {
		if !strings.Contains(got, want) {
			t.Errorf("Abschnitt ohne %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{"## Translation", "About translating", "## License"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("Abschnitt reicht zu weit, enthält %q:\n%s", forbidden, got)
		}
	}
}

// TestSection_KeepsTheFenceWhole: der Grund, warum das hier und nicht per grep
// passiert - ein Abschnitt kommt mit seinen Code-Zäunen vollständig.
func TestSection_KeepsTheFenceWhole(t *testing.T) {
	got, err := Section(manual, "Handbook > Usage > CLI")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(got, "```") != 2 {
		t.Errorf("Zaun nicht vollständig:\n%s", got)
	}
	if !strings.Contains(got, "tool -v") {
		t.Errorf("Zaun-Inhalt fehlt:\n%s", got)
	}
}

// TestSection_AmbiguousTitle: "Two modes" gibt es zweimal. Raten wäre die
// falsche Antwort - der Aufrufer bekommt die Kandidaten.
func TestSection_AmbiguousTitle(t *testing.T) {
	_, err := Section(manual, "Two modes")
	if err == nil {
		t.Fatal("mehrdeutiger Titel wurde nicht erkannt")
	}
	var e *ErrNoSuchSection
	if !errors.As(err, &e) || len(e.Candidates) != 2 {
		t.Fatalf("erwartet zwei Kandidaten, bekommen %v", err)
	}
	// Mit vollem Pfad ist es eindeutig.
	got, err := Section(manual, "Handbook > Translation > Two modes")
	if err != nil {
		t.Fatalf("voller Pfad scheiterte: %v", err)
	}
	if !strings.Contains(got, "Block and chunk.") || strings.Contains(got, "Fast and slow.") {
		t.Errorf("falscher Abschnitt aufgelöst:\n%s", got)
	}
}

func TestSection_BareTitleWhenUnique(t *testing.T) {
	got, err := Section(manual, "license")
	if err != nil {
		t.Fatalf("eindeutiger Titel scheiterte: %v", err)
	}
	if !strings.Contains(got, "MIT.") {
		t.Errorf("got %q", got)
	}
}

func TestSection_Missing(t *testing.T) {
	_, err := Section(manual, "Nonexistent")
	if err == nil {
		t.Fatal("fehlende Überschrift wurde nicht gemeldet")
	}
}

// TestSection_LastSectionReachesTheEnd deckt die Kante ab, an der es keine
// nächste Überschrift mehr gibt.
func TestSection_LastSectionReachesTheEnd(t *testing.T) {
	got, err := Section(manual, "Handbook > License")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "## License") || !strings.Contains(got, "MIT.") {
		t.Errorf("letzter Abschnitt unvollständig:\n%s", got)
	}
}
