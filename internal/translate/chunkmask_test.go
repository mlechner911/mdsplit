package translate

import (
	"strings"
	"testing"
)

// TestMaskChunk_RoundTrip: maskieren und zurückersetzen muss den Chunk exakt
// ergeben - sonst wäre der Chunk-Modus nicht mehr verlustfrei.
func TestMaskChunk_RoundTrip(t *testing.T) {
	masked, tokens := maskChunk(chunk)
	back, err := restore(masked, tokens)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if want := strings.TrimRight(chunk, "\n"); back != want {
		t.Fatalf("Roundtrip:\nWANT %q\nGOT  %q", want, back)
	}
}

// TestMaskChunk_CodeIsGone: der Prompt darf keinen Code mehr enthalten.
func TestMaskChunk_CodeIsGone(t *testing.T) {
	masked, _ := maskChunk(chunk)
	for _, forbidden := range []string{"```", "go install github.com", "<div", "Some boxed text", "$GOBIN", "https://example.com"} {
		if strings.Contains(masked, forbidden) {
			t.Errorf("Prompt enthält noch %q:\n%s", forbidden, masked)
		}
	}
}

// TestMaskChunk_ProseStaysConnected: die Prosa muss zusammenhängend bleiben -
// das ist der ganze Vorteil des Chunk-Modus gegenüber der Block-Ebene.
func TestMaskChunk_ProseStaysConnected(t *testing.T) {
	masked, _ := maskChunk(chunk)
	for _, required := range []string{"Install the tool", "Run ", " and the binary lands in ", "verbose output", "| Flag | Meaning |"} {
		if !strings.Contains(masked, required) {
			t.Errorf("Prosa %q fehlt im Prompt:\n%s", required, masked)
		}
	}
}

// TestMaskChunk_FenceInsideList: ein Zaun im Listenpunkt wird ebenfalls maskiert.
func TestMaskChunk_FenceInsideList(t *testing.T) {
	src := "- Step one:\n\n  ```go\n  func main() {}\n  ```\n\n- Step two"
	masked, tokens := maskChunk(src)
	if strings.Contains(masked, "func main") {
		t.Errorf("Zaun im Listenpunkt steht noch im Prompt: %q", masked)
	}
	if !strings.Contains(masked, "Step one") || !strings.Contains(masked, "Step two") {
		t.Errorf("Listentext ging verloren: %q", masked)
	}
	back, err := restore(masked, tokens)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if back != src {
		t.Errorf("Roundtrip:\nWANT %q\nGOT  %q", src, back)
	}
}

// TestMaskChunk_Savings belegt die Ersparnis, die den Chunk-Modus bei kleinem
// Kontextfenster überhaupt erst gangbar macht.
func TestMaskChunk_Savings(t *testing.T) {
	masked, tokens := maskChunk(chunk)
	if len(tokens) < 4 {
		t.Fatalf("erwartet mindestens 4 maskierte Spans, bekommen %d", len(tokens))
	}
	saved := 100 - 100*len(masked)/len(chunk)
	if saved < 20 {
		t.Errorf("nur %d%% eingespart (%d -> %d Zeichen) - erwartet mindestens 20%%", saved, len(chunk), len(masked))
	}
	t.Logf("%d -> %d Zeichen (%d%% weniger), %d Spans maskiert", len(chunk), len(masked), saved, len(tokens))
}
