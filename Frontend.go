package main

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"

	"CitadelDesktop/Server/App"
)

func frontendFileHandler(assets fs.FS) http.Handler {
	files := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		path := strings.TrimPrefix(request.URL.Path, "/")
		if path != "" {
			if info, err := fs.Stat(assets, path); err == nil && !info.IsDir() {
				files.ServeHTTP(writer, request)
				return
			}
		}
		index, err := fs.ReadFile(assets, "index.html")
		if err != nil {
			missingFrontendHandler().ServeHTTP(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = writer.Write(index)
	})
}

func missingFrontendHandler() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"name": "CitadelOps", "version": App.Version,
			"detail": "Build Client/ or run the Vite development server for the desktop UI.",
		})
	})
}
