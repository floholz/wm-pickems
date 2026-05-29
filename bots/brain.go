package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// Brain wraps the Anthropic client. The large, never-changing tournament
// reference (teams, group memberships, knockout skeleton) lives in a single
// cached system prompt so every prediction call reuses it as a prompt-cache
// prefix — only the per-call task in the user turn varies.
type Brain struct {
	client anthropic.Client
	model  string
	system string // static reference, identical across all calls (cache prefix)
}

func NewBrain(model, reference string) *Brain {
	return &Brain{
		client: anthropic.NewClient(), // reads ANTHROPIC_API_KEY
		model:  model,
		system: reference,
	}
}

// complete runs one streamed request with adaptive thinking and a cached system
// prompt, returning the concatenated final text. Streaming avoids HTTP timeouts
// on larger outputs and lets thinking run without a fixed budget.
func (b *Brain) complete(ctx context.Context, task string) (string, error) {
	stream := b.client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(b.model),
		MaxTokens: 32000,
		Thinking:  anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}},
		System: []anthropic.TextBlockParam{{
			Text:         b.system,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},
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
	return sb.String(), nil
}

// completeJSON runs complete() and unmarshals the response into out, tolerating
// a stray ```json fence if the model adds one.
func (b *Brain) completeJSON(ctx context.Context, task string, out any) error {
	raw, err := b.complete(ctx, task)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(stripFence(raw)), out)
}

func stripFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	// Drop the opening fence line and the trailing fence.
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.LastIndex(s, "```"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
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
	sb.WriteString(`
Respond with ONLY a JSON object, no prose, no markdown:
{"groups": {"A": ["id1","id2","id3","id4"], ... all 12 groups},
 "bestThirds": ["A","C", ... exactly 8 group letters]}
Use the exact team ids given above, each group ordered best-to-worst.`)

	var resp struct {
		Groups     map[string][]string `json:"groups"`
		BestThirds []string            `json:"bestThirds"`
	}
	if err := b.completeJSON(ctx, sb.String(), &resp); err != nil {
		return nil, nil, err
	}
	return resp.Groups, resp.BestThirds, nil
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
	sb.WriteString(`
Respond with ONLY a JSON object mapping each match number to "home" or "away":
{"winners": {"73": "home", "74": "away", ...}}`)

	var resp struct {
		Winners map[string]string `json:"winners"`
	}
	if err := b.completeJSON(ctx, sb.String(), &resp); err != nil {
		return nil, err
	}
	out := map[int]string{}
	for _, m := range ms {
		switch strings.ToLower(strings.TrimSpace(resp.Winners[fmt.Sprintf("%d", m.Num)])) {
		case "away":
			out[m.Num] = m.Away.ID
		default: // default to home if missing/garbled
			out[m.Num] = m.Home.ID
		}
	}
	return out, nil
}

// ---- tips: per-match scorelines ----

type tipTarget struct {
	MatchID string
	Stage   string
	Home    string
	Away    string
	Kickoff string
}

type Scoreline struct{ Home, Away int }

// PredictTips asks Claude for a scoreline for each upcoming match. Knockout
// matches are constrained to a decisive 90' result (no draw).
func (b *Brain) PredictTips(ctx context.Context, targets []tipTarget) (map[string]Scoreline, error) {
	// Stable ordering keeps the user prompt deterministic across runs.
	sort.Slice(targets, func(i, j int) bool { return targets[i].MatchID < targets[j].MatchID })

	var sb strings.Builder
	sb.WriteString("Predict the final score of each upcoming match. ")
	sb.WriteString("For group matches a draw is allowed. For knockout matches pick a DECISIVE 90-minute score (the two scores must differ — the higher score is the team that advances).\n\n")
	for _, t := range targets {
		kind := "group"
		if t.Stage != "group" {
			kind = "knockout"
		}
		fmt.Fprintf(&sb, "key=%s [%s] %s vs %s (kickoff %s)\n", t.MatchID, kind, t.Home, t.Away, t.Kickoff)
	}
	sb.WriteString(`
Respond with ONLY a JSON object mapping each key to [homeGoals, awayGoals] integers:
{"tips": {"<key>": [2, 1], ...}}`)

	var resp struct {
		Tips map[string][]int `json:"tips"`
	}
	if err := b.completeJSON(ctx, sb.String(), &resp); err != nil {
		return nil, err
	}
	out := map[string]Scoreline{}
	for _, t := range targets {
		v := resp.Tips[t.MatchID]
		if len(v) != 2 {
			continue
		}
		h, a := v[0], v[1]
		if h < 0 {
			h = 0
		}
		if a < 0 {
			a = 0
		}
		// Knockouts must be decisive; coerce a predicted draw to a home edge.
		if t.Stage != "group" && h == a {
			h++
		}
		out[t.MatchID] = Scoreline{Home: h, Away: a}
	}
	return out, nil
}
