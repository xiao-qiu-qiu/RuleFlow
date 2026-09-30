package api

import (
	"net/http/httptest"
	"testing"
)

// PUBLIC_BASE_URL 带路径前缀（反向代理把面板挂在随机路径下）时，
// 生成的绝对 URL 必须保留前缀，否则订阅与规则集链接会 404。
func TestPublicBaseURLKeepsPathPrefix(t *testing.T) {
	t.Setenv(publicBaseURLEnv, "https://panel.example.com/7f3a9c2b")

	req := httptest.NewRequest("GET", "http://internal:8080/subscribe?token=abc", nil)

	if got, want := requestURLString(req), "https://panel.example.com/7f3a9c2b/subscribe?token=abc"; got != want {
		t.Fatalf("requestURLString = %q，期望 %q", got, want)
	}
	if got, want := requestBaseURLString(req), "https://panel.example.com/7f3a9c2b"; got != want {
		t.Fatalf("requestBaseURLString = %q，期望 %q", got, want)
	}
}

func TestPublicBaseURLWithoutPathUnchanged(t *testing.T) {
	t.Setenv(publicBaseURLEnv, "https://panel.example.com")

	req := httptest.NewRequest("GET", "http://internal:8080/subscribe?token=abc", nil)

	if got, want := requestURLString(req), "https://panel.example.com/subscribe?token=abc"; got != want {
		t.Fatalf("requestURLString = %q，期望 %q", got, want)
	}
	if got, want := requestBaseURLString(req), "https://panel.example.com"; got != want {
		t.Fatalf("requestBaseURLString = %q，期望 %q", got, want)
	}
}

func TestPublicBaseURLPrefixNotDuplicated(t *testing.T) {
	t.Setenv(publicBaseURLEnv, "https://panel.example.com/7f3a9c2b")

	req := httptest.NewRequest("GET", "http://internal:8080/7f3a9c2b/subscribe?token=abc", nil)

	if got, want := requestURLString(req), "https://panel.example.com/7f3a9c2b/subscribe?token=abc"; got != want {
		t.Fatalf("前缀不应重复拼接: %q，期望 %q", got, want)
	}
}
