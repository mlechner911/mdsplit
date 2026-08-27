package job

import (
	"fmt"
	"strings"
)

// stampKey is the top-level YAML key the stamp lives under.
const stampKey = "translation:"

// YAML renders the provenance as the body of a `translation:` block.
func (p Provenance) YAML(parts string) string {
	var b strings.Builder
	b.WriteString(stampKey + "\n")
	add := func(k, v string) {
		if v != "" {
			fmt.Fprintf(&b, "  %s: %s\n", k, yamlScalar(v))
		}
	}
	add("tool", p.Tool)
	add("version", p.Version)
	add("url", p.URL)
	add("source", p.Source)
	add("source_sha256", p.SourceSHA)
	if p.SourceChars > 0 {
		fmt.Fprintf(&b, "  source_chars: %d\n", p.SourceChars)
	}
	add("target_lang", p.TargetLang)
	add("model", p.Model)
	add("mode", p.Mode)
	add("parts", parts)
	add("translated", p.Translated)
	if p.Machine {
		b.WriteString("  machine_translation: true\n")
	}
	return b.String()
}

// yamlScalar quotes a value only where YAML needs it.
func yamlScalar(v string) string {
	if v == "" {
		return `""`
	}
	if strings.ContainsAny(v, ":#{}[]&*!|>'\"%@`\n") || strings.TrimSpace(v) != v {
		return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", " ").Replace(v) + `"`
	}
	return v
}

// Stamp writes the provenance into a document's YAML front matter.
//
// It never simply prepends. A document that already has front matter would be
// destroyed by a second `---` block: the original block stops being metadata
// and starts being content. So an existing block is edited in place, and a
// `translation:` key already in it is replaced rather than duplicated.
func Stamp(doc, block string) string {
	body, rest, ok := splitFrontMatter(doc)
	if !ok {
		return "---\n" + block + "---\n\n" + strings.TrimLeft(doc, "\n")
	}
	return "---\n" + replaceKey(body, block) + "---\n" + rest
}

// splitFrontMatter returns the body of a leading YAML block and the document
// after it. A `---` that is not closed is a thematic break, not front matter.
func splitFrontMatter(doc string) (body, rest string, ok bool) {
	if !strings.HasPrefix(doc, "---\n") {
		return "", "", false
	}
	lines := strings.Split(doc, "\n")
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], " \t") == "---" {
			body = strings.Join(lines[1:i], "\n")
			if body != "" {
				body += "\n"
			}
			rest = strings.Join(lines[i+1:], "\n")
			return body, rest, true
		}
	}
	return "", "", false
}

// replaceKey drops an existing `translation:` block from the front matter body
// and appends the new one. A top-level key ends where the next unindented,
// non-empty line begins.
func replaceKey(body, block string) string {
	lines := strings.Split(body, "\n")
	var kept []string
	skipping := false
	for _, l := range lines {
		if skipping {
			if l == "" || strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t") {
				continue
			}
			skipping = false
		}
		if strings.HasPrefix(l, stampKey) {
			skipping = true
			continue
		}
		kept = append(kept, l)
	}
	out := strings.Join(kept, "\n")
	out = strings.TrimRight(out, "\n")
	if out != "" {
		out += "\n"
	}
	return out + block
}

// Unstamp removes a `translation:` block, so the round-trip check compares the
// document rather than the stamp we just added to it.
func Unstamp(doc string) string {
	body, rest, ok := splitFrontMatter(doc)
	if !ok {
		return doc
	}
	stripped := replaceKey(body, "")
	if strings.TrimSpace(stripped) == "" {
		return strings.TrimLeft(rest, "\n")
	}
	return "---\n" + stripped + "---\n" + rest
}
