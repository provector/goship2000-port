package world

import (
	"gunship2000-assets/internal/testfixture"
	"os"
	"reflect"
	"testing"
)

func TestPlayerDecoySourceDescriptors(t *testing.T) {
	raw, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), raw...)
	geometries, err := PlayerDecoyGeometries(raw)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []struct {
		id      string
		kind    EmbeddedVertexPreparation
		n       int
		colour  uint16
		indices [2][3]int
	}{
		{PlayerFlareGeometry, EmbeddedPlayerFlare, 5, 0x9f9f, [2][3]int{{2, 0, 3}, {1, 0, 4}}},
		{PlayerChaffGeometry, EmbeddedPlayerChaff, 12, 0xf000, [2][3]int{{3, 4, 5}, {0, 1, 2}}},
	} {
		g := geometries[i]
		if g.ID != want.id || g.EmbeddedPreparation != want.kind || len(g.Vertices) != want.n || g.Extent != 0 || g.CompactCallback != 0 || g.Generator != nil || len(g.DirectGroups) != 2 {
			t.Fatalf("descriptor%d: %+v", i, g)
		}
		for j, group := range g.DirectGroups {
			p := group.Primitive
			if p == nil || p.Opcode != 3 || p.Colour != want.colour || !reflect.DeepEqual(p.Indices, want.indices[j][:]) {
				t.Fatalf("group%d:%+v", j, group)
			}
		}
	}
	chaff := geometries[1].Vertices
	if !reflect.DeepEqual(chaff[:6], []Vertex{{-12, -20, -16}, {0, 10, 0}, {12, 8, 20}, {-20, -8, 16}, {0, -10, 8}, {14, 6, -16}}) || chaff[6] != (Vertex{3, 16624, 16961}) || chaff[11] != (Vertex{}) {
		t.Fatal("full12 signed triples changed", chaff)
	}
	if !reflect.DeepEqual(raw, before) {
		t.Fatal("decoding changed source")
	}
	bad := append([]byte(nil), raw...)
	bad[0x61bc0+0x30ce+20] ^= 1
	if _, err := PlayerDecoyGeometries(bad); err == nil {
		t.Fatal("unknown executable identity accepted")
	}
}
