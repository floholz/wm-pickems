// Package web embeds the compiled SvelteKit SPA so the Go binary can serve
// the frontend without any external files. The Docker build populates
// web/build via `npm run build` before `go build`; the placeholder committed
// to the repo keeps `go build` working without a frontend build present.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:build
var buildFS embed.FS

// DistFS returns the SvelteKit build output rooted at its top level, suitable
// for apis.Static.
func DistFS() fs.FS {
	sub, err := fs.Sub(buildFS, "build")
	if err != nil {
		panic(err)
	}
	return sub
}
