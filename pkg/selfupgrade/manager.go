package selfupgrade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var (
	// ErrUpdateInProgress is returned when a manual trigger or decision is
	// attempted while another release pipeline is still running.
	ErrUpdateInProgress = errors.New("selfupgrade: update already running")
	// ErrNoActiveUpdate is returned when testers respond without an active
	// candidate.
	ErrNoActiveUpdate = errors.New("selfupgrade: no candidate awaiting decision")
)

// Manager wires together polling, deployment stages, and tester feedback using
// a single goroutine. Channels keep the state machine race-free without relying
// on mutexes, following Go's "Don't communicate by sharing memory" proverb.
type Manager struct {
	cfg      Config
	fetcher  ReleaseFetcher
	notifier Notifier
	runner   Runner
	database DatabaseController
	client   *http.Client
	logf     func(string, ...any)

	pollCh         chan struct{}
	manualCh       chan manualRequest
	decisionCh     chan decisionRequest
	statusCh       chan statusRequest
	done           chan struct{}
	failedVersions map[string]struct{}
}

type manualRequest struct {
	reply chan error
}

type decisionRequest struct {
	decision Decision
	reply    chan error
}

type statusRequest struct {
	reply chan Status
}

type pipelineEvent struct {
	stage     string
	message   string
	candidate string
}

type pipelineResult struct {
	promoted  bool
	candidate string
	err       error
	log       string
}

type updatePipeline struct {
	ctx              context.Context
	cancel           context.CancelFunc
	currentVersion   string
	candidateVersion string
	decisionCh       chan Decision
	updates          chan pipelineEvent
	result           chan pipelineResult
	requiresDBBackup bool
	logBuffer        *strings.Builder
	failedVersions   map[string]struct{}
}

// NewManager validates configuration and prepares collaborators. Directories
// are created early so permission issues surface immediately instead of halfway
// through a rollout.
func NewManager(cfg Config) (*Manager, error) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return nil, errors.New("selfupgrade: updater supports only linux/amd64")
	}

	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 30 * time.Minute
	}
	if strings.TrimSpace(cfg.HealthCheckPath) == "" {
		cfg.HealthCheckPath = "/healthz"
	}
	if strings.TrimSpace(cfg.DownloadURL) == "" {
		return nil, errors.New("selfupgrade: download URL must be configured")
	}

	if strings.TrimSpace(cfg.BinaryPath) == "" {
		exe, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("selfupgrade: cannot determine binary path: %w", err)
		}
		cfg.BinaryPath = exe
	}

	if strings.TrimSpace(cfg.DeployDir) == "" {
		cfg.DeployDir = filepath.Join(filepath.Dir(cfg.BinaryPath), "selfupgrade-cache")
	}
	if err := ensureDir(cfg.DeployDir); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.DBBackupsDir) == "" {
		cfg.DBBackupsDir = filepath.Join(cfg.DeployDir, "db_backups")
	}
	if err := ensureDir(cfg.DBBackupsDir); err != nil {
		return nil, err
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	fetcher := cfg.ReleaseFetcher
	if fetcher == nil {
		fetcher = StaticFetcher{URL: cfg.DownloadURL, Client: client}
	}
	notifier := cfg.Notifier
	if notifier == nil {
		notifier = LogNotifier{Logf: cfg.Logf}
	}
	runner := cfg.Runner
	if runner == nil {
		runner = DryRunner{Logf: cfg.Logf}
	}

	m := &Manager{
		cfg:            cfg,
		fetcher:        fetcher,
		notifier:       notifier,
		runner:         runner,
		database:       cfg.Database,
		client:         client,
		logf:           cfg.Logf,
		pollCh:         make(chan struct{}, 1),
		manualCh:       make(chan manualRequest),
		decisionCh:     make(chan decisionRequest),
		statusCh:       make(chan statusRequest),
		done:           make(chan struct{}),
		failedVersions: make(map[string]struct{}),
	}
	return m, nil
}

// Start spins the manager goroutines. The provided context controls the
// lifetime; cancelling it stops both the poller and the state machine.
func (m *Manager) Start(ctx context.Context) error {
	if ctx == nil {
		return errors.New("selfupgrade: nil context")
	}

	go m.poller(ctx)
	go m.run(ctx)
	return nil
}

// Wait blocks until the manager finishes. It is safe to call even when Start
// has not been invoked because the channel closes only once.
func (m *Manager) Wait() {
	<-m.done
}

// Trigger asks the manager to start a release check immediately. The request is
// rejected when another update is in progress.
func (m *Manager) Trigger(ctx context.Context) error {
	req := manualRequest{reply: make(chan error, 1)}
	select {
	case m.manualCh <- req:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-req.reply:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Decision injects the tester verdict. The manager rejects stale decisions so
// testers do not accidentally approve the wrong candidate.
func (m *Manager) Decision(ctx context.Context, d Decision) error {
	req := decisionRequest{decision: d, reply: make(chan error, 1)}
	select {
	case m.decisionCh <- req:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-req.reply:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Status fetches the current deployment snapshot.
func (m *Manager) Status(ctx context.Context) (Status, error) {
	req := statusRequest{reply: make(chan Status, 1)}
	select {
	case m.statusCh <- req:
	case <-ctx.Done():
		return Status{}, ctx.Err()
	}
	select {
	case st := <-req.reply:
		return st, nil
	case <-ctx.Done():
		return Status{}, ctx.Err()
	}
}

func (m *Manager) poller(ctx context.Context) {
	ticker := time.NewTicker(m.cfg.PollInterval)
	defer ticker.Stop()

	m.requestPoll()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.requestPoll()
		}
	}
}

func (m *Manager) requestPoll() {
	select {
	case m.pollCh <- struct{}{}:
	default:
	}
}

func (m *Manager) run(ctx context.Context) {
	defer close(m.done)

	status := Status{
		CurrentVersion: strings.TrimSpace(m.cfg.CurrentVersion),
		ActiveStage:    "idle",
	}
	var (
		pipeline *updatePipeline
		updates  <-chan pipelineEvent
		results  <-chan pipelineResult
	)

	for {
		select {
		case <-ctx.Done():
			if pipeline != nil {
				pipeline.cancel()
				<-results
			}
			return
		case <-m.pollCh:
			if pipeline != nil {
				continue
			}
			status.LastCheck = time.Now()
			pipeline = m.startPipeline(ctx, status.CurrentVersion)
			updates = pipeline.updates
			results = pipeline.result
			status.ActiveStage = "checking-release"
		case req := <-m.manualCh:
			if pipeline != nil {
				req.reply <- ErrUpdateInProgress
				continue
			}
			status.LastCheck = time.Now()
			pipeline = m.startPipeline(ctx, status.CurrentVersion)
			updates = pipeline.updates
			results = pipeline.result
			status.ActiveStage = "manual-check"
			req.reply <- nil
		case req := <-m.decisionCh:
			if pipeline == nil {
				req.reply <- ErrNoActiveUpdate
				continue
			}
			if pipeline.candidateVersion != "" && strings.TrimSpace(req.decision.Version) != "" && sanitize(req.decision.Version) != sanitize(pipeline.candidateVersion) {
				req.reply <- fmt.Errorf("selfupgrade: decision version %s does not match active %s", req.decision.Version, pipeline.candidateVersion)
				continue
			}
			select {
			case pipeline.decisionCh <- req.decision:
				req.reply <- nil
			case <-pipeline.ctx.Done():
				req.reply <- pipeline.ctx.Err()
			}
		case req := <-m.statusCh:
			st := status
			if pipeline != nil && pipeline.candidateVersion != "" {
				st.CandidateVersion = pipeline.candidateVersion
			}
			req.reply <- st
		case evt := <-updates:
			if evt.stage != "" {
				status.ActiveStage = evt.stage
			}
			if evt.candidate != "" {
				status.CandidateVersion = evt.candidate
				if pipeline != nil {
					pipeline.candidateVersion = evt.candidate
				}
			}
			if strings.TrimSpace(evt.message) != "" {
				m.logf("%s", evt.message)
			}
		case res := <-results:
			if res.err != nil {
				status.LastError = res.err.Error()
				if strings.TrimSpace(res.candidate) != "" {
					m.failedVersions[sanitize(res.candidate)] = struct{}{}
					m.recordFailureLog(res.candidate, res.log, res.err)
				}
			} else {
				status.LastError = ""
			}
			if res.promoted && res.candidate != "" {
				status.CurrentVersion = res.candidate
				status.LastSuccess = time.Now()
			}
			status.CandidateVersion = ""
			status.ActiveStage = "idle"
			pipeline = nil
			updates = nil
			results = nil
		}
	}
}

func (m *Manager) startPipeline(ctx context.Context, currentVersion string) *updatePipeline {
	pctx, cancel := context.WithCancel(ctx)
	failedCopy := make(map[string]struct{}, len(m.failedVersions))
	for k := range m.failedVersions {
		failedCopy[k] = struct{}{}
	}
	p := &updatePipeline{
		ctx:            pctx,
		cancel:         cancel,
		currentVersion: currentVersion,
		decisionCh:     make(chan Decision),
		updates:        make(chan pipelineEvent, 8),
		result:         make(chan pipelineResult, 1),
		logBuffer:      &strings.Builder{},
		failedVersions: failedCopy,
	}
	go m.executePipeline(p)
	return p
}

type releaseStageResult struct {
	release     Release
	hasUpdate   bool
	needsBackup bool
	err         error
}

type downloadStageResult struct {
	candidatePath     string
	candidateChecksum string
	lastGoodPath      string
	lastGoodChecksum  string
	err               error
}

type databaseStageResult struct {
	backupPath string
	clone      CloneInfo
	err        error
}

type canaryStageResult struct {
	instance CanaryInstance
	err      error
}

func (m *Manager) executePipeline(p *updatePipeline) {
	defer close(p.result)
	defer close(p.updates)

	ctx := p.ctx
	send := func(stage, message, candidate string) {
		if p.logBuffer != nil && strings.TrimSpace(message) != "" {
			timestamp := time.Now().UTC().Format(time.RFC3339)
			p.logBuffer.WriteString(timestamp)
			p.logBuffer.WriteString(" ")
			if strings.TrimSpace(stage) != "" {
				p.logBuffer.WriteString(stage)
				p.logBuffer.WriteString(": ")
			}
			p.logBuffer.WriteString(message)
			if strings.TrimSpace(candidate) != "" {
				p.logBuffer.WriteString(" (candidate=" + candidate + ")")
			}
			p.logBuffer.WriteString("\n")
		}
		evt := pipelineEvent{stage: stage, message: message, candidate: candidate}
		select {
		case p.updates <- evt:
		case <-ctx.Done():
		}
	}

	finish := func(res pipelineResult) {
		if p.logBuffer != nil {
			res.log = p.logBuffer.String()
		}
		p.result <- res
	}

	recordError := func(err error) {
		if err != nil && p.logBuffer != nil {
			p.logBuffer.WriteString(fmt.Sprintf("ERROR: %v\n", err))
		}
	}

	send("checking-release", "[selfupgrade] checking remote binary availability", "")
	releaseRes := <-m.stageRelease(ctx, p.currentVersion)
	if releaseRes.err != nil {
		recordError(releaseRes.err)
		finish(pipelineResult{err: releaseRes.err})
		return
	}
	if !releaseRes.hasUpdate {
		finish(pipelineResult{promoted: false, candidate: p.currentVersion, err: nil})
		return
	}
	candidateVersion := strings.TrimSpace(releaseRes.release.Tag)
	if candidateVersion == "" {
		err := errors.New("selfupgrade: release tag empty")
		recordError(err)
		finish(pipelineResult{err: err})
		return
	}
	if _, blocked := p.failedVersions[sanitize(candidateVersion)]; blocked {
		send("skipping", fmt.Sprintf("[selfupgrade] version %s previously failed, waiting for a new release", candidateVersion), candidateVersion)
		finish(pipelineResult{promoted: false, candidate: p.currentVersion, err: nil})
		return
	}
	p.requiresDBBackup = releaseRes.needsBackup
	send("downloading", fmt.Sprintf("[selfupgrade] downloading release %s", candidateVersion), candidateVersion)
	downloadRes := <-m.stageDownload(ctx, releaseRes.release, p.currentVersion)
	if downloadRes.err != nil {
		recordError(downloadRes.err)
		finish(pipelineResult{candidate: candidateVersion, err: downloadRes.err})
		return
	}
	candidate := CandidateBuild{
		Version:        candidateVersion,
		BinaryPath:     downloadRes.candidatePath,
		ChecksumSHA256: downloadRes.candidateChecksum,
	}
	var dbRes databaseStageResult
	if p.requiresDBBackup {
		send("backing-up", "[selfupgrade] creating database backup and clone", candidateVersion)
		dbRes = <-m.stageDatabase(ctx, candidateVersion, true)
		if dbRes.err != nil {
			recordError(dbRes.err)
			finish(pipelineResult{candidate: candidateVersion, err: dbRes.err})
			return
		}
	} else {
		send("backing-up", "[selfupgrade] skipping database backup (no migrations detected)", candidateVersion)
	}

	var (
		cloneActive          bool
		activeClone          CloneInfo
		backupPath           = dbRes.backupPath
		lastGoodPath         = downloadRes.lastGoodPath
		promotedSuccessfully bool
	)
	if dbRes.clone.Path != "" {
		cloneActive = true
		activeClone = dbRes.clone
	}

	defer func() {
		if !cloneActive || m.database == nil {
			return
		}
		drop := func() {
			ctxDrop, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = m.database.DropClone(ctxDrop, activeClone)
		}
		if m.cfg.CloneRetention <= 0 || !promotedSuccessfully {
			drop()
			return
		}
		go func(retention time.Duration, clone CloneInfo) {
			timer := time.NewTimer(retention)
			defer timer.Stop()
			select {
			case <-timer.C:
				ctxDrop, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = m.database.DropClone(ctxDrop, clone)
			case <-ctx.Done():
			}
		}(m.cfg.CloneRetention, activeClone)
	}()

	send("canary", "[selfupgrade] launching canary instance", candidateVersion)
	canaryRes := <-m.stageCanary(ctx, candidate, activeClone)
	if canaryRes.err != nil {
		recordError(canaryRes.err)
		finish(pipelineResult{candidate: candidateVersion, err: canaryRes.err})
		return
	}
	instance := canaryRes.instance
	defer func() {
		if instance.ID != "" {
			ctxStop, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = m.runner.StopCanary(ctxStop, instance)
		}
	}()

	if err := m.verifyCanary(ctx); err != nil {
		recordError(err)
		finish(pipelineResult{candidate: candidateVersion, err: err})
		return
	}

	notifyMsg := fmt.Sprintf("Canary %s for %s ready on port %d. Visit /selfupgrade/approve?version=%s or /selfupgrade/reject?version=%s", instance.ID, candidateVersion, m.cfg.CanaryPort, urlQueryEscape(candidateVersion), urlQueryEscape(candidateVersion))
	_ = m.notifier.Notify(ctx, "Canary ready", notifyMsg)
	send("waiting-decision", "[selfupgrade] waiting for tester decision", candidateVersion)

	decision, err := m.awaitDecision(ctx, p.decisionCh)
	if err != nil {
		recordError(err)
		finish(pipelineResult{candidate: candidateVersion, err: err})
		return
	}
	if !decision.Approve {
		reason := strings.TrimSpace(decision.Reason)
		if reason == "" {
			reason = "tester rejected candidate"
		}
		_ = m.notifier.Notify(ctx, "Canary rejected", fmt.Sprintf("Candidate %s rejected: %s", candidateVersion, reason))
		err := errors.New(reason)
		recordError(err)
		finish(pipelineResult{candidate: candidateVersion, promoted: false, err: err})
		return
	}

	send("promoting", "[selfupgrade] promoting candidate", candidateVersion)
	if err := m.applyBinary(ctx, candidate.BinaryPath); err != nil {
		recordError(err)
		finish(pipelineResult{candidate: candidateVersion, err: err})
		return
	}
	if err := m.runner.RestartService(ctx); err != nil {
		rollbackErr := m.rollback(ctx, lastGoodPath, backupPath)
		if rollbackErr != nil {
			combined := fmt.Errorf("restart failed: %v; rollback failed: %w", err, rollbackErr)
			recordError(combined)
			finish(pipelineResult{candidate: candidateVersion, err: combined})
		} else {
			wrapped := fmt.Errorf("restart failed: %w", err)
			recordError(wrapped)
			finish(pipelineResult{candidate: candidateVersion, err: wrapped})
		}
		return
	}
	if err := m.runner.PostPromotionCheck(ctx); err != nil {
		rollbackErr := m.rollback(ctx, lastGoodPath, backupPath)
		if rollbackErr != nil {
			combined := fmt.Errorf("post-promotion check failed: %v; rollback failed: %w", err, rollbackErr)
			recordError(combined)
			finish(pipelineResult{candidate: candidateVersion, err: combined})
		} else {
			wrapped := fmt.Errorf("post-promotion check failed: %w", err)
			recordError(wrapped)
			finish(pipelineResult{candidate: candidateVersion, err: wrapped})
		}
		return
	}

	_ = m.notifier.Notify(ctx, "Promotion complete", fmt.Sprintf("Candidate %s deployed successfully", candidateVersion))
	promotedSuccessfully = true
	finish(pipelineResult{candidate: candidateVersion, promoted: true, err: nil})
}

func (m *Manager) stageRelease(ctx context.Context, currentVersion string) <-chan releaseStageResult {
	ch := make(chan releaseStageResult, 1)
	go func() {
		defer close(ch)
		release, err := m.fetcher.Latest(ctx)
		if err != nil {
			ch <- releaseStageResult{err: err}
			return
		}
		if strings.TrimSpace(release.Tag) == "" {
			ch <- releaseStageResult{hasUpdate: false}
			return
		}
		if sanitize(release.Tag) == sanitize(currentVersion) {
			ch <- releaseStageResult{hasUpdate: false}
			return
		}
		ch <- releaseStageResult{release: release, hasUpdate: true, needsBackup: release.RequiresDBBackup}
	}()
	return ch
}

func (m *Manager) stageDownload(ctx context.Context, release Release, currentVersion string) <-chan downloadStageResult {
	ch := make(chan downloadStageResult, 1)
	go func() {
		defer close(ch)
		var asset ReleaseAsset
		for _, a := range release.Assets {
			if strings.TrimSpace(a.DownloadURL) != "" {
				asset = a
				break
			}
		}
		if strings.TrimSpace(asset.DownloadURL) == "" {
			ch <- downloadStageResult{err: errors.New("selfupgrade: release has no downloadable assets")}
			return
		}
		versionDir := filepath.Join(m.cfg.DeployDir, sanitize(release.Tag))
		if err := ensureDir(versionDir); err != nil {
			ch <- downloadStageResult{err: err}
			return
		}
		candidatePath := filepath.Join(versionDir, asset.Name)
		checksum, err := downloadToFile(ctx, m.client, asset.DownloadURL, candidatePath)
		if err != nil {
			ch <- downloadStageResult{err: err}
			return
		}
		if err := writeChecksumFile(candidatePath+".sha256", checksum); err != nil {
			ch <- downloadStageResult{err: err}
			return
		}

		var lastGoodPath, lastGoodChecksum string
		binaryPath := strings.TrimSpace(m.cfg.BinaryPath)
		if binaryPath != "" {
			if _, err := os.Stat(binaryPath); err == nil {
				base := filepath.Base(binaryPath)
				backupName := fmt.Sprintf("%s.previous", sanitize(base))
				lastGoodPath = filepath.Join(filepath.Dir(binaryPath), backupName)
				lastGoodChecksum, err = copyFileWithHash(ctx, binaryPath, lastGoodPath)
				if err != nil {
					ch <- downloadStageResult{err: err}
					return
				}
				if err := writeChecksumFile(lastGoodPath+".sha256", lastGoodChecksum); err != nil {
					ch <- downloadStageResult{err: err}
					return
				}
			}
		}

		ch <- downloadStageResult{
			candidatePath:     candidatePath,
			candidateChecksum: checksum,
			lastGoodPath:      lastGoodPath,
			lastGoodChecksum:  lastGoodChecksum,
		}
	}()
	return ch
}

func (m *Manager) stageDatabase(ctx context.Context, version string, needBackup bool) <-chan databaseStageResult {
	ch := make(chan databaseStageResult, 1)
	go func() {
		defer close(ch)
		if !needBackup {
			ch <- databaseStageResult{}
			return
		}
		if m.database == nil {
			ch <- databaseStageResult{err: errors.New("selfupgrade: database controller required for backup")}
			return
		}
		backup, err := m.database.Backup(ctx, version)
		if err != nil {
			ch <- databaseStageResult{err: err}
			return
		}
		clone, err := m.database.Clone(ctx, version)
		if err != nil {
			ch <- databaseStageResult{backupPath: backup, err: err}
			return
		}
		ch <- databaseStageResult{backupPath: backup, clone: clone}
	}()
	return ch
}

func (m *Manager) stageCanary(ctx context.Context, candidate CandidateBuild, clone CloneInfo) <-chan canaryStageResult {
	ch := make(chan canaryStageResult, 1)
	go func() {
		defer close(ch)
		inst, err := m.runner.StartCanary(ctx, candidate, clone, m.cfg.CanaryPort)
		if err != nil {
			ch <- canaryStageResult{err: err}
			return
		}
		if inst.Ready != nil {
			select {
			case <-ctx.Done():
				ch <- canaryStageResult{instance: inst, err: ctx.Err()}
				return
			case <-inst.Ready:
			}
		}
		if inst.Err != nil {
			select {
			case err := <-inst.Err:
				if err != nil {
					ch <- canaryStageResult{instance: inst, err: err}
					return
				}
			default:
			}
		}
		ch <- canaryStageResult{instance: inst}
	}()
	return ch
}

func (m *Manager) verifyCanary(ctx context.Context) error {
	path := strings.TrimSpace(m.cfg.HealthCheckPath)
	if path == "" {
		return nil
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	url := fmt.Sprintf("http://127.0.0.1:%d%s", m.cfg.CanaryPort, path)
	client := m.client
	if client == nil {
		client = http.DefaultClient
	}
	attempts := 5
	for i := 0; i < attempts; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return fmt.Errorf("selfupgrade: canary health check failed for %s", url)
}

func (m *Manager) awaitDecision(ctx context.Context, ch <-chan Decision) (Decision, error) {
	select {
	case <-ctx.Done():
		return Decision{}, ctx.Err()
	case d := <-ch:
		return d, nil
	}
}

func (m *Manager) applyBinary(ctx context.Context, candidatePath string) error {
	if strings.TrimSpace(m.cfg.BinaryPath) == "" {
		return errors.New("selfupgrade: binary path not configured")
	}
	temp := m.cfg.BinaryPath + ".new"
	if _, err := copyFileWithHash(ctx, candidatePath, temp); err != nil {
		return err
	}
	return os.Rename(temp, m.cfg.BinaryPath)
}

func (m *Manager) rollback(ctx context.Context, lastGoodPath, backupPath string) error {
	if strings.TrimSpace(lastGoodPath) == "" {
		return errors.New("selfupgrade: no last-good binary available for rollback")
	}
	if err := m.applyBinary(ctx, lastGoodPath); err != nil {
		return err
	}
	if err := m.runner.RestartService(ctx); err != nil {
		return err
	}
	if err := m.runner.PostPromotionCheck(ctx); err == nil {
		_ = m.notifier.Notify(ctx, "Rollback", "Service restored using last-good binary")
		return nil
	}
	if m.database != nil && strings.TrimSpace(backupPath) != "" {
		if err := m.database.RestoreBackup(ctx, backupPath); err != nil {
			return fmt.Errorf("selfupgrade: rollback failed and DB restore failed: %w", err)
		}
		if err := m.applyBinary(ctx, lastGoodPath); err != nil {
			return err
		}
		if err := m.runner.RestartService(ctx); err != nil {
			return err
		}
		if err := m.runner.PostPromotionCheck(ctx); err != nil {
			return err
		}
		_ = m.notifier.Notify(ctx, "Rollback", "Service restored using last-good binary and database backup")
		return nil
	}
	return errors.New("selfupgrade: rollback failed and no database backup available")
}

func (m *Manager) recordFailureLog(version, logContent string, err error) {
	version = strings.TrimSpace(version)
	if version == "" {
		return
	}
	dir := filepath.Join(m.cfg.DeployDir, "logs")
	if err := ensureDir(dir); err != nil {
		m.logf("[selfupgrade] failed to create log directory for %s: %v", version, err)
		return
	}
	filename := fmt.Sprintf("%s-%s.log", sanitize(version), time.Now().UTC().Format("20060102-150405"))
	path := filepath.Join(dir, filename)

	builder := &strings.Builder{}
	if err != nil {
		builder.WriteString(fmt.Sprintf("error: %v\n", err))
	}
	if strings.TrimSpace(logContent) != "" {
		builder.WriteString(logContent)
		if !strings.HasSuffix(logContent, "\n") {
			builder.WriteString("\n")
		}
	}
	if writeErr := os.WriteFile(path, []byte(builder.String()), 0o644); writeErr != nil {
		m.logf("[selfupgrade] failed to write failure log for %s: %v", version, writeErr)
		return
	}
	if err != nil {
		m.logf("[selfupgrade] upgrade %s failed: %v (log at %s)", version, err, path)
	} else {
		m.logf("[selfupgrade] upgrade %s log stored at %s", version, path)
	}
}

func urlQueryEscape(s string) string {
	return url.QueryEscape(strings.TrimSpace(s))
}

// HTTPHandler exposes decision and status endpoints under /selfupgrade/.
func (m *Manager) HTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/selfupgrade")
		switch {
		case path == "" || path == "/":
			http.NotFound(w, r)
		case strings.HasPrefix(path, "/status"):
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			status, err := m.Status(r.Context())
			if err != nil {
				http.Error(w, err.Error(), http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(status)
		case strings.HasPrefix(path, "/trigger"):
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if err := m.Trigger(r.Context()); err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			w.WriteHeader(http.StatusAccepted)
		case strings.HasPrefix(path, "/approve"):
			if r.Method != http.MethodPost && r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			version := r.URL.Query().Get("version")
			if strings.TrimSpace(version) == "" {
				http.Error(w, "missing version", http.StatusBadRequest)
				return
			}
			if err := m.Decision(r.Context(), Decision{Version: version, Approve: true}); err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			w.WriteHeader(http.StatusAccepted)
		case strings.HasPrefix(path, "/reject"):
			if r.Method != http.MethodPost && r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			version := r.URL.Query().Get("version")
			reason := r.URL.Query().Get("reason")
			if strings.TrimSpace(version) == "" {
				http.Error(w, "missing version", http.StatusBadRequest)
				return
			}
			if err := m.Decision(r.Context(), Decision{Version: version, Approve: false, Reason: reason}); err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			w.WriteHeader(http.StatusAccepted)
		default:
			http.NotFound(w, r)
		}
	})
}
