package render3d

import (
	"bytes"
	"gunship2000-assets/game/world"
	"testing"
)

func TestTransparentPadPreservesUnderlyingMaterialOwner(t *testing.T) {
	for _, scale := range []int{1, 2} {
		for _, geometry := range []string{"model/130", "model/131"} {
			scene := Scene{World: "EURO7", RenderScale: scale, View: FullView()}
			base := bytes.Repeat([]byte{0x7f}, scene.TargetPixels())
			masked := bytes.Clone(base)
			mask := bytes.Repeat([]byte{0x5f}, scene.TargetPixels())
			verts := []Vector{{-130 << 16, -55 << 16, 512 << 16}, {132 << 16, -55 << 16, 512 << 16}, {132 << 16, 58 << 16, 512 << 16}, {-130 << 16, 58 << 16, 512 << 16}}
			p := world.Primitive{Opcode: 4, Colour: 0xec00, Indices: []int{0, 1, 2, 3}}
			normal := frame{scene: scene, dst: base, view: scene.rasterView(), geometryID: geometry, edges: map[byte]screenEdge{}}
			normal.primitive(p, verts, false, false)
			scene.SkyMask = mask
			f := frame{scene: scene, dst: masked, view: scene.rasterView(), geometryID: geometry, edges: map[byte]screenEdge{}}
			if geometry == "model/131" {
				f.material = materialBinding{role: GroundContactPad}
			} // Explicit supplied coverage fixture; R7 checks actual source admission.
			f.primitive(p, verts, false, false)
			if !bytes.Equal(base, masked) {
				t.Fatal("mask changed canonical pad raster", scale)
			}
			writes, holes := 0, 0
			for i, pixel := range masked {
				if pixel == 0xec {
					writes++
					want := byte(1)
					if geometry == "model/131" {
						want = 0x5b
					}
					if mask[i] != want {
						t.Fatal("marking failed to erase presentation owner", scale, i, mask[i])
					}
				}
				if mask[i] == 0x5f {
					holes++
					if pixel != 0x7f {
						t.Fatal("owner survived an actual marking write", scale, i)
					}
				}
			}
			if writes == 0 || holes == 0 {
				t.Fatal("test did not cover both pad writes and destination-preserving holes", scale, writes, holes)
			}
		}
	}
}
