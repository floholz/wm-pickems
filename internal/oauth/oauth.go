// Package oauth enables Google sign-in on the users collection from env
// (GOOGLE_CLIENT_ID / GOOGLE_CLIENT_SECRET) so the secret never lives in git
// or the DB config. Idempotent on every boot, so rotating the secret is just
// an env change + restart. No-op when the vars are unset.
package oauth

import (
	"log"
	"os"

	"github.com/pocketbase/pocketbase/core"
)

// Register wires Google OAuth2 onto the users auth collection if configured.
func Register(app core.App) {
	id := os.Getenv("GOOGLE_CLIENT_ID")
	secret := os.Getenv("GOOGLE_CLIENT_SECRET")
	if id == "" || secret == "" {
		log.Printf("[oauth] Google not configured (set GOOGLE_CLIENT_ID/SECRET)")
		return
	}

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		log.Printf("[oauth] users collection not found: %v", err)
		return
	}

	users.OAuth2.Enabled = true
	// Map the Google profile onto our fields (PocketBase downloads the
	// picture into the `avatar` file field — what the UI already reads).
	users.OAuth2.MappedFields.Name = "name"
	users.OAuth2.MappedFields.AvatarURL = "avatar"

	// Replace any existing google entry; keep other providers untouched.
	kept := users.OAuth2.Providers[:0]
	for _, p := range users.OAuth2.Providers {
		if p.Name != "google" {
			kept = append(kept, p)
		}
	}
	users.OAuth2.Providers = append(kept, core.OAuth2ProviderConfig{
		Name:         "google",
		DisplayName:  "Google",
		ClientId:     id,
		ClientSecret: secret,
	})

	if err := app.Save(users); err != nil {
		log.Printf("[oauth] failed to enable Google: %v", err)
		return
	}
	log.Printf("[oauth] Google sign-in enabled")
}
