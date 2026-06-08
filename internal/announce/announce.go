// Package announce serves the in-app announcement / broadcast feature: short
// messages an owner/admin writes from the Admin console that surface as a
// dismissible banner for every signed-in user, and can optionally be fanned out
// as a real email/push notification via internal/notify.
//
// Reads (the active banner list) ride the collection's own list rule, but every
// mutation goes through these owner/admin-gated endpoints so the privileged
// fields can never be written from an ordinary account.
package announce

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/floholz/wm-pickems/internal/notify"
	"github.com/floholz/wm-pickems/internal/users"
)

const collection = "announcements"

var levels = map[string]bool{"info": true, "success": true, "warn": true}

// Register wires the announcement endpoints. The admin group is gated to
// owner/admin accounts; the active-list endpoint is open to any signed-in user
// (the banner reads it).
func Register(app core.App, se *core.ServeEvent) {
	// GET /api/announce/active — active announcements for the in-app banner.
	se.Router.GET("/api/announce/active", func(e *core.RequestEvent) error {
		recs, err := app.FindRecordsByFilter(collection, "active = true", "-created", 0, 0)
		if err != nil {
			return err
		}
		out := make([]map[string]any, 0, len(recs))
		for _, r := range recs {
			out = append(out, view(r))
		}
		return e.JSON(http.StatusOK, map[string]any{"announcements": out})
	}).Bind(apis.RequireAuth())

	g := se.Router.Group("/api/admin/announce")
	g.Bind(apis.RequireAuth())
	// Owner/admin gate for every mutation + the full list.
	g.BindFunc(func(e *core.RequestEvent) error {
		if e.Auth == nil || !users.IsAdmin(e.Auth) {
			return apis.NewForbiddenError("admin only", nil)
		}
		return e.Next()
	})

	// GET /api/admin/announce — every announcement (newest first) for the console.
	g.GET("", func(e *core.RequestEvent) error {
		recs, err := app.FindRecordsByFilter(collection, "id != ''", "-created", 0, 0)
		if err != nil {
			return err
		}
		out := make([]map[string]any, 0, len(recs))
		for _, r := range recs {
			out = append(out, view(r))
		}
		return e.JSON(http.StatusOK, map[string]any{"announcements": out})
	})

	// POST /api/admin/announce — create.
	g.POST("", func(e *core.RequestEvent) error {
		var body payload
		if err := e.BindBody(&body); err != nil {
			return apis.NewBadRequestError(err.Error(), nil)
		}
		title, bodyText, level, err := body.normalize()
		if err != nil {
			return apis.NewBadRequestError(err.Error(), nil)
		}
		col, err := app.FindCollectionByNameOrId(collection)
		if err != nil {
			return err
		}
		rec := core.NewRecord(col)
		rec.Set("title", title)
		rec.Set("body", bodyText)
		rec.Set("level", level)
		rec.Set("active", body.activeOrDefault())
		if err := app.Save(rec); err != nil {
			return err
		}
		return e.JSON(http.StatusOK, view(rec))
	})

	// POST /api/admin/announce/{id} — update (edit fields and/or toggle active).
	g.POST("/{id}", func(e *core.RequestEvent) error {
		rec, err := find(app, e.Request.PathValue("id"))
		if err != nil {
			return err
		}
		var body payload
		if err := e.BindBody(&body); err != nil {
			return apis.NewBadRequestError(err.Error(), nil)
		}
		if body.Title != nil || body.Body != nil || body.Level != nil {
			title, bodyText, level, err := body.normalize()
			if err != nil {
				return apis.NewBadRequestError(err.Error(), nil)
			}
			rec.Set("title", title)
			rec.Set("body", bodyText)
			rec.Set("level", level)
		}
		if body.Active != nil {
			rec.Set("active", *body.Active)
		}
		if err := app.Save(rec); err != nil {
			return err
		}
		return e.JSON(http.StatusOK, view(rec))
	})

	// DELETE /api/admin/announce/{id} — delete.
	g.DELETE("/{id}", func(e *core.RequestEvent) error {
		rec, err := find(app, e.Request.PathValue("id"))
		if err != nil {
			return err
		}
		if err := app.Delete(rec); err != nil {
			return err
		}
		return e.JSON(http.StatusOK, map[string]any{"ok": true})
	})

	// POST /api/admin/announce/{id}/send — also send as email/push. Idempotent:
	// the notify dedup ledger keys off the announcement id, so a re-send is a
	// no-op rather than a re-spam.
	g.POST("/{id}/send", func(e *core.RequestEvent) error {
		rec, err := find(app, e.Request.PathValue("id"))
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(e.Request.Context(), 90*time.Second)
		defer cancel()
		res, err := notify.Broadcast(ctx, app, rec)
		if err != nil {
			return apis.NewApiError(http.StatusInternalServerError, err.Error(), nil)
		}
		if rec.GetDateTime("notifiedAt").IsZero() {
			rec.Set("notifiedAt", time.Now().UTC())
			_ = app.Save(rec)
		}
		return e.JSON(http.StatusOK, map[string]any{
			"announcement": view(rec),
			"result":       res,
		})
	})
}

// payload is the create/update body. Pointers distinguish "absent" (update keeps
// the stored value) from an explicit value.
type payload struct {
	Title  *string `json:"title"`
	Body   *string `json:"body"`
	Level  *string `json:"level"`
	Active *bool   `json:"active"`
}

func (p payload) normalize() (title, body, level string, err error) {
	title = strings.TrimSpace(deref(p.Title))
	body = strings.TrimSpace(deref(p.Body))
	level = strings.TrimSpace(deref(p.Level))
	if level == "" {
		level = "info"
	}
	if title == "" || body == "" {
		return "", "", "", errMsg("title and body are required")
	}
	if !levels[level] {
		return "", "", "", errMsg("invalid level")
	}
	return title, body, level, nil
}

// activeOrDefault defaults new announcements to active so creating one is enough
// to publish it (the common case).
func (p payload) activeOrDefault() bool {
	if p.Active != nil {
		return *p.Active
	}
	return true
}

func find(app core.App, id string) (*core.Record, error) {
	rec, err := app.FindRecordById(collection, id)
	if err != nil {
		return nil, apis.NewNotFoundError("announcement not found", nil)
	}
	return rec, nil
}

func view(r *core.Record) map[string]any {
	return map[string]any{
		"id":         r.Id,
		"title":      r.GetString("title"),
		"body":       r.GetString("body"),
		"level":      r.GetString("level"),
		"active":     r.GetBool("active"),
		"notifiedAt": notifiedAt(r),
		"created":    r.GetDateTime("created").Time().UTC().Format(time.RFC3339),
	}
}

func notifiedAt(r *core.Record) string {
	if dt := r.GetDateTime("notifiedAt"); !dt.IsZero() {
		return dt.Time().UTC().Format(time.RFC3339)
	}
	return ""
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

type errMsg string

func (e errMsg) Error() string { return string(e) }
