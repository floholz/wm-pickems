package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"

	"github.com/floholz/wm-pickems/internal/seed"
)

// Rewrite the default scoring config to the v2 rules: max 6 per game (no
// separate advancer / extra-time bonus; KO "correct result" = who advances),
// and Forecast "+advance per correctly-predicted advancer". Existing DBs
// seeded with the old shape are updated in place.
func init() {
	m.Register(func(app core.App) error {
		rec, err := app.FindFirstRecordByFilter("scoring_configs", "isDefault = true")
		if err != nil {
			return nil // not seeded yet; seed will write the new shape
		}
		rec.Set("config", seed.DefaultScoringConfig)
		return app.Save(rec)
	}, func(app core.App) error {
		return nil // non-destructive
	})
}
