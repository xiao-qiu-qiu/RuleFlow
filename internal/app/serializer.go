package app

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// SerializeNodeURL 将存储的节点数据反向生成分享链接
func SerializeNodeURL(name, protocol, server string, port int, config map[string]interface{}) (string, error) {
	switch protocol {
	case "ss":
		return serializeSS(name, server, port, config)
	case "trojan":
		return serializeTrojan(name, server, port, config)
	case "vless":
		return serializeVLESS(name, server, port, config)
	case "vmess":
		return serializeVMess(name, server, port, config)
	case "hysteria2":
		return serializeHysteria2(name, server, port, config)
	case "tuic":
		return serializeTUIC(name, server, port, config)
	case "anytls":
		return serializeAnyTLS(name, server, port, config)
	default:
		return "", fmt.Errorf("不支持序列化协议: %s", protocol)
	}
}

func serializeSS(name, server string, port int, config map[string]interface{}) (string, error) {
	cipher := configStr(config, "cipher")
	password := configStr(config, "password")
	if cipher == "" || password == "" {
		return "", fmt.Errorf("ss 节点缺少 cipher 或 password")
	}
	userInfo := base64.StdEncoding.EncodeToString([]byte(cipher + ":" + password))
	return fmt.Sprintf("ss://%s@%s:%d#%s", userInfo, server, port, url.PathEscape(name)), nil
}

func serializeTrojan(name, server string, port int, config map[string]interface{}) (string, error) {
	password := configStr(config, "password")
	if password == "" {
		return "", fmt.Errorf("trojan 节点缺少 password")
	}
	q := url.Values{}
	applyTLSParams(q, config)
	applyTransportParams(q, config)
	raw := fmt.Sprintf("trojan://%s@%s:%d", url.PathEscape(password), server, port)
	if encoded := q.Encode(); encoded != "" {
		raw += "?" + encoded
	}
	return raw + "#" + url.PathEscape(name), nil
}

func serializeVLESS(name, server string, port int, config map[string]interface{}) (string, error) {
	uuid := configStr(config, "uuid")
	if uuid == "" {
		return "", fmt.Errorf("vless 节点缺少 uuid")
	}
	q := url.Values{}
	q.Set("encryption", "none")
	if flow := configStr(config, "flow"); flow != "" {
		q.Set("flow", flow)
	}
	applyTLSParams(q, config)
	applyTransportParams(q, config)
	raw := fmt.Sprintf("vless://%s@%s:%d", uuid, server, port)
	if encoded := q.Encode(); encoded != "" {
		raw += "?" + encoded
	}
	return raw + "#" + url.PathEscape(name), nil
}

func serializeVMess(name, server string, port int, config map[string]interface{}) (string, error) {
	uuid := configStr(config, "uuid")
	if uuid == "" {
		return "", fmt.Errorf("vmess 节点缺少 uuid")
	}
	alterID := 0
	if v, ok := config["alterID"]; ok {
		switch n := v.(type) {
		case float64:
			alterID = int(n)
		case int:
			alterID = n
		}
	}

	tlsStr := ""
	sni := ""
	if tls, ok := extractTLSOptions(config); ok && tls.Enabled {
		tlsStr = "tls"
		sni = tls.ServerName
	}

	network := "tcp"
	path := ""
	host := ""
	serviceName := ""
	if transport, ok := extractTransportOptions(config); ok && transport != nil {
		if transport.Type != "" {
			network = transport.Type
		}
		path = transport.Path
		host = transport.Host
		serviceName = transport.ServiceName
	}

	type vmessJSON struct {
		V           string `json:"v"`
		PS          string `json:"ps"`
		Add         string `json:"add"`
		Port        int    `json:"port"`
		ID          string `json:"id"`
		Aid         int    `json:"aid"`
		Net         string `json:"net"`
		Type        string `json:"type"`
		Host        string `json:"host"`
		Path        string `json:"path"`
		TLS         string `json:"tls"`
		SNI         string `json:"sni"`
		ServiceName string `json:"serviceName,omitempty"`
	}

	obj := vmessJSON{
		V:           "2",
		PS:          name,
		Add:         server,
		Port:        port,
		ID:          uuid,
		Aid:         alterID,
		Net:         network,
		Type:        "none",
		Host:        host,
		Path:        path,
		TLS:         tlsStr,
		SNI:         sni,
		ServiceName: serviceName,
	}

	jsonBytes, err := json.Marshal(obj)
	if err != nil {
		return "", fmt.Errorf("序列化 vmess JSON 失败: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(jsonBytes)
	return "vmess://" + encoded, nil
}

func serializeHysteria2(name, server string, port int, config map[string]interface{}) (string, error) {
	password := configStr(config, "password")
	if password == "" {
		return "", fmt.Errorf("hysteria2 节点缺少 password")
	}
	q := url.Values{}
	if obfs := configStr(config, "obfs"); obfs != "" {
		q.Set("obfs", obfs)
	}
	if obfsPassword, ok := stringOption(config, "obfs-password", "obfs_password"); ok {
		q.Set("obfs-password", obfsPassword)
	}
	sni := ""
	insecure := false
	var alpn []string
	if tls := configMap(config, "tls"); tls != nil {
		sni = configStr(tls, "server_name", "server-name", "sni")
		insecure, _ = tls["insecure"].(bool)
		alpn, _ = stringSliceOption(tls, "alpn")
	}
	if sni == "" {
		sni = configStr(config, "sni", "server_name", "server-name")
	}
	if !insecure {
		insecure, _ = boolOption(config, "skip-cert-verify", "insecure")
	}
	if len(alpn) == 0 {
		alpn, _ = stringSliceOption(config, "alpn")
	}
	if sni != "" {
		q.Set("sni", sni)
	}
	if insecure {
		q.Set("insecure", "1")
	}
	if len(alpn) > 0 {
		q.Set("alpn", strings.Join(alpn, ","))
	}
	raw := fmt.Sprintf("hysteria2://%s@%s:%d", url.PathEscape(password), server, port)
	if encoded := q.Encode(); encoded != "" {
		raw += "?" + encoded
	}
	return raw + "#" + url.PathEscape(name), nil
}

func serializeTUIC(name, server string, port int, config map[string]interface{}) (string, error) {
	uuid := configStr(config, "uuid")
	password := configStr(config, "password")
	if uuid == "" || password == "" {
		return "", fmt.Errorf("tuic 节点缺少 uuid 或 password")
	}
	q := url.Values{}
	applySimpleTLSParams(q, config)
	if controller := configStr(config, "congestion-controller", "congestion_control"); controller != "" {
		q.Set("congestion_control", controller)
	}
	raw := fmt.Sprintf("tuic://%s:%s@%s:%d", uuid, url.PathEscape(password), server, port)
	if encoded := q.Encode(); encoded != "" {
		raw += "?" + encoded
	}
	return raw + "#" + url.PathEscape(name), nil
}

func serializeAnyTLS(name, server string, port int, config map[string]interface{}) (string, error) {
	password := configStr(config, "password")
	if password == "" {
		return "", fmt.Errorf("anytls 节点缺少 password")
	}
	q := url.Values{}
	applySimpleTLSParams(q, config)
	raw := fmt.Sprintf("anytls://%s@%s:%d", url.PathEscape(password), server, port)
	if encoded := q.Encode(); encoded != "" {
		raw += "?" + encoded
	}
	return raw + "#" + url.PathEscape(name), nil
}

// applyTLSParams 将 TLS 配置写入 query 参数
func applyTLSParams(q url.Values, config map[string]interface{}) {
	tls, ok := extractTLSOptions(config)
	if !ok || tls == nil {
		return
	}
	if !tls.Enabled {
		return
	}

	// 检查是否为 reality
	if reality := tls.Reality; reality != nil {
		q.Set("security", "reality")
		if reality.PublicKey != "" {
			q.Set("pbk", reality.PublicKey)
		}
		if reality.ShortID != "" {
			q.Set("sid", reality.ShortID)
		}
	} else {
		q.Set("security", "tls")
	}

	if tls.ServerName != "" {
		q.Set("sni", tls.ServerName)
	}
	if tls.Insecure {
		q.Set("allowInsecure", "1")
	}
	if len(tls.ALPN) > 0 {
		q.Set("alpn", strings.Join(tls.ALPN, ","))
	}
	if tls.UTLS != nil && tls.UTLS.Fingerprint != "" {
		q.Set("fp", tls.UTLS.Fingerprint)
	}
}

func applySimpleTLSParams(q url.Values, config map[string]interface{}) {
	tls, ok := extractTLSOptions(config)
	if !ok || tls == nil {
		return
	}
	if tls.ServerName != "" {
		q.Set("sni", tls.ServerName)
	}
	if tls.Insecure {
		q.Set("insecure", "1")
	}
	if len(tls.ALPN) > 0 {
		q.Set("alpn", strings.Join(tls.ALPN, ","))
	}
	if tls.UTLS != nil && tls.UTLS.Fingerprint != "" {
		q.Set("fp", tls.UTLS.Fingerprint)
	}
}

// applyTransportParams 将 transport 配置写入 query 参数
func applyTransportParams(q url.Values, config map[string]interface{}) {
	transport, ok := extractTransportOptions(config)
	if !ok || transport == nil {
		return
	}
	transportType := transport.Type
	if transportType == "" || transportType == "tcp" {
		return
	}
	q.Set("type", transportType)
	if transport.Path != "" {
		q.Set("path", transport.Path)
	}
	if transport.Host != "" {
		q.Set("host", transport.Host)
	}
	if transport.ServiceName != "" {
		q.Set("serviceName", transport.ServiceName)
	}
}

// configStr 从 map 中安全提取字符串
func configStr(m map[string]interface{}, keys ...string) string {
	if m == nil {
		return ""
	}
	for _, key := range keys {
		if value, ok := m[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

// configMap 从 map 中安全提取子 map
func configMap(m map[string]interface{}, key string) map[string]interface{} {
	if m == nil {
		return nil
	}
	sub, _ := m[key].(map[string]interface{})
	return sub
}
