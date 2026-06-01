package main

import "context"

// Predictor is the "brain" interface shared by every bot kind. The Claude brain
// (brain.go) and the algorithmic brain (algo.go) both implement it, so main.go
// can pick one by BOT_KIND and the rest of the flow is identical.
type Predictor interface {
	// PredictGroups orders each group 1st..4th (team ids) and returns the 8
	// chosen group letters whose third-placed team is expected to advance.
	PredictGroups(ctx context.Context, groups []groupPick) (map[string][]string, []string, error)
	// PredictWinners picks the advancing team id for each resolved knockout
	// matchup (by match number).
	PredictWinners(ctx context.Context, stageLabel string, ms []matchup) (map[int]string, error)
	// PredictTips returns an outcome distribution per match (keyed by match id);
	// the shared selectTip turns each into a concrete scoreline.
	PredictTips(ctx context.Context, targets []tipTarget) (map[string]TipOutcome, error)
}
