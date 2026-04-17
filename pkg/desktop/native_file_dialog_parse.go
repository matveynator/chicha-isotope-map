package desktop

import "strings"

// parseAppleScriptSelectedPaths normalizes osascript output into clean POSIX file paths.
// AppleScript can return either newline-separated values or quoted list fragments.
func parseAppleScriptSelectedPaths(output string) []string {
	rawLines := strings.Split(strings.TrimSpace(output), "\n")
	selectedPaths := make([]string, 0, len(rawLines))

	for _, rawLine := range rawLines {
		trimmedLine := strings.TrimSpace(rawLine)
		if trimmedLine == "" {
			continue
		}

		cleanedLine := strings.TrimSuffix(trimmedLine, ",")
		cleanedLine = strings.Trim(cleanedLine, "\"'")
		cleanedLine = strings.TrimSpace(cleanedLine)
		if cleanedLine == "" {
			continue
		}

		selectedPaths = append(selectedPaths, cleanedLine)
	}

	return selectedPaths
}
