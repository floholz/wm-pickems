// Package stats serves the owner-only app dashboard: high-level counts about
// users, leagues and notifications. Everything is computed on demand from a
// handful of full-collection scans — fine at this app's scale and avoids any
// denormalised counters to keep in sync.
package stats

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/floholz/wm-pickems/internal/users"
)

// realUserFilter excludes bot accounts: the role marker plus dev-generated
// bots, which use @dev.local emails and may predate the role marker.
const realUserFilter = "role != 'bot' && email !~ '@dev.local'"

// activeTipThreshold: a user counts as "active" with at least this many tips
// (or, failing that, a complete forecast).
const activeTipThreshold = 3

// Register wires the owner stats endpoint.
func Register(app core.App, se *core.ServeEvent) {
	se.Router.GET("/api/stats/owner", func(e *core.RequestEvent) error {
		if e.Auth == nil || !users.IsOwner(e.Auth) {
			return apis.NewForbiddenError("owner only", nil)
		}
		out, err := compute(app)
		if err != nil {
			return err
		}
		return e.JSON(http.StatusOK, out)
	}).Bind(apis.RequireAuth())
}

func compute(app core.App) (map[string]any, error) {
	realUsers, err := app.FindRecordsByFilter("users", realUserFilter, "", 0, 0)
	if err != nil {
		return nil, err
	}
	realID := map[string]bool{}
	for _, u := range realUsers {
		realID[u.Id] = true
	}

	// One pass over all tips -> per-user tip count (used for "active" users and
	// for whether a league has any tips in it).
	allTips, err := app.FindRecordsByFilter("tips", "id != ''", "", 0, 0)
	if err != nil {
		return nil, err
	}
	tipCount := map[string]int{}
	for _, t := range allTips {
		tipCount[t.GetString("user")]++
	}

	// Forecasts keyed by user, to test completeness without a query per user.
	forecasts, err := app.FindRecordsByFilter("forecasts", "id != ''", "", 0, 0)
	if err != nil {
		return nil, err
	}
	fcByUser := map[string]*core.Record{}
	for _, f := range forecasts {
		fcByUser[f.GetString("user")] = f
	}
	groupCount, err := app.CountRecords("tournament_groups")
	if err != nil {
		return nil, err
	}

	// Distinct users with at least one push subscription.
	subs, err := app.FindRecordsByFilter("push_subscriptions", "id != ''", "", 0, 0)
	if err != nil {
		return nil, err
	}
	pushUsers := map[string]bool{}
	for _, s := range subs {
		pushUsers[s.GetString("user")] = true
	}

	since := time.Now().Add(-24 * time.Hour)
	var usersLast24h, activeUsers, pushEnabled, notifyDisabled int
	for _, u := range realUsers {
		if u.GetDateTime("created").Time().After(since) {
			usersLast24h++
		}
		if tipCount[u.Id] >= activeTipThreshold || forecastComplete(fcByUser[u.Id], int(groupCount)) {
			activeUsers++
		}
		if pushUsers[u.Id] {
			pushEnabled++
		}
		if hasDisabledNotify(u.GetString("notifyPrefs")) {
			notifyDisabled++
		}
	}

	// Leagues, excluding the auto-managed Global league (every user is in it).
	leagues, err := app.FindRecordsByFilter("leagues", "inviteCode != 'GLOBAL'", "", 0, 0)
	if err != nil {
		return nil, err
	}
	members, err := app.FindRecordsByFilter("league_members", "id != ''", "", 0, 0)
	if err != nil {
		return nil, err
	}
	// "More than 1 person" counts real members only (bots don't make a league
	// active), and we require a real member to have actually tipped.
	realMembers := map[string]int{}
	leagueHasTip := map[string]bool{}
	for _, lm := range members {
		uid := lm.GetString("user")
		if !realID[uid] {
			continue
		}
		lid := lm.GetString("league")
		realMembers[lid]++
		if tipCount[uid] > 0 {
			leagueHasTip[lid] = true
		}
	}
	activeLeagues := 0
	for _, l := range leagues {
		if realMembers[l.Id] > 1 && leagueHasTip[l.Id] {
			activeLeagues++
		}
	}

	return map[string]any{
		"users":          len(realUsers),
		"usersLast24h":   usersLast24h,
		"activeUsers":    activeUsers,
		"leagues":        len(leagues),
		"activeLeagues":  activeLeagues,
		"pushEnabled":    pushEnabled,
		"notifyDisabled": notifyDisabled,
	}, nil
}

// forecastComplete mirrors the notify package's completeness rule: every
// tournament group ordered (4 picks each) and a non-empty bracket.
func forecastComplete(rec *core.Record, groupCount int) bool {
	if rec == nil {
		return false
	}
	var order map[string][]string
	if err := json.Unmarshal([]byte(rec.GetString("groupOrder")), &order); err != nil {
		return false
	}
	filled := 0
	for _, ids := range order {
		got := 0
		for _, id := range ids {
			if id != "" {
				got++
			}
		}
		if got >= 4 {
			filled++
		}
	}
	if filled < groupCount {
		return false
	}
	var bracket map[string]string
	if err := json.Unmarshal([]byte(rec.GetString("bracket")), &bracket); err != nil || len(bracket) == 0 {
		return false
	}
	return true
}

// hasDisabledNotify reports whether the user has opted out of at least one
// notification (any event/channel set to false in notifyPrefs). Prefs are
// opt-out, so an empty/absent map means everything is still on.
func hasDisabledNotify(raw string) bool {
	if raw == "" {
		return false
	}
	var prefs map[string]map[string]bool
	if err := json.Unmarshal([]byte(raw), &prefs); err != nil {
		return false
	}
	for _, ev := range prefs {
		for _, on := range ev {
			if !on {
				return true
			}
		}
	}
	return false
}
