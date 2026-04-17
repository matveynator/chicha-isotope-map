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
		"-e", "set selectedPaths to {}",
		"-e", "repeat with selectedFile in selectedFiles",
		"-e", "set end of selectedPaths to POSIX path of selectedFile",
		"-e", "end repeat",
		"-e", "set AppleScript's text item delimiters to linefeed",
		"-e", "selectedPaths as text",
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

	selectedPaths := parseAppleScriptSelectedPaths(string(output))
	if len(selectedPaths) == 0 {
		return nil, errors.New("desktop native file dialog: empty selection")
	}

	return selectedPaths, nil
}
