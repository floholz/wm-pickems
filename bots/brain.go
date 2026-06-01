package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Brain turns the shared prompt/schema logic into predictions over a pluggable
// completer (the provider transport). It owns everything provider-agnostic —
// prompt assembly, JSON schemas, response parsing — so swapping Claude for an
// OpenRouter-fronted model (GPT, Gemini, …) is just a different completer; the
// prompts, schemas, and repair logic downstream are identical.
//
// The large, never-changing tournament reference (teams, group memberships,
// knockout skeleton) lives in the completer's system prompt so every prediction
// call reuses it as a prompt-cache prefix — only the per-call task varies.
type Brain struct {
	comp      completer // provider transport (Anthropic native, or OpenRouter)
	results   string    // results-so-far summary, fed into tip prompts (the feedback loop)
	rationale bool      // ask for + log a one-line reason per prediction
}

func NewBrain(comp completer, results string, rationale bool) *Brain {
	return &Brain{comp: comp, results: results, rationale: rationale}
}

// completer is the provider transport: one structured request (cached system
// prefix + task user message, constrained to schema) returning the final text,
// which the schema guarantees is valid JSON (no fences/preamble to strip).
// anthropicCompleter speaks the native Anthropic API (prompt caching + adaptive
// thinking); openrouterCompleter speaks the OpenAI-compatible wire format
// OpenRouter exposes as one key over every provider.
type completer interface {
	complete(ctx context.Context, label, task string, schema map[string]any) (string, error)
}

// completeStructured runs one completer call with a JSON-schema constraint and
// unmarshals the (schema-guaranteed valid) reply into out.
func (b *Brain) completeStructured(ctx context.Context, label, task string, schema map[string]any, out any) error {
	raw, err := b.comp.complete(ctx, label, task, schema)
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
func (b *Brain) PredictGroups(ctx context.Context, groups []groupPick) (map[string][]string, []string, map[string]string, error) {
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
		return nil, nil, nil, err
	}
	order := make(map[string][]string, len(resp.Groups))
	rationale := map[string]string{}
	for _, g := range resp.Groups {
		order[g.Letter] = g.TeamIDs
		if g.Rationale != "" {
			rationale[g.Letter] = g.Rationale
		}
	}
	return order, resp.BestThirds, rationale, nil
}

// ---- forecast/tips: pick a winner between two concrete teams ----

type matchup struct {
	Num  int
	Home nameID
	Away nameID
}

// PredictWinners asks Claude, for each resolved knockout matchup, which side
// advances. Returns matchNum -> winning team id, plus matchNum -> rationale.
func (b *Brain) PredictWinners(ctx context.Context, stageLabel string, ms []matchup) (map[int]string, map[int]string, error) {
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
		return nil, nil, err
	}
	side := make(map[int]string, len(resp.Winners))
	rationale := map[int]string{}
	for _, w := range resp.Winners {
		side[w.MatchNum] = strings.ToLower(strings.TrimSpace(w.Side))
		if w.Rationale != "" {
			rationale[w.MatchNum] = w.Rationale
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
	return out, rationale, nil
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
