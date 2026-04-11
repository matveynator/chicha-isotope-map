//go:build desktop && darwin

package desktop

import (
	"fmt"
	"os/exec"
)

// RunWebviewWindow opens the app URL in the system browser on macOS.
// We intentionally avoid embedded webview here because file input dialogs are
// unreliable in upstream webview implementations on macOS.
func RunWebviewWindow(address string) error {
	applicationURL := "http://" + address
	openCommand := exec.Command("open", applicationURL)
	if err := openCommand.Start(); err != nil {
		return fmt.Errorf("desktop: open browser: %w", err)
	}
	return ErrExternalBrowserLaunched
}
