//go:build !desktop

package desktop

import "fmt"

// RunWebviewWindow returns a clear error in non-desktop builds so release binaries
// can keep CGO disabled while exposing an explicit runtime message.
func RunWebviewWindow(address, appVersion string) error {
	_ = address
	_ = appVersion
	return fmt.Errorf("desktop mode is unavailable in this binary; rebuild with -tags desktop")
}
