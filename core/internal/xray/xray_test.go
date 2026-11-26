package xray

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	xcore "github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf/serial"

	"github.com/armanlife/arman-vpn-client/core/internal/config"
)

func TestParseURIsAllProtocols(t *testing.T) {
	vmessJSON := `{"v":"2","ps":"VMess Node","add":"example.com","port":"443","id":"11111111-1111-1111-1111-111111111111","aid":"0","scy":"auto","net":"ws","type":"none","host":"cdn.example.com","path":"/ws","tls":"tls","sni":"cdn.example.com","alpn":"h2,http/1.1","fp":"chrome"}`
	vmessURI := "vmess://" + base64.StdEncoding.EncodeToString([]byte(vmessJSON))

	cases := []struct {
		name      string
		uri       string
		wantProto string
		wantHost  string
		wantPort  int
		wantSec   string
		wantNet   string
	}{
		{
			name:      "vless reality vision",
			uri:       "vless://22222222-2222-2222-2222-222222222222@1.2.3.4:443?encryption=none&flow=xtls-rprx-vision&security=reality&sni=www.google.com&fp=chrome&pbk=9t9l8h2V0v-uM3rX4jY7zK1pQ5sW8cA6bD4eF2gH0ik&sid=01ab&spx=%2F&type=tcp#Reality-EU",
			wantProto: "vless",
			wantHost:  "1.2.3.4",
			wantPort:  443,
			wantSec:   "reality",
			wantNet:   "tcp",
		},
		{
			name:      "vless xhttp",
			uri:       "vless://22222222-2222-2222-2222-222222222222@example.org:443?security=tls&type=xhttp&path=%2Fxhttp&host=cdn.example.org&mode=packet-up#XHTTP",
			wantProto: "vless",
			wantHost:  "example.org",
			wantPort:  443,
			wantSec:   "tls",
			wantNet:   "xhttp",
		},
		{
			name:      "vmess ws tls",
			uri:       vmessURI,
			wantProto: "vmess",
			wantHost:  "example.com",
			wantPort:  443,
			wantSec:   "tls",
			wantNet:   "ws",
		},
		{
			name:      "trojan grpc",
			uri:       "trojan://secretpass@tr.example.com:8443?security=tls&type=grpc&serviceName=grpcSvc&sni=tr.example.com#Trojan-gRPC",
			wantProto: "trojan",
			wantHost:  "tr.example.com",
			wantPort:  8443,
			wantSec:   "tls",
			wantNet:   "grpc",
		},
		{
			name:      "shadowsocks sip002",
			uri:       "ss://" + base64.RawURLEncoding.EncodeToString([]byte("aes-256-gcm:mypassword")) + "@ss.example.com:8388#SS-Node",
			wantProto: "shadowsocks",
			wantHost:  "ss.example.com",
			wantPort:  8388,
			wantNet:   "tcp",
		},
		{
			name:      "socks5",
			uri:       "socks://user1:pass1@127.0.0.1:1080#LocalSocks",
			wantProto: "socks",
			wantHost:  "127.0.0.1",
			wantPort:  1080,
			wantNet:   "tcp",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := ParseURI(tc.uri)
			if err != nil {
				t.Fatalf("ParseURI(%s) error: %v", tc.uri, err)
			}
			if p.Protocol != tc.wantProto || p.Address != tc.wantHost || p.Port != tc.wantPort {
				t.Fatalf("got (%s, %s, %d), want (%s, %s, %d)", p.Protocol, p.Address, p.Port, tc.wantProto, tc.wantHost, tc.wantPort)
			}
			if p.Security != tc.wantSec || p.Network != tc.wantNet {
				t.Fatalf("got (sec=%q, net=%q), want (sec=%q, net=%q)", p.Security, p.Network, tc.wantSec, tc.wantNet)
			}

			// Round-trip ToURI -> ParseURI.
			exported := p.ToURI()
			p2, err := ParseURI(exported)
			if err != nil {
				t.Fatalf("re-parse ToURI(%q) error: %v", exported, err)
			}
			if p2.Address != p.Address || p2.Port != p.Port || p2.Protocol != p.Protocol {
				t.Fatalf("round-trip mismatch: %+v vs %+v", p2, p)
			}

			// Validate that Xray-core itself accepts the built JSON config.
			xs := config.DefaultXray()
			xs.FragmentEnabled = true
			xs.MuxEnabled = true
			rawCfg, err := BuildXrayJSON(p, xs)
			if err != nil {
				t.Fatalf("BuildXrayJSON error: %v", err)
			}
			pbCfg, err := serial.LoadJSONConfig(bytes.NewReader(rawCfg))
			if err != nil {
				t.Fatalf("serial.LoadJSONConfig failed for %s: %v\nJSON: %s", tc.name, err, string(rawCfg))
			}
			inst, err := xcore.New(pbCfg)
			if err != nil {
				t.Fatalf("xcore.New failed for %s: %v", tc.name, err)
			}
			_ = inst.Close()
		})
	}
}

func TestParseBase64SubscriptionAndJSON(t *testing.T) {
	lines := strings.Join([]string{
		"vless://22222222-2222-2222-2222-222222222222@a.example.com:443?type=ws&security=tls#Node1",
		"trojan://pw@b.example.com:443?security=tls#Node2",
	}, "\n")
	encoded := base64.StdEncoding.EncodeToString([]byte(lines))
	profiles, err := ParseImport(encoded)
	if err != nil {
		t.Fatalf("ParseImport(base64) failed: %v", err)
	}
	if len(profiles) != 2 || profiles[0].Name != "Node1" || profiles[1].Name != "Node2" {
		t.Fatalf("unexpected profiles: %+v", profiles)
	}

	rawJSON := `{
		"remarks": "Raw-JSON-Node",
		"outbounds": [
			{
				"protocol": "vless",
				"settings": {
					"vnext": [{"address": "json.example.com", "port": 443, "users": [{"id": "33333333-3333-3333-3333-333333333333", "encryption": "none"}]}]
				},
				"streamSettings": {"network": "ws", "security": "tls", "wsSettings": {"path": "/ws2"}}
			},
			{"protocol": "freedom"}
		]
	}`
	jsonProfiles, err := ParseImport(rawJSON)
	if err != nil {
		t.Fatalf("ParseImport(JSON) failed: %v", err)
	}
	if len(jsonProfiles) != 1 || jsonProfiles[0].Address != "json.example.com" || jsonProfiles[0].Name != "Raw-JSON-Node" {
		t.Fatalf("unexpected jsonProfiles: %+v", jsonProfiles)
	}
}

func TestSubscriptionFetchAndStoreAndEngineLatency(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/generate_204", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/sub", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Subscription-Userinfo", "upload=1024; download=2048; total=1048576; expire=1900000000")
		w.Header().Set("Profile-Title", "base64:"+base64.StdEncoding.EncodeToString([]byte("Nika Sub")))
		body := base64.StdEncoding.EncodeToString([]byte("vless://11111111-1111-1111-1111-111111111111@1.1.1.1:443?security=tls#S1\n"))
		_, _ = w.Write([]byte(body))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	subRes, err := FetchSubscription(ctx, srv.URL+"/sub", "", nil)
	if err != nil {
		t.Fatalf("FetchSubscription error: %v", err)
	}
	if subRes.Title != "Nika Sub" || subRes.Upload != 1024 || subRes.Download != 2048 || subRes.Total != 1048576 || subRes.Expire != 1900000000 {
		t.Fatalf("unexpected sub metadata: %+v", subRes)
	}
	if len(subRes.Profiles) != 1 || subRes.Profiles[0].Name != "S1" {
		t.Fatalf("unexpected sub profiles: %+v", subRes.Profiles)
	}

	store, err := OpenStore(filepath.Join(t.TempDir(), FileName))
	if err != nil {
		t.Fatalf("OpenStore error: %v", err)
	}
	added, err := store.AddProfiles(subRes.Profiles)
	if err != nil || len(added) != 1 {
		t.Fatalf("AddProfiles error: %v", err)
	}
	if err := store.ApplyDelayResults([]DelayResult{{ID: added[0].ID, DelayMS: 42}}); err != nil {
		t.Fatalf("ApplyDelayResults error: %v", err)
	}
	got, ok := store.ProfileByID(added[0].ID)
	if !ok || got.DelayMS != 42 {
		t.Fatalf("unexpected stored profile: %+v", got)
	}

	// Test custom freedom outbound end-to-end through TestProfileLatency against local HTTP 204 server.
	freedomProf := Profile{
		ID:       "local-direct",
		Name:     "LocalDirect",
		Protocol: "custom",
		RawJSON:  `{"outbounds":[{"tag":"proxy","protocol":"freedom"}]}`,
	}
	xs := config.DefaultXray()
	xs.TestURL = srv.URL + "/generate_204"
	ms, err := TestProfileLatency(ctx, freedomProf, xs, 5*time.Second)
	if err != nil || ms <= 0 {
		t.Fatalf("TestProfileLatency failed: ms=%d err=%v", ms, err)
	}
}

func TestParseFreeConfigsJSONAndCloudflareDefault(t *testing.T) {
	if got := config.DefaultXray().TestURL; got != "https://cp.cloudflare.com/generate_204" {
		t.Fatalf("DefaultXray().TestURL = %q, want cp.cloudflare.com", got)
	}
	sample := `{
		"items": [
			{"name": "Germany Free Sub", "url": "https://example.com/sub.txt", "description": "Fast EU"},
			{"name": "Direct VLESS", "uri": "vless://22222222-2222-2222-2222-222222222222@1.2.3.4:443?security=tls#N"}
		]
	}`
	entries, err := ParseFreeConfigsJSON([]byte(sample))
	if err != nil {
		t.Fatalf("ParseFreeConfigsJSON failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].Kind != "subscription" || entries[0].Name != "Germany Free Sub" {
		t.Fatalf("unexpected entry 0: %+v", entries[0])
	}
	if entries[1].Kind != "config" || entries[1].Name != "Direct VLESS" {
		t.Fatalf("unexpected entry 1: %+v", entries[1])
	}
}

func TestParseClashYAMLWithHysteria2VlessRealitySocks5(t *testing.T) {
	clashYAML := `
proxies:
  - auth: 900c8c92-1979-4c7c-8f2f-63e87d53d8fe
    name: DE_1|1.7MB/s
    password: 900c8c92-1979-4c7c-8f2f-63e87d53d8fe
    port: 13337
    server: 128.254.186.57
    skip-cert-verify: true
    sni: mother-earth.fyi
    type: hysteria2
    udp: true
  - client-fingerprint: chrome
    flow: xtls-rprx-vision
    name: SG_1|3.6MB/s
    network: tcp
    port: 37998
    reality-opts:
      public-key: j7lihAdLvNKWg67WMGQsRSM8WFbaUK5dAi7D2R7iXHM
      short-id: "d5a01375"
    server: vl-x2.qz10086.top
    tls: true
    type: vless
    udp: true
    uuid: d235ad20-8f30-441e-871e-ee2695e4c430
    servername: www.apple.com
  - name: KR_1|1.7MB/s
    port: 1080
    server: 59.11.209.162
    skip-cert-verify: true
    type: socks5
    udp: true
proxy-groups:
  - name: Auto
    type: url-test
rules:
  - MATCH,Auto
`
	profiles, err := ParseImport(clashYAML)
	if err != nil {
		t.Fatalf("ParseImport(Clash YAML) failed: %v", err)
	}
	if len(profiles) != 3 {
		t.Fatalf("got %d profiles, want 3", len(profiles))
	}
	if profiles[0].Protocol != "hysteria2" || profiles[0].Port != 13337 || profiles[0].SNI != "mother-earth.fyi" || !profiles[0].AllowInsecure {
		t.Fatalf("unexpected hysteria2 profile: %+v", profiles[0])
	}
	if profiles[1].Protocol != "vless" || profiles[1].Security != "reality" || profiles[1].PublicKey != "j7lihAdLvNKWg67WMGQsRSM8WFbaUK5dAi7D2R7iXHM" || profiles[1].ShortID != "d5a01375" || profiles[1].Flow != "xtls-rprx-vision" {
		t.Fatalf("unexpected vless reality profile: %+v", profiles[1])
	}
	if profiles[2].Protocol != "socks" || profiles[2].Port != 1080 {
		t.Fatalf("unexpected socks5 profile: %+v", profiles[2])
	}

	xs := config.DefaultXray()
	for _, p := range profiles {
		pbCfg, err := BuildProtobufConfig(p, xs)
		if err != nil {
			t.Fatalf("BuildProtobufConfig(%s) failed: %v", p.Name, err)
		}
		inst, err := xcore.New(pbCfg)
		if err != nil {
			t.Fatalf("xcore.New(%s) failed: %v", p.Name, err)
		}
		_ = inst.Close()
	}
}
