package notify

import (
	"net/http"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/floholz/wm-pickems/internal/users"
)

// registerPolicy wires the global notification-policy endpoints:
//
//	GET  /api/notify/policy        any signed-in user — lets the settings UI grey
//	                               out toggles that admins have force-disabled.
//	PUT  /api/admin/notify/policy  owner/admin — flip the master channel switches
//	                               and per-event overrides at runtime (no redeploy).
//
// The policy lives in the same app_meta "notify_config" row as the other notify
// settings; writes merge into it so leadHours/allowlist/etc. are preserved.
func registerPolicy(app core.App, se *core.ServeEvent) {
	// Read — visible to any authenticated user (the settings page reads it).
	se.Router.GET("/api/notify/policy", func(e *core.RequestEvent) error {
		return e.JSON(http.StatusOK, policyView(readConfig(app)))
	}).Bind(apis.RequireAuth())

	// Write — owner/admin only.
	g := se.Router.Group("/api/admin/notify")
	g.Bind(apis.RequireAuth())
	g.BindFunc(func(e *core.RequestEvent) error {
		if e.Auth == nil || !users.IsAdmin(e.Auth) {
			return apis.NewForbiddenError("admin only", nil)
		}
		return e.Next()
	})

	g.PUT("/policy", func(e *core.RequestEvent) error {
		var body policyPayload
		if err := e.BindBody(&body); err != nil {
			return apis.NewBadRequestError(err.Error(), nil)
		}
		if err := writePolicy(app, body); err != nil {
			return apis.NewApiError(http.StatusInternalServerError, err.Error(), nil)
		}
		return e.JSON(http.StatusOK, policyView(readConfig(app)))
	})
}

// policyPayload is the PUT body. All fields optional: an absent section is left
// untouched, so the client can patch just the master switches or just overrides.
type policyPayload struct {
	Channels *storedChannels            `json:"channels"`
	Disabled map[string]map[string]bool `json:"disabled"`
}

// policyView is the wire shape returned to clients (resolved, defaults applied).
func policyView(c config) map[string]any {
	disabled := c.Disabled
	if disabled == nil {
		disabled = map[string]map[string]bool{}
	}
	return map[string]any{
		"channels": map[string]bool{"email": c.Channels.Email, "push": c.Channels.Push},
		"disabled": disabled,
	}
}

// writePolicy merges the payload into the app_meta notify_config row, preserving
// every other field. The row is seeded on boot (seedNotifyConfig), so it exists.
func writePolicy(app core.App, body policyPayload) error {
	rec, err := app.FindFirstRecordByFilter("app_meta",
		"key = {:k}", map[string]any{"k": metaKey})
	if err != nil {
		return err
	}
	// Decode the existing value into a generic map so unrelated keys survive.
	value := map[string]any{}
	if err := rec.UnmarshalJSONField("value", &value); err != nil {
		return err
	}

	if body.Channels != nil {
		ch, _ := value["channels"].(map[string]any)
		if ch == nil {
			ch = map[string]any{}
		}
		if body.Channels.Email != nil {
			ch["email"] = *body.Channels.Email
		}
		if body.Channels.Push != nil {
			ch["push"] = *body.Channels.Push
		}
		value["channels"] = ch
	}

	if body.Disabled != nil {
		value["disabled"] = pruneDisabled(body.Disabled)
	}

	rec.Set("value", value)
	return app.Save(rec)
}

// pruneDisabled drops false/empty entries so the stored map only ever lists the
// channels that are actually suppressed (keeps the row tidy and the intent clear).
func pruneDisabled(in map[string]map[string]bool) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for event, channels := range in {
		kept := map[string]bool{}
		for ch, off := range channels {
			if off {
				kept[ch] = true
			}
		}
		if len(kept) > 0 {
			out[event] = kept
		}
	}
	return out
}
