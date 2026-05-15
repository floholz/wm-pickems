package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Tighten Forecast visibility to own-only at the collection API. Other
// members' Forecasts are exposed only after the tournament lock via a custom
// endpoint (added with the scoring/leaderboard phase), mirroring Tips.
func init() {
	m.Register(func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("forecasts")
		if err != nil {
			return err
		}
		own := "user = @request.auth.id"
		c.ListRule = types.Pointer(own)
		c.ViewRule = types.Pointer(own)
		c.CreateRule = types.Pointer(own)
		c.UpdateRule = types.Pointer(own)
		c.DeleteRule = types.Pointer(own)
		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("forecasts")
		if err != nil {
			return err
		}
		authed := "@request.auth.id != ''"
		c.ListRule = types.Pointer(authed)
		c.ViewRule = types.Pointer(authed)
		return app.Save(c)
	})
}
