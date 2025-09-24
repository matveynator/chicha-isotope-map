//go:build !windows

package autodeploy

import "syscall"

// restartBinary replaces the current process image with the new binary on Unix.
// We call Exec directly to avoid leaving orphaned helpers and to respect Go's
// preference for simple, explicit process management.
func restartBinary(binary string, args []string, env []string) error {
	if len(args) == 0 {
		args = []string{binary}
	} else {
		args[0] = binary
	}
	return syscall.Exec(binary, args, env)
}
