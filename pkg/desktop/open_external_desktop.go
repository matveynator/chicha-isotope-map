//go:build desktop

package desktop

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
)

// OpenExternalURL launches a URL in the system browser for desktop builds.
// Keeping this in pkg/desktop avoids leaking OS-specific process logic.
func OpenExternalURL(rawURL string) error {
	trimmedURL := strings.TrimSpace(rawURL)
	if trimmedURL == "" {
		return fmt.Errorf("desktop external open: empty url")
	}
	parsedURL, err := url.Parse(trimmedURL)
	if err != nil {
		return fmt.Errorf("desktop external open: parse url: %w", err)
	}
	scheme := strings.ToLower(strings.TrimSpace(parsedURL.Scheme))
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("desktop external open: unsupported scheme %q", scheme)
	}

	var commandName string
	var commandArgs []string
	switch runtime.GOOS {
	case "darwin":
		commandName = "open"
		commandArgs = []string{trimmedURL}
	case "windows":
		commandName = "rundll32"
		commandArgs = []string{"url.dll,FileProtocolHandler", trimmedURL}
	default:
		commandName = "xdg-open"
		commandArgs = []string{trimmedURL}
	}

	if err := exec.Command(commandName, commandArgs...).Start(); err != nil {
		return fmt.Errorf("desktop external open: %s: %w", commandName, err)
	}
	return nil
}

