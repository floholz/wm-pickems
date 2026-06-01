package main

import (
	"context"
	"errors"
	"testing"
)

func TestExtractJSON(t *testing.T) {
	cases := map[string]string{
		`{"a":1}`:                      `{"a":1}`, // clean (strict-schema path)
		"  {\"a\":1}  ":                `{"a":1}`, // surrounding whitespace
		"```json\n{\"a\":1}\n```":      `{"a":1}`, // json code fence
		"```\n{\"a\":1}\n```":          `{"a":1}`, // bare code fence
		"Here you go:\n{\"a\":1}\nok!": `{"a":1}`, // prose around the object
		"```JSON {\"a\":1} ```":        `{"a":1}`, // uppercase fence tag
	}
	for in, want := range cases {
		if got := extractJSON(in); got != want {
			t.Errorf("extractJSON(%q) = %q, want %q", in, got, want)
		}
	}
}

// scriptCompleter returns a scripted sequence of replies, one per call (the last
// reply repeats if called again). It does NOT implement fallbackCompleter.
type scriptCompleter struct {
	replies []reply
	calls   int
}

type reply struct {
	text string
	err  error
}

func (s *scriptCompleter) complete(_ context.Context, _, _ string, _ map[string]any) (string, error) {
	i := s.calls
	s.calls++
	if i >= len(s.replies) {
		i = len(s.replies) - 1
	}
	return s.replies[i].text, s.replies[i].err
}

// scriptFallback is a scriptCompleter that also implements fallbackCompleter.
type scriptFallback struct {
	scriptCompleter
	degraded bool
}

func (s *scriptFallback) degrade() bool {
	if s.degraded {
		return false
	}
	s.degraded = true
	return true
}

type intOut struct {
	A int `json:"a"`
}

// A transport that degrades recovers when the first attempt is unparseable.
func TestCompleteStructuredFallbackOnBadJSON(t *testing.T) {
	comp := &scriptFallback{scriptCompleter: scriptCompleter{replies: []reply{
		{text: "definitely not json"},
		{text: `{"a":7}`},
	}}}
	b := &Brain{comp: comp}
	var o intOut
	if err := b.completeStructured(context.Background(), "t", "task", map[string]any{}, &o); err != nil {
		t.Fatalf("expected fallback success, got %v", err)
	}
	if o.A != 7 {
		t.Errorf("got a=%d, want 7", o.A)
	}
	if !comp.degraded {
		t.Error("expected degrade() to have been called")
	}
	if comp.calls != 2 {
		t.Errorf("expected 2 calls (strict + fallback), got %d", comp.calls)
	}
}

// It also recovers when the first attempt is an HTTP error (schema unsupported).
func TestCompleteStructuredFallbackOnError(t *testing.T) {
	comp := &scriptFallback{scriptCompleter: scriptCompleter{replies: []reply{
		{err: errors.New("openrouter 400: response_format json_schema not supported")},
		{text: `{"a":3}`},
	}}}
	b := &Brain{comp: comp}
	var o intOut
	if err := b.completeStructured(context.Background(), "t", "task", map[string]any{}, &o); err != nil {
		t.Fatalf("expected fallback success, got %v", err)
	}
	if o.A != 3 {
		t.Errorf("got a=%d, want 3", o.A)
	}
}

// A transport without the fallback capability surfaces the error and does NOT
// retry (the native Anthropic path: one strict attempt, no degrade).
func TestCompleteStructuredNoFallback(t *testing.T) {
	comp := &scriptCompleter{replies: []reply{{text: "not json"}}}
	b := &Brain{comp: comp}
	var o intOut
	if err := b.completeStructured(context.Background(), "t", "task", map[string]any{}, &o); err == nil {
		t.Fatal("expected error for unparseable reply with no fallback")
	}
	if comp.calls != 1 {
		t.Errorf("expected 1 call (no retry), got %d", comp.calls)
	}
}
