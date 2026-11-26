package xray

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	xcore "github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/app/proxyman"
	xnet "github.com/xtls/xray-core/common/net"
	cserial "github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/infra/conf/serial"
	"github.com/xtls/xray-core/transport/internet"
	xtls "github.com/xtls/xray-core/transport/internet/tls"

	// Register required Xray-core features, proxies, transports, and JSON loader.
	_ "github.com/xtls/xray-core/app/dispatcher"
	_ "github.com/xtls/xray-core/app/dns"
	_ "github.com/xtls/xray-core/app/log"
	_ "github.com/xtls/xray-core/app/policy"
	_ "github.com/xtls/xray-core/app/proxyman/inbound"
	_ "github.com/xtls/xray-core/app/proxyman/outbound"
	_ "github.com/xtls/xray-core/app/router"
	_ "github.com/xtls/xray-core/main/json"
	_ "github.com/xtls/xray-core/proxy/blackhole"
	_ "github.com/xtls/xray-core/proxy/dns"
	_ "github.com/xtls/xray-core/proxy/freedom"
	_ "github.com/xtls/xray-core/proxy/http"
	_ "github.com/xtls/xray-core/proxy/hysteria"
	_ "github.com/xtls/xray-core/proxy/shadowsocks"
	_ "github.com/xtls/xray-core/proxy/socks"
	_ "github.com/xtls/xray-core/proxy/trojan"
	_ "github.com/xtls/xray-core/proxy/vless/outbound"
	_ "github.com/xtls/xray-core/proxy/vmess/outbound"
	_ "github.com/xtls/xray-core/transport/internet/grpc"
	_ "github.com/xtls/xray-core/transport/internet/headers/http"
	_ "github.com/xtls/xray-core/transport/internet/headers/noop"
	_ "github.com/xtls/xray-core/transport/internet/httpupgrade"
	_ "github.com/xtls/xray-core/transport/internet/hysteria"
	_ "github.com/xtls/xray-core/transport/internet/kcp"
	_ "github.com/xtls/xray-core/transport/internet/reality"
	_ "github.com/xtls/xray-core/transport/internet/splithttp"
	_ "github.com/xtls/xray-core/transport/internet/tagged/taggedimpl"
	_ "github.com/xtls/xray-core/transport/internet/tcp"
	_ "github.com/xtls/xray-core/transport/internet/udp"
	_ "github.com/xtls/xray-core/transport/internet/websocket"

	"github.com/armanlife/arman-vpn-client/core/internal/config"
	"github.com/armanlife/arman-vpn-client/core/internal/egress"
	"github.com/armanlife/arman-vpn-client/core/internal/proxy"
)

var egressOnce sync.Once

func ensureEgressHook() {
	egressOnce.Do(func() {
		_ = internet.RegisterDialerController(func(network, address string, c syscall.RawConn) error {
			return egress.Control(network, address, c)
		})
	})
}

// Engine runs one embedded Xray-core instance and implements RoutedOpener's TCP
// and UDP tunnel opener interfaces.
type Engine struct {
	profile Profile
	inst    *xcore.Instance
	closeMu sync.Mutex
	closed  bool
}

// BuildProtobufConfig builds a ready-to-start Xray protobuf Config for profile and xs.
// xray-core v26+ rejects allowInsecure in JSON after 2026-06-01, so when allowInsecure
// is requested we apply it directly on the decoded *xtls.Config protobuf message.
func BuildProtobufConfig(profile Profile, xs config.Xray) (*xcore.Config, error) {
	rawCfg, err := BuildXrayJSON(profile, xs)
	if err != nil {
		return nil, err
	}
	pbCfg, err := serial.LoadJSONConfig(bytes.NewReader(rawCfg))
	if err != nil {
		return nil, fmt.Errorf("loading xray config: %w", err)
	}
	if profile.AllowInsecure || xs.AllowInsecure {
		applyAllowInsecure(pbCfg)
	}
	return pbCfg, nil
}

func applyAllowInsecure(pbCfg *xcore.Config) {
	if pbCfg == nil {
		return
	}
	for _, ob := range pbCfg.Outbound {
		if ob == nil || ob.SenderSettings == nil {
			continue
		}
		rawSender, err := ob.SenderSettings.GetInstance()
		if err != nil {
			continue
		}
		sender, ok := rawSender.(*proxyman.SenderConfig)
		if !ok || sender.StreamSettings == nil {
			continue
		}
		changed := false
		for i, secMsg := range sender.StreamSettings.SecuritySettings {
			if secMsg == nil {
				continue
			}
			secInst, err := secMsg.GetInstance()
			if err != nil {
				continue
			}
			if tlsCfg, ok := secInst.(*xtls.Config); ok {
				tlsCfg.AllowInsecure = true
				sender.StreamSettings.SecuritySettings[i] = cserial.ToTypedMessage(tlsCfg)
				changed = true
			}
		}
		if changed {
			ob.SenderSettings = cserial.ToTypedMessage(sender)
		}
	}
}

// StartEngine starts an embedded Xray-core instance for profile and settings.
func StartEngine(profile Profile, xs config.Xray) (*Engine, error) {
	ensureEgressHook()
	pbCfg, err := BuildProtobufConfig(profile, xs)
	if err != nil {
		return nil, err
	}
	inst, err := xcore.New(pbCfg)
	if err != nil {
		return nil, fmt.Errorf("creating xray instance: %w", err)
	}
	if err := inst.Start(); err != nil {
		_ = inst.Close()
		return nil, fmt.Errorf("starting xray instance: %w", err)
	}
	return &Engine{profile: profile, inst: inst}, nil
}

// Close shuts down the Xray instance.
func (e *Engine) Close() error {
	e.closeMu.Lock()
	defer e.closeMu.Unlock()
	if e.closed {
		return nil
	}
	e.closed = true
	if e.inst != nil {
		return e.inst.Close()
	}
	return nil
}

// UnrestrictedPorts signals RoutedOpener that Xray carries all TCP and UDP ports.
func (e *Engine) UnrestrictedPorts() bool { return true }

// OpenTunnel opens a TCP stream through Xray to authority (host:port).
func (e *Engine) OpenTunnel(authority string) (net.Conn, error) {
	return e.dialContext(context.Background(), "tcp", authority)
}

// OpenUDP opens a connected UDP tunnel through Xray to authority (host:port).
func (e *Engine) OpenUDP(authority, _ string) (proxy.UDPTunnel, error) {
	conn, err := e.dialContext(context.Background(), "udp", authority)
	if err != nil {
		return nil, err
	}
	return proxy.NewConnUDPTunnel(conn), nil
}

func (e *Engine) dialContext(ctx context.Context, network, authority string) (net.Conn, error) {
	dest, err := xnet.ParseDestination(network + ":" + authority)
	if err != nil {
		return nil, fmt.Errorf("invalid destination %q: %w", authority, err)
	}
	// Xray ties the dispatched link's lifetime to ctx; never cancel ctx when
	// dialContext returns or the active connection pipe closes immediately.
	return xcore.Dial(ctx, e.inst, dest)
}

// Profile returns the active Xray profile served by this engine.
func (e *Engine) Profile() Profile { return e.profile }

// TestProfileLatency measures HTTPS 204 round-trip delay in milliseconds using
// an isolated in-process Xray instance. Returns delay > 0 on success or -1 + error.
func TestProfileLatency(ctx context.Context, p Profile, xs config.Xray, timeout time.Duration) (int64, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	testCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	eng, err := StartEngine(p, xs)
	if err != nil {
		return -1, err
	}
	defer eng.Close()

	target := firstNonEmpty(xs.TestURL, config.DefaultXrayTestURL)
	tr := &http.Transport{
		DisableKeepAlives:   true,
		TLSHandshakeTimeout: timeout,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
		DialContext: func(c context.Context, network, addr string) (net.Conn, error) {
			return eng.dialContext(c, network, addr)
		},
	}
	defer tr.CloseIdleConnections()

	client := &http.Client{Transport: tr, Timeout: timeout}
	req, err := http.NewRequestWithContext(testCtx, http.MethodGet, target, nil)
	if err != nil {
		return -1, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	started := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return -1, err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	_ = resp.Body.Close()

	elapsed := time.Since(started).Milliseconds()
	if elapsed < 1 {
		elapsed = 1
	}
	if resp.StatusCode >= 400 {
		return -1, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return elapsed, nil
}

// TestProfiles measures delays for multiple profiles concurrently (up to maxConcurrency).
func TestProfiles(ctx context.Context, profiles []Profile, xs config.Xray, timeout time.Duration, maxConcurrency int) []DelayResult {
	return TestProfilesStream(ctx, profiles, xs, timeout, maxConcurrency, nil)
}

// TestProfilesStream measures delays concurrently and invokes onResult immediately
// whenever an individual profile finishes testing.
func TestProfilesStream(ctx context.Context, profiles []Profile, xs config.Xray, timeout time.Duration, maxConcurrency int, onResult func(DelayResult)) []DelayResult {
	if maxConcurrency <= 0 {
		maxConcurrency = 6
	}
	results := make([]DelayResult, len(profiles))
	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup
	var cbMu sync.Mutex
	for i, p := range profiles {
		wg.Add(1)
		go func(idx int, prof Profile) {
			defer wg.Done()
			var res DelayResult
			select {
			case sem <- struct{}{}:
				ms, err := TestProfileLatency(ctx, prof, xs, timeout)
				<-sem
				errMsg := ""
				if err != nil {
					errMsg = err.Error()
				}
				res = DelayResult{ID: prof.ID, DelayMS: ms, Error: errMsg}
			case <-ctx.Done():
				res = DelayResult{ID: prof.ID, DelayMS: -1, Error: ctx.Err().Error()}
			}
			results[idx] = res
			if onResult != nil {
				cbMu.Lock()
				onResult(res)
				cbMu.Unlock()
			}
		}(i, p)
	}
	wg.Wait()
	return results
}

// SubFetchResult holds profiles and metadata parsed from a subscription URL.
type SubFetchResult struct {
	Profiles []Profile
	Title    string
	Upload   int64
	Download int64
	Total    int64
	Expire   int64
}

// FetchSubscription downloads subURL and extracts Xray profiles and headers
// (subscription-userinfo, profile-title). dialContext may be nil for direct egress.
func FetchSubscription(ctx context.Context, subURL, userAgent string, dialContext func(context.Context, string, string) (net.Conn, error)) (SubFetchResult, error) {
	if userAgent == "" {
		userAgent = "NikaRay/1.0 v2rayNG/1.9"
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

	client := &http.Client{Transport: tr, Timeout: 25 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(subURL), nil)
	if err != nil {
		return SubFetchResult{}, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "*/*")

	resp, err := client.Do(req)
	if err != nil {
		return SubFetchResult{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return SubFetchResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return SubFetchResult{}, fmt.Errorf("subscription returned HTTP %d", resp.StatusCode)
	}

	var out SubFetchResult
	if info := resp.Header.Get("Subscription-Userinfo"); info != "" {
		out.Upload, out.Download, out.Total, out.Expire = ParseSubscriptionUserinfo(info)
	}
	if title := resp.Header.Get("Profile-Title"); title != "" {
		out.Title = decodeProfileTitle(title)
	}

	profiles, err := ParseImport(string(body))
	if err != nil {
		// Try fallback parsing Clash-like YAML/text URIs? Return the clean error.
		return out, fmt.Errorf("parsing subscription body: %w", err)
	}
	out.Profiles = profiles
	return out, nil
}

// ParseSubscriptionUserinfo extracts upload, download, total, expire from a
// Subscription-Userinfo header (e.g. "upload=0; download=1234; total=50000; expire=1700000000").
func ParseSubscriptionUserinfo(header string) (upload, download, total, expire int64) {
	scanner := bufio.NewScanner(strings.NewReader(strings.ReplaceAll(header, ";", "\n")))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		val, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "upload":
			upload = val
		case "download":
			download = val
		case "total":
			total = val
		case "expire":
			expire = val
		}
	}
	return upload, download, total, expire
}

func decodeProfileTitle(title string) string {
	title = strings.TrimSpace(title)
	if strings.HasPrefix(strings.ToLower(title), "base64:") {
		if b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(title[7:])); err == nil {
			return strings.TrimSpace(string(b))
		}
	}
	return title
}
