// Package chat is the League Chat backend (v1): a simple per-league text chat
// for private leagues. Reads/realtime ride the league_messages collection's
// member-only rule; every write goes through these validated endpoints (whose
// app.Save still fires the realtime event clients subscribe to). A per-user
// league_reads marker drives unread badges.
package chat

import (
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/floholz/wm-pickems/internal/users"
)

const (
	messages = "league_messages"
	reads    = "league_reads"
	maxLen   = 2000
	pageSize = 50
)

// Register wires the chat endpoints.
func Register(app core.App, se *core.ServeEvent) {
	g := se.Router.Group("/api")
	g.Bind(apis.RequireAuth())

	// GET /api/leagues/{id}/members — member directory (resolves message senders,
	// including for realtime messages from someone not yet in the loaded history).
	g.GET("/leagues/{id}/members", func(e *core.RequestEvent) error {
		lid := e.Request.PathValue("id")
		if _, err := authorize(app, e, lid); err != nil {
			return err
		}
		mems, err := app.FindRecordsByFilter("league_members",
			"league = {:l}", "", 0, 0, dbx.Params{"l": lid})
		if err != nil {
			return err
		}
		out := make([]map[string]any, 0, len(mems))
		for _, m := range mems {
			if u, err := app.FindRecordById("users", m.GetString("user")); err == nil {
				out = append(out, userView(u))
			}
		}
		return e.JSON(http.StatusOK, map[string]any{"members": out})
	})

	// GET /api/leagues/{id}/chat?before=<rfc3339>&limit=N — message history,
	// newest first (the client reverses + prepends for "load older").
	g.GET("/leagues/{id}/chat", func(e *core.RequestEvent) error {
		lid := e.Request.PathValue("id")
		if _, err := authorize(app, e, lid); err != nil {
			return err
		}
		filter := "league = {:l}"
		params := dbx.Params{"l": lid}
		if before := e.Request.URL.Query().Get("before"); before != "" {
			filter += " && created < {:b}"
			params["b"] = before
		}
		recs, err := app.FindRecordsByFilter(messages, filter, "-created", pageSize, 0, params)
		if err != nil {
			return err
		}
		mod := users.IsAdmin(e.Auth) // admins see deleted originals for moderation
		out := make([]map[string]any, 0, len(recs))
		for _, r := range recs {
			out = append(out, msgView(r, mod))
		}
		return e.JSON(http.StatusOK, map[string]any{"messages": out, "hasMore": len(recs) == pageSize})
	})

	// POST /api/leagues/{id}/chat  { "text": "..." } — post a message.
	g.POST("/leagues/{id}/chat", func(e *core.RequestEvent) error {
		lid := e.Request.PathValue("id")
		if _, err := authorize(app, e, lid); err != nil {
			return err
		}
		var body struct {
			Text string `json:"text"`
		}
		if err := e.BindBody(&body); err != nil {
			return apis.NewBadRequestError(err.Error(), nil)
		}
		text := strings.TrimSpace(body.Text)
		if text == "" {
			return apis.NewBadRequestError("message is empty", nil)
		}
		if len([]rune(text)) > maxLen {
			text = string([]rune(text)[:maxLen])
		}
		col, err := app.FindCollectionByNameOrId(messages)
		if err != nil {
			return err
		}
		rec := core.NewRecord(col)
		rec.Set("league", lid)
		rec.Set("user", e.Auth.Id)
		rec.Set("text", text)
		if err := app.Save(rec); err != nil { // fires the realtime create event
			return err
		}
		markRead(app, lid, e.Auth.Id) // sender is caught up by definition
		return e.JSON(http.StatusOK, msgView(rec, false))
	})

	// DELETE /api/leagues/{id}/chat/{msgId} — author or league owner soft-deletes
	// a message: the live text is cleared (members + realtime see only "message
	// deleted") and the original is stashed in the hidden origText field for
	// admin moderation. Fires a realtime update event.
	g.DELETE("/leagues/{id}/chat/{msgId}", func(e *core.RequestEvent) error {
		lid := e.Request.PathValue("id")
		lg, err := authorize(app, e, lid)
		if err != nil {
			return err
		}
		rec, err := app.FindRecordById(messages, e.Request.PathValue("msgId"))
		if err != nil || rec.GetString("league") != lid {
			return apis.NewNotFoundError("message not found", nil)
		}
		if rec.GetString("user") != e.Auth.Id && lg.GetString("owner") != e.Auth.Id {
			return apis.NewForbiddenError("not allowed", nil)
		}
		if !rec.GetBool("deleted") {
			rec.Set("origText", rec.GetString("text"))
			rec.Set("text", "")
			rec.Set("deleted", true)
			rec.Set("deletedBy", e.Auth.Id)
			rec.Set("deletedAt", time.Now().UTC())
			if err := app.Save(rec); err != nil {
				return err
			}
		}
		return e.JSON(http.StatusOK, msgView(rec, users.IsAdmin(e.Auth)))
	})

	// POST /api/leagues/{id}/chat/{msgId}/restore — author or league owner undoes
	// a soft-delete: the original text comes back and the deleted flags clear.
	// Fires a realtime update event. Backs the "Undo" affordance after deleting.
	g.POST("/leagues/{id}/chat/{msgId}/restore", func(e *core.RequestEvent) error {
		lid := e.Request.PathValue("id")
		lg, err := authorize(app, e, lid)
		if err != nil {
			return err
		}
		rec, err := app.FindRecordById(messages, e.Request.PathValue("msgId"))
		if err != nil || rec.GetString("league") != lid {
			return apis.NewNotFoundError("message not found", nil)
		}
		if rec.GetString("user") != e.Auth.Id && lg.GetString("owner") != e.Auth.Id {
			return apis.NewForbiddenError("not allowed", nil)
		}
		if rec.GetBool("deleted") {
			rec.Set("text", rec.GetString("origText"))
			rec.Set("origText", "")
			rec.Set("deleted", false)
			rec.Set("deletedBy", "")
			rec.Set("deletedAt", "")
			if err := app.Save(rec); err != nil {
				return err
			}
		}
		return e.JSON(http.StatusOK, msgView(rec, users.IsAdmin(e.Auth)))
	})

	// POST /api/leagues/{id}/chat/read — mark this league's chat read (to now).
	g.POST("/leagues/{id}/chat/read", func(e *core.RequestEvent) error {
		lid := e.Request.PathValue("id")
		if _, err := authorize(app, e, lid); err != nil {
			return err
		}
		markRead(app, lid, e.Auth.Id)
		return e.JSON(http.StatusOK, map[string]any{"ok": true})
	})

	// GET /api/chat/unread — { leagueId: count } for the caller's private leagues.
	g.GET("/chat/unread", func(e *core.RequestEvent) error {
		uid := e.Auth.Id
		mems, err := app.FindRecordsByFilter("league_members",
			"user = {:u}", "", 0, 0, dbx.Params{"u": uid})
		if err != nil {
			return err
		}
		lastReads := readMarkers(app, uid)
		out := map[string]int{}
		for _, m := range mems {
			lid := m.GetString("league")
			lg, err := app.FindRecordById("leagues", lid)
			if err != nil || lg.GetString("inviteCode") == "GLOBAL" {
				continue
			}
			if n := unreadCount(app, lid, uid, lastReads[lid]); n > 0 {
				out[lid] = n
			}
		}
		return e.JSON(http.StatusOK, map[string]any{"unread": out})
	})
}

// authorize loads the league and confirms the caller may use its chat: it must
// be a private (non-Global) league the caller belongs to.
func authorize(app core.App, e *core.RequestEvent, leagueID string) (*core.Record, error) {
	lg, err := app.FindRecordById("leagues", leagueID)
	if err != nil {
		return nil, apis.NewNotFoundError("league not found", nil)
	}
	if lg.GetString("inviteCode") == "GLOBAL" {
		return nil, apis.NewForbiddenError("chat is not available in the Global league", nil)
	}
	if !isMember(app, leagueID, e.Auth.Id) {
		return nil, apis.NewForbiddenError("you are not a member of this league", nil)
	}
	return lg, nil
}

func isMember(app core.App, leagueID, userID string) bool {
	_, err := app.FindFirstRecordByFilter("league_members",
		"league = {:l} && user = {:u}", dbx.Params{"l": leagueID, "u": userID})
	return err == nil
}

// markRead upserts the caller's last-read marker for a league to now.
func markRead(app core.App, leagueID, userID string) {
	rec, err := app.FindFirstRecordByFilter(reads,
		"league = {:l} && user = {:u}", dbx.Params{"l": leagueID, "u": userID})
	if err != nil {
		col, err := app.FindCollectionByNameOrId(reads)
		if err != nil {
			return
		}
		rec = core.NewRecord(col)
		rec.Set("league", leagueID)
		rec.Set("user", userID)
	}
	rec.Set("lastRead", time.Now().UTC())
	_ = app.Save(rec)
}

// readMarkers returns leagueId -> last-read time string (PB format) for a user.
func readMarkers(app core.App, userID string) map[string]string {
	out := map[string]string{}
	recs, err := app.FindRecordsByFilter(reads, "user = {:u}", "", 0, 0, dbx.Params{"u": userID})
	if err != nil {
		return out
	}
	for _, r := range recs {
		out[r.GetString("league")] = r.GetDateTime("lastRead").String()
	}
	return out
}

// unreadCount counts messages in a league after `since` (PB datetime string;
// empty = all) that weren't sent by the user. Capped so a huge backlog stays a
// cheap query; the UI renders the cap as "99+".
func unreadCount(app core.App, leagueID, userID, since string) int {
	filter := "league = {:l} && user != {:u}"
	params := dbx.Params{"l": leagueID, "u": userID}
	if since != "" {
		filter += " && created > {:s}"
		params["s"] = since
	}
	recs, err := app.FindRecordsByFilter(messages, filter, "", 100, 0, params)
	if err != nil {
		return 0
	}
	return len(recs)
}

// msgView serialises a message. For soft-deleted messages the live text is
// already empty; `mod` (the requester is an app-admin) adds the original text +
// who/when for moderation.
func msgView(r *core.Record, mod bool) map[string]any {
	v := map[string]any{
		"id":      r.Id,
		"user":    r.GetString("user"),
		"text":    r.GetString("text"),
		"created": r.GetDateTime("created").Time().UTC().Format(time.RFC3339Nano),
	}
	if r.GetBool("deleted") {
		v["deleted"] = true
		if mod {
			v["original"] = r.GetString("origText")
			v["deletedBy"] = r.GetString("deletedBy")
			if dt := r.GetDateTime("deletedAt"); !dt.IsZero() {
				v["deletedAt"] = dt.Time().UTC().Format(time.RFC3339)
			}
		}
	}
	return v
}

func userView(u *core.Record) map[string]any {
	return map[string]any{
		"userId": u.Id,
		"name":   u.GetString("name"),
		"avatar": u.GetString("avatar"),
		"role":   u.GetString("role"),
	}
}
