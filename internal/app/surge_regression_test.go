package app

import (
	"strings"
	"testing"
)

func TestBuildSurgeWithoutNodesPlaceholderDoesNotDuplicateExistingProxies(t *testing.T) {
	templateContent := `[General]
loglevel = notify

[Proxy]
Existing = direct

[Proxy Group]
Proxy = select, Existing, __NODES__
`
	nodes := []*ProxyNode{
		{
			Protocol: "trojan",
			Name:     "Added",
			Server:   "added.example.com",
			Port:     443,
			Options: map[string]interface{}{
				"password": "test-password",
				"sni":      "added.example.com",
			},
		},
	}

	config, err := BuildSurgeFromTemplateContent(nodes, templateContent)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(config, "Existing = direct"); got != 1 {
		t.Fatalf("existing proxy definition count = %d, want 1:\n%s", got, config)
	}
}

func TestBuildSurgeConsistentlyFiltersUnsupportedVLESS(t *testing.T) {
	templateContent := `[Proxy]
__NODES__

[Proxy Group]
Proxy = select, __NODES__
`
	vlessNode := &ProxyNode{
		Protocol: "vless",
		Name:     "VLESS Unsupported",
		Server:   "vless.example.com",
		Port:     443,
		Options: map[string]interface{}{
			"uuid": "11111111-1111-1111-1111-111111111111",
			"tls":  true,
			"sni":  "vless.example.com",
		},
	}
	trojanNode := &ProxyNode{
		Protocol: "trojan",
		Name:     "Trojan Supported",
		Server:   "trojan.example.com",
		Port:     443,
		Options: map[string]interface{}{
			"password": "test-password",
			"sni":      "trojan.example.com",
		},
	}

	builders := map[string]func([]*ProxyNode) (string, error){
		"custom": func(nodes []*ProxyNode) (string, error) {
			return BuildSurgeFromTemplateContent(nodes, templateContent)
		},
		"default": BuildSurgeFromDefaultTemplate,
	}

	for name, build := range builders {
		t.Run(name+" mixed", func(t *testing.T) {
			config, err := build([]*ProxyNode{vlessNode, trojanNode})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(config, "VLESS Unsupported") || strings.Contains(config, "= vless,") {
				t.Fatalf("unsupported VLESS node remains in config:\n%s", config)
			}
			if !strings.Contains(config, "Trojan Supported = trojan, trojan.example.com, 443") {
				t.Fatalf("supported Trojan node was dropped:\n%s", config)
			}
			if strings.Contains(config, "Proxy = select, VLESS Unsupported") {
				t.Fatalf("proxy group retains unsupported VLESS member:\n%s", config)
			}
		})

		t.Run(name+" all VLESS", func(t *testing.T) {
			if _, err := build([]*ProxyNode{vlessNode}); err == nil {
				t.Fatal("all-VLESS input should return an explicit unsupported-node error")
			}
		})
	}
}

func TestBuildSurgeRetainsLogicalRules(t *testing.T) {
	templateContent := `[Proxy Group]
Proxy = select, DIRECT

[Rule]
AND,((DOMAIN,example.com),(NETWORK,TCP)),Proxy
FINAL,Proxy
`

	config, err := BuildSurgeFromTemplateContent(nil, templateContent)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(config, "AND,((DOMAIN,example.com),(NETWORK,TCP)),Proxy") {
		t.Fatalf("logical rule was removed:\n%s", config)
	}
}
