package xray

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/armanlife/arman-vpn-client/core/internal/config"
)

// BuildXrayJSON builds an Xray-core JSON configuration for p and settings xs.
// Xray acts strictly as an in-process outbound engine driven via core.Dial, so
// no local listening sockets are opened here.
func BuildXrayJSON(p Profile, xs config.Xray) ([]byte, error) {
	if p.Protocol == "custom" && strings.TrimSpace(p.RawJSON) != "" {
		var custom map[string]any
		if err := json.Unmarshal([]byte(p.RawJSON), &custom); err == nil {
			if _, ok := custom["outbounds"]; ok {
				// Strip inbounds so in-process core.Dial won't fight over local ports.
				delete(custom, "inbounds")
				return json.Marshal(custom)
			}
		}
	}

	outbound, extraOutbounds, err := buildOutbound(p, xs)
	if err != nil {
		return nil, err
	}

	outbounds := []any{outbound}
	outbounds = append(outbounds, extraOutbounds...)
	outbounds = append(outbounds,
		map[string]any{"tag": "direct", "protocol": "freedom"},
		map[string]any{"tag": "block", "protocol": "blackhole"},
	)

	logLevel := xs.LogLevel
	if strings.TrimSpace(logLevel) == "" {
		logLevel = "warning"
	}

	root := map[string]any{
		"log": map[string]any{
			"loglevel": logLevel,
		},
		"outbounds": outbounds,
		"routing": map[string]any{
			"domainStrategy": firstNonEmpty(xs.DomainStrategy, "IPIfNonMatch"),
			"rules":          []any{},
		},
	}

	return json.Marshal(root)
}

func buildOutbound(p Profile, xs config.Xray) (any, []any, error) {
	proto := strings.ToLower(strings.TrimSpace(p.Protocol))
	if proto == "" {
		return nil, nil, errors.New("profile missing protocol")
	}

	// Preserve arbitrary outbound JSON if this profile uses custom raw outbound.
	if p.RawJSON != "" && proto == "custom" {
		var ob map[string]any
		if err := json.Unmarshal([]byte(p.RawJSON), &ob); err == nil {
			if _, hasProto := ob["protocol"]; hasProto {
				ob["tag"] = "proxy"
				return ob, nil, nil
			}
		}
	}

	if p.Address == "" || p.Port <= 0 || p.Port > 65535 {
		return nil, nil, fmt.Errorf("%s profile %q missing valid address/port", proto, p.Name)
	}

	var settings map[string]any
	switch proto {
	case "vless":
		user := map[string]any{
			"id":         p.UUID,
			"encryption": firstNonEmpty(p.Encryption, "none"),
		}
		if p.Flow != "" {
			user["flow"] = p.Flow
		}
		settings = map[string]any{
			"vnext": []any{
				map[string]any{
					"address": p.Address,
					"port":    p.Port,
					"users":   []any{user},
				},
			},
		}
	case "vmess":
		settings = map[string]any{
			"vnext": []any{
				map[string]any{
					"address": p.Address,
					"port":    p.Port,
					"users": []any{
						map[string]any{
							"id":       p.UUID,
							"alterId":  p.AlterID,
							"security": firstNonEmpty(p.Encryption, "auto"),
						},
					},
				},
			},
		}
	case "trojan":
		settings = map[string]any{
			"servers": []any{
				map[string]any{
					"address":  p.Address,
					"port":     p.Port,
					"password": p.Password,
				},
			},
		}
	case "shadowsocks":
		settings = map[string]any{
			"servers": []any{
				map[string]any{
					"address":  p.Address,
					"port":     p.Port,
					"method":   p.Method,
					"password": p.Password,
				},
			},
		}
	case "socks":
		srv := map[string]any{
			"address": p.Address,
			"port":    p.Port,
		}
		if p.Username != "" {
			srv["users"] = []any{map[string]any{"user": p.Username, "pass": p.Password}}
		}
		settings = map[string]any{"servers": []any{srv}}
	case "http":
		srv := map[string]any{
			"address": p.Address,
			"port":    p.Port,
		}
		if p.Username != "" {
			srv["users"] = []any{map[string]any{"user": p.Username, "pass": p.Password}}
		}
		settings = map[string]any{"servers": []any{srv}}
	case "hysteria2", "hysteria":
		settings = map[string]any{
			"version": 2,
			"address": p.Address,
			"port":    p.Port,
		}
		proto = "hysteria"
	default:
		return nil, nil, fmt.Errorf("unsupported Xray protocol %q", proto)
	}

	stream, extras := buildStreamSettings(p, xs)
	if proto == "hysteria" {
		stream["network"] = "hysteria"
		if stream["security"] == "none" || stream["security"] == "" {
			stream["security"] = "tls"
			stream["tlsSettings"] = map[string]any{
				"serverName": firstNonEmpty(p.SNI, p.Host, p.Address),
			}
		}
		stream["hysteriaSettings"] = map[string]any{
			"version": 2,
			"auth":    p.Password,
		}
	}
	outbound := map[string]any{
		"tag":            "proxy",
		"protocol":       proto,
		"settings":       settings,
		"streamSettings": stream,
	}

	// XTLS-Vision and Hysteria2 do not use mux.
	if xs.MuxEnabled && proto != "hysteria" && !strings.Contains(strings.ToLower(p.Flow), "vision") && p.Network != "xhttp" && p.Network != "splithttp" {
		conc := xs.MuxConcurrency
		if conc <= 0 {
			conc = 8
		}
		outbound["mux"] = map[string]any{
			"enabled":     true,
			"concurrency": conc,
		}
	}

	return outbound, extras, nil
}

func buildStreamSettings(p Profile, xs config.Xray) (map[string]any, []any) {
	netType := normalizeNetwork(firstNonEmpty(p.Network, "tcp"))
	stream := map[string]any{
		"network": netType,
	}

	sec := strings.ToLower(strings.TrimSpace(p.Security))
	fp := firstNonEmpty(p.Fingerprint, xs.Fingerprint, "chrome")

	switch sec {
	case "tls":
		stream["security"] = "tls"
		tlsCfg := map[string]any{
			"serverName":  firstNonEmpty(p.SNI, p.Host, p.Address),
			"fingerprint": fp,
		}
		if len(p.ALPN) > 0 {
			tlsCfg["alpn"] = p.ALPN
		}
		stream["tlsSettings"] = tlsCfg
	case "reality":
		stream["security"] = "reality"
		realityCfg := map[string]any{
			"serverName":  firstNonEmpty(p.SNI, p.Address),
			"fingerprint": fp,
			"publicKey":   p.PublicKey,
			"shortId":     p.ShortID,
			"spiderX":     p.SpiderX,
		}
		if p.PQV != "" {
			realityCfg["mldsa65Verify"] = p.PQV
		}
		stream["realitySettings"] = realityCfg
	default:
		stream["security"] = "none"
	}

	switch netType {
	case "tcp":
		if strings.EqualFold(p.HeaderType, "http") {
			hosts := []string{}
			if p.Host != "" {
				hosts = splitCSV(p.Host)
			}
			paths := []string{"/"}
			if p.Path != "" {
				paths = []string{p.Path}
			}
			stream["tcpSettings"] = map[string]any{
				"header": map[string]any{
					"type": "http",
					"request": map[string]any{
						"path": paths,
						"headers": map[string]any{
							"Host": hosts,
						},
					},
				},
			}
		}
	case "ws":
		wsCfg := map[string]any{
			"path": firstNonEmpty(p.Path, "/"),
		}
		if p.Host != "" {
			wsCfg["headers"] = map[string]any{"Host": p.Host}
			wsCfg["host"] = p.Host
		}
		stream["wsSettings"] = wsCfg
	case "grpc":
		grpcCfg := map[string]any{
			"serviceName": firstNonEmpty(p.ServiceName, p.Path),
			"multiMode":   strings.EqualFold(p.Mode, "multi"),
		}
		stream["grpcSettings"] = grpcCfg
	case "httpupgrade":
		huCfg := map[string]any{
			"path": firstNonEmpty(p.Path, "/"),
		}
		if p.Host != "" {
			huCfg["host"] = p.Host
		}
		stream["httpupgradeSettings"] = huCfg
	case "xhttp", "splithttp":
		stream["network"] = "xhttp"
		xhCfg := map[string]any{
			"path": firstNonEmpty(p.Path, "/"),
			"mode": firstNonEmpty(p.Mode, "auto"),
		}
		if p.Host != "" {
			xhCfg["host"] = p.Host
		}
		if p.Extra != "" {
			var extra any
			if json.Unmarshal([]byte(p.Extra), &extra) == nil {
				xhCfg["extra"] = extra
			}
		}
		stream["xhttpSettings"] = xhCfg
	case "kcp":
		kcpCfg := map[string]any{}
		if p.HeaderType != "" {
			kcpCfg["header"] = map[string]any{"type": p.HeaderType}
		}
		if p.Path != "" {
			kcpCfg["seed"] = p.Path
		}
		stream["kcpSettings"] = kcpCfg
	case "h2", "http":
		stream["network"] = "http"
		h2Cfg := map[string]any{
			"path": firstNonEmpty(p.Path, "/"),
		}
		if p.Host != "" {
			h2Cfg["host"] = splitCSV(p.Host)
		}
		stream["httpSettings"] = h2Cfg
	}

	var extras []any
	if xs.FragmentEnabled && sec == "tls" {
		stream["sockopt"] = map[string]any{
			"dialerProxy": "fragment",
		}
		extras = append(extras, map[string]any{
			"tag":      "fragment",
			"protocol": "freedom",
			"settings": map[string]any{
				"fragment": map[string]any{
					"packets":  firstNonEmpty(xs.FragmentPackets, "tlshello"),
					"length":   firstNonEmpty(xs.FragmentLength, "100-200"),
					"interval": firstNonEmpty(xs.FragmentInterval, "10-20"),
				},
			},
			"streamSettings": map[string]any{
				"sockopt": map[string]any{
					"tcpNoDelay": true,
				},
			},
		})
	}

	return stream, extras
}
