// Command wm-pickems is a single-binary WC 2026 prediction app: it runs the
// PocketBase backend (auth + SQLite + REST) and serves the embedded SvelteKit
// SPA from the same process, so the whole thing ships as one Docker image.
package main

import (
	"log"
	"net/http"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
	"github.com/pocketbase/pocketbase/tools/hook"

	"github.com/floholz/wm-pickems/internal/web"
)

func main() {
	app := pocketbase.New()

	// Go-code migrations (collections/schema live in ./migrations).
	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
		TemplateLang: migratecmd.TemplateLangGo,
		Automigrate:  true,
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
