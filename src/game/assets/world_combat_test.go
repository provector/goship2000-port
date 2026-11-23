package assets

import (
	"encoding/binary"
	"gunship2000-assets/game/world"
	"gunship2000-assets/internal/testfixture"
	"os"
	"reflect"
	"testing"
)

func TestCombatPreparedGLBRoundTripAgainstOriginalTriples(t *testing.T) {
	root := os.Getenv("GS2000_TEST_PREPARED")
	if root == "" {
		t.Skip("prepared combat tree opt-in")
	}
	b, err := LoadWorldAssets(root)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil {
		t.Skip("optional original source witness unavailable")
	}
	if Hash(raw) != IslandIce469085.Source(RoleSimulationExecutable).SHA256 {
		t.Fatal("source identity")
	}
	l := islandIceCombatLayout
	for i, id := range b.Combat.Templates {
		header := raw[l.HeaderFile+i*38 : l.HeaderFile+(i+1)*38]
		at := l.FarPoolFile + int(binary.LittleEndian.Uint16(header[14:]))
		count := int(binary.LittleEndian.Uint16(raw[at:]))
		var want []world.Vertex
		for n := 0; n < count; n++ {
			var v world.Vertex
			for j := range v {
				v[j] = int16(binary.LittleEndian.Uint16(raw[at+2+6*n+2*j:]))
			}
			want = append(want, v)
		}
		if !reflect.DeepEqual(b.Geometries[id].Vertices, want) {
			t.Fatal("GLB lost signed original triples", i)
		}
		if len(b.Geometries[id].DirectGroups) == 0 && len(b.Geometries[id].PlaneGroups) == 0 {
			t.Fatal("decorative/unused geometry carrier", i)
		}
	}
	for i, s := range b.Combat.ImpactSprites {
		at := l.DriverFile + int(binary.LittleEndian.Uint16(raw[l.DriverFile+l.SpriteLookupOffset+2*i:]))
		if !reflect.DeepEqual(s.Pixels, raw[at+4:at+4+s.Width*s.Height]) {
			t.Fatal("indexed effect payload changed", i)
		}
	}
}
