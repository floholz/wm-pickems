package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// push_subscriptions stores Web Push endpoints (one row per browser/device).
// Owner-scoped so users manage only their own devices; the notify layer reads
// them with app-level access to deliver pushes. Unique on `endpoint` so the
// subscribe handler can upsert safely.
const nPushSubs = "push_subscriptions"

func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		if _, err := app.FindCollectionByNameOrId(nPushSubs); err == nil {
			return nil
		}
		ps := core.NewBaseCollection(nPushSubs)
		owner := "@request.auth.id = user"
		ps.ListRule = &owner
		ps.ViewRule = &owner
		ps.CreateRule = &owner
		ps.DeleteRule = &owner
		// No UpdateRule: subscriptions are immutable from the client (the
		// server upserts via the /api/push/subscribe handler).
		ps.Fields.Add(&core.RelationField{Name: "user", CollectionId: users.Id, MaxSelect: 1, Required: true, CascadeDelete: true})
		ps.Fields.Add(&core.TextField{Name: "endpoint", Required: true, Max: 1000})
		ps.Fields.Add(&core.TextField{Name: "p256dh", Required: true, Max: 255})
		ps.Fields.Add(&core.TextField{Name: "auth", Required: true, Max: 255})
		ps.Fields.Add(&core.TextField{Name: "userAgent", Max: 512})
		ps.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		ps.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		ps.AddIndex("idx_push_subs_endpoint", true, "endpoint", "")
		ps.AddIndex("idx_push_subs_user", false, "user", "")
		return app.Save(ps)
	}, func(app core.App) error {
		if c, err := app.FindCollectionByNameOrId(nPushSubs); err == nil {
			return app.Delete(c)
		}
		return nil
	})
}
