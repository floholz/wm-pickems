package main

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// anthropicCompleter is the native Anthropic transport: prompt caching on the
// system prefix, adaptive thinking, and Structured Outputs via OutputConfig.
// It's the original, best-ergonomics path — kept as the reference even once the
// live bots move to OpenRouter, so the cleanest Claude integration stays around.
type anthropicCompleter struct {
	client anthropic.Client
	model  string
	system string // static reference, identical across all calls (cache prefix)
	log    *slog.Logger
}

func newAnthropicCompleter(model, system string, log *slog.Logger) *anthropicCompleter {
	return &anthropicCompleter{
		client: anthropic.NewClient(), // reads ANTHROPIC_API_KEY
		model:  model,
		system: system,
		log:    log,
	}
}

// adaptiveThinking returns the adaptive-thinking config for models that support
// it (Opus 4.6+/Sonnet 4.6). Haiku 4.5 has no adaptive thinking — the API 400s —
// so it runs with thinking omitted (fine, and cheaper/faster for dev runs).
func adaptiveThinking(model string) anthropic.ThinkingConfigParamUnion {
	if strings.Contains(strings.ToLower(model), "haiku") {
		return anthropic.ThinkingConfigParamUnion{} // omitted
	}
	return anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}}
}

// complete runs one streamed request with adaptive thinking, a cached system
// prompt, and a Structured Outputs JSON-schema constraint, returning the
// concatenated final text. Streaming avoids HTTP timeouts on larger outputs and
// lets thinking run without a fixed budget. The schema constrains the reply so
// the text is guaranteed valid JSON matching it. The schema sits in OutputConfig
// (not the cached system prefix), so caching is unaffected.
func (a *anthropicCompleter) complete(ctx context.Context, label, task string, schema map[string]any) (string, error) {
	start := time.Now()
	stream := a.client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(a.model),
		MaxTokens: 32000,
		Thinking:  adaptiveThinking(a.model),
		System: []anthropic.TextBlockParam{{
			Text:         a.system,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},
		OutputConfig: anthropic.OutputConfigParam{
			Format: anthropic.JSONOutputFormatParam{Schema: schema},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(task)),
		},
	})
	msg := anthropic.Message{}
	for stream.Next() {
		msg.Accumulate(stream.Current())
	}
	if err := stream.Err(); err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, block := range msg.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			sb.WriteString(t.Text)
		}
	}
	a.log.Info("ai_call",
		"task", label,
		"model", a.model,
		"in", msg.Usage.InputTokens,
		"out", msg.Usage.OutputTokens,
		"cache_read", msg.Usage.CacheReadInputTokens,
		"cache_create", msg.Usage.CacheCreationInputTokens,
		"dur_ms", time.Since(start).Milliseconds(),
	)
	return sb.String(), nil
}
