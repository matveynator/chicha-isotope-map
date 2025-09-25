package selfupgrade

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
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

// ErrProcessReplaced informs the pipeline that a self-restarting runner already
// scheduled a process handover. Once this error is observed the caller must not
// attempt further checks because the current process will terminate shortly.
var ErrProcessReplaced = errors.New("selfupgrade: process handover scheduled")

// SelfReplacingRunner launches a fresh copy of the binary and gracefully exits
// the current process after a short delay. It relies on environment variables
// to coordinate a controlled handoff inside main(), sticking to simple
// primitives instead of mutexes or RPCs as per the Go proverbs.
type SelfReplacingRunner struct {
	DryRunner

	BinaryPath      string
	Args            []string
	ExtraEnv        []string
	ExitDelay       time.Duration
	ChildStartDelay time.Duration
	Logf            func(string, ...any)
}

// NewSelfReplacingRunner wires a runner that can restart the current binary in
// place. We capture os.Args eagerly so later mutations by flag parsing do not
// surprise the spawned process.
func NewSelfReplacingRunner(binaryPath string, logf func(string, ...any)) *SelfReplacingRunner {
	args := make([]string, len(os.Args))
	copy(args, os.Args)
	return &SelfReplacingRunner{
		DryRunner:       DryRunner{Logf: logf},
		BinaryPath:      binaryPath,
		Args:            args,
		ExitDelay:       4 * time.Second,
		ChildStartDelay: 6 * time.Second,
		Logf:            logf,
	}
}

func (r *SelfReplacingRunner) sanitizeEnv() []string {
	base := os.Environ()
	filtered := make([]string, 0, len(base)+len(r.ExtraEnv)+4)
	for _, kv := range base {
		if strings.HasPrefix(kv, "SELFUPGRADE_") {
			continue
		}
		filtered = append(filtered, kv)
	}
	filtered = append(filtered, r.ExtraEnv...)
	return filtered
}

func (r *SelfReplacingRunner) lastGoodPath() string {
	candidate := strings.TrimSpace(r.BinaryPath)
	if candidate == "" {
		return ""
	}
	path := candidate + ".previous"
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}

func (r *SelfReplacingRunner) launch(cmdPath string, args []string, env []string) error {
	cmd := exec.Command(cmdPath, args...)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Start()
}

// RestartService spawns the freshly downloaded binary and schedules the
// current process to exit. The new process pauses for a configurable delay so
// we can finish bookkeeping before the socket handoff happens.
func (r *SelfReplacingRunner) RestartService(ctx context.Context) error {
	if strings.TrimSpace(r.BinaryPath) == "" {
		return errors.New("selfupgrade: binary path not configured")
	}
	args := r.Args
	if len(args) == 0 {
		args = []string{r.BinaryPath}
	}
	if runtime.GOOS == "windows" {
		// Windows expects the first argument to be the command itself.
		if len(args) == 0 || !strings.EqualFold(args[0], r.BinaryPath) {
			args = append([]string{r.BinaryPath}, args...)
		}
	}

	waitDelay := r.ChildStartDelay
	if waitDelay <= 0 {
		waitDelay = 6 * time.Second
	}
	exitDelay := r.ExitDelay
	if exitDelay <= 0 {
		exitDelay = 4 * time.Second
	}

	env := r.sanitizeEnv()
	env = append(env,
		fmt.Sprintf("SELFUPGRADE_WAIT_SECONDS=%.1f", waitDelay.Seconds()),
		fmt.Sprintf("SELFUPGRADE_ORIGIN_PID=%d", os.Getpid()),
	)
	if lastGood := r.lastGoodPath(); lastGood != "" {
		env = append(env, fmt.Sprintf("SELFUPGRADE_LAST_GOOD=%s", lastGood))
	}

	cmd := exec.CommandContext(context.Background(), r.BinaryPath, args[1:]...)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Start(); err != nil {
		return err
	}
	proc := cmd.Process

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = proc.Kill()
		case <-time.After(exitDelay):
			if r.Logf != nil {
				r.Logf("[selfupgrade] exiting old process for handoff to PID %d", proc.Pid)
			}
			close(done)
			os.Exit(0)
		}
	}()

	go func() {
		state, err := proc.Wait()
		if err != nil {
			if r.Logf != nil {
				r.Logf("[selfupgrade] new process wait failed: %v", err)
			}
			return
		}
		select {
		case <-done:
			// Parent already exiting; nothing to do.
			return
		default:
		}
		if state.Success() {
			if r.Logf != nil {
				r.Logf("[selfupgrade] new process exited before takeover: success=%v", state.Success())
			}
			return
		}
		if r.Logf != nil {
			r.Logf("[selfupgrade] new process exited with code %d before takeover", state.ExitCode())
		}
		r.rollback()
	}()

	return ErrProcessReplaced
}

func (r *SelfReplacingRunner) rollback() {
	lastGood := r.lastGoodPath()
	if lastGood == "" {
		if r.Logf != nil {
			r.Logf("[selfupgrade] rollback skipped: no last-good binary found")
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := copyFileWithHash(ctx, lastGood, r.BinaryPath); err != nil {
		if r.Logf != nil {
			r.Logf("[selfupgrade] rollback copy failed: %v", err)
		}
		return
	}
	env := r.sanitizeEnv()
	if err := r.launch(r.BinaryPath, r.Args[1:], env); err != nil {
		if r.Logf != nil {
			r.Logf("[selfupgrade] rollback relaunch failed: %v", err)
		}
		return
	}
	if r.Logf != nil {
		r.Logf("[selfupgrade] rollback started from %s", lastGood)
	}
}

// PostPromotionCheck is intentionally a no-op for self replacement because the
// old process relinquishes control after scheduling the handoff.
func (r *SelfReplacingRunner) PostPromotionCheck(context.Context) error {
	return nil
}
