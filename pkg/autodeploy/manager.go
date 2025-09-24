package autodeploy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// =====================
// Manager wiring
// =====================
// Manager orchestrates the polling loop and the session commands flowing in from
// HTTP handlers. We keep all state within a single goroutine so the design stays
// race-free without mutexes, following the Go proverbs.
type Manager struct {
	cfg            Config
	client         *http.Client
	downloadDir    string
	lastGoodDir    string
	backupDir      string
	cloneDir       string
	pendingDir     string
	statePath      string
	state          stateData
	logf           func(string, ...any)
	commands       chan command
	initialPending []pendingRecord
}

type command interface{}

type queryCmd struct {
	token string
	reply chan queryReply
}

type applyCmd struct {
	token string
	reply chan applyReply
}

type rejectCmd struct {
	token string
	reply chan error
}

type queryReply struct {
	info PendingInfo
	err  error
}

type applyReply struct {
	res PromotionResult
	err error
}

type releaseMetadata struct {
	ETag         string
	Version      string
	LastModified time.Time
}

type releaseCandidate struct {
	token        string
	version      string
	checksum     string
	etag         string
	binaryPath   string
	clonePath    string
	backupBinary string
	downloadedAt time.Time
}

// NewManager prepares the workspace directories and restores any pending
// sessions from disk. We fail fast on missing configuration so operators notice
// misconfiguration before the first poll.
func NewManager(cfg Config) (*Manager, error) {
	if strings.TrimSpace(cfg.ReleaseURL) == "" {
		return nil, errors.New("autodeploy: release URL required")
	}
	if strings.TrimSpace(cfg.LocalBinary) == "" {
		return nil, errors.New("autodeploy: local binary path required")
	}
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 15 * time.Minute
	}
	if strings.TrimSpace(cfg.Workspace) == "" {
		cfg.Workspace = filepath.Join(filepath.Dir(cfg.LocalBinary), "autodeploy-workspace")
	}
	if cfg.NotifyFunc == nil {
		cfg.NotifyFunc = func(context.Context, string, string, string) error { return nil }
	}
	if cfg.StatusLink == nil {
		cfg.StatusLink = func(token string) string { return token }
	}
	if cfg.ApproveLink == nil {
		cfg.ApproveLink = func(token string) string { return token }
	}
	if cfg.RejectLink == nil {
		cfg.RejectLink = func(token string) string { return token }
	}
	cfg.NotifyEmail = strings.TrimSpace(cfg.NotifyEmail)
	if !cfg.AutoApply && cfg.NotifyEmail == "" {
		cfg.AutoApply = true
	}

	workspace, err := ensureDir(cfg.Workspace)
	if err != nil {
		return nil, err
	}
	downloadDir, err := ensureDir(filepath.Join(workspace, "downloads"))
	if err != nil {
		return nil, err
	}
	lastGoodDir, err := ensureDir(filepath.Join(workspace, "last-good"))
	if err != nil {
		return nil, err
	}
	backupDir, err := ensureDir(filepath.Join(workspace, "db_backups"))
	if err != nil {
		return nil, err
	}
	cloneDir, err := ensureDir(filepath.Join(workspace, "db_clones"))
	if err != nil {
		return nil, err
	}
	pendingDir, err := ensureDir(filepath.Join(workspace, "sessions"))
	if err != nil {
		return nil, err
	}

	statePath := filepath.Join(workspace, "state.json")
	st, err := loadState(statePath)
	if err != nil {
		return nil, err
	}
	pending, err := loadPending(pendingDir)
	if err != nil {
		return nil, err
	}

	m := &Manager{
		cfg:            cfg,
		client:         &http.Client{Timeout: 45 * time.Second},
		downloadDir:    downloadDir,
		lastGoodDir:    lastGoodDir,
		backupDir:      backupDir,
		cloneDir:       cloneDir,
		pendingDir:     pendingDir,
		statePath:      statePath,
		state:          st,
		logf:           cfg.Logf,
		commands:       make(chan command),
		initialPending: pending,
	}
	return m, nil
}

// Run launches the polling goroutine and returns the event stream for callers to
// consume. The goroutine stops once the context is cancelled.
func (m *Manager) Run(ctx context.Context) <-chan Event {
	events := make(chan Event)
	go m.loop(ctx, events)
	return events
}

// Pending fetches a single staged release for HTTP handlers. The method uses a
// command sent to the manager goroutine so no additional synchronisation is
// required.
func (m *Manager) Pending(token string) (PendingInfo, error) {
	reply := make(chan queryReply, 1)
	select {
	case m.commands <- queryCmd{token: strings.TrimSpace(token), reply: reply}:
	case <-time.After(5 * time.Second):
		return PendingInfo{}, errors.New("autodeploy: manager busy")
	}
	select {
	case res := <-reply:
		return res.info, res.err
	case <-time.After(5 * time.Second):
		return PendingInfo{}, errors.New("autodeploy: pending response timeout")
	}
}

// Promote instructs the manager to switch production to the specified token.
// We perform heavy work inside the manager goroutine so the calling HTTP
// handler only needs to wait for the reply.
func (m *Manager) Promote(token string) (PromotionResult, error) {
	reply := make(chan applyReply, 1)
	select {
	case m.commands <- applyCmd{token: strings.TrimSpace(token), reply: reply}:
	case <-time.After(5 * time.Second):
		return PromotionResult{}, errors.New("autodeploy: manager busy")
	}
	select {
	case res := <-reply:
		return res.res, res.err
	case <-time.After(30 * time.Second):
		return PromotionResult{}, errors.New("autodeploy: promote response timeout")
	}
}

// Reject cancels a pending release and cleans up the staged files.
func (m *Manager) Reject(token string) error {
	reply := make(chan error, 1)
	select {
	case m.commands <- rejectCmd{token: strings.TrimSpace(token), reply: reply}:
	case <-time.After(5 * time.Second):
		return errors.New("autodeploy: manager busy")
	}
	select {
	case err := <-reply:
		return err
	case <-time.After(5 * time.Second):
		return errors.New("autodeploy: reject response timeout")
	}
}

func (m *Manager) loop(ctx context.Context, events chan<- Event) {
	defer close(events)

	pending := make(map[string]releaseCandidate)
	for _, rec := range m.initialPending {
		if strings.TrimSpace(rec.Token) == "" {
			continue
		}
		cand := releaseCandidate{
			token:        rec.Token,
			version:      rec.Version,
			checksum:     rec.Checksum,
			etag:         rec.ETag,
			binaryPath:   rec.BinaryPath,
			clonePath:    rec.ClonePath,
			backupBinary: rec.BackupBinary,
			downloadedAt: rec.DownloadedAt,
		}
		if _, err := os.Stat(cand.binaryPath); err != nil {
			continue
		}
		pending[cand.token] = cand
		events <- Event{Stage: "pending-restored", Message: "restored pending release", Version: cand.version, Checksum: cand.checksum, Token: cand.token}
	}

	ticker := time.NewTicker(m.cfg.PollInterval)
	defer ticker.Stop()

	m.checkOnce(ctx, events, pending)

	for {
		select {
		case <-ctx.Done():
			return
		case cmd := <-m.commands:
			m.handleCommand(ctx, cmd, events, pending)
		case <-ticker.C:
			m.checkOnce(ctx, events, pending)
		}
	}
}

func (m *Manager) handleCommand(ctx context.Context, cmd command, events chan<- Event, pending map[string]releaseCandidate) {
	switch c := cmd.(type) {
	case queryCmd:
		cand, ok := pending[c.token]
		if !ok {
			c.reply <- queryReply{err: fmt.Errorf("autodeploy: pending token %q not found", c.token)}
			return
		}
		c.reply <- queryReply{info: m.pendingInfo(cand)}
	case applyCmd:
		cand, ok := pending[c.token]
		if !ok {
			c.reply <- applyReply{err: fmt.Errorf("autodeploy: pending token %q not found", c.token)}
			return
		}
		res, err := m.promoteCandidate(ctx, events, cand)
		if err != nil {
			c.reply <- applyReply{err: err}
			return
		}
		delete(pending, c.token)
		_ = deletePending(m.pendingDir, c.token)
		c.reply <- applyReply{res: res}
		m.scheduleRestart()
	case rejectCmd:
		cand, ok := pending[c.token]
		if !ok {
			c.reply <- fmt.Errorf("autodeploy: pending token %q not found", c.token)
			return
		}
		_ = os.Remove(cand.binaryPath)
		if cand.clonePath != "" {
			_ = os.Remove(cand.clonePath)
		}
		delete(pending, c.token)
		_ = deletePending(m.pendingDir, c.token)
		events <- Event{Stage: "rejected", Message: "tester rejected pending release", Version: cand.version, Checksum: cand.checksum, Token: cand.token}
		c.reply <- nil
	}
}

func (m *Manager) checkOnce(ctx context.Context, events chan<- Event, pending map[string]releaseCandidate) {
	meta, err := m.fetchMetadata(ctx)
	if err != nil {
		events <- Event{Stage: "release-metadata", Err: err}
		return
	}
	if meta.ETag != "" && meta.ETag == m.state.LastETag {
		events <- Event{Stage: "up-to-date", Message: "latest release already applied", Version: meta.Version}
		return
	}
	for _, cand := range pending {
		if cand.etag == meta.ETag && meta.ETag != "" {
			events <- Event{Stage: "pending", Message: "release already staged", Version: cand.version, Checksum: cand.checksum, Token: cand.token}
			return
		}
	}

	events <- Event{Stage: "release-detected", Message: "new release detected", Version: meta.Version}
	cand, err := m.stageCandidate(ctx, meta, events)
	if err != nil {
		events <- Event{Stage: "stage", Err: err}
		return
	}

	if m.cfg.AutoApply {
		res, err := m.promoteCandidate(ctx, events, cand)
		if err != nil {
			events <- Event{Stage: "promote", Err: err}
			return
		}
		events <- Event{Stage: "promote", Message: "promotion completed", Version: res.Version, Checksum: res.Checksum}
		m.scheduleRestart()
		return
	}

	pending[cand.token] = cand
	_ = savePending(m.pendingDir, pendingRecord{
		Token:        cand.token,
		Version:      cand.version,
		Checksum:     cand.checksum,
		ETag:         cand.etag,
		BinaryPath:   cand.binaryPath,
		ClonePath:    cand.clonePath,
		BackupBinary: cand.backupBinary,
		DownloadedAt: cand.downloadedAt,
	})
	events <- Event{Stage: "pending", Message: "awaiting tester approval", Version: cand.version, Checksum: cand.checksum, Token: cand.token}
	m.notifyTester(ctx, cand)
}

func (m *Manager) stageCandidate(ctx context.Context, meta releaseMetadata, events chan<- Event) (releaseCandidate, error) {
	backupPath, _, err := m.backupCurrent()
	if err != nil {
		return releaseCandidate{}, err
	}
	path, checksum, err := m.downloadRelease(ctx, meta)
	if err != nil {
		return releaseCandidate{}, err
	}
	clonePath, err := m.cloneDatabase(meta.Version)
	if err != nil {
		return releaseCandidate{}, err
	}
	if err := m.runSmokeTest(ctx, path); err != nil {
		return releaseCandidate{}, err
	}
	token, err := randomToken(24)
	if err != nil {
		return releaseCandidate{}, err
	}
	cand := releaseCandidate{
		token:        token,
		version:      meta.Version,
		checksum:     checksum,
		etag:         meta.ETag,
		binaryPath:   path,
		clonePath:    clonePath,
		backupBinary: backupPath,
		downloadedAt: time.Now().UTC(),
	}
	events <- Event{Stage: "staged", Message: "binary downloaded and smoke-tested", Version: cand.version, Checksum: cand.checksum, Path: cand.binaryPath}
	if cand.clonePath != "" {
		events <- Event{Stage: "clone", Message: "database clone prepared", Version: cand.version, Path: cand.clonePath}
	}
	return cand, nil
}

func (m *Manager) fetchMetadata(ctx context.Context) (releaseMetadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, m.cfg.ReleaseURL, nil)
	if err != nil {
		return releaseMetadata{}, fmt.Errorf("autodeploy: metadata request: %w", err)
	}
	req.Header.Set("User-Agent", "chicha-autodeploy")
	resp, err := m.client.Do(req)
	if err != nil {
		return releaseMetadata{}, fmt.Errorf("autodeploy: metadata fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return releaseMetadata{}, fmt.Errorf("autodeploy: metadata status %s", resp.Status)
	}
	meta := releaseMetadata{}
	meta.ETag = strings.Trim(resp.Header.Get("ETag"), "\"")
	if meta.ETag == "" {
		meta.ETag = strings.TrimSpace(resp.Header.Get("X-GitHub-ETag"))
	}
	if last := resp.Header.Get("Last-Modified"); last != "" {
		if t, err := time.Parse(time.RFC1123, last); err == nil {
			meta.LastModified = t.UTC()
		}
	}
	meta.Version = meta.ETag
	if meta.Version == "" && !meta.LastModified.IsZero() {
		meta.Version = meta.LastModified.Format("20060102T150405Z")
	}
	if meta.Version == "" {
		meta.Version = time.Now().UTC().Format("20060102T150405Z")
	}
	return meta, nil
}

func (m *Manager) downloadRelease(ctx context.Context, meta releaseMetadata) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.cfg.ReleaseURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("autodeploy: download request: %w", err)
	}
	req.Header.Set("User-Agent", "chicha-autodeploy")
	resp, err := m.client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("autodeploy: download fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("autodeploy: download status %s", resp.Status)
	}

	temp, err := os.CreateTemp(m.downloadDir, "artifact-*.bin")
	if err != nil {
		return "", "", fmt.Errorf("autodeploy: download temp: %w", err)
	}
	defer func() {
		temp.Close()
		os.Remove(temp.Name())
	}()

	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(temp, hasher), resp.Body); err != nil {
		return "", "", fmt.Errorf("autodeploy: download copy: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return "", "", fmt.Errorf("autodeploy: download sync: %w", err)
	}
	if err := temp.Close(); err != nil {
		return "", "", fmt.Errorf("autodeploy: download close: %w", err)
	}

	checksum := hex.EncodeToString(hasher.Sum(nil))
	finalName := fmt.Sprintf("%s-%s.bin", sanitize(meta.Version), time.Now().UTC().Format("20060102T150405Z"))
	finalPath := filepath.Join(m.downloadDir, finalName)
	if err := os.Rename(temp.Name(), finalPath); err != nil {
		return "", "", fmt.Errorf("autodeploy: download rename: %w", err)
	}
	if err := os.Chmod(finalPath, 0o755); err != nil {
		return "", "", fmt.Errorf("autodeploy: download chmod: %w", err)
	}
	if err := os.WriteFile(finalPath+".sha256", []byte(checksum), 0o644); err != nil {
		return "", "", fmt.Errorf("autodeploy: download checksum write: %w", err)
	}
	return finalPath, checksum, nil
}

func (m *Manager) cloneDatabase(version string) (string, error) {
	path := strings.TrimSpace(m.cfg.DBPath)
	if path == "" {
		return "", nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("autodeploy: db stat: %w", err)
	}
	if info.IsDir() {
		return "", nil
	}
	name := fmt.Sprintf("%s-%s.clone", sanitize(filepath.Base(path)), time.Now().UTC().Format("20060102T150405Z"))
	clonePath := filepath.Join(m.cloneDir, name)
	if err := copyFile(path, clonePath); err != nil {
		return "", fmt.Errorf("autodeploy: db clone: %w", err)
	}
	return clonePath, nil
}

func (m *Manager) backupDatabase(version string) (string, error) {
	path := strings.TrimSpace(m.cfg.DBPath)
	if path == "" {
		return "", nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("autodeploy: db stat: %w", err)
	}
	if info.IsDir() {
		return "", nil
	}
	name := fmt.Sprintf("%s-%s.backup", sanitize(filepath.Base(path)), time.Now().UTC().Format("20060102T150405Z"))
	backupPath := filepath.Join(m.backupDir, name)
	if err := copyFile(path, backupPath); err != nil {
		return "", fmt.Errorf("autodeploy: db backup: %w", err)
	}
	if sum, err := fileChecksum(backupPath); err == nil {
		_ = os.WriteFile(backupPath+".sha256", []byte(sum), 0o644)
	}
	return backupPath, nil
}

func (m *Manager) backupCurrent() (string, string, error) {
	checksum, err := fileChecksum(m.cfg.LocalBinary)
	if err != nil {
		return "", "", fmt.Errorf("autodeploy: current checksum: %w", err)
	}
	base := filepath.Base(m.cfg.LocalBinary)
	if base == "" {
		base = "app"
	}
	name := fmt.Sprintf("%s-%s", time.Now().UTC().Format("20060102T150405Z"), base)
	dest := filepath.Join(m.lastGoodDir, name)
	if err := copyFile(m.cfg.LocalBinary, dest); err != nil {
		return "", "", fmt.Errorf("autodeploy: binary backup copy: %w", err)
	}
	if err := os.Chmod(dest, 0o755); err != nil {
		return "", "", fmt.Errorf("autodeploy: binary backup chmod: %w", err)
	}
	if err := os.WriteFile(dest+".sha256", []byte(checksum), 0o644); err != nil {
		return "", "", fmt.Errorf("autodeploy: binary backup checksum: %w", err)
	}
	return dest, checksum, nil
}

func (m *Manager) runSmokeTest(ctx context.Context, binaryPath string) error {
	smokeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(smokeCtx, binaryPath, "-version")
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(out))
		if trimmed != "" {
			return fmt.Errorf("autodeploy: smoke test failed: %w: %s", err, trimmed)
		}
		return fmt.Errorf("autodeploy: smoke test failed: %w", err)
	}
	return nil
}

func (m *Manager) promoteCandidate(ctx context.Context, events chan<- Event, cand releaseCandidate) (PromotionResult, error) {
	backupPath, err := m.backupDatabase(cand.version)
	if err != nil {
		return PromotionResult{}, err
	}
	tempPath := m.cfg.LocalBinary + ".autodeploy"
	if err := copyFile(cand.binaryPath, tempPath); err != nil {
		return PromotionResult{}, fmt.Errorf("autodeploy: promote copy: %w", err)
	}
	if err := os.Chmod(tempPath, 0o755); err != nil {
		_ = os.Remove(tempPath)
		return PromotionResult{}, fmt.Errorf("autodeploy: promote chmod: %w", err)
	}
	if err := os.Rename(tempPath, m.cfg.LocalBinary); err != nil {
		_ = os.Remove(tempPath)
		return PromotionResult{}, fmt.Errorf("autodeploy: promote rename: %w", err)
	}

	m.state.LastETag = cand.etag
	m.state.LastChecksum = cand.checksum
	m.state.LastApplied = time.Now().UTC()
	if err := saveState(m.statePath, m.state); err != nil {
		return PromotionResult{}, fmt.Errorf("autodeploy: state save: %w", err)
	}

	result := PromotionResult{
		Version:    cand.version,
		Checksum:   cand.checksum,
		BinaryPath: m.cfg.LocalBinary,
		BackupPath: backupPath,
		ClonePath:  cand.clonePath,
	}
	if cand.clonePath != "" {
		events <- Event{Stage: "clone-ready", Message: "clone preserved for rollback", Path: cand.clonePath, Version: cand.version}
	}
	if backupPath != "" {
		events <- Event{Stage: "backup", Message: "database backup stored", Path: backupPath, Version: cand.version}
	}
	return result, nil
}

func (m *Manager) scheduleRestart() {
	args := make([]string, len(os.Args))
	copy(args, os.Args)
	if len(args) == 0 {
		args = []string{m.cfg.LocalBinary}
	} else {
		args[0] = m.cfg.LocalBinary
	}
	env := os.Environ()
	go func() {
		time.Sleep(3 * time.Second)
		if err := restartBinary(m.cfg.LocalBinary, args, env); err != nil {
			m.logf("autodeploy restart error: %v", err)
			return
		}
		os.Exit(0)
	}()
}

func (m *Manager) pendingInfo(cand releaseCandidate) PendingInfo {
	return PendingInfo{
		Token:        cand.token,
		Version:      cand.version,
		Checksum:     cand.checksum,
		DownloadedAt: cand.downloadedAt,
		BinaryPath:   cand.binaryPath,
		ClonePath:    cand.clonePath,
		BackupDir:    m.backupDir,
		StatusLink:   m.cfg.StatusLink(cand.token),
		ApproveLink:  m.cfg.ApproveLink(cand.token),
		RejectLink:   m.cfg.RejectLink(cand.token),
	}
}

func (m *Manager) notifyTester(ctx context.Context, cand releaseCandidate) {
	if m.cfg.NotifyEmail == "" {
		return
	}
	status := m.cfg.StatusLink(cand.token)
	approve := m.cfg.ApproveLink(cand.token)
	reject := m.cfg.RejectLink(cand.token)
	subject := fmt.Sprintf("chicha-isotope-map pending release %s", cand.version)
	body := fmt.Sprintf("New version is ready for verification.\nVersion: %s\nChecksum: %s\nClone: %s\nStatus: %s\nPromote: %s\nReject: %s\n", cand.version, cand.checksum, cand.clonePath, status, approve, reject)
	if err := m.cfg.NotifyFunc(ctx, m.cfg.NotifyEmail, subject, body); err != nil {
		m.logf("autodeploy notify error: %v", err)
	}
}
