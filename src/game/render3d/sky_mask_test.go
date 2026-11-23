package render3d

import (
	"bytes"
	"gunship2000-assets/game/world"
	"testing"
)

func TestSkyMaskErasesGeometryEvenWhenInkMatchesSky(t *testing.T) {
	dst := bytes.Repeat([]byte{0xa0}, 64000)
	mask := bytes.Repeat([]byte{0xa0}, 64000)
	f := frame{r: &Renderer{Assets: &world.Bundle{}}, scene: Scene{SkyMask: mask}, dst: dst, view: FullView(), edges: map[byte]screenEdge{}}
	p := world.Primitive{Opcode: 3, Colour: 0xa0a0, Indices: []int{0, 1, 2}}
	f.primitive(p, []Vector{{-10 << 16, 0, 128 << 16}, {10 << 16, 0, 128 << 16}, {0, 10 << 16, 128 << 16}}, false, false)
	count := 0
	for _, v := range mask {
		if v == 1 {
			count++
		}
	}
	if count == 0 || count == len(mask) {
		t.Fatal("geometry ownership mask", count)
	}
	for _, v := range dst {
		if v != 0xa0 {
			t.Fatal("presentation mask changed source indexed pixels")
		}
	}
}
