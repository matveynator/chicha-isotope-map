package setupwizard

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Defaults carries the starting values presented by the wizard so operators
// can speed through with sensible answers while still being free to tweak them.
// Keeping the struct small matches the Go proverb about simplicity beating
// cleverness: we only hold what the service file needs instead of mirroring
// every CLI flag.
type Defaults struct {
	Port         int
	Domain       string
	NeedCert     bool
	DBType       string
	DBPath       string
	DBConn       string
	PGHost       string
	PGPort       string
	PGUser       string
	PGPassword   string
	PGDatabase   string
	SafecastLive bool
	ArchivePath  string
	ImportEnable bool
	ImportTGZURL string
	SupportEmail string
	BinaryPath   string
	WorkingDir   string
}

// Result summarises what the wizard wrote to disk so the caller can show
// follow-up instructions. Returning structured data instead of printing from
// the installer makes the function easier to test and keeps side effects in one
// place.
type Result struct {
	ServiceName string
	ServicePath string
	LogPath     string
	UserUnit    bool
	ExecStart   []string
	Commands    []string
	EnableNotes []string
}

// colorTheme mirrors the lightweight ANSI palette used in the main package.
// We keep it here instead of reusing an exported type to avoid coupling the
// wizard to CLI formatting internals.
type colorTheme struct {
	Enabled bool
	Accent  string
	Prompt  string
	Success string
	Reset   string
}

// resolveTheme enables colours only when stdout points at a TTY, respecting
// NO_COLOR so automation remains readable. The helper returns a neutral theme
// when ANSI is unsuitable.
func resolveTheme(out io.Writer) colorTheme {
	theme := colorTheme{}
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
	theme.Accent = "\033[38;5;39m"
	theme.Prompt = "\033[38;5;214m"
	theme.Success = "\033[38;5;70m"
	theme.Reset = "\033[0m"
	return theme
}

// Run guides the operator through a coloured interactive setup, writes a
// systemd unit, and tries to enable it. Everything is time-bound via context so
// the caller can abort cleanly without mutexes or global state.
func Run(ctx context.Context, in io.Reader, out io.Writer, defaults Defaults) (Result, error) {
	if runtime.GOOS != "linux" {
		return Result{}, errors.New("setup wizard is only available on Linux")
	}

	theme := resolveTheme(out)
	reader := bufio.NewReader(in)

	fmt.Fprintf(out, "\n%s🛠  Quick setup (Linux)%s\n", theme.AccentIfEnabled(), theme.ResetIfEnabled())

	answers := enrichDefaults(defaults)
outer:
	for {
		answers.NeedCert = promptYesNo(ctx, reader, out, theme, "Issue HTTPS certificate", answers.NeedCert)
		if answers.NeedCert {
			fmt.Fprintf(out, "%sKeeps 80 and 443 reserved for TLS challenges and traffic.%s\n", theme.AccentIfEnabled(), theme.ResetIfEnabled())
			answers.Domain = promptWithDefault(ctx, reader, out, theme, "Domain", defaultOr(answers.Domain, "maps.example.org"))
			answers.Port = 443
		} else {
			answers.Domain = ""
			answers.Port = promptPort(ctx, reader, out, theme, "HTTP port", suggestPort(false, answers.Port))
		}

		options := availableDBTypes()
		fmt.Fprintf(out, "%sDatabase:%s sqlite/chai store files; pgx is PostgreSQL; clickhouse fits analytics.%s\n", theme.AccentIfEnabled(), theme.ResetIfEnabled(), theme.ResetIfEnabled())
		if duckDBBuilt {
			fmt.Fprintf(out, "%sDuckDB appears only when built with duckdb tag.%s\n", theme.AccentIfEnabled(), theme.ResetIfEnabled())
		}
		answers.DBType = promptChoice(ctx, reader, out, theme, "Database", options, pickDefault(options, answers.DBType))

		dbPath, dbConn := promptDatabaseConfig(ctx, reader, out, theme, &answers)
		answers.DBPath = dbPath
		answers.DBConn = dbConn

		fmt.Fprintf(out, "%sSupport contact:%s short note for users who need help.%s\n", theme.AccentIfEnabled(), theme.ResetIfEnabled(), theme.ResetIfEnabled())
		answers.SupportEmail = promptWithDefault(ctx, reader, out, theme, "Support e-mail", answers.SupportEmail)

		fmt.Fprintf(out, "%sRealtime:%s toggle Safecast live devices.%s\n", theme.AccentIfEnabled(), theme.ResetIfEnabled(), theme.ResetIfEnabled())
		answers.SafecastLive = promptYesNo(ctx, reader, out, theme, "Enable Safecast realtime", answers.SafecastLive)

		suggestedArchive := suggestArchivePath(answers.ArchivePath, answers.Port)
		fmt.Fprintf(out, "%sArchives:%s JSON dumps live under a port-specific folder.%s\n", theme.AccentIfEnabled(), theme.ResetIfEnabled(), theme.ResetIfEnabled())
		answers.ArchivePath = promptWithDefault(ctx, reader, out, theme, "Archive dir", suggestedArchive)

		fmt.Fprintf(out, "%sImport data:%s optional first sync before the service starts.%s\n", theme.AccentIfEnabled(), theme.ResetIfEnabled(), theme.ResetIfEnabled())
		answers.ImportEnable = promptYesNo(ctx, reader, out, theme, "Fetch initial .tgz", answers.ImportEnable)
		if answers.ImportEnable {
			answers.ImportTGZURL = promptWithDefault(ctx, reader, out, theme, "Import .tgz URL", defaultOr(answers.ImportTGZURL, "https://pelora.org/api/json/weekly.tgz"))
		} else {
			answers.ImportTGZURL = ""
		}

		port := answers.Port
		unitPath, userUnit, err := resolveServiceDestination(port)
		if err != nil {
			return Result{}, err
		}

		execPath := answers.BinaryPath
		if execPath == "" {
			execPath, err = os.Executable()
			if err != nil {
				return Result{}, fmt.Errorf("resolve binary path: %w", err)
			}
		}
		execPath, _ = filepath.Abs(execPath)

		if answers.WorkingDir == "" {
			answers.WorkingDir, _ = filepath.Abs(filepath.Dir(execPath))
		}

		args := buildExecArgs(execPath, port, answers.Domain, answers.DBType, answers.DBPath, answers.DBConn, answers.SupportEmail, answers.SafecastLive, answers.ArchivePath, answers.ImportEnable, answers.ImportTGZURL)

		logPath, err := resolveLogPath(userUnit, port)
		if err != nil {
			return Result{}, err
		}

		for {
			fmt.Fprintf(out, "\n%sReview:%s\n", theme.AccentIfEnabled(), theme.ResetIfEnabled())
			fmt.Fprintf(out, "  [1] HTTPS:   %s\n", formatHTTPSChoice(answers.NeedCert, answers.Domain))
			fmt.Fprintf(out, "  [2] Port:    %d\n", port)
			fmt.Fprintf(out, "  [3] DB:      %s\n", answers.DBType)
			fmt.Fprintf(out, "  [4] Support: %s\n", displayValue(answers.SupportEmail))
			fmt.Fprintf(out, "  [5] Live:    %t\n", answers.SafecastLive)
			fmt.Fprintf(out, "  [6] Archive: %s\n", displayValue(answers.ArchivePath))
			fmt.Fprintf(out, "  [7] Import:  %s\n", formatImportChoice(answers.ImportEnable, answers.ImportTGZURL))
			fmt.Fprintf(out, "      Service: %s\n      Logs:    %s\n", unitPath, logPath)

			action := promptWithDefault(ctx, reader, out, theme, "apply / edit number / restart / quit", "apply")
			action = strings.ToLower(strings.TrimSpace(action))

			if action == "apply" || action == "" {
				break
			}
			if action == "restart" {
				fmt.Fprintf(out, "%sRestarting with current answers as defaults.%s\n\n", theme.PromptIfEnabled(), theme.ResetIfEnabled())
				continue outer
			}
			if action == "quit" {
				return Result{}, errors.New("setup wizard cancelled by user")
			}

			changed := false
			switch action {
			case "1":
				answers.NeedCert = promptYesNo(ctx, reader, out, theme, "Issue HTTPS certificate", answers.NeedCert)
				if answers.NeedCert {
					fmt.Fprintf(out, "%s80 and 443 stay reserved for HTTPS.%s\n", theme.AccentIfEnabled(), theme.ResetIfEnabled())
					answers.Domain = promptWithDefault(ctx, reader, out, theme, "Domain", defaultOr(answers.Domain, "maps.example.org"))
					answers.Port = 443
				} else {
					answers.Domain = ""
					answers.Port = promptPort(ctx, reader, out, theme, choosePortLabel(false), suggestPort(false, answers.Port))
				}
				port = answers.Port
				changed = true
			case "2":
				if answers.NeedCert {
					fmt.Fprintf(out, "%sHTTPS keeps port at 443 for clarity.%s\n", theme.PromptIfEnabled(), theme.ResetIfEnabled())
					answers.Port = 443
				} else {
					answers.Port = promptPort(ctx, reader, out, theme, choosePortLabel(false), answers.Port)
				}
				port = answers.Port
				changed = true
			case "3":
				options = availableDBTypes()
				answers.DBType = promptChoice(ctx, reader, out, theme, "Database engine", options, pickDefault(options, answers.DBType))
				answers.DBPath, answers.DBConn = promptDatabaseConfig(ctx, reader, out, theme, &answers)
				changed = true
			case "4":
				answers.SupportEmail = promptWithDefault(ctx, reader, out, theme, "Support e-mail", answers.SupportEmail)
				changed = true
			case "5":
				answers.SafecastLive = promptYesNo(ctx, reader, out, theme, "Enable Safecast realtime", answers.SafecastLive)
				changed = true
			case "6":
				suggestedArchive = suggestArchivePath(answers.ArchivePath, answers.Port)
				answers.ArchivePath = promptWithDefault(ctx, reader, out, theme, "JSON archive directory", suggestedArchive)
				changed = true
			case "7":
				answers.ImportEnable = promptYesNo(ctx, reader, out, theme, "Fetch initial import .tgz", answers.ImportEnable)
				if answers.ImportEnable {
					answers.ImportTGZURL = promptWithDefault(ctx, reader, out, theme, "Import .tgz URL", defaultOr(answers.ImportTGZURL, "https://pelora.org/api/json/weekly.tgz"))
				} else {
					answers.ImportTGZURL = ""
				}
				changed = true
			}
			if changed {
				unitPath, userUnit, err = resolveServiceDestination(answers.Port)
				if err != nil {
					return Result{}, err
				}
				logPath, err = resolveLogPath(userUnit, answers.Port)
				if err != nil {
					return Result{}, err
				}
				port = answers.Port
				continue
			}
		}

		port = answers.Port
		args = buildExecArgs(execPath, port, answers.Domain, answers.DBType, answers.DBPath, answers.DBConn, answers.SupportEmail, answers.SafecastLive, answers.ArchivePath, answers.ImportEnable, answers.ImportTGZURL)
		logPath, err = resolveLogPath(userUnit, port)
		if err != nil {
			return Result{}, err
		}
		if err := prepareDBPath(answers.DBType, answers.DBPath); err != nil {
			return Result{}, err
		}
		if err := prepareArchivePath(answers.ArchivePath); err != nil {
			return Result{}, err
		}

		if err := writeServiceFile(unitPath, answers.WorkingDir, logPath, args, userUnit); err != nil {
			return Result{}, err
		}

		result := Result{
			ServiceName: filepath.Base(unitPath),
			ServicePath: unitPath,
			LogPath:     logPath,
			UserUnit:    userUnit,
			ExecStart:   args,
			Commands:    systemctlCommands(userUnit, filepath.Base(unitPath)),
		}

		enableCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		enableResults := runSystemctl(enableCtx, result.Commands)

		for res := range enableResults {
			if res.Err != nil {
				result.EnableNotes = append(result.EnableNotes, fmt.Sprintf("%s (%s)", res.Message, res.Err))
				continue
			}
			if strings.TrimSpace(res.Output) != "" {
				result.EnableNotes = append(result.EnableNotes, res.Output)
			}
		}

		fmt.Fprintf(out, "\n%s✔ Service written to %s%s\n", theme.SuccessIfEnabled(), unitPath, theme.ResetIfEnabled())
		fmt.Fprintf(out, "%sExecStart:%s %s\n", theme.AccentIfEnabled(), theme.ResetIfEnabled(), strings.Join(args, " "))
		printNextSteps(out, theme, result)
		appendProfilePrimer(result)
		printUsageHint(out, theme, answers.Domain, answers.Port)

		if err := createUpdateScript(result); err != nil {
			fmt.Fprintf(out, "\n%sUpdater:%s could not write /usr/local/bin/chicha-update (%v).\n", theme.PromptIfEnabled(), theme.ResetIfEnabled(), err)
		} else {
			fmt.Fprintf(out, "\n%sUpdater:%s wrote /usr/local/bin/chicha-update for one-command upgrades.\n", theme.AccentIfEnabled(), theme.ResetIfEnabled())
		}

		return result, nil
	}
}

// availableDBTypes lists engines compiled into the binary. DuckDB is opt-in via
// the duckdb build tag, so the wizard hides it when absent while keeping the
// order predictable for muscle memory.
func availableDBTypes() []string {
	types := []string{"sqlite", "chai", "pgx", "clickhouse"}
	if duckDBBuilt {
		types = append([]string{"duckdb"}, types...)
	}
	return types
}

// enrichDefaults derives per-field defaults so restarts can reuse the latest
// answers. When a PostgreSQL connection string is present, we parse it into the
// individual prompts to keep the experience consistent with Go's preference for
// explicit state.
func enrichDefaults(defaults Defaults) Defaults {
	if strings.TrimSpace(defaults.Domain) != "" {
		defaults.NeedCert = true
	}
	if defaults.BinaryPath == "" {
		if exe, err := os.Executable(); err == nil {
			defaults.BinaryPath = exe
		}
	}
	if defaults.WorkingDir == "" {
		if wd, err := os.Getwd(); err == nil {
			defaults.WorkingDir = wd
		}
	}
	if strings.TrimSpace(defaults.ImportTGZURL) != "" {
		defaults.ImportEnable = true
	}
	if defaults.DBType != "pgx" || strings.TrimSpace(defaults.DBConn) == "" {
		return defaults
	}
	parsed, err := url.Parse(defaults.DBConn)
	if err != nil {
		return defaults
	}
	if defaults.PGHost == "" {
		defaults.PGHost = parsed.Hostname()
	}
	if defaults.PGPort == "" {
		defaults.PGPort = parsed.Port()
	}
	if parsed.User != nil {
		if defaults.PGUser == "" {
			defaults.PGUser = parsed.User.Username()
		}
		if defaults.PGPassword == "" {
			if pw, ok := parsed.User.Password(); ok {
				defaults.PGPassword = pw
			}
		}
	}
	if defaults.PGDatabase == "" {
		defaults.PGDatabase = strings.TrimPrefix(parsed.Path, "/")
	}
	return defaults
}

// pickDefault ensures the chosen default is visible in the options list. If an
// old value no longer applies, the wizard falls back to the first item so the
// prompt remains consistent.
func pickDefault(options []string, def string) string {
	for _, opt := range options {
		if strings.EqualFold(opt, def) {
			return opt
		}
	}
	return options[0]
}

// choosePortLabel keeps the wording short while hinting at the best practice
// for TLS setups. The split keeps the prompts tidy and close to the decision
// about certificates.
func choosePortLabel(needCert bool) string {
	if needCert {
		return "HTTPS port (443 recommended)"
	}
	return "HTTP port (e.g. 8765)"
}

// suggestPort proposes a sensible port based on whether TLS is requested. When
// switching to HTTPS we lean toward 443 unless the operator already chose
// something explicit.
func suggestPort(needCert bool, current int) int {
	if needCert && (current == 0 || current == 8765) {
		return 443
	}
	if current > 0 {
		return current
	}
	return 8765
}

// promptPort asks for the listening port and retries on invalid input so the
// wizard never aborts due to a typo. The select-based reader keeps the flow
// cancellable without locks.
func promptPort(ctx context.Context, reader *bufio.Reader, out io.Writer, theme colorTheme, label string, current int) int {
	for {
		portStr := promptWithDefault(ctx, reader, out, theme, label, strconv.Itoa(current))
		port, err := strconv.Atoi(portStr)
		if err == nil && port > 0 {
			return port
		}
		fmt.Fprintf(out, "%sPlease enter a positive port number.%s\n", theme.PromptIfEnabled(), theme.ResetIfEnabled())
	}
}

// promptDigits keeps numeric string fields (like PostgreSQL port) constrained to digits only.
// Using a loop instead of panicking mirrors the Go proverb about solid, direct code.
func promptDigits(ctx context.Context, reader *bufio.Reader, out io.Writer, theme colorTheme, label, current string) string {
	cleaned := strings.TrimSpace(current)
	if cleaned == "" {
		cleaned = "0"
	}
	for {
		answer := promptWithDefault(ctx, reader, out, theme, label, cleaned)
		if _, err := strconv.Atoi(answer); err == nil {
			return answer
		}
		fmt.Fprintf(out, "%sDigits only, please.%s\n", theme.PromptIfEnabled(), theme.ResetIfEnabled())
	}
}

// promptDatabaseConfig prints detailed hints for the selected database and
// returns the appropriate path or connection string. File databases get a
// suggested /var/lib directory that includes the port for clarity, while
// network databases receive structured prompts. Channel-based prompts keep the
// flow cancellable.
func promptDatabaseConfig(ctx context.Context, reader *bufio.Reader, out io.Writer, theme colorTheme, answers *Defaults) (string, string) {
	if answers.DBType == "pgx" {
		fmt.Fprintf(out, "%sPostgreSQL (pgx driver):%s defaults assume a local server with an empty password. Adjust to match your cluster.%s\n", theme.AccentIfEnabled(), theme.ResetIfEnabled(), theme.ResetIfEnabled())
		host := promptWithDefault(ctx, reader, out, theme, "Host", defaultOr(answers.PGHost, "localhost"))
		port := promptDigits(ctx, reader, out, theme, "Port", defaultOr(answers.PGPort, "5432"))
		user := promptWithDefault(ctx, reader, out, theme, "User", defaultOr(answers.PGUser, "postgres"))
		password := promptWithDefault(ctx, reader, out, theme, "Password (leave empty for trust/local auth)", answers.PGPassword)
		dbname := promptWithDefault(ctx, reader, out, theme, "Database name", defaultOr(answers.PGDatabase, "chicha"))
		answers.PGHost, answers.PGPort, answers.PGUser, answers.PGPassword, answers.PGDatabase = host, port, user, password, dbname
		return "", buildPostgresURI(host, port, user, password, dbname)
	}

	if answers.DBType == "clickhouse" {
		fmt.Fprintf(out, "%sClickHouse:%s provide a native or HTTP URI; defaults stay empty so your existing config remains untouched.%s\n", theme.AccentIfEnabled(), theme.ResetIfEnabled(), theme.ResetIfEnabled())
		return "", promptWithDefault(ctx, reader, out, theme, "Connection URI", answers.DBConn)
	}

	defaultPath := suggestFileDBPath(answers.DBType, answers.Port, answers.DBPath)
	fmt.Fprintf(out, "%sFile database:%s the wizard will create directories if missing. Suggested path keeps port in the folder for clarity.%s\n", theme.AccentIfEnabled(), theme.ResetIfEnabled(), theme.ResetIfEnabled())
	return promptWithDefault(ctx, reader, out, theme, "Database file path", defaultPath), ""
}

// buildPostgresURI assembles a connection string while omitting the colon when
// the password is empty. Keeping the string builder here avoids surprises in
// the calling flow.
func buildPostgresURI(host, port, user, password, dbname string) string {
	cred := user
	if password != "" {
		cred = fmt.Sprintf("%s:%s", user, password)
	}
	return fmt.Sprintf("postgres://%s@%s:%s/%s", cred, host, port, dbname)
}

// suggestFileDBPath proposes a stable location under /var/lib that carries the
// driver name and port number. Extensions are kept simple to match each engine.
func suggestFileDBPath(dbType string, port int, existing string) string {
	if strings.TrimSpace(existing) != "" {
		return existing
	}
	baseDir := fmt.Sprintf("/var/lib/%s-%d", dbType, port)
	name := map[string]string{
		"sqlite":     "database.sqlite",
		"duckdb":     "database.duckdb",
		"chai":       "database.chai",
		"clickhouse": "data.clickhouse",
	}[dbType]
	if name == "" {
		name = "database.db"
	}
	return filepath.Join(baseDir, name)
}

// suggestArchivePath proposes a stable archive directory that mirrors the
// port-based service naming. Using /backup keeps exports separate from the data
// directory while staying predictable when multiple services run side by side.
func suggestArchivePath(existing string, port int) string {
	if strings.TrimSpace(existing) != "" {
		return existing
	}
	return filepath.Join("/backup", fmt.Sprintf("chicha-json-%d", port))
}

// prepareDBPath creates the directory tree for file databases so systemd never
// fails on startup due to missing folders. Network databases skip this step.
func prepareDBPath(dbType, dbPath string) error {
	if dbType == "pgx" || dbType == "clickhouse" || strings.TrimSpace(dbPath) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return fmt.Errorf("create db directory: %w", err)
	}
	return nil
}

// prepareArchivePath ensures the archive destination exists before the service
// starts so scheduled exports never fail on missing directories.
func prepareArchivePath(path string) error {
	cleaned := strings.TrimSpace(path)
	if cleaned == "" {
		return nil
	}
	if err := os.MkdirAll(cleaned, 0o755); err != nil {
		return fmt.Errorf("create archive directory: %w", err)
	}
	return nil
}

// displayValue converts empty strings into a human-friendly placeholder for the
// review line, keeping the summary readable even when defaults are blank.
func displayValue(v string) string {
	if strings.TrimSpace(v) == "" {
		return "(empty)"
	}
	return v
}

// formatHTTPSChoice keeps the review summary compact while showing whether a
// certificate will be requested and which domain will be used.
func formatHTTPSChoice(needCert bool, domain string) string {
	if !needCert {
		return "no (HTTP only)"
	}
	return fmt.Sprintf("yes (%s, ports 80+443)", displayValue(domain))
}

// formatImportChoice keeps the review summary compact and explicit about
// whether the operator asked for an initial import and from where.
func formatImportChoice(enabled bool, url string) string {
	if !enabled {
		return "skip"
	}
	return displayValue(url)
}

// defaultOr falls back when the candidate string is empty. This keeps prompt
// defaults meaningful even when previous values were blank.
func defaultOr(candidate, fallback string) string {
	if strings.TrimSpace(candidate) != "" {
		return candidate
	}
	return fallback
}

// promptWithDefault renders a coloured prompt and waits for input without
// blocking the main goroutine. Using a goroutine plus select lets callers cancel
// cleanly via context while keeping the code free from mutexes.
func promptWithDefault(ctx context.Context, reader *bufio.Reader, out io.Writer, theme colorTheme, label, def string) string {
	fmt.Fprintf(out, "%s❯ %s%s [%s]: %s", theme.PromptIfEnabled(), label, theme.ResetIfEnabled(), def, theme.ResetIfEnabled())
	line, err := readLine(ctx, reader)
	if err != nil {
		return def
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return def
	}
	return trimmed
}

// promptChoice shows a list of options, highlights the default, and reuses the
// same channel-based reader so the wizard remains responsive to cancellation.
func promptChoice(ctx context.Context, reader *bufio.Reader, out io.Writer, theme colorTheme, label string, options []string, def string) string {
	fmt.Fprintf(out, "%s❯ %s%s\n", theme.PromptIfEnabled(), label, theme.ResetIfEnabled())
	defaultIndex := 0
	for i, opt := range options {
		if strings.EqualFold(opt, def) {
			defaultIndex = i
			break
		}
	}
	for i, opt := range options {
		marker := " "
		if i == defaultIndex {
			marker = "*"
		}
		fmt.Fprintf(out, "  [%d] %s %s\n", i+1, marker, opt)
	}
	fmt.Fprintf(out, "%sSelect option [%d]: %s", theme.PromptIfEnabled(), defaultIndex+1, theme.ResetIfEnabled())
	line, err := readLine(ctx, reader)
	if err != nil {
		return options[defaultIndex]
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return options[defaultIndex]
	}
	idx, err := strconv.Atoi(trimmed)
	if err != nil || idx < 1 || idx > len(options) {
		return options[defaultIndex]
	}
	return options[idx-1]
}

// promptYesNo keeps boolean prompts consistent by mapping to a short yes/no
// chooser. Returning a bool avoids string parsing downstream and keeps the
// control flow obvious for future maintainers.
func promptYesNo(ctx context.Context, reader *bufio.Reader, out io.Writer, theme colorTheme, label string, current bool) bool {
	options := []string{"no", "yes"}
	def := 1
	if !current {
		def = 0
	}
	choice := promptChoice(ctx, reader, out, theme, label, options, options[def])
	return strings.EqualFold(choice, "yes")
}

// readLine reads from stdin in a goroutine so the select can react to context
// cancellation without extra locking. The buffering keeps the wizard snappy
// even on slow terminals.
func readLine(ctx context.Context, reader *bufio.Reader) (string, error) {
	lineCh := make(chan string, 1)
	errCh := make(chan error, 1)

	go func() {
		text, err := reader.ReadString('\n')
		if err != nil {
			errCh <- err
			return
		}
		lineCh <- text
	}()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case err := <-errCh:
		return "", err
	case line := <-lineCh:
		return line, nil
	}
}

// resolveServiceDestination decides whether to write a system-wide or user
// unit and incorporates the chosen port into the filename so multiple
// instances can coexist. We avoid mutexes by returning the full decision as
// values rather than mutating shared state.
func resolveServiceDestination(port int) (string, bool, error) {
	if runtime.GOOS != "linux" {
		return "", false, errors.New("systemd services are only supported on Linux")
	}
	suffix := fmt.Sprintf("chicha-isotope-map-%d.service", port)
	if os.Geteuid() == 0 {
		return filepath.Join("/etc/systemd/system", suffix), false, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false, fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".config", "systemd", "user", suffix), true, nil
}

// resolveLogPath selects a writable log destination and bakes the port into
// the filename so parallel services never clash. We stick to standard
// locations: /var/log for system units, XDG_STATE_HOME (or ~/.local/state) for
// user sessions.
func resolveLogPath(userUnit bool, port int) (string, error) {
	fileName := fmt.Sprintf("chicha-isotope-map-%d.log", port)
	if !userUnit {
		return filepath.Join("/var/log", fileName), nil
	}

	stateHome := os.Getenv("XDG_STATE_HOME")
	if strings.TrimSpace(stateHome) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir for log: %w", err)
		}
		stateHome = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(stateHome, fileName), nil
}

// buildExecArgs assembles the final ExecStart line. Returning a slice keeps the
// order predictable and avoids clever string concatenation. When a domain is
// present we skip an explicit port because HTTPS mode already binds 80/443 via
// the autocert server, keeping the unit file truthful to the CLI.
func buildExecArgs(binary string, port int, domain, dbType, dbPath, dbConn, support string, safecast bool, archiveDir string, importEnabled bool, importURL string) []string {
	args := []string{binary, "-db-type", dbType}
	if strings.TrimSpace(domain) != "" {
		args = append(args, "-domain", domain)
	} else {
		args = append(args, "-port", strconv.Itoa(port))
	}
	if dbType == "pgx" || dbType == "clickhouse" {
		if strings.TrimSpace(dbConn) != "" {
			args = append(args, "-db-conn", dbConn)
		}
	} else if strings.TrimSpace(dbPath) != "" {
		args = append(args, "-db-path", dbPath)
	}
	if strings.TrimSpace(support) != "" {
		args = append(args, "-support-email", support)
	}
	if safecast {
		args = append(args, "-safecast-realtime")
	}
	if strings.TrimSpace(archiveDir) != "" {
		args = append(args, "-json-archive-path", archiveDir)
	}
	if importEnabled && strings.TrimSpace(importURL) != "" {
		args = append(args, "-import-tgz-url", importURL)
	}
	return args
}

// writeServiceFile writes the unit file with a concise restart policy so
// failures recover automatically. The directories are created on demand to keep
// the happy path smooth for new operators.
func writeServiceFile(path, workdir, logPath string, args []string, userUnit bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir service dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return fmt.Errorf("mkdir log dir: %w", err)
	}
	// Touch the log file so systemd append targets exist even before the first
	// start. We still rely on journald, but the file keeps a stable place for
	// administrators who prefer tailing plain text.
	if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND, 0o644); err == nil {
		_ = f.Close()
	}

	wantedBy := "multi-user.target"
	if userUnit {
		wantedBy = "default.target"
	}

	content := fmt.Sprintf(`[Unit]
Description=Chicha Isotope Map (port %d)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=%s
ExecStart=%s
Restart=on-failure
RestartSec=5
StandardOutput=append:%s
StandardError=append:%s

[Install]
WantedBy=%s
`, extractPort(args), workdir, strings.Join(args, " "), logPath, logPath, wantedBy)

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write service file: %w", err)
	}
	return nil
}

// extractPort peeks at the ExecStart slice to keep the rendered Description in
// sync with the chosen port without storing extra global state.
func extractPort(args []string) int {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-port" {
			if p, err := strconv.Atoi(args[i+1]); err == nil {
				return p
			}
		}
	}
	return 0
}

// systemctlCommands builds the basic lifecycle commands so both automation and
// human instructions share the exact same strings.
func systemctlCommands(userUnit bool, unitName string) []string {
	prefix := "systemctl"
	if userUnit {
		prefix = "systemctl --user"
	}
	return []string{
		fmt.Sprintf("%s daemon-reload", prefix),
		fmt.Sprintf("%s enable --now %s", prefix, unitName),
	}
}

// commandResult streams the outcome of each systemctl call without blocking the
// caller. Returning both message and error keeps logging concise.
type commandResult struct {
	Message string
	Output  string
	Err     error
}

// runSystemctl executes the list of commands sequentially inside a goroutine
// and emits progress via a channel. select/case in the caller keeps cancellation
// simple while avoiding shared locks.
func runSystemctl(ctx context.Context, commands []string) <-chan commandResult {
	results := make(chan commandResult, len(commands))
	go func() {
		defer close(results)
		for _, cmdLine := range commands {
			parts := strings.Fields(cmdLine)
			if len(parts) == 0 {
				continue
			}
			cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
			output, err := cmd.CombinedOutput()
			results <- commandResult{Message: cmdLine, Output: string(output), Err: err}
		}
	}()
	return results
}

// printNextSteps writes a concise how-to block so operators immediately know
// how to manage the service and inspect logs. The goal is to keep post-setup
// actions discoverable without forcing people to hunt for the right systemctl
// incantations.
func printNextSteps(out io.Writer, theme colorTheme, res Result) {
	prefix := "systemctl"
	journal := "journalctl -u"
	edit := fmt.Sprintf("nano %s", res.ServicePath)
	if res.UserUnit {
		prefix = "systemctl --user"
		journal = "journalctl --user -u"
	} else {
		edit = fmt.Sprintf("sudo %s", edit)
	}
	fmt.Fprintf(out, "\n%sNext:%s\n", theme.AccentIfEnabled(), theme.ResetIfEnabled())
	fmt.Fprintf(out, "  reload:  %s daemon-reload\n", prefix)
	fmt.Fprintf(out, "  start:   %s start %s\n", prefix, res.ServiceName)
	fmt.Fprintf(out, "  restart: %s restart %s\n", prefix, res.ServiceName)
	fmt.Fprintf(out, "  stop:    %s stop %s\n", prefix, res.ServiceName)
	fmt.Fprintf(out, "  edit:    %s\n", edit)
	fmt.Fprintf(out, "           %s daemon-reload && %s restart %s\n", prefix, prefix, res.ServiceName)
	fmt.Fprintf(out, "  logs:    %s %s -f (or tail -f %s)\n", journal, res.ServiceName, res.LogPath)
	fmt.Fprintf(out, "  file:    %s\n", res.ServicePath)
}

// appendProfilePrimer appends a short service cheat sheet to ~/.profile so SSH sessions
// remind operators how to manage the unit. Keeping it terse ensures the login remains
// readable while still surfacing start/stop/log hints without extra commands.
func appendProfilePrimer(res Result) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	profilePath := filepath.Join(home, ".profile")

	prefix := "systemctl"
	journal := "journalctl -u"
	edit := fmt.Sprintf("nano %s", res.ServicePath)
	if res.UserUnit {
		prefix = "systemctl --user"
		journal = "journalctl --user -u"
	} else {
		edit = fmt.Sprintf("sudo %s", edit)
	}

	block := fmt.Sprintf("\n# chicha-isotope-map service hint\nif [ -t 1 ]; then\n  echo \"Chicha service: %s\"\n  echo \"reload:  %s daemon-reload\"\n  echo \"restart: %s restart %s\"\n  echo \"stop:    %s stop %s\"\n  echo \"edit:    %s\"\n  echo \"logs:    %s %s -f (or tail -f %s)\"\nfi\n", res.ServiceName, prefix, prefix, res.ServiceName, prefix, res.ServiceName, edit, journal, res.ServiceName, res.LogPath)

	existing, err := os.ReadFile(profilePath)
	if err == nil && strings.Contains(string(existing), "# chicha-isotope-map service hint") {
		return
	}

	f, err := os.OpenFile(profilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(block)
}

// printUsageHint keeps the end-of-setup message short while still telling the
// operator how to reach the service. Mentioning autocert ports when a domain is
// present avoids confusion about why 80/443 must stay open even though we no
// longer ask for an explicit port flag.
func printUsageHint(out io.Writer, theme colorTheme, domain string, port int) {
	target := fmt.Sprintf("http://localhost:%d", port)
	note := "Keep the service running under systemd and reload+restart after edits."
	if trimmed := strings.TrimSpace(domain); trimmed != "" {
		target = fmt.Sprintf("https://%s", trimmed)
		note = "TLS uses ports 80/443 automatically; leave them open for Let's Encrypt."
	}
	fmt.Fprintf(out, "\n%sUse:%s open %s in your browser. %s\n", theme.AccentIfEnabled(), theme.ResetIfEnabled(), target, note)
}

// createUpdateScript writes a tiny helper under /usr/local/bin so operators can stop,
// refresh, and restart the service in one go. Keeping the logic here guarantees the
// script matches the freshly written unit, which avoids mistakes when juggling
// multiple ports. The function intentionally skips mutexes and relies on simple file
// writes to stay in line with Go's preference for straightforward code.
func createUpdateScript(res Result) error {
	if len(res.ExecStart) == 0 {
		return errors.New("missing ExecStart for update script")
	}

	binaryPath := res.ExecStart[0]
	systemctl := "systemctl"
	if res.UserUnit {
		systemctl = "systemctl --user"
	}

	script := fmt.Sprintf("#!/bin/bash\nset -euo pipefail\nSVC=%q\nLOG=%q\nBIN=%q\nCTL=%q\n$CTL stop $SVC\ncurl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_linux_amd64 > $BIN\nchmod +x $BIN\n$BIN --version\n$CTL start $SVC\ntail -f $LOG\n", res.ServiceName, res.LogPath, binaryPath, systemctl)

	if err := os.WriteFile("/usr/local/bin/chicha-update", []byte(script), 0o755); err != nil {
		return fmt.Errorf("write updater: %w", err)
	}
	return nil
}

// tailLogs follows the application log right after startup so operators can see the first lines without another command.
// Using tail keeps the terminal familiar while ctx lets the caller interrupt cleanly.
func tailLogs(ctx context.Context, out io.Writer, theme colorTheme, logPath string) {
	fmt.Fprintf(out, "\n%sLive log (Ctrl+C to exit)%s\n", theme.PromptIfEnabled(), theme.ResetIfEnabled())
	cmd := exec.CommandContext(ctx, "tail", "-n", "40", "-F", logPath)
	cmd.Stdout = out
	cmd.Stderr = out
	_ = cmd.Run()
}

// AccentIfEnabled wraps the text in the accent colour when ANSI is available.
// Keeping the helpers on the theme struct avoids repeating conditionals at each
// print site.
func (c colorTheme) AccentIfEnabled() string {
	if c.Enabled {
		return c.Accent
	}
	return ""
}

// PromptIfEnabled mirrors AccentIfEnabled for prompt highlights.
func (c colorTheme) PromptIfEnabled() string {
	if c.Enabled {
		return c.Prompt
	}
	return ""
}

// SuccessIfEnabled highlights confirmations without forcing colour-only output.
func (c colorTheme) SuccessIfEnabled() string {
	if c.Enabled {
		return c.Success
	}
	return ""
}

// ResetIfEnabled returns the reset sequence only when colours were used.
func (c colorTheme) ResetIfEnabled() string {
	if c.Enabled {
		return c.Reset
	}
	return ""
}
