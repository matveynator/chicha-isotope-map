//go:build desktop

package desktop

// DefaultEnabled keeps desktop-tag binaries double-click friendly by enabling
// desktop mode unless operators explicitly disable it with -desktop=false.
func DefaultEnabled() bool {
	return true
}
