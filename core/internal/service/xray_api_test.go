package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/armanlife/arman-vpn-client/core/internal/config"
)

func TestXrayDashboardAPIAndEngineSwitch(t *testing.T) {
	paths, err := ResolveDataPaths(t.TempDir())
	if err != nil {
		t.Fatalf("ResolveDataPaths: %v", err)
	}
	d, err := NewDaemon(paths, false)
	if err != nil {
		t.Fatalf("NewDaemon: %v", err)
	}
	srv := newDashboardServer(d)
	h := srv.handler()

	// 1. Switch engine mode to xray without Firefox accounts.
	reqBody, _ := json.Marshal(map[string]string{"mode": config.EngineXray})
	req := httptest.NewRequest(http.MethodPost, "/api/engine", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Host = "127.0.0.1:10820"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/engine failed: %d %s", rec.Code, rec.Body.String())
	}

	// 2. Import Xray URIs.
	importBody, _ := json.Marshal(map[string]string{
		"input": "vless://22222222-2222-2222-2222-222222222222@example.com:443?security=tls&type=ws#Test-Node\n" +
			"trojan://pass@tr.example.com:443?security=tls#Trojan-Node",
	})
	req = httptest.NewRequest(http.MethodPost, "/api/xray/import", bytes.NewReader(importBody))
	req.Header.Set("Content-Type", "application/json")
	req.Host = "127.0.0.1:10820"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/xray/import failed: %d %s", rec.Code, rec.Body.String())
	}
	var impResp struct {
		Added           int    `json:"added"`
		ActiveProfileID string `json:"active_profile_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &impResp); err != nil {
		t.Fatalf("unmarshal import response: %v", err)
	}
	if impResp.Added != 2 || impResp.ActiveProfileID == "" {
		t.Fatalf("unexpected import response: %+v", impResp)
	}

	// 3. StatusView reflects xray mode and active profile.
	status, err := d.StatusView()
	if err != nil {
		t.Fatalf("StatusView: %v", err)
	}
	if status.EngineMode != config.EngineXray {
		t.Fatalf("got EngineMode=%q, want xray", status.EngineMode)
	}
	if status.Xray == nil || status.Xray.ProfileCount != 2 || status.Xray.ActiveProfile == nil {
		t.Fatalf("unexpected StatusView.Xray: %+v", status.Xray)
	}
}
