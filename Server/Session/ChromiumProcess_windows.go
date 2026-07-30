//go:build windows

package Session

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func configureDetachedChromiumProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS,
		HideWindow:    true,
	}
}
