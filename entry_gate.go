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
// 浏览器一跳出前缀就会看到伪装页；因此首次通过面板入口访问时写一个 cookie，
// 后续请求凭这个 cookie 放行，前端无需改动。
const entryCookieName = "rf_entry"

// entryCookieMaxAge 入口 cookie 有效期（30 天）
const entryCookieMaxAge = 60 * 60 * 24 * 30

// subscriptionDeliveryPaths 是订阅入口允许的精确路径
var subscriptionDeliveryPaths = []string{"/subscribe", "/universal-sub", "/convert"}

// subscriptionDeliveryPathPrefix 是订阅入口允许的路径前缀（规则集）
const subscriptionDeliveryPathPrefix = "/rulesets/"

// entryGate 实现「随机入口路径 + 伪装页」：
//
//   - 面板入口（panelPrefix）：剥前缀放行，并写入入口 cookie，
//     此后浏览器用普通路径（/dashboard、/api、/assets）访问也放行
//   - 订阅入口（subPrefix）：只放行订阅/规则集/转换路径，**不写 cookie**，
//     因此拿到订阅地址的人无法借此进入面板
//   - 带有效入口 cookie：直接放行
//   - /health：豁免，便于容器与外部探活
//   - 其余请求：返回伪装页（看起来像普通静态站点，不暴露任何 RuleFlow 特征）
//
// 静态文件类路径（.js/.css/.ico/.map 等）返回 404，避免“把 HTML 当图片/脚本返回”这种不自然的行为。
type entryGate struct {
	next        http.Handler
	panelPrefix string
	subPrefix   string
	cookie      string
	cover       []byte
	coverType   string
	healthPath  string
}

func newEntryGate(next http.Handler, panelPrefix, subPrefix, cookie string, cover []byte) http.Handler {
	if cookie == "" {
		cookie = strings.Trim(panelPrefix, "/")
	}
	if cookie == "" {
		cookie = strings.Trim(subPrefix, "/")
	}
	return &entryGate{
		next:        next,
		panelPrefix: panelPrefix,
		subPrefix:   subPrefix,
		cookie:      cookie,
		cover:       cover,
		coverType:   "text/html; charset=utf-8",
		healthPath:  "/health",
	}
}

func (g *entryGate) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	reqPath := r.URL.Path

	// 1) 面板入口：剥前缀放行，并写 cookie
	if g.panelPrefix != "" && pathHasPrefix(reqPath, g.panelPrefix) {
		g.setEntryCookie(w, r)
		g.next.ServeHTTP(w, stripEntryPrefix(r, g.panelPrefix))
		return
	}

	// 2) 订阅入口：只放行下发相关路径，且不写 cookie
	if g.subPrefix != "" && pathHasPrefix(reqPath, g.subPrefix) {
		stripped := stripEntryPrefix(r, g.subPrefix)
		if isSubscriptionDeliveryPath(stripped.URL.Path) {
			g.next.ServeHTTP(w, stripped)
			return
		}
		g.serveCover(w, r)
		return
	}

	// 3) 入口 cookie 放行
	if c, err := r.Cookie(entryCookieName); err == nil && secureEqual(c.Value, g.cookie) {
		g.next.ServeHTTP(w, r)
		return
	}

	// 4) 探活豁免
	if g.healthPath != "" && reqPath == g.healthPath {
		g.next.ServeHTTP(w, r)
		return
	}

	// 5) 伪装页
	g.serveCover(w, r)
}

func (g *entryGate) serveCover(w http.ResponseWriter, r *http.Request) {
	if looksLikeStaticAsset(r.URL.Path) {
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

// isSubscriptionDeliveryPath 判断去掉订阅前缀后的路径是否属于允许下发的接口
func isSubscriptionDeliveryPath(p string) bool {
	for _, exact := range subscriptionDeliveryPaths {
		if p == exact {
			return true
		}
	}
	return strings.HasPrefix(p, subscriptionDeliveryPathPrefix)
}

func pathHasPrefix(p, prefix string) bool {
	return p == prefix || strings.HasPrefix(p, prefix+"/")
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
