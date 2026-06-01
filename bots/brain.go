package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// Brain wraps the Anthropic client. The large, never-changing tournament
// reference (teams, group memberships, knockout skeleton) lives in a single
// cached system prompt so every prediction call reuses it as a prompt-cache
// prefix — only the per-call task in the user turn varies.
type Brain struct {
	client    anthropic.Client
	model     string
	system    string // static reference, identical across all calls (cache prefix)
	results   string // results-so-far summary, fed into tip prompts (the feedback loop)
	rationale bool   // ask for + log a one-line reason per prediction
	log       *slog.Logger
}

func NewBrain(model, reference, results string, rationale bool, log *slog.Logger) *Brain {
	return &Brain{
		client:    anthropic.NewClient(), // reads ANTHROPIC_API_KEY
		model:     model,
		system:    reference,
		results:   results,
		rationale: rationale,
		log:       log,
	}
}

// complete runs one streamed request with adaptive thinking, a cached system
// prompt, and a Structured Outputs JSON-schema constraint, returning the
// concatenated final text. Streaming avoids HTTP timeouts on larger outputs and
// lets thinking run without a fixed budget. The schema constrains the reply so
// the text is guaranteed valid JSON matching it (no fences/preamble to strip).
// The schema sits in OutputConfig (not the cached system prefix), so caching is
// unaffected.
// adaptiveThinking returns the adaptive-thinking config for models that support
// it (Opus 4.6+/Sonnet 4.6). Haiku 4.5 has no adaptive thinking — the API 400s —
// so it runs with thinking omitted (fine, and cheaper/faster for dev runs).
func adaptiveThinking(model string) anthropic.ThinkingConfigParamUnion {
	if strings.Contains(strings.ToLower(model), "haiku") {
		return anthropic.ThinkingConfigParamUnion{} // omitted
	}
	return anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}}
}

func (b *Brain) complete(ctx context.Context, label, task string, schema map[string]any) (string, error) {
	start := time.Now()
	stream := b.client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(b.model),
		MaxTokens: 32000,
		Thinking:  adaptiveThinking(b.model),
		System: []anthropic.TextBlockParam{{
			Text:         b.system,
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
	b.log.Info("ai_call",
		"task", label,
		"model", b.model,
		"in", msg.Usage.InputTokens,
		"out", msg.Usage.OutputTokens,
		"cache_read", msg.Usage.CacheReadInputTokens,
		"cache_create", msg.Usage.CacheCreationInputTokens,
		"dur_ms", time.Since(start).Milliseconds(),
	)
	return sb.String(), nil
}

// completeStructured runs complete() with a JSON-schema constraint and unmarshals
// the (schema-guaranteed valid) reply into out.
func (b *Brain) completeStructured(ctx context.Context, label, task string, schema map[string]any, out any) error {
	raw, err := b.complete(ctx, label, task, schema)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		return fmt.Errorf("structured output for %s not valid JSON: %w; got %.200q", label, err, strings.TrimSpace(raw))
	}
	return nil
}

// ---- JSON schema helpers (Structured Outputs) ----
//
// Every object node must set additionalProperties:false, and dynamic-keyed maps
// aren't expressible — hence the array-of-records response shapes below.

func strSchema() map[string]any { return map[string]any{"type": "string"} }
func intSchema() map[string]any { return map[string]any{"type": "integer"} }
func numSchema() map[string]any { return map[string]any{"type": "number"} }

func arr(items map[string]any) map[string]any { return map[string]any{"type": "array", "items": items} }

func obj(required []string, props map[string]any) map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             required,
		"properties":           props,
	}
}

// withRationale adds a required "rationale" string to a record's schema when the
// rationale feature is on.
func withRationale(on bool, required []string, props map[string]any) ([]string, map[string]any) {
	if on {
		required = append(required, "rationale")
		props["rationale"] = strSchema()
	}
	return required, props
}

func groupsSchema(rationale bool) map[string]any {
	req, props := withRationale(rationale, []string{"letter", "teamIds"}, map[string]any{
		"letter":  strSchema(),
		"teamIds": arr(strSchema()),
	})
	return obj([]string{"groups", "bestThirds"}, map[string]any{
		"groups":     arr(obj(req, props)),
		"bestThirds": arr(strSchema()),
	})
}

func winnersSchema(rationale bool) map[string]any {
	req, props := withRationale(rationale, []string{"matchNum", "side"}, map[string]any{
		"matchNum": intSchema(),
		"side":     map[string]any{"type": "string", "enum": []string{"home", "away"}},
	})
	return obj([]string{"winners"}, map[string]any{"winners": arr(obj(req, props))})
}

func tipsSchema(rationale bool) map[string]any {
	req, props := withRationale(rationale, []string{"key", "scores"}, map[string]any{
		"key": strSchema(),
		"scores": arr(obj([]string{"home", "away", "p"}, map[string]any{
			"home": intSchema(),
			"away": intSchema(),
			"p":    numSchema(),
		})),
	})
	return obj([]string{"tips"}, map[string]any{"tips": arr(obj(req, props))})
}

// ---- forecast: group standings + best thirds ----

type groupPick struct {
	Letter string
	Teams  []nameID // the four teams, in group-membership order
}
type nameID struct {
	ID   string
	Name string
}

// PredictGroups asks Claude to rank each group 1st..4th and choose which 8
// groups' third-placed team it expects to advance. Returns ordered team ids per
// group and the 8 chosen group letters. Output is validated against the known
// membership; anything off is repaired by the caller.
func (b *Brain) PredictGroups(ctx context.Context, groups []groupPick) (map[string][]string, []string, error) {
	var sb strings.Builder
	sb.WriteString("Predict the FINAL group stage standings for the 2026 World Cup.\n\n")
	sb.WriteString("For EACH group, order all four teams from 1st to 4th place. ")
	sb.WriteString("Then choose exactly EIGHT groups whose 3rd-placed team you expect to be among the eight best thirds that advance to the Round of 32.\n\n")
	for _, g := range groups {
		sb.WriteString("Group " + g.Letter + ": ")
		parts := make([]string, len(g.Teams))
		for i, t := range g.Teams {
			parts[i] = fmt.Sprintf("%s (id=%s)", t.Name, t.ID)
		}
		sb.WriteString(strings.Join(parts, ", ") + "\n")
	}
	sb.WriteString("\nFor every group return an object {letter, teamIds} with the four team ids ordered best-to-worst (1st→4th). " +
		"Set bestThirds to exactly 8 group letters whose 3rd-placed team you expect to advance. Use the exact team ids given above.")

	var resp struct {
		Groups []struct {
			Letter    string   `json:"letter"`
			TeamIDs   []string `json:"teamIds"`
			Rationale string   `json:"rationale"`
		} `json:"groups"`
		BestThirds []string `json:"bestThirds"`
	}
	if err := b.completeStructured(ctx, "groups", sb.String(), groupsSchema(b.rationale), &resp); err != nil {
		return nil, nil, err
	}
	order := make(map[string][]string, len(resp.Groups))
	for _, g := range resp.Groups {
		order[g.Letter] = g.TeamIDs
		if g.Rationale != "" {
			b.log.Info("group_pick", "group", g.Letter, "rationale", g.Rationale)
		}
	}
	return order, resp.BestThirds, nil
}

// ---- forecast/tips: pick a winner between two concrete teams ----

type matchup struct {
	Num  int
	Home nameID
	Away nameID
}

// PredictWinners asks Claude, for each resolved knockout matchup, which side
// advances. Returns matchNum -> winning team id.
func (b *Brain) PredictWinners(ctx context.Context, stageLabel string, ms []matchup) (map[int]string, error) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Predict the winner of each %s knockout match (no draws — pick who advances).\n\n", stageLabel)
	for _, m := range ms {
		fmt.Fprintf(&sb, "Match %d: home=%s (id=%s) vs away=%s (id=%s)\n",
			m.Num, m.Home.Name, m.Home.ID, m.Away.Name, m.Away.ID)
	}
	sb.WriteString("\nFor each match return {matchNum, side} where side is \"home\" or \"away\" — the team you expect to advance.")

	var resp struct {
		Winners []struct {
			MatchNum  int    `json:"matchNum"`
			Side      string `json:"side"`
			Rationale string `json:"rationale"`
		} `json:"winners"`
	}
	if err := b.completeStructured(ctx, "winners", sb.String(), winnersSchema(b.rationale), &resp); err != nil {
		return nil, err
	}
	side := make(map[int]string, len(resp.Winners))
	for _, w := range resp.Winners {
		side[w.MatchNum] = strings.ToLower(strings.TrimSpace(w.Side))
		if w.Rationale != "" {
			b.log.Info("winner_pick", "match_num", w.MatchNum, "side", side[w.MatchNum], "rationale", w.Rationale)
		}
	}
	out := map[int]string{}
	for _, m := range ms {
		if side[m.Num] == "away" {
			out[m.Num] = m.Away.ID
		} else { // default to home if missing/garbled
			out[m.Num] = m.Home.ID
		}
	}
	return out, nil
}

// ---- tips: per-match scorelines ----

type tipTarget struct {
	MatchID  string
	Stage    string
	Home     string // display name (or placeholder label)
	Away     string
	HomeID   string // resolved team id (always set for tippable matches)
	AwayID   string
	Kickoff  string
	Matchday int // 1-3 for group matches (derived from kickoff order); 0 otherwise
}

type Scoreline struct{ Home, Away int }

// PredictTips asks Claude for a candidate-score distribution per upcoming match.
// Group matches may draw; knockout candidates should be decisive. The shared
// selectTip turns each distribution into the points-maximizing concrete scoreline.
func (b *Brain) PredictTips(ctx context.Context, targets []tipTarget) (map[string]TipOutcome, error) {
	// Stable ordering keeps the user prompt deterministic across runs.
	sort.Slice(targets, func(i, j int) bool { return targets[i].MatchID < targets[j].MatchID })

	var sb strings.Builder
	if b.results != "" {
		sb.WriteString("Tournament context so far — factor this into form and matchups, and revise as needed:\n")
		sb.WriteString(b.results)
		sb.WriteString("\n")
	}
	sb.WriteString("For each upcoming match, give 3–5 candidate final scorelines with your probability for each (your subjective chance; the probabilities you list should sum to about 1). ")
	sb.WriteString("For group matches a draw is allowed; for knockout matches give DECISIVE scores only (the two scores differ — the higher score advances).\n\n")
	for _, t := range targets {
		tag := "knockout"
		if t.Stage == "group" {
			if t.Matchday > 0 {
				tag = fmt.Sprintf("group MD%d", t.Matchday)
			} else {
				tag = "group"
			}
		}
		fmt.Fprintf(&sb, "key=%s [%s] %s vs %s (kickoff %s)\n", t.MatchID, tag, t.Home, t.Away, t.Kickoff)
	}
	sb.WriteString("\nFor each match return {key, scores:[{home,away,p}, …]} using the key given above.")
	if b.rationale {
		sb.WriteString(" Include a one-sentence rationale per match.")
	}

	var resp struct {
		Tips []struct {
			Key    string `json:"key"`
			Scores []struct {
				Home int     `json:"home"`
				Away int     `json:"away"`
				P    float64 `json:"p"`
			} `json:"scores"`
			Rationale string `json:"rationale"`
		} `json:"tips"`
	}
	if err := b.completeStructured(ctx, "tips", sb.String(), tipsSchema(b.rationale), &resp); err != nil {
		return nil, err
	}
	// Pass the raw distribution through; selectTip does the clamping, knockout
	// draw-removal, and points-maximizing pick (the semantic safety net).
	out := make(map[string]TipOutcome, len(resp.Tips))
	for _, t := range resp.Tips {
		o := TipOutcome{Rationale: t.Rationale}
		for _, s := range t.Scores {
			o.Scores = append(o.Scores, ScoreProb{Home: s.Home, Away: s.Away, P: s.P})
		}
		out[t.Key] = o
	}
	return out, nil
}
