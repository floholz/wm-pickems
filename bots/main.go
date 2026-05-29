// Command wm-pickems-bot is a standalone side project: it logs in to a
// wm-pickems deployment as a bot user (Claude, for v1) and submits a Forecast
// and per-match Tips through the public REST API — playing by the same
// server-side locks as any human. Run it once (cron) or with --loop.
//
// Configuration is via environment:
//
//	WMP_BASE_URL    base URL of the app   (default http://127.0.0.1:8090)
//	BOT_EMAIL       the bot account's email      (required)
//	BOT_PASSWORD    the bot account's password   (required)
//	ANTHROPIC_API_KEY   Anthropic API key        (required; read by the SDK)
//	CLAUDE_MODEL    model id              (default claude-opus-4-8)
//	BOT_LEAGUE_CODE optional invite code to auto-join on start
//
// Provision the bot once in the PocketBase admin: create the user, set
// role=bot / botKind=claude, and add it to your leagues (or set BOT_LEAGUE_CODE).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"
)

type config struct {
	baseURL    string
	email      string
	password   string
	model      string
	leagueCode string
}

func loadConfig() (config, error) {
	c := config{
		baseURL:    envOr("WMP_BASE_URL", "http://127.0.0.1:8090"),
		email:      os.Getenv("BOT_EMAIL"),
		password:   os.Getenv("BOT_PASSWORD"),
		model:      envOr("CLAUDE_MODEL", "claude-opus-4-8"),
		leagueCode: os.Getenv("BOT_LEAGUE_CODE"),
	}
	if c.email == "" || c.password == "" {
		return c, fmt.Errorf("BOT_EMAIL and BOT_PASSWORD are required")
	}
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		return c, fmt.Errorf("ANTHROPIC_API_KEY is required")
	}
	return c, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	loop := flag.Bool("loop", false, "keep running on an interval instead of once")
	interval := flag.Duration("interval", time.Hour, "interval between runs in --loop mode")
	flag.Parse()

	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}

	run := func() {
		if err := runOnce(cfg); err != nil {
			log.Printf("run error: %v", err)
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

	c := NewClient(cfg.baseURL)
	if err := c.Login(ctx, cfg.email, cfg.password); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	log.Printf("logged in as bot user %s", c.UserID)

	if cfg.leagueCode != "" {
		if err := c.JoinLeague(ctx, cfg.leagueCode); err != nil {
			log.Printf("join league %q: %v", cfg.leagueCode, err)
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

	brain := NewBrain(cfg.model, buildReference(teams, structure))

	if err := ensureForecast(ctx, c, brain, structure, teamName); err != nil {
		log.Printf("forecast: %v", err)
	}
	if err := submitTips(ctx, c, brain, matches, teamName); err != nil {
		log.Printf("tips: %v", err)
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

func ensureForecast(ctx context.Context, c *Client, brain *Brain, s *Structure, teamName map[string]string) error {
	id, err := c.MyForecast(ctx)
	if err != nil {
		return err
	}
	if id != "" {
		return nil // already forecast; one-time only
	}
	if s.Locked {
		log.Printf("forecast locked (tournament started) and none submitted — skipping")
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

	rawOrder, rawThirds, err := brain.PredictGroups(ctx, picks)
	if err != nil {
		return fmt.Errorf("predict groups: %w", err)
	}
	order := repairOrder(member, rawOrder)
	thirds := chooseThirds(order, rawThirds)
	log.Printf("forecast: groups ordered, %d best-thirds chosen", len(thirds))

	bracket, err := BuildForecast(ctx, s, order, thirds,
		func(id string) string { return teamName[id] }, brain.PredictWinners)
	if err != nil {
		return fmt.Errorf("build bracket: %w", err)
	}
	if err := c.SaveForecast(ctx, order, thirds, bracket); err != nil {
		return fmt.Errorf("save: %w", err)
	}
	log.Printf("forecast saved (%d bracket picks)", len(bracket))
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

func submitTips(ctx context.Context, c *Client, brain *Brain, matches []Match, teamName map[string]string) error {
	now, err := c.Now(ctx)
	if err != nil {
		return fmt.Errorf("now: %w", err)
	}
	existing, err := c.MyTips(ctx)
	if err != nil {
		return fmt.Errorf("my tips: %w", err)
	}
	tipped := map[string]bool{}
	for _, t := range existing {
		tipped[t.Match] = true
	}

	var targets []tipTarget
	for _, m := range matches {
		if tipped[m.ID] {
			continue
		}
		ko, ok := kickoffTime(m.Kickoff)
		if !ok || !now.Before(ko) {
			continue // already locked (or unparseable)
		}
		if m.Stage != "group" && (m.HomeTeam == "" || m.AwayTeam == "") {
			continue // knockout matchup not resolved yet
		}
		home, away := labelFor(m.HomeTeam, m.HomeLabel, teamName), labelFor(m.AwayTeam, m.AwayLabel, teamName)
		targets = append(targets, tipTarget{
			MatchID: m.ID, Stage: m.Stage, Home: home, Away: away, Kickoff: m.Kickoff,
		})
	}
	if len(targets) == 0 {
		log.Printf("tips: nothing new to tip")
		return nil
	}
	log.Printf("tips: predicting %d match(es)", len(targets))

	created := 0
	for start := 0; start < len(targets); start += tipBatch {
		end := min(start+tipBatch, len(targets))
		batch := targets[start:end]
		scores, err := brain.PredictTips(ctx, batch)
		if err != nil {
			return fmt.Errorf("predict tips: %w", err)
		}
		for _, t := range batch {
			s, ok := scores[t.MatchID]
			if !ok {
				continue
			}
			if err := c.CreateTip(ctx, t.MatchID, s.Home, s.Away); err != nil {
				log.Printf("tip %s: %v", t.MatchID, err)
				continue
			}
			created++
		}
	}
	log.Printf("tips: created %d", created)
	return nil
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
