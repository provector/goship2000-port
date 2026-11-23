package audioopl

import (
	"os/exec"
	"syscall"
)

func configureHelperCommand(command *exec.Cmd) {
	// Preparing from the graphical app must not flash a separate console window.
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
}
