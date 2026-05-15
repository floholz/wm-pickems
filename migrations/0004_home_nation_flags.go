package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"

	"github.com/floholz/wm-pickems/internal/seed"
)

// Backfill iso2 for UK home nations (England/Scotland/Wales/N.Ireland) on
// databases seeded before the flag fix — their flag emoji are tag-sequences,
// not regional indicators, so iso2 was left blank and fell back to a code chip.
func init() {
	m.Register(func(app core.App) error {
		// Widen iso2 (originally Max 2) so "gb-sct"/"gb-eng" fit. No-op if a
		// fresh DB already created it wide.
		if col, err := app.FindCollectionByNameOrId("teams"); err == nil {
			if f, ok := col.Fields.GetByName("iso2").(*core.TextField); ok && f.Max < 8 {
				f.Max = 8
				if err := app.Save(col); err != nil {
					return err
				}
			}
		}
		for code, iso := range seed.HomeNationISO {
			rec, err := app.FindFirstRecordByFilter("teams",
				"fifaCode = {:c}", map[string]any{"c": code})
			if err != nil {
				continue // team not present (qualification slot) — skip
			}
			rec.Set("iso2", iso)
			if err := app.Save(rec); err != nil {
				return err
			}
		}
		return nil
	}, func(app core.App) error {
		return nil // non-destructive; nothing to undo
	})
}
