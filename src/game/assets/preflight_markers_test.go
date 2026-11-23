package assets

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestPreflightStationMarkerOriginalOperands(t *testing.T) {
	root := os.Getenv("GS2000_FIXTURE_ROOT")
	if root == "" {
		t.Skip("optional original-byte check")
	}
	b, err := os.ReadFile(filepath.Join(root, "installation/GS.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	if Hash(b) != IslandIce469085.Source(RoleMainExecutable).SHA256 {
		t.Fatal("source identity mismatch")
	}
	// Independent original operands: initializer crops18 handles,16x16,
	//20 columns, storing at9f1c. ORDNANCE indexes the pair at9f3c.
	for _, c := range []struct {
		at   int
		want []byte
	}{
		{0xf567, []byte{0x83, 0x7e, 0xfe, 0x12}},
		{0xf56d, []byte{0x6a, 0x10, 0x6a, 0x10}},
		{0xf574, []byte{0xb9, 0x14, 0x00}},
		{0xf595, []byte{0xd1, 0xe3, 0x89, 0x87, 0x1c, 0x9f}},
		{0x356fa, []byte{0xd1, 0xe3, 0xff, 0xb7, 0x3c, 0x9f}},
	} {
		if !bytes.Equal(b[c.at:c.at+len(c.want)], c.want) {
			t.Fatalf("original operands at%x", c.at)
		}
	}
	var sources []SourceLocation
	got, err := prepareStationMarkers(&Resource{Payload: b, Source: SourceLocation{Path: "GS.GS2", SHA256: Hash(b)}}, &IslandIce469085, &sources)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != (StationMarker{16, [4]int{256, 0, 16, 16}}) || got[1] != (StationMarker{17, [4]int{272, 0, 16, 16}}) {
		t.Fatalf("marker mapping %+v", got)
	}
	if len(sources) != 2 || sources[0].Offset != 0xf53c || sources[1].Offset != 0x356b2 {
		t.Fatal("marker provenance missing")
	}
	b[0xf595] = 0x90
	if _, err = prepareStationMarkers(&Resource{Payload: b}, &IslandIce469085, &sources); err == nil {
		t.Fatal("changed graphics operand accepted")
	}
}
