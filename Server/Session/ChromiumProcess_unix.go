//go:build darwin || linux

package Session

import (
	"os/exec"
	"syscall"
)

func configureDetachedChromiumProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
