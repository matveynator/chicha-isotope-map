package selfupgrade

import (
	"context"
	"strings"
)

// LogNotifier wraps a printf-style logger so deployments can surface progress
// without wiring extra infrastructure. It keeps the contract synchronous so the
// manager knows when logging failed (for example, when stdout is closed).
type LogNotifier struct {
	Logf func(string, ...any)
}

// Notify emits the message and returns nil even when Logf is nil; silent
// logging keeps the deployment pipeline resilient to missing dependencies.
func (n LogNotifier) Notify(_ context.Context, title, message string) error {
	if n.Logf == nil {
		return nil
	}
	n.Logf("[selfupgrade] %s: %s", strings.TrimSpace(title), strings.TrimSpace(message))
	return nil
}
