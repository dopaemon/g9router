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
		if r.URL.Path == "/" || r.URL.Path == "/dashboard" || r.URL.Path == "/dashboard/" || r.URL.Path == "/login" {
			r.URL.Path = "/index.html"
		}
		if _, err := fs.Stat(files, r.URL.Path[1:]); err != nil {
			r.URL.Path = "/index.html"
		}
		static.ServeHTTP(w, r)
	})
}
