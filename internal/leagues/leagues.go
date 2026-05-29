// Package leagues provides the private-competition ("League") endpoints:
// create (with a unique invite code, creator auto-joined as owner), join by
// code, list mine, and a leaderboard. Scoring totals are filled by the Phase 5
// engine; until then the leaderboard returns members with zeroed points.
package leagues

import (
	"crypto/rand"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/floholz/wm-pickems/internal/scoring"
)

const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no ambiguous chars

// GlobalInviteCode is the fixed invite code of the auto-managed "Global" league
// that every registered user belongs to.
const GlobalInviteCode = "GLOBAL"

func newInviteCode(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	var sb strings.Builder
	for _, v := range b {
		sb.WriteByte(codeAlphabet[int(v)%len(codeAlphabet)])
	}
	return sb.String()
}

func bad(e *core.RequestEvent, code int, msg string) error {
	return e.JSON(code, map[string]string{"error": msg})
}

// uniqueCode returns a fresh 6-char invite code not currently in use (retrying
// on the rare collision).
func uniqueCode(app core.App) string {
	var code string
	for range 10 {
		code = newInviteCode(6)
		if _, err := app.FindFirstRecordByFilter("leagues", "inviteCode = {:c}", map[string]any{"c": code}); err != nil {
			break
		}
	}
	return code
}

// ownedLeague loads the league and authorizes the caller as its owner. It
// returns a 403 for non-owners — which also covers the auto-managed "Global"
// league, whose owner is empty and therefore matches no authenticated user.
func ownedLeague(app core.App, e *core.RequestEvent, id string) (*core.Record, error) {
	lg, err := app.FindRecordById("leagues", id)
	if err != nil {
		return nil, bad(e, http.StatusNotFound, "league not found")
	}
	if lg.GetString("owner") != e.Auth.Id {
		return nil, bad(e, http.StatusForbidden, "only the league owner can do this")
	}
	return lg, nil
}

// Register wires the League endpoints. Most require an authenticated user;
// the invite-preview route below is intentionally public.
func Register(app core.App, se *core.ServeEvent) {
	// Auto-managed "Global" league: ensure it exists, backfill existing users,
	// and add every new user as a member when their account is created.
	if err := backfillGlobal(app); err != nil {
		log.Printf("[leagues] global backfill failed: %v", err)
	}
	app.OnRecordAfterCreateSuccess("users").BindFunc(func(e *core.RecordEvent) error {
		if err := ensureGlobalMember(e.App, e.Record.Id); err != nil {
			log.Printf("[leagues] auto-join global failed for %s: %v", e.Record.Id, err)
		}
		return e.Next()
	})

	// Public: resolve an invite code to a league name for the invite landing
	// page. Possessing the code is the capability (it's an invite link); only
	// id + name are exposed, nothing member- or score-related.
	//
	// Lives under /api/invite (not /api/leagues) on purpose: Go 1.22's router
	// rejects a path-param route under /api/leagues/ as ambiguous against
	// /api/leagues/{id}/leaderboard.
	se.Router.GET("/api/invite/{code}", func(e *core.RequestEvent) error {
		code := strings.ToUpper(strings.TrimSpace(e.Request.PathValue("code")))
		league, err := app.FindFirstRecordByFilter("leagues",
			"inviteCode = {:c}", map[string]any{"c": code})
		if err != nil {
			return bad(e, http.StatusNotFound, "invalid invite code")
		}
		return e.JSON(http.StatusOK, map[string]any{
			"id": league.Id, "name": league.GetString("name"),
		})
	})

	g := se.Router.Group("/api/leagues")
	g.Bind(apis.RequireAuth())

	// POST /api/leagues/create  { "name": "..." }
	g.POST("/create", func(e *core.RequestEvent) error {
		var body struct {
			Name string `json:"name"`
		}
		if err := e.BindBody(&body); err != nil {
			return bad(e, http.StatusBadRequest, err.Error())
		}
		name := strings.TrimSpace(body.Name)
		if name == "" {
			return bad(e, http.StatusBadRequest, "name required")
		}

		col, err := app.FindCollectionByNameOrId("leagues")
		if err != nil {
			return err
		}

		code := uniqueCode(app)

		def, _ := app.FindFirstRecordByFilter("scoring_configs", "isDefault = true")

		league := core.NewRecord(col)
		league.Set("name", name)
		league.Set("inviteCode", code)
		league.Set("owner", e.Auth.Id)
		if def != nil {
			league.Set("scoringConfig", def.Id)
		}
		if err := app.Save(league); err != nil {
			return err
		}
		if err := addMember(app, league.Id, e.Auth.Id, "owner"); err != nil {
			return err
		}
		return e.JSON(http.StatusOK, map[string]any{
			"id": league.Id, "name": name, "inviteCode": code,
		})
	})

	// POST /api/leagues/join  { "code": "ABC123" }
	g.POST("/join", func(e *core.RequestEvent) error {
		var body struct {
			Code string `json:"code"`
		}
		if err := e.BindBody(&body); err != nil {
			return bad(e, http.StatusBadRequest, err.Error())
		}
		code := strings.ToUpper(strings.TrimSpace(body.Code))
		league, err := app.FindFirstRecordByFilter("leagues", "inviteCode = {:c}", map[string]any{"c": code})
		if err != nil {
			return bad(e, http.StatusNotFound, "invalid invite code")
		}
		if existing, _ := app.FindFirstRecordByFilter("league_members",
			"league = {:l} && user = {:u}",
			map[string]any{"l": league.Id, "u": e.Auth.Id}); existing != nil {
			return e.JSON(http.StatusOK, map[string]any{"id": league.Id, "name": league.GetString("name"), "already": true})
		}
		if err := addMember(app, league.Id, e.Auth.Id, "member"); err != nil {
			return err
		}
		return e.JSON(http.StatusOK, map[string]any{"id": league.Id, "name": league.GetString("name")})
	})

	// GET /api/leagues/mine
	g.GET("/mine", func(e *core.RequestEvent) error {
		members, err := app.FindRecordsByFilter("league_members",
			"user = {:u}", "-joinedAt", 0, 0, map[string]any{"u": e.Auth.Id})
		if err != nil {
			return err
		}
		out := make([]map[string]any, 0, len(members))
		for _, m := range members {
			lg, err := app.FindRecordById("leagues", m.GetString("league"))
			if err != nil {
				continue
			}
			cnt, _ := app.CountRecords("league_members",
				dbx.HashExp{"league": lg.Id})
			role := m.GetString("role")
			private := lg.GetBool("privateCode")
			// On a private league only the owner may see/share the code.
			code := lg.GetString("inviteCode")
			if private && role != "owner" {
				code = ""
			}
			out = append(out, map[string]any{
				"id":         lg.Id,
				"name":       lg.GetString("name"),
				"inviteCode": code,
				"role":       role,
				"private":    private,
				"members":    cnt,
			})
		}
		return e.JSON(http.StatusOK, map[string]any{"leagues": out})
	})

	// GET /api/leagues/{id}/leaderboard
	g.GET("/{id}/leaderboard", func(e *core.RequestEvent) error {
		id := e.Request.PathValue("id")
		if _, err := app.FindFirstRecordByFilter("league_members",
			"league = {:l} && user = {:u}",
			map[string]any{"l": id, "u": e.Auth.Id}); err != nil {
			return bad(e, http.StatusForbidden, "not a member of this league")
		}
		lb, err := scoring.Leaderboard(app, id)
		if err != nil {
			return bad(e, http.StatusNotFound, "league not found")
		}
		// Include the league's scoring config so the legend can render it
		// without the client reading the (now members-only) leagues table.
		if lg, err := app.FindRecordById("leagues", id); err == nil {
			cid := lg.GetString("scoringConfig")
			var sc *core.Record
			if cid != "" {
				sc, _ = app.FindRecordById("scoring_configs", cid)
			}
			if sc == nil {
				sc, _ = app.FindFirstRecordByFilter("scoring_configs", "isDefault = true")
			}
			if sc != nil {
				var cfg map[string]any
				if json.Unmarshal([]byte(sc.GetString("config")), &cfg) == nil {
					lb["scoring"] = cfg
				}
			}
		}
		return e.JSON(http.StatusOK, lb)
	})

	// ---- Owner-only management (rename, regenerate code, privacy, remove) ----

	// POST /api/leagues/{id}/rename  { "name": "..." }
	g.POST("/{id}/rename", func(e *core.RequestEvent) error {
		lg, err := ownedLeague(app, e, e.Request.PathValue("id"))
		if err != nil {
			return err
		}
		var body struct {
			Name string `json:"name"`
		}
		if err := e.BindBody(&body); err != nil {
			return bad(e, http.StatusBadRequest, err.Error())
		}
		name := strings.TrimSpace(body.Name)
		if name == "" {
			return bad(e, http.StatusBadRequest, "name required")
		}
		lg.Set("name", name)
		if err := app.Save(lg); err != nil {
			return err
		}
		return e.JSON(http.StatusOK, map[string]any{"id": lg.Id, "name": name})
	})

	// POST /api/leagues/{id}/code/regenerate
	g.POST("/{id}/code/regenerate", func(e *core.RequestEvent) error {
		lg, err := ownedLeague(app, e, e.Request.PathValue("id"))
		if err != nil {
			return err
		}
		code := uniqueCode(app)
		lg.Set("inviteCode", code)
		if err := app.Save(lg); err != nil {
			return err
		}
		return e.JSON(http.StatusOK, map[string]any{"inviteCode": code})
	})

	// POST /api/leagues/{id}/code/visibility  { "private": true }
	g.POST("/{id}/code/visibility", func(e *core.RequestEvent) error {
		lg, err := ownedLeague(app, e, e.Request.PathValue("id"))
		if err != nil {
			return err
		}
		var body struct {
			Private bool `json:"private"`
		}
		if err := e.BindBody(&body); err != nil {
			return bad(e, http.StatusBadRequest, err.Error())
		}
		lg.Set("privateCode", body.Private)
		if err := app.Save(lg); err != nil {
			return err
		}
		return e.JSON(http.StatusOK, map[string]any{"private": body.Private})
	})

	// POST /api/leagues/{id}/members/remove  { "userId": "..." }
	g.POST("/{id}/members/remove", func(e *core.RequestEvent) error {
		lg, err := ownedLeague(app, e, e.Request.PathValue("id"))
		if err != nil {
			return err
		}
		var body struct {
			UserID string `json:"userId"`
		}
		if err := e.BindBody(&body); err != nil {
			return bad(e, http.StatusBadRequest, err.Error())
		}
		if body.UserID == "" {
			return bad(e, http.StatusBadRequest, "userId required")
		}
		if body.UserID == lg.GetString("owner") {
			return bad(e, http.StatusBadRequest, "the owner cannot be removed")
		}
		member, err := app.FindFirstRecordByFilter("league_members",
			"league = {:l} && user = {:u}",
			map[string]any{"l": lg.Id, "u": body.UserID})
		if err != nil {
			return bad(e, http.StatusNotFound, "not a member of this league")
		}
		if err := app.Delete(member); err != nil {
			return err
		}
		return e.JSON(http.StatusOK, map[string]any{"ok": true})
	})
}

func addMember(app core.App, leagueID, userID, role string) error {
	col, err := app.FindCollectionByNameOrId("league_members")
	if err != nil {
		return err
	}
	rec := core.NewRecord(col)
	rec.Set("league", leagueID)
	rec.Set("user", userID)
	rec.Set("role", role)
	return app.Save(rec)
}

// ensureGlobal idempotently creates the "Global" league (owner left empty so
// no one can update/delete it via REST). Returns the league id.
func ensureGlobal(app core.App) (string, error) {
	if rec, err := app.FindFirstRecordByFilter("leagues",
		"inviteCode = {:c}", map[string]any{"c": GlobalInviteCode}); err == nil {
		return rec.Id, nil
	}
	col, err := app.FindCollectionByNameOrId("leagues")
	if err != nil {
		return "", err
	}
	def, _ := app.FindFirstRecordByFilter("scoring_configs", "isDefault = true")
	rec := core.NewRecord(col)
	rec.Set("name", "Global")
	rec.Set("inviteCode", GlobalInviteCode)
	if def != nil {
		rec.Set("scoringConfig", def.Id)
	}
	if err := app.Save(rec); err != nil {
		return "", err
	}
	return rec.Id, nil
}

// ensureGlobalMember adds the user to the Global league if not already a member.
func ensureGlobalMember(app core.App, userID string) error {
	leagueID, err := ensureGlobal(app)
	if err != nil {
		return err
	}
	if existing, _ := app.FindFirstRecordByFilter("league_members",
		"league = {:l} && user = {:u}",
		map[string]any{"l": leagueID, "u": userID}); existing != nil {
		return nil
	}
	return addMember(app, leagueID, userID, "member")
}

// backfillGlobal ensures every existing user is a member of the Global league.
// Cheap on subsequent boots: the per-user membership check short-circuits.
func backfillGlobal(app core.App) error {
	if _, err := ensureGlobal(app); err != nil {
		return err
	}
	users, err := app.FindRecordsByFilter("users", "id != ''", "", 0, 0)
	if err != nil {
		return err
	}
	for _, u := range users {
		if err := ensureGlobalMember(app, u.Id); err != nil {
			return err
		}
	}
	return nil
}
