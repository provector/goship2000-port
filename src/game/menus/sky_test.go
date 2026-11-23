package menus

import (
	"bytes"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/render3d"
	"image"
	"image/color"
	"os"
	"testing"
)

func TestSkyBlendOnlyOwnedPairAndOutsideUI(t *testing.T) {
	p := make(color.Palette, 256)
	for i := range p {
		p[i] = color.RGBA{byte(i), 0, 0, 255}
	}
	indices := []byte{0xa0, 0xa1, 0xa0, 9, 0xa1}
	original := append([]byte(nil), indices...)
	mask := []byte{0xa0, 0xa0, 0, 0xa0, 0xa0}
	out := image.NewRGBA(image.Rect(0, 0, 5, 1))
	for i, c := range indices {
		out.Set(i, 0, p[c])
	}
	blendSkyPresentation(out, indices, mask, p, image.Rect(4, 0, 5, 1))
	if out.RGBAAt(0, 0).R != 0xa1 || out.RGBAAt(1, 0).R != 0xa1 || out.RGBAAt(2, 0).R != 0xa0 || out.RGBAAt(3, 0).R != 9 || out.RGBAAt(4, 0).R != 0xa1 {
		t.Fatal(out.Pix)
	}
	if !bytes.Equal(indices, original) {
		t.Fatal("canonical indices changed")
	}
}

// Supplied source-world renderer witness; no mission, flight or result is staged.
func TestEuroSkyPresentationDoesNotChangeCanonicalFeedback(t *testing.T) {
	a, e := assetdata.LoadWorldAssets(os.Getenv("GS2000_TEST_MISSIONS"))
	if e != nil {
		t.Fatal(e)
	}
	r, e := render3d.New(a)
	if e != nil {
		t.Fatal(e)
	}
	for _, scale := range []int{1, 2} {
		scene := render3d.Scene{World: "EURO6", RenderScale: scale, GraphicsOptions: 2, Camera: render3d.Camera{Pose: render3d.Pose{H0: 200000, H1: 200000, Height: 128}}, View: render3d.FullView()}
		plain, masked := make([]byte, scene.TargetPixels()), make([]byte, scene.TargetPixels())
		if _, e = r.Draw(scene, nil, plain); e != nil {
			t.Fatal(e)
		}
		scene.SkyMask = make([]byte, scene.TargetPixels())
		if _, e = r.Draw(scene, nil, masked); e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(plain, masked) {
			t.Fatal("presentation coverage changed canonical indexed output")
		}
		pal, e := r.Palette(scene, nil)
		if e != nil {
			t.Fatal(e)
		}
		rgba, e := indexedFrameRGBA(masked, pal)
		if e != nil {
			t.Fatal(e)
		}
		blendSkyPresentation(rgba, masked, scene.SkyMask, pal, image.Rectangle{})
		count := 0
		for _, lo := range scene.SkyMask {
			if lo >= 0xa0 && lo < 0xaf {
				count++
			}
		}
		if count == 0 || rgba.Rect.Dx() != 320*scale {
			t.Fatal("source sky ownership absent", scale, count)
		}
		t.Logf("supplied EURO6 renderer scale%d sky-owned%d sourcePixels%d", scale, count, len(masked))
	}
}
