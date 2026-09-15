package app

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestReadResponseBodyCompression(t *testing.T) {
	payload := []byte("proxies:\n  - name: test-node\n    type: trojan\n")
	for _, kind := range []string{"identity", "gzip", "zlib", "raw-deflate"} {
		t.Run(kind, func(t *testing.T) {
			var buf bytes.Buffer
			var writer io.WriteCloser
			encoding := kind
			switch kind {
			case "gzip":
				writer = gzip.NewWriter(&buf)
			case "zlib":
				writer, encoding = zlib.NewWriter(&buf), "deflate"
			case "raw-deflate":
				writer, _ = flate.NewWriter(&buf, flate.DefaultCompression)
				encoding = "deflate"
			default:
				writer = nopWriteCloser{&buf}
			}
			if _, err := writer.Write(payload); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			resp := &http.Response{Header: http.Header{"Content-Encoding": {encoding}}, Body: io.NopCloser(&buf)}
			got, err := readResponseBody(resp)
			if err != nil || !bytes.Equal(got, payload) {
				t.Fatalf("decode %s: got %q, error %v", kind, got, err)
			}
		})
	}
	if _, err := readLimitedSubscriptionBody(strings.NewReader("12345"), 4); err == nil {
		t.Fatal("oversized response was accepted")
	}
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

func TestPublicSubscriptionRejectsPrivateDestinations(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "::1", "10.0.0.1", "172.16.0.1", "192.168.1.1", "169.254.169.254", "::ffff:127.0.0.1", "100.64.0.1", "198.18.0.1", "fc00::1", "fe80::1", "64:ff9b::a00:1"} {
		if isPublicSubscriptionIP(netip.MustParseAddr(ip)) {
			t.Errorf("accepted restricted IP %s", ip)
		}
	}
	for _, ip := range []string{"1.1.1.1", "8.8.8.8", "2606:4700:4700::1111"} {
		if !isPublicSubscriptionIP(netip.MustParseAddr(ip)) {
			t.Errorf("rejected public IP %s", ip)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("anonymous converter reached a private server")
	}))
	defer server.Close()
	if _, _, err := FetchPublicSubscriptionContent(context.Background(), server.URL); err == nil {
		t.Fatal("private URL was accepted")
	}
	if conn, err := dialPublicSubscription(context.Background(), "tcp", "localhost:80"); err == nil {
		conn.Close()
		t.Fatal("DNS-resolved loopback was accepted")
	}
}

type subscriptionRoundTripper func(*http.Request) (*http.Response, error)

func (f subscriptionRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPublicSubscriptionRejectsPrivateRedirect(t *testing.T) {
	client := newPublicSubscriptionClient()
	defer client.CloseIdleConnections()
	requests := 0
	client.Transport = subscriptionRoundTripper(func(r *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{
			StatusCode: http.StatusFound, Header: http.Header{"Location": {"http://127.0.0.1/private"}},
			Body: io.NopCloser(strings.NewReader("")), Request: r,
		}, nil
	})
	if _, _, err := fetchSubscriptionContent(context.Background(), "https://public.example/sub", client); err == nil {
		t.Fatal("private redirect was accepted")
	}
	if requests != 1 {
		t.Fatalf("redirect reached the transport: %d requests", requests)
	}
}
