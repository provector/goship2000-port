//go:build !windows

package audioopl

import "os/exec"

func configureHelperCommand(command *exec.Cmd) {}
