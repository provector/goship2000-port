//go:build !headless && !linux && !windows

package menus

import "image"

func (*desktopFullscreen) workArea(m image.Rectangle) (image.Rectangle, int, int, int, int) {
	return m, 0, 0, 0, 0
}
