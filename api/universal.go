package api

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/ablate-ai/RuleFlow/internal/app"
)

var universalSubscriptionTargets = map[string]struct{}{
	"mihomo":   {},
	"stash":    {},
	"surge":    {},
	"sing-box": {},
	"v2ray":    {},
}

// UniversalSubscription 根据客户端类型返回对应的订阅格式。
// v2ray 客户端先复用 Mihomo 配置生成链路，再转换为 Base64 协议链接列表。
func (h *Handlers) UniversalSubscription(w http.ResponseWriter, r *http.Request) {
	clientToken := strings.TrimSpace(r.URL.Query().Get("token"))
	if clientToken == "" {
		http.Error(w, "缺少 token 参数", http.StatusBadRequest)
		return
	}

	target := universalTargetForRequest(r)
	policyTarget := target
	if target == "v2ray" {
		policyTarget = "mihomo"
	}
	policyToken := derivedPolicyToken(clientToken, policyTarget)

	// 直接调用现有策略生成器，保留缓存、模板、规则和访问日志行为。
	requestCopy := r.Clone(r.Context())
	query := requestCopy.URL.Query()
	query.Set("token", policyToken)
	requestCopy.URL.RawQuery = query.Encode()
	recorder := httptest.NewRecorder()
	h.GenerateConfig(recorder, requestCopy)
	response := recorder.Result()
	defer response.Body.Close()

	content, err := io.ReadAll(response.Body)
	if err != nil {
		http.Error(w, "读取订阅配置失败", http.StatusBadGateway)
		return
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		copyUniversalHeaders(w.Header(), response.Header)
		w.WriteHeader(response.StatusCode)
		_, _ = w.Write(content)
		return
	}

	if target != "v2ray" {
		copyUniversalHeaders(w.Header(), response.Header)
		w.Header().Set("X-Universal-Target", target)
		w.WriteHeader(response.StatusCode)
		_, _ = w.Write(content)
		return
	}

	encoded, count, err := buildV2RaySubscription(string(content))
	if err != nil {
		http.Error(w, "生成 V2Ray 订阅失败: "+err.Error(), http.StatusBadGateway)
		return
	}

	// V2RayN 兼容常见的纯文本 Base64 订阅格式，并保留流量信息头。
	copyUniversalHeaders(w.Header(), response.Header)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `inline; filename="v2rayn-sub.txt"`)
	w.Header().Set("X-Subscription-Node-Count", fmt.Sprintf("%d", count))
	w.Header().Set("X-Universal-Target", "v2ray")
	w.Header().Del("Content-Length")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, encoded+"\n")
}

func universalTargetForRequest(r *http.Request) string {
	if target := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("target"))); target != "" {
		if _, ok := universalSubscriptionTargets[target]; ok {
			return target
		}
	}

	userAgent := strings.ToLower(r.UserAgent())
	switch {
	case strings.Contains(userAgent, "v2rayn"), strings.Contains(userAgent, "v2ray"):
		return "v2ray"
	case strings.Contains(userAgent, "sing-box"), strings.Contains(userAgent, "singbox"), strings.Contains(userAgent, "hiddify"):
		return "sing-box"
	case strings.Contains(userAgent, "surge"), strings.Contains(userAgent, "shadowrocket"):
		return "surge"
	case strings.Contains(userAgent, "stash"):
		return "stash"
	default:
		return "mihomo"
	}
}

func derivedPolicyToken(clientToken, target string) string {
	sum := sha256.Sum256([]byte("xqqq-ruleflow-auto-v1:" + clientToken + ":" + target))
	return hex.EncodeToString(sum[:])
}

func buildV2RaySubscription(content string) (string, int, error) {
	nodes, err := app.ParseSubscription(content)
	if err != nil {
		return "", 0, fmt.Errorf("解析 Mihomo 配置失败: %w", err)
	}

	links := make([]string, 0, len(nodes))
	for _, node := range nodes {
		link, err := app.SerializeNodeURL(node.Name, node.Protocol, node.Server, node.Port, node.Options)
		if err != nil {
			// V2Ray 不支持 WireGuard 等协议，跳过不兼容节点，保留其余节点。
			continue
		}
		links = append(links, link)
	}
	links = app.DedupeStrings(links)
	if len(links) == 0 {
		return "", 0, fmt.Errorf("Mihomo 配置中没有可转换的节点")
	}

	return base64.StdEncoding.EncodeToString([]byte(strings.Join(links, "\n"))), len(links), nil
}

func copyUniversalHeaders(dst, src http.Header) {
	for key, values := range src {
		if strings.EqualFold(key, "Content-Length") || strings.EqualFold(key, "Transfer-Encoding") {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}
