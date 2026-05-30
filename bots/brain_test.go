package main

import (
	"encoding/json"
	"testing"
)

func TestExtractJSON(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"plain", `{"a":1}`, `{"a":1}`},
		{"preamble", `Looking at the form, here is my answer: {"a":1}`, `{"a":1}`},
		{"trailing prose", `{"a":1} — hope that helps!`, `{"a":1}`},
		{"code fence", "```json\n{\"a\":1}\n```", `{"a":1}`},
		{"brace in string", `note {"name":"A}B","x":2} end`, `{"name":"A}B","x":2}`},
		{"nested", `pre {"o":{"b":1},"c":2} post`, `{"o":{"b":1},"c":2}`},
		{"none", `no json here`, ``},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractJSON(c.in)
			if got != c.want {
				t.Fatalf("extractJSON(%q) = %q, want %q", c.in, got, c.want)
			}
			if got != "" { // result must be valid JSON
				var v any
				if err := json.Unmarshal([]byte(got), &v); err != nil {
					t.Fatalf("extracted %q is not valid JSON: %v", got, err)
				}
			}
		})
	}
}
