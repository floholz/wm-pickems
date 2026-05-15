// Package sync keeps the matches collection up to date: a cron job pulls
// results from API-Football (one request per run), a superuser endpoint
// forces a refresh, and another superuser endpoint applies manual results
// when the provider is wrong or no API key is configured.
package sync

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/floholz/wm-pickems/internal/football"
)

// cronExpr runs the sync every 30 minutes => max 48 requests/day, comfortably
// under the API-Football free tier (100/day).
const cronExpr = "*/30 * * * *"

// nameAliases maps API-Football names that differ from the openfootball seed
// names to the seeded team name.
var nameAliases = map[string]string{
	football.NormalizeName("Korea Republic"): football.NormalizeName("South Korea"),
	football.NormalizeName("Czechia"):        football.NormalizeName("Czech Republic"),
	football.NormalizeName("USA"):            football.NormalizeName("United States"),
	football.NormalizeName("IR Iran"):        football.NormalizeName("Iran"),
}

func canonName(s string) string {
	n := football.NormalizeName(s)
	if a, ok := nameAliases[n]; ok {
		return a
	}
	return n
}

// pickProvider decides the live-results source: API-Football when its key can
// actually reach WC2026 (a paid plan — free can't), otherwise the free
// openfootball JSON. RESULTS_SOURCE=apifootball|openfootball forces it.
// Returns a label and a sync function (nil = none / manual-only).
func pickProvider(app core.App) (string, func(context.Context) error) {
	key := os.Getenv("API_FOOTBALL_KEY")
	mode := os.Getenv("RESULTS_SOURCE")

	apiFn := func(ctx context.Context) error {
		return SyncOnce(ctx, app, football.New(key))
	}
	ofFn := func(ctx context.Context) error {
		return openfootballSync(ctx, app)
	}

	if mode == "openfootball" {
		return "openfootball", ofFn
	}
	if mode == "apifootball" {
		if key == "" {
			return "", nil
		}
		return "api-football", apiFn
	}
	// auto: prefer API-Football only if the key can actually fetch 2026.
	if key != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if fx, err := football.New(key).Fixtures(ctx); err == nil && len(fx) > 0 {
			return "api-football", apiFn
		}
		log.Printf("[sync] API-Football key can't reach WC2026 (free plan?) — using openfootball")
	}
	return "openfootball", ofFn
}

// Register wires the live-results cron + manual override endpoints.
// Called from the OnServe hook.
func Register(app core.App, se *core.ServeEvent) {
	source, run := pickProvider(app)

	if run != nil {
		app.Cron().MustAdd("results-sync", cronExpr, func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := run(ctx); err != nil {
				log.Printf("[sync] %v", err)
			}
		})
		log.Printf("[sync] auto-sync enabled via %s (%s)", source, cronExpr)
	} else {
		log.Printf("[sync] no results source — manual override only")
	}

	// Force a sync now (superuser).
	se.Router.POST("/api/sync/refresh", func(e *core.RequestEvent) error {
		if run == nil {
			return e.JSON(400, map[string]string{"error": "no results source configured"})
		}
		ctx, cancel := context.WithTimeout(e.Request.Context(), 30*time.Second)
		defer cancel()
		if err := run(ctx); err != nil {
			return e.JSON(500, map[string]string{"error": err.Error()})
		}
		return e.JSON(200, map[string]string{"status": "ok", "source": source})
	}).Bind(apis.RequireSuperuserAuth())

	// Manual result override (superuser). Body: ftHome,ftAway,etHome,etAway,
	// penHome,penAway (ints, et/pen optional) and status.
	se.Router.POST("/api/admin/matches/{id}/result", func(e *core.RequestEvent) error {
		id := e.Request.PathValue("id")
		rec, err := app.FindRecordById("matches", id)
		if err != nil {
			return e.JSON(404, map[string]string{"error": "match not found"})
		}
		var body struct {
			FTHome, FTAway   *int
			ETHome, ETAway   *int
			PenHome, PenAway *int
			Status           string
		}
		if err := e.BindBody(&body); err != nil {
			return e.JSON(400, map[string]string{"error": err.Error()})
		}
		applyResult(rec, body.Status, body.FTHome, body.FTAway, body.ETHome, body.ETAway, body.PenHome, body.PenAway)
		if err := app.Save(rec); err != nil {
			return e.JSON(500, map[string]string{"error": err.Error()})
		}
		if err := ResolveBracket(app); err != nil {
			log.Printf("[sync] resolve after manual override: %v", err)
		}
		return e.JSON(200, map[string]any{"status": "ok", "id": rec.Id})
	}).Bind(apis.RequireSuperuserAuth())
}

// SyncOnce pulls all fixtures once and updates matched records.
func SyncOnce(ctx context.Context, app core.App, client *football.Client) error {
	fixtures, err := client.Fixtures(ctx)
	if err != nil {
		return fmt.Errorf("fetch fixtures: %w", err)
	}

	matches, err := app.FindRecordsByFilter("matches", "id != ''", "kickoff", 0, 0)
	if err != nil {
		return fmt.Errorf("load matches: %w", err)
	}

	// Index our matches by the normalized team-name pair (group stage) so we
	// can line them up with provider fixtures regardless of fixture ids.
	teamName := map[string]string{} // teamId -> normalized name
	teams, _ := app.FindRecordsByFilter("teams", "id != ''", "", 0, 0)
	for _, t := range teams {
		teamName[t.Id] = canonName(t.GetString("name"))
	}

	byPair := map[string]*core.Record{}
	for _, mrec := range matches {
		h := teamName[mrec.GetString("homeTeam")]
		a := teamName[mrec.GetString("awayTeam")]
		if h != "" && a != "" {
			byPair[h+"|"+a] = mrec
		}
	}

	updated := 0
	for _, f := range fixtures {
		key := canonName(f.HomeName) + "|" + canonName(f.AwayName)
		rec, ok := byPair[key]
		if !ok {
			// KO matches resolve via ResolveBracket; unmatched group names
			// usually mean an alias is missing — logged, not fatal.
			continue
		}
		status := "scheduled"
		switch {
		case f.Finished():
			status = "finished"
		case f.Live():
			status = "live"
		}
		// API `score.extratime` is the ET-only delta; our model (and Tips /
		// scoring) use the cumulative after-120 score, which is exactly the
		// provider `goals` field once a match has gone to extra time.
		var etH, etA *int
		if f.ETHome != nil || f.ETAway != nil {
			etH, etA = f.HomeGoals, f.AwayGoals
		}
		applyResult(rec, status, f.FTHome, f.FTAway, etH, etA, f.PenHome, f.PenAway)
		if app.Save(rec) == nil {
			updated++
		}
	}

	if err := ResolveBracket(app); err != nil {
		log.Printf("[sync] resolve bracket: %v", err)
	}
	log.Printf("[sync] fixtures=%d updated=%d", len(fixtures), updated)
	return nil
}

// APICheck is a dev diagnostic: fetch a season's fixtures from API-Football
// and report parse health, team-name mapping coverage against our seed, how
// many of our match rows resolve, and the status / ET / penalty distribution
// (point it at a finished season like 2022 to validate the results path).
func APICheck(ctx context.Context, app core.App, client *football.Client, yr int) (map[string]any, error) {
	fixtures, err := client.FixturesForSeason(ctx, yr)
	if err != nil {
		return nil, err
	}

	teams, _ := app.FindRecordsByFilter("teams", "id != ''", "", 0, 0)
	seedCanon := map[string]string{} // canonName -> seeded display name
	teamName := map[string]string{}  // teamId -> canonName
	for _, t := range teams {
		c := canonName(t.GetString("name"))
		seedCanon[c] = t.GetString("name")
		teamName[t.Id] = c
	}

	matches, _ := app.FindRecordsByFilter("matches", "id != ''", "kickoff", 0, 0)
	byPair := map[string]*core.Record{}
	for _, m := range matches {
		h, a := teamName[m.GetString("homeTeam")], teamName[m.GetString("awayTeam")]
		if h != "" && a != "" {
			byPair[h+"|"+a] = m
		}
	}

	statusHist := map[string]int{}
	unmapped := map[string]bool{}
	matchedRows := map[string]bool{}
	etCount, penCount := 0, 0
	var sample []map[string]any

	for _, f := range fixtures {
		statusHist[f.Status]++
		for _, nm := range []string{f.HomeName, f.AwayName} {
			if _, ok := seedCanon[canonName(nm)]; !ok {
				unmapped[nm] = true
			}
		}
		if rec, ok := byPair[canonName(f.HomeName)+"|"+canonName(f.AwayName)]; ok {
			matchedRows[rec.Id] = true
		}
		if f.ETHome != nil || f.ETAway != nil {
			etCount++
		}
		if f.PenHome != nil || f.PenAway != nil {
			penCount++
		}
		// Prefer extra-time / penalty fixtures in the sample — that's the
		// path most worth eyeballing.
		if (f.ETHome != nil || f.PenHome != nil) && len(sample) < 6 {
			sample = append(sample, map[string]any{
				"round": f.Round, "status": f.Status,
				"home": f.HomeName, "away": f.AwayName,
				"ft":                []any{f.FTHome, f.FTAway},
				"et":                []any{f.ETHome, f.ETAway},
				"pen":               []any{f.PenHome, f.PenAway},
				"advancerDerivable": f.Finished(),
			})
		}
	}
	unm := make([]string, 0, len(unmapped))
	for n := range unmapped {
		unm = append(unm, n)
	}
	sort.Strings(unm)

	return map[string]any{
		"season":           yr,
		"fixtures":         len(fixtures),
		"statusHistogram":  statusHist,
		"unmappedTeams":    unm,
		"ourMatchesTotal":  len(matches),
		"ourMatchesMapped": len(matchedRows),
		"withExtraTime":    etCount,
		"withPenalties":    penCount,
		"sample":           sample,
	}, nil
}

func ip(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

// ApplyResult is the exported entry point (used by the dev simulator) that
// writes a result onto a match record using the same logic as live sync /
// manual override.
func ApplyResult(rec *core.Record, status string, ftH, ftA, etH, etA, penH, penA *int) {
	applyResult(rec, status, ftH, ftA, etH, etA, penH, penA)
}

// applyResult writes scores/status onto a match record and, for knockout
// matches, derives the advancer (ET > penalties > regulation).
func applyResult(rec *core.Record, status string, ftH, ftA, etH, etA, penH, penA *int) {
	if status != "" {
		rec.Set("status", status)
	}
	if ftH != nil {
		rec.Set("ftHome", *ftH)
	}
	if ftA != nil {
		rec.Set("ftAway", *ftA)
	}
	rec.Set("etHome", ip(etH))
	rec.Set("etAway", ip(etA))
	rec.Set("penHome", ip(penH))
	rec.Set("penAway", ip(penA))

	finished := rec.GetString("status") == "finished"
	if finished {
		rec.Set("finalizedAt", time.Now().UTC())
	}

	if rec.GetString("stage") == "group" || !finished {
		return
	}
	// Knockout advancer resolution.
	home := rec.GetString("homeTeam")
	away := rec.GetString("awayTeam")
	switch {
	case penH != nil && penA != nil && *penH != *penA:
		if *penH > *penA {
			rec.Set("penWinner", home)
			rec.Set("advancer", home)
		} else {
			rec.Set("penWinner", away)
			rec.Set("advancer", away)
		}
	case etH != nil && etA != nil && *etH != *etA:
		if *etH > *etA {
			rec.Set("advancer", home)
		} else {
			rec.Set("advancer", away)
		}
	case ftH != nil && ftA != nil && *ftH != *ftA:
		if *ftH > *ftA {
			rec.Set("advancer", home)
		} else {
			rec.Set("advancer", away)
		}
	}
}
