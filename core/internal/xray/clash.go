package xray

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

type clashConfig struct {
	Proxies []map[any]any `yaml:"proxies"`
}

// ParseClashYAML parses Clash / Mihomo YAML configuration (`proxies:` list)
// into Xray Profiles (vless, vmess, trojan, ss, hysteria2, socks5, http).
func ParseClashYAML(raw string) ([]Profile, error) {
	var cfg clashConfig
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, err
	}
	if len(cfg.Proxies) == 0 {
		return nil, errors.New("no proxies found in Clash YAML")
	}
	var out []Profile
	now := time.Now().UTC()
	for _, m := range cfg.Proxies {
		p, ok := profileFromClashMap(m, now)
		if ok {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no supported proxies found in Clash YAML")
	}
	return out, nil
}

func profileFromClashMap(m map[any]any, now time.Time) (Profile, bool) {
	typ := strings.ToLower(yamlStr(m, "type"))
	server := yamlStr(m, "server")
	port := yamlInt(m, "port")
	if typ == "" || server == "" || port <= 0 || port > 65535 {
		return Profile{}, false
	}
	name := yamlStr(m, "name")
	if name == "" {
		name = net.JoinHostPort(server, strconv.Itoa(port))
	}

	p := Profile{
		ID:            newID(),
		Name:          name,
		Address:       server,
		Port:          port,
		Network:       normalizeNetwork(firstNonEmpty(yamlStr(m, "network"), "tcp")),
		SNI:           firstNonEmpty(yamlStr(m, "servername"), yamlStr(m, "sni")),
		Fingerprint:   yamlStr(m, "client-fingerprint"),
		AllowInsecure: yamlBool(m, "skip-cert-verify"),
		ALPN:          yamlStrSlice(m, "alpn"),
		Flow:          yamlStr(m, "flow"),
		AddedAt:       now,
	}

	switch typ {
	case "hysteria2", "hy2":
		p.Protocol = "hysteria2"
		p.Network = "hysteria"
		p.Security = "tls"
		p.Password = firstNonEmpty(yamlStr(m, "password"), yamlStr(m, "auth"))
		if p.Password == "" {
			return Profile{}, false
		}
	case "vless":
		p.Protocol = "vless"
		p.UUID = yamlStr(m, "uuid")
		if p.UUID == "" {
			return Profile{}, false
		}
		p.Encryption = firstNonEmpty(yamlStr(m, "encryption"), "none")
		if yamlBool(m, "tls") {
			p.Security = "tls"
		}
		if ro := yamlMap(m, "reality-opts"); ro != nil {
			p.Security = "reality"
			p.PublicKey = yamlStr(ro, "public-key")
			p.ShortID = yamlStr(ro, "short-id")
			p.SpiderX = yamlStr(ro, "spider-x")
		}
	case "vmess":
		p.Protocol = "vmess"
		p.UUID = yamlStr(m, "uuid")
		if p.UUID == "" {
			return Profile{}, false
		}
		p.AlterID = yamlInt(m, "alterId")
		p.Encryption = firstNonEmpty(yamlStr(m, "cipher"), "auto")
		if yamlBool(m, "tls") {
			p.Security = "tls"
		}
	case "trojan":
		p.Protocol = "trojan"
		p.Password = yamlStr(m, "password")
		if p.Password == "" {
			return Profile{}, false
		}
		p.Security = "tls"
		if m["tls"] != nil && !yamlBool(m, "tls") {
			p.Security = "none"
		}
		if ro := yamlMap(m, "reality-opts"); ro != nil {
			p.Security = "reality"
			p.PublicKey = yamlStr(ro, "public-key")
			p.ShortID = yamlStr(ro, "short-id")
		}
	case "ss", "shadowsocks":
		p.Protocol = "shadowsocks"
		p.Method = yamlStr(m, "cipher")
		p.Password = yamlStr(m, "password")
		p.Network = "tcp"
		if p.Method == "" || p.Password == "" {
			return Profile{}, false
		}
	case "socks5", "socks":
		p.Protocol = "socks"
		p.Username = firstNonEmpty(yamlStr(m, "username"), yamlStr(m, "user"))
		p.Password = yamlStr(m, "password")
		p.Network = "tcp"
		if yamlBool(m, "tls") {
			p.Security = "tls"
		}
	case "http", "https":
		p.Protocol = "http"
		p.Username = firstNonEmpty(yamlStr(m, "username"), yamlStr(m, "user"))
		p.Password = yamlStr(m, "password")
		p.Network = "tcp"
		if typ == "https" || yamlBool(m, "tls") {
			p.Security = "tls"
		}
	default:
		return Profile{}, false
	}

	if ws := yamlMap(m, "ws-opts"); ws != nil {
		p.Path = firstNonEmpty(yamlStr(ws, "path"), p.Path)
		if hdr := yamlMap(ws, "headers"); hdr != nil {
			p.Host = firstNonEmpty(yamlStr(hdr, "Host"), yamlStr(hdr, "host"), p.Host)
		}
	}
	if gr := yamlMap(m, "grpc-opts"); gr != nil {
		p.ServiceName = firstNonEmpty(yamlStr(gr, "grpc-service-name"), yamlStr(gr, "serviceName"), p.ServiceName)
	}
	if h2 := yamlMap(m, "h2-opts"); h2 != nil {
		p.Path = firstNonEmpty(yamlStr(h2, "path"), p.Path)
		if hosts := yamlStrSlice(h2, "host"); len(hosts) > 0 {
			p.Host = strings.Join(hosts, ",")
		}
	}
	if xh := yamlMap(m, "xhttp-opts"); xh != nil {
		p.Path = firstNonEmpty(yamlStr(xh, "path"), p.Path)
		p.Host = firstNonEmpty(yamlStr(xh, "host"), p.Host)
		p.Mode = firstNonEmpty(yamlStr(xh, "mode"), p.Mode)
	}
	if hu := yamlMap(m, "httpupgrade-opts"); hu != nil {
		p.Path = firstNonEmpty(yamlStr(hu, "path"), p.Path)
		p.Host = firstNonEmpty(yamlStr(hu, "host"), p.Host)
	}
	p.RawURI = p.ToURI()
	return p, true
}

func yamlStr(m map[any]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func yamlInt(m map[any]any, key string) int {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(x))
		return n
	default:
		n, _ := strconv.Atoi(strings.TrimSpace(fmt.Sprint(v)))
		return n
	}
}

func yamlBool(m map[any]any, key string) bool {
	v, ok := m[key]
	if !ok || v == nil {
		return false
	}
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return isTruthy(x)
	default:
		return false
	}
}

func yamlMap(m map[any]any, key string) map[any]any {
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	if sub, ok := v.(map[any]any); ok {
		return sub
	}
	return nil
}

func yamlStrSlice(m map[any]any, key string) []string {
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	switch x := v.(type) {
	case []any:
		var out []string
		for _, item := range x {
			if s := strings.TrimSpace(fmt.Sprint(item)); s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		return splitCSV(x)
	default:
		return nil
	}
}
