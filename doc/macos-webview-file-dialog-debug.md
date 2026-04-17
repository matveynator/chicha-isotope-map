# macOS WebView file dialog dependency chain

This project uses `go-webview-selector` in app code and switches the implementation in `go.mod`.

## Why this file exists

On macOS, `<input type="file">` depends on the native `WKUIDelegate` open panel callback.
If your patch lives in a fork, the Go wrapper must point to that fork:

1. `github.com/webview/webview_go` (Go wrapper)

## Current module wiring

`go.mod` currently contains the implementation override:

- `github.com/webview/webview_go => github.com/matveynator/webview_go`

For desktop macOS builds we also provide a native upload fallback endpoint
(`/desktop/upload-native`) that opens the OS file picker via AppleScript when
`<input type="file">` is blocked by embedded WebView behavior.

## Verification checklist

Run these commands from project root:

```bash
GOPRIVATE=github.com/matveynator go mod download
GOPRIVATE=github.com/matveynator go list -m all | rg 'webview(_go)?|matveynator'
CGO_ENABLED=1 GOPRIVATE=github.com/matveynator go build -tags desktop ./...
```

Do not build desktop mode with `CGO_ENABLED=0`. In that case native webview code is
excluded by Go build tags and desktop mode cannot work.

Expected result:

- `webview_go` resolves to `github.com/matveynator/...`
- desktop build succeeds
- file picker opens on macOS via native fallback upload button
