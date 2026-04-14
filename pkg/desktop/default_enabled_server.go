//go:build !desktop

package desktop

// DefaultEnabled keeps server builds behavior unchanged, requiring an explicit
// -desktop flag only for binaries compiled with desktop support.
func DefaultEnabled() bool {
	return false
}
