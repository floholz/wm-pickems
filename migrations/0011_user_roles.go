package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Add a single `role` marker to users (member | admin | bot) plus `botKind`
// (which bot, e.g. "claude"). The field is left empty for existing/new users —
// empty is treated as a plain member everywhere — and is only meant to be set
// from the PocketBase admin dashboard. A request hook in internal/users
// prevents non-superusers from changing it via the public API, so the marker
// (and any future admin-only feature gated on it) can be trusted.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		if users.Fields.GetByName("role") == nil {
			users.Fields.Add(&core.SelectField{
				Name:      "role",
				MaxSelect: 1,
				Values:    []string{"member", "admin", "bot"},
			})
		}
		if users.Fields.GetByName("botKind") == nil {
			users.Fields.Add(&core.TextField{Name: "botKind", Max: 32})
		}
		return app.Save(users)
	}, func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		users.Fields.RemoveByName("role")
		users.Fields.RemoveByName("botKind")
		return app.Save(users)
	})
}
