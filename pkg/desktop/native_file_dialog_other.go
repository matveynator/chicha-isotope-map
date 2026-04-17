//go:build !darwin || !desktop || !cgo

package desktop

import "fmt"

// PickFiles is unsupported outside desktop macOS CGO builds.
func PickFiles() ([]string, error) {
	return nil, fmt.Errorf("desktop native file dialog is supported only on macOS desktop builds with CGO")
}
