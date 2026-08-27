package translate

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/mlechner911/mdsplit/internal/llm"
)

// shortFragment is the length below which a fragment is translated together
// with its neighbours rather than on its own.
//
// A table cell reading "never transmitted" is three imperative words. Alone in
// a request, under a system prompt full of rules, a small model cannot tell it
// apart from one of those rules and answers by translating the rules - measured
// on this README, ten of a table's twenty-three cells came back unusable. Sent
// together, the same cells read as a list of things to translate, which is what
// they are.
const shortFragment = 60

// numberedLineRx reads one line of a numbered reply.
var numberedLineRx = regexp.MustCompile(`^\s*(\d+)\s*[.):\-]\s*(.*)$`)

// batchPrompt asks for a numbered list back.
func batchPrompt(opts Options, items []string) (string, string) {
	task := opts.Instruction
	if task == "" {
		lang := LanguageName(opts.Language)
		if lang == "" {
			lang = "the target language"
		}
		task = "Translate each numbered line into " + lang + "."
	}
	system := task + `

These are short fragments from one table or list in a technical document.

Rules:
- Answer with the same numbers, one line each, in the same order.
- Translate only. Do not explain, do not merge lines, do not add lines.
- Markers like ⟦0⟧ are placeholders. Reproduce each exactly once, unchanged.
- Keep every line as short as the original. These are labels, not sentences.`

	var b strings.Builder
	for i, it := range items {
		fmt.Fprintf(&b, "%d. %s\n", i+1, it)
	}
	return system, b.String()
}

// batchShort translates a chunk's short fragments in one request and primes the
// memo with the results.
//
// It is an improvement, not a requirement: anything the reply does not cover, or
// that fails the same checks a single translation must pass, is simply absent
// from the memo and gets translated on its own afterwards.
func batchShort(ctx context.Context, c *llm.Client, pieces []piece, opts Options, memo map[string]string, st *stats) {
	var items []string
	sources := map[string]string{} // masked -> original fragment
	seen := map[string]bool{}
	for _, p := range pieces {
		if !p.translate || len([]rune(p.text)) > shortFragment {
			continue
		}
		masked, _ := protect(p.text)
		if seen[masked] {
			continue
		}
		seen[masked] = true
		sources[masked] = p.text
		items = append(items, masked)
	}
	if len(items) < 2 {
		return // ein einzelnes Fragment gewinnt nichts durch eine Liste
	}

	system, user := batchPrompt(opts, items)
	reply, err := c.Ask(ctx, opts.prompt(system, user))
	if err != nil {
		return // der Einzelweg übernimmt
	}
	st.requests++
	st.batched = len(items)

	for _, line := range strings.Split(reply, "\n") {
		m := numberedLineRx.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil || n < 1 || n > len(items) {
			continue
		}
		masked := items[n-1]
		out := strings.TrimSpace(m[2])
		if out == "" {
			continue
		}
		fitted, err := fitFragment(sources[masked], out)
		if err != nil {
			continue // fällt auf den Einzelweg zurück
		}
		memo[masked] = fitted
	}
	st.accepted = len(memo)
}
