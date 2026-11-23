package cockpit

import (
	"bytes"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestExteriorHUDGateAndMastEmptyRecipe(t *testing.T) {
	a := testAssets()
	for i := range a.Atlases["fixed-strip"].Pixels {
		a.Atlases["fixed-strip"].Pixels[i] = 42
	}
	frame := bytes.Repeat([]byte{99}, 64000)
	if e := Compose(frame, a, Inputs{AircraftID: 0, CameraMode: 3, HUD: true}); e != nil {
		t.Fatal(e)
	}
	// S33-HUD-02: section2:1d1f branches to24d8, retaining common flight
	// information in external views. Only target/mast operations use0/7.
	if frame[40*320+308] != 42 {
		t.Fatal("source common flight HUD absent in chase")
	}
	art := make([]byte, 211*73)
	art[0] = 77
	a.Sprites["mast-view"] = Image{Width: 211, Height: 73, Pixels: art}
	frame = bytes.Repeat([]byte{99}, 64000)
	if e := Compose(frame, a, Inputs{AircraftID: 0, CameraMode: 7, HUD: true, MastEmpty: true, Damage: [10]byte{0x80}}); e != nil {
		t.Fatal(e)
	}
	if frame[127*320+55] != 77 || frame[133*320+70] != 0x15 || frame[127*320+54] != 99 {
		t.Fatal("source empty mast bounds/layering")
	}
	if a.Sprites["mast-view"].Pixels[0] != 77 {
		t.Fatal("drawing changed immutable mast art")
	}
}

func TestOriginalCockpitCameraAndMastOperands(t *testing.T) {
	raw, e := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(e) {
		t.Skip("optional source missing")
	}
	if e != nil {
		t.Fatal(e)
	}
	base := 0x1dc60
	for _, q := range []struct {
		at    int
		bytes []byte
	}{
		{0x1d23, []byte{0x26, 0x83, 0x3e, 0xd1, 0x0a, 0x00, 0x74, 0x0b, 0x26, 0x83, 0x3e, 0xd1, 0x0a, 0x07}},
		{0x222c, []byte{0x68, 0x15, 0x15, 0x68, 0xc4, 0x00, 0x68, 0xfb, 0x00, 0x68, 0x84, 0x00, 0x6a, 0x45}},
		{0x2256, []byte{0x6a, 0x04, 0x6a, 0x7f, 0x6a, 0x37}},
	} {
		if !bytes.Equal(raw[base+q.at:base+q.at+len(q.bytes)], q.bytes) {
			t.Fatalf("source view operand%06x", base+q.at)
		}
	}
}
