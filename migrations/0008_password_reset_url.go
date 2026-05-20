package migrations

import (
	"strings"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Point the reset-password email at the SPA route so users land on a branded
// /confirm-password-reset/<token> page rather than PocketBase's admin UI at
// /_/#/auth/confirm-password-reset/<token>. The token + behavior is identical
// — only the link destination differs.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		before := users.ResetPasswordTemplate.Body
		after := strings.ReplaceAll(before,
			"/_/#/auth/confirm-password-reset/",
			"/confirm-password-reset/",
		)
		if before == after {
			return nil
		}
		users.ResetPasswordTemplate.Body = after
		return app.Save(users)
	}, func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		before := users.ResetPasswordTemplate.Body
		after := strings.ReplaceAll(before,
			"/confirm-password-reset/",
			"/_/#/auth/confirm-password-reset/",
		)
		if before == after {
			return nil
		}
		users.ResetPasswordTemplate.Body = after
		return app.Save(users)
	})
}
