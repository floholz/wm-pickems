package push

import (
	"net/http"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

const collection = "push_subscriptions"

// Register resolves the VAPID keys (generating them on first boot) and wires the
// subscription endpoints used by the frontend push store.
func Register(app core.App, se *core.ServeEvent) {
	ResolveKeys(app) // resolve/generate + log once at boot

	// Public: the browser needs the VAPID public key to subscribe.
	se.Router.GET("/api/push/key", func(e *core.RequestEvent) error {
		return e.JSON(http.StatusOK, map[string]string{"publicKey": ResolveKeys(app).Public})
	})

	// Upsert a subscription for the authenticated user.
	se.Router.POST("/api/push/subscribe", func(e *core.RequestEvent) error {
		var body struct {
			Endpoint string `json:"endpoint"`
			Keys     struct {
				P256dh string `json:"p256dh"`
				Auth   string `json:"auth"`
			} `json:"keys"`
		}
		if err := e.BindBody(&body); err != nil {
			return e.JSON(400, map[string]string{"error": err.Error()})
		}
		if body.Endpoint == "" || body.Keys.P256dh == "" || body.Keys.Auth == "" {
			return e.JSON(400, map[string]string{"error": "incomplete subscription"})
		}
		rec, err := app.FindFirstRecordByFilter(collection,
			"endpoint = {:e}", map[string]any{"e": body.Endpoint})
		if err != nil {
			col, err := app.FindCollectionByNameOrId(collection)
			if err != nil {
				return e.JSON(500, map[string]string{"error": err.Error()})
			}
			rec = core.NewRecord(col)
			rec.Set("endpoint", body.Endpoint)
		}
		rec.Set("user", e.Auth.Id)
		rec.Set("p256dh", body.Keys.P256dh)
		rec.Set("auth", body.Keys.Auth)
		rec.Set("userAgent", e.Request.UserAgent())
		if err := app.Save(rec); err != nil {
			return e.JSON(500, map[string]string{"error": err.Error()})
		}
		return e.JSON(http.StatusOK, map[string]string{"status": "ok"})
	}).Bind(apis.RequireAuth())

	// Remove a subscription (only the caller's own).
	se.Router.POST("/api/push/unsubscribe", func(e *core.RequestEvent) error {
		var body struct {
			Endpoint string `json:"endpoint"`
		}
		if err := e.BindBody(&body); err != nil {
			return e.JSON(400, map[string]string{"error": err.Error()})
		}
		if rec, err := app.FindFirstRecordByFilter(collection,
			"endpoint = {:e} && user = {:u}",
			map[string]any{"e": body.Endpoint, "u": e.Auth.Id}); err == nil {
			_ = app.Delete(rec)
		}
		return e.JSON(http.StatusOK, map[string]string{"status": "ok"})
	}).Bind(apis.RequireAuth())
}

// Subscriptions returns all push subscriptions for a user.
func Subscriptions(app core.App, userID string) ([]Subscription, error) {
	recs, err := app.FindRecordsByFilter(collection,
		"user = {:u}", "", 0, 0, map[string]any{"u": userID})
	if err != nil {
		return nil, err
	}
	out := make([]Subscription, 0, len(recs))
	for _, r := range recs {
		out = append(out, Subscription{
			Endpoint: r.GetString("endpoint"),
			P256dh:   r.GetString("p256dh"),
			Auth:     r.GetString("auth"),
		})
	}
	return out, nil
}

// PruneEndpoint deletes a dead subscription by endpoint (best effort).
func PruneEndpoint(app core.App, endpoint string) {
	if rec, err := app.FindFirstRecordByFilter(collection,
		"endpoint = {:e}", map[string]any{"e": endpoint}); err == nil {
		_ = app.Delete(rec)
	}
}
