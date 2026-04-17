//go:build desktop && !cgo

package desktop

// DefaultEnabled stays false when desktop tag is set without CGO.
// This avoids auto-starting a webview mode that cannot work on this build.
func DefaultEnabled() bool {
	return false
}
