// Package users guards the privileged `role` / `botKind` marker fields on the
// users collection. PocketBase rules are per-collection (not per-field), so a
// plain users update rule can't stop a signed-in user from PATCHing themselves
// to role=admin. These request-scoped hooks fix that: any attempt to set or
// change role/botKind through the public API by a non-superuser is silently
// reverted to the stored value. The fields are therefore only ever settable
// from the PocketBase admin dashboard (which authenticates as a superuser).
package users

import "github.com/pocketbase/pocketbase/core"

// protected are the user fields only a superuser may write.
var protected = []string{"role", "botKind"}

// Role returns a user record's role, treating an empty/missing value as a plain
// member.
func Role(u *core.Record) string {
	if r := u.GetString("role"); r != "" {
		return r
	}
	return "member"
}

// IsAdmin reports whether the user is an app-level admin. This is distinct from
// a PocketBase superuser (the backend dashboard login); it gates SPA admin
// features for normal accounts flagged role=admin. Owner inherits admin — an
// owner is "basically an admin" with the extra owner-only surfaces on top.
func IsAdmin(u *core.Record) bool {
	r := u.GetString("role")
	return r == "admin" || r == "owner"
}

// IsOwner reports whether the user is the app owner (role=owner) — the
// super-admin that can see the owner stats page.
func IsOwner(u *core.Record) bool { return u.GetString("role") == "owner" }

// IsBot reports whether the user is a bot account.
func IsBot(u *core.Record) bool { return u.GetString("role") == "bot" }

// Register wires the field-protection hooks. Only request-scoped hooks fire
// here, so internal app.Save() calls (seed, league backfill, the dev bot
// generator) are unaffected — they may set role/botKind freely.
func Register(app core.App) {
	// On create: a non-superuser signup can never self-assign a role.
	app.OnRecordCreateRequest("users").BindFunc(func(e *core.RecordRequestEvent) error {
		if !e.HasSuperuserAuth() {
			for _, f := range protected {
				e.Record.Set(f, "")
			}
		}
		return e.Next()
	})
	// On update: revert any change to a protected field to its stored value.
	app.OnRecordUpdateRequest("users").BindFunc(func(e *core.RecordRequestEvent) error {
		if !e.HasSuperuserAuth() {
			if orig := e.Record.Original(); orig != nil {
				for _, f := range protected {
					e.Record.Set(f, orig.Get(f))
				}
			}
		}
		return e.Next()
	})
}
