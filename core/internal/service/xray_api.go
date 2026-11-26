package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/armanlife/arman-vpn-client/core/internal/config"
	"github.com/armanlife/arman-vpn-client/core/internal/logx"
	"github.com/armanlife/arman-vpn-client/core/internal/proxy"
	"github.com/armanlife/arman-vpn-client/core/internal/xray"
)

var errNoXrayProfile = errors.New("no Xray configuration selected")

// XrayStatusView summarizes Xray state in GET /api/status.
type XrayStatusView struct {
	SelectionMode     string        `json:"selection_mode"`
	AutoFailover      bool          `json:"auto_failover"`
	FailoverScope     string        `json:"failover_scope"`
	ActiveProfile     *xray.Profile `json:"active_profile,omitempty"`
	ProfileCount      int           `json:"profile_count"`
	SubscriptionCount int           `json:"subscription_count"`
}

func effectiveEngineMode(cfg config.Config) string {
	if strings.EqualFold(strings.TrimSpace(cfg.EngineMode), config.EngineXray) {
		return config.EngineXray
	}
	return config.EngineVPN
}

func (d *Daemon) isXrayMode() bool {
	return effectiveEngineMode(d.config()) == config.EngineXray
}

func (d *Daemon) activeXrayProfile(cfg config.Config) (xray.Profile, bool) {
	if d.xrayStore == nil {
		return xray.Profile{}, false
	}
	if id := strings.TrimSpace(cfg.Xray.ActiveProfileID); id != "" {
		if p, ok := d.xrayStore.ProfileByID(id); ok {
			return p, true
		}
	}
	return d.xrayStore.FirstProfile()
}

func (d *Daemon) xrayTestURL() string {
	u := strings.TrimSpace(d.config().Xray.TestURL)
	if u != "" {
		return u
	}
	return config.DefaultXrayTestURL
}

func (d *Daemon) xrayStatusView(cfg config.Config) *XrayStatusView {
	if d.xrayStore == nil {
		return nil
	}
	snap := d.xrayStore.Snapshot()
	xs := cfg.Xray
	xs.Normalize()
	v := &XrayStatusView{
		SelectionMode:     xs.SelectionMode,
		AutoFailover:      xs.AutoFailover,
		FailoverScope:     xs.FailoverScope,
		ProfileCount:      len(snap.Profiles),
		SubscriptionCount: len(snap.Subscriptions),
	}
	if p, ok := d.activeXrayProfile(cfg); ok {
		cp := p
		v.ActiveProfile = &cp
	}
	return v
}

// ensureXrayEngine returns the running xray.Engine, waking it if sleeping, and
// calls release when finished.
func (d *Daemon) ensureXrayEngine(e *proxyEngine, why wakeReason) (*xray.Engine, func(), error) {
	if eng := e.reserveXray(); eng != nil {
		return eng, e.releaseFunc(), nil
	}
	e.idle.wakeMu.Lock()
	defer e.idle.wakeMu.Unlock()
	if eng := e.reserveXray(); eng != nil {
		return eng, e.releaseFunc(), nil
	}
	waitReason := why
	d.mu.Lock()
	if d.connectAttempt != nil {
		waitReason = wakeForClient
	}
	d.mu.Unlock()
	if err, wait := e.wakeWait(waitReason, time.Now()); err != nil {
		at, _, _ := e.wakeFailure()
		return nil, nil, fmt.Errorf("starting Xray failed %s ago; trying again in %s: %w", time.Since(at).Round(time.Second), wait.Round(time.Second), err)
	}
	if err := d.wakeXray(e, why); err != nil {
		return nil, nil, err
	}
	if eng := e.reserveXray(); eng != nil {
		return eng, e.releaseFunc(), nil
	}
	return nil, nil, errProxyNotRunning
}

func (e *proxyEngine) reserveXray() *xray.Engine {
	e.idle.mu.Lock()
	defer e.idle.mu.Unlock()
	if e.xrayEngine != nil {
		e.idle.inUse++
		e.idle.lastUsed = time.Now()
	}
	return e.xrayEngine
}

func (e *proxyEngine) currentXray() *xray.Engine {
	e.idle.mu.Lock()
	defer e.idle.mu.Unlock()
	return e.xrayEngine
}

func (d *Daemon) wakeXray(e *proxyEngine, why wakeReason) error {
	d.mu.Lock()
	current, attempt := d.engine == e && !d.off, d.connectAttempt
	d.mu.Unlock()
	if !current {
		return errProxyNotRunning
	}
	started := time.Now()
	eng, err := xray.StartEngine(e.xrayProfile, e.cfg.Xray)
	if err != nil {
		now := time.Now()
		e.idle.mu.Lock()
		e.idle.wakeFailures++
		wait := proxy.WakeRetryDelay(err, e.idle.wakeFailures, now)
		if attempt != nil {
			wait = min(wait, connectRetryDelay)
		}
		e.idle.wakeErr, e.idle.wakeCode, e.idle.wakeFailedAt = err, proxy.FailureOther, now
		e.idle.wakeRetryAt, e.idle.wakeLasting = now.Add(wait), false
		e.idle.mu.Unlock()
		logx.Warn("starting Xray failed profile=%q retry_in=%s err=%s", e.xrayProfile.Name, wait, logx.Err(err))
		d.noteConnectFailure(e, attempt, proxy.FailureOther, false)
		if d.shouldAutoFailoverXray(e.cfg.Xray) {
			go d.failoverXrayProfile(e.xrayProfile.ID)
		}
		return err
	}
	e.idle.mu.Lock()
	if e.idle.stopped {
		e.idle.mu.Unlock()
		_ = eng.Close()
		return errProxyNotRunning
	}
	e.xrayEngine = eng
	e.connected = true
	e.idle.sleeping = time.Time{}
	e.idle.wakeErr, e.idle.wakeCode, e.idle.wakeFailedAt = nil, "", time.Time{}
	e.idle.wakeRetryAt, e.idle.wakeFailures, e.idle.wakeLasting = time.Time{}, 0, false
	e.idle.mu.Unlock()
	d.markConnected(e, attempt)
	logx.Info("Xray started profile=%q protocol=%s took=%s", e.xrayProfile.Name, e.xrayProfile.Protocol, time.Since(started).Round(time.Millisecond))
	if why != wakeForTest {
		go d.autoTest(e, "proxy connected")
	}
	return nil
}

// SetEngineMode switches between "vpn" and "xray".
func (d *Daemon) SetEngineMode(mode string) error {
	norm := strings.ToLower(strings.TrimSpace(mode))
	if norm != config.EngineVPN && norm != config.EngineXray {
		return fmt.Errorf("unknown engine mode %q", mode)
	}
	d.mu.Lock()
	cfg := d.cfg
	d.mu.Unlock()
	if effectiveEngineMode(cfg) == norm {
		return nil
	}
	cfg.EngineMode = norm
	_, err := d.updateSettings(cfg, nil)
	return err
}

func (d *Daemon) setActiveXrayProfile(id string) error {
	id = strings.TrimSpace(id)
	if id != "" {
		if _, ok := d.xrayStore.ProfileByID(id); !ok {
			return fmt.Errorf("profile %q not found", id)
		}
	}
	cfg := d.config()
	if cfg.Xray.ActiveProfileID == id {
		return nil
	}
	cfg.Xray.ActiveProfileID = id
	if _, err := d.updateSettings(cfg, nil); err != nil {
		return err
	}
	if effectiveEngineMode(cfg) == config.EngineXray {
		d.mu.Lock()
		d.exit = nil
		d.lastTest = nil
		d.mu.Unlock()
		d.RequestRestart()
	}
	return nil
}

func (s *dashboardServer) handleSetEngineMode(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Mode string `json:"mode"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if err := s.d.SetEngineMode(in.Mode); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"engine_mode": effectiveEngineMode(s.d.config())})
}

func (s *dashboardServer) handleXrayState(w http.ResponseWriter, r *http.Request) {
	cfg := s.d.config()
	snap := s.d.xrayStore.Snapshot()
	activeID := cfg.Xray.ActiveProfileID
	if activeID != "" {
		if _, ok := s.d.xrayStore.ProfileByID(activeID); !ok {
			activeID = ""
		}
	}
	if activeID == "" && len(snap.Profiles) > 0 {
		activeID = snap.Profiles[0].ID
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"engine_mode":       effectiveEngineMode(cfg),
		"active_profile_id": activeID,
		"profiles":          snap.Profiles,
		"subscriptions":     snap.Subscriptions,
		"settings":          cfg.Xray,
	})
}

func (s *dashboardServer) handleXrayImport(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Input string `json:"input"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	parsed, err := xray.ParseImport(in.Input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	added, err := s.d.xrayStore.AddProfiles(parsed)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cfg := s.d.config()
	if cfg.Xray.ActiveProfileID == "" && len(added) > 0 {
		_ = s.d.setActiveXrayProfile(added[0].ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"added":             len(added),
		"profiles":          added,
		"active_profile_id": s.d.config().Xray.ActiveProfileID,
	})
}

func (s *dashboardServer) handleXrayProfileUpsert(w http.ResponseWriter, r *http.Request) {
	var p xray.Profile
	if !readJSON(w, r, &p) {
		return
	}
	if pathID := r.PathValue("id"); pathID != "" {
		p.ID = pathID
	}
	saved, err := s.d.xrayStore.UpsertProfile(p)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	cfg := s.d.config()
	if cfg.Xray.ActiveProfileID == "" {
		_ = s.d.setActiveXrayProfile(saved.ID)
	} else if cfg.Xray.ActiveProfileID == saved.ID && effectiveEngineMode(cfg) == config.EngineXray {
		s.d.RequestRestart()
	}
	writeJSON(w, http.StatusOK, map[string]any{"profile": saved})
}

func (s *dashboardServer) handleXrayProfileDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.d.xrayStore.DeleteProfile(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	cfg := s.d.config()
	if cfg.Xray.ActiveProfileID == id {
		nextID := ""
		if first, ok := s.d.xrayStore.FirstProfile(); ok {
			nextID = first.ID
		}
		_ = s.d.setActiveXrayProfile(nextID)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *dashboardServer) handleXrayProfileActivate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.d.setActiveXrayProfile(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"active_profile_id": id})
}

func (s *dashboardServer) handleXrayProfilesTest(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []string `json:"ids"`
	}
	if r.ContentLength > 0 {
		if !readJSON(w, r, &in) {
			return
		}
	}
	targets := s.d.selectXrayTargets(in.IDs)
	results := xray.TestProfiles(r.Context(), targets, s.d.config().Xray, 8*time.Second, 6)
	_ = s.d.xrayStore.ApplyDelayResults(results)
	_ = s.d.xrayStore.SortByDelay()
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (s *dashboardServer) handleXrayProfilesTestStream(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []string `json:"ids"`
	}
	if r.ContentLength > 0 {
		if !readJSON(w, r, &in) {
			return
		}
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.handleXrayProfilesTest(w, r)
		return
	}
	targets := s.d.selectXrayTargets(in.IDs)
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	var writeMu = make(chan struct{}, 1)
	results := xray.TestProfilesStream(r.Context(), targets, s.d.config().Xray, 8*time.Second, 6, func(res xray.DelayResult) {
		s.d.xrayStore.ApplyDelayResult(res)
		line, err := json.Marshal(res)
		if err != nil {
			return
		}
		writeMu <- struct{}{}
		_, _ = w.Write(append(line, '\n'))
		flusher.Flush()
		<-writeMu
	})
	_ = s.d.xrayStore.ApplyDelayResults(results)
	_ = s.d.xrayStore.SortByDelay()
}

func (s *dashboardServer) handleXrayConnectBest(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs     []string `json:"ids"`
		Connect bool     `json:"connect"`
	}
	if r.ContentLength > 0 {
		if !readJSON(w, r, &in) {
			return
		}
	}
	best, err := s.d.selectBestXrayProfile(r.Context(), in.IDs, "")
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if err := s.d.setActiveXrayProfile(best.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if in.Connect && s.d.isOff() {
		s.d.Connect()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"active_profile_id": best.ID,
		"delay_ms":          best.DelayMS,
		"profile":           best,
	})
}

func (s *dashboardServer) handleXrayFreeCatalog(w http.ResponseWriter, r *http.Request) {
	var dialContext func(context.Context, string, string) (net.Conn, error)
	if eng := s.d.currentEngine(); eng != nil && eng.routed != nil && !s.d.isOff() {
		dialContext = func(_ context.Context, _, addr string) (net.Conn, error) {
			return eng.routed.OpenTunnel(addr)
		}
	}
	entries, source, err := xray.FetchFreeConfigs(r.Context(), nil, dialContext)
	if err != nil && dialContext != nil {
		entries, source, err = xray.FetchFreeConfigs(r.Context(), nil, nil)
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  entries,
		"source": source,
	})
}

func (s *dashboardServer) handleXrayFreeAdd(w http.ResponseWriter, r *http.Request) {
	var entry xray.FreeConfigEntry
	if !readJSON(w, r, &entry) {
		return
	}
	if strings.TrimSpace(entry.URL) != "" {
		sub, err := s.d.xrayStore.AddSubscription(xray.Subscription{
			Name:       strings.TrimSpace(entry.Name),
			URL:        strings.TrimSpace(entry.URL),
			Enabled:    true,
			AutoUpdate: true,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		updatedSub, added, fetchErr := s.d.refreshSubscription(r.Context(), sub.ID)
		if fetchErr != nil {
			writeError(w, http.StatusBadGateway, fetchErr.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"kind":         "subscription",
			"subscription": updatedSub,
			"added":        added,
		})
		return
	}
	parsed, err := xray.ParseImport(entry.Config)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(parsed) == 1 && strings.TrimSpace(entry.Name) != "" {
		parsed[0].Name = strings.TrimSpace(entry.Name)
	}
	added, err := s.d.xrayStore.AddProfiles(parsed)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s.d.config().Xray.ActiveProfileID == "" && len(added) > 0 {
		_ = s.d.setActiveXrayProfile(added[0].ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"kind":     "config",
		"added":    len(added),
		"profiles": added,
	})
}

func (s *dashboardServer) handleXrayProfilesSort(w http.ResponseWriter, r *http.Request) {
	if err := s.d.xrayStore.SortByDelay(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"sorted": true})
}

func (s *dashboardServer) handleXrayProfilesCleanup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RemoveFailed     bool `json:"remove_failed"`
		RemoveDuplicates bool `json:"remove_duplicates"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	removed, err := s.d.xrayStore.Cleanup(in.RemoveFailed, in.RemoveDuplicates)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cfg := s.d.config()
	if cfg.Xray.ActiveProfileID != "" {
		if _, ok := s.d.xrayStore.ProfileByID(cfg.Xray.ActiveProfileID); !ok {
			nextID := ""
			if first, ok := s.d.xrayStore.FirstProfile(); ok {
				nextID = first.ID
			}
			_ = s.d.setActiveXrayProfile(nextID)
		}
	}
	writeJSON(w, http.StatusOK, map[string]int{"removed": removed})
}

func (s *dashboardServer) handleXraySubAdd(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name       string `json:"name"`
		URL        string `json:"url"`
		AutoUpdate bool   `json:"auto_update"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	u := strings.TrimSpace(in.URL)
	if u == "" || (!strings.HasPrefix(strings.ToLower(u), "http://") && !strings.HasPrefix(strings.ToLower(u), "https://")) {
		writeError(w, http.StatusBadRequest, "subscription url must start with http:// or https://")
		return
	}
	sub, err := s.d.xrayStore.AddSubscription(xray.Subscription{
		Name:       strings.TrimSpace(in.Name),
		URL:        u,
		Enabled:    true,
		AutoUpdate: in.AutoUpdate,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	updatedSub, added, fetchErr := s.d.refreshSubscription(r.Context(), sub.ID)
	if fetchErr != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"subscription": updatedSub,
			"added":        0,
			"error":        fetchErr.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"subscription": updatedSub,
		"added":        added,
	})
}

func (s *dashboardServer) handleXraySubUpdateMeta(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sub, ok := s.d.xrayStore.SubscriptionByID(id)
	if !ok {
		writeError(w, http.StatusNotFound, "subscription not found")
		return
	}
	var in struct {
		Name       *string `json:"name"`
		URL        *string `json:"url"`
		Enabled    *bool   `json:"enabled"`
		AutoUpdate *bool   `json:"auto_update"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Name != nil && strings.TrimSpace(*in.Name) != "" {
		sub.Name = strings.TrimSpace(*in.Name)
	}
	if in.URL != nil && strings.TrimSpace(*in.URL) != "" {
		sub.URL = strings.TrimSpace(*in.URL)
	}
	if in.Enabled != nil {
		sub.Enabled = *in.Enabled
	}
	if in.AutoUpdate != nil {
		sub.AutoUpdate = *in.AutoUpdate
	}
	if err := s.d.xrayStore.UpdateSubscription(sub); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"subscription": sub})
}

func (s *dashboardServer) handleXraySubDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	keep := r.URL.Query().Get("keep_profiles") == "true"
	if err := s.d.xrayStore.DeleteSubscription(id, keep); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	cfg := s.d.config()
	if cfg.Xray.ActiveProfileID != "" {
		if _, ok := s.d.xrayStore.ProfileByID(cfg.Xray.ActiveProfileID); !ok {
			nextID := ""
			if first, ok := s.d.xrayStore.FirstProfile(); ok {
				nextID = first.ID
			}
			_ = s.d.setActiveXrayProfile(nextID)
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *dashboardServer) handleXraySubRefresh(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sub, count, err := s.d.refreshSubscription(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"subscription": sub, "count": count})
}

func (s *dashboardServer) handleXraySubRefreshAll(w http.ResponseWriter, r *http.Request) {
	snap := s.d.xrayStore.Snapshot()
	updated := 0
	var errs []string
	for _, sub := range snap.Subscriptions {
		if !sub.Enabled {
			continue
		}
		if _, _, err := s.d.refreshSubscription(r.Context(), sub.ID); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", sub.Name, err))
		} else {
			updated++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated": updated, "errors": errs})
}

func (d *Daemon) refreshSubscription(ctx context.Context, id string) (xray.Subscription, int, error) {
	sub, ok := d.xrayStore.SubscriptionByID(id)
	if !ok {
		return xray.Subscription{}, 0, fmt.Errorf("subscription %q not found", id)
	}
	ua := d.config().Xray.SubUserAgent

	var dialContext func(context.Context, string, string) (net.Conn, error)
	if eng := d.currentEngine(); eng != nil && eng.routed != nil && !d.isOff() {
		dialContext = func(_ context.Context, _, addr string) (net.Conn, error) {
			return eng.routed.OpenTunnel(addr)
		}
	}

	res, err := xray.FetchSubscription(ctx, sub.URL, ua, dialContext)
	if err != nil && dialContext != nil {
		// Fallback to direct egress in case active tunnel doesn't reach subscription host.
		res, err = xray.FetchSubscription(ctx, sub.URL, ua, nil)
	}
	if err != nil {
		sub.LastError = err.Error()
		_ = d.xrayStore.UpdateSubscription(sub)
		return sub, 0, err
	}

	if (sub.Name == "" || sub.Name == "Subscription") && res.Title != "" {
		sub.Name = res.Title
	}
	sub.UpdatedAt = time.Now().UTC()
	sub.Upload, sub.Download, sub.Total, sub.Expire = res.Upload, res.Download, res.Total, res.Expire
	sub.LastError = ""
	sub.ProfileCount = len(res.Profiles)

	prevActive := d.config().Xray.ActiveProfileID
	newActive, err := d.xrayStore.ReplaceSubscriptionProfiles(sub, res.Profiles, prevActive)
	if err != nil {
		return sub, 0, err
	}
	if newActive != prevActive || (prevActive == "" && len(res.Profiles) > 0) {
		if newActive == "" && len(res.Profiles) > 0 {
			newActive = res.Profiles[0].ID
		}
		_ = d.setActiveXrayProfile(newActive)
	}
	return sub, len(res.Profiles), nil
}

func (d *Daemon) runXraySubUpdates() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-d.stop:
			return
		case <-ticker.C:
		}
		hours := d.config().Xray.SubAutoUpdateHours
		if hours <= 0 || d.xrayStore == nil {
			continue
		}
		interval := time.Duration(hours) * time.Hour
		snap := d.xrayStore.Snapshot()
		for _, sub := range snap.Subscriptions {
			if !sub.Enabled || !sub.AutoUpdate {
				continue
			}
			if time.Since(sub.UpdatedAt) >= interval {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				_, _, _ = d.refreshSubscription(ctx, sub.ID)
				cancel()
			}
		}
	}
}

func (d *Daemon) shouldAutoFailoverXray(xs config.Xray) bool {
	xs.Normalize()
	return xs.SelectionMode == config.XraySelectionAuto || xs.AutoFailover
}

func (d *Daemon) selectXrayTargets(ids []string) []xray.Profile {
	snap := d.xrayStore.Snapshot()
	if len(ids) == 0 {
		return snap.Profiles
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var targets []xray.Profile
	for _, p := range snap.Profiles {
		if want[p.ID] {
			targets = append(targets, p)
		}
	}
	return targets
}

func (d *Daemon) selectBestXrayProfile(ctx context.Context, ids []string, excludeID string) (xray.Profile, error) {
	cfg := d.config()
	xs := cfg.Xray
	xs.Normalize()
	snap := d.xrayStore.Snapshot()
	if len(snap.Profiles) == 0 {
		return xray.Profile{}, errNoXrayProfile
	}
	candidates := d.selectXrayTargets(ids)
	if len(ids) == 0 && xs.FailoverScope == config.XrayScopeSubscription {
		if active, ok := d.activeXrayProfile(cfg); ok && active.SubscriptionID != "" {
			var scoped []xray.Profile
			for _, p := range candidates {
				if p.SubscriptionID == active.SubscriptionID {
					scoped = append(scoped, p)
				}
			}
			if len(scoped) > 0 {
				candidates = scoped
			}
		}
	}
	if excludeID != "" && len(candidates) > 1 {
		var filtered []xray.Profile
		for _, p := range candidates {
			if p.ID != excludeID {
				filtered = append(filtered, p)
			}
		}
		if len(filtered) > 0 {
			candidates = filtered
		}
	}
	// Prioritize known-working and untested configs before previously failed ones,
	// then cap batch test size so huge subscriptions connect quickly.
	sort.SliceStable(candidates, func(i, j int) bool {
		return autoPickRank(candidates[i].DelayMS) < autoPickRank(candidates[j].DelayMS)
	})
	const maxAutoCandidates = 24
	if len(candidates) > maxAutoCandidates {
		candidates = candidates[:maxAutoCandidates]
	}

	probeCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	results := xray.TestProfilesStream(probeCtx, candidates, xs, 5*time.Second, 8, func(res xray.DelayResult) {
		d.xrayStore.ApplyDelayResult(res)
	})
	_ = d.xrayStore.ApplyDelayResults(results)
	_ = d.xrayStore.SortByDelay()

	var best xray.Profile
	var bestDelay int64 = -1
	delayMap := map[string]int64{}
	for _, r := range results {
		delayMap[r.ID] = r.DelayMS
	}
	for _, p := range candidates {
		ms := delayMap[p.ID]
		if ms > 0 && (bestDelay <= 0 || ms < bestDelay) {
			bestDelay = ms
			best = p
			best.DelayMS = ms
		}
	}
	if bestDelay <= 0 {
		return xray.Profile{}, errors.New("no reachable Xray configuration found")
	}
	return best, nil
}

func (d *Daemon) failoverXrayProfile(failedID string) {
	if !d.xrayFailoverInProgress.CompareAndSwap(false, true) {
		return
	}
	defer d.xrayFailoverInProgress.Store(false)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	best, err := d.selectBestXrayProfile(ctx, nil, failedID)
	if err != nil {
		logx.Warn("Xray auto-failover found no working config err=%s", logx.Err(err))
		return
	}
	logx.Info("Xray auto-failover switching to profile=%q delay_ms=%d", best.Name, best.DelayMS)
	_ = d.setActiveXrayProfile(best.ID)
}

func (d *Daemon) watchXrayHealth() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-d.stop:
			return
		case <-ticker.C:
		}
		if d.isOff() {
			continue
		}
		cfg := d.config()
		if effectiveEngineMode(cfg) != config.EngineXray || !d.shouldAutoFailoverXray(cfg.Xray) {
			continue
		}
		eng := d.currentEngine()
		if eng == nil || eng.mode != config.EngineXray || eng.currentXray() == nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		_, err := xray.TestProfileLatency(ctx, eng.xrayProfile, cfg.Xray, 8*time.Second)
		cancel()
		if err == nil {
			continue
		}
		// Double-check once after brief settle before switching server.
		select {
		case <-d.stop:
			return
		case <-time.After(3 * time.Second):
		}
		if d.isOff() || d.currentEngine() != eng {
			continue
		}
		ctx2, cancel2 := context.WithTimeout(context.Background(), 8*time.Second)
		_, err2 := xray.TestProfileLatency(ctx2, eng.xrayProfile, d.config().Xray, 8*time.Second)
		cancel2()
		if err2 != nil {
			logx.Warn("Xray active config unresponsive; running auto-failover profile=%q err=%s", eng.xrayProfile.Name, logx.Err(err2))
			d.failoverXrayProfile(eng.xrayProfile.ID)
		}
	}
}

func autoPickRank(delayMS int64) int64 {
	if delayMS > 0 {
		return delayMS
	}
	if delayMS == 0 {
		return 1 << 40
	}
	return 1 << 50
}
