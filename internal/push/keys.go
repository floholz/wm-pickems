// Package push is the Web Push channel: VAPID key management, a Sender that
// delivers encrypted pushes to browser endpoints, and the subscription
// endpoints the frontend uses to opt a device in/out. The notify layer drives
// it through the shared Sender interface, exactly like the email mailer.
package push

import (
	"log"
	"os"
	"strings"
	"sync"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/pocketbase/pocketbase/core"
)

const metaKey = "vapid_keys"

// Keys is a VAPID keypair plus the JWT subject (a mailto: or https: contact).
type Keys struct {
	Public  string `json:"public"`
	Private string `json:"private"`
	Subject string `json:"subject"`
}

var (
	keysOnce sync.Once
	keysVal  Keys
)

// ResolveKeys returns the app's VAPID keys, resolving them once per process:
// env vars first (VAPID_PUBLIC_KEY / VAPID_PRIVATE_KEY / VAPID_SUBJECT), then a
// persisted app_meta row, otherwise a freshly generated pair persisted back so
// push works out of the box. Pin the env vars to keep keys stable across a
// pb_data wipe (regenerated keys invalidate existing subscriptions).
func ResolveKeys(app core.App) Keys {
	keysOnce.Do(func() {
		keysVal = resolveKeys(app)
	})
	return keysVal
}

func resolveKeys(app core.App) Keys {
	subject := os.Getenv("VAPID_SUBJECT")
	if subject == "" {
		if addr := app.Settings().Meta.SenderAddress; addr != "" {
			subject = "mailto:" + addr
		} else {
			subject = "mailto:admin@localhost"
		}
	}

	if pub, priv := os.Getenv("VAPID_PUBLIC_KEY"), os.Getenv("VAPID_PRIVATE_KEY"); pub != "" && priv != "" {
		log.Printf("[push] using VAPID keys from env")
		return Keys{Public: pub, Private: priv, Subject: subject}
	}

	if rec, err := app.FindFirstRecordByFilter("app_meta",
		"key = {:k}", map[string]any{"k": metaKey}); err == nil {
		var stored Keys
		if err := rec.UnmarshalJSONField("value", &stored); err == nil &&
			stored.Public != "" && stored.Private != "" {
			stored.Subject = subject // subject can still come from env/settings
			return stored
		}
	}

	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		log.Printf("[push] could not generate VAPID keys: %v — push disabled", err)
		return Keys{Subject: subject}
	}
	k := Keys{Public: pub, Private: priv, Subject: subject}
	if err := persistKeys(app, k); err != nil {
		log.Printf("[push] could not persist VAPID keys: %v", err)
	} else {
		log.Printf("[push] generated and stored a new VAPID keypair")
	}
	return k
}

func persistKeys(app core.App, k Keys) error {
	col, err := app.FindCollectionByNameOrId("app_meta")
	if err != nil {
		return err
	}
	rec := core.NewRecord(col)
	rec.Set("key", metaKey)
	rec.Set("value", map[string]any{"public": k.Public, "private": k.Private})
	return app.Save(rec)
}

// Enabled reports whether push can actually be sent (a keypair is available).
func (k Keys) Enabled() bool {
	return k.Public != "" && k.Private != ""
}

// normalizeSubject ensures the VAPID subject has a scheme webpush accepts.
func normalizeSubject(s string) string {
	if s == "" {
		return "mailto:admin@localhost"
	}
	if !strings.HasPrefix(s, "mailto:") && !strings.HasPrefix(s, "http") {
		return "mailto:" + s
	}
	return s
}
