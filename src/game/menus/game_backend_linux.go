//go:build cgo && !headless && !drm && !rgfw && !sdl && !sdl3 && !android

package menus

/*
extern int glfwGetPlatform(void);
extern int glfwPlatformSupported(int platform);
extern void glfwInitHint(int hint, int value);
static void preferX11(void) {
    // GLFW_PLATFORM and GLFW_PLATFORM_X11 from the pinned GLFW public API.
    if (glfwPlatformSupported(0x00060004)) glfwInitHint(0x00050003, 0x00060004);
}
*/
import "C"

import (
	"os"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// Native Wayland does not let a client read or set its desktop position.
// Use this session's XWayland compositor when the dual-backend development
// build can reach it. The packaged GLFW library already uses X11 on Linux.
func preferPositionableGameWindow() {
	if os.Getenv("WAYLAND_DISPLAY") == "" || os.Getenv("DISPLAY") == "" {
		return
	}
	c, err := xgb.NewConn()
	if err != nil {
		return
	}
	defer c.Close()
	if ext, err := xproto.QueryExtension(c, 8, "XWAYLAND").Reply(); err == nil && ext.Present {
		C.preferX11()
	}
}

// GLFW_PLATFORM_WAYLAND in the GLFW public API.
func nativeWaylandGameWindow() bool { return C.glfwGetPlatform() == 0x00060003 }
