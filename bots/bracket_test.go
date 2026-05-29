package main

import (
	"context"
	"testing"
)

// TestBuildForecastConsistency checks the resolver port: every recorded bracket
// winner must be one of that match's resolved participants, and W/L feeder
// labels must resolve through earlier rounds. A bracket that violates this
// wouldn't score on the server.
func TestBuildForecastConsistency(t *testing.T) {
	s := &Structure{
		Groups: []struct {
			Letter string   `json:"letter"`
			Teams  []string `json:"teams"`
		}{
			{Letter: "A", Teams: []string{"a1", "a2", "a3", "a4"}},
			{Letter: "B", Teams: []string{"b1", "b2", "b3", "b4"}},
		},
		Knockout: []struct {
			Num       int    `json:"num"`
			Stage     string `json:"stage"`
			HomeLabel string `json:"homeLabel"`
			AwayLabel string `json:"awayLabel"`
		}{
			{Num: 1, Stage: "R32", HomeLabel: "1A", AwayLabel: "2B"},
			{Num: 2, Stage: "R32", HomeLabel: "1B", AwayLabel: "2A"},
			// The 3rd-place match and final carry no number in the real data
			// (num=0) — they must be keyed by stage, not "0".
			{Num: 0, Stage: "3RD", HomeLabel: "L1", AwayLabel: "L2"},
			{Num: 0, Stage: "FINAL", HomeLabel: "W1", AwayLabel: "W2"},
		},
	}

	// Predicted order: group winners a1/b1, runners-up a2/b2.
	order := map[string][]string{
		"A": {"a1", "a2", "a3", "a4"},
		"B": {"b1", "b2", "b3", "b4"},
	}
	// Deterministic picker: always the home side advances.
	homePicker := func(_ context.Context, _ string, ms []matchup) (map[int]string, error) {
		out := map[int]string{}
		for _, m := range ms {
			out[m.Num] = m.Home.ID
		}
		return out, nil
	}
	ident := func(id string) string { return id }

	bracket, err := BuildForecast(context.Background(), s, order, map[string]string{}, ident, homePicker)
	if err != nil {
		t.Fatalf("BuildForecast: %v", err)
	}

	// R32: home advances.
	if bracket["1"] != "a1" {
		t.Errorf("match 1 winner = %q, want a1", bracket["1"])
	}
	if bracket["2"] != "b1" {
		t.Errorf("match 2 winner = %q, want b1", bracket["2"])
	}
	// 3RD: losers were b2 (match1: a1 vs b2) and a2 (match2: b1 vs a2); home (b2)
	// advances. Keyed by stage because the match has no number.
	if bracket["3RD"] != "b2" {
		t.Errorf("third-place winner = %q (key 3RD), want b2 (loser of match 1)", bracket["3RD"])
	}
	// FINAL: winners a1 vs b1; home (a1) advances → champion. Keyed "FINAL".
	if bracket["FINAL"] != "a1" {
		t.Errorf("final winner = %q (key FINAL), want a1", bracket["FINAL"])
	}
}

// TestAssignThirdsTable verifies the FIFA Annex C lookup path: the chosen-group
// key selects the winner→thirdGroup mapping, and each slot gets that group's
// third-placed team id.
func TestAssignThirdsTable(t *testing.T) {
	s := &Structure{
		ThirdSlots: []struct {
			MatchNum int      `json:"matchNum"`
			Winner   string   `json:"winner"`
			Allowed  []string `json:"allowed"`
		}{
			{MatchNum: 73, Winner: "A", Allowed: []string{"C", "D"}},
			{MatchNum: 74, Winner: "B", Allowed: []string{"C", "D"}},
		},
		ThirdTable: map[string]map[string]string{
			// key is the sorted concatenation of the 8 chosen group letters.
			"ABCDEFGH": {"A": "C", "B": "D"},
		},
	}
	thirds := map[string]string{
		"A": "a3", "B": "b3", "C": "c3", "D": "d3",
		"E": "e3", "F": "f3", "G": "g3", "H": "h3",
	}
	got := assignThirds(s, thirds)
	if got[73] != "c3" {
		t.Errorf("slot 73 = %q, want c3 (group C third via winner A)", got[73])
	}
	if got[74] != "d3" {
		t.Errorf("slot 74 = %q, want d3 (group D third via winner B)", got[74])
	}
}
