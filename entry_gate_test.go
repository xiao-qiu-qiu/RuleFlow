package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

const testPrefix = "/7f3a9c2b"
const testCookieValue = "s3cr3t-entry-token"

func newTestGate() (http.Handler, *[]string) {
	var seen []string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PANEL"))
	})
	gate := newEntryGate(next, testPrefix, testCookieValue, []byte("<html>cover</html>"))
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
			// .html 属于页面请求，返回伪装页
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

func TestEntryGatePrefixStripsAndSetsCookie(t *testing.T) {
	gate, seen := newTestGate()

	rec := doRequest(gate, http.MethodGet, testPrefix+"/dashboard?x=1")
	if rec.Code != http.StatusOK || rec.Body.String() != "PANEL" {
		t.Fatalf("入口路径应放行到业务处理器，实际 code=%d body=%q", rec.Code, rec.Body.String())
	}
	if len(*seen) != 1 || (*seen)[0] != "/dashboard" {
		t.Fatalf("业务处理器应看到去掉前缀的路径 /dashboard，实际 %v", *seen)
	}

	cookies := rec.Result().Cookies()
	var entry *http.Cookie
	for _, c := range cookies {
		if c.Name == entryCookieName {
			entry = c
		}
	}
	if entry == nil {
		t.Fatalf("入口路径访问后应写入 %s cookie，实际 %v", entryCookieName, cookies)
	}
	if entry.Value != testCookieValue {
		t.Fatalf("cookie 值不正确: %q", entry.Value)
	}
	if entry.Path != "/" || !entry.HttpOnly {
		t.Fatalf("cookie 属性不正确: path=%q httpOnly=%v", entry.Path, entry.HttpOnly)
	}
}

func TestEntryGatePrefixExactPathBecomesRoot(t *testing.T) {
	gate, seen := newTestGate()

	rec := doRequest(gate, http.MethodGet, testPrefix)
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

func TestEntryGateCookieSecureFollowsForwardedProto(t *testing.T) {
	gate, _ := newTestGate()

	req := httptest.NewRequest(http.MethodGet, testPrefix+"/", nil)
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
