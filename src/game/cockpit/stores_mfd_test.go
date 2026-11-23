package cockpit

import (
	"bytes"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestStoresMFDFormatter41SourceCases(t *testing.T) {
	for _, tc := range []struct {
		value int16
		want  string
	}{{0, "0%"}, {1, "1%"}, {99, "99%"}, {100, "100%"}, {-1, "65535%"}} {
		if got := storesFuelText(tc.value); got != tc.want {
			t.Fatalf("source unsigned format41: %q != %q", got, tc.want)
		}
	}
}

func TestStoresMFDSourcePlacementAndPostRemap(t *testing.T) {
	a := testAssets()
	a.Literals["fuel"] = "FUEL"
	a.ScenePresentation.ConfirmationRemap = make([]byte, 256)
	for i := range a.ScenePresentation.ConfirmationRemap {
		a.ScenePresentation.ConfirmationRemap[i] = byte(i & 15)
	}
	// One bit per glyph gives independently countable source anchors.
	clear(a.Font.Rows)
	for i := 0; i < 95; i++ {
		a.Font.Rows[i*6] = 0x80
	}
	d := Definition{}
	for off, v := range map[int]int16{0x54: 20, 0x56: 100, 0x58: 100, 0x5a: 145} {
		d.Parameters[off/2] = v
	}
	in := Inputs{Fuel: 50, Stations: [4]Station{{SystemID: 1, Name: "A", Quantity: 20}, {SystemID: 2, Name: "B", Quantity: 10}}}
	frame := bytes.Repeat([]byte{7}, 64000)
	if err := a.storesMFD(frame, d, false, in); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		x, y int
		want byte
	}{
		{19, 100, 7}, {20, 100, 28}, {100, 145, 28}, {101, 145, 7},
		{22, 106, 31},                               // FUEL: left+2,top+6, 1a remapped to15+0a
		{80, 106, 31}, {86, 106, 31}, {92, 106, 31}, //50%, centered about right-12
		{22, 112, 31}, {88, 112, 28}, //ID1 name; quantity intentionally absent
		{22, 118, 31}, {83, 118, 31}, {89, 118, 31}, //ID2 quantity10
	} {
		if got := frame[tc.y*320+tc.x]; got != tc.want {
			t.Fatalf("MFD source pixel%d,%d got%d want%d", tc.x, tc.y, got, tc.want)
		}
	}
	frame = bytes.Repeat([]byte{7}, 64000)
	in.SystemFlags = 1
	if err := a.storesMFD(frame, d, false, in); err != nil {
		t.Fatal(err)
	}
	if frame[100*320+20] != 7 || frame[106*320+22] != 0x1a {
		t.Fatal("alternate source panel must skip final brightness remap")
	}
}

func TestStoresMFDBoundedSourceOperands(t *testing.T) {
	raw, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional original unavailable")
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		file int
		hex  string
	}{
		{0x600 + 0x1b4, "f64608407406c684543525"},
		{0x1dc60 + 0x1617, "6812086a418e06b83326ff361700"},
		{0x1dc60 + 0x16a5, "8e06b8338b5ef22680bf0100017e9e"},
		{0x3e550 + 0x87, "ea340e471d"},
		{0x3e550 + 0xe6c, "268a052ed702c4aae2f6"},
	} {
		want, err := hex.DecodeString(tc.hex)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(raw[tc.file:tc.file+len(want)], want) {
			t.Fatalf("MFD source mismatch file%x", tc.file)
		}
	}
}

func TestRuntimePresenceRetainsRawZeroAndOneStores(t *testing.T) {
	a := testAssets()
	a.Literals["fuel"] = "FUEL"
	clear(a.Font.Rows)
	for i := 0; i < 95; i++ {
		a.Font.Rows[i*6] = 0x80
	}
	var d Definition
	for off, v := range map[int]int16{0x54: 20, 0x56: 100, 0x58: 100, 0x5a: 145} {
		d.Parameters[off/2] = v
	}
	// These remain real accepted raw IDs. Explicit runtime presence supplies the
	// domain boundary; nothing globally rebases WSHD0/1 or accepted rows.
	in := Inputs{SystemFlags: 1, Fuel: 100, Stations: [4]Station{
		{Present: true, SystemID: 0, Name: "STINGER", Quantity: 2},
		{Present: true, SystemID: 1, Name: "SIDEWNDR", Quantity: 3},
	}}
	p := make([]byte, 64000)
	if err := a.storesMFD(p, d, false, in); err != nil {
		t.Fatal(err)
	}
	for _, y := range []int{112, 118} {
		if p[y*320+22] != 0x1a || p[y*320+86] != 0x1a {
			t.Fatal("present raw0/1 weapon name or quantity suppressed", y)
		}
	}
}
