package autodeploy

import "time"

// Config keeps knobs for the background checker. We keep the surface minimal so
// applications only pass the manifest URL while the rest follows sensible
// defaults.
type Config struct {
	ManifestURL  string
	LocalBinary  string
	Workspace    string
	PollInterval time.Duration
	Logf         func(string, ...any)
}

// Event is streamed back to the caller so operators understand which step the
// background goroutine is executing at any moment.
type Event struct {
	Stage    string
	Message  string
	Path     string
	Checksum string
	Err      error
}

// manifest describes the remote payload. We expect a minimal JSON document that
// contains a checksum and the URL of the binary to download. Leaving the struct
// private gives us room to evolve the protocol later without exposing
// unnecessary fields.
type manifest struct {
	Checksum string `json:"checksum"`
	URL      string `json:"url"`
	Version  string `json:"version"`
}
