//go:build !headless

package menus

import (
	"image"
	"testing"
)

func TestCenterDecoratedClientOnMonitorWorkArea(t *testing.T) {
	for _, tt := range []struct {
		area                                    image.Rectangle
		width, height, left, top, right, bottom int
		want                                    image.Point
	}{
		{image.Rect(67, 32, 2560, 1440), 800, 600, 0, 37, 0, 0, image.Pt(913, 454)},
		{image.Rect(-1920, 40, 0, 1080), 1024, 768, 4, 28, 4, 4, image.Pt(-1472, 188)},
		{image.Rect(0, 32, 900, 632), 900, 600, 0, 28, 0, 0, image.Pt(0, 60)},
	} {
		if got := centeredClient(tt.area, tt.width, tt.height, tt.left, tt.top, tt.right, tt.bottom); got != tt.want {
			t.Errorf("center on %v: got %v, want %v", tt.area, got, tt.want)
		}
	}
}
