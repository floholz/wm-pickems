package main

import (
	"context"
	"sort"
	"strconv"
	"strings"
)

// This file re-implements, in plain Go (no PocketBase dependency), the exact
// bracket-resolution logic the server's scoring engine uses
// (internal/scoring.fcResolver + assignThirds). The bot is a separate module
// and can't import the server's internal packages, so the small amount of logic
// is mirrored here and driven by the data from /api/forecast/structure. Getting
// this right is what makes the bot's Forecast actually score.

// stageOrder is the feeder order: a round is resolved only after the rounds
// that feed it, so W/L labels always reference already-decided matches.
var stageOrder = []string{"R32", "R16", "QF", "SF", "3RD", "FINAL"}

type koMatch struct {
	num       int
	stage     string
	homeLabel string
	awayLabel string
}

type resolver struct {
	order      map[string][]string // groupLetter -> [teamId x4]
	thirdByNum map[int]string      // R32 match num -> chosen third teamId
	bracket    map[string]string   // matchNum (string) -> winner teamId
	ko         map[int]koMatch
}

// resolve maps a knockout placeholder label ("1A", "2B", "3C/D/E", "W73",
// "L101") to a concrete team id, given the predicted group order, the assigned
// thirds, and the winners picked so far.
func (r *resolver) resolve(label string, forNum int, seen map[int]bool) string {
	if label == "" {
		return ""
	}
	switch label[0] {
	case '1', '2':
		idx := 0
		if label[0] == '2' {
			idx = 1
		}
		o := r.order[label[1:]]
		if len(o) > idx {
			return o[idx]
		}
		return ""
	case '3':
		return r.thirdByNum[forNum]
	case 'W', 'L':
		n, _ := strconv.Atoi(label[1:])
		if seen[n] {
			return ""
		}
		seen[n] = true
		w := r.bracket[strconv.Itoa(n)]
		if label[0] == 'W' {
			return w
		}
		src, ok := r.ko[n]
		if !ok || w == "" {
			return ""
		}
		h := r.resolve(src.homeLabel, n, seen)
		a := r.resolve(src.awayLabel, n, seen)
		if w == h {
			return a
		}
		if w == a {
			return h
		}
		return ""
	}
	return ""
}

// assignThirds maps the 8 chosen groups' third-placed teams onto the R32
// third-slots using FIFA's official Annex C table for that combination, with a
// deterministic backtracking fallback — identical to the server.
func assignThirds(s *Structure, thirds map[string]string) map[int]string {
	chosen := make([]string, 0, len(thirds))
	for letter := range thirds {
		chosen = append(chosen, strings.ToUpper(letter))
	}
	sort.Strings(chosen)
	key := strings.Join(chosen, "")

	out := map[int]string{}
	if m, ok := s.ThirdTable[key]; ok && len(chosen) == 8 {
		for _, slot := range s.ThirdSlots {
			if g, ok := m[slot.Winner]; ok {
				out[slot.MatchNum] = thirds[g]
			}
		}
		return out
	}

	// Fallback: deterministic backtracking perfect matching over allowed groups.
	slots := append([]struct {
		MatchNum int      `json:"matchNum"`
		Winner   string   `json:"winner"`
		Allowed  []string `json:"allowed"`
	}{}, s.ThirdSlots...)
	sort.Slice(slots, func(i, j int) bool { return slots[i].MatchNum < slots[j].MatchNum })

	assign := make([]string, len(slots))
	var solve func(i int) bool
	solve = func(i int) bool {
		if i == len(slots) {
			return true
		}
		for _, letter := range chosen {
			taken := false
			for _, a := range assign {
				if a == letter {
					taken = true
					break
				}
			}
			if taken {
				continue
			}
			allowed := false
			for _, a := range slots[i].Allowed {
				if a == letter {
					allowed = true
					break
				}
			}
			if !allowed {
				continue
			}
			assign[i] = letter
			if solve(i + 1) {
				return true
			}
			assign[i] = ""
		}
		return false
	}
	solve(0)
	for i, slot := range slots {
		if assign[i] != "" {
			out[slot.MatchNum] = thirds[assign[i]]
		}
	}
	return out
}

// pickWinners chooses, for a stage's resolved matchups, the advancing team id by
// match number. Implemented by the Brain (one Claude call per stage).
type pickWinners func(ctx context.Context, stageLabel string, ms []matchup) (map[int]string, error)

// BuildForecast assembles a self-consistent bracket: it walks the knockout
// rounds in feeder order, resolves each match's two concrete participants from
// the predicted group order + thirds + winners-so-far, asks the picker who
// advances, and records the winner. Returns {matchNum: winnerTeamId}.
func BuildForecast(ctx context.Context, s *Structure, order map[string][]string, thirds map[string]string, name func(id string) string, pick pickWinners) (map[string]string, error) {
	// Only matches with a real number are referenced by W/L labels (R32..SF);
	// the 3rd-place match and final carry num=0 in the data, so keying ko by
	// num would collide them — keep just the numbered ones here.
	ko := map[int]koMatch{}
	for _, m := range s.Knockout {
		if m.Num > 0 {
			ko[m.Num] = koMatch{num: m.Num, stage: m.Stage, homeLabel: m.HomeLabel, awayLabel: m.AwayLabel}
		}
	}
	r := &resolver{
		order:      order,
		thirdByNum: assignThirds(s, thirds),
		bracket:    map[string]string{},
		ko:         ko,
	}

	for _, stage := range stageOrder {
		// All matches in this stage, in match-number order.
		entries := make([]koMatch, 0)
		for _, m := range s.Knockout {
			if m.Stage == stage {
				entries = append(entries, koMatch{num: m.Num, stage: m.Stage, homeLabel: m.HomeLabel, awayLabel: m.AwayLabel})
			}
		}
		sort.SliceStable(entries, func(i, j int) bool { return entries[i].num < entries[j].num })

		var ms []matchup
		keyByNum := map[int]string{} // matchup.Num -> bracket key to store the winner under
		for _, e := range entries {
			h := r.resolve(e.homeLabel, e.num, map[int]bool{})
			a := r.resolve(e.awayLabel, e.num, map[int]bool{})
			if h == "" || a == "" {
				continue // unresolved — leave this match unpicked
			}
			ms = append(ms, matchup{
				Num:  e.num,
				Home: nameID{ID: h, Name: name(h)},
				Away: nameID{ID: a, Name: name(a)},
			})
			keyByNum[e.num] = stableKey(e.num, e.stage)
		}
		if len(ms) == 0 {
			continue
		}
		winners, err := pick(ctx, stage, ms)
		if err != nil {
			return nil, err
		}
		for num, winner := range winners {
			if key, ok := keyByNum[num]; ok {
				r.bracket[key] = winner
			}
		}
	}
	return r.bracket, nil
}

// stableKey is the bracket map key for a knockout match — the same convention
// the server's scoring engine uses (koStableKey): the match number when it has
// one, otherwise the stage name. The 3rd-place match and final have no number,
// so they're keyed "3RD" and "FINAL".
func stableKey(num int, stage string) string {
	if num > 0 {
		return strconv.Itoa(num)
	}
	return stage
}
