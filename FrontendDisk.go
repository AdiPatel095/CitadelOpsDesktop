//go:build !desktop

package main

import (
	"net/http"
	"os"
)

const desktopBuild = false

func frontendHandler(directory string) http.Handler {
	assets := os.DirFS(directory)
	index, err := assets.Open("index.html")
	if err != nil {
		return missingFrontendHandler()
	}
	_ = index.Close()
	return frontendFileHandler(assets)
}
