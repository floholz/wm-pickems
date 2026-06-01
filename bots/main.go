// Command wm-pickems-bot is a standalone side project: it logs in to a
// wm-pickems deployment as a bot user and submits a Forecast and per-match
// Tips through the public REST API — playing by the same server-side locks as
// any human. Run it once (cron) or with --loop.
//
// Configuration is via environment:
//
//	WMP_BASE_URL    base URL of the app   (default http://127.0.0.1:8090)
//	BOT_EMAIL       the bot account's email      (required)
//	BOT_PASSWORD    the bot account's password   (required)
//	BOT_KIND        strategy: algo (default) | claude
//	ANTHROPIC_API_KEY   Anthropic API key   (required for BOT_KIND=claude)
//	CLAUDE_MODEL    model id (claude only) (default claude-opus-4-8)
//	BOT_LEAGUE_CODE optional invite code to auto-join on start
//	BOT_RATIONALE   ask for + log a one-line reason per prediction (claude only)
//	LOG_FORMAT      log output: text (default) | json
//
// Provision the bot once in the PocketBase admin: create the user, set
// role=bot / botKind to match BOT_KIND, and add it to your leagues (or set
// BOT_LEAGUE_CODE).
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"
)

type config struct {
	baseURL    string
	email      string
	password   string
	kind       string // "claude" (default) | "algo"
	model      string
	leagueCode string
	rationale  bool // ask the model for + log a one-line reason per prediction (claude only)
}

func loadConfig() (config, error) {
	c := config{
		baseURL:    envOr("WMP_BASE_URL", "http://127.0.0.1:8090"),
		email:      os.Getenv("BOT_EMAIL"),
		password:   os.Getenv("BOT_PASSWORD"),
		kind:       strings.ToLower(envOr("BOT_KIND", "algo")),
		model:      envOr("CLAUDE_MODEL", "claude-opus-4-8"),
		leagueCode: os.Getenv("BOT_LEAGUE_CODE"),
		rationale:  envBool("BOT_RATIONALE"),
	}
	if c.email == "" || c.password == "" {
		return c, fmt.Errorf("BOT_EMAIL and BOT_PASSWORD are required")
	}
	switch c.kind {
	case "claude":
		if os.Getenv("ANTHROPIC_API_KEY") == "" {
			return c, fmt.Errorf("ANTHROPIC_API_KEY is required for BOT_KIND=claude")
		}
	case "algo":
		// No API key needed — fully deterministic.
	default:
		return c, fmt.Errorf("unknown BOT_KIND %q (want claude or algo)", c.kind)
	}
	return c, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string) bool {
	switch strings.ToLower(os.Getenv(key)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// setupLogger configures the global slog logger from LOG_FORMAT: "text"
// (default, human-readable for local dev) or "json" (structured, for log
// shippers like Grafana Alloy/Loki). Everything is written to stdout.
func setupLogger() {
	var h slog.Handler
	if strings.ToLower(os.Getenv("LOG_FORMAT")) == "json" {
		h = slog.NewJSONHandler(os.Stdout, nil)
	} else {
		h = slog.NewTextHandler(os.Stdout, nil)
	}
	slog.SetDefault(slog.New(h))
}

func main() {
	loop := flag.Bool("loop", false, "keep running on an interval instead of once")
	interval := flag.Duration("interval", time.Hour, "interval between runs in --loop mode")
	flag.Parse()

	setupLogger()

	cfg, err := loadConfig()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}
	// bot_kind is process-constant — attach it to every line.
	slog.SetDefault(slog.Default().With("bot_kind", cfg.kind))

	run := func() {
		if err := runOnce(cfg); err != nil {
			slog.Error("run failed", "err", err)
		}
	}
	run()
	if !*loop {
		return
	}
	for range time.Tick(*interval) {
		run()
	}
}

func runOnce(cfg config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// run_id correlates every event from this run; bot_kind is already on the default logger.
	runID := time.Now().UTC().Format("20060102T150405.000Z")
	log := slog.With("run_id", runID)

	c := NewClient(cfg.baseURL)
	if err := c.Login(ctx, cfg.email, cfg.password); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	log.Info("logged in", "user_id", c.UserID)

	if cfg.leagueCode != "" {
		if err := c.JoinLeague(ctx, cfg.leagueCode); err != nil {
			log.Warn("join league failed", "code", cfg.leagueCode, "err", err)
		}
	}

	teams, err := c.Teams(ctx)
	if err != nil {
		return fmt.Errorf("teams: %w", err)
	}
	teamName := map[string]string{}
	for _, t := range teams {
		teamName[t.ID] = t.Name
	}

	matches, err := c.Matches(ctx)
	if err != nil {
		return fmt.Errorf("matches: %w", err)
	}
	structure, err := c.Structure(ctx)
	if err != nil {
		return fmt.Errorf("structure: %w", err)
	}

	// Finished matches are the feedback signal: the algo Elo-adjusts its
	// ratings from them, Claude gets them as prompt context.
	var finished []Match
	for _, m := range matches {
		if m.Finished() {
			finished = append(finished, m)
		}
	}

	var predictor Predictor
	switch cfg.kind {
	case "algo":
		predictor = NewAlgoBrain(teams, finished)
		log.Info("strategy selected", "strategy", "algo", "results_applied", len(finished))
	default:
		ctxText := buildResultsText(finished, teamName)
		if st := buildStandings(finished, structure, teamName); st != "" {
			if ctxText != "" {
				ctxText += "\n"
			}
			ctxText += st
		}
		predictor = NewBrain(cfg.model, buildReference(teams, structure), ctxText, cfg.rationale, log)
		log.Info("strategy selected", "strategy", "claude", "model", cfg.model, "results_in_context", len(finished))
	}

	if err := ensureForecast(ctx, c, predictor, structure, teamName, log); err != nil {
		log.Error("forecast failed", "err", err)
	}
	if err := submitTips(ctx, c, predictor, matches, teamName, log); err != nil {
		log.Error("tips failed", "err", err)
	}
	return nil
}

// buildReference is the static, cached system prompt: who the bot is plus the
// full tournament reference (teams, group memberships, knockout skeleton). It is
// byte-identical across every prediction call, so it serves as a prompt-cache
// prefix.
func buildReference(teams []Team, s *Structure) string {
	var sb strings.Builder
	sb.WriteString("You are Claude, competing as a player in a FIFA World Cup 2026 prediction game against human friends. ")
	sb.WriteString("Make your best, realistic predictions using your football knowledge. Always answer with exactly the JSON the task asks for and nothing else.\n\n")

	sb.WriteString("How to predict: weigh each team's historical strength, squad pedigree, and FIFA ranking, plus — once results exist — the in-tournament form and group standings shown in the task. ")
	sb.WriteString("You have NO live squad, lineup, injury, or suspension data; do not invent or assume specific player availability — base predictions on team strength and observed results. ")
	sb.WriteString("Fixture home/away positions are nominal slots, not venues: do not infer a home advantage from them. The only genuine host advantage belongs to the host nations (USA, Canada, Mexico). ")
	sb.WriteString("Group matchdays differ in character — a side already through may rotate, a side needing a result will chase it — so factor the matchday and current standings into group games.\n\n")

	sb.WriteString("TEAMS (id — name [FIFA code]):\n")
	for _, t := range teams {
		fmt.Fprintf(&sb, "  %s — %s [%s]\n", t.ID, t.Name, t.FifaCode)
	}
	sb.WriteString("\nGROUPS (membership):\n")
	for _, g := range s.Groups {
		fmt.Fprintf(&sb, "  Group %s: %s\n", g.Letter, strings.Join(g.Teams, ", "))
	}
	sb.WriteString("\nKNOCKOUT SKELETON (match num: home-label vs away-label; labels like 1A=winner group A, 2B=runner-up B, 3X/Y=a best third, W73=winner of match 73, L101=loser of match 101):\n")
	ko := append([]struct {
		Num       int    `json:"num"`
		Stage     string `json:"stage"`
		HomeLabel string `json:"homeLabel"`
		AwayLabel string `json:"awayLabel"`
	}{}, s.Knockout...)
	sort.Slice(ko, func(i, j int) bool { return ko[i].Num < ko[j].Num })
	for _, m := range ko {
		fmt.Fprintf(&sb, "  %d [%s]: %s vs %s\n", m.Num, m.Stage, m.HomeLabel, m.AwayLabel)
	}
	return sb.String()
}

func ensureForecast(ctx context.Context, c *Client, pred Predictor, s *Structure, teamName map[string]string, log *slog.Logger) error {
	id, err := c.MyForecast(ctx)
	if err != nil {
		return err
	}
	if id != "" {
		return nil // already forecast; one-time only
	}
	if s.Locked {
		log.Warn("forecast locked, none submitted — skipping")
		return nil
	}

	// Membership per group, in the structure's order.
	picks := make([]groupPick, 0, len(s.Groups))
	member := map[string][]string{}
	for _, g := range s.Groups {
		member[g.Letter] = g.Teams
		teamsND := make([]nameID, 0, len(g.Teams))
		for _, id := range g.Teams {
			teamsND = append(teamsND, nameID{ID: id, Name: teamName[id]})
		}
		picks = append(picks, groupPick{Letter: g.Letter, Teams: teamsND})
	}

	rawOrder, rawThirds, err := pred.PredictGroups(ctx, picks)
	if err != nil {
		return fmt.Errorf("predict groups: %w", err)
	}
	order := repairOrder(member, rawOrder)
	thirds := chooseThirds(order, rawThirds)
	log.Info("forecast groups predicted", "best_thirds", len(thirds))

	bracket, err := BuildForecast(ctx, s, order, thirds,
		func(id string) string { return teamName[id] }, pred.PredictWinners)
	if err != nil {
		return fmt.Errorf("build bracket: %w", err)
	}
	if err := c.SaveForecast(ctx, order, thirds, bracket); err != nil {
		return fmt.Errorf("save: %w", err)
	}
	log.Info("forecast saved", "bracket_picks", len(bracket))
	return nil
}

// repairOrder keeps only valid group members in the model's ordering, dedupes,
// and appends any missing members so every group has exactly its four teams.
func repairOrder(member map[string][]string, raw map[string][]string) map[string][]string {
	out := map[string][]string{}
	for letter, members := range member {
		set := map[string]bool{}
		for _, id := range members {
			set[id] = true
		}
		seen := map[string]bool{}
		var ordered []string
		for _, id := range raw[letter] {
			if set[id] && !seen[id] {
				seen[id] = true
				ordered = append(ordered, id)
			}
		}
		for _, id := range members { // fill any missing in membership order
			if !seen[id] {
				ordered = append(ordered, id)
			}
		}
		out[letter] = ordered
	}
	return out
}

// chooseThirds turns the model's chosen group letters into the
// {letter: thirdPlaceTeamId} map the scorer expects, forcing exactly 8 valid,
// distinct groups (truncating or topping up deterministically).
func chooseThirds(order map[string][]string, raw []string) map[string]string {
	seen := map[string]bool{}
	var chosen []string
	for _, l := range raw {
		l = strings.ToUpper(strings.TrimSpace(l))
		if _, ok := order[l]; ok && !seen[l] {
			seen[l] = true
			chosen = append(chosen, l)
		}
	}
	if len(chosen) > 8 {
		chosen = chosen[:8]
	}
	if len(chosen) < 8 { // top up with other groups, alphabetically
		letters := make([]string, 0, len(order))
		for l := range order {
			letters = append(letters, l)
		}
		sort.Strings(letters)
		for _, l := range letters {
			if len(chosen) == 8 {
				break
			}
			if !seen[l] {
				seen[l] = true
				chosen = append(chosen, l)
			}
		}
	}
	thirds := map[string]string{}
	for _, l := range chosen {
		if o := order[l]; len(o) >= 3 {
			thirds[l] = o[2]
		}
	}
	return thirds
}

const tipBatch = 40

func submitTips(ctx context.Context, c *Client, pred Predictor, matches []Match, teamName map[string]string, log *slog.Logger) error {
	now, err := c.Now(ctx)
	if err != nil {
		return fmt.Errorf("now: %w", err)
	}
	existing, err := c.MyTips(ctx)
	if err != nil {
		return fmt.Errorf("my tips: %w", err)
	}
	tipByMatch := make(map[string]Tip, len(existing))
	var lastTipped time.Time
	for _, t := range existing {
		tipByMatch[t.Match] = t
		if u, ok := kickoffTime(t.Updated); ok && u.After(lastTipped) {
			lastTipped = u
		}
	}
	// "New info" = a result has been finalized since we last tipped. When false,
	// already-tipped matches are left alone (no churn / no needless API calls);
	// only brand-new open matches get an initial tip.
	var lastResult time.Time
	for _, m := range matches {
		if !m.Finished() {
			continue
		}
		if f, ok := kickoffTime(m.FinalizedAt); ok && f.After(lastResult) {
			lastResult = f
		}
	}
	hasNewInfo := lastResult.After(lastTipped)

	matchday := groupMatchdays(matches)
	var targets []tipTarget
	for _, m := range matches {
		ko, ok := kickoffTime(m.Kickoff)
		if !ok || !now.Before(ko) {
			continue // already locked (or unparseable) — can't tip/edit
		}
		if m.Stage != "group" && (m.HomeTeam == "" || m.AwayTeam == "") {
			continue // knockout matchup not resolved yet
		}
		if _, already := tipByMatch[m.ID]; already && !hasNewInfo {
			continue // already tipped and nothing new to reconsider
		}
		targets = append(targets, tipTarget{
			MatchID: m.ID, Stage: m.Stage,
			Home:   labelFor(m.HomeTeam, m.HomeLabel, teamName),
			Away:   labelFor(m.AwayTeam, m.AwayLabel, teamName),
			HomeID: m.HomeTeam, AwayID: m.AwayTeam,
			Kickoff:  m.Kickoff,
			Matchday: matchday[m.ID],
		})
	}
	if len(targets) == 0 {
		log.Info("tips: nothing to do")
		return nil
	}
	log.Info("tips: evaluating open matches", "count", len(targets), "has_new_info", hasNewInfo)

	created, updated := 0, 0
	for start := 0; start < len(targets); start += tipBatch {
		end := min(start+tipBatch, len(targets))
		batch := targets[start:end]
		outcomes, err := pred.PredictTips(ctx, batch)
		if err != nil {
			return fmt.Errorf("predict tips: %w", err)
		}
		for _, t := range batch {
			o, ok := outcomes[t.MatchID]
			if !ok {
				continue
			}
			// Pure shared decision layer: turn the brain's distribution into the
			// points-maximizing concrete scoreline under the (global default) scoring.
			s := selectTip(o, t.Stage, defaultWeights)
			if ex, exists := tipByMatch[t.MatchID]; exists {
				if ex.FtHome == s.Home && ex.FtAway == s.Away {
					continue // prediction unchanged
				}
				if err := c.UpdateTip(ctx, ex.ID, s.Home, s.Away); err != nil {
					log.Warn("update tip failed", "match", t.MatchID, "err", err)
					continue
				}
				updated++
				logTip(log, "revised", t, fmt.Sprintf("%d-%d", ex.FtHome, ex.FtAway), s, "new_result", o.Rationale)
			} else {
				if err := c.CreateTip(ctx, t.MatchID, s.Home, s.Away); err != nil {
					log.Warn("create tip failed", "match", t.MatchID, "err", err)
					continue
				}
				created++
				logTip(log, "created", t, "", s, "first_tip", o.Rationale)
			}
		}
	}
	log.Info("tips submitted", "created", created, "revised", updated)
	return nil
}

// logTip emits one structured "tip" event. old is the previous scoreline ("" for
// a first tip); rationale is included only when the brain supplied one.
func logTip(log *slog.Logger, action string, t tipTarget, old string, s Scoreline, trigger, rationale string) {
	attrs := []any{
		"action", action,
		"match", t.MatchID,
		"home_team", t.Home,
		"away_team", t.Away,
		"new", fmt.Sprintf("%d-%d", s.Home, s.Away),
		"trigger", trigger,
	}
	if old != "" {
		attrs = append(attrs, "old", old)
	}
	if rationale != "" {
		attrs = append(attrs, "rationale", rationale)
	}
	log.Info("tip", attrs...)
}

// buildResultsText is the Claude bot's feedback context: a compact, chronological
// list of finished matches ("Brazil 2-1 Serbia"). Empty before any result.
func buildResultsText(finished []Match, teamName map[string]string) string {
	if len(finished) == 0 {
		return ""
	}
	ms := append([]Match(nil), finished...)
	sort.SliceStable(ms, func(i, j int) bool { return ms[i].Kickoff < ms[j].Kickoff })
	var sb strings.Builder
	sb.WriteString("Results so far (oldest first):\n")
	for _, m := range ms {
		h, a := m.FtHome, m.FtAway
		if m.EtHome > 0 || m.EtAway > 0 {
			h, a = m.EtHome, m.EtAway
		}
		fmt.Fprintf(&sb, "[%s] %s %d-%d %s\n", m.Stage, teamName[m.HomeTeam], h, a, teamName[m.AwayTeam])
	}
	return sb.String()
}

// buildStandings renders a compact current group table (points, goal diff, goals
// for, played) per group, computed from finished group matches — the in-context
// "where each group stands" signal. Returns "" before any group result. This is
// derived from data the bot already pulls; no full FIFA tiebreaker resolution,
// just enough for the model to see who's through, chasing, or out.
func buildStandings(finished []Match, s *Structure, teamName map[string]string) string {
	type stat struct{ pts, gf, ga, pld int }
	byGroup := make(map[string]map[string]*stat, len(s.Groups))
	for _, g := range s.Groups {
		byGroup[g.Letter] = map[string]*stat{}
		for _, id := range g.Teams {
			byGroup[g.Letter][id] = &stat{}
		}
	}
	any := false
	for _, m := range finished {
		if m.Stage != "group" {
			continue
		}
		grp := byGroup[m.GroupLetter]
		h, a := grp[m.HomeTeam], grp[m.AwayTeam]
		if h == nil || a == nil {
			continue
		}
		any = true
		h.pld, a.pld = h.pld+1, a.pld+1
		h.gf, h.ga = h.gf+m.FtHome, h.ga+m.FtAway
		a.gf, a.ga = a.gf+m.FtAway, a.ga+m.FtHome
		switch {
		case m.FtHome > m.FtAway:
			h.pts += 3
		case m.FtAway > m.FtHome:
			a.pts += 3
		default:
			h.pts, a.pts = h.pts+1, a.pts+1
		}
	}
	if !any {
		return ""
	}
	letters := make([]string, 0, len(byGroup))
	for l := range byGroup {
		letters = append(letters, l)
	}
	sort.Strings(letters)
	var sb strings.Builder
	sb.WriteString("Current group standings (played so far):\n")
	for _, l := range letters {
		type row struct {
			id string
			s  *stat
		}
		var rows []row
		played := false
		for id, st := range byGroup[l] {
			rows = append(rows, row{id, st})
			if st.pld > 0 {
				played = true
			}
		}
		if !played {
			continue
		}
		sort.Slice(rows, func(i, j int) bool {
			a, b := rows[i].s, rows[j].s
			if a.pts != b.pts {
				return a.pts > b.pts
			}
			if da, db := a.gf-a.ga, b.gf-b.ga; da != db {
				return da > db
			}
			if a.gf != b.gf {
				return a.gf > b.gf
			}
			return rows[i].id < rows[j].id
		})
		parts := make([]string, len(rows))
		for i, r := range rows {
			parts[i] = fmt.Sprintf("%s %dpts (%+d, %dgf, %dpld)", teamName[r.id], r.s.pts, r.s.gf-r.s.ga, r.s.gf, r.s.pld)
		}
		fmt.Fprintf(&sb, "Group %s: %s\n", l, strings.Join(parts, "; "))
	}
	return sb.String()
}

// groupMatchdays derives the matchday (1–3) of each group match from kickoff order
// within its group — a group's six matches cluster into three matchday pairs
// (matchday-3 games kick off simultaneously), so kickoff-ordering is robust.
func groupMatchdays(matches []Match) map[string]int {
	byGroup := map[string][]Match{}
	for _, m := range matches {
		if m.Stage == "group" {
			byGroup[m.GroupLetter] = append(byGroup[m.GroupLetter], m)
		}
	}
	out := map[string]int{}
	for _, ms := range byGroup {
		sort.SliceStable(ms, func(i, j int) bool { return ms[i].Kickoff < ms[j].Kickoff })
		for i, m := range ms {
			out[m.ID] = i/2 + 1
		}
	}
	return out
}

// labelFor names a side:: the resolved team name if known, else the placeholder
// label (so group matches always read as real teams).
func labelFor(teamID, label string, teamName map[string]string) string {
	if teamID != "" {
		if n := teamName[teamID]; n != "" {
			return n
		}
	}
	return label
}

func kickoffTime(s string) (time.Time, bool) {
	for _, layout := range []string{
		"2006-01-02 15:04:05.000Z",
		"2006-01-02 15:04:05Z",
		time.RFC3339,
		"2006-01-02 15:04:05.000Z07:00",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}
