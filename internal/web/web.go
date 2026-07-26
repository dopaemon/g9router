package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed index.html favicon.svg manifest.json icon-192.svg icon-512.svg
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
	page := strings.Replace(string(data), "</body>", webI18nScript+`<script>(async()=>{try{const r=await fetch('/api/cli-tools/guides');if(!r.ok)return;const data=await r.json(),tools=data.tools||[],select=document.querySelector('#cliTool');if(!select)return;tools.forEach(tool=>{if(!select.querySelector('option[value="'+tool.id+'"]')){const option=document.createElement('option');option.value=tool.id;option.textContent=tool.name;select.appendChild(option)}});const card=document.createElement('section');card.className='card full';card.innerHTML='<h2>CLI tool guide</h2><p class="muted">Source-backed tool metadata</p><div id="cliGuideContent"></div><pre id="cliGuideMetadata" hidden></pre>';document.querySelector('main')?.appendChild(card);const render=()=>{const tool=tools.find(item=>item.id===select.value),content=document.querySelector('#cliGuideContent'),metadata=document.querySelector('#cliGuideMetadata');content.replaceChildren();metadata.textContent=tool?JSON.stringify(tool,null,2):'';if(!tool)return;(tool.guideSteps||[]).forEach(step=>{const row=document.createElement('p');row.textContent=(step.step||'')+'. '+(step.title||'')+(step.desc?' — '+step.desc:'')+(step.value?' ['+step.value+']':'');content.appendChild(row)});if(tool.codeBlock){const pre=document.createElement('pre');pre.textContent=tool.codeBlock.code||'';content.appendChild(pre)}};select.addEventListener('change',render);render()}catch{}})()</script></body>`, 1)
	_, _ = w.Write([]byte(page))
}

const webI18nScript = `<script>(function(){const vi={"Basic Chat":"Trò chuyện cơ bản","Providers":"Nhà cung cấp","CLI Tools":"Công cụ CLI","Combos":"Combo","Usage":"Sử dụng","Endpoint":"Endpoint","Media":"Media","Media Web":"Media Web","Console Log":"Console log","Translator":"Dịch","Token Saver":"Tiết kiệm token","Quota":"Hạn mức","Skills":"Kỹ năng","Profile":"Hồ sơ","Refresh":"Làm mới","Health JSON":"JSON sức khỏe","Access":"Truy cập","Runtime":"Runtime","Quick chat":"Chat nhanh","Requests":"Request","Errors":"Lỗi","Input tokens":"Token đầu vào","Output tokens":"Token đầu ra","Login":"Đăng nhập","Logout":"Đăng xuất","Reset password":"Đặt lại mật khẩu","Change password":"Đổi mật khẩu","Dashboard password":"Mật khẩu dashboard","Current password":"Mật khẩu hiện tại","New password":"Mật khẩu mới","Loading…":"Đang tải…","Display language":"Ngôn ngữ hiển thị"};const locale=decodeURIComponent((document.cookie.match(/(?:^|; )locale=([^;]+)/)||[])[1]||'en');if(locale!=='vi')return;const walker=document.createTreeWalker(document.body,NodeFilter.SHOW_TEXT);while(walker.nextNode()){const node=walker.currentNode,key=node.nodeValue.trim();if(vi[key])node.nodeValue=node.nodeValue.replace(key,vi[key])}document.querySelectorAll('[placeholder],[aria-label]').forEach(node=>{['placeholder','aria-label'].forEach(attr=>{if(vi[node.getAttribute(attr)])node.setAttribute(attr,vi[node.getAttribute(attr)])})});const select=document.querySelector('#localeSelect');if(select)select.value='vi'})()</script>`
