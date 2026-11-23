package assets

import (
	"encoding/binary"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestPreparedGearBindingOriginalGraphAndWriters(t *testing.T) {
	gs, err := os.ReadFile(testfixture.Path("installation/GS.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional original unavailable")
	}
	if err != nil {
		t.Fatal(err)
	}
	gs2, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	objs, err := os.ReadFile(testfixture.Path("installation/OBJFILE"))
	if err != nil {
		t.Fatal(err)
	}
	tables, err := prepareMissionInitialization(gs, gs2, &IslandIce469085)
	if err != nil {
		t.Fatal(err)
	}
	want := GearGraphBinding{ModelID: 139, Node: 46, Side: 0, DownChild: 41, UpChild: 255}
	if tables.Version != 3 || tables.GearGraph != want {
		t.Fatalf("prepared component%+v", tables)
	}
	// Independently walk original record139's five framing words, rather than
	// using the production graph parser or compiled binding as an oracle.
	lo := int(binary.LittleEndian.Uint32(objs[139*4:]))
	at := lo
	var blocks [5][]byte
	for i := range blocks {
		n := int(binary.LittleEndian.Uint16(objs[at:]))
		blocks[i] = objs[at+2 : at+2+n]
		at += 2 + n
	}
	if lo != 130572 || len(blocks[3]) != 1033 || blocks[3][0x5d] != 0x29 || blocks[3][0x5e] != 43 || 1+2*46 != 0x5d {
		t.Fatal("original near-graph layout changed")
	}
	if gs2[0xd8b0+0xcf8] != 255 || gs2[0xd8b0+0xd10] != 41 {
		t.Fatal("source gear-up/down writer bytes")
	}
	copyProfile := IslandIce469085
	copyLayout := *copyProfile.InitializationLayout
	copyLayout.GearGraphSource[0].File++
	copyProfile.InitializationLayout = &copyLayout
	if _, err := prepareMissionInitialization(gs, gs2, &copyProfile); err == nil {
		t.Fatal("bad source mapping accepted")
	}
}
