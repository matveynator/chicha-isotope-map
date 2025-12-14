package setupwizard

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
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
	DBType       string
	DBPath       string
	DBConn       string
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

	fmt.Fprintf(out, "\n%s🛠  Interactive setup for chicha-isotope-map%s\n", theme.AccentIfEnabled(), theme.ResetIfEnabled())
	fmt.Fprintf(out, "%sAnswer with Enter to accept defaults. Values are written into a systemd service so the map restarts automatically.%s\n\n", theme.AccentIfEnabled(), theme.ResetIfEnabled())

	portStr := promptWithDefault(ctx, reader, out, theme, "HTTP port", strconv.Itoa(defaults.Port))
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 {
		return Result{}, fmt.Errorf("invalid port: %s", portStr)
	}

	domain := promptWithDefault(ctx, reader, out, theme, "Domain (leave empty for HTTP only)", defaults.Domain)
	dbType := promptChoice(ctx, reader, out, theme, "Database engine", []string{"sqlite", "duckdb", "chai", "pgx", "clickhouse"}, defaults.DBType)

	dbPath := defaults.DBPath
	dbConn := defaults.DBConn
	if dbType == "pgx" || dbType == "clickhouse" {
		dbConn = promptWithDefault(ctx, reader, out, theme, "Connection URI", defaults.DBConn)
	} else {
		dbPath = promptWithDefault(ctx, reader, out, theme, "Database file path", defaults.DBPath)
	}

	support := promptWithDefault(ctx, reader, out, theme, "Support e-mail shown in legal notice", defaults.SupportEmail)

	unitPath, userUnit, err := resolveServiceDestination()
	if err != nil {
		return Result{}, err
	}

	execPath := defaults.BinaryPath
	if execPath == "" {
		execPath, err = os.Executable()
		if err != nil {
			return Result{}, fmt.Errorf("resolve binary path: %w", err)
		}
	}
	execPath, _ = filepath.Abs(execPath)

	args := buildExecArgs(execPath, port, domain, dbType, dbPath, dbConn, support)
	if defaults.WorkingDir == "" {
		defaults.WorkingDir = filepath.Dir(execPath)
	}

	if err := writeServiceFile(unitPath, defaults.WorkingDir, args, userUnit); err != nil {
		return Result{}, err
	}

	result := Result{
		ServiceName: filepath.Base(unitPath),
		ServicePath: unitPath,
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

	return result, nil
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
// unit. We avoid mutexes by returning the full decision as values rather than
// mutating shared state.
func resolveServiceDestination() (string, bool, error) {
	if runtime.GOOS != "linux" {
		return "", false, errors.New("systemd services are only supported on Linux")
	}
	if os.Geteuid() == 0 {
		return "/etc/systemd/system/chicha-isotope-map.service", false, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false, fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".config", "systemd", "user", "chicha-isotope-map.service"), true, nil
}

// buildExecArgs assembles the final ExecStart line. Returning a slice keeps the
// order predictable and avoids clever string concatenation.
func buildExecArgs(binary string, port int, domain, dbType, dbPath, dbConn, support string) []string {
	args := []string{binary, "-port", strconv.Itoa(port), "-db-type", dbType}
	if strings.TrimSpace(domain) != "" {
		args = append(args, "-domain", domain)
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
	return args
}

// writeServiceFile writes the unit file with a concise restart policy so
// failures recover automatically. The directories are created on demand to keep
// the happy path smooth for new operators.
func writeServiceFile(path, workdir string, args []string, userUnit bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir service dir: %w", err)
	}

	wantedBy := "multi-user.target"
	if userUnit {
		wantedBy = "default.target"
	}

	content := fmt.Sprintf(`[Unit]
Description=Chicha Isotope Map
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=%s
ExecStart=%s
Restart=on-failure
RestartSec=5

[Install]
WantedBy=%s
`, workdir, strings.Join(args, " "), wantedBy)

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write service file: %w", err)
	}
	return nil
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
		fmt.Sprintf("%s enable %s", prefix, unitName),
		fmt.Sprintf("%s start %s", prefix, unitName),
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
// how to manage the service and inspect logs.
func printNextSteps(out io.Writer, theme colorTheme, res Result) {
	prefix := "systemctl"
	journal := "journalctl -u"
	if res.UserUnit {
		prefix = "systemctl --user"
		journal = "journalctl --user -u"
	}
	fmt.Fprintf(out, "\n%sNext steps:%s\n", theme.AccentIfEnabled(), theme.ResetIfEnabled())
	fmt.Fprintf(out, "  • Start:    %s start %s\n", prefix, res.ServiceName)
	fmt.Fprintf(out, "  • Restart:  %s restart %s\n", prefix, res.ServiceName)
	fmt.Fprintf(out, "  • Stop:     %s stop %s\n", prefix, res.ServiceName)
	fmt.Fprintf(out, "  • Status:   %s status %s\n", prefix, res.ServiceName)
	fmt.Fprintf(out, "  • Logs:     %s %s -f\n", journal, res.ServiceName)
	fmt.Fprintf(out, "  • Binary:   %s\n", res.ExecStart[0])
	fmt.Fprintf(out, "  • Service:  %s\n", res.ServicePath)
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
