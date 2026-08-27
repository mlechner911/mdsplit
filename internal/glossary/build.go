package glossary

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/mlechner911/mdsplit/internal/llm"
)

// FileName is the reviewable artifact next to the chunks.
const FileName = "glossary.json"

// File is what gets written. It is meant to be read and edited by a person:
// "Interface = Schnittstelle" is a decision, not a fact, and this is the one
// place in the pipeline where a couple of minutes of human judgement is cheap
// and pays off across every chunk.
type File struct {
	// SourceLang is the language the terms are in. Recorded because a glossary
	// is only valid for the pair it was built for.
	SourceLang string            `json:"source_lang"`
	TargetLang string            `json:"target_lang"`
	Model      string            `json:"model,omitempty"`
	Generated  string            `json:"generated,omitempty"`
	Terms      map[string]string `json:"terms"`
	// Notes carries the evidence for each term so a reviewer can judge it
	// without opening the source.
	Notes map[string]string `json:"notes,omitempty"`
}

// Load reads a glossary from a chunk directory. A missing file is not an error:
// translating without a glossary is the normal case.
func Load(dir string) (*File, error) {
	data, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read glossary: %w", err)
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse glossary: %w", err)
	}
	return &f, nil
}

// Save writes the glossary for review.
func (f *File) Save(dir string) error {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("encode glossary: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, FileName), append(data, '\n'), 0o644)
}

// prompt asks for the whole list in one request.
//
// One call, not one per chunk. A per-chunk pass would make the glossary depend
// on the order chunks were processed in, and it would tie a fragile structured
// output to the valuable one: a JSON parse failure would cost a translation
// too. Here a failure costs a retry and nothing else.
func prompt(srcLang, lang string, cands []Candidate) (string, string) {
	system := "You are a terminology expert preparing a glossary for a technical translation from " +
		srcLang + " into " + lang + ".\n\n" +
		"For each " + srcLang + " term, give the translation a professional technical translator would use " +
		"consistently throughout a software manual.\n\n" +
		"Rules:\n" +
		"- Answer with a single JSON object mapping each " + srcLang + " term to its " + lang + " translation.\n" +
		"- No commentary, no code fence, no other keys.\n" +
		"- Leave a term unchanged when that is what practitioners actually say in " + lang + ".\n" +
		"- Never translate an identifier, a flag, a file name or a command.\n" +
		"- Each value is a term of roughly the same length as the key. Never a sentence, " +
		"never an explanation, never alternatives separated by slashes."

	var b strings.Builder
	b.WriteString("Translate each term. Answer with the term as key and its translation as value.\n")
	b.WriteString("A value must be a term, never a sentence and never an explanation.\n\n")
	for _, c := range cands {
		fmt.Fprintf(&b, "- %s\n", c.Term)
	}
	return system, b.String()
}

// jsonObjectRx finds the first JSON object in a reply, so a model that adds a
// sentence or wraps the answer in a fence still gets understood.
var jsonObjectRx = regexp.MustCompile(`(?s)\{.*\}`)

// Build asks the model for one glossary in one request. srcLang and lang are
// language names ("English", "German"), because that is what a prompt reads.
func Build(ctx context.Context, c *llm.Client, srcLang, lang string, cands []Candidate) (map[string]string, error) {
	if len(cands) == 0 {
		return map[string]string{}, nil
	}
	if srcLang == "" {
		srcLang = "English"
	}
	system, user := prompt(srcLang, lang, cands)
	reply, err := c.Ask(ctx, llm.PromptData{
		System: system, User: user,
		TargetLang: lang, TargetLangName: lang,
		SourceLang: srcLang, SourceLangName: srcLang,
	})
	if err != nil {
		return nil, err
	}
	raw := jsonObjectRx.FindString(reply)
	if raw == "" {
		return nil, fmt.Errorf("no JSON object in the reply: %.200q", reply)
	}
	var terms map[string]string
	if err := json.Unmarshal([]byte(raw), &terms); err != nil {
		return nil, fmt.Errorf("parse glossary JSON: %w (reply: %.200q)", err, raw)
	}
	return clean(terms, cands), nil
}

// clean drops entries the model invented, left untranslated, or answered with a
// sentence.
//
// The last one is not cosmetic. A glossary entry is injected into every
// translation prompt that mentions the term, as "term = value". A value that is
// a whole sentence turns a rule into noise and can steer the translation
// outright - measured here: "chunk starts" came back as "un bloque que comienza
// en mitad de una fence es dañino." A term that came back unchanged is dropped
// too: it carries no decision and would spend prompt budget saying nothing.
func clean(terms map[string]string, cands []Candidate) map[string]string {
	asked := map[string]bool{}
	for _, c := range cands {
		asked[strings.ToLower(c.Term)] = true
	}
	out := map[string]string{}
	for k, v := range terms {
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if k == "" || v == "" || !asked[strings.ToLower(k)] || strings.EqualFold(k, v) {
			continue
		}
		if !isTerm(k, v) {
			continue
		}
		out[k] = v
	}
	return out
}

// isTerm rejects a value that is a sentence, an explanation or a list of
// alternatives rather than a term.
func isTerm(key, value string) bool {
	if strings.ContainsAny(value, ".!?;\n") {
		return false
	}
	if strings.Contains(value, " / ") || strings.Contains(value, "(") {
		return false
	}
	keyWords := len(strings.Fields(key))
	if len(strings.Fields(value)) > keyWords+3 {
		return false
	}
	return len([]rune(value)) <= 4*len([]rune(key))+20
}

// Notes renders the evidence for each candidate, for the reviewer.
func Notes(cands []Candidate) map[string]string {
	notes := map[string]string{}
	for _, c := range cands {
		n := fmt.Sprintf("%d occurrences in %d chunks", c.Count, c.Chunks)
		if c.InCode {
			n += ", also appears in code"
		}
		notes[c.Term] = n
	}
	return notes
}

// Sorted returns the terms in a stable order for printing.
func Sorted(terms map[string]string) []string {
	out := make([]string, 0, len(terms))
	for k := range terms {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
