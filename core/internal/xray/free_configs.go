package xray

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/armanlife/arman-vpn-client/core/internal/egress"
)

// DefaultFreeConfigsURLs lists raw GitHub locations checked for sl-HapY/nikaray's configs.json.
var DefaultFreeConfigsURLs = []string{
	"https://raw.githubusercontent.com/sl-HapY/nikaray/main/configs.json",
	"https://raw.githubusercontent.com/sl-HapY/nikaray/master/configs.json",
}

// FreeConfigEntry represents one named subscription or config item offered in
// the remote configs.json catalog.
type FreeConfigEntry struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Kind        string `json:"kind"` // "subscription" | "config"
	URL         string `json:"url,omitempty"`
	Config      string `json:"config,omitempty"`
}

// FetchFreeConfigs fetches configs.json from GitHub (or custom URL) and extracts
// named subscription / config entries.
func FetchFreeConfigs(ctx context.Context, urls []string, dialContext func(context.Context, string, string) (net.Conn, error)) ([]FreeConfigEntry, string, error) {
	if len(urls) == 0 {
		urls = DefaultFreeConfigsURLs
	}
	if dialContext == nil {
		dialContext = func(c context.Context, netw, addr string) (net.Conn, error) {
			return egress.Dialer(15*time.Second).DialContext(c, netw, addr)
		}
	}
	tr := &http.Transport{
		DialContext:         dialContext,
		TLSHandshakeTimeout: 15 * time.Second,
	}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 20 * time.Second}

	var lastErr error
	for _, u := range urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("User-Agent", "NikaRay/1.0")
		req.Header.Set("Accept", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode == http.StatusNotFound {
			lastErr = fmt.Errorf("configs.json not found (%d)", resp.StatusCode)
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("GitHub returned HTTP %d", resp.StatusCode)
			continue
		}
		entries, err := ParseFreeConfigsJSON(body)
		if err != nil {
			lastErr = err
			continue
		}
		return entries, u, nil
	}
	if lastErr == nil {
		lastErr = errors.New("no remote configs.json URL configured")
	}
	return nil, "", lastErr
}

// ParseFreeConfigsJSON supports flexible JSON shapes for configs.json:
//   - JSON array: [{"name": "...", "url": "..."}, ...]
//   - Object with items/subscriptions/free_configs/configs array
func ParseFreeConfigsJSON(raw []byte) ([]FreeConfigEntry, error) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(string(raw), "\ufeff"))
	if trimmed == "" {
		return nil, errors.New("empty configs.json")
	}

	var rawItems []json.RawMessage
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal([]byte(trimmed), &rawItems); err != nil {
			return nil, fmt.Errorf("invalid configs.json array: %w", err)
		}
	} else {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
			return nil, fmt.Errorf("invalid configs.json object: %w", err)
		}
		for _, key := range []string{"items", "subscriptions", "free_configs", "configs", "servers"} {
			if val, ok := obj[key]; ok {
				if err := json.Unmarshal(val, &rawItems); err == nil {
					break
				}
			}
		}
	}

	var out []FreeConfigEntry
	for idx, rawItem := range rawItems {
		var m map[string]any
		if err := json.Unmarshal(rawItem, &m); err != nil {
			continue
		}
		name := firstString(m, "name", "title", "remark", "label")
		desc := firstString(m, "description", "note", "info", "country")
		subURL := firstString(m, "url", "sub_url", "sub", "subscription", "subscription_url")
		cfgText := firstString(m, "config", "configs", "uri", "link", "data", "raw")
		if cfgText == "" {
			if list, ok := m["uris"].([]any); ok {
				var lines []string
				for _, v := range list {
					if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
						lines = append(lines, strings.TrimSpace(s))
					}
				}
				cfgText = strings.Join(lines, "\n")
			}
		}
		if subURL == "" && cfgText == "" {
			continue
		}
		// If subURL is actually a proxy scheme (vless://, vmess://, etc.), treat as config.
		if subURL != "" && !strings.HasPrefix(strings.ToLower(subURL), "http://") && !strings.HasPrefix(strings.ToLower(subURL), "https://") {
			if cfgText == "" {
				cfgText = subURL
			}
			subURL = ""
		}
		kind := "config"
		if subURL != "" {
			kind = "subscription"
		}
		if name == "" {
			name = fmt.Sprintf("Free #%d", idx+1)
		}
		id := firstString(m, "id")
		if id == "" {
			id = fmt.Sprintf("free-%d-%s", idx+1, strings.ToLower(strings.ReplaceAll(name, " ", "-")))
		}
		out = append(out, FreeConfigEntry{
			ID:          id,
			Name:        name,
			Description: desc,
			Kind:        kind,
			URL:         subURL,
			Config:      cfgText,
		})
	}
	if len(out) == 0 {
		return nil, errors.New("no valid entries in configs.json")
	}
	return out, nil
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}
