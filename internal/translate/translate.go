// Package translate runs one chunk of a split through an LLM and stores the
// result, once the reply has been checked against the source structure.
//
// Every chunk is an isolated request: system prompt plus that one chunk, no
// conversation history. This is what actually keeps the context flat - a tool
// that merely hands chunks to a chat client still accumulates every chunk and
// every reply in that client's transcript.
package translate

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/mlechner911/mdsplit/internal/job"
	"github.com/mlechner911/mdsplit/internal/llm"
	"github.com/mlechner911/mdsplit/internal/split"
)

// languages maps the common short codes to a name a model understands. An
// unknown value is passed through, so "Bavarian" or "pt-BR" work too.
var languages = map[string]string{
	"de": "German", "en": "English", "es": "Spanish", "fr": "French",
	"it": "Italian", "pt": "Portuguese", "nl": "Dutch", "pl": "Polish",
	"ru": "Russian", "ja": "Japanese", "zh": "Chinese", "ko": "Korean",
	"tr": "Turkish", "cs": "Czech", "sv": "Swedish", "da": "Danish",
}

// termMatchers compiles one matcher per glossary term.
//
// An exact substring is not enough. The extractor finds a term in whichever
// form recurs - "code fences" - while the prose uses both numbers, and the
// singular occurrences then get no rule at all. Measured on this README: the
// glossary said code fences, the text said "a code fence ends up half in one
// piece", the rule never fired, and three of four languages invented something
// (Code-Spaltengrenze, una valla de código, une balise de code) for exactly
// those sentences.
//
// The last word of a term is matched with an optional plural, on word
// boundaries so that "part" cannot match "particular".
func termMatchers(gloss map[string]string) map[string]*regexp.Regexp {
	out := make(map[string]*regexp.Regexp, len(gloss))
	for term := range gloss {
		words := strings.Fields(term)
		if len(words) == 0 {
			continue
		}
		parts := make([]string, len(words))
		for i, w := range words {
			parts[i] = regexp.QuoteMeta(w)
		}
		last := words[len(words)-1]
		// Nur ein "s" abschneiden. Zweimal zu kürzen macht aus "fences" den
		// Stamm "fenc", und der trifft "fences", aber nicht "fence" - also
		// genau die Form, deretwegen das hier existiert.
		stem := strings.TrimSuffix(last, "s")
		if len(stem) >= 3 {
			parts[len(parts)-1] = regexp.QuoteMeta(stem) + `(?:e?s)?`
		}
		rx, err := regexp.Compile(`(?i)\b` + strings.Join(parts, `[\s-]+`) + `\b`)
		if err != nil {
			continue
		}
		out[term] = rx
	}
	return out
}

// prompt builds the payload for one request, carrying the language pair so a
// template can render whatever shape its model expects.
func (o Options) prompt(system, user string) llm.PromptData {
	src := o.SourceLang
	if src == "" {
		src = "en"
	}
	return llm.PromptData{
		System:         system,
		User:           user,
		SourceLang:     src,
		TargetLang:     o.Language,
		SourceLangName: LanguageName(src),
		TargetLangName: LanguageName(o.Language),
	}
}

// matcher returns the compiled matcher for a glossary term.
func (o *Options) matcher(term string) *regexp.Regexp {
	if o.matchers == nil {
		o.matchers = termMatchers(o.Glossary)
	}
	return o.matchers[term]
}

// LanguageName resolves a code or name into something to put in a prompt.
func LanguageName(s string) string {
	s = strings.TrimSpace(s)
	if full, ok := languages[strings.ToLower(s)]; ok {
		return full
	}
	return s
}

// Mode selects how much of a chunk goes to the model at once.
type Mode string

const (
	// ModeBlock sends only the prose fragments and reproduces code, HTML,
	// bullets, pipes and inline code mechanically. Structure is guaranteed
	// rather than requested, so even a model with no instruction channel is
	// safe to use. This is the default.
	ModeBlock Mode = "block"
	// ModeChunk sends the whole chunk and relies on the model to honour the
	// rules, with VerifyStructure as the net. Fewer requests and more context
	// per request; needs an instruction-following model.
	ModeChunk Mode = "chunk"
)

// Options controls one translation run.
type Options struct {
	// Mode picks block or chunk granularity; empty means ModeBlock.
	Mode Mode
	// Language is the target language; a code like "de" is expanded.
	Language string
	// SourceLang is the language the document is in; empty means "en".
	SourceLang string
	// Instruction replaces the default translation task when set, so the same
	// machinery can rewrite or summarise instead of translate.
	Instruction string
	// Glossary pins terminology. Only entries whose term occurs in the chunk
	// are sent, which keeps the prompt small and the rules relevant.
	Glossary map[string]string
	// matchers is built lazily from Glossary; see termMatchers.
	matchers map[string]*regexp.Regexp
	// SkipVerify stores the reply even when its structure drifted. Off by
	// default: a damaged chunk is worse than a missing one, because the damage
	// is only found when the whole document is reassembled.
	SkipVerify bool
}

// Result describes what happened to one part.
type Result struct {
	Part     int
	Mode     Mode
	InChars  int
	OutChars int
	Glossary int // glossary entries that applied to this chunk
	Requests int // requests actually sent
	Masked   int // spans replaced by a sentinel before sending (chunk mode)
	Reused   int // fragments answered from the memo (block mode)
	Kept     int // fragments left untranslated after an unusable reply
	Batched  int // short fragments offered together in one request
	Accepted int // of those, how many came back usable
	Verified bool
	// Structure is set when verification failed but SkipVerify was on.
	Structure error
}

// systemPrompt builds the instruction sent with every chunk.
func systemPrompt(opts *Options, chunk string) (string, []string) {
	task := opts.Instruction
	if task == "" {
		lang := LanguageName(opts.Language)
		if lang == "" {
			lang = "the target language"
		}
		task = "You are a professional technical translator. Translate the Markdown below into " + lang + "."
	}

	var b strings.Builder
	b.WriteString(task)
	b.WriteString(`

Rules:
- Output ONLY the resulting Markdown. No preamble, no commentary, no explanation.
- Do NOT wrap your whole answer in a code fence.
- Keep the structure identical: the same blocks in the same order, headings at
  the same level, the same number of list items and table rows.
- Markers like ⟦7⟧ stand for code blocks, links and inline code that have been
  removed. Reproduce every marker exactly once, unchanged, in its place. Never
  invent, drop, merge or renumber one.
- Leave file paths and command names untouched.`)

	var applied []string
	if len(opts.Glossary) > 0 {
		var lines []string
		for term, want := range opts.Glossary {
			if rx := opts.matcher(term); rx != nil && rx.MatchString(chunk) {
				lines = append(lines, fmt.Sprintf("- %s = %s", term, want))
				applied = append(applied, term)
			}
		}
		if len(lines) > 0 {
			sortStrings(lines)
			b.WriteString("\n\nUse exactly these terms:\n")
			b.WriteString(strings.Join(lines, "\n"))
		}
	}
	return b.String(), applied
}

// sortStrings keeps the prompt stable across runs, so the same chunk produces
// the same request and a redo is comparable.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// unwrapFence removes a code fence the model wrapped around its whole answer.
// Small models do this often; the source chunk itself being a single fence is
// the one case where the wrapper is real content.
func unwrapFence(reply, source string) string {
	trimmed := strings.TrimSpace(reply)
	lines := strings.Split(trimmed, "\n")
	if len(lines) < 3 {
		return reply
	}
	marker, ok := fenceMarker(lines[0])
	if !ok || !isClosing(lines[len(lines)-1], marker) {
		return reply
	}
	if _, srcIsFence := fenceMarker(strings.SplitN(strings.TrimSpace(source), "\n", 2)[0]); srcIsFence {
		return reply // die Quelle ist selbst ein Zaun - der Rahmen ist Inhalt
	}
	return strings.Join(lines[1:len(lines)-1], "\n")
}

func fenceMarker(line string) (string, bool) {
	t := strings.TrimSpace(line)
	for _, c := range []byte{'`', '~'} {
		n := 0
		for n < len(t) && t[n] == c {
			n++
		}
		if n >= 3 {
			return t[:n], true
		}
	}
	return "", false
}

func isClosing(line, marker string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, marker) && strings.Trim(t, string(marker[0])) == ""
}

// Part translates one part of a job and writes the result back.
func Part(ctx context.Context, c *llm.Client, m *job.Manifest, n int, opts Options) (Result, error) {
	src, _, err := m.ReadChunk(n)
	if err != nil {
		return Result{}, err
	}
	// Immer vom Original ausgehen, nie von einer früheren Übersetzung.
	if sp, err := m.SourcePath(n); err == nil {
		if data, err := readFile(sp); err == nil {
			src = data
		}
	}

	mode := opts.Mode
	if mode == "" {
		mode = ModeBlock
	}

	var out string
	st := newStats()
	switch mode {
	case ModeChunk:
		masked, tokens := maskChunk(src)
		system, applied := systemPrompt(&opts, masked)
		reply, err := c.Ask(ctx, opts.prompt(system, masked))
		if err != nil {
			return Result{}, fmt.Errorf("part %d: %w", n, err)
		}
		restored, err := restore(strings.TrimSpace(unwrapFence(reply, masked)), tokens)
		if err != nil {
			return Result{}, fmt.Errorf("part %d rejected: %w", n, err)
		}
		out = strings.TrimSpace(restored) + "\n"
		for _, t := range applied {
			st.terms[t] = true
		}
		st.requests, st.masked = 1, len(tokens)
	default:
		body, err := byBlocks(ctx, c, src, opts, &st)
		if err != nil {
			return Result{}, fmt.Errorf("part %d: %w", n, err)
		}
		out = strings.TrimSpace(body) + "\n"
	}

	res := Result{
		Part: n, Mode: mode, InChars: len(src), OutChars: len(out),
		Glossary: len(st.terms), Requests: st.requests, Reused: st.reused,
		Kept: st.kept, Masked: st.masked,
		Batched: st.batched, Accepted: st.accepted,
	}
	if err := split.VerifyStructure(src, out); err != nil {
		res.Structure = err
		if !opts.SkipVerify {
			return res, fmt.Errorf("part %d rejected: %w", n, err)
		}
	} else {
		res.Verified = true
	}
	if err := m.WriteChunk(n, out); err != nil {
		return res, err
	}
	return res, nil
}
