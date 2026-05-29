package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Add leagues.privateCode: when true, only the league owner can see/share the
// invite code (the /api/leagues/mine handler blanks the code for non-owners).
// Defaults to false so existing leagues keep the original behavior where every
// member can see and share the code.
func init() {
	m.Register(func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("leagues")
		if err != nil {
			return err
		}
		if col.Fields.GetByName("privateCode") == nil {
			col.Fields.Add(&core.BoolField{Name: "privateCode"})
			return app.Save(col)
		}
		return nil
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("leagues")
		if err != nil {
			return err
		}
		if col.Fields.GetByName("privateCode") != nil {
			col.Fields.RemoveByName("privateCode")
			return app.Save(col)
		}
		return nil
	})
}
