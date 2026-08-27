package split

import (
	"strings"
	"testing"
)

const srcChunk = "## Install\n\nRun the command below.\n\n```bash\ngo install ./cmd/tool\n```\n\n| Flag | Meaning |\n|---|---|\n| -v | verbose |\n\n- first item\n- second item\n"

func TestVerifyStructure_GoodTranslation(t *testing.T) {
	// Prosa und Überschrift übersetzt, Code und Struktur unangetastet.
	got := "## Installation\n\nFühre den folgenden Befehl aus.\n\n```bash\ngo install ./cmd/tool\n```\n\n| Schalter | Bedeutung |\n|---|---|\n| -v | ausführlich |\n\n- erster Punkt\n- zweiter Punkt\n"
	if err := VerifyStructure(srcChunk, got); err != nil {
		t.Fatalf("saubere Übersetzung abgelehnt: %v", err)
	}
}

func TestVerifyStructure_TranslatedCode(t *testing.T) {
	// Der klassische Schaden: das Modell "übersetzt" den Code mit.
	got := "## Installation\n\nFühre den folgenden Befehl aus.\n\n```bash\ngo installiere ./befehl/werkzeug\n```\n\n| Flag | Meaning |\n|---|---|\n| -v | verbose |\n\n- first item\n- second item\n"
	err := VerifyStructure(srcChunk, got)
	if err == nil {
		t.Fatal("veränderter Code wurde nicht erkannt")
	}
	if !strings.Contains(err.Error(), "verbatim") {
		t.Errorf("unklare Meldung: %v", err)
	}
}

func TestVerifyStructure_DroppedBlock(t *testing.T) {
	got := "## Installation\n\nFühre den folgenden Befehl aus.\n\n```bash\ngo install ./cmd/tool\n```\n"
	err := VerifyStructure(srcChunk, got)
	if err == nil {
		t.Fatal("weggelassene Blöcke wurden nicht erkannt")
	}
	if !strings.Contains(err.Error(), "block count changed") {
		t.Errorf("unklare Meldung: %v", err)
	}
}

func TestVerifyStructure_HeadingLevelChanged(t *testing.T) {
	got := strings.Replace(srcChunk, "## Install", "# Installation", 1)
	err := VerifyStructure(srcChunk, got)
	if err == nil {
		t.Fatal("geänderte Überschriftenebene wurde nicht erkannt")
	}
	if !strings.Contains(err.Error(), "level 2 to 1") {
		t.Errorf("unklare Meldung: %v", err)
	}
}

func TestVerifyStructure_TableRowDropped(t *testing.T) {
	got := strings.Replace(srcChunk, "| -v | verbose |\n", "", 1)
	err := VerifyStructure(srcChunk, got)
	if err == nil {
		t.Fatal("verlorene Tabellenzeile wurde nicht erkannt")
	}
}

// TestVerifyStructure_ProseIsFree: der Prosatext darf sich frei ändern -
// sonst wäre die Prüfung für eine Übersetzung unbrauchbar.
func TestVerifyStructure_ProseIsFree(t *testing.T) {
	got := strings.Replace(srcChunk, "Run the command below.",
		"Ein völlig anders formulierter, deutlich längerer Satz an dieser Stelle.", 1)
	if err := VerifyStructure(srcChunk, got); err != nil {
		t.Fatalf("freie Prosa abgelehnt: %v", err)
	}
}
