package app

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// FetchPublicSubscriptionContent is for the anonymous converter. Administrative
// subscription and rule-source fetches may intentionally use private services.
func FetchPublicSubscriptionContent(ctx context.Context, subURL string) (string, http.Header, error) {
	u, err := url.Parse(subURL)
	if err != nil || validatePublicSubscriptionURL(u) != nil {
		return "", nil, fmt.Errorf("公开转换仅接受公网 HTTP/HTTPS 订阅地址")
	}
	client := newPublicSubscriptionClient()
	defer client.CloseIdleConnections()
	return fetchSubscriptionContent(ctx, subURL, client)
}

func newPublicSubscriptionClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// An environment proxy would bypass the checked destination dialer.
	transport.Proxy = nil
	transport.DialContext = dialPublicSubscription
	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("重定向次数过多")
			}
			return validatePublicSubscriptionURL(req.URL)
		},
	}
}

func validatePublicSubscriptionURL(u *url.URL) error {
	if u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return fmt.Errorf("公开转换仅接受 HTTP/HTTPS 订阅地址")
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return fmt.Errorf("公开转换不允许访问本机地址")
	}
	if addr, err := netip.ParseAddr(host); err == nil && !isPublicSubscriptionIP(addr) {
		return fmt.Errorf("公开转换不允许访问非公网地址")
	}
	return nil
}

var restrictedSubscriptionNetworks = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("fec0::/10"),
}

func isPublicSubscriptionIP(addr netip.Addr) bool {
	addr = addr.Unmap()
	if addr.Zone() != "" || !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() {
		return false
	}
	for _, prefix := range restrictedSubscriptionNetworks {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

func dialPublicSubscription(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("无效的订阅服务器地址")
	}
	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(addresses) == 0 {
		return nil, fmt.Errorf("订阅服务器域名解析失败")
	}
	for _, addr := range addresses {
		if !isPublicSubscriptionIP(addr) {
			return nil, fmt.Errorf("公开转换不允许访问非公网地址")
		}
	}
	var lastErr error
	dialer := net.Dialer{Timeout: 10 * time.Second}
	for _, addr := range addresses {
		// Dial the validated IP directly; a second DNS lookup could rebind it.
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(addr.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, lastErr
}
