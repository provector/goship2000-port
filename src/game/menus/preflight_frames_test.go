package menus

import (
	assetdata "gunship2000-assets/game/assets"
	"testing"
)

func TestPreflightFrameSourceCornersEdgesAndFill(t *testing.T) {
	a := &assetdata.MissionAssets{Images: map[string][]byte{"POINTER.PIC": make([]byte, 64000)}}
	art := a.Images["POINTER.PIC"]
	for y := 0; y < 5; y++ {
		for x := 0; x < 5; x++ {
			art[(100+y)*320+x] = byte(10*y + x + 1)
		}
	}
	art[105*320], art[105*320+1] = 234, 207
	frame := make([]byte, 64000)
	drawPreflightFrame(a, frame, [4]int{10, 20, 30, 40}, 0, 0)
	for _, c := range []struct {
		x, y int
		want byte
	}{{10, 20, 1}, {39, 20, 5}, {10, 59, 41}, {39, 59, 45}, {20, 20, 3}, {20, 59, 43}, {10, 30, 21}, {39, 30, 25}, {20, 21, 13}, {20, 58, 33}, {11, 30, 22}, {38, 30, 24}, {20, 30, 234}} {
		if got := frame[c.y*320+c.x]; got != c.want {
			t.Fatalf("frame%d,%d=%d want%d", c.x, c.y, got, c.want)
		}
	}
	drawPreflightFrame(a, frame, [4]int{50, 20, 20, 20}, 0, 1)
	if frame[30*320+60] != 207 || frame[19*320+20] != 0 {
		t.Fatal("alternate fill or bounds changed")
	}
}
