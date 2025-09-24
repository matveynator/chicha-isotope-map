package selfupgrade

import (
	"context"
	"fmt"
	"time"
)

// DryRunner performs no side effects and merely pretends that actions succeed.
// It lets developers wire the manager without risking accidental restarts while
// keeping the call graph explicit.
type DryRunner struct {
	Logf func(string, ...any)
}

// StartCanary pretends to boot a new process by closing the Ready channel after
// a short delay. We intentionally keep the goroutine small so the event loop
// remains responsive even if nobody ever consumes the readiness signal.
func (r DryRunner) StartCanary(ctx context.Context, build CandidateBuild, clone CloneInfo, port int) (CanaryInstance, error) {
	ready := make(chan struct{})
	errCh := make(chan error, 1)

	go func() {
		defer close(errCh)
		select {
		case <-ctx.Done():
			errCh <- ctx.Err()
		case <-time.After(2 * time.Second):
			if r.Logf != nil {
				r.Logf("[selfupgrade] dry-run canary ready: version=%s port=%d clone=%s", build.Version, port, clone.Name)
			}
			close(ready)
		}
	}()

	return CanaryInstance{
		ID:    fmt.Sprintf("dry-%d", time.Now().UnixNano()),
		Ready: ready,
		Err:   errCh,
	}, nil
}

// StopCanary logs and returns immediately because there is nothing to shut
// down in the dry-run scenario.
func (r DryRunner) StopCanary(_ context.Context, instance CanaryInstance) error {
	if r.Logf != nil {
		r.Logf("[selfupgrade] dry-run stop canary: %s", instance.ID)
	}
	return nil
}

// RestartService is a stub that simply records the intent to restart.
func (r DryRunner) RestartService(_ context.Context) error {
	if r.Logf != nil {
		r.Logf("[selfupgrade] dry-run restart service requested")
	}
	return nil
}

// PostPromotionCheck instantly approves the deployment in dry mode.
func (r DryRunner) PostPromotionCheck(_ context.Context) error {
	if r.Logf != nil {
		r.Logf("[selfupgrade] dry-run post-promotion check succeeded")
	}
	return nil
}
