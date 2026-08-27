package split

import (
	"fmt"
	"strings"
)

// Topic is one entry in a document's outline.
//
// Chars is the size of the section the heading opens, subsections included -
// the number a caller needs to decide whether reading it is affordable, and
// the one a heading alone cannot give.
type Topic struct {
	Level int    `json:"level"` // 1..6
	Title string `json:"title"` // the text, without the # markers
	Path  string `json:"path"`  // "Translation > Two modes" - an unambiguous address
	Line  int    `json:"line"`  // 1-based line in the source
	Chars int    `json:"chars"` // size of the section, subsections included
}

// outlineEntry pairs a heading with the block it starts at.
type outlineEntry struct {
	Topic
	block int
}

// PathSeparator joins the ancestors of a heading into an address. Two sections
// can carry the same title - "Two modes" appears under more than one topic in a
// real manual - so the path is what makes a request unambiguous.
const PathSeparator = " > "

// scan builds the outline together with the block index of each heading.
func scan(doc string) ([]Block, []outlineEntry) {
	blocks := ExtractBlocks(doc)

	// Leerzeilen am Dokumentanfang zählen nicht als Block, verschieben aber
	// jede Zeilennummer.
	line := 1
	for _, l := range strings.Split(doc, "\n") {
		if strings.TrimSpace(l) != "" {
			break
		}
		line++
	}

	type anc struct {
		level int
		title string
	}
	var stack []anc
	var out []outlineEntry

	for i, b := range blocks {
		if b.Kind == Heading {
			title := HeadingTitle(b.Text)
			for len(stack) > 0 && stack[len(stack)-1].level >= b.Level {
				stack = stack[:len(stack)-1]
			}
			parts := make([]string, 0, len(stack)+1)
			for _, a := range stack {
				parts = append(parts, a.title)
			}
			parts = append(parts, title)
			stack = append(stack, anc{b.Level, title})

			out = append(out, outlineEntry{
				Topic: Topic{
					Level: b.Level,
					Title: title,
					Path:  strings.Join(parts, PathSeparator),
					Line:  line,
					Chars: rangeSize(blocks, i, sectionEnd(blocks, i)),
				},
				block: i,
			})
		}
		line += strings.Count(b.Text, "\n") + 1 + b.Gap
	}
	return blocks, out
}

// Outline lists a document's headings without returning any of its text. It is
// the reading counterpart to a split manifest: enough to choose what to read,
// small enough that choosing costs nothing.
func Outline(doc string) []Topic {
	_, entries := scan(doc)
	out := make([]Topic, len(entries))
	for i, e := range entries {
		out[i] = e.Topic
	}
	return out
}

// sectionEnd finds where a heading's section stops: at the next heading of the
// same or a shallower level, or at the end of the document.
func sectionEnd(blocks []Block, start int) int {
	lvl := blocks[start].Level
	for j := start + 1; j < len(blocks); j++ {
		if blocks[j].Kind == Heading && blocks[j].Level <= lvl {
			return j
		}
	}
	return len(blocks)
}

// HeadingTitle strips the leading # markers from a heading line.
func HeadingTitle(text string) string {
	t := strings.TrimSpace(text)
	t = strings.TrimLeft(t, "#")
	return strings.TrimSpace(t)
}

// ErrNoSuchSection reports a heading that does not resolve, and offers what
// would have.
type ErrNoSuchSection struct {
	Want       string
	Candidates []string
}

func (e *ErrNoSuchSection) Error() string {
	if len(e.Candidates) == 0 {
		return fmt.Sprintf("no heading matches %q", e.Want)
	}
	return fmt.Sprintf("%q matches %d headings - use a full path: %s",
		e.Want, len(e.Candidates), strings.Join(e.Candidates, ", "))
}

// Section returns one section verbatim: the heading and everything under it,
// down to the next heading of the same or a shallower level.
//
// A section is not a chunk. Chunks follow a byte budget, sections follow the
// outline: "## Usage" may span several chunks, and a short section shares one
// with its neighbour. Retrieval wants the section.
//
// The address may be a full path ("Translation > Two modes") or a bare title
// when that is unambiguous; matching ignores case. An ambiguous title is an
// error naming the candidates, not a guess.
func Section(doc, address string) (string, error) {
	blocks, entries := scan(doc)
	want := strings.ToLower(strings.TrimSpace(address))
	if want == "" {
		return "", &ErrNoSuchSection{Want: address}
	}

	var exact, byTitle []outlineEntry
	for _, e := range entries {
		if strings.ToLower(e.Path) == want {
			exact = append(exact, e)
		}
		if strings.ToLower(e.Title) == want {
			byTitle = append(byTitle, e)
		}
	}
	hits := exact
	if len(hits) == 0 {
		hits = byTitle
	}
	switch len(hits) {
	case 0:
		return "", &ErrNoSuchSection{Want: address, Candidates: suggest(entries, want)}
	case 1:
		e := hits[0]
		return render(blocks, e.block, sectionEnd(blocks, e.block)), nil
	default:
		paths := make([]string, len(hits))
		for i, h := range hits {
			paths[i] = h.Path
		}
		return "", &ErrNoSuchSection{Want: address, Candidates: paths}
	}
}

// suggest offers headings that contain the requested text, so a near miss is
// answerable rather than merely refused.
func suggest(entries []outlineEntry, want string) []string {
	var out []string
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Title), want) {
			out = append(out, e.Path)
		}
	}
	if len(out) > 8 {
		out = out[:8]
	}
	return out
}
