package autodeploy

import (
	"context"
	"time"
)

// =====================
// Configuration
// =====================
// Config keeps every knob the background autodeployer needs. We keep the surface
// compact so applications can reuse the manager across binaries by wiring only
// the release URL, workspace paths, and optional notification hooks.
type Config struct {
	ReleaseURL  string
	LocalBinary string
	Workspace   string

	PollInterval time.Duration

	Logf func(string, ...any)

	NotifyEmail string

	StatusLink  func(token string) string
	ApproveLink func(token string) string
	RejectLink  func(token string) string

	NotifyFunc func(ctx context.Context, to, subject, body string) error

	DBPath string

	AutoApply bool
}

// =====================
// Streaming events
// =====================
// Event reports activity back to the embedding application. We keep a single
// channel of events instead of callbacks so callers can fan out updates to logs
// or metrics without extra locking.
type Event struct {
	Stage    string
	Message  string
	Path     string
	Checksum string
	Version  string
	Token    string
	Err      error
}

// PendingInfo represents a staged release waiting for tester approval. We keep
// only serialisable data so HTTP handlers can expose the struct directly as
// JSON.
type PendingInfo struct {
	Token        string    `json:"token"`
	Version      string    `json:"version"`
	Checksum     string    `json:"checksum"`
	DownloadedAt time.Time `json:"downloaded_at"`
	BinaryPath   string    `json:"binary_path"`
	ClonePath    string    `json:"clone_path"`
	BackupDir    string    `json:"backup_dir"`
	StatusLink   string    `json:"status_link"`
	ApproveLink  string    `json:"approve_link"`
	RejectLink   string    `json:"reject_link"`
}

// PromotionResult summarises what happened during a promotion. Embedders can
// log it or expose it to dashboards without needing to read the filesystem.
type PromotionResult struct {
	Version    string
	Checksum   string
	BinaryPath string
	BackupPath string
	ClonePath  string
}
