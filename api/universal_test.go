package api

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUniversalTargetForRequest(t *testing.T) {
	tests := []struct {
		name      string
		userAgent string
		target    string
		want      string
	}{
		{name: "mihomo default", want: "mihomo"},
		{name: "v2rayn ua", userAgent: "v2rayN/7.0", want: "v2ray"},
		{name: "sing box ua", userAgent: "hiddify/2.0", want: "sing-box"},
		{name: "explicit target", target: "surge", userAgent: "v2rayN", want: "surge"},
		{name: "explicit clash alias", target: "clash-mihomo", userAgent: "v2rayN", want: "mihomo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := http.NewRequest(http.MethodGet, "https://example.com/universal-sub?target="+tt.target, nil)
			if err != nil {
				t.Fatal(err)
			}
			r.Header.Set("User-Agent", tt.userAgent)
			if got := universalTargetForRequest(r); got != tt.want {
				t.Fatalf("universalTargetForRequest() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPolicyTargetForRequestAdaptive(t *testing.T) {
	tests := []struct {
		name      string
		userAgent string
		want      string
		v2ray     bool
	}{
		{name: "mihomo", userAgent: "clash.meta/1.0", want: "clash-mihomo"},
		{name: "sing-box", userAgent: "sing-box/1.0", want: "sing-box"},
		{name: "v2ray", userAgent: "v2rayN/7.0", want: "clash-mihomo", v2ray: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "https://example.com/subscribe", nil)
			r.Header.Set("User-Agent", tt.userAgent)
			got, v2ray, adaptive, err := policyTargetForRequest("adaptive", r)
			if err != nil || !adaptive || got != tt.want || v2ray != tt.v2ray {
				t.Fatalf("policyTargetForRequest() = (%q, %v, %v, %v), want (%q, %v, true, nil)", got, v2ray, adaptive, err, tt.want, tt.v2ray)
			}
		})
	}
}

func TestBuildV2RaySubscriptionPreservesRealityAndHysteria2(t *testing.T) {
	content := `proxies:
  - name: LA Reality
    type: vless
    server: reality.example.com
    port: 443
    uuid: 11111111-1111-1111-1111-111111111111
    tls: true
    servername: www.example.com
    reality-opts:
      public-key: public-key
      short-id: short-id
  - name: HY2
    type: hysteria2
    server: hy2.example.com
    port: 443
    password: hy2-password
    sni: hy2.example.com
    obfs: salamander
    obfs-password: obfs-password
    alpn: [h3]
`
	encoded, count, err := buildV2RaySubscription(content)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("node count = %d, want 2", count)
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	text := string(decoded)
	if !strings.Contains(text, "vless://11111111-1111-1111-1111-111111111111@reality.example.com:443") ||
		!strings.Contains(text, "security=reality") ||
		!strings.Contains(text, "pbk=public-key") ||
		!strings.Contains(text, "sid=short-id") {
		t.Fatalf("Reality link fields missing: %s", text)
	}
	if !strings.Contains(text, "hysteria2://hy2-password@hy2.example.com:443") ||
		!strings.Contains(text, "obfs=salamander") ||
		!strings.Contains(text, "obfs-password=obfs-password") ||
		!strings.Contains(text, "alpn=h3") {
		t.Fatalf("Hysteria2 link fields missing: %s", text)
	}
}

func TestBuildV2RaySubscriptionPreservesFlatTransportAndTUIC(t *testing.T) {
	content := `proxies:
  - name: WS VLESS
    type: vless
    server: ws.example.com
    port: 443
    uuid: 22222222-2222-2222-2222-222222222222
    tls: true
    servername: edge.example.com
    network: ws
    ws-opts:
      path: /socket
      headers:
        Host: cdn.example.com
  - name: TUIC
    type: tuic
    server: tuic.example.com
    port: 443
    uuid: 33333333-3333-3333-3333-333333333333
    password: tuic-password
    sni: edge.example.com
    skip-cert-verify: true
    alpn: [h3]
`
	encoded, count, err := buildV2RaySubscription(content)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("node count = %d, want 2", count)
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	text := string(decoded)
	if !strings.Contains(text, "type=ws") || !strings.Contains(text, "path=%2Fsocket") || !strings.Contains(text, "host=cdn.example.com") {
		t.Fatalf("VLESS transport fields missing: %s", text)
	}
	if !strings.Contains(text, "tuic://33333333-3333-3333-3333-333333333333:tuic-password@tuic.example.com:443") ||
		!strings.Contains(text, "sni=edge.example.com") || !strings.Contains(text, "insecure=1") || !strings.Contains(text, "alpn=h3") {
		t.Fatalf("TUIC TLS fields missing: %s", text)
	}
}
