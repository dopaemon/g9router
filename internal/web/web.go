package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed index.html
var files embed.FS

func Handler() http.Handler {
	static := http.FileServer(http.FS(files))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" || path == "/dashboard" || path == "/dashboard/" || path == "/login" {
			serveIndex(w)
			return
		}
		if _, err := fs.Stat(files, path[1:]); err != nil {
			serveIndex(w)
			return
		}
		static.ServeHTTP(w, r)
	})
}

func serveIndex(w http.ResponseWriter) {
	data, err := files.ReadFile("index.html")
	if err != nil {
		http.Error(w, "dashboard unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}
