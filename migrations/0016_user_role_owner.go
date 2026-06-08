package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Add an "owner" value to the users.role marker. Owner is an app-level
// super-admin — it unlocks the owner stats page and (like admin) any admin-only
// affordance. As with admin/bot it is only settable from the PocketBase
// dashboard; the internal/users request hooks block changing it via the API.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		if f, ok := users.Fields.GetByName("role").(*core.SelectField); ok {
			f.Values = []string{"admin", "bot", "owner"}
		}
		return app.Save(users)
	}, func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		if f, ok := users.Fields.GetByName("role").(*core.SelectField); ok {
			f.Values = []string{"admin", "bot"}
		}
		return app.Save(users)
	})
}
