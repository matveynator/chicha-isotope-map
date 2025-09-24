package selfupgrade

import (
	"context"
	"net/http"
	"time"
)

// Config defines the knobs used by the background deployment manager.
// We pass collaborators instead of global singletons so the package stays
// testable and reusable across different binaries, following "The bigger the
// interface, the weaker the abstraction". Callers can plug in mocks or real
// implementations to match their environment.
type Config struct {
	RepoOwner       string        // GitHub owner that publishes releases
	RepoName        string        // GitHub repository name with releases
	CurrentVersion  string        // Currently running version tag
	PollInterval    time.Duration // How often to check for updates
	BinaryPath      string        // Path to the production binary to swap
	DeployDir       string        // Workspace for downloaded artifacts
	LastGoodDir     string        // Directory holding the last known good build
	DBBackupsDir    string        // Directory holding database backups
	CanaryPort      int           // Port used for canary launches
	HealthCheckPath string        // HTTP path to probe the canary
	ServiceName     string        // Optional systemd unit for restart hooks
	CloneRetention  time.Duration // How long to keep database clones
	HTTPClient      *http.Client  // HTTP client used for API calls
	ReleaseFetcher  ReleaseFetcher
	Notifier        Notifier
	Runner          Runner
	Database        DatabaseController
	Logf            func(string, ...any)
}

// Release models the data we need from a source control release feed.
// Only the subset required by the deployment pipeline is exposed so we can
// layer additional sources in the future without breaking callers.
type Release struct {
	Tag        string
	Name       string
	Body       string
	Published  time.Time
	Draft      bool
	Prerelease bool
	Assets     []ReleaseAsset
}

// ReleaseAsset points to a downloadable artifact (usually a binary archive).
type ReleaseAsset struct {
	Name        string
	DownloadURL string
	ContentType string
	Size        int64
}

// ReleaseFetcher abstracts the source that produces versioned artifacts.
// GitHub is the default, but tests or private registries can plug their own
// implementation by following the same contract.
type ReleaseFetcher interface {
	Latest(ctx context.Context) (Release, error)
}

// Notifier pushes human-friendly messages about the rollout progress.
// Deployments rarely happen without humans in the loop, so we keep the
// interface tiny and synchronous to encourage simple logging or webhook
// adapters.
type Notifier interface {
	Notify(ctx context.Context, title, message string) error
}

// Runner encapsulates process level operations: staging a canary, restarting
// the production service, and reporting whether the new build is healthy.
// We intentionally avoid exposing process handles so callers can integrate with
// supervisors like systemd without leaking implementation details.
type Runner interface {
	StartCanary(ctx context.Context, build CandidateBuild, clone CloneInfo, port int) (CanaryInstance, error)
	StopCanary(ctx context.Context, instance CanaryInstance) error
	RestartService(ctx context.Context) error
	PostPromotionCheck(ctx context.Context) error
}

// DatabaseController performs backup/clone/restore work so the manager stays
// decoupled from any specific SQL dialect. The cloning API returns an opaque
// descriptor that the runner can feed into a canary, keeping the data flow
// explicit and debuggable.
type DatabaseController interface {
	Backup(ctx context.Context, version string) (string, error)
	Clone(ctx context.Context, version string) (CloneInfo, error)
	DropClone(ctx context.Context, clone CloneInfo) error
	RestoreBackup(ctx context.Context, backupPath string) error
}

// CandidateBuild describes the artifact we downloaded for a release.
type CandidateBuild struct {
	Version        string
	BinaryPath     string
	ChecksumSHA256 string
}

// CloneInfo explains where the cloned database lives so callers can plug it
// into their canary process without guessing paths or DSNs.
type CloneInfo struct {
	Name string
	DSN  string
	Path string
}

// CanaryInstance is a lightweight handle for a launched test process.
type CanaryInstance struct {
	ID    string
	Ready <-chan struct{}
	Err   <-chan error
}

// Decision communicates the tester's verdict back into the manager.
type Decision struct {
	Version string
	Approve bool
	Reason  string
}

// Status summarizes the deployment state for status endpoints and tests.
type Status struct {
	CurrentVersion   string    `json:"current_version"`
	CandidateVersion string    `json:"candidate_version"`
	ActiveStage      string    `json:"active_stage"`
	LastError        string    `json:"last_error"`
	LastCheck        time.Time `json:"last_check"`
	LastSuccess      time.Time `json:"last_success"`
}
