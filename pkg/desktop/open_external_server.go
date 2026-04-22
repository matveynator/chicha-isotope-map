//go:build !desktop

package desktop

import "fmt"

// OpenExternalURL is unavailable in server-only builds.
func OpenExternalURL(rawURL string) error {
	_ = rawURL
	return fmt.Errorf("desktop external open unavailable in non-desktop build")
}

