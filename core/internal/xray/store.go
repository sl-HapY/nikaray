package xray

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/armanlife/arman-vpn-client/core/internal/securefile"
)

// Store persists Xray profiles and subscriptions in xray-profiles.json.
type Store struct {
	path string
	mu   sync.Mutex
	data StateData
}

// OpenStore loads or initializes an Xray Store at path.
func OpenStore(path string) (*Store, error) {
	s := &Store{path: path}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(b))) > 0 {
		if err := json.Unmarshal(b, &s.data); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
	}
	return s, nil
}

func (s *Store) saveLocked() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return securefile.WritePrivate(s.path, b)
}

// Snapshot returns a copy of profiles and subscriptions (with ProfileCount computed).
func (s *Store) Snapshot() StateData {
	s.mu.Lock()
	defer s.mu.Unlock()
	counts := map[string]int{}
	profiles := append([]Profile(nil), s.data.Profiles...)
	for i := range profiles {
		if len(profiles[i].ALPN) > 0 {
			profiles[i].ALPN = append([]string(nil), profiles[i].ALPN...)
		}
		counts[profiles[i].SubscriptionID]++
	}
	subs := append([]Subscription(nil), s.data.Subscriptions...)
	for i := range subs {
		subs[i].ProfileCount = counts[subs[i].ID]
	}
	return StateData{Profiles: profiles, Subscriptions: subs}
}

// ProfileByID returns the matching profile if found.
func (s *Store) ProfileByID(id string) (Profile, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.data.Profiles {
		if p.ID == id {
			return p, true
		}
	}
	return Profile{}, false
}

// FirstProfile returns the first stored profile if any.
func (s *Store) FirstProfile() (Profile, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.data.Profiles) == 0 {
		return Profile{}, false
	}
	return s.data.Profiles[0], true
}

// AddProfiles appends profiles (assigning IDs if missing) and saves.
func (s *Store) AddProfiles(items []Profile) ([]Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	for i := range items {
		if items[i].ID == "" {
			items[i].ID = newID()
		}
		if items[i].AddedAt.IsZero() {
			items[i].AddedAt = now
		}
		if items[i].RawURI == "" && items[i].Protocol != "custom" {
			items[i].RawURI = items[i].ToURI()
		}
	}
	s.data.Profiles = append(s.data.Profiles, items...)
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	return items, nil
}

// UpsertProfile updates an existing profile by ID or appends a new one.
func (s *Store) UpsertProfile(p Profile) (Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.ID == "" {
		p.ID = newID()
	}
	if p.AddedAt.IsZero() {
		p.AddedAt = time.Now().UTC()
	}
	if p.Protocol != "custom" {
		p.RawURI = p.ToURI()
	}
	replaced := false
	for i := range s.data.Profiles {
		if s.data.Profiles[i].ID == p.ID {
			p.DelayMS = s.data.Profiles[i].DelayMS
			p.DelayError = s.data.Profiles[i].DelayError
			p.TestedAt = s.data.Profiles[i].TestedAt
			s.data.Profiles[i] = p
			replaced = true
			break
		}
	}
	if !replaced {
		s.data.Profiles = append(s.data.Profiles, p)
	}
	if err := s.saveLocked(); err != nil {
		return Profile{}, err
	}
	return p, nil
}

// DeleteProfile deletes one profile by ID.
func (s *Store) DeleteProfile(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := make([]Profile, 0, len(s.data.Profiles))
	found := false
	for _, p := range s.data.Profiles {
		if p.ID == id {
			found = true
			continue
		}
		kept = append(kept, p)
	}
	if !found {
		return fmt.Errorf("profile %q not found", id)
	}
	s.data.Profiles = kept
	return s.saveLocked()
}

// ApplyDelayResult updates a single profile's measured latency in memory without sorting.
func (s *Store) ApplyDelayResult(r DelayResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	for i := range s.data.Profiles {
		if s.data.Profiles[i].ID == r.ID {
			s.data.Profiles[i].DelayMS = r.DelayMS
			s.data.Profiles[i].DelayError = r.Error
			s.data.Profiles[i].TestedAt = now
			return
		}
	}
}

// ApplyDelayResults saves measured latencies to matching profiles.
func (s *Store) ApplyDelayResults(results []DelayResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	byID := make(map[string]DelayResult, len(results))
	for _, r := range results {
		byID[r.ID] = r
	}
	now := time.Now().UTC()
	for i := range s.data.Profiles {
		if r, ok := byID[s.data.Profiles[i].ID]; ok {
			s.data.Profiles[i].DelayMS = r.DelayMS
			s.data.Profiles[i].DelayError = r.Error
			s.data.Profiles[i].TestedAt = now
		}
	}
	return s.saveLocked()
}

// SortByDelay sorts profiles so reachable profiles appear fastest-first, untested next,
// and failed profiles last.
func (s *Store) SortByDelay() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sort.SliceStable(s.data.Profiles, func(i, j int) bool {
		return delayRank(s.data.Profiles[i].DelayMS) < delayRank(s.data.Profiles[j].DelayMS)
	})
	return s.saveLocked()
}

func delayRank(d int64) int64 {
	if d > 0 {
		return d
	}
	if d == 0 {
		return 1 << 50
	}
	return 1 << 60
}

// Cleanup removes failed profiles (if removeFailed) and/or duplicates (if removeDuplicates).
func (s *Store) Cleanup(removeFailed, removeDuplicates bool) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := map[string]bool{}
	kept := make([]Profile, 0, len(s.data.Profiles))
	removed := 0
	for _, p := range s.data.Profiles {
		if removeFailed && p.DelayMS < 0 {
			removed++
			continue
		}
		if removeDuplicates {
			key := strings.ToLower(fmt.Sprintf("%s|%s|%d|%s|%s|%s|%s|%s",
				p.Protocol, p.Address, p.Port, p.UUID, p.Password, p.Network, p.SNI, p.Path))
			if seen[key] {
				removed++
				continue
			}
			seen[key] = true
		}
		kept = append(kept, p)
	}
	s.data.Profiles = kept
	if err := s.saveLocked(); err != nil {
		return 0, err
	}
	return removed, nil
}

// AddSubscription stores a new subscription and returns it.
func (s *Store) AddSubscription(sub Subscription) (Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sub.ID == "" {
		sub.ID = newID()
	}
	if sub.Name == "" {
		sub.Name = "Subscription"
	}
	s.data.Subscriptions = append(s.data.Subscriptions, sub)
	if err := s.saveLocked(); err != nil {
		return Subscription{}, err
	}
	return sub, nil
}

// SubscriptionByID returns a subscription by ID.
func (s *Store) SubscriptionByID(id string) (Subscription, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sub := range s.data.Subscriptions {
		if sub.ID == id {
			return sub, true
		}
	}
	return Subscription{}, false
}

// UpdateSubscription updates subscription metadata.
func (s *Store) UpdateSubscription(sub Subscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Subscriptions {
		if s.data.Subscriptions[i].ID == sub.ID {
			s.data.Subscriptions[i] = sub
			return s.saveLocked()
		}
	}
	return fmt.Errorf("subscription %q not found", sub.ID)
}

// ReplaceSubscriptionProfiles replaces all profiles belonging to subID with incoming,
// preserving activeProfileID if an identical server remains in the updated subscription.
func (s *Store) ReplaceSubscriptionProfiles(sub Subscription, incoming []Profile, activeProfileID string) (newActiveID string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var prevActiveKey string
	for _, p := range s.data.Profiles {
		if p.ID == activeProfileID && p.SubscriptionID == sub.ID {
			prevActiveKey = strings.ToLower(fmt.Sprintf("%s|%s|%d|%s|%s", p.Protocol, p.Address, p.Port, p.UUID, p.Name))
			break
		}
	}

	kept := make([]Profile, 0, len(s.data.Profiles))
	for _, p := range s.data.Profiles {
		if p.SubscriptionID != sub.ID {
			kept = append(kept, p)
		}
	}

	now := time.Now().UTC()
	newActiveID = activeProfileID
	for i := range incoming {
		if incoming[i].ID == "" {
			incoming[i].ID = newID()
		}
		incoming[i].SubscriptionID = sub.ID
		if incoming[i].AddedAt.IsZero() {
			incoming[i].AddedAt = now
		}
		if prevActiveKey != "" {
			key := strings.ToLower(fmt.Sprintf("%s|%s|%d|%s|%s", incoming[i].Protocol, incoming[i].Address, incoming[i].Port, incoming[i].UUID, incoming[i].Name))
			if key == prevActiveKey {
				newActiveID = incoming[i].ID
			}
		}
	}
	s.data.Profiles = append(kept, incoming...)
	for i := range s.data.Subscriptions {
		if s.data.Subscriptions[i].ID == sub.ID {
			s.data.Subscriptions[i] = sub
			break
		}
	}
	if err := s.saveLocked(); err != nil {
		return activeProfileID, err
	}
	return newActiveID, nil
}

// DeleteSubscription removes a subscription and optionally its profiles.
func (s *Store) DeleteSubscription(id string, keepProfiles bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	subs := make([]Subscription, 0, len(s.data.Subscriptions))
	found := false
	for _, sub := range s.data.Subscriptions {
		if sub.ID == id {
			found = true
			continue
		}
		subs = append(subs, sub)
	}
	if !found {
		return fmt.Errorf("subscription %q not found", id)
	}
	s.data.Subscriptions = subs
	kept := make([]Profile, 0, len(s.data.Profiles))
	for _, p := range s.data.Profiles {
		if p.SubscriptionID == id {
			if keepProfiles {
				p.SubscriptionID = ""
				kept = append(kept, p)
			}
			continue
		}
		kept = append(kept, p)
	}
	s.data.Profiles = kept
	return s.saveLocked()
}
