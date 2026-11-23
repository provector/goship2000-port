package render3d

import (
	"bytes"
	"gunship2000-assets/game/world"
	"testing"
)

func TestImpactSpriteDepthPhaseCenterTransparencyAndScale(t *testing.T) {
	c := &world.CombatPresentation{}
	for i := 0; i < 28; i++ {
		c.ImpactSprites = append(c.ImpactSprites, world.EffectSprite{Width: 3, Height: 3, Pixels: []byte{0, byte(i + 1), 0, byte(i + 1), 0, byte(i + 1), 0, byte(i + 1), 0}})
	}
	for _, scale := range []int{1, 2} {
		s := Scene{RenderScale: scale, CombatPhase: 2, View: FullView()}
		dst := bytes.Repeat([]byte{99}, s.TargetPixels())
		f := frame{r: &Renderer{Assets: &world.Bundle{Combat: c}}, scene: s, dst: dst, view: s.rasterView()}
		// Exact source operands: depth0200 selects band2, phase2=>sprite10;
		// quotient(+512*256)/512=256? Here x=0,y=0 centers at160,70.
		f.drawImpact(Vector{0, 0, 0x200 << 16})
		for y := 0; y < 200*scale; y++ {
			for x := 0; x < 320*scale; x++ {
				want := byte(99)
				if x >= 159*scale && x < 162*scale && y >= 69*scale && y < 72*scale {
					sx, sy := (x-159*scale)/scale, (y-69*scale)/scale
					if (sx+sy)&1 != 0 {
						want = 11
					}
				}
				if dst[y*320*scale+x] != want {
					t.Fatalf("scale%d source center/00 transparency at%d,%d", scale, x, y)
				}
			}
		}
		before := append([]byte(nil), dst...)
		f.drawImpact(Vector{0, 0, 16 << 16})
		if !bytes.Equal(dst, before) {
			t.Fatal("sourcez<=16 guard")
		}
	}
}
