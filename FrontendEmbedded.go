//go:build desktop

package main

import (
	"embed"
	"io/fs"
	"net/http"
)

const desktopBuild = true

//go:embed all:Client/dist
var embeddedFrontend embed.FS

func frontendHandler(_ string) http.Handler {
	assets, err := fs.Sub(embeddedFrontend, "Client/dist")
	if err != nil {
		return missingFrontendHandler()
	}
	return frontendFileHandler(assets)
}
