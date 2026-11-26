package xray

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func newID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b[:])
}

func decodeBase64Flexible(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	s = strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, s)
	if s == "" {
		return nil, errors.New("empty base64")
	}
	encodings := []*base64.Encoding{
		base64.RawStdEncoding,
		base64.StdEncoding,
		base64.RawURLEncoding,
		base64.URLEncoding,
	}
	for _, enc := range encodings {
		if b, err := enc.DecodeString(strings.TrimRight(s, "=")); err == nil && len(b) > 0 {
			return b, nil
		}
		if b, err := enc.DecodeString(s); err == nil && len(b) > 0 {
			return b, nil
		}
	}
	return nil, errors.New("invalid base64")
}

// ParseImport parses raw clipboard, file, or subscription text into one or more
// Profiles. It handles single or multi-line URIs (vless://, vmess://, trojan://,
// ss://, socks://, http://, https://), base64-encoded subscription bundles, and
// raw Xray JSON configs or outbound objects/arrays.
func ParseImport(input string) ([]Profile, error) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(input, "\ufeff"))
	if trimmed == "" {
		return nil, errors.New("empty input")
	}
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		if profiles, err := ParseJSONProfiles(trimmed); err == nil && len(profiles) > 0 {
			return profiles, nil
		}
	}
	if looksLikeClashYAML(trimmed) {
		if profiles, err := ParseClashYAML(trimmed); err == nil && len(profiles) > 0 {
			return profiles, nil
		}
	}

	profiles := parseLines(trimmed)
	if len(profiles) > 0 {
		return profiles, nil
	}

	if decoded, err := decodeBase64Flexible(trimmed); err == nil {
		text := strings.TrimSpace(strings.TrimPrefix(string(decoded), "\ufeff"))
		if strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") {
			if profiles, err := ParseJSONProfiles(text); err == nil && len(profiles) > 0 {
				return profiles, nil
			}
		}
		if looksLikeClashYAML(text) {
			if profiles, err := ParseClashYAML(text); err == nil && len(profiles) > 0 {
				return profiles, nil
			}
		}
		if profiles := parseLines(text); len(profiles) > 0 {
			return profiles, nil
		}
	}

	// Single URI explicit error reporting.
	if strings.Contains(trimmed, "://") && !strings.Contains(trimmed, "\n") {
		if p, err := ParseURI(trimmed); err == nil {
			return []Profile{p}, nil
		} else {
			return nil, err
		}
	}
	return nil, errors.New("no valid Xray URIs or JSON config found")
}

func parseLines(text string) []Profile {
	lines := strings.Split(text, "\n")
	var out []Profile
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		if p, err := ParseURI(line); err == nil {
			out = append(out, p)
		}
	}
	return out
}

// ParseURI parses a single proxy URI into a Profile.
func ParseURI(raw string) (Profile, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Profile{}, errors.New("empty URI")
	}
	lower := strings.ToLower(raw)
	switch {
	case strings.HasPrefix(lower, "vmess://"):
		return parseVMessURI(raw)
	case strings.HasPrefix(lower, "vless://"):
		return parseVLESSURI(raw)
	case strings.HasPrefix(lower, "trojan://"):
		return parseTrojanURI(raw)
	case strings.HasPrefix(lower, "ss://"):
		return parseShadowsocksURI(raw)
	case strings.HasPrefix(lower, "hysteria2://"), strings.HasPrefix(lower, "hy2://"):
		return parseHysteria2URI(raw)
	case strings.HasPrefix(lower, "socks://"), strings.HasPrefix(lower, "socks5://"):
		return parseSocksOrHTTPURI(raw, "socks")
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"):
		return parseSocksOrHTTPURI(raw, "http")
	default:
		return Profile{}, fmt.Errorf("unsupported URI scheme")
	}
}

func parseVLESSURI(raw string) (Profile, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Profile{}, fmt.Errorf("invalid vless URI: %w", err)
	}
	host := u.Hostname()
	port, err := strconv.Atoi(u.Port())
	if err != nil || host == "" || port <= 0 || port > 65535 {
		return Profile{}, errors.New("vless URI missing valid host or port")
	}
	uuid := ""
	if u.User != nil {
		uuid = u.User.Username()
	}
	if uuid == "" {
		return Profile{}, errors.New("vless URI missing user ID")
	}
	q := u.Query()
	remark, _ := url.PathUnescape(u.Fragment)
	if remark == "" {
		remark = net.JoinHostPort(host, strconv.Itoa(port))
	}
	p := Profile{
		ID:            newID(),
		Name:          remark,
		Protocol:      "vless",
		Address:       host,
		Port:          port,
		RawURI:        raw,
		UUID:          uuid,
		Encryption:    firstNonEmpty(q.Get("encryption"), "none"),
		Flow:          q.Get("flow"),
		Network:       normalizeNetwork(firstNonEmpty(q.Get("type"), "tcp")),
		HeaderType:    q.Get("headerType"),
		Host:          q.Get("host"),
		Path:          q.Get("path"),
		ServiceName:   q.Get("serviceName"),
		Mode:          q.Get("mode"),
		Extra:         q.Get("extra"),
		Security:      strings.ToLower(q.Get("security")),
		SNI:           firstNonEmpty(q.Get("sni"), q.Get("peer")),
		ALPN:          splitCSV(q.Get("alpn")),
		Fingerprint:   q.Get("fp"),
		AllowInsecure: isTruthy(q.Get("allowInsecure")) || isTruthy(q.Get("insecure")),
		PublicKey:     q.Get("pbk"),
		ShortID:       q.Get("sid"),
		SpiderX:       q.Get("spx"),
		PQV:           q.Get("pqv"),
		AddedAt:       time.Now().UTC(),
	}
	if p.Network == "kcp" && p.Path == "" {
		p.Path = q.Get("seed")
	}
	return p, nil
}

type vmessJSONPayload struct {
	V    any    `json:"v"`
	PS   string `json:"ps"`
	Add  string `json:"add"`
	Port any    `json:"port"`
	ID   string `json:"id"`
	Aid  any    `json:"aid"`
	Scy  string `json:"scy"`
	Net  string `json:"net"`
	Type string `json:"type"`
	Host string `json:"host"`
	Path string `json:"path"`
	TLS  string `json:"tls"`
	SNI  string `json:"sni"`
	ALPN string `json:"alpn"`
	FP   string `json:"fp"`
}

func parseAnyInt(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(x))
		return n
	default:
		return 0
	}
}

func parseVMessURI(raw string) (Profile, error) {
	body := strings.TrimSpace(raw[len("vmess://"):])
	if !strings.Contains(body, "@") {
		if decoded, err := decodeBase64Flexible(body); err == nil {
			var v vmessJSONPayload
			if err := json.Unmarshal(decoded, &v); err == nil && v.Add != "" {
				port := parseAnyInt(v.Port)
				if port <= 0 || port > 65535 || v.ID == "" {
					return Profile{}, errors.New("invalid vmess JSON payload")
				}
				name := strings.TrimSpace(v.PS)
				if name == "" {
					name = net.JoinHostPort(v.Add, strconv.Itoa(port))
				}
				sec := strings.ToLower(strings.TrimSpace(v.TLS))
				if sec != "tls" {
					sec = "none"
				}
				netType := normalizeNetwork(firstNonEmpty(v.Net, "tcp"))
				serviceName := ""
				if netType == "grpc" {
					serviceName = v.Path
				}
				return Profile{
					ID:          newID(),
					Name:        name,
					Protocol:    "vmess",
					Address:     strings.TrimSpace(v.Add),
					Port:        port,
					RawURI:      raw,
					UUID:        strings.TrimSpace(v.ID),
					AlterID:     parseAnyInt(v.Aid),
					Encryption:  firstNonEmpty(v.Scy, "auto"),
					Network:     netType,
					HeaderType:  v.Type,
					Host:        v.Host,
					Path:        v.Path,
					ServiceName: serviceName,
					Security:    sec,
					SNI:         v.SNI,
					ALPN:        splitCSV(v.ALPN),
					Fingerprint: v.FP,
					AddedAt:     time.Now().UTC(),
				}, nil
			}
		}
	}
	// SIP-style vmess://uuid@host:port?params#name
	u, err := url.Parse(raw)
	if err != nil {
		return Profile{}, fmt.Errorf("invalid vmess URI: %w", err)
	}
	host := u.Hostname()
	port, err := strconv.Atoi(u.Port())
	if err != nil || host == "" || port <= 0 || port > 65535 {
		return Profile{}, errors.New("vmess URI missing valid host or port")
	}
	uuid := ""
	if u.User != nil {
		uuid = u.User.Username()
	}
	if uuid == "" {
		return Profile{}, errors.New("vmess URI missing user ID")
	}
	q := u.Query()
	remark, _ := url.PathUnescape(u.Fragment)
	if remark == "" {
		remark = net.JoinHostPort(host, strconv.Itoa(port))
	}
	return Profile{
		ID:            newID(),
		Name:          remark,
		Protocol:      "vmess",
		Address:       host,
		Port:          port,
		RawURI:        raw,
		UUID:          uuid,
		Encryption:    firstNonEmpty(q.Get("encryption"), "auto"),
		Network:       normalizeNetwork(firstNonEmpty(q.Get("type"), "tcp")),
		HeaderType:    q.Get("headerType"),
		Host:          q.Get("host"),
		Path:          q.Get("path"),
		ServiceName:   q.Get("serviceName"),
		Security:      strings.ToLower(q.Get("security")),
		SNI:           q.Get("sni"),
		ALPN:          splitCSV(q.Get("alpn")),
		Fingerprint:   q.Get("fp"),
		AllowInsecure: isTruthy(q.Get("allowInsecure")) || isTruthy(q.Get("insecure")),
		AddedAt:       time.Now().UTC(),
	}, nil
}

func parseTrojanURI(raw string) (Profile, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Profile{}, fmt.Errorf("invalid trojan URI: %w", err)
	}
	host := u.Hostname()
	port := 443
	if p := u.Port(); p != "" {
		port, err = strconv.Atoi(p)
		if err != nil {
			return Profile{}, errors.New("invalid trojan port")
		}
	}
	if host == "" || port <= 0 || port > 65535 {
		return Profile{}, errors.New("trojan URI missing valid host or port")
	}
	pw := ""
	if u.User != nil {
		pw = u.User.Username()
	}
	if pw == "" {
		return Profile{}, errors.New("trojan URI missing password")
	}
	q := u.Query()
	remark, _ := url.PathUnescape(u.Fragment)
	if remark == "" {
		remark = net.JoinHostPort(host, strconv.Itoa(port))
	}
	sec := strings.ToLower(firstNonEmpty(q.Get("security"), "tls"))
	return Profile{
		ID:            newID(),
		Name:          remark,
		Protocol:      "trojan",
		Address:       host,
		Port:          port,
		RawURI:        raw,
		Password:      pw,
		Flow:          q.Get("flow"),
		Network:       normalizeNetwork(firstNonEmpty(q.Get("type"), "tcp")),
		HeaderType:    q.Get("headerType"),
		Host:          q.Get("host"),
		Path:          q.Get("path"),
		ServiceName:   q.Get("serviceName"),
		Mode:          q.Get("mode"),
		Security:      sec,
		SNI:           firstNonEmpty(q.Get("sni"), q.Get("peer"), host),
		ALPN:          splitCSV(q.Get("alpn")),
		Fingerprint:   q.Get("fp"),
		AllowInsecure: isTruthy(q.Get("allowInsecure")) || isTruthy(q.Get("insecure")),
		PublicKey:     q.Get("pbk"),
		ShortID:       q.Get("sid"),
		SpiderX:       q.Get("spx"),
		AddedAt:       time.Now().UTC(),
	}, nil
}

func parseShadowsocksURI(raw string) (Profile, error) {
	withoutScheme := raw[len("ss://"):]
	remark := ""
	if idx := strings.IndexByte(withoutScheme, '#'); idx >= 0 {
		remark, _ = url.PathUnescape(withoutScheme[idx+1:])
		withoutScheme = withoutScheme[:idx]
	}
	if idx := strings.IndexByte(withoutScheme, '?'); idx >= 0 {
		withoutScheme = withoutScheme[:idx]
	}

	var method, password, hostPort string
	if atIdx := strings.LastIndexByte(withoutScheme, '@'); atIdx >= 0 {
		userInfo := withoutScheme[:atIdx]
		hostPort = withoutScheme[atIdx+1:]
		if colon := strings.IndexByte(userInfo, ':'); colon >= 0 {
			method = userInfo[:colon]
			password = userInfo[colon+1:]
		} else if decoded, err := decodeBase64Flexible(userInfo); err == nil {
			parts := strings.SplitN(string(decoded), ":", 2)
			if len(parts) == 2 {
				method, password = parts[0], parts[1]
			}
		}
	} else {
		decoded, err := decodeBase64Flexible(withoutScheme)
		if err != nil {
			return Profile{}, errors.New("invalid shadowsocks URI")
		}
		// Legacy format: method:password@host:port
		s := string(decoded)
		atIdx := strings.LastIndexByte(s, '@')
		if atIdx < 0 {
			return Profile{}, errors.New("invalid legacy shadowsocks URI")
		}
		parts := strings.SplitN(s[:atIdx], ":", 2)
		if len(parts) != 2 {
			return Profile{}, errors.New("invalid shadowsocks credentials")
		}
		method, password = parts[0], parts[1]
		hostPort = s[atIdx+1:]
	}

	host, portStr, err := net.SplitHostPort(strings.TrimSuffix(hostPort, "/"))
	if err != nil {
		return Profile{}, errors.New("shadowsocks URI missing host:port")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || host == "" || port <= 0 || port > 65535 {
		return Profile{}, errors.New("shadowsocks URI has invalid port")
	}
	if method == "" || password == "" {
		return Profile{}, errors.New("shadowsocks URI missing method or password")
	}
	if remark == "" {
		remark = net.JoinHostPort(host, portStr)
	}
	return Profile{
		ID:       newID(),
		Name:     remark,
		Protocol: "shadowsocks",
		Address:  host,
		Port:     port,
		RawURI:   raw,
		Method:   method,
		Password: password,
		Network:  "tcp",
		AddedAt:  time.Now().UTC(),
	}, nil
}

func parseSocksOrHTTPURI(raw, proto string) (Profile, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Profile{}, fmt.Errorf("invalid %s URI: %w", proto, err)
	}
	host := u.Hostname()
	portStr := u.Port()
	if portStr == "" {
		switch strings.ToLower(u.Scheme) {
		case "https":
			portStr = "443"
		case "http":
			portStr = "8080"
		default:
			portStr = "1080"
		}
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || host == "" || port <= 0 || port > 65535 {
		return Profile{}, fmt.Errorf("%s URI missing valid host or port", proto)
	}
	user, pass := "", ""
	if u.User != nil {
		user = u.User.Username()
		pass, _ = u.User.Password()
		// Some socks:// base64 userinfo format (e.g. socks://base64(user:pass)@host:port)
		if pass == "" && user != "" && !strings.Contains(raw, ":"+user) {
			if decoded, err := decodeBase64Flexible(user); err == nil {
				if parts := strings.SplitN(string(decoded), ":", 2); len(parts) == 2 {
					user, pass = parts[0], parts[1]
				}
			}
		}
	}
	remark, _ := url.PathUnescape(u.Fragment)
	if remark == "" {
		remark = net.JoinHostPort(host, portStr)
	}
	sec := ""
	if strings.EqualFold(u.Scheme, "https") {
		sec = "tls"
	}
	return Profile{
		ID:       newID(),
		Name:     remark,
		Protocol: proto,
		Address:  host,
		Port:     port,
		RawURI:   raw,
		Username: user,
		Password: pass,
		Security: sec,
		Network:  "tcp",
		AddedAt:  time.Now().UTC(),
	}, nil
}

// ParseJSONProfiles imports an Xray JSON config (full config with outbounds[], a
// single outbound object, or a JSON array of outbounds/configs).
func ParseJSONProfiles(raw string) ([]Profile, error) {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "[") {
		var items []json.RawMessage
		if err := json.Unmarshal([]byte(trimmed), &items); err != nil {
			return nil, fmt.Errorf("invalid JSON array: %w", err)
		}
		var all []Profile
		for _, item := range items {
			ps, err := ParseJSONProfiles(string(item))
			if err == nil {
				all = append(all, ps...)
			}
		}
		if len(all) == 0 {
			return nil, errors.New("no valid Xray outbounds in JSON array")
		}
		return all, nil
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &doc); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	// Single outbound object: {"protocol": "...", ...}
	if _, hasProto := doc["protocol"]; hasProto {
		p, err := profileFromOutboundJSON(trimmed, "")
		if err != nil {
			return nil, err
		}
		return []Profile{p}, nil
	}

	// Full Xray config: extract primary proxy outbound(s), preserve full JSON if custom/complex.
	rawOutbounds, ok := doc["outbounds"]
	if !ok {
		return nil, errors.New("JSON must contain \"protocol\" or \"outbounds\"")
	}
	var outbounds []json.RawMessage
	if err := json.Unmarshal(rawOutbounds, &outbounds); err != nil {
		return nil, fmt.Errorf("invalid outbounds array: %w", err)
	}
	remark := ""
	if r, ok := doc["remarks"]; ok {
		_ = json.Unmarshal(r, &remark)
	}
	var profiles []Profile
	for _, ob := range outbounds {
		var header struct {
			Protocol string `json:"protocol"`
			Tag      string `json:"tag"`
		}
		if err := json.Unmarshal(ob, &header); err != nil {
			continue
		}
		switch strings.ToLower(header.Protocol) {
		case "", "freedom", "blackhole", "dns", "loopback":
			continue
		}
		p, err := profileFromOutboundJSON(string(ob), firstNonEmpty(remark, header.Tag))
		if err != nil {
			// Still allow complex custom full config if it passes xray loader later.
			p = Profile{
				ID:       newID(),
				Name:     firstNonEmpty(remark, header.Tag, header.Protocol),
				Protocol: "custom",
				RawJSON:  trimmed,
				AddedAt:  time.Now().UTC(),
			}
		}
		profiles = append(profiles, p)
	}
	if len(profiles) == 0 {
		return nil, errors.New("no proxy outbound found in Xray JSON")
	}
	return profiles, nil
}

func profileFromOutboundJSON(rawOutbound, remark string) (Profile, error) {
	var ob struct {
		Protocol string `json:"protocol"`
		Tag      string `json:"tag"`
		Settings struct {
			Vnext []struct {
				Address string `json:"address"`
				Port    int    `json:"port"`
				Users   []struct {
					ID         string `json:"id"`
					AlterID    int    `json:"alterId"`
					Security   string `json:"security"`
					Encryption string `json:"encryption"`
					Flow       string `json:"flow"`
				} `json:"users"`
			} `json:"vnext"`
			Servers []struct {
				Address  string `json:"address"`
				Port     int    `json:"port"`
				Password string `json:"password"`
				Method   string `json:"method"`
				Users    []struct {
					User string `json:"user"`
					Pass string `json:"pass"`
				} `json:"users"`
			} `json:"servers"`
		} `json:"settings"`
		StreamSettings struct {
			Network         string `json:"network"`
			Security        string `json:"security"`
			TLSSettings     *struct {
				ServerName    string   `json:"serverName"`
				ALPN          []string `json:"alpn"`
				Fingerprint   string   `json:"fingerprint"`
				AllowInsecure bool     `json:"allowInsecure"`
			} `json:"tlsSettings"`
			RealitySettings *struct {
				ServerName  string `json:"serverName"`
				Fingerprint string `json:"fingerprint"`
				PublicKey   string `json:"publicKey"`
				ShortID     string `json:"shortId"`
				SpiderX     string `json:"spiderX"`
			} `json:"realitySettings"`
			WSSettings *struct {
				Path    string            `json:"path"`
				Host    string            `json:"host"`
				Headers map[string]string `json:"headers"`
			} `json:"wsSettings"`
			GRPCSettings *struct {
				ServiceName string `json:"serviceName"`
				MultiMode   bool   `json:"multiMode"`
			} `json:"grpcSettings"`
			HTTPUpgradeSettings *struct {
				Path string `json:"path"`
				Host string `json:"host"`
			} `json:"httpupgradeSettings"`
			XHTTPSettings *struct {
				Path string `json:"path"`
				Host string `json:"host"`
				Mode string `json:"mode"`
			} `json:"xhttpSettings"`
			SplitHTTPSettings *struct {
				Path string `json:"path"`
				Host string `json:"host"`
				Mode string `json:"mode"`
			} `json:"splithttpSettings"`
			TCPSettings *struct {
				Header struct {
					Type string `json:"type"`
				} `json:"header"`
			} `json:"tcpSettings"`
		} `json:"streamSettings"`
	}
	if err := json.Unmarshal([]byte(rawOutbound), &ob); err != nil {
		return Profile{}, err
	}
	proto := strings.ToLower(strings.TrimSpace(ob.Protocol))
	p := Profile{
		ID:       newID(),
		Protocol: proto,
		RawJSON:  rawOutbound,
		Network:  normalizeNetwork(firstNonEmpty(ob.StreamSettings.Network, "tcp")),
		Security: strings.ToLower(ob.StreamSettings.Security),
		AddedAt:  time.Now().UTC(),
	}
	if len(ob.Settings.Vnext) > 0 {
		v := ob.Settings.Vnext[0]
		p.Address, p.Port = v.Address, v.Port
		if len(v.Users) > 0 {
			u := v.Users[0]
			p.UUID = u.ID
			p.AlterID = u.AlterID
			p.Encryption = firstNonEmpty(u.Encryption, u.Security)
			p.Flow = u.Flow
		}
	} else if len(ob.Settings.Servers) > 0 {
		s := ob.Settings.Servers[0]
		p.Address, p.Port = s.Address, s.Port
		p.Password = s.Password
		p.Method = s.Method
		if len(s.Users) > 0 {
			p.Username = s.Users[0].User
			p.Password = firstNonEmpty(s.Users[0].Pass, s.Password)
		}
	}
	if p.Address == "" || p.Port <= 0 {
		return Profile{}, errors.New("outbound JSON missing server address or port")
	}
	if t := ob.StreamSettings.TLSSettings; t != nil {
		p.SNI = t.ServerName
		p.ALPN = t.ALPN
		p.Fingerprint = t.Fingerprint
		p.AllowInsecure = t.AllowInsecure
	}
	if r := ob.StreamSettings.RealitySettings; r != nil {
		p.SNI = firstNonEmpty(r.ServerName, p.SNI)
		p.Fingerprint = firstNonEmpty(r.Fingerprint, p.Fingerprint)
		p.PublicKey = r.PublicKey
		p.ShortID = r.ShortID
		p.SpiderX = r.SpiderX
	}
	if ws := ob.StreamSettings.WSSettings; ws != nil {
		p.Path = ws.Path
		p.Host = firstNonEmpty(ws.Host, ws.Headers["Host"])
	}
	if gr := ob.StreamSettings.GRPCSettings; gr != nil {
		p.ServiceName = gr.ServiceName
		if gr.MultiMode {
			p.Mode = "multi"
		}
	}
	if hu := ob.StreamSettings.HTTPUpgradeSettings; hu != nil {
		p.Path, p.Host = hu.Path, hu.Host
	}
	if xh := ob.StreamSettings.XHTTPSettings; xh != nil {
		p.Path, p.Host, p.Mode = xh.Path, xh.Host, xh.Mode
	} else if sh := ob.StreamSettings.SplitHTTPSettings; sh != nil {
		p.Path, p.Host, p.Mode = sh.Path, sh.Host, sh.Mode
	}
	if tcp := ob.StreamSettings.TCPSettings; tcp != nil && tcp.Header.Type != "" {
		p.HeaderType = tcp.Header.Type
	}
	p.Name = firstNonEmpty(remark, ob.Tag, net.JoinHostPort(p.Address, strconv.Itoa(p.Port)))
	return p, nil
}

// ToURI encodes a Profile as a standard shareable URI.
func (p Profile) ToURI() string {
	switch strings.ToLower(p.Protocol) {
	case "vmess":
		payload := map[string]any{
			"v":    "2",
			"ps":   p.Name,
			"add":  p.Address,
			"port": strconv.Itoa(p.Port),
			"id":   p.UUID,
			"aid":  strconv.Itoa(p.AlterID),
			"scy":  firstNonEmpty(p.Encryption, "auto"),
			"net":  firstNonEmpty(p.Network, "tcp"),
			"type": firstNonEmpty(p.HeaderType, "none"),
			"host": p.Host,
			"path": firstNonEmpty(p.Path, p.ServiceName),
			"tls":  ifThen(strings.EqualFold(p.Security, "tls"), "tls", ""),
			"sni":  p.SNI,
			"alpn": strings.Join(p.ALPN, ","),
			"fp":   p.Fingerprint,
		}
		b, _ := json.Marshal(payload)
		return "vmess://" + base64.StdEncoding.EncodeToString(b)
	case "vless":
		q := buildCommonQuery(p)
		if p.Encryption != "" && p.Encryption != "none" {
			q.Set("encryption", p.Encryption)
		} else {
			q.Set("encryption", "none")
		}
		if p.Flow != "" {
			q.Set("flow", p.Flow)
		}
		u := url.URL{
			Scheme:   "vless",
			User:     url.User(p.UUID),
			Host:     net.JoinHostPort(p.Address, strconv.Itoa(p.Port)),
			RawQuery: q.Encode(),
			Fragment: p.Name,
		}
		return u.String()
	case "trojan":
		q := buildCommonQuery(p)
		if p.Flow != "" {
			q.Set("flow", p.Flow)
		}
		u := url.URL{
			Scheme:   "trojan",
			User:     url.User(p.Password),
			Host:     net.JoinHostPort(p.Address, strconv.Itoa(p.Port)),
			RawQuery: q.Encode(),
			Fragment: p.Name,
		}
		return u.String()
	case "shadowsocks":
		cred := base64.RawURLEncoding.EncodeToString([]byte(p.Method + ":" + p.Password))
		u := url.URL{
			Scheme:   "ss",
			User:     url.User(cred),
			Host:     net.JoinHostPort(p.Address, strconv.Itoa(p.Port)),
			Fragment: p.Name,
		}
		return u.String()
	case "hysteria2":
		q := url.Values{}
		if p.SNI != "" {
			q.Set("sni", p.SNI)
		}
		if p.AllowInsecure {
			q.Set("insecure", "1")
		}
		if len(p.ALPN) > 0 {
			q.Set("alpn", strings.Join(p.ALPN, ","))
		}
		u := url.URL{
			Scheme:   "hysteria2",
			User:     url.User(p.Password),
			Host:     net.JoinHostPort(p.Address, strconv.Itoa(p.Port)),
			RawQuery: q.Encode(),
			Fragment: p.Name,
		}
		return u.String()
	case "socks":
		var user *url.Userinfo
		if p.Username != "" {
			user = url.UserPassword(p.Username, p.Password)
		}
		u := url.URL{
			Scheme:   "socks",
			User:     user,
			Host:     net.JoinHostPort(p.Address, strconv.Itoa(p.Port)),
			Fragment: p.Name,
		}
		return u.String()
	case "http":
		scheme := "http"
		if strings.EqualFold(p.Security, "tls") {
			scheme = "https"
		}
		var user *url.Userinfo
		if p.Username != "" {
			user = url.UserPassword(p.Username, p.Password)
		}
		u := url.URL{
			Scheme:   scheme,
			User:     user,
			Host:     net.JoinHostPort(p.Address, strconv.Itoa(p.Port)),
			Fragment: p.Name,
		}
		return u.String()
	default:
		if p.RawURI != "" {
			return p.RawURI
		}
		return p.RawJSON
	}
}

func parseHysteria2URI(raw string) (Profile, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Profile{}, fmt.Errorf("invalid hysteria2 URI: %w", err)
	}
	host := u.Hostname()
	port := 443
	if p := u.Port(); p != "" {
		port, err = strconv.Atoi(p)
		if err != nil {
			return Profile{}, errors.New("invalid hysteria2 port")
		}
	}
	if host == "" || port <= 0 || port > 65535 {
		return Profile{}, errors.New("hysteria2 URI missing valid host or port")
	}
	pw := ""
	if u.User != nil {
		pw = u.User.Username()
	}
	if pw == "" {
		return Profile{}, errors.New("hysteria2 URI missing auth password")
	}
	q := u.Query()
	remark, _ := url.PathUnescape(u.Fragment)
	if remark == "" {
		remark = net.JoinHostPort(host, strconv.Itoa(port))
	}
	return Profile{
		ID:            newID(),
		Name:          remark,
		Protocol:      "hysteria2",
		Address:       host,
		Port:          port,
		RawURI:        raw,
		Password:      pw,
		Network:       "hysteria",
		Security:      "tls",
		SNI:           firstNonEmpty(q.Get("sni"), q.Get("peer"), host),
		ALPN:          splitCSV(q.Get("alpn")),
		AllowInsecure: isTruthy(q.Get("insecure")) || isTruthy(q.Get("allowInsecure")),
		AddedAt:       time.Now().UTC(),
	}, nil
}

func looksLikeClashYAML(text string) bool {
	return strings.Contains(text, "proxies:")
}

func buildCommonQuery(p Profile) url.Values {
	q := url.Values{}
	netType := normalizeNetwork(firstNonEmpty(p.Network, "tcp"))
	q.Set("type", netType)
	if p.Security != "" {
		q.Set("security", p.Security)
	}
	if p.HeaderType != "" {
		q.Set("headerType", p.HeaderType)
	}
	if p.Host != "" {
		q.Set("host", p.Host)
	}
	if p.Path != "" {
		q.Set("path", p.Path)
	}
	if p.ServiceName != "" {
		q.Set("serviceName", p.ServiceName)
	}
	if p.Mode != "" {
		q.Set("mode", p.Mode)
	}
	if p.SNI != "" {
		q.Set("sni", p.SNI)
	}
	if len(p.ALPN) > 0 {
		q.Set("alpn", strings.Join(p.ALPN, ","))
	}
	if p.Fingerprint != "" {
		q.Set("fp", p.Fingerprint)
	}
	if p.AllowInsecure {
		q.Set("allowInsecure", "1")
	}
	if p.PublicKey != "" {
		q.Set("pbk", p.PublicKey)
	}
	if p.ShortID != "" {
		q.Set("sid", p.ShortID)
	}
	if p.SpiderX != "" {
		q.Set("spx", p.SpiderX)
	}
	return q
}

func normalizeNetwork(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "raw":
		return "tcp"
	case "splithttp":
		return "xhttp"
	default:
		return strings.ToLower(strings.TrimSpace(s))
	}
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func isTruthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func ifThen(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}
