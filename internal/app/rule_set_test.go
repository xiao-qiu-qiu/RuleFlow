package app

import (
	"strings"
	"testing"
)

func TestParseRuleSetYAMLPayloads(t *testing.T) {
	for _, tc := range []struct {
		format, body, wantType, wantValue string
	}{
		{"clash-domain", "payload:\n  - '+.example.com'\n", "domain_suffix", "example.com"},
		{"clash-domain", "+.example.com\n", "domain_suffix", "example.com"},
		{"clash-ipcidr", "payload: ['192.0.2.0/24']", "ip_cidr", "192.0.2.0/24"},
		{"surge", "payload:\n  - 'DOMAIN-SUFFIX,example.com'\n", "domain_suffix", "example.com"},
	} {
		rules, err := ParseRuleSet(tc.body, tc.format)
		if err != nil || len(rules) != 1 || rules[0].Type != tc.wantType || rules[0].Value != tc.wantValue {
			t.Errorf("parse %s: rules=%v err=%v", tc.format, rules, err)
		}
	}
}

func TestParseRuleSetClassical(t *testing.T) {
	content := `
DOMAIN-SUFFIX,claude.ai,🤖 AI
DOMAIN,example.com,DIRECT
IP-CIDR,1.1.1.1/32,no-resolve
`

	rules, err := ParseRuleSet(content, "surge")
	if err != nil {
		t.Fatalf("ParseRuleSet() error = %v", err)
	}
	if len(rules) != 3 {
		t.Fatalf("ParseRuleSet() len = %d, want 3", len(rules))
	}
	if rules[0].Type != "domain_suffix" || rules[0].Value != "claude.ai" {
		t.Fatalf("unexpected first rule: %+v", rules[0])
	}
}

func TestExportRuleSetSingBox(t *testing.T) {
	rules := []RuleSetRule{
		{Type: "domain_suffix", Value: "claude.ai"},
		{Type: "domain", Value: "example.com"},
		{Type: "ip_cidr", Value: "1.1.1.1/32", NoResolve: true},
	}

	content, err := ExportRuleSet(rules, "sing-box")
	if err != nil {
		t.Fatalf("ExportRuleSet() error = %v", err)
	}
	if !strings.Contains(content, "\"domain_suffix\"") || !strings.Contains(content, "\"ip_cidr\"") {
		t.Fatalf("unexpected sing-box export: %s", content)
	}
}

func TestDedupeRuleSetRulesIgnoreNoResolveForNonIP(t *testing.T) {
	rules := []RuleSetRule{
		{Type: "domain", Value: "example.com"},
		{Type: "domain", Value: "example.com", NoResolve: true},
		{Type: "ip_cidr", Value: "1.1.1.1/32"},
		{Type: "ip_cidr", Value: "1.1.1.1/32", NoResolve: true},
	}

	got := dedupeRuleSetRules(rules)
	if len(got) != 3 {
		t.Fatalf("dedupeRuleSetRules() len = %d, want 3; rules=%+v", len(got), got)
	}

	domainCount := 0
	ipCount := 0
	for _, rule := range got {
		switch rule.Type {
		case "domain":
			domainCount++
		case "ip_cidr":
			ipCount++
		}
	}

	if domainCount != 1 {
		t.Fatalf("domain rules count = %d, want 1; rules=%+v", domainCount, got)
	}
	if ipCount != 2 {
		t.Fatalf("ip rules count = %d, want 2; rules=%+v", ipCount, got)
	}
}
