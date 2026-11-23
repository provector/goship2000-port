package cockpit

import (
	"bytes"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestPairedQuantityOriginalLiteralAndGlyphs(t *testing.T) {
	raw, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional original unavailable")
	}
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw[0x61bc0+0x3142:0x61bc0+0x3146], []byte(" - \x00")) {
		t.Fatal("paired source separator changed")
	}
	a := hudTestAssets()
	// Isolate the source quantity anchor from all other HUD content. Glyphs
	// have distinct first-row bit patterns so '-' cannot pass as '/'.
	clear(a.Font.Rows)
	for i, ch := range []byte("19 -8/") {
		a.Font.Rows[(int(ch)-32)*6] = byte(0x80 >> i)
	}
	d := a.Definitions["APA"]
	d.Parameters[0], d.Parameters[2], d.Parameters[3] = 100, 110, 120
	a.Definitions["APA"] = d
	for _, tc := range []struct {
		quantity int16
		want     string
	}{{38, "19 - 19"}, {37, "19 - 18"}} {
		p := make([]byte, 64000)
		in := Inputs{AircraftID: 0, HUD: true, Stations: [4]Station{{Present: true, Quantity: tc.quantity, Mode: 3}}, Damage: [10]byte{0x80}}
		if err := Compose(p, a, in); err != nil {
			t.Fatal(err)
		}
		want := make([]byte, 64000)
		if err := a.Text(want, tc.want, 100, 120, 0x0a, 0x802, Full); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(p[120*320:126*320], want[120*320:126*320]) {
			t.Fatalf("paired%d differs from original literal %q", tc.quantity, tc.want)
		}
	}
}

func TestPrimaryStoresDamageGatePreservesDestination(t *testing.T) {
	a := hudTestAssets()
	a.Literals["fuel"] = "FUEL"
	a.ScenePresentation.ConfirmationRemap = make([]byte, 256)
	for i := range a.ScenePresentation.ConfirmationRemap {
		a.ScenePresentation.ConfirmationRemap[i] = byte(i & 15)
	}
	d := a.Definitions["APA"]
	for off, v := range map[int]int16{0x54: 80, 0x56: 100, 0x58: 150, 0x5a: 145} {
		d.Parameters[off/2] = v
	}
	a.Definitions["APA"] = d
	for _, mask := range []byte{0, 0x20} {
		p := bytes.Repeat([]byte{7}, 64000)
		in := Inputs{AircraftID: 0, Arrangement: 2, HUD: true, PrimaryMFD: 4, Damage: [10]byte{mask | 0x80}}
		// Arrangement0 admits primary dispatch; artwork only replaces one
		// supplied mask pixel, far outside this checked source MFD rectangle.
		in.Arrangement = 0
		if err := Compose(p, a, in); err != nil {
			t.Fatal(err)
		}
		got := p[140*320+140]
		if mask == 0 && got != 28 {
			t.Fatalf("healthy stores remap missing: %d", got)
		}
		if mask != 0 && got != 7 {
			t.Fatalf("damaged stores touched retained destination: %d", got)
		}
	}
}
