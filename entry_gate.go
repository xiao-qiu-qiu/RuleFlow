package main

import (
	"crypto/subtle"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// entryCookieName 是入口 cookie 的名字。
// 面板前端内部跳转、/api 与 /assets 都用绝对路径，如果只靠“路径前缀”放行，
// 浏览器一跳出前缀就会看到伪装页；因此首次通过入口路径访问时写一个 cookie，
// 后续请求凭这个 cookie 放行，前端无需改动。
const entryCookieName = "rf_entry"

// entryCookieMaxAge 入口 cookie 有效期（30 天）
const entryCookieMaxAge = 60 * 60 * 24 * 30

// entryGate 实现「随机入口路径 + 伪装页」：
//
//   - 请求路径等于入口前缀或以它开头：去掉前缀后交给正常路由，并写入入口 cookie
//   - 带有效入口 cookie：直接交给正常路由（面板内部绝对路径请求靠这条放行）
//   - /health：豁免，便于容器与外部探活
//   - 其余请求：返回伪装页（看起来像普通静态站点，不暴露任何 RuleFlow 特征）
//
// 静态文件类路径（.js/.css/.ico/.map 等）返回 404，避免“把 HTML 当图片/脚本返回”这种不自然的行为。
type entryGate struct {
	next       http.Handler
	prefix     string
	cookie     string
	cover      []byte
	coverType  string
	healthPath string
}

func newEntryGate(next http.Handler, prefix, cookie string, cover []byte) http.Handler {
	if cookie == "" {
		cookie = strings.Trim(prefix, "/")
	}
	return &entryGate{
		next:       next,
		prefix:     prefix,
		cookie:     cookie,
		cover:      cover,
		coverType:  "text/html; charset=utf-8",
		healthPath: "/health",
	}
}

func (g *entryGate) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	reqPath := r.URL.Path

	// 1) 入口路径：剥掉前缀放行，并写 cookie
	if reqPath == g.prefix || strings.HasPrefix(reqPath, g.prefix+"/") {
		g.setEntryCookie(w, r)
		g.next.ServeHTTP(w, stripEntryPrefix(r, g.prefix))
		return
	}

	// 2) 入口 cookie 放行
	if c, err := r.Cookie(entryCookieName); err == nil && secureEqual(c.Value, g.cookie) {
		g.next.ServeHTTP(w, r)
		return
	}

	// 3) 探活豁免
	if reqPath == g.healthPath {
		g.next.ServeHTTP(w, r)
		return
	}

	// 4) 伪装页
	if looksLikeStaticAsset(reqPath) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", g.coverType)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(g.cover)
}

func (g *entryGate) setEntryCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     entryCookieName,
		Value:    g.cookie,
		Path:     "/",
		MaxAge:   entryCookieMaxAge,
		HttpOnly: true,
		Secure:   requestIsHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
}

// stripEntryPrefix 返回去掉入口前缀的浅拷贝请求（保留 Host、Header、Query）
func stripEntryPrefix(r *http.Request, prefix string) *http.Request {
	clone := r.Clone(r.Context())
	stripped := strings.TrimPrefix(r.URL.Path, prefix)
	if stripped == "" {
		stripped = "/"
	}
	clone.URL.Path = stripped
	if r.URL.RawPath != "" {
		clone.URL.RawPath = strings.TrimPrefix(r.URL.RawPath, prefix)
		if clone.URL.RawPath == "" {
			clone.URL.RawPath = stripped
		}
	}
	return clone
}

func requestIsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	proto := firstForwardedValue(r.Header.Get("X-Forwarded-Proto"))
	return strings.EqualFold(proto, "https")
}

func firstForwardedValue(value string) string {
	if value == "" {
		return ""
	}
	parts := strings.Split(value, ",")
	return strings.TrimSpace(parts[0])
}

func secureEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// staticAssetExts 是被当作静态资源、缺失时直接 404 的扩展名
var staticAssetExts = map[string]struct{}{
	".js": {}, ".mjs": {}, ".css": {}, ".map": {}, ".json": {},
	".ico": {}, ".png": {}, ".jpg": {}, ".jpeg": {}, ".gif": {}, ".webp": {},
	".svg": {}, ".woff": {}, ".woff2": {}, ".ttf": {}, ".eot": {},
	".txt": {}, ".xml": {}, ".webmanifest": {},
}

func looksLikeStaticAsset(reqPath string) bool {
	ext := strings.ToLower(path.Ext(reqPath))
	if ext == "" {
		return false
	}
	_, ok := staticAssetExts[ext]
	return ok
}

// loadCoverPage 读取内嵌的伪装页
func loadCoverPage() ([]byte, error) {
	return fs.ReadFile(coverFS, "cover/index.html")
}
