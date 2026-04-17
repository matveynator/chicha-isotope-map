//go:build darwin && desktop && cgo

package desktop

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// PickFiles opens the native macOS choose-file panel and returns absolute POSIX paths.
// We call osascript because it is stable on macOS and keeps the desktop fallback self-contained.
func PickFiles() ([]string, error) {
	command := exec.Command(
		"osascript",
		"-s", "s",
		"-e", "set selectedFiles to choose file with multiple selections allowed",
		"-e", "set AppleScript's text item delimiters to \"\\n\"",
		"-e", "POSIX path of selectedFiles as text",
	)

	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		lowerMessage := strings.ToLower(message)
		if strings.Contains(lowerMessage, "user canceled") || strings.Contains(lowerMessage, "user cancelled") {
			return nil, ErrNativeDialogCanceled
		}
		return nil, fmt.Errorf("desktop native file dialog: %w (%s)", err, message)
	}

	rawPaths := strings.Split(strings.TrimSpace(string(output)), "\n")
	selectedPaths := make([]string, 0, len(rawPaths))
	for _, rawPath := range rawPaths {
		trimmedPath := strings.TrimSpace(rawPath)
		if trimmedPath == "" {
			continue
		}
		selectedPaths = append(selectedPaths, trimmedPath)
	}
	if len(selectedPaths) == 0 {
		return nil, errors.New("desktop native file dialog: empty selection")
	}

	return selectedPaths, nil
}
