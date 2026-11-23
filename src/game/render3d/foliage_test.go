package render3d

import (
	"gunship2000-assets/game/world"
	"testing"
)

func TestCompactFoliageDisplayAdmission(t *testing.T) {
	b := tinyBundle()
	b.TerrainFeedback = testTerrainFeedback(t)
	f := frame{r: &Renderer{Assets: b}, scene: Scene{GraphicsOptions: 1}}
	for _, c := range []struct {
		cb   int
		op   byte
		word uint16
		want bool
	}{
		{1, 3, 0x24, true}, {4, 3, 0x24, true}, {4, 3, 0x26, true}, {4, 3, 0x28, true}, {5, 1, 0x20, true}, {5, 1, 0xb0, true},
		{0, 3, 0x24, false}, {1, 3, 0x26, false}, {4, 4, 0xb8b8, false}, {4, 3, 0x2424, false}, {5, 3, 0x20, false}, {4, 1, 0xb0, false},
	} {
		f.compactCallback = c.cb
		p := world.Primitive{Opcode: c.op, Colour: c.word}
		if got := f.foliageFeedback(p) != nil; got != c.want {
			t.Fatalf("%+v got %t", c, got)
		}
		for _, s := range []Scene{{GraphicsOptions: 1, SourceFillReference: true}, {GraphicsOptions: 0}, {GraphicsOptions: 1, MaterialCarrier: &Carrier{}}} {
			f.scene = s
			if f.foliageFeedback(p) != nil {
				t.Fatal("reference, simplified or explicit carrier changed", c)
			}
		}
		f.scene = Scene{GraphicsOptions: 1}
	}
}
