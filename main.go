// Command wm-pickems is a single-binary WC 2026 prediction app: it runs the
// PocketBase backend (auth + SQLite + REST) and serves the embedded SvelteKit
// SPA from the same process, so the whole thing ships as one Docker image.
package main

import (
	"io/fs"
	"log"
	"net/http"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
	"github.com/pocketbase/pocketbase/tools/hook"

	"github.com/floholz/wm-pickems/internal/dev"
	"github.com/floholz/wm-pickems/internal/forecast"
	"github.com/floholz/wm-pickems/internal/leagues"
	"github.com/floholz/wm-pickems/internal/notify"
	"github.com/floholz/wm-pickems/internal/oauth"
	"github.com/floholz/wm-pickems/internal/push"
	"github.com/floholz/wm-pickems/internal/scoring"
	"github.com/floholz/wm-pickems/internal/seed"
	wmsync "github.com/floholz/wm-pickems/internal/sync"
	"github.com/floholz/wm-pickems/internal/tips"
	"github.com/floholz/wm-pickems/internal/users"
	"github.com/floholz/wm-pickems/internal/web"
	_ "github.com/floholz/wm-pickems/migrations"
)

func main() {
	app := pocketbase.New()

	// Go-code migrations (collections/schema live in ./migrations).
	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
		TemplateLang: migratecmd.TemplateLangGo,
		Automigrate:  true,
	})

	// Seed teams/groups/fixtures from the embedded openfootball dataset on
	// first boot (idempotent).
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		if err := seed.Run(e.App); err != nil {
			return err
		}
		oauth.Register(e.App)
		users.Register(e.App)
		wmsync.Register(e.App, e)
		leagues.Register(e.App, e)
		tips.Register(e.App, e)
		forecast.Register(e.App, e)
		scoring.Register(e.App, e)
		push.Register(e.App, e)
		notify.Register(e.App, e)
		dev.Register(e.App, e)

		// Serve the web manifest with the correct MIME so it installs as a
		// proper PWA (apis.Static would send text/plain for .webmanifest).
		e.Router.GET("/manifest.webmanifest", func(re *core.RequestEvent) error {
			b, err := fs.ReadFile(web.DistFS(), "manifest.webmanifest")
			if err != nil {
				return apis.NewNotFoundError("", nil)
			}
			return re.Blob(200, "application/manifest+json", b)
		})
		return e.Next()
	})

	// Serve the embedded SvelteKit build with SPA (index.html) fallback so
	// client-side routes resolve. Registered last and only if no API/user
	// route already owns the path.
	app.OnServe().Bind(&hook.Handler[*core.ServeEvent]{
		Func: func(e *core.ServeEvent) error {
			if !e.Router.HasRoute(http.MethodGet, "/{path...}") {
				e.Router.GET("/{path...}", apis.Static(web.DistFS(), true))
			}
			return e.Next()
		},
		Priority: 999,
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
