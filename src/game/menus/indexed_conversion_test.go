package menus

import (
	"bytes"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/internal/rasterlayout"
	"image"
	"image/color"
	"testing"
)

// V2.2 scalar conversion, including the legacy nil-buffer entry and error order.
func frozenRGBAInto(indices []byte, palette color.Palette, out *image.RGBA) (*image.RGBA, error) {
	w, h, err := indexedFrameSize(indices)
	if err != nil {
		return nil, err
	}
	if w == 320 && out == nil {
		return assetdata.FrameRGBA(indices, palette)
	}
	if len(palette) != 256 {
		return nil, fmt.Errorf("simulation requires256 palette entries")
	}
	if out == nil || out.Bounds() != image.Rect(0, 0, w, h) {
		out = image.NewRGBA(image.Rect(0, 0, w, h))
	}
	var colours [256]color.RGBA
	for i, c := range palette {
		colours[i] = color.RGBAModel.Convert(c).(color.RGBA)
	}
	l := rasterlayout.FromPixels(len(indices))
	bar := l.Top * w
	black := color.RGBA{A: 255}
	for i, v := range indices {
		c := colours[v]
		if i < bar || i >= len(indices)-bar {
			c = black
		}
		j := i * 4
		out.Pix[j], out.Pix[j+1], out.Pix[j+2], out.Pix[j+3] = c.R, c.G, c.B, 255
	}
	return out, nil
}

func TestIndexedConversionEquivalence(t *testing.T) {
	pal := make(color.Palette, 256)
	for i := range pal {
		pal[i] = color.NRGBA{R: byte(i * 3), G: byte(i * 7), B: byte(i), A: byte(255 - i)}
	}
	for _, res := range []string{"320x200", "640x400", "640x480", "800x600", "1024x768", "1280x1024"} {
		l := rasterlayout.ByName(res)
		frame := make([]byte, l.Width*l.Height)
		for i := range frame {
			frame[i] = byte(i * 71)
		}
		var want, got *image.RGBA
		for draw := 0; draw < 4; draw++ {
			if draw == 1 {
				pal[71] = color.NRGBA{R: 190, G: 80, B: 216, A: 127}
			}
			if draw == 2 {
				want = image.NewRGBA(image.Rect(0, 0, 1, 1))
				got = image.NewRGBA(want.Rect)
			}
			if draw == 3 {
				for i := range got.Pix {
					got.Pix[i] = 0x19
					want.Pix[i] = 0x19
				}
				pal[71] = color.Black
			}
			previous := got
			want, _ = frozenRGBAInto(frame, pal, want)
			var err error
			got, err = indexedFrameRGBAInto(frame, pal, got)
			if err != nil || !bytes.Equal(want.Pix, got.Pix) || want.Rect != got.Rect || (draw == 1 || draw == 3) && previous != got {
				t.Fatalf("%s draw %d", res, draw)
			}
		}
		for _, out := range []*image.RGBA{nil, got} {
			_, a := frozenRGBAInto(frame, pal[:255], out)
			_, b := indexedFrameRGBAInto(frame, pal[:255], out)
			if fmt.Sprint(a) != fmt.Sprint(b) {
				t.Fatal(a, b)
			}
		}
	}
	_, a := frozenRGBAInto(nil, nil, nil)
	_, b := indexedFrameRGBAInto(nil, nil, nil)
	if fmt.Sprint(a) != fmt.Sprint(b) {
		t.Fatal(a, b)
	}
}
