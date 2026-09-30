package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testPanelPrefix = "/7f3a9c2b"
const testSubPrefix = "/sub91x7k4"
const testCookieValue = "s3cr3t-entry-token"

func newTestGate() (http.Handler, *[]string) {
	var seen []string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PANEL"))
	})
	gate := newEntryGate(next, testPanelPrefix, testSubPrefix, testCookieValue, []byte("<html>cover</html>"))
	return gate, &seen
}

func doRequest(h http.Handler, method, target string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func entryCookieFrom(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == entryCookieName {
			return c
		}
	}
	return nil
}

func TestEntryGateServesCoverWithoutEntry(t *testing.T) {
	gate, seen := newTestGate()

	for _, target := range []string{"/", "/login", "/dashboard", "/subscribe?token=t", "/api/nodes", "/version"} {
		rec := doRequest(gate, http.MethodGet, target)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: 期望 200，实际 %d", target, rec.Code)
		}
		if body := rec.Body.String(); body != "<html>cover</html>" {
			t.Fatalf("%s: 期望返回伪装页，实际 %q", target, body)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
			t.Fatalf("%s: Content-Type 期望 html，实际 %q", target, ct)
		}
	}
	if len(*seen) != 0 {
		t.Fatalf("未带入口凭据的请求不应到达业务处理器，实际到达: %v", *seen)
	}
}

func TestEntryGateStaticAssetReturns404(t *testing.T) {
	gate, _ := newTestGate()

	for _, target := range []string{"/favicon.svg", "/assets/main.js", "/robots.txt", "/index.html"} {
		rec := doRequest(gate, http.MethodGet, target)
		if target == "/index.html" {
			if rec.Body.String() != "<html>cover</html>" {
				t.Fatalf("%s: 期望伪装页，实际 %q", target, rec.Body.String())
			}
			continue
		}
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: 期望 404，实际 %d", target, rec.Code)
		}
	}
}

func TestEntryGatePanelPrefixStripsAndSetsCookie(t *testing.T) {
	gate, seen := newTestGate()

	rec := doRequest(gate, http.MethodGet, testPanelPrefix+"/dashboard?x=1")
	if rec.Code != http.StatusOK || rec.Body.String() != "PANEL" {
		t.Fatalf("入口路径应放行到业务处理器，实际 code=%d body=%q", rec.Code, rec.Body.String())
	}
	if len(*seen) != 1 || (*seen)[0] != "/dashboard" {
		t.Fatalf("业务处理器应看到去掉前缀的路径 /dashboard，实际 %v", *seen)
	}

	entry := entryCookieFrom(t, rec)
	if entry == nil {
		t.Fatal("面板入口访问后应写入入口 cookie")
	}
	if entry.Value != testCookieValue {
		t.Fatalf("cookie 值不正确: %q", entry.Value)
	}
	if entry.Path != "/" || !entry.HttpOnly {
		t.Fatalf("cookie 属性不正确: path=%q httpOnly=%v", entry.Path, entry.HttpOnly)
	}
}

func TestEntryGatePanelPrefixExactPathBecomesRoot(t *testing.T) {
	gate, seen := newTestGate()

	rec := doRequest(gate, http.MethodGet, testPanelPrefix)
	if rec.Code != http.StatusOK {
		t.Fatalf("入口根路径应放行，实际 %d", rec.Code)
	}
	if len(*seen) != 1 || (*seen)[0] != "/" {
		t.Fatalf("业务处理器应看到 /，实际 %v", *seen)
	}
}

func TestEntryGateCookiePassesThrough(t *testing.T) {
	gate, seen := newTestGate()
	cookie := &http.Cookie{Name: entryCookieName, Value: testCookieValue}

	for _, target := range []string{"/dashboard", "/api/nodes", "/assets/main.js", "/version"} {
		rec := doRequest(gate, http.MethodGet, target, cookie)
		if rec.Code != http.StatusOK || rec.Body.String() != "PANEL" {
			t.Fatalf("%s: 带入口 cookie 应放行，实际 code=%d body=%q", target, rec.Code, rec.Body.String())
		}
	}
	if len(*seen) != 4 {
		t.Fatalf("应全部到达业务处理器，实际 %v", *seen)
	}
}

func TestEntryGateRejectsWrongCookie(t *testing.T) {
	gate, seen := newTestGate()

	rec := doRequest(gate, http.MethodGet, "/dashboard", &http.Cookie{Name: entryCookieName, Value: "wrong"})
	if rec.Body.String() != "<html>cover</html>" {
		t.Fatalf("错误 cookie 应看到伪装页，实际 %q", rec.Body.String())
	}
	rec = doRequest(gate, http.MethodGet, "/dashboard", &http.Cookie{Name: entryCookieName, Value: testCookieValue + "x"})
	if rec.Body.String() != "<html>cover</html>" {
		t.Fatalf("长度不同的 cookie 应看到伪装页，实际 %q", rec.Body.String())
	}
	if len(*seen) != 0 {
		t.Fatalf("不应到达业务处理器，实际 %v", *seen)
	}
}

func TestEntryGateHealthExempt(t *testing.T) {
	gate, seen := newTestGate()

	rec := doRequest(gate, http.MethodGet, "/health")
	if rec.Body.String() != "PANEL" {
		t.Fatalf("/health 应豁免（便于探活），实际 %q", rec.Body.String())
	}
	if len(*seen) != 1 || (*seen)[0] != "/health" {
		t.Fatalf("业务处理器应收到 /health，实际 %v", *seen)
	}
}

// 订阅入口：只放行下发相关路径，且不能借此拿到面板 cookie
func TestEntryGateSubPrefixAllowsDeliveryPaths(t *testing.T) {
	gate, seen := newTestGate()

	cases := []struct {
		target   string
		wantPath string
	}{
		{testSubPrefix + "/subscribe?token=abc", "/subscribe"},
		{testSubPrefix + "/universal-sub?token=abc", "/universal-sub"},
		{testSubPrefix + "/rulesets/need_proxy?target=clash-classical", "/rulesets/need_proxy"},
		{testSubPrefix + "/convert?url=https%3A%2F%2Fexample.com", "/convert"},
	}
	for _, c := range cases {
		rec := doRequest(gate, http.MethodGet, c.target)
		if rec.Code != http.StatusOK || rec.Body.String() != "PANEL" {
			t.Fatalf("%s: 订阅入口应放行，实际 code=%d body=%q", c.target, rec.Code, rec.Body.String())
		}
		if entry := entryCookieFrom(t, rec); entry != nil {
			t.Fatalf("%s: 订阅入口不应写入口 cookie", c.target)
		}
	}
	if len(*seen) != len(cases) {
		t.Fatalf("应全部到达业务处理器，实际 %v", *seen)
	}
	for i, c := range cases {
		if (*seen)[i] != c.wantPath {
			t.Fatalf("业务处理器应看到 %q，实际 %q", c.wantPath, (*seen)[i])
		}
	}
}

func TestEntryGateSubPrefixBlocksPanelPaths(t *testing.T) {
	gate, seen := newTestGate()

	// 页面类路径应看到伪装页；静态资源类路径应得到 404 —— 两者都不能是面板内容
	coverPaths := []string{testSubPrefix + "/", testSubPrefix, testSubPrefix + "/dashboard", testSubPrefix + "/login", testSubPrefix + "/api/nodes"}
	for _, target := range coverPaths {
		rec := doRequest(gate, http.MethodGet, target)
		if rec.Code != http.StatusOK || rec.Body.String() != "<html>cover</html>" {
			t.Fatalf("%s: 订阅入口不应放行面板路径，实际 code=%d body=%q", target, rec.Code, rec.Body.String())
		}
		if entry := entryCookieFrom(t, rec); entry != nil {
			t.Fatalf("%s: 订阅入口不应写入口 cookie", target)
		}
	}

	assetRec := doRequest(gate, http.MethodGet, testSubPrefix+"/assets/main.js")
	if assetRec.Code != http.StatusNotFound {
		t.Fatalf("订阅入口下的静态资源路径应 404，实际 %d", assetRec.Code)
	}
	if entry := entryCookieFrom(t, assetRec); entry != nil {
		t.Fatalf("订阅入口不应写入口 cookie")
	}

	if len(*seen) != 0 {
		t.Fatalf("订阅入口访问面板路径不应到达业务处理器，实际 %v", *seen)
	}
}

func TestEntryGateCookieSecureFollowsForwardedProto(t *testing.T) {
	gate, _ := newTestGate()

	req := httptest.NewRequest(http.MethodGet, testPanelPrefix+"/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	gate.ServeHTTP(rec, req)
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 || !cookies[0].Secure {
		t.Fatalf("X-Forwarded-Proto=https 时应写入 Secure cookie，实际 %v", cookies)
	}
}

func TestLooksLikeStaticAsset(t *testing.T) {
	assets := []string{"/a.js", "/a.CSS", "/x/y.png", "/robots.txt", "/sitemap.xml"}
	for _, p := range assets {
		if !looksLikeStaticAsset(p) {
			t.Fatalf("%s 应被识别为静态资源", p)
		}
	}
	for _, p := range []string{"/", "/dashboard", "/a/b", "/index.html"} {
		if looksLikeStaticAsset(p) {
			t.Fatalf("%s 不应被识别为静态资源", p)
		}
	}
}

func TestInjectPublicBase(t *testing.T) {
	html := []byte("<html><head><title>x</title></head><body></body></html>")
	out := string(injectPublicBase(html, "https://panel.example.com/7f3a9c2b"))
	want := `<script>window.__RF_PUBLIC_BASE__="https://panel.example.com/7f3a9c2b";</script></head>`
	if !strings.Contains(out, want) {
		t.Fatalf("应注入公开地址，实际 %q", out)
	}
	if got := string(injectPublicBase(html, "")); got != string(html) {
		t.Fatalf("公开地址为空时不应改动 HTML，实际 %q", got)
	}
}
