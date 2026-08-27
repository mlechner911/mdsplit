// Package llm talks to an OpenAI-compatible chat endpoint. Ollama serves that
// schema under /v1, so one code path covers both a local Ollama box and the
// OpenAI API - only the base URL and the token differ.
//
// The endpoint is configured when the server starts, never per request. A tool
// argument would put the token into the conversation transcript, and a
// caller-chosen URL would turn a tool that reads local files into an
// exfiltration channel the moment a translated document carries an injected
// instruction.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/template"
	"time"
)

// DefaultURL is a local Ollama instance.
const DefaultURL = "http://localhost:11434/v1"

// Transport selects which endpoint carries a request.
type Transport string

const (
	// TransportChat uses /chat/completions with a system and a user message.
	TransportChat Transport = "chat"
	// TransportCompletions uses /completions with a prompt we render
	// ourselves, bypassing the server-side chat template.
	//
	// Some models ship a chat template that an OpenAI-compatible layer cannot
	// satisfy. TranslateGemma is the case in point: its template rejects a
	// system role outright ("Conversations must start with a user prompt") and
	// demands the user content be a mapping with source_lang_code and
	// target_lang_code - fields the OpenAI schema strips before the template
	// ever sees them. Rendering the turn ourselves sidesteps all of it.
	TransportCompletions Transport = "completions"
)

// DefaultPromptTemplate renders a Gemma-style turn, which is what the
// completions transport needs for the Gemma and TranslateGemma families.
// Available fields: .System, .User, .SourceLang, .TargetLang.
const DefaultPromptTemplate = "<start_of_turn>user\n{{.System}}\n\n{{.User}}<end_of_turn>\n<start_of_turn>model\n"

// TranslateGemmaTemplate reproduces the instruction TranslateGemma's own chat
// template builds, for use with the completions transport. One template covers
// every language pair.
const TranslateGemmaTemplate = "<start_of_turn>user\n" +
	"You are a professional {{.SourceLangName}} ({{.SourceLang}}) to {{.TargetLangName}} ({{.TargetLang}}) " +
	"translator. Your goal is to accurately convey the meaning and nuances of the original " +
	"{{.SourceLangName}} text while adhering to {{.TargetLangName}} grammar, vocabulary, and cultural " +
	"sensitivities.\nProduce only the {{.TargetLangName}} translation, without any additional " +
	"explanations or commentary. Please translate the following {{.SourceLangName}} text into " +
	"{{.TargetLangName}}:\n\n\n{{.User}}<end_of_turn>\n<start_of_turn>model\n"

// DefaultStop ends generation at the end of the model turn.
var DefaultStop = []string{"<end_of_turn>"}

// Config holds the endpoint settings.
type Config struct {
	BaseURL   string
	Model     string
	Token     string
	Timeout   time.Duration
	Transport Transport
	// PromptTemplate is a Go text/template used by the completions transport.
	PromptTemplate string
	// Stop ends generation. Empty means DefaultStop for completions.
	Stop []string
}

// PromptData is what a prompt template can reference. Codes and names are both
// offered because models want different halves: TranslateGemma's own template
// writes "professional English (en) to German (de) translator", so a template
// that only had one of them could not reproduce it.
type PromptData struct {
	System         string
	User           string
	SourceLang     string // "en"
	TargetLang     string // "de"
	SourceLangName string // "English"
	TargetLangName string // "German"
}

// ConfigFromEnv reads MDSPLIT_LLM_URL / _MODEL / _TOKEN / _TIMEOUT. Flags
// override whatever this returns.
func ConfigFromEnv() Config {
	c := Config{
		BaseURL:        os.Getenv("MDSPLIT_LLM_URL"),
		Model:          os.Getenv("MDSPLIT_LLM_MODEL"),
		Token:          os.Getenv("MDSPLIT_LLM_TOKEN"),
		Timeout:        5 * time.Minute,
		Transport:      Transport(os.Getenv("MDSPLIT_LLM_TRANSPORT")),
		PromptTemplate: os.Getenv("MDSPLIT_LLM_TEMPLATE"),
	}
	if v := os.Getenv("MDSPLIT_LLM_STOP"); v != "" {
		c.Stop = strings.Split(v, ",")
	}
	if v := os.Getenv("MDSPLIT_LLM_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.Timeout = d
		}
	}
	return c
}

// Ready reports whether the config names an endpoint and a model.
func (c Config) Ready() bool {
	return strings.TrimSpace(c.Model) != ""
}

// Describe renders the configuration for a status message. The token is never
// printed, only whether one is set.
func (c Config) Describe() string {
	auth := "no token"
	if c.Token != "" {
		auth = "token set"
	}
	t := c.transport()
	return fmt.Sprintf("%s model=%s transport=%s (%s)", c.url(), c.Model, t, auth)
}

// transport falls back to the chat endpoint.
func (c Config) transport() Transport {
	if c.Transport == TransportCompletions {
		return TransportCompletions
	}
	return TransportChat
}

func (c Config) url() string {
	u := strings.TrimRight(c.BaseURL, "/")
	if u == "" {
		u = DefaultURL
	}
	return u
}

// ResolveTemplate expands a named shorthand, so a caller does not have to
// paste a multi-line template into an env var to use a known model family.
func ResolveTemplate(name string) string {
	switch strings.TrimSpace(name) {
	case "":
		return DefaultPromptTemplate
	case "translategemma":
		return TranslateGemmaTemplate
	default:
		return name
	}
}

// Client sends chat completions.
type Client struct {
	cfg  Config
	http *http.Client
}

// New builds a client. A zero Timeout falls back to five minutes, which is
// generous enough for a cold model load on a local box.
func New(cfg Config) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Minute
	}
	return &Client{cfg: cfg, http: &http.Client{Timeout: cfg.Timeout}}
}

// Config returns the settings in use.
func (c *Client) Config() Config { return c.cfg }

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	Stream      bool          `json:"stream"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type completionRequest struct {
	Model       string   `json:"model"`
	Prompt      string   `json:"prompt"`
	Temperature float64  `json:"temperature"`
	Stream      bool     `json:"stream"`
	Stop        []string `json:"stop,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message      chatMessage `json:"message"`
		Text         string      `json:"text"` // completions transport
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// ErrTruncated means the model stopped before it was done - a length cap or an
// interrupted stream. Storing such a reply would silently lose text, which is
// the exact failure this whole pipeline exists to prevent.
type ErrTruncated struct{ Reason string }

func (e *ErrTruncated) Error() string {
	return fmt.Sprintf("model stopped early (finish_reason %q) - the reply is incomplete", e.Reason)
}

// Chat sends one isolated request: system prompt plus user text, no history.
// That isolation is the point - it is what keeps the context flat no matter
// how many chunks a document has.
func (c *Client) Chat(ctx context.Context, system, user string) (string, error) {
	return c.Ask(ctx, PromptData{System: system, User: user})
}

// Ask sends one isolated request over the configured transport.
func (c *Client) Ask(ctx context.Context, data PromptData) (string, error) {
	if !c.cfg.Ready() {
		return "", fmt.Errorf("no model configured - set MDSPLIT_LLM_MODEL or pass -llm-model")
	}
	path, body, err := c.buildRequest(data)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.url()+path, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("call %s: %w", c.cfg.url(), err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("endpoint returned %s: %s", resp.Status, snippet(raw))
	}

	var out chatResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("decode response: %w (body: %s)", err, snippet(raw))
	}
	if out.Error != nil {
		return "", fmt.Errorf("endpoint error: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("endpoint returned no choices")
	}
	ch := out.Choices[0]
	if r := ch.FinishReason; r != "" && r != "stop" {
		return "", &ErrTruncated{Reason: r}
	}
	content := ch.Message.Content
	if content == "" {
		content = ch.Text // completions transport
	}
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("endpoint returned an empty message")
	}
	return content, nil
}

// buildRequest renders the payload for the configured transport.
func (c *Client) buildRequest(data PromptData) (string, []byte, error) {
	if c.cfg.transport() == TransportCompletions {
		tmplText := ResolveTemplate(c.cfg.PromptTemplate)
		tmpl, err := template.New("prompt").Parse(tmplText)
		if err != nil {
			return "", nil, fmt.Errorf("parse prompt template: %w", err)
		}
		var sb strings.Builder
		if err := tmpl.Execute(&sb, data); err != nil {
			return "", nil, fmt.Errorf("render prompt template: %w", err)
		}
		stop := c.cfg.Stop
		if len(stop) == 0 {
			stop = DefaultStop
		}
		body, err := json.Marshal(completionRequest{
			Model: c.cfg.Model, Prompt: sb.String(),
			Temperature: 0, Stream: false, Stop: stop,
		})
		if err != nil {
			return "", nil, fmt.Errorf("encode request: %w", err)
		}
		return "/completions", body, nil
	}

	body, err := json.Marshal(chatRequest{
		Model: c.cfg.Model,
		Messages: []chatMessage{
			{Role: "system", Content: data.System},
			{Role: "user", Content: data.User},
		},
		Temperature: 0,
		Stream:      false,
	})
	if err != nil {
		return "", nil, fmt.Errorf("encode request: %w", err)
	}
	return "/chat/completions", body, nil
}

// snippet trims a body for an error message.
func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}
