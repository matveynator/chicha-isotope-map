package autodeploy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Manager owns the polling goroutine. Channels keep the pipeline race-free and
// make the state transitions explicit for callers that consume Event updates.
type Manager struct {
	cfg         Config
	client      *http.Client
	downloadDir string
	lastGoodDir string
	logf        func(string, ...any)
}

// NewManager normalises configuration and prepares the filesystem. We prefer
// failing fast during setup so operators never wait until the first poll to see
// permission errors.
func NewManager(cfg Config) (*Manager, error) {
	if strings.TrimSpace(cfg.ManifestURL) == "" {
		return nil, errors.New("autodeploy: manifest URL required")
	}
	if strings.TrimSpace(cfg.LocalBinary) == "" {
		return nil, errors.New("autodeploy: local binary path required")
	}
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 30 * time.Minute
	}
	if strings.TrimSpace(cfg.Workspace) == "" {
		cfg.Workspace = filepath.Join(filepath.Dir(cfg.LocalBinary), "autodeploy-workspace")
	}
	absWorkspace, err := filepath.Abs(cfg.Workspace)
	if err != nil {
		return nil, fmt.Errorf("autodeploy: workspace: %w", err)
	}
	downloadDir := filepath.Join(absWorkspace, "downloads")
	if err := os.MkdirAll(downloadDir, 0o755); err != nil {
		return nil, fmt.Errorf("autodeploy: downloads dir: %w", err)
	}
	lastGoodDir := filepath.Join(absWorkspace, "last-good")
	if err := os.MkdirAll(lastGoodDir, 0o755); err != nil {
		return nil, fmt.Errorf("autodeploy: last-good dir: %w", err)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	m := &Manager{
		cfg:         cfg,
		client:      client,
		downloadDir: downloadDir,
		lastGoodDir: lastGoodDir,
		logf:        cfg.Logf,
	}
	return m, nil
}

// Run starts the polling goroutine and returns the event stream. The caller
// reads from the channel until the context is cancelled.
func (m *Manager) Run(ctx context.Context) <-chan Event {
	events := make(chan Event)
	go m.loop(ctx, events)
	return events
}

func (m *Manager) loop(ctx context.Context, events chan<- Event) {
	defer close(events)
	ticker := time.NewTicker(m.cfg.PollInterval)
	defer ticker.Stop()

	m.emitLocalChecksum(events)
	m.checkOnce(ctx, events)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.emitLocalChecksum(events)
			m.checkOnce(ctx, events)
		}
	}
}

func (m *Manager) emitLocalChecksum(events chan<- Event) {
	checksum, err := fileChecksum(m.cfg.LocalBinary)
	if err != nil {
		m.logf("autodeploy local checksum error: %v", err)
		events <- Event{Stage: "local-checksum", Err: err}
		return
	}
	m.logf("autodeploy checksum recorded: %s", checksum)
	events <- Event{Stage: "local-checksum", Message: "recorded current binary checksum", Checksum: checksum}
}

func (m *Manager) checkOnce(ctx context.Context, events chan<- Event) {
	manifest, err := m.fetchManifest(ctx)
	if err != nil {
		m.logf("autodeploy manifest error: %v", err)
		events <- Event{Stage: "fetch-manifest", Err: err}
		return
	}
	if manifest.Checksum == "" {
		m.logf("autodeploy manifest missing checksum")
		events <- Event{Stage: "fetch-manifest", Err: errors.New("autodeploy: manifest missing checksum")}
		return
	}
	localChecksum, err := fileChecksum(m.cfg.LocalBinary)
	if err != nil {
		m.logf("autodeploy checksum error: %v", err)
		events <- Event{Stage: "local-checksum", Err: err}
		return
	}
	if strings.EqualFold(localChecksum, manifest.Checksum) {
		m.logf("autodeploy up-to-date: %s", localChecksum)
		events <- Event{Stage: "up-to-date", Message: "current binary matches manifest", Checksum: localChecksum}
		return
	}
	m.logf("autodeploy detected new checksum: %s", manifest.Checksum)
	events <- Event{Stage: "detected", Message: "new checksum differs from local binary", Checksum: manifest.Checksum}
	if err := m.backupCurrent(localChecksum); err != nil {
		m.logf("autodeploy backup error: %v", err)
		events <- Event{Stage: "backup-current", Err: err}
		return
	}
	path, err := m.downloadArtifact(ctx, manifest)
	if err != nil {
		m.logf("autodeploy download error: %v", err)
		events <- Event{Stage: "download", Err: err}
		return
	}
	m.logf("autodeploy staged new binary at %s", path)
	events <- Event{Stage: "staged", Message: "downloaded new binary", Path: path, Checksum: manifest.Checksum}
}

func (m *Manager) fetchManifest(ctx context.Context) (manifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.cfg.ManifestURL, nil)
	if err != nil {
		return manifest{}, fmt.Errorf("autodeploy: manifest request: %w", err)
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return manifest{}, fmt.Errorf("autodeploy: manifest fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return manifest{}, fmt.Errorf("autodeploy: manifest status %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return manifest{}, fmt.Errorf("autodeploy: manifest read: %w", err)
	}

	var man manifest
	if err := json.Unmarshal(data, &man); err == nil {
		if strings.TrimSpace(man.Checksum) != "" || strings.TrimSpace(man.URL) != "" {
			return man, nil
		}
	}

	fields := strings.Fields(string(data))
	if len(fields) >= 2 {
		man.Checksum = fields[0]
		man.URL = fields[1]
		if len(fields) >= 3 {
			man.Version = fields[2]
		}
		return man, nil
	}

	return manifest{}, fmt.Errorf("autodeploy: manifest parse: expected JSON or 'checksum url'")
}

func (m *Manager) backupCurrent(checksum string) error {
	base := filepath.Base(m.cfg.LocalBinary)
	if base == "" {
		base = "app"
	}
	timestamp := time.Now().UTC().Format("20060102T150405Z")
	dest := filepath.Join(m.lastGoodDir, fmt.Sprintf("%s-%s", timestamp, base))
	if err := copyFile(m.cfg.LocalBinary, dest); err != nil {
		return fmt.Errorf("autodeploy: backup copy: %w", err)
	}
	if err := os.WriteFile(dest+".sha256", []byte(checksum), 0o644); err != nil {
		return fmt.Errorf("autodeploy: backup checksum: %w", err)
	}
	return nil
}

func (m *Manager) downloadArtifact(ctx context.Context, man manifest) (string, error) {
	url := strings.TrimSpace(man.URL)
	if url == "" {
		url = m.cfg.ManifestURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("autodeploy: artifact request: %w", err)
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("autodeploy: artifact fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("autodeploy: artifact status %s", resp.Status)
	}
	tempFile, err := os.CreateTemp(m.downloadDir, "artifact-*.bin")
	if err != nil {
		return "", fmt.Errorf("autodeploy: temp file: %w", err)
	}
	defer func() {
		tempFile.Close()
		os.Remove(tempFile.Name())
	}()

	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tempFile, hasher), resp.Body); err != nil {
		return "", fmt.Errorf("autodeploy: download copy: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		return "", fmt.Errorf("autodeploy: temp sync: %w", err)
	}

	got := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(got, man.Checksum) {
		return "", fmt.Errorf("autodeploy: checksum mismatch got %s want %s", got, man.Checksum)
	}

	finalName := m.finalName(man)
	finalPath := filepath.Join(m.downloadDir, finalName)
	if err := os.Rename(tempFile.Name(), finalPath); err != nil {
		return "", fmt.Errorf("autodeploy: rename: %w", err)
	}
	if err := os.WriteFile(finalPath+".sha256", []byte(got), 0o644); err != nil {
		return "", fmt.Errorf("autodeploy: checksum write: %w", err)
	}
	return finalPath, nil
}

func (m *Manager) finalName(man manifest) string {
	pieces := []string{"artifact"}
	if strings.TrimSpace(man.Version) != "" {
		pieces = append(pieces, sanitize(man.Version))
	}
	pieces = append(pieces, time.Now().UTC().Format("20060102T150405Z"))
	return strings.Join(pieces, "-") + ".bin"
}
