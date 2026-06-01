package main

import (
	"context"
	"math"
	"sort"
)

// AlgoBrain is a deterministic, API-free predictor: a rating-based ("Elo-lite")
// model. Each team gets a strength rating (from an embedded table keyed by FIFA
// code, with a neutral default for unknowns); group order, the bracket, and
// scorelines all follow from those ratings. Same inputs → same predictions, so
// it's a stable "house" baseline for the others to beat.
type AlgoBrain struct {
	rating map[string]int // teamId -> rating
}

// NewAlgoBrain seeds ratings from the static table, then applies Elo updates
// from every finished match — the feedback loop. After a few results the
// ratings (and therefore the tips) reflect what actually happened, not just the
// pre-tournament table.
func NewAlgoBrain(teams []Team, finished []Match) *AlgoBrain {
	r := make(map[string]int, len(teams))
	for _, t := range teams {
		r[t.ID] = ratingFor(t.FifaCode)
	}
	applyElo(r, finished)
	return &AlgoBrain{rating: r}
}

// eloK is the update step. A short tournament wants fairly responsive ratings,
// so this is on the higher side.
const eloK = 40.0

// applyElo nudges team ratings toward what results showed, in chronological
// order, using a goal-difference-weighted Elo update (the standard
// World-Football-Elo shape).
func applyElo(r map[string]int, finished []Match) {
	ms := append([]Match(nil), finished...)
	sort.SliceStable(ms, func(i, j int) bool { return ms[i].Kickoff < ms[j].Kickoff })
	for _, m := range ms {
		if !m.Finished() {
			continue
		}
		hs, as := referenceScore(m)
		ra, rb := float64(r[m.HomeTeam]), float64(r[m.AwayTeam])
		expHome := 1.0 / (1.0 + math.Pow(10, (rb-ra)/400.0))
		delta := int(math.Round(eloK * goalMultiplier(absInt(hs-as)) * (outcome(hs, as) - expHome)))
		r[m.HomeTeam] += delta
		r[m.AwayTeam] -= delta
	}
}

// referenceScore is the after-extra-time score when ET was played, else the 90'
// score. A penalty shootout (level after ET) reads as a draw for rating
// purposes.
func referenceScore(m Match) (int, int) {
	if m.EtHome > 0 || m.EtAway > 0 {
		return m.EtHome, m.EtAway
	}
	return m.FtHome, m.FtAway
}

func outcome(home, away int) float64 {
	switch {
	case home > away:
		return 1
	case away > home:
		return 0
	default:
		return 0.5
	}
}

// goalMultiplier weights bigger wins more (eloratings.net convention).
func goalMultiplier(margin int) float64 {
	switch {
	case margin <= 1:
		return 1
	case margin == 2:
		return 1.5
	default:
		return float64(11+margin) / 8.0
	}
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func (a *AlgoBrain) rat(id string) int {
	if v, ok := a.rating[id]; ok {
		return v
	}
	return defaultRating
}

// PredictGroups: order each group by rating (desc), and pick the 8 groups whose
// third-placed team is the strongest. Ties break by id/letter for determinism.
func (a *AlgoBrain) PredictGroups(_ context.Context, groups []groupPick) (map[string][]string, []string, error) {
	order := make(map[string][]string, len(groups))
	type third struct {
		letter string
		r      int
	}
	var thirds []third
	for _, g := range groups {
		ids := make([]string, len(g.Teams))
		for i, t := range g.Teams {
			ids[i] = t.ID
		}
		sort.SliceStable(ids, func(i, j int) bool {
			if ri, rj := a.rat(ids[i]), a.rat(ids[j]); ri != rj {
				return ri > rj
			}
			return ids[i] < ids[j]
		})
		order[g.Letter] = ids
		if len(ids) >= 3 {
			thirds = append(thirds, third{letter: g.Letter, r: a.rat(ids[2])})
		}
	}
	sort.SliceStable(thirds, func(i, j int) bool {
		if thirds[i].r != thirds[j].r {
			return thirds[i].r > thirds[j].r
		}
		return thirds[i].letter < thirds[j].letter
	})
	best := make([]string, 0, 8)
	for i := 0; i < len(thirds) && i < 8; i++ {
		best = append(best, thirds[i].letter)
	}
	return order, best, nil
}

// PredictWinners: the higher-rated team advances; a tie goes to the home side.
func (a *AlgoBrain) PredictWinners(_ context.Context, _ string, ms []matchup) (map[int]string, error) {
	out := make(map[int]string, len(ms))
	for _, m := range ms {
		if a.rat(m.Away.ID) > a.rat(m.Home.ID) {
			out[m.Num] = m.Away.ID
		} else {
			out[m.Num] = m.Home.ID
		}
	}
	return out, nil
}

// PredictTips: expected goals from the rating gap. Group games may draw;
// knockouts are coerced to a decisive favourite (the server needs a winner). The
// algo commits to a single scoreline, so it returns a degenerate distribution
// (one candidate at p=1) — selectTip then returns exactly that score.
func (a *AlgoBrain) PredictTips(_ context.Context, targets []tipTarget) (map[string]TipOutcome, error) {
	out := make(map[string]TipOutcome, len(targets))
	for _, t := range targets {
		rh, ra := a.rat(t.HomeID), a.rat(t.AwayID)
		h, av := expectedGoals(rh, ra), expectedGoals(ra, rh)
		if t.Stage != "group" && h == av {
			if rh >= ra {
				h++
			} else {
				av++
			}
		}
		out[t.MatchID] = TipOutcome{Scores: []ScoreProb{{Home: h, Away: av, P: 1}}}
	}
	return out, nil
}

const (
	algoBase     = 1.25  // expected goals for an evenly-matched side
	algoStep     = 160.0 // rating points ≈ one extra/fewer goal
	algoMaxGoals = 7     // safety bound only — not a realism cap
)

// expectedGoals maps a rating gap to a goal tally: round(base + gap/step). It's
// linear and deliberately *not* capped low — a genuine mismatch can read 4-5+,
// which is realistic for some group games. It no longer blows up on every game
// because the ratings table now covers the whole field (so most gaps are
// modest); only a top side vs a real minnow produces a rout. The opponent's
// tally uses the negated gap.
func expectedGoals(forRating, oppRating int) int {
	g := int(math.Round(algoBase + float64(forRating-oppRating)/algoStep))
	return max(0, min(g, algoMaxGoals))
}

// defaultRating is used for any team whose FIFA code isn't in the table below
// (e.g. a late or unexpected qualifier) — a mid/low baseline.
const defaultRating = 1500

// ratings is the strength table for the seeded WC2026 field (48 teams), keyed
// by FIFA 3-letter code. Values are rough Elo-style numbers (stronger = higher)
// and only need to rank teams sensibly relative to each other. This single
// table is what gives the algo bot its "opinion" — edit freely.
var ratings = map[string]int{
	// Top contenders
	"ESP": 2120, "FRA": 2100, "ARG": 2100, "ENG": 2050, "BRA": 2040,
	"POR": 2030, "NED": 2010, "GER": 1990, "COL": 1960, "BEL": 1950,
	"CRO": 1950, "URU": 1950,
	// Strong
	"MAR": 1900, "SEN": 1900, "NOR": 1900, "JPN": 1880, "SUI": 1860,
	"ECU": 1850, "CZE": 1820, "AUT": 1820, "KOR": 1810, "TUR": 1810,
	"CAN": 1800, "SCO": 1800, "USA": 1800,
	// Mid
	"ALG": 1790, "IRN": 1790, "MEX": 1790, "SWE": 1780, "CIV": 1780,
	"EGY": 1760, "AUS": 1740, "BIH": 1740, "PAR": 1730, "TUN": 1720,
	"GHA": 1720, "UZB": 1720, "COD": 1710, "RSA": 1700,
	// Lower
	"PAN": 1690, "QAT": 1680, "KSA": 1670, "IRQ": 1660, "JOR": 1640,
	"CPV": 1620, "NZL": 1600, "HAI": 1580, "CUW": 1520,
}

func ratingFor(fifaCode string) int {
	if v, ok := ratings[fifaCode]; ok {
		return v
	}
	return defaultRating
}
