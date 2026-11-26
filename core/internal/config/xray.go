package config

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	EngineVPN  = "vpn"
	EngineXray = "xray"

	XraySelectionManual = "manual"
	XraySelectionAuto   = "auto"

	XrayScopeAll          = "all"
	XrayScopeSubscription = "subscription"

	DefaultXrayTestURL = "https://cp.cloudflare.com/generate_204"
)

// Xray configures the embedded Xray-core engine and defaults for its profiles,
// latency tests, auto-failover, and subscriptions.
type Xray struct {
	ActiveProfileID    string `json:"active_profile_id"`
	SelectionMode      string `json:"selection_mode"`
	AutoFailover       bool   `json:"auto_failover"`
	FailoverScope      string `json:"failover_scope"`
	TestURL            string `json:"test_url"`
	SniffEnabled       bool   `json:"sniff_enabled"`
	AllowInsecure      bool   `json:"allow_insecure"`
	Fingerprint        string `json:"fingerprint"`
	MuxEnabled         bool   `json:"mux_enabled"`
	MuxConcurrency     int    `json:"mux_concurrency"`
	FragmentEnabled    bool   `json:"fragment_enabled"`
	FragmentPackets    string `json:"fragment_packets"`
	FragmentLength     string `json:"fragment_length"`
	FragmentInterval   string `json:"fragment_interval"`
	DomainStrategy     string `json:"domain_strategy"`
	DNSMode            string `json:"dns_mode"`
	LogLevel           string `json:"log_level"`
	SubAutoUpdateHours int    `json:"sub_auto_update_hours"`
	SubUserAgent       string `json:"sub_user_agent"`
}

// DefaultXray returns the default Xray configuration.
func DefaultXray() Xray {
	return Xray{
		SelectionMode:      XraySelectionManual,
		AutoFailover:       true,
		FailoverScope:      XrayScopeAll,
		TestURL:            DefaultXrayTestURL,
		SniffEnabled:       true,
		AllowInsecure:      false,
		Fingerprint:        "chrome",
		MuxEnabled:         false,
		MuxConcurrency:     8,
		FragmentEnabled:    false,
		FragmentPackets:    "tlshello",
		FragmentLength:     "100-200",
		FragmentInterval:   "10-20",
		DomainStrategy:     "IPIfNonMatch",
		DNSMode:            "xray",
		LogLevel:           "warning",
		SubAutoUpdateHours: 24,
	}
}

func validateEngineMode(mode string) error {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", EngineVPN, EngineXray:
		return nil
	default:
		return fmt.Errorf("engine_mode must be %q or %q, not %q", EngineVPN, EngineXray, mode)
	}
}

// Normalize fills empty Xray fields with defaults.
func (x *Xray) Normalize() {
	if strings.TrimSpace(x.SelectionMode) == "" {
		x.SelectionMode = XraySelectionManual
	}
	if strings.TrimSpace(x.FailoverScope) == "" {
		x.FailoverScope = XrayScopeAll
	}
	if strings.TrimSpace(x.TestURL) == "" || x.TestURL == "https://www.gstatic.com/generate_204" {
		x.TestURL = DefaultXrayTestURL
	}
	if x.MuxConcurrency <= 0 {
		x.MuxConcurrency = 8
	}
	if strings.TrimSpace(x.FragmentPackets) == "" {
		x.FragmentPackets = "tlshello"
	}
	if strings.TrimSpace(x.FragmentLength) == "" {
		x.FragmentLength = "100-200"
	}
	if strings.TrimSpace(x.FragmentInterval) == "" {
		x.FragmentInterval = "10-20"
	}
	if strings.TrimSpace(x.DomainStrategy) == "" {
		x.DomainStrategy = "IPIfNonMatch"
	}
	if strings.TrimSpace(x.DNSMode) == "" {
		x.DNSMode = "xray"
	}
	if strings.TrimSpace(x.LogLevel) == "" {
		x.LogLevel = "warning"
	}
}

func (x Xray) validate() error {
	switch strings.ToLower(strings.TrimSpace(x.SelectionMode)) {
	case "", XraySelectionManual, XraySelectionAuto:
	default:
		return fmt.Errorf("xray.selection_mode must be %q or %q, not %q", XraySelectionManual, XraySelectionAuto, x.SelectionMode)
	}
	switch strings.ToLower(strings.TrimSpace(x.FailoverScope)) {
	case "", XrayScopeAll, XrayScopeSubscription:
	default:
		return fmt.Errorf("xray.failover_scope must be %q or %q, not %q", XrayScopeAll, XrayScopeSubscription, x.FailoverScope)
	}
	if u := strings.TrimSpace(x.TestURL); u != "" {
		parsed, err := url.Parse(u)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return fmt.Errorf("xray.test_url must be an http or https URL, not %q", u)
		}
	}
	if x.MuxConcurrency < 1 || x.MuxConcurrency > 1024 {
		return fmt.Errorf("xray.mux_concurrency must be between 1 and 1024")
	}
	switch x.DomainStrategy {
	case "", "AsIs", "IPIfNonMatch", "IPOnDemand", "UseIP", "UseIPv4", "UseIPv6":
	default:
		return fmt.Errorf("xray.domain_strategy %q is not valid", x.DomainStrategy)
	}
	switch x.DNSMode {
	case "", "system", "xray", "fakedns":
	default:
		return fmt.Errorf("xray.dns_mode %q must be system, xray, or fakedns", x.DNSMode)
	}
	switch x.LogLevel {
	case "", "none", "error", "warning", "info", "debug":
	default:
		return fmt.Errorf("xray.log_level %q must be none, error, warning, info, or debug", x.LogLevel)
	}
	if x.SubAutoUpdateHours < 0 {
		return fmt.Errorf("xray.sub_auto_update_hours must not be negative")
	}
	return nil
}
