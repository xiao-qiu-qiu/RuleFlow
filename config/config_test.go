package config

import "testing"

func TestNormalizeEntryPath(t *testing.T) {
	cases := map[string]string{
		"":            "",
		"/":           "",
		"   ":         "",
		"abc123":      "/abc123",
		"/abc123":     "/abc123",
		"/abc123/":    "/abc123",
		"abc123/":     "/abc123",
		"/a/b/":       "/a/b",
		"  /7f3a9c2b ": "/7f3a9c2b",
	}
	for input, want := range cases {
		if got := normalizeEntryPath(input); got != want {
			t.Fatalf("normalizeEntryPath(%q) = %q，期望 %q", input, got, want)
		}
	}
}
