package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

// openrouterCompleter is the OpenAI-compatible transport for OpenRouter — the
// single-key gateway that fronts Claude, GPT, Gemini and others, so one API key
// and one balance covers every bot. It mirrors anthropicCompleter's contract but
// speaks the OpenAI Chat Completions wire format: the cached tournament
// reference goes in a system message, the per-call task in a user message, and
// Structured Outputs is requested via response_format's strict json_schema
// (honored by the Tier-1 providers — Claude/GPT/Gemini).
//
// It's a hand-rolled HTTP client rather than a vendored SDK: OpenRouter is a
// single, stable endpoint, and this keeps the module lean (the same approach as
// client.go for the wm-pickems API).
//
// Not every provider honors strict json_schema. When the strict path fails, the
// completer degrades (via degrade()) to JSON mode with the schema moved into the
// prompt — the DeepSeek/Grok/Kimi fallback.
type openrouterCompleter struct {
	http     *http.Client
	apiKey   string
	model    string // OpenRouter model id, e.g. "openai/gpt-5.1", "google/gemini-2.5-pro"
	system   string // static reference, identical across all calls (cache prefix)
	log      *slog.Logger
	jsonOnly bool // sticky: this model rejected strict json_schema, so use JSON mode
}

const openrouterURL = "https://openrouter.ai/api/v1/chat/completions"

func newOpenRouterCompleter(model, system string, log *slog.Logger) (*openrouterCompleter, error) {
	key := os.Getenv("OPENROUTER_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("OPENROUTER_API_KEY is required")
	}
	return &openrouterCompleter{
		// No client-level timeout: each call's context (the run/batch deadline set
		// by the caller) bounds the request instead.
		http:   &http.Client{},
		apiKey: key,
		model:  model,
		system: system,
		log:    log,
	}, nil
}

func (o *openrouterCompleter) complete(ctx context.Context, label, task string, schema map[string]any) (string, error) {
	start := time.Now()
	b, err := json.Marshal(o.requestBody(task, schema))
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openrouterURL, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)
	req.Header.Set("X-Title", "wm-pickems") // OpenRouter dashboard attribution (optional)

	resp, err := o.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("openrouter %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}

	var out openRouterResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("decode openrouter response: %w; got %.200q", err, strings.TrimSpace(string(raw)))
	}
	// OpenRouter can return a 200 with an embedded error (e.g. upstream provider
	// failure) instead of a non-2xx status.
	if out.Error != nil {
		return "", fmt.Errorf("openrouter error: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 || out.Choices[0].Message.Content == "" {
		// finish_reason gives the error context: "length" = truncated by
		// max_tokens, "content_filter" = blocked, etc.
		var reason string
		if len(out.Choices) > 0 {
			reason = out.Choices[0].FinishReason
		}
		return "", fmt.Errorf("openrouter returned no content (finish_reason=%q); got %.200q",
			reason, strings.TrimSpace(string(raw)))
	}
	o.log.Info("ai_call",
		"task", label,
		"model", o.model,
		"in", out.Usage.PromptTokens,
		"out", out.Usage.CompletionTokens,
		"cache_read", out.Usage.PromptTokensDetails.CachedTokens,
		"dur_ms", time.Since(start).Milliseconds(),
	)
	return out.Choices[0].Message.Content, nil
}

// requestBody builds the chat-completion payload for the current structured-output
// mode. The strict path (default) sets response_format to a strict json_schema —
// the OpenAI-compatible equivalent of Anthropic's OutputConfig, honored cleanly by
// the Tier-1 providers (Claude/GPT/Gemini). The degraded path (after degrade())
// uses plain JSON mode and moves the schema into the prompt, so providers that
// don't support strict json_schema (DeepSeek/Grok/Kimi) can still produce the
// right shape.
func (o *openrouterCompleter) requestBody(task string, schema map[string]any) map[string]any {
	messages := []map[string]string{
		{"role": "system", "content": o.system},
		{"role": "user", "content": task},
	}
	var respFmt map[string]any
	if o.jsonOnly {
		// The literal lowercase "json" must appear in the prompt — OpenAI/DeepSeek
		// JSON mode rejects the request otherwise.
		messages[1]["content"] = task +
			"\n\nReturn ONLY a json object — no prose, no markdown, no code fences — matching this JSON Schema:\n" +
			schemaJSON(schema)
		respFmt = map[string]any{"type": "json_object"}
	} else {
		respFmt = map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "prediction",
				"strict": true,
				"schema": schema,
			},
		}
	}
	return map[string]any{
		"model":           o.model,
		"messages":        messages,
		"max_tokens":      32000,
		"response_format": respFmt,
	}
}

// degrade switches from strict json_schema to JSON mode (schema moved into the
// prompt) for the rest of the run, and reports whether it actually switched —
// false if already degraded, so the caller won't retry pointlessly. Sticky:
// once a model has shown it can't do strict schema, every later call skips
// straight to the looser mode instead of wasting a failed strict attempt first.
func (o *openrouterCompleter) degrade() bool {
	if o.jsonOnly {
		return false
	}
	o.jsonOnly = true
	o.log.Warn("structured-output fallback",
		"model", o.model,
		"mode", "json_object",
		"note", "strict json_schema failed; using JSON mode for the rest of this run",
	)
	return true
}

// schemaJSON renders a schema for embedding in the prompt (the degraded path).
func schemaJSON(schema map[string]any) string {
	b, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}

// openRouterResponse is the subset of the OpenAI-compatible chat-completion reply
// the bot needs: the first choice's message content, token usage for the ai_call
// log, and an embedded error (OpenRouter sometimes 200s with an error body).
type openRouterResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		PromptTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}
