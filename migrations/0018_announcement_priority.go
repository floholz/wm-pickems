package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Add an opt-in `highPriority` flag to announcements. When set, the broadcast's
// push is sent with high Web Push urgency (delivered promptly even on dozing /
// battery-saver devices) and asks the service worker to keep the notification
// on screen until acted on. Absent/false keeps the normal delivery.
func init() {
	m.Register(func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("announcements")
		if err != nil {
			return err
		}
		if col.Fields.GetByName("highPriority") == nil {
			col.Fields.Add(&core.BoolField{Name: "highPriority"})
			return app.Save(col)
		}
		return nil
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("announcements")
		if err != nil {
			return err
		}
		col.Fields.RemoveByName("highPriority")
		return app.Save(col)
	})
}
