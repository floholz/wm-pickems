package main

import (
	"context"
	"testing"
)

func testTeams() []Team {
	return []Team{
		{ID: "arg", FifaCode: "ARG"}, // 2100
		{ID: "bra", FifaCode: "BRA"}, // 2040
		{ID: "usa", FifaCode: "USA"}, // 1800
		{ID: "nzl", FifaCode: "NZL"}, // 1600
		{ID: "xx1", FifaCode: "ZZ1"}, // default 1500
		{ID: "xx2", FifaCode: "ZZ2"}, // default 1500
	}
}

func testAlgo() *AlgoBrain {
	return NewAlgoBrain(testTeams(), nil)
}

func TestAlgoGroupsOrderByRating(t *testing.T) {
	a := testAlgo()
	order, thirds, _ := a.PredictGroups(context.Background(), []groupPick{
		{Letter: "A", Teams: []nameID{{ID: "nzl"}, {ID: "arg"}, {ID: "usa"}, {ID: "bra"}}},
	})
	want := []string{"arg", "bra", "usa", "nzl"}
	for i, id := range want {
		if order["A"][i] != id {
			t.Fatalf("order[A] = %v, want %v", order["A"], want)
		}
	}
	if len(thirds) != 1 || thirds[0] != "A" {
		t.Errorf("bestThirds = %v, want [A]", thirds)
	}
}

func TestAlgoWinnerHigherRated(t *testing.T) {
	a := testAlgo()
	// Stronger away team advances; equal ratings go to home.
	got, _ := a.PredictWinners(context.Background(), "R16", []matchup{
		{Num: 1, Home: nameID{ID: "nzl"}, Away: nameID{ID: "arg"}},
		{Num: 2, Home: nameID{ID: "xx1"}, Away: nameID{ID: "xx2"}},
	})
	if got[1] != "arg" {
		t.Errorf("match 1 winner = %q, want arg (higher rated)", got[1])
	}
	if got[2] != "xx1" {
		t.Errorf("match 2 winner = %q, want xx1 (home on a tie)", got[2])
	}
}

func TestAlgoEloFeedback(t *testing.T) {
	base := NewAlgoBrain(testTeams(), nil)

	// Two big NZL wins over ARG: the loser drops, the (over-performing) winner
	// rises, and the gap narrows sharply — the bot learns from results. Elo is
	// conservative, so a single huge gap need not fully invert in two games.
	results := []Match{
		{Status: "finished", HomeTeam: "nzl", AwayTeam: "arg", FtHome: 3, FtAway: 0, Kickoff: "2026-06-12 15:00:00.000Z", FinalizedAt: "2026-06-12 17:00:00.000Z"},
		{Status: "finished", HomeTeam: "arg", AwayTeam: "nzl", FtHome: 0, FtAway: 2, Kickoff: "2026-06-16 15:00:00.000Z", FinalizedAt: "2026-06-16 17:00:00.000Z"},
	}
	learned := NewAlgoBrain(testTeams(), results)
	if learned.rat("nzl") <= base.rat("nzl") {
		t.Errorf("NZL rating did not rise after wins: %d -> %d", base.rat("nzl"), learned.rat("nzl"))
	}
	if learned.rat("arg") >= base.rat("arg") {
		t.Errorf("ARG rating did not fall after losses: %d -> %d", base.rat("arg"), learned.rat("arg"))
	}
	if gOld, gNew := base.rat("arg")-base.rat("nzl"), learned.rat("arg")-learned.rat("nzl"); gNew >= gOld {
		t.Errorf("gap did not narrow: %d -> %d", gOld, gNew)
	}

	// On an even pair, a single win flips the predicted winner.
	flip := []Match{
		{Status: "finished", HomeTeam: "xx2", AwayTeam: "xx1", FtHome: 1, FtAway: 0, Kickoff: "2026-06-12 15:00:00.000Z", FinalizedAt: "2026-06-12 17:00:00.000Z"},
	}
	learned2 := NewAlgoBrain(testTeams(), flip)
	ms := []matchup{{Num: 1, Home: nameID{ID: "xx1"}, Away: nameID{ID: "xx2"}}}
	if got, _ := learned2.PredictWinners(context.Background(), "R16", ms); got[1] != "xx2" {
		t.Errorf("after xx2 beat xx1, winner = %q, want xx2", got[1])
	}
}

func TestAlgoTips(t *testing.T) {
	a := testAlgo()
	got, _ := a.PredictTips(context.Background(), []tipTarget{
		{MatchID: "g", Stage: "group", HomeID: "xx1", AwayID: "xx2"}, // equal
		{MatchID: "f", Stage: "group", HomeID: "arg", AwayID: "nzl"}, // big favourite home
		{MatchID: "k", Stage: "R16", HomeID: "xx1", AwayID: "xx2"},   // equal, knockout
	})
	// The algo returns a degenerate distribution (one candidate at p=1).
	if s := got["g"].Scores[0]; s.Home != 1 || s.Away != 1 {
		t.Errorf("equal group tip = %d-%d, want 1-1", s.Home, s.Away)
	}
	if s := got["f"].Scores[0]; s.Home <= s.Away {
		t.Errorf("favourite tip = %d-%d, want home > away", s.Home, s.Away)
	}
	if s := got["k"].Scores[0]; s.Home == s.Away {
		t.Errorf("knockout tip = %d-%d, must be decisive (no draw)", s.Home, s.Away)
	}
}
