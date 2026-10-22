// Package web embeds the built web app into the Dashboard binary.
//
// `npm run build` writes the app to dist/app; dist/.gitkeep keeps the embed
// pattern valid in a checkout where the app has not been built.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// App returns the built web app, or nil if it was not built before compiling.
func App() fs.FS {
	app, err := fs.Sub(dist, "dist/app")
	if err != nil {
		return nil
	}
	if _, err := fs.Stat(app, "index.html"); err != nil {
		return nil
	}
	return app
}
