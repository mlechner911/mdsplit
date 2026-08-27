package translate

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/mlechner911/mdsplit/internal/llm"
	"github.com/mlechner911/mdsplit/internal/split"
)

// piece is one span of a chunk. A literal piece is reproduced byte for byte and
// never reaches the model; only translatable pieces are sent.
//
// This is what block mode buys: code cannot be damaged, because it is never
// transmitted. No rule to follow, no check to pass, no model to trust - a
// translator that has not seen the code cannot rewrite it. It also lets a pure
// translation model (TranslateGemma and friends) be used at all: those accept
// text and a language pair, with no channel for instructions like "leave the
// code alone".
type piece struct {
	text      string
	translate bool
}

var (
	headingSplitRx = regexp.MustCompile(`^( {0,3}#{1,6}[ \t]+)(.*)$`)
	listSplitRx    = regexp.MustCompile(`^([ \t]*(?:[-*+]|\d{1,9}[.)])[ \t]+)(.*)$`)
	tableSepRx     = regexp.MustCompile(`^[ \t]*\|[ \t:|-]*\|[ \t]*$`)

	// protectRx covers the spans a translator must not touch even inside a
	// sentence. Inline code comes first so that backticks win over anything
	// they contain. The target half of a link or image is protected while its
	// label stays translatable - alt text is prose a reader sees, the path is
	// not. Instructing a model to leave these alone is politeness; restore()
	// checking that each one came back exactly once is the mechanism.
	protectRx = regexp.MustCompile(
		"`+[^`]*`+" + // inline code
			`|https?://[^\s)]+` + // bare URL
			`|\]\([^)]*\)` + // ](target) for links and images
			`|\]\[[^\]]*\]` + // ][ref] for reference links
			`|\[\^[^\]]+\]` + // [^1] footnote marker
			`|</?[a-zA-Z][^>]*>`) // inline HTML tag, e.g. <img src=...>, <br>

	// placeholderRx finds a sentinel even when the model put spaces inside the
	// brackets, which they do often enough to be worth absorbing rather than
	// rejecting a whole reply over.
	placeholderRx = regexp.MustCompile(`⟦\s*(\d+)\s*⟧`)

	// letterRx decides whether a fragment carries language at all.
	letterRx = regexp.MustCompile(`\p{L}{2,}`)

	// flagRx catches a bare command-line flag. "-out" reads like a word to a
	// letter count, and a translator duly turned it into "– aus"; anything
	// shaped like a flag is an identifier, not prose.
	flagRx = regexp.MustCompile(`^-{1,2}[\p{L}\d][\p{L}\d_-]*$`)

	// pathRx catches a bare path or a dotted identifier: ./cmd/tool,
	// internal/split, config.yaml, os.ReadFile.
	pathRx = regexp.MustCompile(`^[.~]?/?[\p{L}\d_.-]+(?:[/.][\p{L}\d_.-]+)+/?$`)
)

// translatable reports whether a fragment contains prose once the protected
// spans are removed. "-v", "`$GOBIN`" and "1.2.0" do not, and sending them to a
// translator can only make them worse.
func translatable(s string) bool {
	rest := strings.TrimSpace(protectRx.ReplaceAllString(s, ""))
	if flagRx.MatchString(rest) || pathRx.MatchString(rest) {
		return false
	}
	return letterRx.MatchString(rest)
}

// protect swaps the untouchable spans for sentinels, so the sentence around
// them still reaches the model whole. Translating the fragments between them
// separately would be worse: German word order is decided across the whole
// clause, not within the gaps.
func protect(s string) (string, []string) {
	var tokens []string
	return protectInto(s, &tokens), tokens
}

// protectInto is protect against a shared token list, so block-level and
// inline masking can share one numbering.
func protectInto(s string, tokens *[]string) string {
	return protectRx.ReplaceAllStringFunc(s, func(m string) string {
		*tokens = append(*tokens, m)
		return fmt.Sprintf("⟦%d⟧", len(*tokens)-1)
	})
}

// restore puts the protected spans back. Every sentinel must come back exactly
// once; anything else means the model dropped or duplicated one, and the reply
// is unusable rather than merely imperfect.
func restore(s string, tokens []string) (string, error) {
	if len(tokens) == 0 {
		return s, nil
	}
	s = placeholderRx.ReplaceAllString(s, "⟦$1⟧")
	for i, t := range tokens {
		ph := fmt.Sprintf("⟦%d⟧", i)
		if n := strings.Count(s, ph); n != 1 {
			return "", fmt.Errorf("placeholder %s came back %d times, expected once (%d of %d sentinels survived)",
				ph, n, len(placeholderRx.FindAllString(s, -1)), len(tokens))
		}
		s = strings.Replace(s, ph, t, 1)
	}
	return s, nil
}

// planChunk decomposes a chunk into literal and translatable pieces.
func planChunk(chunk string) []piece {
	blocks := split.ExtractBlocks(chunk)
	var out []piece
	for i, b := range blocks {
		out = append(out, planBlock(b)...)
		if i+1 < len(blocks) {
			out = append(out, piece{text: strings.Repeat("\n", b.Gap+1)})
		}
	}
	return out
}

func planBlock(b split.Block) []piece {
	switch b.Kind {
	case split.Code, split.HTML:
		return []piece{{text: b.Text}} // nie gesendet
	case split.Heading:
		if m := headingSplitRx.FindStringSubmatch(b.Text); m != nil {
			return append([]piece{{text: m[1]}}, textPieces(m[2])...)
		}
		return []piece{{text: b.Text, translate: true}}
	case split.Table:
		return planTable(b.Text)
	case split.List:
		return planList(b.Text)
	default:
		return []piece{{text: b.Text, translate: true}}
	}
}

// planTable keeps every pipe and every separator row literal, so a table cannot
// lose or gain a column no matter what comes back.
func planTable(text string) []piece {
	var out []piece
	for i, line := range strings.Split(text, "\n") {
		if i > 0 {
			out = append(out, piece{text: "\n"})
		}
		if tableSepRx.MatchString(line) {
			out = append(out, piece{text: line})
			continue
		}
		for j, cell := range strings.Split(line, "|") {
			if j > 0 {
				out = append(out, piece{text: "|"})
			}
			out = append(out, textPieces(cell)...)
		}
	}
	return out
}

// planList keeps bullets, numbers and indentation literal. A fenced block
// inside a list item is passed through like any other code.
func planList(text string) []piece {
	var out []piece
	lines := strings.Split(text, "\n")
	fence := ""
	for i, line := range lines {
		if i > 0 {
			out = append(out, piece{text: "\n"})
		}
		if fence != "" {
			out = append(out, piece{text: line})
			if isClosing(line, fence) {
				fence = ""
			}
			continue
		}
		if m, ok := fenceMarker(line); ok {
			fence = m
			out = append(out, piece{text: line})
			continue
		}
		if m := listSplitRx.FindStringSubmatch(line); m != nil {
			out = append(out, piece{text: m[1]})
			out = append(out, textPieces(m[2])...)
			continue
		}
		out = append(out, textPieces(line)...)
	}
	return out
}

// textPieces marks a fragment translatable, keeping its surrounding whitespace
// literal so indentation and cell padding survive untouched.
func textPieces(s string) []piece {
	core := strings.TrimSpace(s)
	if core == "" || !translatable(core) {
		return []piece{{text: s}}
	}
	lead := s[:strings.Index(s, core)]
	tail := s[len(lead)+len(core):]
	var out []piece
	if lead != "" {
		out = append(out, piece{text: lead})
	}
	out = append(out, piece{text: core, translate: true})
	if tail != "" {
		out = append(out, piece{text: tail})
	}
	return out
}

// segmentPrompt is the instruction for one fragment. Instruction-capable models
// honour it; a pure translation model ignores it, which is exactly why the
// structure is guaranteed mechanically rather than asked for.
func segmentPrompt(opts Options, fragment string) (string, int) {
	task := opts.Instruction
	if task == "" {
		lang := LanguageName(opts.Language)
		if lang == "" {
			lang = "the target language"
		}
		task = "Translate the text below into " + lang + "."
	}
	var b strings.Builder
	b.WriteString(task)
	b.WriteString(`

Rules:
- Output ONLY the result. No quotes, no commentary, no explanation.
- This is a fragment of a larger document. Do not add or remove sentences.
- Markers like ⟦0⟧ are placeholders. Reproduce each exactly once, unchanged.
- Keep Markdown emphasis (**bold**, *italic*) where it is.`)

	applied := 0
	if len(opts.Glossary) > 0 {
		var lines []string
		lower := strings.ToLower(fragment)
		for term, want := range opts.Glossary {
			if strings.Contains(lower, strings.ToLower(term)) {
				lines = append(lines, fmt.Sprintf("- %s = %s", term, want))
			}
		}
		if len(lines) > 0 {
			applied = len(lines)
			sortStrings(lines)
			b.WriteString("\n\nUse exactly these terms:\n" + strings.Join(lines, "\n"))
		}
	}
	return b.String(), applied
}

// byBlocks translates a chunk fragment by fragment. Identical fragments are
// translated once and reused, which both saves requests and removes a source of
// drift: the same heading or table header cannot come out two ways.
func byBlocks(ctx context.Context, c *llm.Client, chunk string, opts Options, st *stats) (string, error) {
	pieces := planChunk(chunk)
	memo := map[string]string{}
	var b strings.Builder

	for _, p := range pieces {
		if !p.translate {
			b.WriteString(p.text)
			continue
		}
		masked, tokens := protect(p.text)
		out, hit := memo[masked]
		if !hit {
			system, applied := segmentPrompt(opts, p.text)
			reply, err := c.Ask(ctx, opts.prompt(system, masked))
			if err != nil {
				return "", err
			}
			out = strings.TrimSpace(reply)
			st.requests++
			st.glossary += applied
			memo[masked] = out
		} else {
			st.reused++
		}
		fitted, err := fitFragment(p.text, out)
		if err == nil {
			out, err = restore(fitted, tokens)
		}
		if err != nil {
			// Ein verlorener Platzhalter macht die Antwort unbrauchbar, nicht
			// nur unschön - dann bleibt das Original stehen.
			st.kept++
			b.WriteString(p.text)
			continue
		}
		b.WriteString(out)
	}
	return b.String(), nil
}

// blankLineRx finds a run of newlines that would end a block.
var blankLineRx = regexp.MustCompile(`\n[ \t]*\n[\s]*`)

var structuralStartRx = regexp.MustCompile("^[ \t]*(?:#{1,6}[ \t]|[-*+][ \t]|\\d{1,9}[.)][ \t]|>|\\||`{3,}|~{3,})")

// fitFragment forces a reply back into the shape of the fragment it replaces.
//
// Block mode keeps every marker literal, but a reply can still smuggle
// structure in through the text itself. A fragment that was one line - a table
// cell, a list item, a heading - must stay one line: a stray newline does not
// merely look wrong, it re-parses into a different block and shifts every block
// after it. And a reply that begins with "- " or "## " turns a paragraph into a
// list or a heading. Neither is a translation error the reader would notice
// later; both silently change the document.
func fitFragment(src, out string) (string, error) {
	if !strings.Contains(src, "\n") && strings.Contains(out, "\n") {
		out = strings.Join(strings.Fields(strings.ReplaceAll(out, "\n", " ")), " ")
	}
	// A blank line ends a block. No fragment the planner produces contains one
	// - a paragraph stops at the first blank line, and headings, cells and list
	// items are single lines - so a blank line in the reply is the model
	// splitting one block into two.
	if !blankLineRx.MatchString(src) {
		out = blankLineRx.ReplaceAllString(out, "\n")
	}
	if structuralStartRx.MatchString(out) && !structuralStartRx.MatchString(src) {
		return "", fmt.Errorf("reply starts a new Markdown construct the source did not: %.40q", out)
	}
	return out, nil
}

// stats counts what happened during one chunk.
type stats struct {
	requests int // fragments actually sent
	reused   int // fragments answered from the memo
	kept     int // fragments left in the source language after a bad reply
	masked   int // spans replaced by a sentinel before sending (chunk mode)
	glossary int
}
