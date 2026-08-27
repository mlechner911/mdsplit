package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// capture nimmt eine Anfrage entgegen und antwortet mit reply.
type capture struct {
	path string
	auth string
	body map[string]any
}

func fakeEndpoint(t *testing.T, status int, reply string, got *capture) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.path = r.URL.Path
		got.auth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got.body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		io.WriteString(w, reply)
	}))
}

const okChat = `{"choices":[{"message":{"role":"assistant","content":"Hallo Welt"},"finish_reason":"stop"}]}`

func TestChat_Transport(t *testing.T) {
	var got capture
	srv := fakeEndpoint(t, 200, okChat, &got)
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Model: "m", Token: "secret"})
	out, err := c.Chat(context.Background(), "be brief", "Hello world")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if out != "Hallo Welt" {
		t.Errorf("out = %q", out)
	}
	if got.path != "/chat/completions" {
		t.Errorf("path = %q", got.path)
	}
	if got.auth != "Bearer secret" {
		t.Errorf("auth = %q", got.auth)
	}
	msgs, _ := got.body["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("erwartet 2 Nachrichten, bekommen %d", len(msgs))
	}
	first, _ := msgs[0].(map[string]any)
	if first["role"] != "system" || first["content"] != "be brief" {
		t.Errorf("erste Nachricht = %v", first)
	}
	if got.body["temperature"] != float64(0) {
		t.Errorf("temperature = %v, erwartet 0 - Übersetzen ist keine Kreativaufgabe", got.body["temperature"])
	}
}

// TestChat_NoTokenNoHeader: ohne Token darf kein Authorization-Header raus.
func TestChat_NoTokenNoHeader(t *testing.T) {
	var got capture
	srv := fakeEndpoint(t, 200, okChat, &got)
	defer srv.Close()
	if _, err := New(Config{BaseURL: srv.URL, Model: "m"}).Chat(context.Background(), "s", "u"); err != nil {
		t.Fatal(err)
	}
	if got.auth != "" {
		t.Errorf("Authorization gesetzt ohne Token: %q", got.auth)
	}
}

// TestCompletions_Transport deckt den Weg ab, den TranslateGemma braucht: die
// Vorlage wird hier gerendert, nicht auf dem Server.
func TestCompletions_Transport(t *testing.T) {
	var got capture
	srv := fakeEndpoint(t, 200, `{"choices":[{"text":"Hallo","finish_reason":"stop"}]}`, &got)
	defer srv.Close()

	c := New(Config{
		BaseURL:        srv.URL,
		Model:          "translategemma",
		Transport:      TransportCompletions,
		PromptTemplate: "[{{.SourceLang}}->{{.TargetLang}}] {{.System}} :: {{.User}}",
		Stop:           []string{"<end>"},
	})
	out, err := c.Ask(context.Background(), PromptData{
		System: "rules", User: "Hello", SourceLang: "en", TargetLang: "de",
	})
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if out != "Hallo" {
		t.Errorf("out = %q", out)
	}
	if got.path != "/completions" {
		t.Errorf("path = %q, erwartet /completions", got.path)
	}
	if _, hasMessages := got.body["messages"]; hasMessages {
		t.Error("completions-Transport darf keine messages senden")
	}
	if p := got.body["prompt"]; p != "[en->de] rules :: Hello" {
		t.Errorf("prompt = %q", p)
	}
	stop, _ := got.body["stop"].([]any)
	if len(stop) != 1 || stop[0] != "<end>" {
		t.Errorf("stop = %v", got.body["stop"])
	}
}

func TestCompletions_DefaultTemplate(t *testing.T) {
	var got capture
	srv := fakeEndpoint(t, 200, `{"choices":[{"text":"x","finish_reason":"stop"}]}`, &got)
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Model: "m", Transport: TransportCompletions})
	if _, err := c.Chat(context.Background(), "rules", "text"); err != nil {
		t.Fatal(err)
	}
	p, _ := got.body["prompt"].(string)
	for _, want := range []string{"<start_of_turn>user", "rules", "text", "<end_of_turn>", "<start_of_turn>model"} {
		if !strings.Contains(p, want) {
			t.Errorf("Standardvorlage ohne %q: %q", want, p)
		}
	}
	stop, _ := got.body["stop"].([]any)
	if len(stop) != 1 || stop[0] != "<end_of_turn>" {
		t.Errorf("stop = %v", got.body["stop"])
	}
}

// TestChat_TruncatedIsAnError ist der Kernvertrag des Clients: eine
// abgeschnittene Antwort ist kein Ergebnis. Sie zu speichern hieße, still Text
// zu verlieren - genau das, was diese Pipeline verhindern soll.
func TestChat_TruncatedIsAnError(t *testing.T) {
	var got capture
	srv := fakeEndpoint(t, 200, `{"choices":[{"message":{"content":"halb"},"finish_reason":"length"}]}`, &got)
	defer srv.Close()

	_, err := New(Config{BaseURL: srv.URL, Model: "m"}).Chat(context.Background(), "s", "u")
	if err == nil {
		t.Fatal("abgeschnittene Antwort wurde akzeptiert")
	}
	var te *ErrTruncated
	if !asTruncated(err, &te) {
		t.Fatalf("erwartet *ErrTruncated, bekommen %T: %v", err, err)
	}
	if te.Reason != "length" {
		t.Errorf("Reason = %q", te.Reason)
	}
}

func asTruncated(err error, target **ErrTruncated) bool {
	if e, ok := err.(*ErrTruncated); ok {
		*target = e
		return true
	}
	return false
}

func TestChat_Failures(t *testing.T) {
	cases := map[string]struct {
		status int
		reply  string
		want   string
	}{
		"HTTP-Fehler":   {500, `{"error":{"message":"boom"}}`, "500"},
		"leere Antwort": {200, `{"choices":[{"message":{"content":"  "},"finish_reason":"stop"}]}`, "empty"},
		"keine choices": {200, `{"choices":[]}`, "no choices"},
		"Fehlerfeld":    {200, `{"error":{"message":"model not found"}}`, "model not found"},
		"kaputtes JSON": {200, `nicht json`, "decode"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			var got capture
			srv := fakeEndpoint(t, c.status, c.reply, &got)
			defer srv.Close()
			_, err := New(Config{BaseURL: srv.URL, Model: "m"}).Chat(context.Background(), "s", "u")
			if err == nil {
				t.Fatal("Fehler erwartet")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("Meldung %q enthält nicht %q", err.Error(), c.want)
			}
		})
	}
}

func TestConfig_NotReadyWithoutModel(t *testing.T) {
	if (Config{BaseURL: "http://x"}).Ready() {
		t.Error("Config ohne Modell gilt als bereit")
	}
	if _, err := New(Config{BaseURL: "http://x"}).Chat(context.Background(), "s", "u"); err == nil {
		t.Error("Aufruf ohne Modell hätte fehlschlagen müssen")
	}
}

// TestDescribe_HidesToken: die Statuszeile geht in Logs und in MCP-Ausgaben.
func TestDescribe_HidesToken(t *testing.T) {
	d := Config{BaseURL: "http://x/v1", Model: "m", Token: "sk-geheim"}.Describe()
	if strings.Contains(d, "sk-geheim") {
		t.Fatalf("Token steht in der Statuszeile: %q", d)
	}
	if !strings.Contains(d, "token set") {
		t.Errorf("Statuszeile verschweigt, dass ein Token gesetzt ist: %q", d)
	}
}

// TestResolveTemplate_Shorthand: eine mehrzeilige Vorlage in eine Env-Variable
// zu kleben ist fehleranfällig; für bekannte Modellfamilien reicht ein Name.
func TestResolveTemplate_Shorthand(t *testing.T) {
	if got := ResolveTemplate(""); got != DefaultPromptTemplate {
		t.Error("leerer Name liefert nicht die Standardvorlage")
	}
	if got := ResolveTemplate("translategemma"); got != TranslateGemmaTemplate {
		t.Error("Kurzform translategemma nicht aufgelöst")
	}
	if got := ResolveTemplate("custom {{.User}}"); got != "custom {{.User}}" {
		t.Error("eigene Vorlage wurde verändert")
	}
}

// TestTranslateGemmaTemplate_RendersLanguagePair: eine Vorlage für alle
// Sprachpaare - sonst bräuchte jede Zielsprache ihre eigene Env-Variable.
func TestTranslateGemmaTemplate_RendersLanguagePair(t *testing.T) {
	var got capture
	srv := fakeEndpoint(t, 200, `{"choices":[{"text":"x","finish_reason":"stop"}]}`, &got)
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Model: "tg", Transport: TransportCompletions,
		PromptTemplate: "translategemma"})
	if _, err := c.Ask(context.Background(), PromptData{
		User: "Hello", SourceLang: "en", TargetLang: "zh",
		SourceLangName: "English", TargetLangName: "Chinese",
	}); err != nil {
		t.Fatal(err)
	}
	p, _ := got.body["prompt"].(string)
	for _, want := range []string{"English (en) to Chinese (zh) translator", "into Chinese:", "Hello", "<start_of_turn>model"} {
		if !strings.Contains(p, want) {
			t.Errorf("Prompt ohne %q:\n%s", want, p)
		}
	}
}
