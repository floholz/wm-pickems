// Package dev is a test harness, only active when WMP_DEV=1. It can pin a
// virtual clock and simulate match results up to a timestamp, reusing the
// exact production paths (ApplyResult -> ResolveBracket -> Recompute) so the
// simulation also exercises the real logic. The mutating routes are not
// registered unless WMP_DEV=1, so it can never run in production.
package dev

import (
	"context"
	"fmt"
	"hash/fnv"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/floholz/wm-pickems/internal/clock"
	"github.com/floholz/wm-pickems/internal/football"
	"github.com/floholz/wm-pickems/internal/forecast"
	"github.com/floholz/wm-pickems/internal/scoring"
	wmsync "github.com/floholz/wm-pickems/internal/sync"
	"github.com/floholz/wm-pickems/internal/tips"
)

var botNames = []string{
	"Bot Alex", "Bot Robin", "Bot Sam", "Bot Casey", "Bot Jordan",
	"Bot Riley", "Bot Quinn", "Bot Skylar", "Bot Morgan", "Bot Drew",
	"Bot Pat", "Bot Lee", "Bot Noor", "Bot Kai", "Bot Remy",
}

// Match windows: a result is "finished" once sim time passes kickoff+window,
// "live" between kickoff and that, otherwise still scheduled.
const (
	groupWindow = 120 * time.Minute // 90' + half-time + stoppage buffer
	koWindow    = 140 * time.Minute // + extra time + penalties
)

func Enabled() bool { return os.Getenv("WMP_DEV") == "1" }

// LoadDotenv loads ./.env into the process environment so `go run .` /
// `make run` see the same config docker-compose would inject — no container
// rebuild for local dev. Only active when WMP_DEV=1, and variables already
// set in the shell always win (so explicit overrides keep working).
func LoadDotenv() {
	if !Enabled() {
		return
	}
	b, err := os.ReadFile(".env")
	if err != nil {
		return
	}
	n := 0
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(strings.TrimPrefix(k, "export "))
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		}
		if k == "" || v == "" {
			continue
		}
		if _, exists := os.LookupEnv(k); !exists {
			os.Setenv(k, v)
			n++
		}
	}
	if n > 0 {
		log.Printf("dev: loaded %d vars from .env", n)
	}
}

func intp(v int) *int { return &v }

// rngFor returns a deterministic RNG for a match so re-advancing to the same
// timestamp yields stable results.
func rngFor(extID string) *rand.Rand {
	h := fnv.New64a()
	h.Write([]byte(extID))
	return rand.New(rand.NewSource(int64(h.Sum64())))
}

func windowFor(stage string) time.Duration {
	if stage == "group" {
		return groupWindow
	}
	return koWindow
}

// resetMatch returns a match to its pre-result state.
func resetMatch(m *core.Record) {
	m.Set("status", "scheduled")
	for _, f := range []string{"ftHome", "ftAway", "etHome", "etAway", "penHome", "penAway"} {
		m.Set(f, 0)
	}
	m.Set("penWinner", "")
	m.Set("advancer", "")
	m.Set("finalizedAt", "")
	if m.GetString("stage") != "group" {
		// Knockout teams are only filled by the resolver from results.
		m.Set("homeTeam", "")
		m.Set("awayTeam", "")
	}
}

// simulate brings all matches to the state they'd be in at simNow.
func simulate(app core.App, simNow time.Time) error {
	all, err := app.FindRecordsByFilter("matches", "id != ''", "kickoff", 0, 0)
	if err != nil {
		return err
	}
	// Reset anything now in the future (supports moving the clock back).
	for _, m := range all {
		if simNow.Before(m.GetDateTime("kickoff").Time()) {
			resetMatch(m)
			if err := app.Save(m); err != nil {
				return err
			}
		}
	}

	// Converge: resolve the bracket, finalize/mark-live everything due, repeat
	// until stable (knockout matches need their feeders finished first).
	for pass := 0; pass < 12; pass++ {
		if err := wmsync.ResolveBracket(app); err != nil {
			return err
		}
		matches, err := app.FindRecordsByFilter("matches", "id != ''", "kickoff", 0, 0)
		if err != nil {
			return err
		}
		changed := false
		for _, m := range matches {
			if m.GetString("status") == "finished" {
				continue
			}
			ko := m.GetDateTime("kickoff").Time()
			if simNow.Before(ko) {
				continue
			}
			if simNow.Before(ko.Add(windowFor(m.GetString("stage")))) {
				if m.GetString("status") != "live" {
					m.Set("status", "live")
					m.Set("finalizedAt", "")
					if err := app.Save(m); err != nil {
						return err
					}
					changed = true
				}
				continue
			}
			// Finished. Knockout needs both teams resolved first.
			if m.GetString("stage") != "group" &&
				(m.GetString("homeTeam") == "" || m.GetString("awayTeam") == "") {
				continue
			}
			r := rngFor(m.GetString("extId"))
			if m.GetString("stage") == "group" {
				wmsync.ApplyResult(m, "finished",
					intp(r.Intn(5)), intp(r.Intn(5)), nil, nil, nil, nil)
			} else {
				h, a := r.Intn(4), r.Intn(4)
				if h != a {
					wmsync.ApplyResult(m, "finished",
						intp(h), intp(a), nil, nil, nil, nil)
				} else {
					// Drawn at 90' -> decided in extra time.
					wmsync.ApplyResult(m, "finished",
						intp(h), intp(a),
						intp(h+1), intp(a), nil, nil)
				}
			}
			if err := app.Save(m); err != nil {
				return err
			}
			changed = true
		}
		if !changed {
			break
		}
	}
	if err := backfillBotAdvancers(app); err != nil {
		return err
	}
	return scoring.Recompute(app)
}

// backfillBotAdvancers sets the advancer on dev tips whose knockout match has
// since resolved its teams. Bot tips are created before the bracket is known,
// so they store no advancer; without it their knockout "correct result" never
// scores. Dev KO tips are decisive, so the advancer is the higher-scoring side.
// Only empty-advancer tips are touched — real user KO tips always carry one.
func backfillBotAdvancers(app core.App) error {
	kos, err := app.FindRecordsByFilter("matches",
		"stage != 'group' && homeTeam != '' && awayTeam != ''", "", 0, 0)
	if err != nil {
		return err
	}
	tips.SetBypass(true)
	defer tips.SetBypass(false)
	for _, m := range kos {
		ts, err := app.FindRecordsByFilter("tips",
			"match = {:m} && advancer = ''", "", 0, 0, map[string]any{"m": m.Id})
		if err != nil {
			return err
		}
		for _, t := range ts {
			fh, fa := t.GetInt("ftHome"), t.GetInt("ftAway")
			if fh == fa {
				continue // not decisive — winner unknowable from the score alone
			}
			if fh > fa {
				t.Set("advancer", m.GetString("homeTeam"))
			} else {
				t.Set("advancer", m.GetString("awayTeam"))
			}
			if err := app.Save(t); err != nil {
				return err
			}
		}
	}
	return nil
}

// makeBots creates `count` bot users, each with a complete consistent
// Forecast and a Tip on every match, joined to the given leagues. Uses the
// dev-only validation bypass so it works even after the clock is advanced.
func makeBots(app core.App, count int, leagueIDs []string) ([]string, error) {
	usersCol, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return nil, err
	}
	fcCol, err := app.FindCollectionByNameOrId("forecasts")
	if err != nil {
		return nil, err
	}
	tipsCol, err := app.FindCollectionByNameOrId("tips")
	if err != nil {
		return nil, err
	}
	lmCol, err := app.FindCollectionByNameOrId("league_members")
	if err != nil {
		return nil, err
	}
	matches, err := app.FindRecordsByFilter("matches", "id != ''", "kickoff", 0, 0)
	if err != nil {
		return nil, err
	}

	forecast.SetBypass(true)
	tips.SetBypass(true)
	defer forecast.SetBypass(false)
	defer tips.SetBypass(false)

	// One transaction for the whole batch: ~100 tips/bot as individual saves is
	// slow and can trip "database is locked" under any concurrent request, which
	// left earlier batches half-created. Batching makes it fast and all-or-nothing.
	var created []string
	used := map[string]int{}
	err = app.RunInTransaction(func(tx core.App) error {
		for i := 0; i < count; i++ {
			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(i*7919)))
			name := botNames[rng.Intn(len(botNames))]
			if used[name]++; used[name] > 1 {
				name = fmt.Sprintf("%s %d", name, used[name])
			}

			u := core.NewRecord(usersCol)
			u.SetEmail(fmt.Sprintf("bot-%d@dev.local", time.Now().UnixNano()+int64(i)))
			u.SetRandomPassword()
			u.Set("name", name)
			u.Set("verified", true)
			if err := tx.Save(u); err != nil {
				return err
			}

			order, thirds, bracket, err := scoring.RandomForecast(tx, rng)
			if err != nil {
				return err
			}
			f := core.NewRecord(fcCol)
			f.Set("user", u.Id)
			f.Set("groupOrder", order)
			f.Set("thirdQualifiers", thirds)
			f.Set("bracket", bracket)
			if err := tx.Save(f); err != nil {
				return err
			}

			for _, m := range matches {
				t := core.NewRecord(tipsCol)
				t.Set("user", u.Id)
				t.Set("match", m.Id)
				fh, fa := rng.Intn(5), rng.Intn(5)
				// Knockouts need a clear winner. The dev path bypasses
				// validateAndDerive, and the teams often aren't resolved yet at
				// generation time (so we can't store an advancer id) — keep the
				// tip decisive so the scoreline's winner arrow still works, and
				// set the advancer when the matchup is already known.
				if m.GetString("stage") != "group" {
					if fh == fa {
						if rng.Intn(2) == 0 {
							fh++
						} else {
							fa++
						}
					}
					if home, away := m.GetString("homeTeam"), m.GetString("awayTeam"); home != "" && away != "" {
						if fh > fa {
							t.Set("advancer", home)
						} else {
							t.Set("advancer", away)
						}
					}
				}
				t.Set("ftHome", fh)
				t.Set("ftAway", fa)
				if err := tx.Save(t); err != nil {
					return err
				}
			}

			for _, lid := range leagueIDs {
				lm := core.NewRecord(lmCol)
				lm.Set("league", lid)
				lm.Set("user", u.Id)
				lm.Set("role", "member")
				if err := tx.Save(lm); err != nil {
					return err
				}
			}
			created = append(created, name)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func parseTimestamp(s string) (time.Time, bool) {
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

func state(app core.App) map[string]any {
	now := clock.Now(app)
	sim, isSim := clock.Sim(app)
	out := map[string]any{
		"dev":       Enabled(),
		"now":       now.UnixMilli(),
		"simulated": isSim,
	}
	if isSim {
		out["simTime"] = sim.Format(time.RFC3339)
	}
	return out
}

// Register wires /api/now (always) and, only when WMP_DEV=1, the dev
// mutating endpoints.
func Register(app core.App, se *core.ServeEvent) {
	se.Router.GET("/api/now", func(e *core.RequestEvent) error {
		return e.JSON(http.StatusOK, state(app))
	})

	if !Enabled() {
		return
	}

	g := se.Router.Group("/api/dev")
	g.Bind(apis.RequireAuth())

	g.GET("/state", func(e *core.RequestEvent) error {
		return e.JSON(http.StatusOK, state(app))
	})

	// GET /api/dev/apicheck?season=2026 — validate the live API: plan/quota,
	// schema parse, team-name mapping vs our seed, our-match coverage, and
	// the status/ET/penalty distribution. Point season at a finished World
	// Cup (e.g. 2022) to exercise the results path before 2026 kicks off.
	g.GET("/apicheck", func(e *core.RequestEvent) error {
		key := os.Getenv("API_FOOTBALL_KEY")
		if key == "" {
			return e.JSON(400, map[string]string{"error": "API_FOOTBALL_KEY not set"})
		}
		yr := 2026
		if s := e.Request.URL.Query().Get("season"); s != "" {
			if n, err := strconv.Atoi(s); err == nil {
				yr = n
			}
		}
		ctx, cancel := context.WithTimeout(e.Request.Context(), 30*time.Second)
		defer cancel()
		client := football.New(key)
		out := map[string]any{}
		if st, err := client.Status(ctx); err == nil {
			out["account"] = st
		} else {
			out["statusError"] = err.Error()
		}
		rep, err := wmsync.APICheck(ctx, app, client, yr)
		if err != nil {
			out["error"] = err.Error()
			return e.JSON(502, out)
		}
		for k, v := range rep {
			out[k] = v
		}
		return e.JSON(http.StatusOK, out)
	})

	// POST /api/dev/advance { "timestamp": "2026-06-20T16:20:00Z" }
	g.POST("/advance", func(e *core.RequestEvent) error {
		var body struct {
			Timestamp string `json:"timestamp"`
		}
		if err := e.BindBody(&body); err != nil {
			return e.JSON(400, map[string]string{"error": err.Error()})
		}
		ts, ok := parseTimestamp(body.Timestamp)
		if !ok {
			return e.JSON(400, map[string]string{"error": "bad timestamp"})
		}
		if err := clock.Set(app, ts); err != nil {
			return e.JSON(500, map[string]string{"error": err.Error()})
		}
		if err := simulate(app, ts); err != nil {
			return e.JSON(500, map[string]string{"error": err.Error()})
		}
		return e.JSON(http.StatusOK, state(app))
	})

	// POST /api/dev/bots { "count": 3, "leagueId": "" } — create bot players
	// with a full Forecast + a Tip on every match. Joins the given league, or
	// every league the caller is in if omitted.
	g.POST("/bots", func(e *core.RequestEvent) error {
		var body struct {
			Count    int    `json:"count"`
			LeagueID string `json:"leagueId"`
		}
		_ = e.BindBody(&body)
		if body.Count <= 0 {
			body.Count = 1
		}
		if body.Count > 20 {
			body.Count = 20
		}
		var leagueIDs []string
		if body.LeagueID != "" {
			leagueIDs = []string{body.LeagueID}
		} else {
			mems, _ := app.FindRecordsByFilter("league_members",
				"user = {:u}", "", 0, 0, map[string]any{"u": e.Auth.Id})
			for _, m := range mems {
				leagueIDs = append(leagueIDs, m.GetString("league"))
			}
		}
		names, err := makeBots(app, body.Count, leagueIDs)
		if err != nil {
			return e.JSON(500, map[string]any{"error": err.Error(), "created": names})
		}
		if err := scoring.Recompute(app); err != nil {
			return e.JSON(500, map[string]any{"error": err.Error()})
		}
		return e.JSON(http.StatusOK, map[string]any{"created": names})
	})

	// POST /api/dev/reset — clear all results and the dev clock.
	g.POST("/reset", func(e *core.RequestEvent) error {
		matches, err := app.FindRecordsByFilter("matches", "id != ''", "", 0, 0)
		if err != nil {
			return e.JSON(500, map[string]string{"error": err.Error()})
		}
		for _, m := range matches {
			resetMatch(m)
			if err := app.Save(m); err != nil {
				return e.JSON(500, map[string]string{"error": err.Error()})
			}
		}
		if err := clock.Clear(app); err != nil {
			return e.JSON(500, map[string]string{"error": err.Error()})
		}
		if err := scoring.Recompute(app); err != nil {
			return e.JSON(500, map[string]string{"error": err.Error()})
		}
		return e.JSON(http.StatusOK, state(app))
	})
}
