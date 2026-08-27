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
	"time"
)

// DefaultURL is a local Ollama instance.
const DefaultURL = "http://localhost:11434/v1"

// Config holds the endpoint settings.
type Config struct {
	BaseURL string
	Model   string
	Token   string
	Timeout time.Duration
}

// ConfigFromEnv reads MDSPLIT_LLM_URL / _MODEL / _TOKEN / _TIMEOUT. Flags
// override whatever this returns.
func ConfigFromEnv() Config {
	c := Config{
		BaseURL: os.Getenv("MDSPLIT_LLM_URL"),
		Model:   os.Getenv("MDSPLIT_LLM_MODEL"),
		Token:   os.Getenv("MDSPLIT_LLM_TOKEN"),
		Timeout: 5 * time.Minute,
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
	return fmt.Sprintf("%s model=%s (%s)", c.url(), c.Model, auth)
}

func (c Config) url() string {
	u := strings.TrimRight(c.BaseURL, "/")
	if u == "" {
		u = DefaultURL
	}
	return u
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

type chatResponse struct {
	Choices []struct {
		Message      chatMessage `json:"message"`
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
	if !c.cfg.Ready() {
		return "", fmt.Errorf("no model configured - set MDSPLIT_LLM_MODEL or pass -llm-model")
	}
	body, err := json.Marshal(chatRequest{
		Model: c.cfg.Model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: 0,
		Stream:      false,
	})
	if err != nil {
		return "", fmt.Errorf("encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.url()+"/chat/completions", bytes.NewReader(body))
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

	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("endpoint returned %s: %s", resp.Status, snippet(data))
	}

	var out chatResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("decode response: %w (body: %s)", err, snippet(data))
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
	if strings.TrimSpace(ch.Message.Content) == "" {
		return "", fmt.Errorf("endpoint returned an empty message")
	}
	return ch.Message.Content, nil
}

// snippet trims a body for an error message.
func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}
