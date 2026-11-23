//go:build windows

package main

import (
	"golang.org/x/sys/windows"
	"syscall"
	"unsafe"
)

func sizeOwnedTerminal(_ bool) {
	dll := windows.NewLazySystemDLL("kernel32.dll")
	list := dll.NewProc("GetConsoleProcessList")
	pids := make([]uint32, 8)
	n, _, _ := list.Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)))
	if n != 1 {
		return
	}
	h, e := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if e != nil {
		return
	}
	// Only a standalone console reaches here. Ask for its normal placement
	// before applying the compact dimensions, without activating another window.
	hwnd, _, _ := dll.NewProc("GetConsoleWindow").Call()
	if hwnd != 0 {
		user := windows.NewLazySystemDLL("user32.dll")
		// Standalone console only. A shared Windows Terminal tab owns its icon.
		module, _, _ := dll.NewProc("GetModuleHandleW").Call(0)
		for kind, metric := range []uintptr{49, 11} { // SM_CXSMICON, SM_CXICON
			size, _, _ := user.NewProc("GetSystemMetrics").Call(metric)
			icon, _, _ := user.NewProc("LoadImageW").Call(module, 1, 1, size, size, 0x8000) // IMAGE_ICON, LR_SHARED
			if icon != 0 {
				user.NewProc("SendMessageW").Call(hwnd, 0x0080, uintptr(kind), icon) // WM_SETICON
			}
		}
		if zoomed, _, _ := user.NewProc("IsZoomed").Call(hwnd); zoomed != 0 {
			user.NewProc("ShowWindow").Call(hwnd, 4) // SW_SHOWNOACTIVATE
		}
	}
	rect := [4]int16{0, 0, 75, 23}
	dll.NewProc("SetConsoleWindowInfo").Call(uintptr(h), 1, uintptr(unsafe.Pointer(&rect[0])))
	dll.NewProc("SetConsoleScreenBufferSize").Call(uintptr(h), uintptr(uint32(24)<<16|76))
}
