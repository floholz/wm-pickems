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
type openrouterCompleter struct {
	http   *http.Client
	apiKey string
	model  string // OpenRouter model id, e.g. "openai/gpt-5.1", "google/gemini-2.5-pro"
	system string // static reference, identical across all calls (cache prefix)
	log    *slog.Logger
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
	body := map[string]any{
		"model": o.model,
		"messages": []map[string]string{
			{"role": "system", "content": o.system},
			{"role": "user", "content": task},
		},
		"max_tokens": 32000,
		// Strict json_schema is the OpenAI-compatible equivalent of Anthropic's
		// OutputConfig: the Tier-1 providers return JSON matching the schema with
		// no fences/preamble. (DeepSeek/Kimi support is spottier — that's the
		// later JSON-mode + retry fallback, not this path.)
		"response_format": map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "prediction",
				"strict": true,
				"schema": schema,
			},
		},
	}
	b, err := json.Marshal(body)
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
