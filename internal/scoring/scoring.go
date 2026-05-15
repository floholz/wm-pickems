// Package scoring computes match (Tip) and tournament (Forecast) points from
// a per-League scoring config, recomputes on every result change, and builds
// League leaderboards with the agreed tiebreakers.
//
// Scale is tiny (friends app: a handful of users, 104 matches), so every
// result change triggers a full, idempotent recompute — simplest and always
// correct.
package scoring

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// ---- Config ----

type Config struct {
	Match struct {
		Tendency   int  `json:"tendency"`
		Exact      int  `json:"exact"`
		TotalGoals int  `json:"totalGoals"`
		GoalDiff   int  `json:"goalDiff"`
		KoOtBonus  bool `json:"koOtBonus"`
		Advancer   int  `json:"advancer"`
	} `json:"match"`
	Forecast struct {
		GroupPosition     int            `json:"groupPosition"`
		PerfectGroupBonus int            `json:"perfectGroupBonus"`
		ThirdQualifier    int            `json:"thirdQualifier"`
		Round             map[string]int `json:"round"`
	} `json:"forecast"`
}

func loadConfig(rec *core.Record) Config {
	var c Config
	_ = json.Unmarshal([]byte(rec.GetString("config")), &c)
	return c
}

// configsInUse returns every scoring config referenced by a League plus the
// default, so per-(user,match,config) scores cover all Leagues.
func configsInUse(app core.App) (map[string]Config, string, error) {
	out := map[string]Config{}
	def, err := app.FindFirstRecordByFilter("scoring_configs", "isDefault = true")
	if err != nil {
		return nil, "", err
	}
	out[def.Id] = loadConfig(def)
	leagues, err := app.FindRecordsByFilter("leagues", "id != ''", "", 0, 0)
	if err != nil {
		return nil, "", err
	}
	for _, l := range leagues {
		cid := l.GetString("scoringConfig")
		if _, done := out[cid]; cid == "" || done {
			continue
		}
		if cr, err := app.FindRecordById("scoring_configs", cid); err == nil {
			out[cid] = loadConfig(cr)
		}
	}
	return out, def.Id, nil
}

func sign(n int) int {
	if n > 0 {
		return 1
	}
	if n < 0 {
		return -1
	}
	return 0
}

// ---- Match (Tip) scoring ----

type tipComponents struct {
	Tendency   int `json:"tendency"`
	Exact      int `json:"exact"`
	TotalGoals int `json:"totalGoals"`
	GoalDiff   int `json:"goalDiff"`
	OtBonus    int `json:"otBonus"`
	Advancer   int `json:"advancer"`
	GdDev      int `json:"gdDev"` // |predicted GD - actual GD| (tiebreaker only)
}

func (c tipComponents) points() int {
	return c.Tendency + c.Exact + c.TotalGoals + c.GoalDiff + c.OtBonus + c.Advancer
}

// MatchResult / TipPrediction are the plain inputs to the pure scorer, so the
// rules are unit-testable without a database.
type MatchResult struct {
	Stage    string
	FtH, FtA int
	EtH, EtA int
	Advancer string
}
type TipPrediction struct {
	FtH, FtA int
	EtH, EtA int
	Advancer string
}

// scoreValues is the pure scoring core (see scoring_test.go).
func scoreValues(cfg Config, m MatchResult, p TipPrediction) tipComponents {
	var r tipComponents
	if sign(p.FtH-p.FtA) == sign(m.FtH-m.FtA) {
		r.Tendency = cfg.Match.Tendency
	}
	if p.FtH == m.FtH && p.FtA == m.FtA {
		r.Exact = cfg.Match.Exact
	}
	if p.FtH+p.FtA == m.FtH+m.FtA {
		r.TotalGoals = cfg.Match.TotalGoals
	}
	if p.FtH-p.FtA == m.FtH-m.FtA {
		r.GoalDiff = cfg.Match.GoalDiff
	}
	if d := (p.FtH - p.FtA) - (m.FtH - m.FtA); d < 0 {
		r.GdDev = -d
	} else {
		r.GdDev = d
	}

	if m.Stage == "group" {
		return r
	}

	// Knockout: ET bonus only if the game went to ET (drawn at 90') and the
	// user also predicted a 90' draw.
	if cfg.Match.KoOtBonus && m.FtH == m.FtA && p.FtH == p.FtA {
		if m.EtH != 0 || m.EtA != 0 {
			if p.EtH == m.EtH && p.EtA == m.EtA {
				r.OtBonus += cfg.Match.Exact
			}
			if p.EtH+p.EtA == m.EtH+m.EtA {
				r.OtBonus += cfg.Match.TotalGoals
			}
			if p.EtH-p.EtA == m.EtH-m.EtA {
				r.OtBonus += cfg.Match.GoalDiff
			}
		}
	}
	if m.Advancer != "" && m.Advancer == p.Advancer {
		r.Advancer = cfg.Match.Advancer
	}
	return r
}

func scoreTip(cfg Config, match, tip *core.Record) tipComponents {
	return scoreValues(cfg,
		MatchResult{
			Stage:    match.GetString("stage"),
			FtH:      match.GetInt("ftHome"),
			FtA:      match.GetInt("ftAway"),
			EtH:      match.GetInt("etHome"),
			EtA:      match.GetInt("etAway"),
			Advancer: match.GetString("advancer"),
		},
		TipPrediction{
			FtH:      tip.GetInt("ftHome"),
			FtA:      tip.GetInt("ftAway"),
			EtH:      tip.GetInt("etHome"),
			EtA:      tip.GetInt("etAway"),
			Advancer: tip.GetString("advancer"),
		},
	)
}

// ---- Group standings (final, from finalized group matches) ----

type teamAgg struct {
	id                 string
	pts, gd, gf, games int
}

// finalGroups returns, for each fully-finished group, the ordered team ids
// (1st..4th) and collects the 12 third-placed teams for the best-third rank.
func finalGroups(app core.App) (order map[string][]string, thirds []teamAgg) {
	order = map[string][]string{}
	ms, _ := app.FindRecordsByFilter("matches",
		"stage = 'group' && finalizedAt != ''", "", 0, 0)
	groups := map[string]map[string]*teamAgg{}
	for _, m := range ms {
		g := m.GetString("groupLetter")
		if groups[g] == nil {
			groups[g] = map[string]*teamAgg{}
		}
		h, a := m.GetString("homeTeam"), m.GetString("awayTeam")
		hg, ag := m.GetInt("ftHome"), m.GetInt("ftAway")
		for _, id := range []string{h, a} {
			if groups[g][id] == nil {
				groups[g][id] = &teamAgg{id: id}
			}
		}
		H, A := groups[g][h], groups[g][a]
		H.games++
		A.games++
		H.gf += hg
		A.gf += ag
		H.gd += hg - ag
		A.gd += ag - hg
		switch {
		case hg > ag:
			H.pts += 3
		case ag > hg:
			A.pts += 3
		default:
			H.pts++
			A.pts++
		}
	}
	for g, tbl := range groups {
		if len(tbl) < 4 {
			continue
		}
		arr := make([]teamAgg, 0, 4)
		complete := true
		for _, v := range tbl {
			arr = append(arr, *v)
			if v.games < 3 {
				complete = false
			}
		}
		if !complete {
			continue
		}
		sortAggs(arr)
		ids := make([]string, len(arr))
		for i, v := range arr {
			ids[i] = v.id
		}
		order[g] = ids
		thirds = append(thirds, arr[2])
	}
	return order, thirds
}

func sortAggs(a []teamAgg) {
	sort.Slice(a, func(i, j int) bool {
		if a[i].pts != a[j].pts {
			return a[i].pts > a[j].pts
		}
		if a[i].gd != a[j].gd {
			return a[i].gd > a[j].gd
		}
		return a[i].gf > a[j].gf
	})
}

func bestThirdSet(thirds []teamAgg) map[string]bool {
	sortAggs(thirds)
	set := map[string]bool{}
	for i, t := range thirds {
		if i >= 8 {
			break
		}
		set[t.id] = true
	}
	return set
}

// ---- Forecast scoring ----

// actualRoundTeams maps stage -> set(teamId) of teams that actually reached
// that round, plus the actual champion.
func actualRoundTeams(app core.App) (map[string]map[string]bool, string) {
	res := map[string]map[string]bool{}
	champion := ""
	ms, _ := app.FindRecordsByFilter("matches", "stage != 'group'", "num", 0, 0)
	for _, m := range ms {
		st := m.GetString("stage")
		if res[st] == nil {
			res[st] = map[string]bool{}
		}
		for _, f := range []string{"homeTeam", "awayTeam"} {
			if id := m.GetString(f); id != "" {
				res[st][id] = true
			}
		}
		if st == "FINAL" && m.GetString("finalizedAt") != "" {
			champion = m.GetString("advancer")
		}
	}
	return res, champion
}

type fcResolver struct {
	order      map[string][]string
	thirdByNum map[int]string // R32 match num -> chosen third teamId
	bracket    map[string]string
	ko         map[int]*core.Record
}

// assignThirds slots the user's chosen best thirds ({groupLetter: teamId})
// into the 8 R32 third-slots: slots in match order, each filled by the
// lowest-letter chosen third its rule allows that isn't used yet. Identical
// to the frontend so Forecast knockout scoring agrees.
func assignThirds(koList []*core.Record, thirds map[string]string) map[int]string {
	type slot struct {
		num     int
		allowed []string
	}
	var slots []slot
	for _, mt := range koList {
		if mt.GetString("stage") != "R32" {
			continue
		}
		for _, lbl := range []string{mt.GetString("homeLabel"), mt.GetString("awayLabel")} {
			if strings.HasPrefix(lbl, "3") && strings.Contains(lbl, "/") {
				slots = append(slots, slot{
					num:     mt.GetInt("num"),
					allowed: strings.Split(strings.TrimPrefix(lbl, "3"), "/"),
				})
			}
		}
	}
	sort.Slice(slots, func(i, j int) bool { return slots[i].num < slots[j].num })

	chosen := make([]string, 0, len(thirds))
	for letter := range thirds {
		chosen = append(chosen, letter)
	}
	sort.Strings(chosen)

	used := map[string]bool{}
	out := map[int]string{}
	for _, s := range slots {
		for _, letter := range chosen {
			if used[letter] {
				continue
			}
			ok := false
			for _, a := range s.allowed {
				if a == letter {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
			out[s.num] = thirds[letter]
			used[letter] = true
			break
		}
	}
	return out
}

func (r *fcResolver) resolve(label string, forNum int, seen map[int]bool) string {
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
		src := r.ko[n]
		if src == nil || w == "" {
			return ""
		}
		h := r.resolve(src.GetString("homeLabel"), n, seen)
		a := r.resolve(src.GetString("awayLabel"), n, seen)
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

func koStableKey(m *core.Record) string {
	if n := m.GetInt("num"); n > 0 {
		return strconv.Itoa(n)
	}
	return m.GetString("stage")
}

type fcBreakdown struct {
	Groups   int `json:"groups"`
	Thirds   int `json:"thirds"`
	Knockout int `json:"knockout"`
	Champion int `json:"champion"`
}

func (b fcBreakdown) total() int { return b.Groups + b.Thirds + b.Knockout + b.Champion }

func scoreForecast(app core.App, cfg Config, fc *core.Record) (fcBreakdown, int) {
	var b fcBreakdown

	var order map[string][]string
	_ = fc.UnmarshalJSONField("groupOrder", &order)
	var thirds map[string]string
	_ = fc.UnmarshalJSONField("thirdQualifiers", &thirds)
	var bracket map[string]string
	_ = fc.UnmarshalJSONField("bracket", &bracket)

	actualOrder, thirdAggs := finalGroups(app)
	for g, actual := range actualOrder {
		pred := order[g]
		allCorrect := len(pred) == 4
		for i := 0; i < 4 && i < len(actual); i++ {
			if i < len(pred) && pred[i] == actual[i] {
				b.Groups += cfg.Forecast.GroupPosition
			} else {
				allCorrect = false
			}
		}
		if allCorrect {
			b.Groups += cfg.Forecast.PerfectGroupBonus
		}
	}

	if len(thirdAggs) >= 12 { // all groups done -> best-8 fixed
		best := bestThirdSet(thirdAggs)
		for _, tid := range thirds {
			if best[tid] {
				b.Thirds += cfg.Forecast.ThirdQualifier
			}
		}
	}

	actualRounds, actualChamp := actualRoundTeams(app)
	koList, _ := app.FindRecordsByFilter("matches", "stage != 'group'", "num", 0, 0)
	koByNum := map[int]*core.Record{}
	for _, m := range koList {
		if n := m.GetInt("num"); n > 0 {
			koByNum[n] = m
		}
	}
	r := &fcResolver{
		order:      order,
		thirdByNum: assignThirds(koList, thirds),
		bracket:    bracket,
		ko:         koByNum,
	}

	for _, m := range koList {
		st := m.GetString("stage")
		w := cfg.Forecast.Round[st]
		if w == 0 {
			continue
		}
		predHome := r.resolve(m.GetString("homeLabel"), m.GetInt("num"), map[int]bool{})
		predAway := r.resolve(m.GetString("awayLabel"), m.GetInt("num"), map[int]bool{})
		for _, pid := range []string{predHome, predAway} {
			if pid != "" && actualRounds[st] != nil && actualRounds[st][pid] {
				b.Knockout += w
			}
		}
	}

	if actualChamp != "" {
		var champKey string
		for _, m := range koList {
			if m.GetString("stage") == "FINAL" {
				champKey = koStableKey(m)
			}
		}
		if champKey != "" && bracket[champKey] == actualChamp {
			b.Champion += cfg.Forecast.Round["CHAMPION"]
		}
	}

	return b, b.total()
}
