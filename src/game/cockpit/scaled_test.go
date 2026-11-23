package cockpit

import (
	"bytes"
	"testing"
)

func TestScaledAllAircraftArrangementsAndMasks(t *testing.T) {
	a := hudTestAssets()
	for id := 0; id < 8; id++ {
		for arrangement := 0; arrangement < 4; arrangement++ {
			low := bytes.Repeat([]byte{99}, 64000)
			high := bytes.Repeat([]byte{99}, 256000)
			in := Inputs{AircraftID: id, Arrangement: arrangement, HUD: true, Height: 150, Heading: 0x4321, HUDScale: 128, Vertical: 87, Lateral: -4, Navigation: &NavigationBearing{Active: true, Bearing: 0x4000}}
			if e := Compose(low, a, in); e != nil {
				t.Fatal(e)
			}
			if e := ComposeScaled(high, a, in, 2); e != nil {
				t.Fatal(e)
			}
			for y := 0; y < 200; y++ {
				for x := 0; x < 320; x++ {
					for dy := 0; dy < 2; dy++ {
						for dx := 0; dx < 2; dx++ {
							if high[(y*2+dy)*640+x*2+dx] != low[y*320+x] {
								t.Fatalf("aircraft%d arrangement%d mismatch at%d,%d", id, arrangement, x, y)
							}
						}
					}
				}
			}
		}
	}
}
func TestLogicalPassKeepsPhysicalDestinationIdentity(t *testing.T) {
	dst := bytes.Repeat([]byte{3}, 256000)
	dst[0] = 2
	dst[1] = 9
	dst[640] = 2
	dst[641] = 8
	if e := LogicalPass(dst, 2, func(p []byte) error {
		if p[0] == 2 {
			p[0] = 7
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	if dst[0] != 7 || dst[1] != 9 || dst[640] != 7 || dst[641] != 8 {
		t.Fatal("exact match lost physical destination", dst[:2], dst[640:642])
	}
}
