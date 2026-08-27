// Package glossary pins terminology across a translation run.
//
// Because every chunk is translated in isolation, nothing otherwise stops a
// model from rendering the same term two ways in two chunks. Measured on this
// project's own README, one term - "code fences" - came back as "delimitador de
// código", "un code" (the term simply dropped), "代码块" and "Code-Abschnitte".
//
// The glossary is built once, before any translation, and frozen. That ordering
// is the point: a glossary grown while translating would leave the first chunks
// translated with an empty one and the last with a full one, baking the very
// inconsistency it was meant to remove into the parts done first - and making
// a single part impossible to redo on its own.
package glossary

import (
	"regexp"
	"sort"
	"strings"

	"github.com/mlechner911/mdsplit/internal/split"
)

// Candidate is a term worth deciding on, with the evidence for it.
type Candidate struct {
	Term     string `json:"term"`
	Chunks   int    `json:"chunks"`  // how many chunks it appears in
	Count    int    `json:"count"`   // total occurrences in prose
	InCode   bool   `json:"in_code"` // also appears inside code or an identifier
	Examples string `json:"example"` // one sentence it occurs in
	score    int
}

var (
	codeSpanRx = regexp.MustCompile("`+[^`]*`+")
	wordRx     = regexp.MustCompile(`\p{L}[\p{L}\d-]*`)
	sentenceRx = regexp.MustCompile(`[^.!?\n]*[.!?]`)
	// clauseRx splits on punctuation so a pair is never formed across it.
	// "code fences, tables and list items" must not yield "fences tables".
	clauseRx = regexp.MustCompile(`[,.;:!?()\[\]{}"—–\n]+`)
)

// stopwords keeps the everyday scaffolding of English out of a terminology
// list. It is deliberately short: the frequency and in-code signals do most of
// the work, and an over-long list would drop real terms like "block" or "part".
var stopwords = map[string]bool{
	"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
	"of": true, "to": true, "in": true, "on": true, "at": true, "by": true,
	"for": true, "with": true, "from": true, "into": true, "as": true,
	"is": true, "are": true, "was": true, "were": true, "be": true, "been": true,
	"it": true, "its": true, "that": true, "this": true, "these": true, "those": true,
	"not": true, "no": true, "so": true, "than": true, "then": true, "when": true,
	"if": true, "can": true, "will": true, "would": true, "does": true, "do": true,
	"has": true, "have": true, "had": true, "you": true, "your": true, "they": true,
	"there": true, "which": true, "what": true, "how": true, "why": true,
	"one": true, "two": true, "every": true, "each": true, "all": true, "any": true,
	"more": true, "most": true, "only": true, "also": true, "just": true,
	"other": true, "same": true, "such": true, "own": true, "up": true, "out": true,
	"over": true, "after": true, "before": true, "while": true, "because": true,
	"who": true, "whose": true, "where": true, "here": true, "now": true,
	// Allerweltsverben und -adverbien: sie tragen keine Terminologie, tauchen
	// aber häufig genug auf, um die Frequenzsignale zu schlagen.
	"whether": true, "next": true, "returns": true, "return": true,
	"comes": true, "come": true, "back": true, "build": true, "test": true,
	"use": true, "used": true, "using": true, "make": true, "makes": true,
	"made": true, "get": true, "gets": true, "give": true, "gives": true,
	"keep": true, "keeps": true, "take": true, "takes": true, "need": true,
	"needs": true, "want": true, "wants": true, "look": true, "looks": true,
	"say": true, "says": true, "see": true, "seen": true, "still": true,
	"even": true, "already": true, "instead": true, "rather": true,
	"whole": true, "exactly": true, "without": true, "actually": true,
	// Verbformen auf -s bilden mit dem Vorwort gern Scheinbegriffe
	// ("chunk starts", "tool writes"), sind aber Satzbau, nicht Terminologie.
	"starts": true, "ends": true, "goes": true, "works": true, "holds": true,
	"means": true, "shows": true, "reads": true, "writes": true, "runs": true,
	"contains": true, "carries": true, "counts": true, "count": true,
	"alone": true, "always": true, "never": true, "often": true, "later": true,
	"first": true, "last": true, "both": true, "either": true, "neither": true,
}

// Candidates proposes terminology from a document, without a model.
//
// Three signals, in order of weight. A word or pair that recurs across several
// chunks is terminology rather than phrasing. A word that also appears inside
// code or an identifier somewhere in the document is almost certainly domain
// vocabulary - "chunk" is prose in one line and `chunks/` in the next. And
// two-word phrases outrank single words, because that is where translations
// actually drift ("code fence", "round-trip", "block mode").
func Candidates(doc string, limit int) []Candidate {
	if limit <= 0 {
		limit = 40
	}
	codeVocab := codeWords(doc)

	type acc struct {
		count   int
		chunks  map[int]bool
		example string
	}
	seen := map[string]*acc{}

	for i, para := range proseParagraphs(doc) {
		clean := codeSpanRx.ReplaceAllString(para, " ")
		note := func(term, example string) {
			key := strings.ToLower(term)
			a := seen[key]
			if a == nil {
				a = &acc{chunks: map[int]bool{}, example: example}
				seen[key] = a
			}
			a.count++
			a.chunks[i] = true
		}
		for _, clause := range clauseRx.Split(clean, -1) {
			words := wordRx.FindAllString(clause, -1)
			for j, w := range words {
				lw := strings.ToLower(w)
				if len(lw) < 3 || stopwords[lw] {
					continue
				}
				note(lw, sentenceAround(para, w))
				if j+1 < len(words) {
					next := strings.ToLower(words[j+1])
					if len(next) >= 3 && !stopwords[next] {
						note(lw+" "+next, sentenceAround(para, w))
					}
				}
			}
		}
	}

	var out []Candidate
	for term, a := range seen {
		if len(a.chunks) < 2 && a.count < 3 {
			continue // einmaliges Vorkommen ist Formulierung, nicht Terminologie
		}
		phrase := strings.Contains(term, " ")
		inCode := codeVocab[headWord(term)]
		// Ein einzelnes gebräuchliches Wort ohne Code-Beleg ist Sprache, keine
		// Terminologie. Mehrwortbegriffe bleiben immer - dort driftet es.
		if !phrase && !inCode {
			continue
		}
		c := Candidate{
			Term: term, Chunks: len(a.chunks), Count: a.count,
			InCode: inCode, Examples: a.example,
		}
		c.score = c.Chunks*3 + c.Count
		if inCode {
			c.score += 8
		}
		if phrase {
			c.score += 5
		}
		out = append(out, c)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return out[i].Term < out[j].Term
	})
	out = dropIdentifiers(out, doc)
	out = mergeVariants(out)
	out = dropSubsumed(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// headWord returns the first word of a phrase.
func headWord(term string) string {
	if i := strings.IndexByte(term, ' '); i >= 0 {
		return term[:i]
	}
	return term
}

// dropIdentifiers removes a candidate that names a thing in the code rather
// than a concept in the prose. "put chunk" comes from put_chunk, a tool name
// that must never be translated - and a 7B duly rendered it "Chunk
// hinzufügen". A phrase is an identifier when its words, joined the way code
// joins them, appear in the document.
func dropIdentifiers(in []Candidate, doc string) []Candidate {
	code := strings.ToLower(doc)
	isIdent := func(term string) bool {
		words := strings.Fields(term)
		if len(words) < 2 {
			return false
		}
		for _, sep := range []string{"_", "-", "", "."} {
			if strings.Contains(code, strings.Join(words, sep)+"(") ||
				strings.Contains(code, "`"+strings.Join(words, sep)) {
				return true
			}
		}
		return strings.Contains(code, strings.Join(words, "_"))
	}
	var out []Candidate
	for _, c := range in {
		if isIdent(c.Term) {
			continue
		}
		out = append(out, c)
	}
	return out
}

// mergeVariants folds "blank-line" into "blank line": the hyphen is an
// attributive form of the same term, and two entries for one concept give a
// reviewer two chances to disagree with themselves.
func mergeVariants(in []Candidate) []Candidate {
	seen := map[string]int{}
	var out []Candidate
	for _, c := range in {
		key := strings.ReplaceAll(c.Term, "-", " ")
		if i, ok := seen[key]; ok {
			out[i].Count += c.Count
			if c.Chunks > out[i].Chunks {
				out[i].Chunks = c.Chunks
			}
			continue
		}
		seen[key] = len(out)
		c.Term = key
		out = append(out, c)
	}
	return out
}

// dropSubsumed removes a single word when a stronger phrase already contains
// it, so "code" does not compete with "code fence" for a slot.
func dropSubsumed(in []Candidate) []Candidate {
	phrases := map[string]bool{}
	for _, c := range in {
		if strings.Contains(c.Term, " ") {
			for _, w := range strings.Fields(c.Term) {
				phrases[w] = true
			}
		}
	}
	var out []Candidate
	for _, c := range in {
		if !strings.Contains(c.Term, " ") && phrases[c.Term] {
			continue
		}
		out = append(out, c)
	}
	return out
}

// proseParagraphs returns the blocks a translator would actually see: code and
// HTML are excluded, because a term is only worth pinning where it is prose.
func proseParagraphs(doc string) []string {
	var out []string
	for _, b := range split.ExtractBlocks(doc) {
		if b.Kind == split.Code || b.Kind == split.HTML {
			continue
		}
		out = append(out, b.Text)
	}
	return out
}

// identRx matches a token that is an identifier rather than a word: it carries
// a separator or a camelCase hump.
var identRx = regexp.MustCompile(`[\p{L}\d]+(?:[_./-][\p{L}\d]+)+|[a-z]+[A-Z][\p{L}\d]*`)

// codeWords collects the vocabulary that marks a word as domain terminology
// rather than plain English.
//
// Inline code counts wholesale: a person writing `manifest` in backticks means
// the thing, not the word. Fenced blocks do not, because they are full of
// English - comments, JSON string values, prose in help text - and counting
// that made "without", "returns" and "exactly" look like technical terms. From
// a fence, only identifier-shaped tokens count: index.json, put_chunk,
// ExtractBlocks, cmd/mdsplit.
func codeWords(doc string) map[string]bool {
	vocab := map[string]bool{}
	addAll := func(s string) {
		for _, w := range wordRx.FindAllString(s, -1) {
			if len(w) >= 3 {
				vocab[strings.ToLower(w)] = true
			}
		}
	}
	addIdentifiers := func(s string) {
		for _, tok := range identRx.FindAllString(s, -1) {
			for _, w := range wordRx.FindAllString(tok, -1) {
				if len(w) >= 3 {
					vocab[strings.ToLower(w)] = true
				}
			}
		}
	}
	for _, b := range split.ExtractBlocks(doc) {
		if b.Kind == split.Code {
			addIdentifiers(b.Text)
			continue
		}
		for _, m := range codeSpanRx.FindAllString(b.Text, -1) {
			addAll(m)
		}
	}
	return vocab
}

// sentenceAround finds a sentence containing the word, to show a reviewer what
// the term means here before they decide on a translation.
func sentenceAround(para, word string) string {
	for _, s := range sentenceRx.FindAllString(para, -1) {
		if strings.Contains(strings.ToLower(s), strings.ToLower(word)) {
			s = strings.Join(strings.Fields(s), " ")
			if len(s) > 160 {
				s = s[:160] + "…"
			}
			return s
		}
	}
	return ""
}
