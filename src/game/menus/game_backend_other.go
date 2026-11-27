//go:build !headless && (!linux || !cgo || drm || rgfw || sdl || sdl3 || android)

package menus

func preferPositionableGameWindow() {}

func nativeWaylandGameWindow() bool { return false }
