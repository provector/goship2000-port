package menus

import (
	"bytes"
	assetdata "gunship2000-assets/game/assets"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTooltipInsetClippingAndTwoScales(t *testing.T) {
	c := renderingController(t)
	font := c.Menus.Fonts[3]
	for _, s := range []string{"ARMAMENT", strings.Repeat("LONG HELP PATH ", 60)} {
		low := bytes.Repeat([]byte{99}, 64000)
		assetdata.DrawBottomTooltip(low, font, s, 7)
		if !bytes.Equal(low[:191*320], bytes.Repeat([]byte{99}, 191*320)) {
			t.Fatal("body changed")
		}
		minX := 320
		for y := 191; y < 200; y++ {
			for x := 0; x < 320; x++ {
				v := low[y*320+x]
				if v != 0 {
					if v != 7 || x < 2 || x >= 318 {
						t.Fatal("outside tooltip clip", x, v)
					}
					minX = min(minX, x)
				}
			}
		}
		// Q2.1 intentionally centers this strip; the old left-inset test is
		// retained verbatim in evidence/stepQ21/prior-tests.
		if len(s) < 100 && minX != (320-font.Measure([]byte(s)))/2 {
			t.Fatal("strip not centered", minX)
		}
		if minX < 2 {
			t.Fatal("outside inset", minX)
		}
		a := make([]byte, 64000)
		b := make([]byte, 256000)
		c.drawOriginalStrip(a, s, 12)
		c.drawOriginalStrip(b, s, 12)
		for y := 191; y < 200; y++ {
			for x := 0; x < 320; x++ {
				at := 2*y*640 + 2*x
				v := a[y*320+x]
				if b[at] != v || b[at+1] != v || b[at+640] != v || b[at+641] != v {
					t.Fatal("2x UI inset changed")
				}
			}
		}
	}
	// Controlled controller frames establish paint/layout; OS input is separate.
	out := os.Getenv("GS2000_Q14_CAPTURE_DIR")
	if out != "" {
		if e := os.MkdirAll(out, 0700); e != nil {
			t.Fatal(e)
		}
		for _, tc := range []struct {
			name  string
			frame []byte
		}{{"office", c.Office.Frame(false)}, {"roster", func() []byte { c.Screen = ScreenRoster; return c.Frame(false) }()}} {
			img, e := assetdata.FrameRGBA(tc.frame, c.Office.Assets.Palette)
			if e != nil {
				t.Fatal(e)
			}
			f, e := os.Create(filepath.Join(out, tc.name+".png"))
			if e != nil {
				t.Fatal(e)
			}
			e = png.Encode(f, img)
			f.Close()
			if e != nil {
				t.Fatal(e)
			}
		}
	}
}
