package xray

import "time"

// FileName is the Xray profiles and subscriptions state file in the data dir.
const FileName = "xray-profiles.json"

// Profile describes one Xray server / outbound configuration.
type Profile struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Protocol       string    `json:"protocol"`
	Address        string    `json:"address"`
	Port           int       `json:"port"`
	RawURI         string    `json:"raw_uri,omitempty"`
	RawJSON        string    `json:"raw_json,omitempty"`
	SubscriptionID string    `json:"subscription_id,omitempty"`

	// Auth / protocol settings.
	UUID       string `json:"uuid,omitempty"`
	Password   string `json:"password,omitempty"`
	Username   string `json:"username,omitempty"`
	Method     string `json:"method,omitempty"`
	AlterID    int    `json:"alter_id,omitempty"`
	Encryption string `json:"encryption,omitempty"`
	Flow       string `json:"flow,omitempty"`

	// Stream / transport settings.
	Network     string `json:"network,omitempty"`
	HeaderType  string `json:"header_type,omitempty"`
	Host        string `json:"host,omitempty"`
	Path        string `json:"path,omitempty"`
	ServiceName string `json:"service_name,omitempty"`
	Mode        string `json:"mode,omitempty"`
	Extra       string `json:"extra,omitempty"`

	// Security / TLS / Reality settings.
	Security      string   `json:"security,omitempty"`
	SNI           string   `json:"sni,omitempty"`
	ALPN          []string `json:"alpn,omitempty"`
	Fingerprint   string   `json:"fingerprint,omitempty"`
	AllowInsecure bool     `json:"allow_insecure,omitempty"`
	PublicKey     string   `json:"public_key,omitempty"`
	ShortID       string   `json:"short_id,omitempty"`
	SpiderX       string   `json:"spider_x,omitempty"`
	PQV           string   `json:"pqv,omitempty"`

	// Last test measurement. DelayMS < 0 means last test failed, 0 means untested.
	DelayMS    int64     `json:"delay_ms,omitempty"`
	DelayError string    `json:"delay_error,omitempty"`
	TestedAt   time.Time `json:"tested_at,omitzero"`
	AddedAt    time.Time `json:"added_at"`
}

// Subscription describes a remote Xray subscription source.
type Subscription struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	URL          string    `json:"url"`
	Enabled      bool      `json:"enabled"`
	AutoUpdate   bool      `json:"auto_update"`
	UpdatedAt    time.Time `json:"updated_at,omitzero"`
	Upload       int64     `json:"upload,omitempty"`
	Download     int64     `json:"download,omitempty"`
	Total        int64     `json:"total,omitempty"`
	Expire       int64     `json:"expire,omitempty"`
	LastError    string    `json:"last_error,omitempty"`
	ProfileCount int       `json:"profile_count,omitempty"`
}

// StateData is stored on disk in xray-profiles.json.
type StateData struct {
	Profiles      []Profile      `json:"profiles"`
	Subscriptions []Subscription `json:"subscriptions"`
}

// DelayResult is returned when testing one or more Xray profiles.
type DelayResult struct {
	ID      string `json:"id"`
	DelayMS int64  `json:"delay_ms"`
	Error   string `json:"error,omitempty"`
}
