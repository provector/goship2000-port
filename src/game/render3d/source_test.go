package render3d

import (
	"bytes"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestMastLaterWriterOriginalBytes(t *testing.T) {
	raw, e := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(e) {
		t.Skip("optional original source unavailable")
	}
	if e != nil {
		t.Fatal(e)
	}
	// Correct named section1 base d8b0 (not the other0975 overlay or old10bb base).
	for ip, want := range map[int][]byte{0x1563: {0x83, 0x3e, 0x3a, 0x31, 0, 0x75, 0x15}, 0x156a: {0xb8, 0xa2, 0x3b, 0x8b, 0xf8, 0xbe, 0x98, 0x3b}, 0x1574: {0xb9, 5, 0, 0xf3, 0xa5}, 0x1579: {0x81, 6, 0xaa, 0x3b, 0x80, 0}, 0x95a: {0x2b, 0xc0, 0xa3, 0x3a, 0x31}, 0x720: {0x6a, 0, 0x68, 0xc8, 0, 0x68, 0x40, 1, 0x50, 0x6a, 0, 0x9a, 0x97, 0x19, 0x7f, 3}} {
		if !bytes.Equal(raw[0xd8b0+ip:][:len(want)], want) {
			t.Fatalf("source mast writer %04x", ip)
		}
	}
	in := CameraInputs{AircraftSelector: 1, Player: CameraAnchor{Camera: Camera{Pose: Pose{Height: 362}}}}
	c := NewCameraController(in)
	if _, e = c.Select(2, false, in, nil); e != nil {
		t.Fatal(e)
	}
	if c.Camera.Height != 490 {
		t.Fatal("362+128 source carrier mast offset", c.Camera.Height)
	}
	in.Player.Camera.Height = 300
	if e = c.Tick(in, nil); e != nil {
		t.Fatal(e)
	}
	if c.Camera.Height != 428 {
		t.Fatal("later writer must refresh rather than accumulate", c.Camera.Height)
	}
}
