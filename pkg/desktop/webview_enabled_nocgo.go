//go:build desktop && !cgo

package desktop

import "fmt"

// RunWebviewWindow returns a clear error when desktop builds are compiled
// without CGO. Native webview backends require CGO on all supported desktop OSes.
func RunWebviewWindow(address string) error {
	_ = address
	return fmt.Errorf("desktop mode requires CGO_ENABLED=1; rebuild with CGO and -tags desktop or run with -desktop=false")
}
