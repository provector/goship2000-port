//go:build !headless

package menus

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"golang.org/x/sys/windows"
	"image"
	"unsafe"
)

var windowUser32 = windows.NewLazySystemDLL("user32.dll")
var monitorForRect = windowUser32.NewProc("MonitorFromRect")
var monitorInfo = windowUser32.NewProc("GetMonitorInfoW")
var windowRect = windowUser32.NewProc("GetWindowRect")
var clientRect = windowUser32.NewProc("GetClientRect")
var toScreen = windowUser32.NewProc("ClientToScreen")

func (*desktopFullscreen) workArea(monitor image.Rectangle) (area image.Rectangle, left, top, right, bottom int) {
	area = monitor
	type rect struct{ l, t, r, b int32 }
	r := rect{int32(monitor.Min.X), int32(monitor.Min.Y), int32(monitor.Max.X), int32(monitor.Max.Y)}
	m, _, _ := monitorForRect.Call(uintptr(unsafe.Pointer(&r)), 2)
	var info struct {
		Size         uint32
		Bounds, Work rect
		Flags        uint32
	}
	info.Size = uint32(unsafe.Sizeof(info))
	if ok, _, _ := monitorInfo.Call(m, uintptr(unsafe.Pointer(&info))); ok != 0 {
		area = image.Rect(int(info.Work.l), int(info.Work.t), int(info.Work.r), int(info.Work.b))
	}
	h := uintptr(rl.GetWindowHandle())
	var outer, client rect
	var origin struct{ x, y int32 }
	a, _, _ := windowRect.Call(h, uintptr(unsafe.Pointer(&outer)))
	b, _, _ := clientRect.Call(h, uintptr(unsafe.Pointer(&client)))
	c, _, _ := toScreen.Call(h, uintptr(unsafe.Pointer(&origin)))
	if a*b*c != 0 {
		left, top = int(origin.x-outer.l), int(origin.y-outer.t)
		right, bottom = int(outer.r-origin.x-client.r), int(outer.b-origin.y-client.b)
	}
	return
}
