package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// announcements are short in-app broadcast messages an owner/admin writes from
// the Admin console. Active ones surface as a dismissible banner for every
// signed-in user (read via the public API under the active-only list rule); an
// owner can optionally also fan one out as a real email/push notification.
//
// Writes (create/update/delete) have no collection rule — they're superuser-only
// at the PB layer and only ever happen through the owner/admin-gated Go
// endpoints in internal/announce. Reads are gated to authed users seeing active
// rows, so the banner can fetch them directly.
const nAnnouncements = "announcements"

func init() {
	m.Register(func(app core.App) error {
		if _, err := app.FindCollectionByNameOrId(nAnnouncements); err == nil {
			return nil
		}
		a := core.NewBaseCollection(nAnnouncements)
		// Any signed-in user may list/view active announcements (for the banner).
		read := "@request.auth.id != '' && active = true"
		a.ListRule = &read
		a.ViewRule = &read
		// Create/update/delete intentionally left nil = superuser-only; all
		// mutations go through the owner/admin-gated endpoints in internal/announce.
		a.Fields.Add(&core.TextField{Name: "title", Required: true, Max: 120})
		a.Fields.Add(&core.TextField{Name: "body", Required: true, Max: 2000})
		a.Fields.Add(&core.SelectField{Name: "level", Required: true, MaxSelect: 1, Values: []string{"info", "success", "warn"}})
		a.Fields.Add(&core.BoolField{Name: "active"})
		// notifiedAt is stamped when the announcement is broadcast as a real
		// email/push, so the admin UI can show it's already been sent.
		a.Fields.Add(&core.DateField{Name: "notifiedAt"})
		a.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		a.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		a.AddIndex("idx_announcements_active", false, "active", "")
		return app.Save(a)
	}, func(app core.App) error {
		if c, err := app.FindCollectionByNameOrId(nAnnouncements); err == nil {
			return app.Delete(c)
		}
		return nil
	})
}
