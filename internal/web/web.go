package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
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
	page := strings.Replace(string(data), "</body>", `<script>(async()=>{try{const r=await fetch('/api/cli-tools/guides');if(!r.ok)return;const data=await r.json(),tools=data.tools||[],select=document.querySelector('#cliTool');if(!select)return;const card=document.createElement('section');card.className='card full';card.innerHTML='<h2>CLI tool guide</h2><p class="muted">Source-backed tool metadata</p><pre id="cliGuideMetadata"></pre>';document.querySelector('main')?.appendChild(card);const render=()=>{const tool=tools.find(item=>item.id===select.value);document.querySelector('#cliGuideMetadata').textContent=tool?JSON.stringify(tool,null,2):''};select.addEventListener('change',render);render()}catch{}})()</script></body>`, 1)
	_, _ = w.Write([]byte(page))
}
