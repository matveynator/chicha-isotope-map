//go:build desktop && cgo

package desktop

import (
	"fmt"
	"log"

	webview "github.com/jchv/go-webview-selector"
)

// RunWebviewWindow opens a native desktop window bound to the running local HTTP server.
// The function is isolated behind the desktop build tag so server-only builds stay CGO-free.
func RunWebviewWindow(address, appVersion string) error {
	window := webview.New(false)
	if window == nil {
		return fmt.Errorf("webview: failed to create window")
	}
	defer window.Destroy()

	windowTitle := fmt.Sprintf("chicha-isotope-map: world radiation map (%s)", appVersion)
	window.SetTitle(windowTitle)
	window.SetSize(1120, 760, webview.HintNone)
	window.Navigate("http://" + address)

	log.Printf("webview: window started")
	window.Run()
	log.Printf("shutdown: webview stopped")

	return nil
}
