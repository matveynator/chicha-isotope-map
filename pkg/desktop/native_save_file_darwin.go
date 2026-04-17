//go:build darwin && desktop && cgo

package desktop

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// SaveFile asks macOS for a destination path and writes bytes to disk.
// This bypasses embedded WebView download limitations in desktop mode.
func SaveFile(defaultName string, contents []byte) (string, error) {
	scriptLine := fmt.Sprintf("POSIX path of (choose file name with prompt %q default name %q)", "Save track JSON", defaultName)
	command := exec.Command("osascript", "-s", "s", "-e", scriptLine)

	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		lowerMessage := strings.ToLower(message)
		if strings.Contains(lowerMessage, "user canceled") || strings.Contains(lowerMessage, "user cancelled") {
			return "", ErrNativeDialogCanceled
		}
		return "", fmt.Errorf("desktop native save dialog: %w (%s)", err, message)
	}

	paths := parseAppleScriptSelectedPaths(string(output))
	if len(paths) == 0 {
		return "", fmt.Errorf("desktop native save dialog: empty destination")
	}
	destinationPath := paths[0]

	if err := os.WriteFile(destinationPath, contents, 0o644); err != nil {
		return "", fmt.Errorf("desktop native save dialog: write file: %w", err)
	}
	return destinationPath, nil
}
