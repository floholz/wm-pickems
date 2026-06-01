package main

import "sort"

// ScoringWeights mirrors the app's DEFAULT scoring_configs for WC2026 (per-match
// Tips, max 6 pts). A user's tip is global — one per match, shared across every
// league they're in — so the bot can't honor per-league overrides; it optimizes
// for the global default.
//
// NOTE: hardcoded for this World Cup. Keep in sync with the app default
// (internal/scoring + seed.DefaultScoringConfig). Revisit if that default changes.
type ScoringWeights struct {
	Result int // correct tendency (group 1/X/2) or knockout advancer
	Exact  int // exact reference score
	Total  int // correct total goals
	Diff   int // correct goal difference
}

var defaultWeights = ScoringWeights{Result: 3, Exact: 1, Total: 1, Diff: 1}

// ScoreProb is one candidate scoreline with the model's subjective probability.
type ScoreProb struct {
	Home, Away int
	P          float64
}

// TipOutcome is a brain's prediction for one match: a distribution over candidate
// scorelines, plus an optional rationale. A degenerate distribution (a single
// score at p=1, as the algo brain returns) is valid — selectTip handles it.
//
// This is the only thing a brain exposes through the Predictor interface; how it
// derived the distribution stays private to that brain (no cross-bot data).
type TipOutcome struct {
	Scores    []ScoreProb
	Rationale string
}

func sign(d int) int {
	switch {
	case d > 0:
		return 1
	case d < 0:
		return -1
	default:
		return 0
	}
}

// points scores a submitted scoreline against a hypothetical true scoreline under
// w: tendency/advancer (Result) + exact + total goals + goal difference. The
// advancer term reduces to the tendency sign because knockout candidates are
// always decisive.
func points(sub, tru Scoreline, w ScoringWeights) int {
	p := 0
	if sign(sub.Home-sub.Away) == sign(tru.Home-tru.Away) {
		p += w.Result
	}
	if sub.Home == tru.Home && sub.Away == tru.Away {
		p += w.Exact
	}
	if sub.Home+sub.Away == tru.Home+tru.Away {
		p += w.Total
	}
	if sub.Home-sub.Away == tru.Home-tru.Away {
		p += w.Diff
	}
	return p
}

// selectTip returns the scoreline that maximizes expected points under w, given
// the model's candidate-score distribution. The "true outcome" distribution is
// the model's own (clamped/normalized); candidate submissions are the model's
// scores plus a small common-score grid, so a safer pick the model didn't list
// is still reachable. For knockouts, draws are excluded (must be decisive).
//
// This is the shared decision layer: a pure function each bot calls with its own
// distribution. It holds no state and never sees more than one bot's data.
func selectTip(o TipOutcome, stage string, w ScoringWeights) Scoreline {
	ko := stage != "group"
	dist := normalizeDist(o.Scores, ko)
	if len(dist) == 0 {
		return Scoreline{Home: 1, Away: 0} // nothing usable — safe decisive default
	}
	cands := candidateGrid(o.Scores, ko)
	best, bestEV := cands[0], -1.0
	for _, c := range cands {
		ev := 0.0
		for _, d := range dist {
			ev += d.P * float64(points(c, Scoreline{Home: d.Home, Away: d.Away}, w))
		}
		if ev > bestEV {
			best, bestEV = c, ev
		}
	}
	return best
}

// normalizeDist clamps negative goals, drops knockout draws and non-positive
// probabilities, then renormalizes the survivors to sum to 1.
func normalizeDist(scores []ScoreProb, ko bool) []ScoreProb {
	var out []ScoreProb
	var sum float64
	for _, s := range scores {
		h, a := max(s.Home, 0), max(s.Away, 0)
		if (ko && h == a) || s.P <= 0 {
			continue
		}
		out = append(out, ScoreProb{Home: h, Away: a, P: s.P})
		sum += s.P
	}
	for i := range out {
		out[i].P /= sum
	}
	return out
}

// candidateGrid is the set of scorelines selectTip may submit: the model's own
// candidates (highest-probability first, as a tie-break anchor) plus a small grid
// of common scorelines in both orientations. Knockout candidates are decisive.
func candidateGrid(scores []ScoreProb, ko bool) []Scoreline {
	seen := map[Scoreline]bool{}
	var out []Scoreline
	add := func(h, a int) {
		if h < 0 || a < 0 || (ko && h == a) {
			return
		}
		s := Scoreline{Home: h, Away: a}
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	ms := append([]ScoreProb(nil), scores...)
	sort.SliceStable(ms, func(i, j int) bool { return ms[i].P > ms[j].P })
	for _, s := range ms {
		add(s.Home, s.Away)
	}
	for _, c := range [][2]int{{0, 0}, {1, 0}, {1, 1}, {2, 0}, {2, 1}, {2, 2}, {3, 1}, {3, 0}} {
		add(c[0], c[1])
		add(c[1], c[0])
	}
	return out
}
