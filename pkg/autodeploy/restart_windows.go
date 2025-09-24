//go:build windows

package autodeploy

import (
	"os/exec"
)

// restartBinary launches a fresh process on Windows. We cannot swap the image in
// place because the filesystem keeps the original binary locked, so we spawn a
// new copy and exit the current one once the child starts successfully.
func restartBinary(binary string, args []string, env []string) error {
	trimmed := make([]string, 0, len(args))
	if len(args) > 0 {
		trimmed = append(trimmed, args[1:]...)
	}
	cmd := exec.Command(binary, trimmed...)
	cmd.Env = env
	return cmd.Start()
}
