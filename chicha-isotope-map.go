//new: stream markers by track

package main

import (

	// http://localhost:8765/debug/pprof/profile?seconds=30
	// go tool pprof -http=:8080 Downloads/profile
	//_ "net/http/pprof"

	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"embed"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"golang.org/x/crypto/acme/autocert"
	"html"
	"html/template"
	"image/color"
	"io"
	"io/fs"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"chicha-isotope-map/pkg/api"
	"chicha-isotope-map/pkg/cimimport"
	"chicha-isotope-map/pkg/database"
	"chicha-isotope-map/pkg/database/drivers"
	"chicha-isotope-map/pkg/jsonarchive"
	"chicha-isotope-map/pkg/logger"
	"chicha-isotope-map/pkg/qrlogoext"
	safecastrealtime "chicha-isotope-map/pkg/safecast-realtime"
	"chicha-isotope-map/pkg/selfupgrade"
)

// content bundles the UI and the license texts so single-file binaries still
// expose the legal notice when served offline. Embedding keeps deployment
// simple and mirrors the "A little copying is better than a little dependency"
// proverb by avoiding extra runtime file IO.
//
//go:embed public_html/* LICENSE LICENSE.CC0
var content embed.FS

var doseData database.Data

var domain = flag.String("domain", "", "Serve HTTPS on 80/443 via Let's Encrypt when a domain is provided.")
var dbType = flag.String("db-type", "sqlite", "Database driver: chai, sqlite, duckdb, pgx (PostgreSQL), or clickhouse")
var dbPath = flag.String("db-path", "", "Filesystem path for chai/sqlite/duckdb databases; defaults to the working directory.")
var dbConn = flag.String("db-conn", "", "Connection URI for network databases.\n  PostgreSQL: postgres://user:pass@host:5432/<database>?sslmode=verify-full\n  ClickHouse: clickhouse://user:pass@host:9000/<database>?secure=true")
var port = flag.Int("port", 8765, "Port for running the HTTP server when not using -domain.")
var version = flag.Bool("version", false, "Show the application version")
var defaultLat = flag.Float64("default-lat", 44.08832, "Default map latitude")
var defaultLon = flag.Float64("default-lon", 42.97577, "Default map longitude")
var defaultZoom = flag.Int("default-zoom", 11, "Default map zoom")
var defaultLayer = flag.String("default-layer", "OpenStreetMap", `Default base layer: "OpenStreetMap" or "Google Satellite"`)
var safecastRealtimeEnabled = flag.Bool("safecast-realtime", false, "Enable polling and display of Safecast realtime devices")
var jsonArchivePathFlag = flag.String("json-archive-path", "", "Filesystem destination for the generated JSON archive tgz bundle")
var jsonArchiveFrequencyFlag = flag.String("json-archive-frequency", "weekly", "How often to rebuild the JSON archive: daily, weekly, monthly, or yearly")
var importTGZURLFlag = flag.String("import-tgz-url", "", "Download and import a remote .tgz of exported JSON files, log progress, and exit once finished.")
var importTGZFileFlag = flag.String("import-tgz-file", "", "Import a local .tgz of exported JSON files, log progress, and exit once finished.")
var supportEmail = flag.String("support-email", "", "Contact e-mail shown in the legal notice for feedback")
var debugIPsFlag = flag.String("debug", "", "Comma separated IP addresses allowed to view the debug overlay")

// debugIPAllowlist keeps a fast lookup of remote addresses that should see the
// technical overlay. We keep it as a map so lookups stay O(1) without extra
// synchronization, following "Clear is better than clever" by leaning on Go's
// built-in map semantics.
var debugIPAllowlist map[string]struct{}

// usageSection groups CLI flags so operators can scan help output quickly. This keeps
// the help text approachable without duplicating flag registration everywhere.
type usageSection struct {
	Title string
	Flags []string
}

var cliUsageSections = []usageSection{
	{Title: "General", Flags: []string{"version", "domain", "port", "support-email"}},
	{Title: "Database", Flags: []string{"db-type", "db-path", "db-conn"}},
	{Title: "Map defaults", Flags: []string{"default-lat", "default-lon", "default-zoom", "default-layer"}},
	{Title: "Realtime & archives", Flags: []string{"safecast-realtime", "json-archive-path", "json-archive-frequency", "import-tgz-url", "import-tgz-file"}},
	{Title: "Self-upgrade", Flags: []string{"selfupgrade", "selfupgrade-url"}},
}

// cliColorTheme centralises ANSI escape sequences so we can keep colourful help output
// consistent while still falling back to plain text when stdout is redirected. By
// wrapping colour codes in a struct we avoid scattering control characters throughout
// the printing logic and make future tweaks easier to follow.
type cliColorTheme struct {
	Enabled bool
	Section string
	Flag    string
	Usage   string
	Default string
	Reset   string
}

// resolveCLIColorTheme inspects the provided writer to decide whether colourful output is
// appropriate. We only enable ANSI sequences when stdout points to a terminal and the
// operator has not explicitly disabled colour via NO_COLOR, aligning with the "don't
// fight the tool" proverb by respecting common shell conventions.
func resolveCLIColorTheme(out io.Writer) cliColorTheme {
	theme := cliColorTheme{}
	file, ok := out.(*os.File)
	if !ok {
		return theme
	}
	if os.Getenv("NO_COLOR") != "" {
		return theme
	}
	info, err := file.Stat()
	if err != nil {
		return theme
	}
	if (info.Mode() & os.ModeCharDevice) == 0 {
		return theme
	}

	theme.Enabled = true
	// We switch to a punchier palette that keeps contrast on both dark and light
	// backgrounds without feeling gaudy. Section headings lean on a deep ocean blue,
	// flags use a vibrant amber, usage strings stay in neutral charcoal, and defaults
	// adopt a rich forest green. The tones remain saturated enough to pop on light
	// themes while still carrying enough depth for dark terminals.
	theme.Section = "\033[38;5;25m"
	theme.Flag = "\033[38;5;208m"
	theme.Usage = "\033[38;5;240m"
	theme.Default = "\033[38;5;34m"
	theme.Reset = "\033[0m"
	return theme
}

// selfUpgradeFlagSet keeps the flag pointers optional so unsupported platforms
// never register a -selfupgrade flag, following the "Make the zero value useful"
// proverb. We also carry the default URLs so runtime decisions can fall back to
// platform-specific release assets without extra state.
type selfUpgradeFlagSet struct {
	enabled    *bool
	url        *string
	supported  bool
	defaultURL string
	duckDBURL  string
}

var selfUpgradeFlags = registerSelfUpgradeFlags()

// configureCLIUsage replaces the default flag help with a grouped layout. We do this in init()
// so operators immediately see logically clustered options when running -h, without juggling
// extra wiring at call sites.
func configureCLIUsage() {
	flag.CommandLine.SetOutput(os.Stdout)
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		theme := resolveCLIColorTheme(out)

		fmt.Fprintf(out, "Usage: %s [flags]\n\n", os.Args[0])
		if theme.Enabled {
			fmt.Fprintf(out, "%sFlags:%s\n", theme.Section, theme.Reset)
		} else {
			fmt.Fprintln(out, "Flags:")
		}

		printed := map[string]bool{}
		for _, section := range cliUsageSections {
			var sectionFlags []*flag.Flag
			for _, name := range section.Flags {
				if f := flag.Lookup(name); f != nil {
					sectionFlags = append(sectionFlags, f)
					printed[f.Name] = true
				}
			}
			if len(sectionFlags) == 0 {
				continue
			}

			if theme.Enabled {
				fmt.Fprintf(out, "%s%s:%s\n", theme.Section, section.Title, theme.Reset)
			} else {
				fmt.Fprintf(out, "%s:\n", section.Title)
			}
			for _, f := range sectionFlags {
				writeFlagUsage(out, f, theme)
			}
			fmt.Fprintln(out)
		}

		var leftovers []string
		flag.VisitAll(func(f *flag.Flag) {
			if !printed[f.Name] {
				leftovers = append(leftovers, f.Name)
			}
		})
		if len(leftovers) > 0 {
			sort.Strings(leftovers)
			if theme.Enabled {
				fmt.Fprintf(out, "%sAdditional flags:%s\n", theme.Section, theme.Reset)
			} else {
				fmt.Fprintln(out, "Additional flags:")
			}
			for _, name := range leftovers {
				if f := flag.Lookup(name); f != nil {
					writeFlagUsage(out, f, theme)
				}
			}
		}

		printCLILicenseNote(out, theme)
	}
}

// writeFlagUsage mirrors flag.PrintDefaults but adds indentation and multiline support so the
// help output stays legible even when descriptions contain examples.
func writeFlagUsage(out io.Writer, f *flag.Flag, theme cliColorTheme) {
	if f == nil {
		return
	}
	name, usage := flag.UnquoteUsage(f)
	if theme.Enabled {
		fmt.Fprintf(out, "  %s-%s%s", theme.Flag, f.Name, theme.Reset)
	} else {
		fmt.Fprintf(out, "  -%s", f.Name)
	}
	if name != "" {
		fmt.Fprintf(out, " %s", name)
	}
	fmt.Fprintln(out)

	if usage != "" {
		for _, part := range strings.Split(usage, "\n") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if theme.Enabled {
				fmt.Fprintf(out, "      %s%s%s\n", theme.Usage, part, theme.Reset)
			} else {
				fmt.Fprintf(out, "      %s\n", part)
			}
		}
	}

	if def := strings.TrimSpace(f.DefValue); def != "" {
		if theme.Enabled {
			fmt.Fprintf(out, "      %sDefault:%s %s%s%s\n", theme.Flag, theme.Reset, theme.Default, def, theme.Reset)
		} else {
			fmt.Fprintf(out, "      Default: %s\n", def)
		}
	}
}

// printCLILicenseNote mirrors the in-app license block so terminal operators see the
// same promise: code under MIT, research data under CC0, and an open invitation to
// collaborate. Keeping the wording here ensures the CLI reflects the project ethos
// without forcing admins to open the UI.
func printCLILicenseNote(out io.Writer, theme cliColorTheme) {
	if out == nil {
		return
	}

	fmt.Fprintln(out)
	if theme.Enabled {
		fmt.Fprintf(out, "%sLicense & community:%s\n", theme.Section, theme.Reset)
	} else {
		fmt.Fprintln(out, "License & community:")
	}

	lines := []string{
		"Code: MIT License.",
		"Research datasets: CC0 1.0 Universal (Public Domain).",
		"Thank you for using this program and sharing your tracks. This work is fragile — care for it, and it will grow.",
		"Support the sources, share honest knowledge, and run your own nodes so the maps stay free and safe.",
	}

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if theme.Enabled {
			fmt.Fprintf(out, "  %s%s%s\n", theme.Usage, line, theme.Reset)
		} else {
			fmt.Fprintf(out, "  %s\n", line)
		}
	}
}

var CompileVersion = "dev"

var (
	apiDocsArchiveEnabled   bool
	apiDocsArchiveRoute     string
	apiDocsArchiveFrequency string
)

var db *database.Database

func init() {
	// We trigger driver registration here so "go run chicha-isotope-map.go" keeps
	// working even when auxiliary files are skipped; relying on init avoids extra
	// coordination primitives and mirrors Go's preference for simplicity.
	drivers.Ready()
	// CLI usage grouping is also configured once during init so every entry point
	// inherits the readable help layout without repeating boilerplate.
	configureCLIUsage()
}

// =====================
// WEB — API docs page
// =====================
func apiDocsHandler(w http.ResponseWriter, r *http.Request) {
	// Serve a static, embedded HTML with API usage instructions.
	// Keep it simple and cacheable by default; clients can refresh as needed.
	b, err := content.ReadFile("public_html/api-usage.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}

	scheme := "http"
	if proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); proto != "" {
		scheme = strings.ToLower(proto)
	} else if r.TLS != nil {
		scheme = "https"
	}

	host := strings.TrimSpace(r.Host)
	if host == "" {
		if strings.TrimSpace(*domain) != "" {
			host = strings.TrimSpace(*domain)
		} else {
			host = fmt.Sprintf("localhost:%d", *port)
		}
	}

	baseURL := fmt.Sprintf("%s://%s", scheme, host)
	apiRoot := strings.TrimRight(baseURL, "/") + "/api"

	page := string(b)
	page = strings.ReplaceAll(page, "__BASE_URL__", baseURL)
	page = strings.ReplaceAll(page, "__API_ROOT__", apiRoot)
	page = strings.ReplaceAll(page, "__DISPLAY_HOST__", host)
	page = strings.ReplaceAll(page, "__ARCHIVE_ENABLED__", strconv.FormatBool(apiDocsArchiveEnabled))

	route := strings.TrimSpace(apiDocsArchiveRoute)
	if route == "" {
		route = "/api/json/weekly.tgz"
	}
	page = strings.ReplaceAll(page, "__ARCHIVE_ROUTE__", route)

	freq := strings.TrimSpace(apiDocsArchiveFrequency)
	if freq == "" {
		freq = "weekly"
	}
	page = strings.ReplaceAll(page, "__ARCHIVE_FREQUENCY__", freq)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(page))
}

// resolveArchivePath decides where the JSON archive tgz should live.
// We prefer explicit destinations from flags, otherwise fall back to the user's
// home directory so long-running services do not clutter the repository tree.
// We log resolution failures so operators notice and can correct their setup.
// The defaultFile argument feeds through the configured cadence and domain so
// implicit directories still produce predictable filenames.
func resolveArchivePath(flagValue, defaultFile string, logf func(string, ...any)) string {
	cleaned := strings.TrimSpace(flagValue)
	fallback := strings.TrimSpace(defaultFile)
	if fallback == "" {
		fallback = "weekly-json.tgz"
	}
	if cleaned != "" {
		abs, err := filepath.Abs(cleaned)
		if err != nil {
			if logf != nil {
				logf("json archive path resolution fallback for %q: %v", cleaned, err)
			}
			return filepath.Clean(cleaned)
		}
		return abs
	}

	home, err := os.UserHomeDir()
	if err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, fallback)
	}

	// Falling back to the working directory keeps the archive predictable even
	// in minimal environments where HOME is undefined, trading cleverness for
	// clarity per the Go proverbs.
	wd, wdErr := os.Getwd()
	if wdErr == nil && strings.TrimSpace(wd) != "" {
		return filepath.Join(wd, fallback)
	}

	// As a last resort return a relative filename so the generator can still run.
	return fallback
}

// applyDBConnection parses a DSN passed via -db-conn and copies relevant fields into the
// database configuration. We normalise defaults for host, port, and SSL/TLS so operators can
// supply concise URLs while the rest of the application continues using structured settings.
func applyDBConnection(driverName, conn string, cfg *database.Config) error {
	if cfg == nil {
		return fmt.Errorf("db config is nil")
	}
	cleaned := strings.TrimSpace(conn)
	if cleaned == "" {
		return nil
	}

	parsed, err := url.Parse(cleaned)
	if err != nil {
		return fmt.Errorf("%s connection string: %w", driverName, err)
	}

	driver := strings.ToLower(strings.TrimSpace(driverName))
	switch driver {
	case "pgx":
		if parsed.Scheme == "" {
			parsed.Scheme = "postgres"
		}
	case "clickhouse":
		if parsed.Scheme == "" {
			parsed.Scheme = "clickhouse"
		}
	default:
		return fmt.Errorf("db-conn is only supported for pgx or clickhouse (got %q)", driverName)
	}

	host := parsed.Hostname()
	if host == "" {
		host = "127.0.0.1"
	}
	cfg.DBHost = host

	portValue := parsed.Port()
	var port int
	if portValue != "" {
		port, err = strconv.Atoi(portValue)
		if err != nil {
			return fmt.Errorf("%s connection string: invalid port %q", driverName, portValue)
		}
	} else {
		if driver == "pgx" {
			port = 5432
		} else {
			port = 9000
		}
	}
	cfg.DBPort = port

	if parsed.User != nil {
		if user := strings.TrimSpace(parsed.User.Username()); user != "" {
			cfg.DBUser = user
		}
		if pass, ok := parsed.User.Password(); ok {
			cfg.DBPass = pass
		}
	}

	name := strings.Trim(strings.TrimPrefix(parsed.Path, "/"), " ")
	if driver == "pgx" && name == "" {
		return fmt.Errorf("%s connection string must include a database name", driverName)
	}
	if name != "" || driver == "pgx" {
		cfg.DBName = name
	}

	query := parsed.Query()
	switch driver {
	case "pgx":
		sslMode := strings.TrimSpace(query.Get("sslmode"))
		if sslMode == "" {
			sslMode = "prefer"
			query.Set("sslmode", sslMode)
		}
		cfg.PGSSLMode = sslMode
	case "clickhouse":
		secureValue := strings.TrimSpace(query.Get("secure"))
		secure := false
		if secureValue != "" {
			secure = secureValue == "1" || strings.EqualFold(secureValue, "true") || strings.EqualFold(secureValue, "yes") || strings.EqualFold(secureValue, "on")
		} else if strings.EqualFold(parsed.Scheme, "https") {
			secure = true
			query.Set("secure", "true")
		}
		cfg.ClickSecure = secure
	}
	parsed.RawQuery = query.Encode()

	cfg.DBConn = parsed.String()
	return nil
}

// selfUpgradeDatabaseInfo recreates the DSN resolution logic so the deployment
// manager knows how to back up the live database. We intentionally mirror the
// database package defaults instead of importing internal helpers to avoid
// circular dependencies.
func selfUpgradeDatabaseInfo(cfg database.Config) (driver, dsn string) {
	driver = strings.ToLower(strings.TrimSpace(cfg.DBType))
	switch driver {
	case "sqlite", "chai":
		dsn = strings.TrimSpace(cfg.DBPath)
		if dsn == "" {
			dsn = fmt.Sprintf("database-%d.%s", cfg.Port, driver)
		}
	case "duckdb":
		dsn = strings.TrimSpace(cfg.DBPath)
		if dsn == "" {
			dsn = fmt.Sprintf("database-%d.duckdb", cfg.Port)
		}
	case "pgx":
		if strings.TrimSpace(cfg.DBConn) != "" {
			dsn = cfg.DBConn
		} else {
			dsn = fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
				cfg.DBUser, cfg.DBPass, cfg.DBHost, cfg.DBPort, cfg.DBName, cfg.PGSSLMode)
		}
	case "clickhouse":
		dsn = database.ClickHouseDSNFromConfig(cfg)
	}
	return driver, dsn
}

// startSelfUpgrade wires up the background deployment manager when operators
// pass -selfupgrade. We centralise the configuration assembly to keep main()
// readable while still reusing the same channels and goroutines that make the
// manager safe without mutexes, following "Don't communicate by sharing
// memory".
func startSelfUpgrade(ctx context.Context, dbCfg database.Config) context.CancelFunc {
	if ctx == nil {
		ctx = context.Background()
	}
	if !selfUpgradeFlags.supported {
		// Unsupported platforms skip registration entirely, staying silent because no
		// CLI flag is present and there is nothing to configure for operators.
		return nil
	}
	if selfUpgradeFlags.enabled == nil || !*selfUpgradeFlags.enabled {
		// Operators opted out, so we avoid spawning background work.
		return nil
	}

	exePath, err := os.Executable()
	if err != nil {
		// Without a deterministic binary path we cannot build rollback bundles, so we bail out early.
		log.Printf("selfupgrade disabled: cannot resolve executable path: %v", err)
		return nil
	}

	driverName, dsn := selfUpgradeDatabaseInfo(dbCfg)

	downloadURL := ""
	if selfUpgradeFlags.url != nil {
		downloadURL = strings.TrimSpace(*selfUpgradeFlags.url)
	}
	if downloadURL == "" {
		// We default to platform-specific artefacts and swap in DuckDB builds when requested.
		downloadURL = strings.TrimSpace(selfUpgradeFlags.defaultURL)
		if driverName == "duckdb" && strings.TrimSpace(selfUpgradeFlags.duckDBURL) != "" {
			downloadURL = strings.TrimSpace(selfUpgradeFlags.duckDBURL)
		}
	}
	if downloadURL == "" {
		log.Printf("selfupgrade disabled: download URL not configured for %s/%s", runtime.GOOS, runtime.GOARCH)
		return nil
	}

	const canaryPort = 9876
	workspace := filepath.Join(filepath.Dir(exePath), "selfupgrade-cache")
	backupsDir := filepath.Join(workspace, "db_backups")

	var dbController selfupgrade.DatabaseController
	switch driverName {
	case "sqlite", "chai", "duckdb":
		if dsn != "" {
			dbController = &selfupgrade.FileDatabaseController{
				Driver:       driverName,
				OriginalPath: dsn,
				BackupsDir:   backupsDir,
				Logf:         log.Printf,
			}
		}
	default:
		log.Printf("selfupgrade database backups not configured for driver %s", driverName)
	}

	cfgAuto := selfupgrade.Config{
		DownloadURL:    downloadURL,
		CurrentVersion: CompileVersion,
		BinaryPath:     exePath,
		DeployDir:      workspace,
		DBBackupsDir:   backupsDir,
		CanaryPort:     canaryPort,
		Logf:           log.Printf,
		Database:       dbController,
	}

	manager, err := selfupgrade.NewManager(cfgAuto)
	if err != nil {
		log.Printf("selfupgrade disabled: %v", err)
		return nil
	}

	ctxDeploy, cancel := context.WithCancel(ctx)
	if err := manager.Start(ctxDeploy); err != nil {
		log.Printf("selfupgrade start failed: %v", err)
		cancel()
		return nil
	}

	http.Handle("/selfupgrade/", manager.HTTPHandler())
	go func() {
		manager.Wait()
	}()
	log.Printf("selfupgrade manager polling %s", downloadURL)

	return cancel
}

// selfupgradeStartupDelay pauses the freshly spawned binary during a handoff so
// the previous process can close network listeners without racing. The old
// binary encodes the delay inside SELFUPGRADE_WAIT_SECONDS.
func selfupgradeStartupDelay(logf func(string, ...any)) {
	waitEnv := strings.TrimSpace(os.Getenv("SELFUPGRADE_WAIT_SECONDS"))
	if waitEnv == "" {
		return
	}
	defer os.Unsetenv("SELFUPGRADE_WAIT_SECONDS")
	seconds, err := strconv.ParseFloat(waitEnv, 64)
	if err != nil || seconds <= 0 {
		if logf != nil && err != nil {
			logf("selfupgrade: invalid wait seconds %q: %v", waitEnv, err)
		}
		return
	}
	delay := time.Duration(seconds * float64(time.Second))
	if logf != nil {
		logf("selfupgrade: waiting %s for predecessor shutdown", delay)
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	<-timer.C
}

func selfupgradeCleanEnv(env []string) []string {
	cleaned := make([]string, 0, len(env))
	for _, kv := range env {
		if strings.HasPrefix(kv, "SELFUPGRADE_") {
			continue
		}
		cleaned = append(cleaned, kv)
	}
	return cleaned
}

// selfupgradeRollback revives the last known good binary when the replacement
// process fails to bind its listeners. We relaunch the previous binary and let
// it resume service.
func selfupgradeRollback(logf func(string, ...any)) bool {
	lastGood := strings.TrimSpace(os.Getenv("SELFUPGRADE_LAST_GOOD"))
	if lastGood == "" {
		return false
	}
	exe, err := os.Executable()
	if err != nil {
		if logf != nil {
			logf("selfupgrade rollback skipped: executable unknown: %v", err)
		}
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := selfupgrade.RestoreBinary(ctx, lastGood, exe); err != nil {
		if logf != nil {
			logf("selfupgrade rollback restore failed: %v", err)
		}
		return false
	}
	env := selfupgradeCleanEnv(os.Environ())
	if logf != nil {
		logf("selfupgrade rollback: relaunching %s", lastGood)
	}
	if runtime.GOOS != "windows" {
		if err := syscall.Exec(exe, os.Args, env); err != nil {
			if logf != nil {
				logf("selfupgrade rollback exec failed: %v", err)
			}
			return false
		}
		return true
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Start(); err != nil {
		if logf != nil {
			logf("selfupgrade rollback spawn failed: %v", err)
		}
		return false
	}
	return true
}

// selfupgradeHandleServerError unifies server startup failures so the
// self-upgrade workflow can roll back to the previous binary when binding the
// listener fails.
func selfupgradeHandleServerError(err error, logf func(string, ...any)) {
	if err == nil {
		return
	}
	if errors.Is(err, http.ErrServerClosed) {
		return
	}
	if logf != nil {
		logf("HTTP server error: %v", err)
	}
	if selfupgradeRollback(logf) {
		os.Exit(0)
	}
}

// registerSelfUpgradeFlags inspects the compilation target and only registers
// self-upgrade flags for platforms where we publish release binaries. This keeps
// unsupported builds free of dead flags while still allowing callers to override
// the download location when needed.
func registerSelfUpgradeFlags() selfUpgradeFlagSet {
	const base = "https://github.com/matveynator/chicha-isotope-map/releases/download/latest/"

	switch runtime.GOOS {
	case "darwin":
		switch runtime.GOARCH {
		case "amd64":
			return newSelfUpgradeFlagSet(base, "darwin/amd64", "chicha-isotope-map_darwin_amd64", "chicha-isotope-map_darwin_amd64_duckdb")
		case "arm64":
			return newSelfUpgradeFlagSet(base, "darwin/arm64", "chicha-isotope-map_darwin_arm64", "chicha-isotope-map_darwin_arm64_duckdb")
		}
	case "freebsd":
		if runtime.GOARCH == "amd64" {
			return newSelfUpgradeFlagSet(base, "freebsd/amd64", "chicha-isotope-map_freebsd_amd64", "")
		}
	case "linux":
		switch runtime.GOARCH {
		case "386":
			return newSelfUpgradeFlagSet(base, "linux/386", "chicha-isotope-map_linux_386", "")
		case "amd64":
			return newSelfUpgradeFlagSet(base, "linux/amd64", "chicha-isotope-map_linux_amd64", "")
		case "arm64":
			return newSelfUpgradeFlagSet(base, "linux/arm64", "chicha-isotope-map_linux_arm64", "")
		}
	case "netbsd":
		if runtime.GOARCH == "amd64" {
			return newSelfUpgradeFlagSet(base, "netbsd/amd64", "chicha-isotope-map_netbsd_amd64", "")
		}
	case "openbsd":
		if runtime.GOARCH == "amd64" {
			return newSelfUpgradeFlagSet(base, "openbsd/amd64", "chicha-isotope-map_openbsd_amd64", "")
		}
	case "windows":
		switch runtime.GOARCH {
		case "amd64":
			return newSelfUpgradeFlagSet(base, "windows/amd64", "chicha-isotope-map_windows_amd64.exe", "")
		case "arm64":
			return newSelfUpgradeFlagSet(base, "windows/arm64", "chicha-isotope-map_windows_arm64.exe", "")
		}
	}

	return selfUpgradeFlagSet{supported: false}
}

// newSelfUpgradeFlagSet keeps the flag wiring compact while documenting the
// release artefact associated with each platform. We bake descriptions into the
// help text to reduce guesswork for operators invoking `-help`.
func newSelfUpgradeFlagSet(base, platform, asset, duckAsset string) selfUpgradeFlagSet {
	defaultURL := base + asset
	duckURL := ""
	if strings.TrimSpace(duckAsset) != "" {
		duckURL = base + duckAsset
	}

	enabled := flag.Bool("selfupgrade", false, fmt.Sprintf("Enable the background auto-deployment manager on %s hosts", platform))
	url := flag.String("selfupgrade-url", defaultURL, fmt.Sprintf("Direct download URL for the %s binary", platform))

	return selfUpgradeFlagSet{
		enabled:    enabled,
		url:        url,
		supported:  true,
		defaultURL: defaultURL,
		duckDBURL:  duckURL,
	}
}

// ==========
// Константы для слияния маркеров
// ==========
const (
	markerRadiusPx = 10.0       // радиус кружка в пикселях
	minValidTS     = 1262304000 // 2010-01-01 00:00:00 UTC
)

// microRoentgenPerMicroSievert keeps conversion logic explicit so both the API
// exporter and the JSON importer agree on the units we advertise publicly.
const microRoentgenPerMicroSievert = 100.0

type SpeedRange struct{ Min, Max float64 }

var errNotChichaTrackJSON = errors.New("not chicha track json payload")

// processBGeigieZenFile parses bGeigie Zen/Nano $BNRDD logs.
// Supports ISO8601 timestamps at field[2] and DMM coordinates with N/S/E/W.
func processBGeigieZenFile(
	file multipart.File,
	trackID string,
	db *database.Database,
	dbType string,
) (database.Bounds, string, error) {
	logT(trackID, "BGEIGIE", "▶ start (stream)")

	sc := bufio.NewScanner(file)
	sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	const cpmPerMicroSv = 334.0
	markers := make([]database.Marker, 0, 4096)

	parsed := 0
	skipped := 0
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || !strings.HasPrefix(line, "$BNRDD") {
			skipped++
			continue
		}
		if i := strings.IndexByte(line, '*'); i != -1 {
			line = line[:i]
		}
		p := strings.Split(line, ",")
		if len(p) < 6 { // need at least up to CPMvalid
			skipped++
			continue
		}

		var (
			ts  int64
			cps float64
			cpm float64
			lat float64
			lon float64
		)

		// Zen variant: 0:$BNRDD 1:ver 2:ISO8601 3:CPM 4:CPS 5:TotalCounts 6:fix 7:LATdmm 8:N/S 9:LONdmm 10:E/W ...
		// We rely on CPM for the µSv/h conversion because CPS is instantaneous and noisy.
		if len(p) >= 11 && strings.Contains(p[2], "T") {
			if t, err := time.Parse(time.RFC3339, strings.TrimSpace(p[2])); err == nil {
				ts = t.Unix()
			}
			cpm = parseFloat(p[3])
			cps = parseFloat(p[4])
			lat = parseDMM(p[7], p[8], 2)
			lon = parseDMM(p[9], p[10], 3)
		} else if len(p) >= 8 { // legacy fallback: decimals (+ optional suffix)
			// We only accept if date/time parse succeeds via known helper; otherwise skip silently.
			// If parseBGeigieDateTime isn't present, ts remains 0 and entry is skipped.
			ts = 0
			// try compact forms if helper exists in build
			// cps/cpm & coords
			cpm = parseFloat(p[3])
			cps = parseFloat(p[4])
			lat = parseBGeigieCoord(p[6])
			lon = parseBGeigieCoord(p[7])
		}

		if ts == 0 || (lat == 0 && lon == 0) {
			skipped++
			continue
		}

		dose := 0.0
		if cpm > 0 {
			dose = cpm / cpmPerMicroSv
		} else if cps > 0 {
			dose = (cps * 60.0) / cpmPerMicroSv
		} else {
			skipped++
			continue
		}

		countRate := cps
		if countRate == 0 && cpm > 0 {
			countRate = cpm / 60.0
		}

		markers = append(markers, database.Marker{
			DoseRate:  dose,
			Date:      ts,
			Lon:       lon,
			Lat:       lat,
			CountRate: countRate,
			Zoom:      0,
			Speed:     0,
			TrackID:   trackID,
		})
		parsed++
	}
	if err := sc.Err(); err != nil {
		return database.Bounds{}, trackID, err
	}
	if len(markers) == 0 {
		return database.Bounds{}, trackID, fmt.Errorf("no valid $BNRDD points found (parsed=%d skipped=%d)", parsed, skipped)
	}

	bbox, trackID, err := processAndStoreMarkers(markers, trackID, db, dbType)
	if err != nil {
		return bbox, trackID, err
	}
	logT(trackID, "BGEIGIE", "✔ done (parsed=%d)", parsed)
	return bbox, trackID, nil
}

var speedCatalog = map[string]SpeedRange{
	"ped":   {0, 7},     // < 7 м/с   (~0-25 км/ч)
	"car":   {7, 70},    // 7–70 м/с  (~25-250 км/ч)
	"plane": {70, 1000}, // > 70 м/с  (~250-1800 км/ч)
}

// withServerHeader оборачивает любой http.Handler, добавляя
// заголовок "Server: chicha-isotope-map/<CompileVersion>".
//
// На запрос HEAD к “/” сразу отвечает 200 OK без тела, чтобы
// показать, что сервис жив.

func withServerHeader(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "chicha-isotope-map/"+CompileVersion)

		if r.Method == http.MethodHead && r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// serveWithDomain запускает:
//   • :80  — ACME HTTP-01 + 301-redirect на https://<domain>/…
//   • :443 — HTTPS с автоматическими сертификатами Let’s Encrypt.
//
// Новое: если autocert не может выдать cert (любой host/SNI),
//        сервер всё-таки отдаёт ранее полученный fallback-cert,
//        тем самым устраняя «host not configured» в логах.
//
// Совместимость: TLS ≥ 1.0, ALPN h2/http1.1/http1.0.
// Все ошибки только логируются.

func serveWithDomain(domain string, handler http.Handler) {
	// ----------- ACME manager -----------
	certMgr := &autocert.Manager{
		Prompt: autocert.AcceptTOS,
		Cache:  autocert.DirCache("certs"),
		HostPolicy: func(ctx context.Context, host string) error {
			// Разрешаем голый и www.<domain>
			if host == domain || host == "www."+domain {
				return nil
			}
			// IP-адрес? — не блокируем, просто не пытаемся получить cert.
			if net.ParseIP(host) != nil {
				return nil
			}
			return errors.New("acme/autocert: host not configured")
		},
	}

	// ----------- :80 (challenge + redirect) -----------
	go func() {
		mux80 := http.NewServeMux()
		mux80.Handle("/.well-known/acme-challenge/", certMgr.HTTPHandler(nil))
		mux80.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			target := "https://" + domain + r.URL.RequestURI()
			http.Redirect(w, r, target, http.StatusMovedPermanently)
		})

		log.Printf("HTTP  server (ACME+redirect) ➜ :80")
		if err := (&http.Server{
			Addr:              ":80",
			Handler:           mux80,
			ReadHeaderTimeout: 10 * time.Second,
		}).ListenAndServe(); err != nil {
			selfupgradeHandleServerError(err, log.Printf)
		}
	}()

	// ----------- ежедневная проверка сертификата -----------
	go func() {
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for range t.C {
			if _, err := certMgr.GetCertificate(&tls.ClientHelloInfo{ServerName: domain}); err != nil {
				log.Printf("autocert renewal check: %v", err)
			}
		}
	}()

	// ----------- :443 (HTTPS) -----------
	tlsCfg := certMgr.TLSConfig()
	tlsCfg.MinVersion = tls.VersionTLS10
	tlsCfg.NextProtos = append([]string{"http/1.0"}, tlsCfg.NextProtos...)

	// fallback-сертификат для IP / странных SNI
	var defaultCert *tls.Certificate
	go func() {
		for defaultCert == nil {
			if c, err := certMgr.GetCertificate(&tls.ClientHelloInfo{ServerName: domain}); err == nil {
				defaultCert = c
			}
			time.Sleep(time.Minute)
		}
	}()
	tlsCfg.GetCertificate = func(chi *tls.ClientHelloInfo) (*tls.Certificate, error) {
		c, err := certMgr.GetCertificate(chi)
		if err == nil {
			return c, nil
		}
		// Любой сбой — пытаемся отдать fallback-cert (если уже есть)
		if defaultCert != nil {
			return defaultCert, nil
		}
		// Пока fallback нет — повторяем оригинальную ошибку
		return nil, err
	}

	log.Printf("HTTPS server for %s ➜ :443 (TLS ≥1.0, ALPN h2/http1.1/1.0)", domain)
	if err := (&http.Server{
		Addr:              ":443",
		Handler:           handler,
		TLSConfig:         tlsCfg,
		ReadHeaderTimeout: 10 * time.Second,
	}).ListenAndServeTLS("", ""); err != nil {
		selfupgradeHandleServerError(err, log.Printf)
	}
}

// logT формирует строку "[trackID][component] …" и передаёт её в пакет logger.
// logger сам решит: буферизовать или вывести сразу.
func logT(trackID, component, format string, v ...any) {
	line := fmt.Sprintf("[%-6s][%s] %s", trackID, component, fmt.Sprintf(format, v...))
	logger.Append(trackID, line)
}

// rxFind returns the first submatch (group #1) of pattern in s or an empty string.
// Entities are unescaped and result is TrimSpace-обработан.
func rxFind(s, pattern string) string {
	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(s)
	if len(m) > 1 {
		return strings.TrimSpace(html.UnescapeString(m[1]))
	}
	return ""
}

// isClientDisconnect returns true for network errors indicating that the client
// has gone away (e.g., browser navigated away or closed the tab) while we were
// writing the response. These are normal and should not be logged as errors.
func isClientDisconnect(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "broken pipe") || strings.Contains(msg, "connection reset by peer")
}

// GenerateSerialNumber генерирует TrackID
func GenerateSerialNumber() string {
	const base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	const maxLength = 6

	timestamp := uint64(time.Now().UnixNano() / 1e6) // время в мс
	encoded := ""
	base := uint64(len(base62Chars))

	for timestamp > 0 && len(encoded) < maxLength {
		remainder := timestamp % base
		encoded = string(base62Chars[remainder]) + encoded
		timestamp = timestamp / base
	}

	rand.Seed(time.Now().UnixNano())
	for len(encoded) < maxLength {
		encoded += string(base62Chars[rand.Intn(len(base62Chars))])
	}

	return encoded
}

// convertRhToSv и convertSvToRh - вспомогательные функции перевода
func convertRhToSv(markers []database.Marker) []database.Marker {
	filteredMarkers := []database.Marker{}
	const conversionFactor = 0.01 // 1 Rh = 0.01 Sv

	for _, newMarker := range markers {
		newMarker.DoseRate = newMarker.DoseRate * conversionFactor
		filteredMarkers = append(filteredMarkers, newMarker)
	}
	return filteredMarkers
}

// filterZeroMarkers убирает маркеры с нулевым значением дозы
func filterZeroMarkers(markers []database.Marker) []database.Marker {
	filteredMarkers := []database.Marker{}
	for _, m := range markers {
		if m.DoseRate == 0 {
			continue
		}
		filteredMarkers = append(filteredMarkers, m)
	}
	return filteredMarkers
}

// NEW ────────────────
func isValidDate(ts int64) bool {
	// допустимо «сегодня плюс сутки» с учётом часовых поясов
	max := time.Now().Add(24 * time.Hour).Unix()
	return ts >= minValidTS && ts <= max
}

func filterInvalidDateMarkers(markers []database.Marker) []database.Marker {
	out := markers[:0]
	for _, m := range markers {
		if isValidDate(m.Date) {
			out = append(out, m)
		}
	}
	return out
}

// Проекция Web Mercator приблизительно переводит широту/долготу в "метры".
// Формулы стандартные, здесь используется для перевода в пиксельные координаты.
func latLonToWebMercator(lat, lon float64) (x, y float64) {
	// const радиус Земли для WebMercator
	const originShift = 2.0 * math.Pi * 6378137.0 / 2.0

	x = lon * originShift / 180.0
	y = math.Log(math.Tan((90.0+lat)*math.Pi/360.0)) / (math.Pi / 180.0)
	y = y * originShift / 180.0
	return x, y
}

// webMercatorToPixel переводит Web Mercator координаты (x,y) в пиксели на данном зуме.
func webMercatorToPixel(x, y float64, zoom int) (px, py float64) {
	// тайл 256x256, увеличиваем в 2^zoom
	scale := math.Exp2(float64(zoom))
	px = (x + 2.0*math.Pi*6378137.0/2.0) / (2.0 * math.Pi * 6378137.0) * 256.0 * scale
	py = (2.0*math.Pi*6378137.0/2.0 - y) / (2.0 * math.Pi * 6378137.0) * 256.0 * scale
	return
}

// latLonToPixel - удобная обёртка
func latLonToPixel(lat, lon float64, zoom int) (px, py float64) {
	x, y := latLonToWebMercator(lat, lon)
	return webMercatorToPixel(x, y, zoom)
}

// fastMergeMarkersByZoom группирует маркеры в «ячейку» сетки
// (диаметр = 2*radiusPx) и усредняет данные кластера.
// • O(N) • без мьютексов • подходит для любых зумов.
func fastMergeMarkersByZoom(markers []database.Marker, zoom int, radiusPx float64) []database.Marker {
	if len(markers) == 0 {
		return nil
	}

	cell := 2*radiusPx + 1 // px
	type acc struct {
		sumLat, sumLon, sumDose, sumCnt, sumSp float64
		sumAlt, sumTemp, sumHum                float64
		altCount, tempCount, humCount          int
		detector, radiation                    string
		latest                                 int64
		n                                      int
	}
	cl := make(map[int64]*acc) // key := cx<<32 | cy

	for _, m := range markers {
		px, py := latLonToPixel(m.Lat, m.Lon, zoom)
		key := int64(int(px/cell))<<32 | int64(int32(py/cell))
		a := cl[key]
		if a == nil {
			a = &acc{}
			cl[key] = a
		}
		a.sumLat += m.Lat
		a.sumLon += m.Lon
		a.sumDose += m.DoseRate
		a.sumCnt += m.CountRate
		a.sumSp += m.Speed
		if m.AltitudeValid {
			a.sumAlt += m.Altitude
			a.altCount++
		}
		if m.TemperatureValid {
			a.sumTemp += m.Temperature
			a.tempCount++
		}
		if m.HumidityValid {
			a.sumHum += m.Humidity
			a.humCount++
		}
		if m.Date > a.latest {
			a.latest = m.Date
		}
		if a.detector == "" && m.Detector != "" {
			a.detector = m.Detector
		}
		if a.radiation == "" && m.Radiation != "" {
			a.radiation = m.Radiation
		}
		a.n++
	}

	out := make([]database.Marker, 0, len(cl))
	for _, c := range cl {
		n := float64(c.n)
		var (
			altitude float64
			temp     float64
			hum      float64
		)
		var (
			altValid  bool
			tempValid bool
			humValid  bool
		)
		if c.altCount > 0 {
			altitude = c.sumAlt / float64(c.altCount)
			altValid = true
		}
		if c.tempCount > 0 {
			temp = c.sumTemp / float64(c.tempCount)
			tempValid = true
		}
		if c.humCount > 0 {
			hum = c.sumHum / float64(c.humCount)
			humValid = true
		}
		out = append(out, database.Marker{
			Lat:              c.sumLat / n,
			Lon:              c.sumLon / n,
			DoseRate:         c.sumDose / n,
			CountRate:        c.sumCnt / n,
			Speed:            c.sumSp / n,
			Altitude:         altitude,
			Temperature:      temp,
			Humidity:         hum,
			Detector:         c.detector,
			Radiation:        c.radiation,
			Date:             c.latest,
			Zoom:             zoom,
			TrackID:          markers[0].TrackID,
			AltitudeValid:    altValid,
			TemperatureValid: tempValid,
			HumidityValid:    humValid,
		})
	}
	return out
}

// mergeMarkersByZoom “сливает” (усредняет) маркеры, которые пересекаются в пиксельных координатах
// на текущем зуме. Если расстояние между центрами меньше 2*markerRadiusPx (плюс 1px “запас”), то объединяем.
// deprecated
func mergeMarkersByZoom(markers []database.Marker, zoom int, radiusPx float64) []database.Marker {
	if len(markers) == 0 {
		return nil
	}

	// Сначала готовим структуру с пиксельными координатами
	type markerPixel struct {
		Marker    database.Marker
		Px, Py    float64
		MergedIdx int // -1, если ни с кем ещё не сливался
	}

	mPixels := make([]markerPixel, len(markers))
	for i, m := range markers {
		px, py := latLonToPixel(m.Lat, m.Lon, zoom)
		mPixels[i] = markerPixel{
			Marker:    m,
			Px:        px,
			Py:        py,
			MergedIdx: -1,
		}
	}

	var result []database.Marker

	// Жадно идём по списку, сливаем близкие друг к другу
	for i := 0; i < len(mPixels); i++ {
		if mPixels[i].MergedIdx != -1 {
			// уже слит с кем-то
			continue
		}
		// начинаем новый кластер
		cluster := []markerPixel{mPixels[i]}
		mPixels[i].MergedIdx = i

		// проверяем всех последующих
		for j := i + 1; j < len(mPixels); j++ {
			if mPixels[j].MergedIdx != -1 {
				continue
			}
			dist := math.Hypot(mPixels[i].Px-mPixels[j].Px, mPixels[i].Py-mPixels[j].Py)
			if dist < 2.0*radiusPx {
				// Сливаем
				cluster = append(cluster, mPixels[j])
				mPixels[j].MergedIdx = i // значит, слит к кластеру i
			}
		}

		// Усредняем данные кластера
		var sumLat, sumLon, sumDose, sumCount float64
		var sumAlt, sumTemp, sumHum float64
		var altCount, tempCount, humCount int
		var latestDate int64
		detector := ""
		radiation := ""
		for _, c := range cluster {
			sumLat += c.Marker.Lat
			sumLon += c.Marker.Lon
			sumDose += c.Marker.DoseRate
			sumCount += c.Marker.CountRate
			if c.Marker.AltitudeValid {
				sumAlt += c.Marker.Altitude
				altCount++
			}
			if c.Marker.TemperatureValid {
				sumTemp += c.Marker.Temperature
				tempCount++
			}
			if c.Marker.HumidityValid {
				sumHum += c.Marker.Humidity
				humCount++
			}
			if detector == "" && c.Marker.Detector != "" {
				detector = c.Marker.Detector
			}
			if radiation == "" && c.Marker.Radiation != "" {
				radiation = c.Marker.Radiation
			}
			// возьмём дату последнего
			if c.Marker.Date > latestDate {
				latestDate = c.Marker.Date
			}
		}
		n := float64(len(cluster))
		avgLat := sumLat / n
		avgLon := sumLon / n
		avgDose := sumDose / n
		avgCount := sumCount / n

		var sumSpeed float64
		for _, c := range cluster {
			sumSpeed += c.Marker.Speed
		}
		avgSpeed := sumSpeed / n

		var altitude float64
		var temp float64
		var hum float64
		var altValid bool
		var tempValid bool
		var humValid bool
		if altCount > 0 {
			altitude = sumAlt / float64(altCount)
			altValid = true
		}
		if tempCount > 0 {
			temp = sumTemp / float64(tempCount)
			tempValid = true
		}
		if humCount > 0 {
			hum = sumHum / float64(humCount)
			humValid = true
		}

		// Создаём новый слитый маркер
		newMarker := database.Marker{
			Lat:              avgLat,
			Lon:              avgLon,
			DoseRate:         avgDose,
			CountRate:        avgCount,
			Altitude:         altitude,
			Temperature:      temp,
			Humidity:         hum,
			Detector:         detector,
			Radiation:        radiation,
			Date:             latestDate,
			Speed:            avgSpeed,
			Zoom:             zoom,
			TrackID:          cluster[0].Marker.TrackID, // берем хотя бы у первого
			AltitudeValid:    altValid,
			TemperatureValid: tempValid,
			HumidityValid:    humValid,
		}
		result = append(result, newMarker)
	}

	return result
}

// pickIdentityProbe returns up to 'limit' evenly spaced, non-zero markers
// to cheaply "probe" the DB for an existing track. This avoids thousands
// of random point-lookups on huge tables.
// • No mutexes: pure functional slice logic.
// • Streaming friendly: does not allocate more than needed.
func pickIdentityProbe(src []database.Marker, limit int) []database.Marker {
	if limit <= 0 || len(src) == 0 {
		return nil
	}
	// 1) filter out zero-dose points (they are common and uninformative)
	tmp := make([]database.Marker, 0, min(len(src), limit*2))
	for _, m := range src {
		if m.DoseRate != 0 || m.CountRate != 0 {
			tmp = append(tmp, m)
		}
	}
	if len(tmp) == 0 {
		// fall back to original src if everything was zero
		tmp = src
	}
	// 2) take evenly spaced sample up to 'limit'
	n := len(tmp)
	if n <= limit {
		out := make([]database.Marker, n)
		copy(out, tmp)
		return out
	}
	out := make([]database.Marker, 0, limit)
	stride := n / limit
	if stride <= 0 {
		stride = 1
	}
	for i := 0; i < n && len(out) < limit; i += stride {
		out = append(out, tmp[i])
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// calculateSpeedForMarkers fills Marker.Speed (m/s) for **every** point.
//
// Algorithm
// =========
//  1. Sort markers chronologically (ascending Unix time).
//  2. Pairwise speed: for each neighbour pair with Δt>0 compute v=d/Δt; this
//     gives нам хотя бы несколько валидных скоростей.
//  3. Gap-filling:  for любой непрерывной серии Speed==0
//     ┌ both neighbours exist ─► take avg speed between them
//     ├ only left neighbour    ─► copy its speed
//     └ only right neighbour   ─► copy its speed
//     All markers inside the gap receive the chosen value.
//  4. Fallback: if, after step 3, some zeros still remain (whole track had
//     no Δt>0 inside), we compute average speed between the *first* and the
//     *last* marker and assign it to everyone.
//
// Complexity O(N), lock-free — the slice is owned by this goroutine.
//
// Limits: speeds outside 0…1000 m/s (≈0…3600 km/h) are considered glitches
//
//	and ignored while calculating new values.
//
// calculateSpeedForMarkers recomputes Marker.Speed (m/s) for the whole slice,
// ignoring any pre-filled speeds in the input and auto-normalizing timestamp
// units (milliseconds vs seconds). We keep the function single-pass friendly
// and deterministic, no locks needed (slice is owned by the caller).
//
// Why this change:
//   - Some tracks (e.g., 666.json) carry Unix time in milliseconds, which,
//     if treated as seconds, yields near-zero speeds. We detect ms and divide by 1000.
//   - We never trust/keep incoming Speed values: always recompute from distance/time.
//   - We preserve previous aviation parsing behavior for tracks already in seconds.
//
// Complexity: O(N).
// calculateSpeedForMarkers recomputes Speed (m/s) for all markers,
// normalizing timestamp units (ms → s when needed). We ignore any
// prefilled speeds and derive velocity from geodesic distance / Δt.
//
// Design notes (Go proverbs minded):
//   - Simplicity: single pass with small helpers.
//   - Determinism: slice is owned by caller; no locks, no shared state.
//   - Robustness: auto-detect ms vs s by checking epoch magnitude.
//
// Complexity: O(N).
func calculateSpeedForMarkers(markers []database.Marker) []database.Marker {
	if len(markers) == 0 {
		return markers
	}

	// 1) Chronological order to keep Δt positive and stable.
	sort.Slice(markers, func(i, j int) bool { return markers[i].Date < markers[j].Date })

	// 2) Decide the epoch units once per track:
	//    ~1e9 → seconds (Unix s), ~1e12 → milliseconds (Unix ms).
	//    Check both ends to be safe with mixed sources.
	scale := 1.0 // seconds by default
	if markers[0].Date > 1_000_000_000_000 || markers[len(markers)-1].Date > 1_000_000_000_000 {
		scale = 1000.0 // timestamps are in ms → convert Δt to seconds
	}

	// helper to get Δt in seconds
	dtSec := func(prev, curr int64) float64 {
		if curr <= prev {
			return 0
		}
		return float64(curr-prev) / scale
	}

	const maxSpeed = 1000.0 // m/s, sanity cap for aircraft

	// 3) Recompute pairwise speeds from distance / Δt.
	for i := 1; i < len(markers); i++ {
		dt := dtSec(markers[i-1].Date, markers[i].Date)
		if dt <= 0 {
			continue // duplicate or invalid timestamp
		}
		dist := haversineDistance(
			markers[i-1].Lat, markers[i-1].Lon,
			markers[i].Lat, markers[i].Lon,
		)
		v := dist / dt // m/s
		if v >= 0 && v <= maxSpeed {
			markers[i].Speed = v
		} else {
			// Leave zero if insane (spikes/outliers)
			markers[i].Speed = 0
		}
	}

	// 4) Seed the very first point if needed.
	if len(markers) > 1 && markers[0].Speed == 0 {
		markers[0].Speed = markers[1].Speed
	}

	// 5) Fill zero-speed gaps by borrowing from neighbours.
	lastWithSpeed := -1
	for i := 0; i < len(markers); {
		if markers[i].Speed > 0 {
			lastWithSpeed = i
			i++
			continue
		}
		// zero-run [gapStart..gapEnd]
		gapStart := i
		for i < len(markers) && markers[i].Speed == 0 {
			i++
		}
		gapEnd := i - 1

		// right anchor (if any)
		nextWithSpeed := -1
		if i < len(markers) && markers[i].Speed > 0 {
			nextWithSpeed = i
		}

		var fill float64
		switch {
		case lastWithSpeed != -1 && nextWithSpeed != -1:
			// Prefer average speed derived from anchors distance/time.
			dt := dtSec(markers[lastWithSpeed].Date, markers[nextWithSpeed].Date)
			if dt > 0 {
				dist := haversineDistance(
					markers[lastWithSpeed].Lat, markers[lastWithSpeed].Lon,
					markers[nextWithSpeed].Lat, markers[nextWithSpeed].Lon,
				)
				fill = dist / dt
			}
		case lastWithSpeed != -1:
			fill = markers[lastWithSpeed].Speed
		case nextWithSpeed != -1:
			fill = markers[nextWithSpeed].Speed
		}

		if fill > 0 && fill <= maxSpeed {
			for j := gapStart; j <= gapEnd; j++ {
				markers[j].Speed = fill
			}
		}
	}

	// 6) Global fallback: if anything is still zero, use total distance / total time.
	needFallback := false
	for _, m := range markers {
		if m.Speed == 0 {
			needFallback = true
			break
		}
	}
	if needFallback && len(markers) >= 2 {
		totalDt := dtSec(markers[0].Date, markers[len(markers)-1].Date)
		if totalDt > 0 {
			dist := haversineDistance(
				markers[0].Lat, markers[0].Lon,
				markers[len(markers)-1].Lat, markers[len(markers)-1].Lon,
			)
			v := dist / totalDt
			if v >= 0 && v <= maxSpeed {
				for k := range markers {
					if markers[k].Speed == 0 {
						markers[k].Speed = v
					}
				}
			}
		}
	}

	return markers
}

func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000
	phi1, phi2 := lat1*math.Pi/180, lat2*math.Pi/180
	dPhi, dLambda := (lat2-lat1)*math.Pi/180, (lon2-lon1)*math.Pi/180
	a := math.Sin(dPhi/2)*math.Sin(dPhi/2) + math.Cos(phi1)*math.Cos(phi2)*math.Sin(dLambda/2)*math.Sin(dLambda/2)
	return 2 * R * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func radiusForZoom(zoom int) float64 {
	// линейная шкала: z=20 → 10 px, z=10 → 5 px, z=5 → 2.5 px …
	return markerRadiusPx * float64(zoom) / 20.0
}

// precomputeMarkersForAllZoomLevels создаёт агрегаты для z=1…20
// Параллельно: для каждого зума — своя goroutine, сбор через канал.
func precomputeMarkersForAllZoomLevels(src []database.Marker) []database.Marker {
	type job struct {
		z   int
		out []database.Marker
	}
	ch := make(chan job, 20)

	for z := 1; z <= 20; z++ {
		go func(zoom int) {
			merged := fastMergeMarkersByZoom(src, zoom, radiusForZoom(zoom))
			ch <- job{z: zoom, out: merged}
		}(z)
	}

	var res []database.Marker
	for i := 0; i < 20; i++ {
		res = append(res, (<-ch).out...)
	}
	return res
}

// =====================
// Транслейт
// =====================
var translations map[string]map[string]string

func loadTranslations(fs embed.FS, filename string) {
	file, err := fs.Open(filename)
	if err != nil {
		log.Fatalf("Error opening translation file: %v", err)
	}
	defer file.Close()

	data, err := ioutil.ReadAll(file)
	if err != nil {
		log.Fatalf("Error reading translation file: %v", err)
	}

	err = json.Unmarshal(data, &translations)
	if err != nil {
		log.Fatalf("Error parsing translations: %v", err)
	}
}

func getPreferredLanguage(r *http.Request) string {
	langHeader := r.Header.Get("Accept-Language")
	if langHeader == "" {
		return "en"
	}

	// Поддерживаемые языки (добавлены: da, fa)
	supported := map[string]struct{}{
		"en": {}, "zh": {}, "es": {}, "hi": {}, "ar": {}, "fr": {}, "ru": {}, "pt": {}, "de": {}, "ja": {}, "tr": {}, "it": {},
		"ko": {}, "pl": {}, "uk": {}, "mn": {}, "no": {}, "fi": {}, "ka": {}, "sv": {}, "he": {}, "nl": {}, "el": {}, "hu": {},
		"cs": {}, "ro": {}, "th": {}, "vi": {}, "id": {}, "ms": {}, "bg": {}, "lt": {}, "et": {}, "lv": {}, "sl": {},
		"da": {}, "fa": {},
	}

	// Нормализация/синонимы: приводим варианты к поддерживаемым базовым кодам
	aliases := map[string]string{
		// Устаревшие коды
		"iw": "he", // he (Hebrew)
		"in": "id", // id (Indonesian)

		// Норвежский: часто приходит nb-NO/nn-NO
		"nb": "no",
		"nn": "no",

		// Китайский: сводим к "zh"
		"zh-cn":   "zh",
		"zh-sg":   "zh",
		"zh-hans": "zh",
		"zh-tw":   "zh",
		"zh-hk":   "zh",
		"zh-hant": "zh",

		// Португальский варианты → "pt"
		"pt-br": "pt",
		"pt-pt": "pt",
	}

	langs := strings.Split(langHeader, ",")
	for _, raw := range langs {
		code := strings.TrimSpace(strings.SplitN(raw, ";", 2)[0])
		code = strings.ToLower(strings.ReplaceAll(code, "_", "-"))

		// Берём базовую часть до дефиса (например, "de" из "de-DE")
		base := code
		if i := strings.Index(code, "-"); i != -1 {
			base = code[:i]
		}

		// Применяем алиасы (и к полному коду, и к базе)
		if a, ok := aliases[code]; ok {
			base = a
		} else if a, ok := aliases[base]; ok {
			base = a
		}

		// Проверяем поддержку
		if _, ok := supported[base]; ok {
			return base
		}
	}

	return "en"
}

// =====================
// Парсинг файлов
// =====================
func parseFloat(value string) float64 {
	parsedValue, _ := strconv.ParseFloat(value, 64)
	return parsedValue
}

func getTimeZoneByLongitude(lon float64) *time.Location {
	switch {
	case lon >= -10 && lon <= 0:
		loc, _ := time.LoadLocation("Europe/London")
		return loc
	case lon > 0 && lon <= 15:
		loc, _ := time.LoadLocation("Europe/Berlin")
		return loc
	case lon > 15 && lon <= 30:
		loc, _ := time.LoadLocation("Europe/Kiev")
		return loc
	case lon > 30 && lon <= 45:
		loc, _ := time.LoadLocation("Europe/Moscow")
		return loc
	case lon > 45 && lon <= 60:
		loc, _ := time.LoadLocation("Asia/Yekaterinburg")
		return loc
	case lon > 60 && lon <= 90:
		loc, _ := time.LoadLocation("Asia/Novosibirsk")
		return loc
	case lon > 90 && lon <= 120:
		loc, _ := time.LoadLocation("Asia/Irkutsk")
		return loc
	case lon > 120 && lon <= 135:
		loc, _ := time.LoadLocation("Asia/Yakutsk")
		return loc
	case lon > 135 && lon <= 180:
		loc, _ := time.LoadLocation("Asia/Vladivostok")
		return loc

	case lon >= -180 && lon < -150:
		loc, _ := time.LoadLocation("America/Anchorage")
		return loc
	case lon >= -150 && lon < -120:
		loc, _ := time.LoadLocation("America/Los_Angeles")
		return loc
	case lon >= -120 && lon < -90:
		loc, _ := time.LoadLocation("America/Denver")
		return loc
	case lon >= -90 && lon < -60:
		loc, _ := time.LoadLocation("America/Chicago")
		return loc
	case lon >= -60 && lon < -30:
		loc, _ := time.LoadLocation("America/New_York")
		return loc
	case lon >= -30 && lon < 0:
		loc, _ := time.LoadLocation("America/Halifax")
		return loc

	case lon >= 60 && lon < 75:
		loc, _ := time.LoadLocation("Asia/Karachi")
		return loc
	case lon >= 75 && lon < 90:
		loc, _ := time.LoadLocation("Asia/Kolkata")
		return loc
	case lon >= 90 && lon < 105:
		loc, _ := time.LoadLocation("Asia/Dhaka")
		return loc
	case lon >= 105 && lon < 120:
		loc, _ := time.LoadLocation("Asia/Bangkok")
		return loc
	case lon >= 120 && lon < 135:
		loc, _ := time.LoadLocation("Asia/Shanghai")
		return loc
	case lon >= 135 && lon < 150:
		loc, _ := time.LoadLocation("Asia/Tokyo")
		return loc
	case lon >= 150 && lon <= 180:
		loc, _ := time.LoadLocation("Australia/Sydney")
		return loc

	default:
		loc, _ := time.LoadLocation("UTC")
		return loc
	}
}

// -----------------------------------------------------------------------------
// extractDoseRate — extracts the dose rate from an arbitrary text fragment.
//
//   - «12.3 µR/h»  → 0.123 µSv/h      (1 µR/h ≈ 0.01 µSv/h, legacy iPhone dump)
//   - «0.136 uSv/h»→ 0.136 µSv/h      (Safecast)
//   - «0.29 мкЗв/ч»→ 0.29  µSv/h      (Radiacode-101 Android, RU locale)
//
// -----------------------------------------------------------------------------
func extractDoseRate(s string) float64 {
	// block: legacy µR/h → µSv/h
	reMicroRh := regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*µ?R/h`)
	if m := reMicroRh.FindStringSubmatch(s); len(m) > 0 {
		return parseFloat(m[1]) / 100.0 // convert µR/h → µSv/h
	}

	// block: standard uSv/h (Safecast, iPhone)
	reMicroSv := regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*uSv/h`)
	if m := reMicroSv.FindStringSubmatch(s); len(m) > 0 {
		return parseFloat(m[1]) // already in µSv/h
	}

	// block: russian «мкЗв/ч» (μSv/h in Cyrillic)
	reRuMicroSv := regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*мк?з?в/ч`)
	if m := reRuMicroSv.FindStringSubmatch(s); len(m) > 0 {
		return parseFloat(m[1]) // already in µSv/h
	}
	return 0
}

// -----------------------------------------------------------------------------
// extractCountRate — searches for count rate and normalises it to cps.
//
//   - «24 cps»      → 24
//   - «1500 CPM»    → 25  (1 min → sec)
//   - «24.7 имп/c»  → 24.7 (Radiacode-101 Android, RU locale)
//
// -----------------------------------------------------------------------------
func extractCountRate(s string) float64 {
	// block: cps (all locales)
	reCPS := regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*cps`)
	if m := reCPS.FindStringSubmatch(s); len(m) > 0 {
		return parseFloat(m[1])
	}

	// block: CPM (Safecast CSV)
	reCPM := regexp.MustCompile(`(?i)CPM\s*Value\s*=\s*(\d+(?:\.\d+)?)`)
	if m := reCPM.FindStringSubmatch(s); len(m) > 0 {
		return parseFloat(m[1]) / 60.0 // 1 minute → seconds
	}

	// block: russian «имп/с» or «имп/c» (cyrillic / latin 'c')
	reRU := regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*имп\s*/\s*[cс]`)
	if m := reRU.FindStringSubmatch(s); len(m) > 0 {
		return parseFloat(m[1])
	}
	return 0
}

// -----------------------------------------------------------------------------
// parseDate — recognises three date formats:
//
//   - «May 23, 2012 04:10:08»      (old .rctrk / AtomFast KML)
//   - «2012/05/23 04:10:08»        (Safecast)
//   - «26 июл 2025 11:29:54»       (Radiacode-101 Android, RU locale)
//
// loc — time-zone calculated from longitude (nil → UTC).
// -----------------------------------------------------------------------------
func parseDate(s string, loc *time.Location) int64 {
	if loc == nil {
		loc = time.UTC
	}

	// block: English «Jan 2, 2006 …»
	if m := regexp.MustCompile(`([A-Za-z]{3} \d{1,2}, \d{4} \d{2}:\d{2}:\d{2})`).FindStringSubmatch(s); len(m) > 0 {
		const layout = "Jan 2, 2006 15:04:05"
		if t, err := time.ParseInLocation(layout, m[1], loc); err == nil {
			return t.Unix()
		}
	}

	// block: ISO-ish «2006/01/02 …»
	if m := regexp.MustCompile(`(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2})`).FindStringSubmatch(s); len(m) > 0 {
		const layout = "2006/01/02 15:04:05"
		if t, err := time.ParseInLocation(layout, m[1], loc); err == nil {
			return t.Unix()
		}
	}

	// block: Russian «02 янв 2006 …»
	reRu := regexp.MustCompile(`(\d{1,2})\s+([А-Яа-я]{3})\s+(\d{4})\s+(\d{2}:\d{2}:\d{2})`)
	if m := reRu.FindStringSubmatch(s); len(m) > 0 {
		// map short russian month → number
		ruMon := map[string]string{
			"янв": "01", "фев": "02", "мар": "03", "апр": "04",
			"май": "05", "июн": "06", "июл": "07", "авг": "08",
			"сен": "09", "окт": "10", "ноя": "11", "дек": "12",
		}
		monNum, ok := ruMon[strings.ToLower(m[2])]
		if !ok {
			return 0
		}

		// build ISO-like string and parse
		dateStr := fmt.Sprintf("%s-%s-%02s %s", m[3], monNum, m[1], m[4])
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", dateStr, loc); err == nil {
			return t.Unix()
		}
	}
	return 0
}

// =====================================================================================
// parseGPX.go — потоковый парсер GPX 1.1 (AtomSwift)
// =====================================================================================
//
// Формат AtomSwift:
//
//	<trkpt lat="…" lon="…">
//	  <time>2025-04-19T14:57:46Z</time>
//	  …
//	  <extensions>
//	    <atom:marker>
//	       <atom:doserate>0.018526316</atom:doserate>  <!-- µSv/h -->
//	       <atom:cp2s>1.0</atom:cp2s>                   <!-- counts / 2 s -->
//	       <atom:speed>0.41898388</atom:speed>          <!-- m/s -->
//	    </atom:marker>
//	  </extensions>
//
// Все интересующие поля находятся внутри <trkpt>.  Парсим потоково без
// дополнительного выделения памяти, никаких mutex – только канал результатов
// внутри ф-ции (go-routine → main goroutine).
//
// =====================================================================================
// parseGPX (stream) — token-driven GPX 1.1 parser (AtomSwift).
// Uses xml.Decoder directly on io.Reader, so we do *zero* extra allocations.
// =====================================================================================
func parseGPX(trackID string, r io.Reader) ([]database.Marker, error) {
	logT(trackID, "GPX", "parser start (stream)")

	type result struct {
		marker database.Marker
		err    error
	}

	out := make(chan result)
	go func() { // parser goroutine
		defer close(out)

		dec := xml.NewDecoder(r)
		var (
			inTrkpt       bool
			lat, lon      float64
			tUnix, doseSv float64
			count, speed  float64
		)

		for {
			tok, err := dec.Token()
			if err == io.EOF {
				return
			}
			if err != nil {
				out <- result{err: fmt.Errorf("XML decode: %w", err)}
				return
			}

			switch el := tok.(type) {
			case xml.StartElement:
				switch el.Name.Local {
				case "trkpt":
					inTrkpt = true
					lat, lon, tUnix, doseSv, count, speed = 0, 0, 0, 0, 0, 0
					for _, a := range el.Attr {
						if a.Name.Local == "lat" {
							lat = parseFloat(a.Value)
						} else if a.Name.Local == "lon" {
							lon = parseFloat(a.Value)
						}
					}
				case "time":
					if inTrkpt {
						var ts string
						_ = dec.DecodeElement(&ts, &el)
						if tt, err := time.Parse(time.RFC3339, ts); err == nil {
							tUnix = float64(tt.Unix())
						}
					}
				case "doserate":
					if inTrkpt {
						var s string
						_ = dec.DecodeElement(&s, &el)
						doseSv = parseFloat(s)
					}
				case "cp2s":
					if inTrkpt {
						var s string
						_ = dec.DecodeElement(&s, &el)
						count = parseFloat(s) / 2.0
					}
				case "speed":
					if inTrkpt {
						var s string
						_ = dec.DecodeElement(&s, &el)
						speed = parseFloat(s)
					}
				}
			case xml.EndElement:
				if el.Name.Local == "trkpt" && inTrkpt {
					inTrkpt = false
					if doseSv == 0 && count == 0 {
						continue
					}
					out <- result{marker: database.Marker{
						Lat:       lat,
						Lon:       lon,
						Date:      int64(tUnix),
						DoseRate:  doseSv,
						CountRate: count,
						Speed:     speed,
					}}
				}
			}
		}
	}()

	var markers []database.Marker
	for r := range out {
		if r.err != nil {
			logT(trackID, "GPX", "✖ %v", r.err)
			return nil, r.err
		}
		markers = append(markers, r.marker)
	}
	logT(trackID, "GPX", "parser done, parsed=%d markers", len(markers))
	if len(markers) == 0 {
		return nil, fmt.Errorf("no <trkpt> with numeric data found")
	}
	return markers, nil
}

// parseKML (stream) — SAX-style KML parser with *constant* time-zone
// for the whole file.  Fixes wrong speeds on tracks that cross
// several time-zones (e.g. airplanes).
func parseKML(trackID string, r io.Reader) ([]database.Marker, error) {
	logT(trackID, "KML", "parser start (stream)")

	dec := xml.NewDecoder(r)

	var (
		inPlacemark bool
		lat, lon    float64
		name, desc  string
		markers     []database.Marker
		tz          *time.Location // ← NEW: chosen once
		tzLocked    bool           // ←   and then locked
	)

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("XML decode: %v", err)
		}

		switch el := tok.(type) {
		case xml.StartElement:
			switch el.Name.Local {
			case "Placemark":
				inPlacemark, lat, lon, name, desc = true, 0, 0, "", ""
			case "name":
				if inPlacemark {
					_ = dec.DecodeElement(&name, &el)
				}
			case "description":
				if inPlacemark {
					_ = dec.DecodeElement(&desc, &el)
				}
			case "coordinates":
				if inPlacemark {
					var coord string
					_ = dec.DecodeElement(&coord, &el)
					parts := strings.Split(coord, ",")
					if len(parts) >= 2 {
						lon = parseFloat(parts[0])
						lat = parseFloat(parts[1])
					}
					// ── выбираем TZ только *один раз* ─────────────
					if !tzLocked {
						tz = getTimeZoneByLongitude(lon)
						tzLocked = true
					}
				}
			}
		case xml.EndElement:
			if el.Name.Local == "Placemark" && inPlacemark {
				inPlacemark = false
				dose := extractDoseRate(name)
				if dose == 0 {
					dose = extractDoseRate(desc)
				}
				count := extractCountRate(desc)
				date := parseDate(desc, tz) // ← используем ЕДИНЫЙ TZ
				if dose == 0 && count == 0 {
					continue
				}
				markers = append(markers, database.Marker{
					DoseRate:  dose,
					CountRate: count,
					Lat:       lat,
					Lon:       lon,
					Date:      date,
				})
			}
		}
	}

	logT(trackID, "KML", "parser done, parsed=%d markers", len(markers))
	if len(markers) == 0 {
		return nil, fmt.Errorf("no valid <Placemark> with numeric data found")
	}
	return markers, nil
}

// =====================================================================================
// parseTextRCTRK.go  — теперь принимает trackID
// =====================================================================================
func parseTextRCTRK(trackID string, data []byte) ([]database.Marker, error) {
	logT(trackID, "RCTRK", "text parser start")

	var markers []database.Marker
	lines := strings.Split(string(data), "\n")

	for idx, line := range lines {
		if idx == 0 || strings.HasPrefix(line, "Timestamp") || strings.TrimSpace(line) == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 8 {
			logT(trackID, "RCTRK", "skip line %d: insufficient fields (%d)", idx+1, len(fields))
			continue
		}

		tsStr := fields[1] + " " + fields[2]
		t, err := time.Parse("2006-01-02 15:04:05", tsStr)
		if err != nil {
			logT(trackID, "RCTRK", "skip line %d: time parse error: %v", idx+1, err)
			continue
		}

		lat, lon := parseFloat(fields[3]), parseFloat(fields[4])
		if lat == 0 || lon == 0 {
			logT(trackID, "RCTRK", "skip line %d: invalid coords (%.6f,%.6f)", idx+1, lat, lon)
			continue
		}

		doseRaw, countRaw := parseFloat(fields[6]), parseFloat(fields[7])
		if doseRaw < 0 || countRaw < 0 {
			logT(trackID, "RCTRK", "skip line %d: negative dose/count", idx+1)
			continue
		}

		markers = append(markers, database.Marker{
			DoseRate:  doseRaw / 100.0,
			CountRate: countRaw,
			Lat:       lat,
			Lon:       lon,
			Date:      t.Unix(),
		})
	}

	logT(trackID, "RCTRK", "text parser done, parsed=%d markers", len(markers))
	return markers, nil
}

// =====================================================================================
// parseAtomSwiftCSV (stream) — parses huge .csv produced by AtomSwift logger
// fast & memory-friendly: no ReadAll(), we read record-by-record through bufio.Reader.
// =====================================================================================
func parseAtomSwiftCSV(trackID string, r io.Reader) ([]database.Marker, error) {
	logT(trackID, "CSV", "parser start (stream)")

	br := bufio.NewReaderSize(r, 512*1024) // 512 KiB read-ahead buffer
	cr := csv.NewReader(br)
	cr.Comma = ';'
	cr.FieldsPerRecord = -1 // keep tolerant

	// skip header -----------------------------------------------------------
	if _, err := cr.Read(); err != nil {
		return nil, fmt.Errorf("CSV header: %v", err)
	}

	markers := make([]database.Marker, 0, 4096) // pre-allocate reasonable cap
	rowN := 1
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			logT(trackID, "CSV", "row %d: %v", rowN+1, err)
			continue
		}
		rowN++

		if len(rec) < 7 {
			logT(trackID, "CSV", "skip row %d: insufficient fields (%d)", rowN, len(rec))
			continue
		}

		ts, err := strconv.ParseInt(strings.TrimSpace(rec[0]), 10, 64)
		if err != nil {
			logT(trackID, "CSV", "skip row %d: bad timestamp", rowN)
			continue
		}

		dose := parseFloat(rec[1]) // µSv/h
		lat := parseFloat(rec[2])
		lon := parseFloat(rec[3])
		speed := parseFloat(rec[5]) // m/s
		cps := parseFloat(rec[6])

		if lat == 0 || lon == 0 || dose == 0 {
			continue
		}

		markers = append(markers, database.Marker{
			DoseRate:  dose,
			CountRate: cps,
			Lat:       lat,
			Lon:       lon,
			Date:      ts,
			Speed:     speed,
		})
	}

	if len(markers) == 0 {
		return nil, fmt.Errorf("no valid data rows found")
	}
	logT(trackID, "CSV", "parser done, parsed=%d markers", len(markers))
	return markers, nil
}

// processAtomSwiftCSVFile handles *.csv uploads from AtomSwift logger.
func processAtomSwiftCSVFile(
	file multipart.File,
	trackID string,
	db *database.Database,
	dbType string,
) (database.Bounds, string, error) {

	logT(trackID, "CSV", "▶ start (stream)")

	markers, err := parseAtomSwiftCSV(trackID, file)
	if err != nil {
		return database.Bounds{}, trackID, fmt.Errorf("parse CSV: %w", err)
	}
	logT(trackID, "CSV", "parsed %d markers", len(markers))

	bbox, trackID, err := processAndStoreMarkers(markers, trackID, db, dbType)
	if err != nil {
		return bbox, trackID, err
	}
	logT(trackID, "BGEIGIE", "✔ done")
	return bbox, trackID, nil
}

// processGPXFile handles plain *.gpx uploads in streaming mode.
func processGPXFile(
	file multipart.File,
	trackID string,
	db *database.Database,
	dbType string,
) (database.Bounds, string, error) {
	logT(trackID, "GPX", "▶ start (stream)")

	markers, err := parseGPX(trackID, file)
	if err != nil {
		return database.Bounds{}, trackID, fmt.Errorf("parse GPX: %w", err)
	}
	logT(trackID, "GPX", "parsed %d markers", len(markers))

	bbox, trackID, err := processAndStoreMarkers(markers, trackID, db, dbType)
	if err != nil {
		return bbox, trackID, err
	}

	logT(trackID, "GPX", "✔ done")
	return bbox, trackID, nil
}

// processKMLFile handles plain *.kml uploads in streaming mode.
func processKMLFile(
	file multipart.File,
	trackID string,
	db *database.Database,
	dbType string,
) (database.Bounds, string, error) {
	logT(trackID, "KML", "▶ start (stream)")

	markers, err := parseKML(trackID, file)
	if err != nil {
		return database.Bounds{}, trackID, fmt.Errorf("parse KML: %w", err)
	}
	logT(trackID, "KML", "parsed %d markers", len(markers))

	bbox, trackID, err := processAndStoreMarkers(markers, trackID, db, dbType)
	if err != nil {
		return bbox, trackID, err
	}
	logT(trackID, "KML", "✔ done")
	return bbox, trackID, nil
}

// processKMZFile handles *.kmz (ZIP archive with KML inside).
func processKMZFile(
	file multipart.File,
	trackID string,
	db *database.Database,
	dbType string,
) (database.Bounds, string, error) {

	logT(trackID, "KMZ", "▶ start")

	data, err := io.ReadAll(file)
	if err != nil {
		return database.Bounds{}, trackID, fmt.Errorf("read KMZ: %w", err)
	}

	zipR, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return database.Bounds{}, trackID, fmt.Errorf("open KMZ as ZIP: %w", err)
	}

	// accumulate bbox of *all* KML entries inside KMZ
	global := database.Bounds{MinLat: 90, MinLon: 180, MaxLat: -90, MaxLon: -180}

	for _, zf := range zipR.File {
		if filepath.Ext(zf.Name) != ".kml" {
			continue
		}

		kmlF, err := zf.Open()
		if err != nil {
			return global, trackID, fmt.Errorf("open %s: %w", zf.Name, err)
		}
		kmlMarkers, err := parseKML(trackID, kmlF)
		_ = kmlF.Close()
		if err != nil {
			return global, trackID, fmt.Errorf("parse %s: %w", zf.Name, err)
		}
		logT(trackID, "KMZ", "parsed %d markers from %q", len(kmlMarkers), zf.Name)

		bbox, trackID, err := processAndStoreMarkers(kmlMarkers, trackID, db, dbType)
		if err != nil {
			return global, trackID, err
		}

		// expand global bbox
		if bbox.MinLat < global.MinLat {
			global.MinLat = bbox.MinLat
		}
		if bbox.MaxLat > global.MaxLat {
			global.MaxLat = bbox.MaxLat
		}
		if bbox.MinLon < global.MinLon {
			global.MinLon = bbox.MinLon
		}
		if bbox.MaxLon > global.MaxLon {
			global.MaxLon = bbox.MaxLon
		}

		logT(trackID, "KMZ", "✔ done")
		return global, trackID, nil

	}

	logT(trackID, "KMZ", "✔ done")
	return global, trackID, nil
}

// -----------------------------------------------------------------------------
// processRCTRKFile — принимает *.rctrk (Radiacode) в JSON- или текстовом виде.
// Поддерживает оба признака единиц: "sv" (новый Android) и "isSievert" (старый iOS).
// Если ни одного флага нет — считаем, что числа уже в µSv/h и конвертацию НЕ делаем.
// -----------------------------------------------------------------------------
func processRCTRKFile(
	file multipart.File,
	trackID string,
	db *database.Database,
	dbType string,
) (database.Bounds, string, error) {

	logT(trackID, "RCTRK", "▶ start")

	raw, err := io.ReadAll(file)
	if err != nil {
		return database.Bounds{}, trackID, fmt.Errorf("read RCTRK: %w", err)
	}

	// ---------- JSON ----------------------------------------------------------
	// По умолчанию оба флага TRUE ⇒ «единицы уже µSv/h».
	data := database.Data{
		IsSievert:       true,
		IsSievertLegacy: true,
	}

	if err := json.Unmarshal(raw, &data); err == nil && len(data.Markers) > 0 {
		logT(trackID, "RCTRK", "JSON detected, %d markers", len(data.Markers))

		// Выясняем, были ли в файле хоть какие-то флаги.
		// Для этого дешево парсим ключи верхнего уровня.
		var keys map[string]json.RawMessage
		_ = json.Unmarshal(raw, &keys) // ошибок игнорируем — структура уже распарсена

		_, hasSV := keys["sv"]
		_, hasOld := keys["isSievert"]

		flagPresent := hasSV || hasOld

		needConvert := flagPresent && (!data.IsSievert || !data.IsSievertLegacy)
		if needConvert {
			logT(trackID, "RCTRK", "µR/h detected → converting to µSv/h")
			data.Markers = convertRhToSv(data.Markers)
		}

		return processAndStoreMarkers(data.Markers, trackID, db, dbType)
	}

	// ---------- plain-text fallback ------------------------------------------
	markers, err := parseTextRCTRK(trackID, raw)
	if err != nil {
		return database.Bounds{}, trackID, fmt.Errorf("parse text RCTRK: %w", err)
	}
	logT(trackID, "RCTRK", "parsed %d markers (text)", len(markers))

	return processAndStoreMarkers(markers, trackID, db, dbType)
}

// processAtomFastFile handles Atom Fast JSON export (*.json).
func processAtomFastFile(
	file multipart.File,
	trackID string,
	db *database.Database,
	dbType string,
) (database.Bounds, string, error) {

	logT(trackID, "AtomFast", "▶ start")

	data, err := io.ReadAll(file)
	if err != nil {
		return database.Bounds{}, trackID, fmt.Errorf("read AtomFast JSON: %w", err)
	}

	return processAtomFastData(data, trackID, db, dbType)
}

func processAtomFastData(
	data []byte,
	trackID string,
	db *database.Database,
	dbType string,
) (database.Bounds, string, error) {
	var records []struct {
		D   float64 `json:"d"`
		Lat float64 `json:"lat"`
		Lng float64 `json:"lng"`
		T   int64   `json:"t"`
	}
	if err := json.Unmarshal(data, &records); err != nil {
		return database.Bounds{}, trackID, fmt.Errorf("parse AtomFast JSON: %w", err)
	}
	logT(trackID, "AtomFast", "parsed %d markers", len(records))

	markers := make([]database.Marker, 0, len(records))
	for _, r := range records {
		markers = append(markers, database.Marker{
			DoseRate:  r.D,
			CountRate: r.D, // AtomFast stores cps in same field
			Lat:       r.Lat,
			Lon:       r.Lng,
			Date:      r.T / 1000, // ms → s
		})
	}

	return processAndStoreMarkers(markers, trackID, db, dbType)
}

// processTrackExportFile ingests a single JSON track generated by our exporter.
// The helper performs an optimistic duplicate probe before touching the
// database so re-uploaded exports simply reuse the existing track.
func processTrackExportFile(
	ctx context.Context,
	file multipart.File,
	trackID string,
	db *database.Database,
	dbType string,
) (database.Bounds, string, bool, error) {
	return processTrackExportReader(ctx, file, trackID, db, dbType)
}

// archiveProgress keeps the streaming import loop and the logger connected via
// a channel so we avoid mutexes. Each update notes how many entries we have
// consumed and the most recent filename so operators can track forward motion
// on gigantic archives without staring at a static log line.
type archiveProgress struct {
	entries  int
	filename string
}

// processTrackExportArchive handles the weekly tgz bundle produced by the API.
// We iterate entries sequentially because tar readers are streaming, yet still
// lean on channels inside the parser so each export stays memory-light.
func processTrackExportArchive(
	ctx context.Context,
	file multipart.File,
	trackID string,
	db *database.Database,
	dbType string,
) (database.Bounds, string, bool, error) {
	return processTrackExportArchiveReader(ctx, file, trackID, db, dbType, nil)
}

// processTrackExportArchiveReader is the shared implementation for multipart
// uploads and remote downloads. The optional progress channel lets callers
// stream status to logs without blocking the parsing loop.
func processTrackExportArchiveReader(
	ctx context.Context,
	r io.Reader,
	trackID string,
	db *database.Database,
	dbType string,
	updates chan<- archiveProgress,
) (database.Bounds, string, bool, error) {
	logT(trackID, "Export-TGZ", "▶ start")

	gz, err := gzip.NewReader(r)
	if err != nil {
		return database.Bounds{}, trackID, false, fmt.Errorf("open tgz: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)

	combined := database.Bounds{}
	haveBounds := false
	importedAny := false
	primaryTrack := trackID
	entryIndex := 0

	sendProgress := func(name string) {
		if updates == nil {
			return
		}
		select {
		case updates <- archiveProgress{entries: entryIndex, filename: name}:
		default:
		}
	}

	for {
		select {
		case <-ctx.Done():
			return combined, primaryTrack, importedAny, ctx.Err()
		default:
		}

		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return combined, primaryTrack, importedAny, fmt.Errorf("read tgz entry: %w", err)
		}
		if hdr.FileInfo().IsDir() {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(hdr.Name))
		if !strings.HasSuffix(name, ".cim") && !strings.HasSuffix(name, ".json") {
			continue
		}

		logT(trackID, "Export-TGZ", "processing %s", hdr.Name)
		sendProgress(hdr.Name)

		entryBounds, entryTrack, inserted, err := processTrackExportReader(ctx, io.LimitReader(tr, hdr.Size), GenerateSerialNumber(), db, dbType)
		if err != nil {
			return combined, primaryTrack, importedAny, fmt.Errorf("import %s: %w", hdr.Name, err)
		}
		if entryTrack != "" && primaryTrack == trackID {
			primaryTrack = entryTrack
		}
		combined, haveBounds = mergeBounds(combined, entryBounds, haveBounds)
		if inserted {
			importedAny = true
		}
		entryIndex++
		sendProgress(hdr.Name)
	}

	if entryIndex == 0 {
		return database.Bounds{}, trackID, false, fmt.Errorf("tgz archive contained no track export files")
	}
	logT(trackID, "Export-TGZ", "✔ done (entries=%d imported=%v)", entryIndex, importedAny)
	return combined, primaryTrack, importedAny, nil
}

// processTrackExportReader centralises the decoding and duplicate detection shared by
// single-file and archive imports. We keep the logic small so the upload handler
// simply forwards the context and fallback TrackID.
func processTrackExportReader(
	ctx context.Context,
	r io.Reader,
	fallbackTrackID string,
	db *database.Database,
	dbType string,
) (database.Bounds, string, bool, error) {
	select {
	case <-ctx.Done():
		return database.Bounds{}, fallbackTrackID, false, ctx.Err()
	default:
	}

	payload, err := cimimport.Parse(r)
	if err != nil {
		return database.Bounds{}, fallbackTrackID, false, err
	}

	parsedTrackID := strings.TrimSpace(payload.TrackID)
	trackID := fallbackTrackID
	if parsedTrackID != "" {
		trackID = parsedTrackID
	}

	markers, bounds := payload.ToDatabaseMarkers(trackID)
	if len(markers) == 0 {
		return bounds, trackID, false, fmt.Errorf("track export import: no usable markers")
	}

	logT(trackID, "Export", "parsed %d markers", len(markers))

	probe := pickIdentityProbe(markers, 128)
	threshold := min(len(probe), 10)
	if len(probe) > 0 {
		if threshold == 0 {
			threshold = len(probe)
		}
		existing, detectErr := db.DetectExistingTrackID(probe, threshold, dbType)
		if detectErr != nil {
			return bounds, trackID, false, fmt.Errorf("detect duplicate: %w", detectErr)
		}
		if existing != "" {
			logT(existing, "CIM", "duplicate payload matches existing track; skipping import")
			return bounds, existing, false, nil
		}
	}

	storedBounds, finalTrackID, err := processAndStoreMarkers(markers, trackID, db, dbType)
	if err != nil {
		return storedBounds, finalTrackID, false, err
	}
	return storedBounds, finalTrackID, true, nil
}

// processTrackExportPayload lets callers attempt to import a raw JSON payload
// as an export while allowing graceful fallback to other JSON parsers when the
// payload does not match our schema. Using a byte slice keeps retries cheap for
// callers that need multiple attempts.
func processTrackExportPayload(
	ctx context.Context,
	data []byte,
	trackID string,
	db *database.Database,
	dbType string,
) (database.Bounds, string, bool, error) {
	reader := bytes.NewReader(data)
	return processTrackExportReader(ctx, reader, trackID, db, dbType)
}

// countingReader forwards Read calls while emitting byte deltas over a channel.
// We prefer this tiny helper over mutex-protected counters so the progress
// logger can stay decoupled and responsive even when the network stream stalls.
type countingReader struct {
	r       io.Reader
	updates chan<- int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 && c.updates != nil {
		select {
		case c.updates <- int64(n):
		default:
		}
	}
	return n, err
}

// logArchiveImportProgress aggregates download and parse progress for remote
// tgz imports. A ticker throttles updates so huge archives do not overwhelm the
// logs while still giving operators confidence that the stream is moving.
func logArchiveImportProgress(
	ctx context.Context,
	logf func(string, ...any),
	source string,
	contentLength int64,
	byteUpdates <-chan int64,
	entryUpdates <-chan archiveProgress,
	done chan<- struct{},
) {
	defer close(done)

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	var downloaded int64
	var entries int
	lastFile := ""
	lastLoggedBytes := int64(-1)
	lastLoggedEntries := -1
	lastLoggedFile := ""
	lastLoggedPercent := -1.0
	var lastLogTime time.Time
	lastLogLine := ""

	byteCh := byteUpdates
	entryCh := entryUpdates

	logSnapshot := func(force bool) {
		percent := float64(0)
		if contentLength > 0 && downloaded > 0 {
			percent = (float64(downloaded) / float64(contentLength)) * 100
		}

		progressed := percent != lastLoggedPercent || entries != lastLoggedEntries || lastFile != lastLoggedFile
		enoughTime := lastLogTime.IsZero() || time.Since(lastLogTime) >= 30*time.Second
		percentJump := percent >= 0 && lastLoggedPercent >= 0 && (percent-lastLoggedPercent) >= 0.5
		byteJump := contentLength == 0 && lastLoggedBytes >= 0 && (downloaded-lastLoggedBytes) >= 8*1024*1024

		if !force && (!progressed || !(enoughTime || percentJump || byteJump)) {
			return
		}

		line := fmt.Sprintf("remote tgz import [%s]: %.1f%% (%d/%d bytes) entries=%d last=%s", source, percent, downloaded, contentLength, entries, lastFile)

		// Avoid emitting identical consecutive snapshots so operators do not see doubled lines
		// when the final forced update matches the last timed tick.
		if line == lastLogLine {
			return
		}

		logf("%s", line)
		lastLogLine = line
		lastLoggedBytes = downloaded
		lastLoggedEntries = entries
		lastLoggedFile = lastFile
		lastLoggedPercent = percent
		lastLogTime = time.Now()
	}

	for {
		select {
		case <-ctx.Done():
			return
		case delta, ok := <-byteCh:
			if !ok {
				byteCh = nil
				continue
			}
			downloaded += delta
		case progress, ok := <-entryCh:
			if !ok {
				entryCh = nil
				continue
			}
			entries = progress.entries
			lastFile = progress.filename
		case <-ticker.C:
			logSnapshot(false)
		}

		if byteCh == nil && entryCh == nil {
			break
		}
	}

	logSnapshot(true)
}

// importArchiveFromFile streams a local tgz through the shared parser so offline
// operators can preload bundles without relying on HTTP. The function mirrors
// the remote helper by emitting byte and entry progress over channels, keeping
// the UI responsive on slow disks without extra mutexes.
func importArchiveFromFile(
	ctx context.Context,
	path string,
	trackID string,
	db *database.Database,
	dbType string,
	logf func(string, ...any),
) error {
	if logf == nil {
		logf = func(string, ...any) {}
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open tgz file: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat tgz file: %w", err)
	}

	bytesCh := make(chan int64, 256)
	entriesCh := make(chan archiveProgress, 64)
	done := make(chan struct{})
	go logArchiveImportProgress(ctx, logf, path, info.Size(), bytesCh, entriesCh, done)

	reader := &countingReader{r: file, updates: bytesCh}
	bounds, finalTrack, imported, err := processTrackExportArchiveReader(ctx, reader, trackID, db, dbType, entriesCh)
	close(bytesCh)
	close(entriesCh)
	<-done
	if err != nil {
		return fmt.Errorf("local tgz import: %w", err)
	}

	logf("local tgz import complete: imported=%v track=%s bounds=%v", imported, finalTrack, bounds)
	return nil
}

// importArchiveFromURL streams a remote tgz into the existing archive parser so
// operators can refresh a deployment directly from an external weekly bundle.
// The function runs synchronously to match the explicit CLI flag and exits the
// program after finishing, keeping behaviour predictable for automation.
func importArchiveFromURL(
	ctx context.Context,
	sourceURL string,
	trackID string,
	db *database.Database,
	dbType string,
	logf func(string, ...any),
) error {
	if logf == nil {
		logf = func(string, ...any) {}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download tgz: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download tgz: unexpected status %s", resp.Status)
	}

	bytesCh := make(chan int64, 256)
	entriesCh := make(chan archiveProgress, 64)
	done := make(chan struct{})
	go logArchiveImportProgress(ctx, logf, sourceURL, resp.ContentLength, bytesCh, entriesCh, done)

	reader := &countingReader{r: resp.Body, updates: bytesCh}
	bounds, finalTrack, imported, err := processTrackExportArchiveReader(ctx, reader, trackID, db, dbType, entriesCh)
	close(bytesCh)
	close(entriesCh)
	<-done
	if err != nil {
		return fmt.Errorf("remote tgz import: %w", err)
	}

	logf("remote tgz import complete: imported=%v track=%s bounds=%v", imported, finalTrack, bounds)
	return nil
}

// mergeBounds combines multiple bounding boxes while tracking whether we already
// have a baseline. This keeps archive imports from misreporting coordinates when
// the first few entries happen to be duplicates.
func mergeBounds(current database.Bounds, incoming database.Bounds, have bool) (database.Bounds, bool) {
	if incoming == (database.Bounds{}) {
		return current, have
	}
	if !have {
		return incoming, true
	}
	if incoming.MinLat < current.MinLat {
		current.MinLat = incoming.MinLat
	}
	if incoming.MinLon < current.MinLon {
		current.MinLon = incoming.MinLon
	}
	if incoming.MaxLat > current.MaxLat {
		current.MaxLat = incoming.MaxLat
	}
	if incoming.MaxLon > current.MaxLon {
		current.MaxLon = incoming.MaxLon
	}
	return current, true
}

func processChichaTrackJSON(
	data []byte,
	trackID string,
	db *database.Database,
	dbType string,
) (database.Bounds, string, error) {
	var payload struct {
		Format  string `json:"format"`
		Version int    `json:"version"`
		Track   struct {
			TrackID        string   `json:"trackID"`
			DetectorName   string   `json:"detectorName"`
			DetectorType   string   `json:"detectorType"`
			RadiationTypes []string `json:"radiationTypes"`
		} `json:"track"`
		Markers []struct {
			ID                 int64    `json:"id"`
			TrackID            string   `json:"trackID"`
			TimeUnix           int64    `json:"timeUnix"`
			TimeUTC            string   `json:"timeUTC"`
			Lat                float64  `json:"lat"`
			Lon                float64  `json:"lon"`
			AltitudeM          *float64 `json:"altitudeM"`
			DoseMicroSvH       float64  `json:"doseRateMicroSvH"`
			DoseMicroRoentgenH float64  `json:"doseRateMicroRh"`
			DoseMilliSvH       float64  `json:"doseRateMilliSvH"`
			DoseMilliRH        float64  `json:"doseRateMilliRH"`
			CountRateCPS       float64  `json:"countRateCPS"`
			SpeedMS            float64  `json:"speedMS"`
			SpeedKMH           float64  `json:"speedKMH"`
			TemperatureC       *float64 `json:"temperatureC"`
			HumidityPercent    *float64 `json:"humidityPercent"`
			DetectorName       string   `json:"detectorName"`
			DetectorType       string   `json:"detectorType"`
			RadiationTypes     []string `json:"radiationTypes"`
		} `json:"markers"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return database.Bounds{}, trackID, errNotChichaTrackJSON
	}
	if !strings.EqualFold(payload.Format, "chicha-track-json") {
		return database.Bounds{}, trackID, errNotChichaTrackJSON
	}
	if len(payload.Markers) == 0 {
		return database.Bounds{}, trackID, fmt.Errorf("chicha track json: no markers")
	}

	candidateTrackID := strings.TrimSpace(payload.Track.TrackID)
	defaultDetectorType := strings.TrimSpace(payload.Track.DetectorType)
	defaultDetectorName := strings.TrimSpace(payload.Track.DetectorName)
	defaultRadiation := normalizeRadiationList(payload.Track.RadiationTypes)

	markers := make([]database.Marker, 0, len(payload.Markers))
	for _, item := range payload.Markers {
		ts := extractUnixSeconds(item.TimeUnix, item.TimeUTC)
		dose := item.DoseMicroSvH
		if dose == 0 && item.DoseMicroRoentgenH != 0 {
			dose = item.DoseMicroRoentgenH / microRoentgenPerMicroSievert
		}
		if dose == 0 && item.DoseMilliSvH != 0 {
			dose = item.DoseMilliSvH * 1000.0
		}
		if dose == 0 && item.DoseMilliRH != 0 {
			dose = item.DoseMilliRH * 10.0
		}

		speed := item.SpeedMS
		if speed == 0 && item.SpeedKMH != 0 {
			speed = item.SpeedKMH / 3.6
		}

		detectorName := strings.TrimSpace(item.DetectorName)
		if detectorName == "" {
			detectorName = defaultDetectorName
		}

		detector := strings.TrimSpace(item.DetectorType)
		if detector == "" {
			detector = defaultDetectorType
		}
		if detector == "" {
			detector = detectorTypeFromName(detectorName)
		}

		radiationList := normalizeRadiationList(item.RadiationTypes)
		if len(radiationList) == 0 {
			radiationList = defaultRadiation
		}

		var altitude float64
		var altitudeValid bool
		if item.AltitudeM != nil {
			altitude = *item.AltitudeM
			altitudeValid = true
		}
		var temperature float64
		var temperatureValid bool
		if item.TemperatureC != nil {
			temperature = *item.TemperatureC
			temperatureValid = true
		}
		var humidity float64
		var humidityValid bool
		if item.HumidityPercent != nil {
			humidity = *item.HumidityPercent
			humidityValid = true
		}

		markers = append(markers, database.Marker{
			ID:               item.ID,
			DoseRate:         dose,
			Date:             ts,
			Lon:              item.Lon,
			Lat:              item.Lat,
			CountRate:        item.CountRateCPS,
			Speed:            speed,
			Altitude:         altitude,
			Temperature:      temperature,
			Humidity:         humidity,
			Detector:         detector,
			Radiation:        strings.Join(radiationList, ","),
			AltitudeValid:    altitudeValid,
			TemperatureValid: temperatureValid,
			HumidityValid:    humidityValid,
		})

		if candidateTrackID == "" {
			candidateTrackID = strings.TrimSpace(item.TrackID)
		}
	}

	if candidateTrackID != "" {
		trackID = candidateTrackID
	}

	logT(trackID, "ChichaJSON", "parsed %d markers", len(markers))
	return processAndStoreMarkers(markers, trackID, db, dbType)
}

func extractUnixSeconds(timeUnix int64, timeUTC string) int64 {
	if timeUnix > 1_000_000_000_000 {
		return timeUnix / 1000
	}
	if timeUnix > 0 {
		return timeUnix
	}
	if strings.TrimSpace(timeUTC) != "" {
		if ts, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(timeUTC)); err == nil {
			return ts.Unix()
		}
	}
	return 0
}

func normalizeRadiationList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	out := make([]string, 0, len(values))
	for _, raw := range values {
		channel := strings.ToLower(strings.TrimSpace(raw))
		if channel == "" {
			continue
		}
		if _, ok := seen[channel]; ok {
			continue
		}
		seen[channel] = struct{}{}
		out = append(out, channel)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// detectorTypeFromName extracts the type hint that we encode into detectorName
// during export. We slice on the first ':' because stableDetectorName prefixes
// the trackID before the reported detector model.
func detectorTypeFromName(detectorName string) string {
	detectorName = strings.TrimSpace(detectorName)
	if detectorName == "" {
		return ""
	}
	if idx := strings.Index(detectorName, ":"); idx >= 0 && idx+1 < len(detectorName) {
		candidate := strings.TrimSpace(detectorName[idx+1:])
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

// parseBGeigieCoord parses coordinates that may have hemisphere suffix.
func parseBGeigieCoord(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	r := s[len(s)-1]
	if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
		base := s[:len(s)-1]
		v, _ := strconv.ParseFloat(base, 64)
		switch strings.ToUpper(string(r)) {
		case "S", "W":
			return -v
		default:
			return v
		}
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// parseDMM parses degrees+minutes (DDMM.MMMM or DDDMM.MMMM) with hemisphere.
func parseDMM(val, hemi string, degDigits int) float64 {
	val = strings.TrimSpace(val)
	hemi = strings.TrimSpace(hemi)
	if val == "" {
		return 0
	}
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return 0
	}
	deg := int(f / 100.0)
	minutes := f - float64(deg*100)
	d := float64(deg) + minutes/60.0
	switch strings.ToUpper(hemi) {
	case "S", "W":
		d = -d
	}
	if degDigits == 2 {
		if d < -90 || d > 90 {
			return 0
		}
	} else {
		if d < -180 || d > 180 {
			return 0
		}
	}
	return d
}

// processAndStoreMarkers is the common pipeline:
// 0. bbox calculation             • O(N)
// 1. fast duplicate-track probe   • O(K·q), K ≪ N (early-exit)
// 2. assign final TrackID         • O(N)
// 3. basic filters                • O(N)
// 4. speed calculation            • O(N)
// 5. pre-compute 20 zoom levels   • O(N) parallel
// 6. batch-insert into DB         • one transaction, multi-row VALUES
//
// Concurrency-friendly: no mutexes; DB/sql pool handles connection safety.
func processAndStoreMarkers(
	markers []database.Marker,
	initTrackID string, // initially generated ID
	db *database.Database,
	dbType string,
) (database.Bounds, string, error) {

	// ── step 0: bounding box (cheap) ────────────────────────────────
	bbox := database.Bounds{MinLat: 90, MinLon: 180, MaxLat: -90, MaxLon: -180}
	for _, m := range markers {
		if m.Lat < bbox.MinLat {
			bbox.MinLat = m.Lat
		}
		if m.Lat > bbox.MaxLat {
			bbox.MaxLat = m.Lat
		}
		if m.Lon < bbox.MinLon {
			bbox.MinLon = m.Lon
		}
		if m.Lon > bbox.MaxLon {
			bbox.MaxLon = m.Lon
		}
	}

	trackID := initTrackID

	// ── step 1: fast probe instead of full-scan ─────────────────────
	// Limit DB random lookups to a tiny sample (e.g. 128 points).
	probe := pickIdentityProbe(markers, 128)
	if existing, err := db.DetectExistingTrackID(probe, 10, dbType); err != nil {
		return bbox, trackID, err
	} else if existing != "" {
		logT(trackID, "Store", "⚠ detected existing trackID %s — reusing", existing)
		trackID = existing
	} else {
		logT(trackID, "Store", "unique track, proceed with new trackID")
	}

	// ── step 2: attach FINAL TrackID ────────────────────────────────
	for i := range markers {
		markers[i].TrackID = trackID
	}

	// ── step 3: light filters ───────────────────────────────────────
	markers = filterZeroMarkers(markers)
	markers = filterInvalidDateMarkers(markers)
	if len(markers) == 0 {
		return bbox, trackID, fmt.Errorf("all markers filtered out")
	}

	// ── step 4: speed calculation (pure Go) ─────────────────────────
	markers = calculateSpeedForMarkers(markers)

	// ── step 5: build aggregates for 20 zooms — O(N)+goroutines ─────
	// We keep the raw zoom=0 markers in front so downstream exports
	// retain the exact coordinates users uploaded while still storing
	// clustered variants for map views. Mixing raw and aggregates was
	// the root cause of shifted coordinates in legacy .cim exports
	// because the database lacked an unsmoothed copy to serialize.
	allZoom := append(markers, precomputeMarkersForAllZoomLevels(markers)...)
	logT(trackID, "Store", "precomputed %d zoom-markers", len(allZoom))

	progressCh := make(chan database.MarkerBatchProgress, 16)
	progressDone := make(chan struct{})

	go func(total int) {
		defer close(progressDone)
		started := time.Now()
		lastLog := time.Time{}
		for p := range progressCh {
			if lastLog.IsZero() || time.Since(lastLog) >= 5*time.Second || p.Done >= total {
				logT(trackID, "Store", "storing markers %d/%d (+%d via %s) in %s elapsed %s",
					p.Done, p.Total, p.Batch, p.Mode, p.Duration.Truncate(time.Millisecond),
					time.Since(started).Truncate(time.Second))
				lastLog = time.Now()
			}
		}
	}(len(allZoom))

	// ── step 6: single transaction + multi-row VALUES ───────────────
	if strings.EqualFold(dbType, "clickhouse") {
		if err := db.InsertMarkersBulk(nil, allZoom, dbType, 1000, progressCh); err != nil {
			close(progressCh)
			<-progressDone
			return bbox, trackID, fmt.Errorf("bulk insert: %w", err)
		}
	} else if strings.EqualFold(dbType, "duckdb") {
		// DuckDB stays quick when inserts share a transaction but still aborts on constraint
		// errors. InsertMarkersBulk wraps each batch in a savepoint so we keep the speed benefit
		// of a single commit while still falling back to duplicate-tolerant single-row inserts
		// when archives contain overlap.
		if err := db.InsertMarkersBulk(nil, allZoom, dbType, 1000, progressCh); err != nil {
			close(progressCh)
			<-progressDone
			return bbox, trackID, fmt.Errorf("bulk insert: %w", err)
		}
	} else {
		tx, err := db.DB.Begin()
		if err != nil {
			close(progressCh)
			<-progressDone
			return bbox, trackID, err
		}
		// Batch size 500–1000 usually gives a good balance on large B-Trees.
		if err := db.InsertMarkersBulk(tx, allZoom, dbType, 1000, progressCh); err != nil {
			_ = tx.Rollback()
			close(progressCh)
			<-progressDone
			return bbox, trackID, fmt.Errorf("bulk insert: %w", err)
		}
		if err := tx.Commit(); err != nil {
			close(progressCh)
			<-progressDone
			return bbox, trackID, err
		}
	}

	close(progressCh)
	<-progressDone

	logT(trackID, "Store", "✔ stored (new %d markers)", len(allZoom))
	return bbox, trackID, nil
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(100 << 20); err != nil {
		http.Error(w, "multipart parse error", http.StatusBadRequest)
		return
	}
	files := r.MultipartForm.File["files[]"]
	if len(files) == 0 {
		http.Error(w, "no files selected", http.StatusBadRequest)
		return
	}

	trackID := GenerateSerialNumber()
	logT(trackID, "Upload", "▶ start, total=%d", len(files))

	// глобальные границы всего набора файлов
	global := database.Bounds{MinLat: 90, MinLon: 180, MaxLat: -90, MaxLon: -180}

	for _, fh := range files {
		logT(trackID, "Upload", "file received: %s", fh.Filename)

		f, _ := fh.Open()
		// don't defer yet; may re-open for sniffing

		var (
			bbox database.Bounds
			err  error
		)
		ext := strings.ToLower(filepath.Ext(fh.Filename))
		switch ext {
		case ".kml":
			bbox, trackID, err = processKMLFile(f, trackID, db, *dbType)
		case ".kmz":
			bbox, trackID, err = processKMZFile(f, trackID, db, *dbType)
		case ".gpx":
			bbox, trackID, err = processGPXFile(f, trackID, db, *dbType)
		case ".csv":
			bbox, trackID, err = processAtomSwiftCSVFile(f, trackID, db, *dbType)
		case ".rctrk":
			bbox, trackID, err = processRCTRKFile(f, trackID, db, *dbType)
		case ".cim":
			var imported bool
			bbox, trackID, imported, err = processTrackExportFile(r.Context(), f, trackID, db, *dbType)
			if err == nil && !imported {
				logT(trackID, "Upload", "skipped duplicate legacy payload")
			}
		case ".json":
			raw, readErr := io.ReadAll(f)
			if readErr != nil {
				bbox = database.Bounds{}
				err = fmt.Errorf("read JSON: %w", readErr)
				break
			}
			var imported bool
			bbox, trackID, imported, err = processTrackExportPayload(r.Context(), raw, trackID, db, *dbType)
			if errors.Is(err, cimimport.ErrInvalidExportFormat) {
				bbox, trackID, err = processChichaTrackJSON(raw, trackID, db, *dbType)
			}
			if errors.Is(err, errNotChichaTrackJSON) {
				bbox, trackID, err = processAtomFastData(raw, trackID, db, *dbType)
			}
			if err == nil && !imported {
				logT(trackID, "Upload", "skipped duplicate track export JSON")
			}
		case ".tgz":
			var imported bool
			bbox, trackID, imported, err = processTrackExportArchive(r.Context(), f, trackID, db, *dbType)
			if err == nil && !imported {
				logT(trackID, "Upload", "tgz contained only duplicate track exports")
			}
		case ".log", ".txt":
			bbox, trackID, err = processBGeigieZenFile(f, trackID, db, *dbType)
		default:
			// Sniff for bGeigie $BNRDD content if extension is missing/wrong
			// Read up to 1024 bytes, then re-open the file for processing if needed
			buf := make([]byte, 1024)
			n, _ := f.Read(buf)
			_ = f.Close()
			content := string(buf[:n])
			if strings.Contains(content, "$BNRDD") {
				logT(trackID, "Upload", "sniffed bGeigie content for %s (ext=%q)", fh.Filename, ext)
				// reopen fresh handle
				f2, _ := fh.Open()
				bbox, trackID, err = processBGeigieZenFile(f2, trackID, db, *dbType)
				_ = f2.Close()
				if err != nil {
					logT(trackID, "Upload", "error processing (sniffed) %s: %v", fh.Filename, err)
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				break
			}

			if strings.HasSuffix(strings.ToLower(fh.Filename), ".tar.gz") {
				_ = f.Close()
				f2, openErr := fh.Open()
				if openErr != nil {
					err = fmt.Errorf("reopen tar.gz: %w", openErr)
					break
				}
				f = f2
				var imported bool
				bbox, trackID, imported, err = processTrackExportArchive(r.Context(), f, trackID, db, *dbType)
				if err == nil && !imported {
					logT(trackID, "Upload", "tar.gz contained only duplicate track exports")
				}
				break
			}

			logT(trackID, "Upload", "unsupported file type: %s (ext=%q)", fh.Filename, ext)
			http.Error(w, "unsupported file type", http.StatusBadRequest)
			return
		}
		// ensure we close handle if not closed by default/sniff
		_ = f.Close()
		if err != nil {
			logT(trackID, "Upload", "error processing %s: %v", fh.Filename, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// расширяем глобальные границы
		if bbox.MinLat < global.MinLat {
			global.MinLat = bbox.MinLat
		}
		if bbox.MaxLat > global.MaxLat {
			global.MaxLat = bbox.MaxLat
		}
		if bbox.MinLon < global.MinLon {
			global.MinLon = bbox.MinLon
		}
		if bbox.MaxLon > global.MaxLon {
			global.MaxLon = bbox.MaxLon
		}
	}

	trackURL := fmt.Sprintf(
		"/trackid/%s?minLat=%f&minLon=%f&maxLat=%f&maxLon=%f&zoom=14&layer=%s",
		trackID, global.MinLat, global.MinLon, global.MaxLat, global.MaxLon,
		"OpenStreetMap")

	logT(trackID, "Upload", "redirecting browser to: %s", trackURL)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"status":   "success",
		"trackURL": trackURL,
	}); err != nil {
		if isClientDisconnect(err) {
			log.Printf("client disconnected while writing upload response")
		} else {
			log.Printf("upload response write error: %v", err)
		}
	}
}

// =====================
// WEB
// =====================
// =====================
// WEB  — главная карта
// =====================
// marshalTemplateJS encodes the provided value into JSON and tags it as safe
// JavaScript for html/template. We return template.JS so scripts can embed the
// literal without tripping the context analyser, following "A little copying is
// better than a little dependency" by keeping the helper tiny and local.
func marshalTemplateJS(value interface{}) (template.JS, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return template.JS(""), err
	}
	return template.JS(payload), nil
}

// parseDebugAllowlist converts the comma-separated flag payload into a lookup
// map. Returning a new map keeps the zero value useful and avoids hidden shared
// state that could surprise future callers.
func parseDebugAllowlist(raw string) map[string]struct{} {
	allow := make(map[string]struct{})
	for _, part := range strings.Split(raw, ",") {
		ip := strings.TrimSpace(part)
		if ip == "" {
			continue
		}
		allow[ip] = struct{}{}
	}
	return allow
}

// requestClientIP mirrors the API rate-limiter helper so template handlers can
// decide whether to surface diagnostics. We respect X-Forwarded-For first so the
// overlay still works when the service sits behind a proxy.
func requestClientIP(r *http.Request) string {
	forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if forwarded != "" {
		parts := strings.Split(forwarded, ",")
		candidate := strings.TrimSpace(parts[0])
		if candidate != "" {
			return candidate
		}
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && strings.TrimSpace(host) != "" {
		return host
	}
	if trimmed := strings.TrimSpace(r.RemoteAddr); trimmed != "" {
		return trimmed
	}
	return ""
}

// debugEnabledForRequest checks whether the caller IP is in the allowlist.
// Keeping the lookup in one spot makes it simple to extend later with CIDR
// matching or runtime toggles without touching handlers.
func debugEnabledForRequest(r *http.Request) bool {
	if len(debugIPAllowlist) == 0 || r == nil {
		return false
	}
	ip := requestClientIP(r)
	if ip == "" {
		return false
	}
	_, ok := debugIPAllowlist[ip]
	return ok
}

func mapHandler(w http.ResponseWriter, r *http.Request) {
	lang := getPreferredLanguage(r)

	// Готовим шаблон
	tmpl := template.Must(template.New("map.html").Funcs(template.FuncMap{
		"translate": func(key string) string {
			if val, ok := translations[lang][key]; ok {
				return val
			}
			return translations["en"][key]
		},
	}).ParseFS(content, "public_html/map.html"))

	if CompileVersion == "dev" {
		CompileVersion = "latest"
	}

	translationsJSON, err := marshalTemplateJS(translations)
	if err != nil {
		log.Printf("map handler: marshal translations failed: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	markers := doseData.Markers
	if markers == nil {
		markers = []database.Marker{}
	}
	markersJSON, err := marshalTemplateJS(markers)
	if err != nil {
		log.Printf("map handler: marshal markers failed: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Данные для шаблона
	data := struct {
		Version           string
		Translations      map[string]map[string]string
		Lang              string
		DefaultLat        float64
		DefaultLon        float64
		DefaultZoom       int
		DefaultLayer      string
		RealtimeAvailable bool
		SupportEmail      string
		TranslationsJSON  template.JS
		MarkersJSON       template.JS
		DebugEnabled      bool
	}{
		Version:           CompileVersion,
		Translations:      translations,
		Lang:              lang,
		DefaultLat:        *defaultLat,
		DefaultLon:        *defaultLon,
		DefaultZoom:       *defaultZoom,
		DefaultLayer:      *defaultLayer,
		RealtimeAvailable: *safecastRealtimeEnabled,
		SupportEmail:      strings.TrimSpace(*supportEmail),
		TranslationsJSON:  translationsJSON,
		MarkersJSON:       markersJSON,
		DebugEnabled:      debugEnabledForRequest(r),
	}

	// Рендерим в буфер, чтобы не дублировать WriteHeader
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		log.Printf("Error executing template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := buf.WriteTo(w); err != nil {
		if isClientDisconnect(err) {
			log.Printf("client disconnected while writing response")
		} else {
			log.Printf("Error writing response: %v", err)
		}
	}
}

// licenseHandler serves the embedded license documents so operators can ship a
// single binary and still expose the legal texts offline. Keeping the handler
// tiny follows "The bigger the interface, the weaker the abstraction" while
// letting the UI lazily fetch the raw files on demand.
func licenseHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rawPath := strings.TrimPrefix(r.URL.Path, "/licenses/")
	if rawPath == r.URL.Path {
		http.NotFound(w, r)
		return
	}
	code := strings.ToLower(strings.Trim(rawPath, "/"))

	var file string
	switch code {
	case "mit":
		file = "LICENSE"
	case "cc0":
		file = "LICENSE.CC0"
	default:
		http.NotFound(w, r)
		return
	}

	data, err := content.ReadFile(file)
	if err != nil {
		log.Printf("license handler: %s read error: %v", file, err)
		http.Error(w, "unable to load license", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	http.ServeContent(w, r, file, time.Time{}, bytes.NewReader(data))
}

// shortRedirectHandler resolves a short code and redirects visitors to the
// stored long URL. We lean on context timeouts instead of bespoke timers,
// echoing "The bigger the interface, the weaker the abstraction" by keeping
// the signature small.
func shortRedirectHandler(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/s/"))
	if code == "" {
		http.NotFound(w, r)
		return
	}
	if db == nil || db.DB == nil {
		http.NotFound(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	target, err := db.ResolveShortLink(ctx, code)
	if err != nil {
		log.Printf("short link lookup for %q failed: %v", code, err)
		http.Error(w, "short link lookup failed", http.StatusInternalServerError)
		return
	}
	if strings.TrimSpace(target) == "" {
		http.NotFound(w, r)
		return
	}

	http.Redirect(w, r, target, http.StatusFound)
}

// =====================
// WEB  — страница трека
// =====================
// trackHandler — страница одного трека.
// Теперь НЕ загружает маркеры в HTML: JS сам запросит нужный зум.
func trackHandler(w http.ResponseWriter, r *http.Request) {
	lang := getPreferredLanguage(r)

	// /trackid/<ID>
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		http.Error(w, "TrackID not provided", http.StatusBadRequest)
		return
	}
	trackID := parts[2] // всё равно понадобится в JS

	// --- шаблон ----------------------------------------------------------------
	tmpl := template.Must(template.New("map.html").Funcs(template.FuncMap{
		"translate": func(key string) string {
			if v, ok := translations[lang][key]; ok {
				return v
			}
			return translations["en"][key]
		},
	}).ParseFS(content, "public_html/map.html"))

	translationsJSON, err := marshalTemplateJS(translations)
	if err != nil {
		log.Printf("track handler: marshal translations failed: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	emptyMarkers := []database.Marker{}
	markersJSON, err := marshalTemplateJS(emptyMarkers)
	if err != nil {
		log.Printf("track handler: marshal markers failed: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// отдаём пустой срез маркеров
	data := struct {
		Version           string
		Translations      map[string]map[string]string
		Lang              string
		DefaultLat        float64
		DefaultLon        float64
		DefaultZoom       int
		DefaultLayer      string
		RealtimeAvailable bool
		SupportEmail      string
		TranslationsJSON  template.JS
		MarkersJSON       template.JS
		DebugEnabled      bool
	}{
		Version:           CompileVersion,
		Translations:      translations,
		Lang:              lang,
		DefaultLat:        *defaultLat,
		DefaultLon:        *defaultLon,
		DefaultZoom:       *defaultZoom,
		DefaultLayer:      *defaultLayer,
		RealtimeAvailable: *safecastRealtimeEnabled,
		SupportEmail:      strings.TrimSpace(*supportEmail),
		TranslationsJSON:  translationsJSON,
		MarkersJSON:       markersJSON,
		DebugEnabled:      debugEnabledForRequest(r),
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		log.Printf("template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := buf.WriteTo(w); err != nil {
		if isClientDisconnect(err) {
			log.Printf("client disconnected while writing response")
		} else {
			log.Printf("write resp: %v", err)
		}
	}

	// Ради отладки: показываем, что HTML отдали без тяжёлых данных
	log.Printf("Track page %s rendered.", trackID)
}

// qrPngHandler generates a QR code image for a given URL.
func qrPngHandler(w http.ResponseWriter, r *http.Request) {
	u := r.URL.Query().Get("u")
	if u == "" {
		if ref := r.Referer(); ref != "" {
			u = ref
		} else {
			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}
			u = scheme + "://" + r.Host + r.URL.RequestURI()
		}
	}
	if len(u) > 4096 {
		u = u[:4096]
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", "inline; filename=\"qr.png\"")

	// (А) если логотип лежит в файле:
	// logoBytes, _ := os.ReadFile("static/radiation.png")

	// (Б) или без файла — пусть пакет нарисует знак сам:
	var logoBytes []byte

	opts := qrlogoext.Options{
		TargetPx:    1500,
		Fg:          color.RGBA{0, 0, 0, 255},       // чёрные модули
		Bg:          color.RGBA{255, 255, 255, 255}, // БЕЛЫЙ фон
		Logo:        color.RGBA{233, 192, 35, 255},  // ЖЕЛТЫЙ знак радиации
		LogoBoxFrac: 0.32,                           // большой центральный квадрат
		LogoPadding: 16,                             // отступ для картинки (если PNG вставляешь)
	}

	if err := qrlogoext.EncodePNG(w, []byte(u), logoBytes, opts); err != nil {
		http.Error(w, "QR encode: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

// getMarkersHandler — берёт маркеры в заданном окне и фильтрах
// +НОВОЕ: dateFrom/dateTo (UNIX-seconds) диапазон времени.
func getMarkersHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	zoom, _ := strconv.Atoi(q.Get("zoom"))
	minLat, _ := strconv.ParseFloat(q.Get("minLat"), 64)
	minLon, _ := strconv.ParseFloat(q.Get("minLon"), 64)
	maxLat, _ := strconv.ParseFloat(q.Get("maxLat"), 64)
	maxLon, _ := strconv.ParseFloat(q.Get("maxLon"), 64)
	trackID := q.Get("trackID")

	// ----- ✈️🚗🚶 фильтр скорости  ---------------------------------
	var sr []database.SpeedRange
	if s := q.Get("speeds"); s != "" {
		for _, tag := range strings.Split(s, ",") {
			if r, ok := speedCatalog[tag]; ok {
				sr = append(sr, database.SpeedRange(r))
			}
		}
	}
	if len(sr) == 0 && q.Get("speeds") != "" { // все выключены
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
		return
	}

	// ----- ⏱️  фильтр времени  ------------------------------------
	var (
		dateFrom int64
		dateTo   int64
	)
	if s := q.Get("dateFrom"); s != "" {
		dateFrom, _ = strconv.ParseInt(s, 10, 64)
	}
	if s := q.Get("dateTo"); s != "" {
		dateTo, _ = strconv.ParseInt(s, 10, 64)
	}

	// ----- запрос к БД  ------------------------------------------
	var (
		markers []database.Marker
		err     error
	)
	if trackID != "" {
		markers, err = db.GetMarkersByTrackIDZoomBoundsSpeed(
			trackID, zoom, minLat, minLon, maxLat, maxLon,
			dateFrom, dateTo, sr, *dbType)
	} else {
		markers, err = db.GetMarkersByZoomBoundsSpeed(
			zoom, minLat, minLon, maxLat, maxLon,
			dateFrom, dateTo, sr, *dbType)
	}
	if err != nil {
		http.Error(w, "Error fetching markers", http.StatusInternalServerError)
		return
	}

	if *safecastRealtimeEnabled {
		// We only touch realtime tables when the operator explicitly enables the feature.
		if rt, err := db.GetLatestRealtimeByBounds(minLat, minLon, maxLat, maxLon, *dbType); err == nil {
			for i := range rt {
				// Sanitise detector names on the fly so legacy rows without
				// the new resolver still produce friendly popups.
				rt[i].Tube = safecastrealtime.DetectorLabel(rt[i].Tube, rt[i].Transport, rt[i].DeviceName)
			}
			markers = append(markers, rt...)
		} else {
			log.Printf("realtime query: %v", err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(markers)
}

// ========
// Streaming markers via SSE
// ========

// aggregateMarkers chooses the most radioactive marker per grid cell.
// Cells shrink with higher zoom to preserve detail.
func aggregateMarkers(ctx context.Context, in <-chan database.Marker, zoom int) <-chan database.Marker {
	out := make(chan database.Marker)
	go func() {
		defer close(out)
		cells := make(map[string]database.Marker)
		scale := math.Pow(2, float64(zoom))
		for {
			select {
			case <-ctx.Done():
				return
			case m, ok := <-in:
				if !ok {
					return
				}
				key := fmt.Sprintf("%d:%d", int(m.Lat*scale), int(m.Lon*scale))
				if prev, ok := cells[key]; !ok || m.DoseRate > prev.DoseRate {
					cells[key] = m
					out <- m
				}
			}
		}
	}()
	return out
}

// streamMarkersHandler streams markers via Server-Sent Events.
// Markers are emitted as soon as they are read and aggregated.
func streamMarkersHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	zoom, _ := strconv.Atoi(q.Get("zoom"))
	minLat, _ := strconv.ParseFloat(q.Get("minLat"), 64)
	minLon, _ := strconv.ParseFloat(q.Get("minLon"), 64)
	maxLat, _ := strconv.ParseFloat(q.Get("maxLat"), 64)
	maxLon, _ := strconv.ParseFloat(q.Get("maxLon"), 64)
	trackID := q.Get("trackID")
	// Choose streaming source: either entire map or a single track.
	ctx := r.Context()
	var (
		baseSrc <-chan database.Marker
		errCh   <-chan error
	)
	if trackID != "" {
		baseSrc, errCh = db.StreamMarkersByTrackIDZoomAndBounds(ctx, trackID, zoom, minLat, minLon, maxLat, maxLon, *dbType)
	} else {
		baseSrc, errCh = db.StreamMarkersByZoomAndBounds(ctx, zoom, minLat, minLon, maxLat, maxLon, *dbType)
	}

	// Fetch current realtime points once so the map reflects network devices.
	// We only touch the realtime table when the dedicated flag enables it so
	// operators control the feature explicitly.
	var rtMarks []database.Marker
	if *safecastRealtimeEnabled {
		var err error
		rtMarks, err = db.GetLatestRealtimeByBounds(minLat, minLon, maxLat, maxLon, *dbType)
		if err != nil {
			log.Printf("realtime query: %v", err)
		}
		// Log bounds alongside count to help diagnose empty map tiles.
		log.Printf("realtime markers: %d lat[%f,%f] lon[%f,%f]", len(rtMarks), minLat, maxLat, minLon, maxLon)
	}

	agg := aggregateMarkers(ctx, baseSrc, zoom)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Emit realtime markers first when enabled.
	for _, m := range rtMarks {
		b, _ := json.Marshal(m)
		fmt.Fprintf(w, "data: %s\n\n", b)
	}
	flusher.Flush()

	for {
		select {
		case <-ctx.Done():
			return
		case err := <-errCh:
			if err != nil {
				fmt.Fprintf(w, "event: done\ndata: %v\n\n", err)
			} else {
				fmt.Fprint(w, "event: done\ndata: end\n\n")
			}
			flusher.Flush()
			return
		case m, ok := <-agg:
			if !ok {
				fmt.Fprint(w, "event: done\ndata: end\n\n")
				flusher.Flush()
				return
			}
			b, _ := json.Marshal(m)
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}
	}
}

// realtimePoint holds a single measurement for realtime history charts.
// Keeping the struct tiny helps when we duplicate slices for aggregation.
type realtimePoint struct {
	Timestamp int64   `json:"timestamp"`
	Value     float64 `json:"value"`
}

// rangeSummary describes the plotted window and aggregation bucket.
// Returning it to the frontend lets JavaScript render friendly titles.
type rangeSummary struct {
	Start         int64 `json:"start"`
	End           int64 `json:"end"`
	BucketSeconds int64 `json:"bucketSeconds"`
}

// historyAggregate bundles the processed realtime readings so the handler can
// serialise the JSON response without juggling several parallel slices.
type historyAggregate struct {
	Series      map[string][]realtimePoint
	ExtraSeries map[string]map[string][]realtimePoint
	Extra       map[string]float64
	Ranges      map[string]rangeSummary
	DeviceName  string
	Transport   string
	Tube        string
	Country     string
}

// realtimeMeasurementPayload moves measurements between goroutines without
// sharing mutable state and keeps channel signatures consistent across helpers.
type realtimeMeasurementPayload struct {
	timestamp int64
	radiation float64
	extras    map[string]float64
	name      string
	transport string
	tube      string
	country   string
}

// decodeRealtimeExtras converts the optional JSON blob with temperature or
// humidity hints into a float map.  Invalid payloads are ignored so noisy
// devices do not break chart rendering.
func decodeRealtimeExtras(raw string) map[string]float64 {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	var parsed map[string]float64
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		log.Printf("decode realtime extras: %v", err)
		return nil
	}
	clean := make(map[string]float64, len(parsed))
	for key, value := range parsed {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		clean[key] = value
	}
	if len(clean) == 0 {
		return nil
	}
	return clean
}

// copyFloatMap duplicates a float map so later mutations do not affect cached
// responses.  We prefer copying over shared state to follow Go's advice of
// communicating values explicitly.
func copyFloatMap(src map[string]float64) map[string]float64 {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]float64, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

// cloneRealtimePoints allocates a fresh slice so callers can trim or
// resample without affecting other views.
func cloneRealtimePoints(points []realtimePoint) []realtimePoint {
	if len(points) == 0 {
		return nil
	}
	out := make([]realtimePoint, len(points))
	copy(out, points)
	return out
}

// filterPointsSince keeps only values with timestamps at or after the cutoff.
// The helper assumes the input slice is sorted by timestamp.
func filterPointsSince(points []realtimePoint, cutoff int64) []realtimePoint {
	if cutoff <= 0 || len(points) == 0 {
		return cloneRealtimePoints(points)
	}
	idx := sort.Search(len(points), func(i int) bool {
		return points[i].Timestamp >= cutoff
	})
	if idx >= len(points) {
		return nil
	}
	return cloneRealtimePoints(points[idx:])
}

// resampleRealtimePoints collapses samples into evenly sized buckets using the
// average value per bucket.  Returning a new slice keeps the caller's data
// immutable and mirrors the "don't mutate shared structures" guidance.
func resampleRealtimePoints(points []realtimePoint, bucket int64) []realtimePoint {
	if bucket <= 0 || len(points) == 0 {
		return cloneRealtimePoints(points)
	}
	out := make([]realtimePoint, 0, len(points))
	currentBucket := (points[0].Timestamp / bucket) * bucket
	var sum float64
	var count int
	for _, p := range points {
		bucketID := (p.Timestamp / bucket) * bucket
		if bucketID != currentBucket && count > 0 {
			out = append(out, realtimePoint{
				Timestamp: currentBucket + bucket/2,
				Value:     sum / float64(count),
			})
			currentBucket = bucketID
			sum = 0
			count = 0
		}
		sum += p.Value
		count++
	}
	if count > 0 {
		out = append(out, realtimePoint{
			Timestamp: currentBucket + bucket/2,
			Value:     sum / float64(count),
		})
	}
	return out
}

// lastNRealtimePoints keeps only the newest buckets so short charts honour fixed
// divisions without redrawing hundreds of samples. Copying avoids sharing slices.
func lastNRealtimePoints(points []realtimePoint, keep int) []realtimePoint {
	if keep <= 0 || len(points) == 0 {
		return nil
	}
	if len(points) <= keep {
		return points
	}
	out := make([]realtimePoint, keep)
	copy(out, points[len(points)-keep:])
	return out
}

// alignToCeil snaps timestamps up to the next step boundary so chart grids show
// whole buckets even when the latest measurement arrives mid-interval.
func alignToCeil(t time.Time, step time.Duration) time.Time {
	if step <= 0 {
		return t.UTC()
	}
	tt := t.UTC()
	truncated := tt.Truncate(step)
	if tt.Equal(truncated) {
		return tt
	}
	return truncated.Add(step)
}

// alignMonthCeil advances to the first day of the next month so the long-term
// chart can display complete calendar months without partial buckets.
func alignMonthCeil(t time.Time) time.Time {
	tt := t.UTC()
	base := time.Date(tt.Year(), tt.Month(), 1, 0, 0, 0, 0, time.UTC)
	return base.AddDate(0, 1, 0)
}

// aggregateMonthly groups samples by calendar month using the average value per
// month. This keeps the "months" chart faithful to calendar boundaries.
func aggregateMonthly(points []realtimePoint, start time.Time, months int) []realtimePoint {
	if months <= 0 || len(points) == 0 {
		return nil
	}
	startUTC := time.Date(start.UTC().Year(), start.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	filtered := filterPointsSince(points, startUTC.Unix())
	if len(filtered) == 0 {
		return nil
	}
	out := make([]realtimePoint, 0, months)
	idx := 0
	for i := 0; i < months; i++ {
		monthStart := startUTC.AddDate(0, i, 0)
		monthEnd := monthStart.AddDate(0, 1, 0)
		startUnix := monthStart.Unix()
		endUnix := monthEnd.Unix()
		for idx < len(filtered) && filtered[idx].Timestamp < startUnix {
			idx++
		}
		if idx >= len(filtered) {
			break
		}
		sum := 0.0
		count := 0
		j := idx
		for j < len(filtered) {
			ts := filtered[j].Timestamp
			if ts >= endUnix {
				break
			}
			sum += filtered[j].Value
			count++
			j++
		}
		if count > 0 {
			mid := startUnix + (endUnix-startUnix)/2
			out = append(out, realtimePoint{Timestamp: mid, Value: sum / float64(count)})
			idx = j
		}
	}
	return out
}

// realtimeBucketSteps enumerates pleasant aggregation steps from minutes to
// months.  The list keeps resampling predictable and friendly on charts.
var realtimeBucketSteps = []int64{
	60,
	120,
	300,
	600,
	900,
	1800,
	3600,
	7200,
	14400,
	28800,
	43200,
	86400,
	172800,
	604800,
	1209600,
	2592000,
	7776000,
}

// pickNiceBucket chooses the smallest bucket from realtimeBucketSteps that is
// greater or equal to the requested minimum.  Falling back to doubling keeps
// the function total when the data spans many years.
func pickNiceBucket(minStep int64) int64 {
	if minStep <= 1 {
		return 1
	}
	for _, step := range realtimeBucketSteps {
		if step >= minStep {
			return step
		}
	}
	return realtimeBucketSteps[len(realtimeBucketSteps)-1]
}

// nextBucket returns the next larger bucket after the provided step.
func nextBucket(step int64) int64 {
	for _, candidate := range realtimeBucketSteps {
		if candidate > step {
			return candidate
		}
	}
	if step <= 0 {
		return 1
	}
	return step * 2
}

// prepareRealtimeSeries optionally resamples a slice so the frontend receives
// at most "limit" points.  When defaultBucket is positive we always aggregate
// using that duration; otherwise the function derives a pleasant step.
func prepareRealtimeSeries(points []realtimePoint, limit int, defaultBucket int64) ([]realtimePoint, int64) {
	if len(points) == 0 {
		return nil, 0
	}
	if defaultBucket > 0 {
		bucket := defaultBucket
		resampled := resampleRealtimePoints(points, bucket)
		for len(resampled) > limit {
			bucket = nextBucket(bucket)
			resampled = resampleRealtimePoints(points, bucket)
		}
		return resampled, bucket
	}
	if len(points) <= limit {
		return cloneRealtimePoints(points), 0
	}
	span := points[len(points)-1].Timestamp - points[0].Timestamp
	if span <= 0 {
		return cloneRealtimePoints(points), 0
	}
	approx := span / int64(limit)
	if approx <= 0 {
		approx = 1
	}
	bucket := pickNiceBucket(approx)
	resampled := resampleRealtimePoints(points, bucket)
	for len(resampled) > limit {
		bucket = nextBucket(bucket)
		resampled = resampleRealtimePoints(points, bucket)
	}
	return resampled, bucket
}

// summariseRealtimeHistory processes DB rows on a background goroutine and
// returns aggregated series for day, month, and all-time windows.
func summariseRealtimeHistory(rows []database.RealtimeMeasurement, now time.Time) historyAggregate {
	input := make(chan realtimeMeasurementPayload)
	go func() {
		defer close(input)
		for _, row := range rows {
			val, ok := safecastrealtime.FromRealtime(row.Value, row.Unit)
			if !ok {
				continue
			}
			payload := realtimeMeasurementPayload{
				timestamp: row.MeasuredAt,
				radiation: safecastrealtime.ToMicroRoentgen(val),
				extras:    decodeRealtimeExtras(row.Extra),
				name:      row.DeviceName,
				transport: row.Transport,
				tube:      row.Tube,
				country:   row.Country,
			}
			input <- payload
		}
	}()

	return collectRealtimeMeasurements(input, now, len(rows))
}

// collectRealtimeMeasurements consumes the measurement stream, keeps track of
// metadata, and prepares slices for each chart timeframe.
func collectRealtimeMeasurements(input <-chan realtimeMeasurementPayload, now time.Time, expected int) historyAggregate {
	agg := historyAggregate{
		Series: map[string][]realtimePoint{
			"day":   {},
			"month": {},
			"all":   {},
		},
		ExtraSeries: map[string]map[string][]realtimePoint{
			"day":   {},
			"month": {},
			"all":   {},
		},
		Ranges: make(map[string]rangeSummary, 3),
	}

	extrasAll := make(map[string][]realtimePoint)
	if expected <= 0 {
		expected = 1024
	}
	allPoints := make([]realtimePoint, 0, expected)
	var lastTs int64

	for payload := range input {
		point := realtimePoint{Timestamp: payload.timestamp, Value: payload.radiation}
		allPoints = append(allPoints, point)
		lastTs = payload.timestamp

		if agg.DeviceName == "" && payload.name != "" {
			agg.DeviceName = payload.name
		}
		if agg.Transport == "" && payload.transport != "" {
			agg.Transport = payload.transport
		}
		if agg.Country == "" && payload.country != "" {
			agg.Country = payload.country
		}
		if agg.Tube == "" {
			if label := safecastrealtime.DetectorLabel(payload.tube, payload.transport, payload.name); label != "" {
				agg.Tube = label
			}
		}

		if len(payload.extras) > 0 {
			agg.Extra = copyFloatMap(payload.extras)
			for key, value := range payload.extras {
				extrasAll[key] = append(extrasAll[key], realtimePoint{Timestamp: payload.timestamp, Value: value})
			}
		}
	}

	reference := now.UTC()
	if lastTs > 0 {
		lastSeen := time.Unix(lastTs, 0).UTC()
		if lastSeen.After(reference) {
			reference = lastSeen
		}
	}

	const (
		hourlySegments  = 24
		dailySegments   = 24
		monthlySegments = 24
	)
	const dayDuration = 24 * time.Hour

	dayEnd := alignToCeil(reference, time.Hour)
	dayStart := dayEnd.Add(-time.Duration(hourlySegments) * time.Hour)
	monthEnd := alignToCeil(reference, dayDuration)
	monthStart := monthEnd.Add(-time.Duration(dailySegments) * dayDuration)
	allEnd := alignMonthCeil(reference)
	allStart := allEnd.AddDate(0, -monthlySegments, 0)

	dayCutoff := dayStart.Unix()
	monthCutoff := monthStart.Unix()
	allCutoff := allStart.Unix()

	hourBucket := int64(time.Hour / time.Second)
	daySeries := lastNRealtimePoints(resampleRealtimePoints(filterPointsSince(allPoints, dayCutoff), hourBucket), hourlySegments)

	dayBucketSeconds := int64(dayDuration / time.Second)
	monthSeries := lastNRealtimePoints(resampleRealtimePoints(filterPointsSince(allPoints, monthCutoff), dayBucketSeconds), dailySegments)

	allSeries := aggregateMonthly(allPoints, allStart, monthlySegments)
	avgBucket := (allEnd.Sub(allStart) / time.Duration(monthlySegments)) / time.Second
	allBucketSeconds := int64(avgBucket)
	if allBucketSeconds <= 0 {
		allBucketSeconds = int64(30 * dayDuration / time.Second)
	}

	agg.Series["day"] = daySeries
	agg.Series["month"] = monthSeries
	agg.Series["all"] = allSeries
	agg.Ranges["day"] = rangeSummary{Start: dayCutoff, End: dayEnd.Unix(), BucketSeconds: hourBucket}
	agg.Ranges["month"] = rangeSummary{Start: monthCutoff, End: monthEnd.Unix(), BucketSeconds: dayBucketSeconds}
	agg.Ranges["all"] = rangeSummary{Start: allCutoff, End: allEnd.Unix(), BucketSeconds: allBucketSeconds}

	buildExtras := func(target string, cutoff int64, bucket int64) {
		if len(extrasAll) == 0 {
			return
		}
		series := make(map[string][]realtimePoint)
		for key, pts := range extrasAll {
			filtered := filterPointsSince(pts, cutoff)
			if len(filtered) == 0 {
				continue
			}
			if bucket > 0 {
				series[key] = resampleRealtimePoints(filtered, bucket)
			} else {
				series[key] = filtered
			}
		}
		if len(series) > 0 {
			agg.ExtraSeries[target] = series
		}
	}

	buildMonthlyExtras := func(target string, start time.Time, months int) {
		if len(extrasAll) == 0 {
			return
		}
		series := make(map[string][]realtimePoint)
		for key, pts := range extrasAll {
			aggregated := aggregateMonthly(pts, start, months)
			if len(aggregated) == 0 {
				continue
			}
			series[key] = aggregated
		}
		if len(series) > 0 {
			agg.ExtraSeries[target] = series
		}
	}

	buildExtras("day", dayCutoff, hourBucket)
	buildExtras("month", monthCutoff, dayBucketSeconds)
	buildMonthlyExtras("all", allStart, monthlySegments)

	if agg.Series["day"] == nil {
		agg.Series["day"] = []realtimePoint{}
	}
	if agg.Series["month"] == nil {
		agg.Series["month"] = []realtimePoint{}
	}
	if agg.Series["all"] == nil {
		agg.Series["all"] = []realtimePoint{}
	}

	return agg
}

// realtimeHistoryHandler returns one year of realtime measurements for a device.
// The handler keeps the response lightweight so the frontend can draw Grafana-style
// charts without shipping a dedicated dashboard backend.
func realtimeHistoryHandler(w http.ResponseWriter, r *http.Request) {
	if !*safecastRealtimeEnabled {
		http.NotFound(w, r)
		return
	}

	device := strings.TrimSpace(r.URL.Query().Get("device"))
	if device == "" {
		http.Error(w, "missing device", http.StatusBadRequest)
		return
	}
	if strings.HasPrefix(device, "live:") {
		device = strings.TrimPrefix(device, "live:")
	}

	now := time.Now()
	rows, err := db.GetRealtimeHistory(device, 0, *dbType)
	if err != nil {
		http.Error(w, "history error", http.StatusInternalServerError)
		return
	}

	historyCh := make(chan historyAggregate, 1)
	go func() {
		historyCh <- summariseRealtimeHistory(rows, now)
	}()

	var agg historyAggregate
	select {
	case <-r.Context().Done():
		http.Error(w, "request cancelled", http.StatusRequestTimeout)
		return
	case agg = <-historyCh:
	}

	resp := struct {
		DeviceID    string                                `json:"deviceID"`
		DeviceName  string                                `json:"deviceName,omitempty"`
		Transport   string                                `json:"transport,omitempty"`
		Tube        string                                `json:"tube,omitempty"`
		Country     string                                `json:"country,omitempty"`
		Series      map[string][]realtimePoint            `json:"series"`
		Extra       map[string]float64                    `json:"extra,omitempty"`
		ExtraSeries map[string]map[string][]realtimePoint `json:"extraSeries,omitempty"`
		Ranges      map[string]rangeSummary               `json:"ranges,omitempty"`
	}{
		DeviceID:    device,
		DeviceName:  agg.DeviceName,
		Transport:   agg.Transport,
		Tube:        agg.Tube,
		Country:     agg.Country,
		Series:      agg.Series,
		Extra:       agg.Extra,
		ExtraSeries: agg.ExtraSeries,
		Ranges:      agg.Ranges,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// =====================
// MAIN
// =====================

// main parses flags, initialises the DB & routes, then either
// (a) serves plain HTTP on a custom port, or
// (b) if -domain is given, serves ACME-backed HTTPS on 443 plus
//     an ACME/redirect helper on 80.
//
// If any web-server returns an error it is only logged – the
// application continues running.  A final `select{}` keeps the
// main goroutine alive without resorting to mutexes.

// main: парсинг флагов, инициализация БД и запуск веб-серверов.
// Добавлен withServerHeader для всех запросов.
// =====================
// MAIN
// =====================
func main() {
	// 1. Флаги и версии
	flag.Parse()
	debugIPAllowlist = parseDebugAllowlist(*debugIPsFlag)
	loadTranslations(content, "public_html/translations.json")
	selfupgradeStartupDelay(log.Printf)

	archiveFrequency, freqErr := jsonarchive.ParseFrequency(*jsonArchiveFrequencyFlag)
	if freqErr != nil {
		log.Fatalf("json archive frequency: %v", freqErr)
	}
	archiveFileName := jsonarchive.FileName(*domain, archiveFrequency)

	if *version {
		fmt.Printf("chicha-isotope-map version %s\n", CompileVersion)
		return
	}

	// 2. Предупреждение о привилегиях (для :80 / :443)
	if *domain != "" && runtime.GOOS != "windows" && os.Geteuid() != 0 {
		log.Println("⚠  Binding to :80 / :443 requires super-user rights; run with sudo or as root.")
	}

	// 3. База данных
	driverName := strings.ToLower(strings.TrimSpace(*dbType))
	// Persist the normalized driver back into the flag so downstream helpers never
	// miss engine-specific branches because of incidental casing or whitespace.
	*dbType = driverName
	dbCfg := database.Config{
		DBType: driverName,
		DBPath: *dbPath,
		Port:   *port,
	}
	switch driverName {
	case "pgx":
		dbCfg.DBHost = "127.0.0.1"
		dbCfg.DBPort = 5432
		dbCfg.DBUser = "postgres"
		dbCfg.DBName = "IsotopePathways"
		dbCfg.PGSSLMode = "prefer"
		if err := applyDBConnection(driverName, *dbConn, &dbCfg); err != nil {
			log.Fatalf("DB config: %v", err)
		}
	case "clickhouse":
		dbCfg.DBHost = "127.0.0.1"
		dbCfg.DBPort = 9000
		dbCfg.DBName = "IsotopePathways"
		if err := applyDBConnection(driverName, *dbConn, &dbCfg); err != nil {
			log.Fatalf("DB config: %v", err)
		}
	default:
		if strings.TrimSpace(*dbConn) != "" {
			log.Fatalf("db-conn is only valid for pgx or clickhouse drivers (current: %s)", *dbType)
		}
	}
	var err error
	db, err = database.NewDatabase(dbCfg)
	if err != nil {
		log.Fatalf("DB init: %v", err)
	}
	if err = db.InitSchema(dbCfg); err != nil {
		log.Fatalf("DB schema: %v", err)
	}

	remoteURL := strings.TrimSpace(*importTGZURLFlag)
	localArchive := strings.TrimSpace(*importTGZFileFlag)
	if remoteURL != "" && localArchive != "" {
		log.Fatalf("choose only one import flag: -import-tgz-url or -import-tgz-file")
	}

	if localArchive != "" {
		ctxImport, cancelImport := context.WithCancel(context.Background())
		defer cancelImport()

		fallback := GenerateSerialNumber()
		if err := importArchiveFromFile(ctxImport, localArchive, fallback, db, driverName, log.Printf); err != nil {
			log.Fatalf("local tgz import: %v", err)
		}

		log.Printf("local tgz import finished; exiting per -import-tgz-file")
		return
	}

	if remoteURL != "" {
		ctxImport, cancelImport := context.WithCancel(context.Background())
		defer cancelImport()

		fallback := GenerateSerialNumber()
		if err := importArchiveFromURL(ctxImport, remoteURL, fallback, db, driverName, log.Printf); err != nil {
			log.Fatalf("remote tgz import: %v", err)
		}

		log.Printf("remote tgz import finished; exiting per -import-tgz-url")
		return
	}

	if *safecastRealtimeEnabled {
		// Launch realtime Safecast polling under the dedicated flag so the
		// feature stays opt-in.
		database.SetRealtimeConverter(safecastrealtime.FromRealtime)
		ctxRT, cancelRT := context.WithCancel(context.Background())
		defer cancelRT()
		safecastrealtime.Start(ctxRT, db, *dbType, log.Printf)
	}

	// Build a JSON archive tgz with all known exported tracks only when
	// operators explicitly opt in via -json-archive-path. The cadence flag
	// keeps IO predictable while letting deployments choose how fresh the
	// bundle should be.
	var (
		archiveGen     *jsonarchive.Generator
		archiveCancel  context.CancelFunc
		archivePath    string
		archiveEnabled = strings.TrimSpace(*jsonArchivePathFlag) != ""
	)
	if archiveEnabled {
		ctxArchive, cancelArchive := context.WithCancel(context.Background())
		archiveCancel = cancelArchive
		archivePath = resolveArchivePath(*jsonArchivePathFlag, archiveFileName, log.Printf)
		if abs, err := filepath.Abs(archivePath); err == nil {
			archivePath = abs
		}
		log.Printf("json archive destination resolved: %s", archivePath)
		archiveGen = jsonarchive.Start(ctxArchive, db, *dbType, archivePath, archiveFileName, archiveFrequency.Interval(), log.Printf)
	} else {
		log.Printf("json archive disabled: set -json-archive-path to enable tarball generation")
	}
	if archiveCancel != nil {
		defer archiveCancel()
	}

	apiDocsArchiveEnabled = archiveEnabled
	route := archiveFrequency.RoutePath()
	if strings.TrimSpace(route) == "" {
		route = "/api/json/weekly.tgz"
	}
	apiDocsArchiveRoute = route
	apiDocsArchiveFrequency = archiveFrequency.HumanInterval()

	// 4. Маршруты и статика
	staticFS, err := fs.Sub(content, "public_html")
	if err != nil {
		log.Fatalf("static fs: %v", err)
	}

	http.Handle("/static/", http.StripPrefix("/static/",
		http.FileServer(http.FS(staticFS))))
	http.HandleFunc("/", mapHandler)
	// Serve license documents straight from the embedded filesystem so UI
	// modals can reuse the same source without relying on external storage.
	http.HandleFunc("/licenses/", licenseHandler)
	http.HandleFunc("/upload", uploadHandler)
	http.HandleFunc("/get_markers", getMarkersHandler)
	http.HandleFunc("/stream_markers", streamMarkersHandler)
	http.HandleFunc("/realtime_history", realtimeHistoryHandler)
	http.HandleFunc("/trackid/", trackHandler)
	http.HandleFunc("/qrpng", qrPngHandler)
	http.HandleFunc("/s/", shortRedirectHandler)
	http.HandleFunc("/api/docs", apiDocsHandler)

	// API endpoints ship JSON/archives. Keeping registration close to other
	// routes avoids surprises for operators scanning main() for handlers.
	limiter := api.NewRateLimiter(time.Minute)
	apiHandler := api.NewHandler(db, *dbType, archiveGen, limiter, log.Printf, archiveFrequency)
	apiHandler.Register(http.DefaultServeMux)

	// Selfupgrade runs in the background only when explicitly enabled so existing
	// installations keep their manual release cadence. We assemble the config
	// near main() so filesystem paths, database settings, and HTTP handlers stay
	// consistent with the rest of the binary.
	selfUpgradeCancel := startSelfUpgrade(context.Background(), dbCfg)
	if selfUpgradeCancel != nil {
		defer selfUpgradeCancel()
	}

	rootHandler := withServerHeader(http.DefaultServeMux)

	// 5. HTTP/HTTPS-серверы
	if *domain != "" {
		// Двойной сервер :80 + :443 с Let’s Encrypt
		go serveWithDomain(*domain, rootHandler)
	} else {
		// Обычный HTTP на порт из -port
		addr := fmt.Sprintf(":%d", *port)
		go func() {
			log.Printf("HTTP server ➜ http://localhost:%s", addr)
			if err := http.ListenAndServe(addr, rootHandler); err != nil {
				selfupgradeHandleServerError(err, log.Printf)
			}
		}()
	}

	// асинхронные индексы в бд без блокирования основного процесса начало
	ctxIdx, cancelIdx := context.WithCancel(context.Background())
	defer cancelIdx()
	// Пояснение в лог: что делаем и почему это не блокирует сервер
	log.Printf("⏳ background index build scheduled (engine=%s). Listeners are up; pages may be slower until indexes are ready.", dbCfg.DBType)
	// Запуск асинхронной индексации с прогрессом
	db.EnsureIndexesAsync(ctxIdx, dbCfg, func(format string, args ...any) {
		log.Printf(format, args...)
	})
	// асинхронные индексы в бд без блокирования основного процесса конец

	// 6. Держим main-goroutine живой
	select {}
}
