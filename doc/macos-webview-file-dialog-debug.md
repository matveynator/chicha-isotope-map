# macOS WebView file dialog dependency chain

This project uses `go-webview-selector` in app code and switches the implementation in `go.mod`.

## Why this file exists

On macOS, `<input type="file">` depends on the native `WKUIDelegate` open panel callback.
If your patch lives in a fork, both layers must point to that fork:

1. `github.com/webview/webview_go` (Go wrapper)
2. `github.com/webview/webview` (native webview core)

If only layer 1 is replaced, layer 2 can still come from upstream and file dialogs stay broken.

## Current module wiring

`go.mod` now contains two replace directives:

- `github.com/webview/webview_go => github.com/matveynator/webview_go`
- `github.com/webview/webview => github.com/matveynator/webview`

## Verification checklist

Run these commands from project root:

```bash
GOPRIVATE=github.com/matveynator go mod download
GOPRIVATE=github.com/matveynator go list -m all | rg 'webview(_go)?|matveynator'
GOPRIVATE=github.com/matveynator go build -tags desktop ./...
```

Expected result:

- both `webview_go` and `webview` resolve to `github.com/matveynator/...`
- desktop build succeeds
- file picker opens on macOS when clicking the upload button
