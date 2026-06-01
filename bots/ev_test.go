package main

import "testing"

func TestPoints(t *testing.T) {
	// 2-1 vs 2-1: result+exact+total+diff = 6. 2-1 vs 1-0: result+diff = 4.
	if p := points(Scoreline{Home: 2, Away: 1}, Scoreline{Home: 2, Away: 1}, defaultWeights); p != 6 {
		t.Errorf("exact match = %d, want 6", p)
	}
	if p := points(Scoreline{Home: 2, Away: 1}, Scoreline{Home: 1, Away: 0}, defaultWeights); p != 4 {
		t.Errorf("2-1 vs 1-0 = %d, want 4 (result+diff)", p)
	}
}

func TestSelectTipDegenerate(t *testing.T) {
	// A single candidate at p=1 (the algo brain's shape) is returned verbatim.
	if s := selectTip(TipOutcome{Scores: []ScoreProb{{Home: 2, Away: 0, P: 1}}}, "group", defaultWeights); s != (Scoreline{Home: 2, Away: 0}) {
		t.Errorf("degenerate group = %v, want 2-0", s)
	}
	if s := selectTip(TipOutcome{Scores: []ScoreProb{{Home: 1, Away: 0, P: 1}}}, "R16", defaultWeights); s != (Scoreline{Home: 1, Away: 0}) {
		t.Errorf("degenerate KO = %v, want 1-0", s)
	}
}

func TestSelectTipPrefersDominantTendency(t *testing.T) {
	// The modal exact score is a draw (1-1, .40), but the home-win tendency holds
	// the majority of the mass (.60). Since result (3) dwarfs the score-shape
	// terms, the EV-optimal submission is a home win, not the draw.
	o := TipOutcome{Scores: []ScoreProb{
		{Home: 1, Away: 1, P: 0.40},
		{Home: 2, Away: 1, P: 0.35},
		{Home: 1, Away: 0, P: 0.20},
		{Home: 3, Away: 1, P: 0.05},
	}}
	if s := selectTip(o, "group", defaultWeights); s.Home <= s.Away {
		t.Errorf("selected %v, want a home-win scoreline (dominant tendency)", s)
	}
}

func TestSelectTipKnockoutDecisive(t *testing.T) {
	// All listed candidates are draws; the knockout pick must still be decisive.
	o := TipOutcome{Scores: []ScoreProb{{Home: 1, Away: 1, P: 0.6}, {Home: 0, Away: 0, P: 0.4}}}
	if s := selectTip(o, "R16", defaultWeights); s.Home == s.Away {
		t.Errorf("knockout pick %v must be decisive", s)
	}
}
