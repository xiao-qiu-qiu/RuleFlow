package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ablate-ai/RuleFlow/database"
)

func TestPrepareSubscriptionSyncAppliesFilters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`proxies:
  - {name: HK Node, type: trojan, server: hk.example.com, port: 443, password: fixture}
  - {name: US Node, type: trojan, server: us.example.com, port: 443, password: fixture}
  - {name: US VLESS, type: vless, server: us.example.com, port: 443, uuid: 11111111-1111-1111-1111-111111111111}
`))
	}))
	defer server.Close()
	for _, tc := range []struct {
		name   string
		filter database.SubscriptionFilter
		count  int
		fail   bool
	}{
		{name: "exclude keyword", filter: database.SubscriptionFilter{ExcludeKeywords: []string{"hk"}}, count: 2},
		{name: "exclude regex", filter: database.SubscriptionFilter{ExcludeRegex: "US"}, count: 1},
		{name: "protocol whitelist", filter: database.SubscriptionFilter{IncludeProtocols: []string{" vless "}}, count: 1},
		{name: "empty keyword", filter: database.SubscriptionFilter{ExcludeKeywords: []string{" "}}, count: 3},
		{name: "all filtered", filter: database.SubscriptionFilter{ExcludeRegex: ".*"}, fail: true},
		{name: "invalid regex", filter: database.SubscriptionFilter{ExcludeRegex: "["}, fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &SubscriptionSyncService{}
			prepared := svc.prepareSubscriptionSync(context.Background(), &database.Subscription{
				ID: 1, Name: "fixture", URL: &server.URL, Enabled: true, FilterRules: &tc.filter,
			})
			if (prepared.err != nil) != tc.fail {
				t.Fatalf("prepare error = %v, want failure %v", prepared.err, tc.fail)
			}
			if !tc.fail && len(prepared.nodes) != tc.count {
				t.Fatalf("prepared %d nodes, want %d", len(prepared.nodes), tc.count)
			}
		})
	}
}
