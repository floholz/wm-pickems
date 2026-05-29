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

func NewAlgoBrain(teams []Team) *AlgoBrain {
	r := make(map[string]int, len(teams))
	for _, t := range teams {
		r[t.ID] = ratingFor(t.FifaCode)
	}
	return &AlgoBrain{rating: r}
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
// knockouts are coerced to a decisive favourite (the server needs a winner).
func (a *AlgoBrain) PredictTips(_ context.Context, targets []tipTarget) (map[string]Scoreline, error) {
	out := make(map[string]Scoreline, len(targets))
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
		out[t.MatchID] = Scoreline{Home: h, Away: av}
	}
	return out, nil
}

const (
	algoBase     = 1.3   // expected goals for an evenly-matched side
	algoAmp      = 1.6   // max goal swing from a large rating gap (saturating)
	algoScale    = 300.0 // rating points that produce a sizeable swing
	algoMaxGoals = 5
)

// expectedGoals maps a rating gap to a goal tally with a *saturating* curve:
// round(base + amp·tanh(gap/scale)). tanh keeps blowouts realistic — equal
// teams → 1, a moderate edge → 2, and even a huge gap tops out around 3 rather
// than running away to 5-6. The opponent's tally uses the negated gap.
func expectedGoals(forRating, oppRating int) int {
	g := int(math.Round(algoBase + algoAmp*math.Tanh(float64(forRating-oppRating)/algoScale)))
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
