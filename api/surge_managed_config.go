package api

import (
	"net/http"
	"net/url"
	"os"
	"strings"
)

const surgeManagedConfigSuffix = " interval=43200 strict=false"
const publicBaseURLEnv = "PUBLIC_BASE_URL"

func finalizeConfigContent(r *http.Request, target, content string) string {
	if target != "surge" {
		return content
	}

	managedLine := buildSurgeManagedConfigLine(r)
	cleaned := stripManagedConfigLines(content)
	if cleaned == "" {
		return managedLine + "\n"
	}
	return managedLine + "\n" + cleaned
}

func buildSurgeManagedConfigLine(r *http.Request) string {
	return "#!MANAGED-CONFIG " + requestURLString(r) + surgeManagedConfigSuffix
}

func requestURLString(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}

	u := *r.URL
	if applyPublicBaseURL(&u) {
		return u.String()
	}

	scheme := forwardedProto(r)
	host := forwardedHost(r)
	if scheme != "" {
		u.Scheme = scheme
	} else if u.Scheme == "" {
		u.Scheme = scheme
	}
	if host != "" {
		u.Host = host
	} else if u.Host == "" {
		u.Host = host
	}

	return u.String()
}

// PublicBaseURL 返回对外可用的公开地址（优先 PUBLIC_BASE_URL，其次请求的转发头），
// 供 SPA 拼接订阅 / 规则集 / 转换链接使用。面板挂在随机路径下时它已包含路径前缀。
func PublicBaseURL(r *http.Request) string {
	return requestBaseURLString(r)
}

func requestBaseURLString(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}

	u := *r.URL
	u.Path = ""
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""

	if applyPublicBaseURL(&u) {
		return strings.TrimRight(u.String(), "/")
	}

	scheme := forwardedProto(r)
	host := forwardedHost(r)
	if scheme != "" {
		u.Scheme = scheme
	}
	if host != "" {
		u.Host = host
	}

	return strings.TrimRight(u.String(), "/")
}

func applyPublicBaseURL(u *url.URL) bool {
	return applyBaseURLFromEnv(u, publicBaseURLEnv)
}

func applyBaseURLFromEnv(u *url.URL, envName string) bool {
	baseURL := strings.TrimSpace(os.Getenv(envName))
	if baseURL == "" {
		return false
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return false
	}

	if parsed.Scheme != "" {
		u.Scheme = parsed.Scheme
	}
	if parsed.Host != "" {
		u.Host = parsed.Host
	}
	applyBasePath(u, parsed.Path)
	return parsed.Scheme != "" && parsed.Host != ""
}

// applyBasePath 把 PUBLIC_BASE_URL 里的路径前缀（例如 /7f3a9c）并入请求路径。
// 反向代理把面板挂在随机路径下时，生成的绝对 URL（订阅、规则集）才不会丢掉前缀。
func applyBasePath(u *url.URL, basePath string) bool {
	prefix := strings.TrimRight(strings.TrimSpace(basePath), "/")
	if prefix == "" || prefix == "/" {
		return false
	}
	if u.Path == prefix || strings.HasPrefix(u.Path, prefix+"/") {
		return false
	}
	u.Path = prefix + u.Path
	if u.RawPath != "" {
		u.RawPath = prefix + u.RawPath
	}
	return true
}

func forwardedProto(r *http.Request) string {
	if proto := firstForwardedValue(r.Header.Get("X-Forwarded-Proto")); proto != "" {
		return proto
	}
	if proto := forwardedDirective(r.Header.Get("Forwarded"), "proto"); proto != "" {
		return proto
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func forwardedHost(r *http.Request) string {
	if host := firstForwardedValue(r.Header.Get("X-Forwarded-Host")); host != "" {
		return host
	}
	if host := forwardedDirective(r.Header.Get("Forwarded"), "host"); host != "" {
		return host
	}
	if r.Host != "" {
		return r.Host
	}
	return ""
}

func firstForwardedValue(value string) string {
	if value == "" {
		return ""
	}
	parts := strings.Split(value, ",")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func forwardedDirective(headerValue, key string) string {
	if headerValue == "" {
		return ""
	}
	entries := strings.Split(headerValue, ",")
	for _, entry := range entries {
		directives := strings.Split(entry, ";")
		for _, directive := range directives {
			name, value, ok := strings.Cut(strings.TrimSpace(directive), "=")
			if !ok || !strings.EqualFold(name, key) {
				continue
			}
			return strings.Trim(strings.TrimSpace(value), "\"")
		}
	}
	return ""
}

func stripManagedConfigLines(content string) string {
	if content == "" {
		return ""
	}

	lines := strings.Split(content, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))
		if strings.HasPrefix(trimmed, "#!MANAGED-CONFIG ") {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.TrimLeft(strings.Join(filtered, "\n"), "\r\n\t ")
}
