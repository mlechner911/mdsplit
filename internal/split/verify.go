package split

import (
	"fmt"
	"strings"
)

// StructureError reports how a rewritten chunk drifted from its source.
type StructureError struct {
	Reasons []string
}

func (e *StructureError) Error() string {
	if len(e.Reasons) == 1 {
		return e.Reasons[0]
	}
	return fmt.Sprintf("%d structural differences: %s", len(e.Reasons), strings.Join(e.Reasons, "; "))
}

// kindName renders a Kind for an error message.
func kindName(k Kind) string {
	switch k {
	case Heading:
		return "heading"
	case Code:
		return "code fence"
	case Table:
		return "table"
	case List:
		return "list"
	case HTML:
		return "html block"
	default:
		return "paragraph"
	}
}

// VerifyStructure checks that a translated or rewritten chunk still has the
// shape of its source. A translator is allowed to change words; it is not
// allowed to change the document.
//
// This is the check a generic chunker cannot offer: because the splitter
// already parses the block structure, it can tell a translation from damage.
// Three things are compared:
//
//   - the sequence of block kinds, so no block is dropped, added or merged;
//   - code fences byte for byte, because code is not prose and must survive
//     untouched (this is where a helpful model renames identifiers);
//   - heading levels and table row counts, which carry structure rather than
//     language.
func VerifyStructure(orig, rewritten string) error {
	a, b := ExtractBlocks(orig), ExtractBlocks(rewritten)
	var reasons []string

	if len(a) != len(b) {
		reasons = append(reasons, fmt.Sprintf("block count changed: %d in the source, %d in the reply", len(a), len(b)))
	}
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i].Kind != b[i].Kind {
			reasons = append(reasons, fmt.Sprintf("block %d became a %s, was a %s", i+1, kindName(b[i].Kind), kindName(a[i].Kind)))
			continue
		}
		switch a[i].Kind {
		case Code:
			if a[i].Text != b[i].Text {
				reasons = append(reasons, fmt.Sprintf("code fence %d was modified - code must be reproduced verbatim", i+1))
			}
		case Heading:
			if a[i].Level != b[i].Level {
				reasons = append(reasons, fmt.Sprintf("heading %d changed from level %d to %d", i+1, a[i].Level, b[i].Level))
			}
		case Table:
			ra, rb := strings.Count(a[i].Text, "\n"), strings.Count(b[i].Text, "\n")
			if ra != rb {
				reasons = append(reasons, fmt.Sprintf("table %d has %d rows, the source had %d", i+1, rb+1, ra+1))
			}
		}
	}
	if len(reasons) > 0 {
		return &StructureError{Reasons: reasons}
	}
	return nil
}
