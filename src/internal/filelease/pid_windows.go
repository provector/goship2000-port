package filelease

import (
	"errors"
	"golang.org/x/sys/windows"
)

func processGone(pid int) bool {
	if int(uint32(pid)) != pid {
		return false
	}
	h, e := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if e != nil {
		return errors.Is(e, windows.ERROR_INVALID_PARAMETER)
	}
	defer windows.CloseHandle(h)
	status, e := windows.WaitForSingleObject(h, 0)
	return e == nil && status == windows.WAIT_OBJECT_0
}
