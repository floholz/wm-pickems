package notify

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/floholz/wm-pickems/internal/mailer"
	"github.com/floholz/wm-pickems/internal/scoring"
)

// pushIcon maps an event to its contextual notification icon (served from the
// embedded static assets). Falls back to a generic bell.
func pushIcon(event string) string {
	switch event {
	case "stage_starting":
		return "/icons/notif/stage.png"
	case "forecast_reminder":
		return "/icons/notif/forecast.png"
	case "tips_reminder":
		return "/icons/notif/tips.png"
	case "results_recap":
		return "/icons/notif/recap.png"
	default:
		return "/icons/notif/default.png"
	}
}

// toPath reduces an (absolute or relative) URL to an origin-relative path+query
// for push notifications, so the deep-link resolves against the service worker's
// own origin (works on localhost, the tailnet, and prod without configuring the
// app URL). Emails keep the absolute URL — a relative href has no origin there.
func toPath(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.RequestURI() == "" {
		return raw
	}
	return u.RequestURI()
}

// inLeadWindow reports whether `start` is within [now, now+lead) — i.e. the
// deadline is in the future but no further off than the lead time.
func inLeadWindow(now, start time.Time, lead time.Duration) bool {
	return start.After(now) && !start.After(now.Add(lead))
}

// humanizeDur renders a coarse, friendly "time until" string.
func humanizeDur(d time.Duration) string {
	if d < time.Hour {
		return "less than an hour"
	}
	if d < 48*time.Hour {
		h := int((d + 30*time.Minute) / time.Hour)
		return fmt.Sprintf("%d hours", h)
	}
	days := int((d + 12*time.Hour) / (24 * time.Hour))
	return fmt.Sprintf("%d days", days)
}

// formatKickoff renders a kickoff time for email bodies (UTC, stable).
func formatKickoff(t time.Time) string {
	return t.UTC().Format("Mon, Jan 2 · 15:04") + " UTC"
}

func mailerMessage(u *core.Record, subject, html, text string) mailer.Message {
	return mailer.Message{
		ToEmail: u.Email(),
		ToName:  u.GetString("name"),
		Subject: subject,
		HTML:    html,
		Text:    text,
	}
}

// hasTip reports whether the user has a tip row for a match.
func (r *Runner) hasTip(userID, matchID string) bool {
	n, err := r.app.CountRecords("tips", dbx.HashExp{"user": userID, "match": matchID})
	return err == nil && n > 0
}

// teamNames returns teamId -> display name for labelling fixtures.
func (r *Runner) teamNames() map[string]string {
	out := map[string]string{}
	teams, err := r.app.FindRecordsByFilter("teams", "id != ''", "", 0, 0)
	if err != nil {
		return out
	}
	for _, t := range teams {
		out[t.Id] = t.GetString("name")
	}
	return out
}

// teamLabel resolves a match side to a team name, falling back to the
// placeholder label (e.g. "W74") when the team isn't decided yet.
func (r *Runner) teamLabel(m *core.Record, relField, labelField string, names map[string]string) string {
	if id := m.GetString(relField); id != "" {
		if n, ok := names[id]; ok {
			return n
		}
	}
	if l := m.GetString(labelField); l != "" {
		return l
	}
	return "TBD"
}

// forecastIncomplete reports whether a user's Forecast is missing or unfinished:
// no record, fewer filled groups than there are tournament groups, any group
// without 4 picks, or an empty bracket.
func (r *Runner) forecastIncomplete(userID string, groupCount int) bool {
	rec, err := r.app.FindFirstRecordByFilter("forecasts",
		"user = {:u}", map[string]any{"u": userID})
	if err != nil {
		return true
	}

	var order map[string][]string
	if err := json.Unmarshal([]byte(rec.GetString("groupOrder")), &order); err != nil {
		return true
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
		return true
	}

	var bracket map[string]string
	if err := json.Unmarshal([]byte(rec.GetString("bracket")), &bracket); err != nil || len(bracket) == 0 {
		return true
	}
	return false
}

// defaultConfigID returns the id of the default scoring config (empty if none).
func (r *Runner) defaultConfigID() string {
	def, err := r.app.FindFirstRecordByFilter("scoring_configs", "isDefault = true")
	if err != nil {
		return ""
	}
	return def.Id
}

// userPoints returns (points earned from the given finalized matches, total
// points) for a user under the given config.
func (r *Runner) userPoints(userID, cfgID string, finalized map[string]bool) (gained, total int) {
	ms, _ := r.app.FindRecordsByFilter("match_scores",
		"user = {:u} && config = {:c}", "", 0, 0,
		map[string]any{"u": userID, "c": cfgID})
	for _, s := range ms {
		p := s.GetInt("points")
		total += p
		if finalized[s.GetString("match")] {
			gained += p
		}
	}
	if fs, err := r.app.FindFirstRecordByFilter("forecast_scores",
		"user = {:u} && config = {:c}",
		map[string]any{"u": userID, "c": cfgID}); err == nil {
		total += fs.GetInt("points")
	}
	return gained, total
}

// userRanks computes the user's standing in each league they belong to, caching
// each league's leaderboard for the duration of one pass.
func (r *Runner) userRanks(userID string, cache map[string][]scoring.Row) []rankLine {
	mems, err := r.app.FindRecordsByFilter("league_members",
		"user = {:u}", "", 0, 0, map[string]any{"u": userID})
	if err != nil {
		return nil
	}
	var out []rankLine
	for _, mem := range mems {
		lid := mem.GetString("league")
		rows, ok := cache[lid]
		name := ""
		if !ok {
			lb, err := scoring.Leaderboard(r.app, lid)
			if err != nil {
				continue
			}
			if rr, ok := lb["rows"].([]scoring.Row); ok {
				rows = rr
			}
			cache[lid] = rows
		}
		if l, ok := r.leagueName(lid); ok {
			name = l
		}
		for i, row := range rows {
			if row.UserID == userID {
				out = append(out, rankLine{League: name, Rank: i + 1, Of: len(rows)})
				break
			}
		}
	}
	return out
}

func (r *Runner) leagueName(id string) (string, bool) {
	l, err := r.app.FindRecordById("leagues", id)
	if err != nil {
		return "", false
	}
	return l.GetString("name"), true
}
