package main

import (
	"golang.org/x/sys/windows"
	"os"
)

// A GUI-subsystem SETUP has no console of its own. When invoked as a CLI,
// reuse the caller's console if available. Preserve redirected pipe handles.
func attachCLI() {
	attach := windows.NewLazySystemDLL("kernel32.dll").NewProc("AttachConsole")
	attach.Call(0xffffffff) // ATTACH_PARENT_PROCESS
	for _, out := range []struct {
		id  uint32
		dst **os.File
	}{{windows.STD_OUTPUT_HANDLE, &os.Stdout}, {windows.STD_ERROR_HANDLE, &os.Stderr}} {
		old := windows.Handle((*out.dst).Fd())
		if _, err := windows.GetFileType(old); err == nil {
			continue
		}
		if h, e := windows.GetStdHandle(out.id); e == nil && h != 0 && h != windows.InvalidHandle {
			*out.dst = os.NewFile(uintptr(h), "console")
		}
	}
}
