package cockpit

import (
	"bytes"
	"encoding/binary"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestDamageOverlayOriginalSpriteAndLogicalPlanes(t *testing.T) {
	source, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional original source")
	}
	if err != nil {
		t.Fatal(err)
	}
	// Independent section7 file3e550+6138, selected VGA sprite with flags7.
	raw := source[0x44688 : 0x44688+158]
	if binary.LittleEndian.Uint16(raw) != 11 || binary.LittleEndian.Uint16(raw[2:]) != 14 {
		t.Fatal("source crack dimensions")
	}
	a := &Assets{Sprites: map[string]Image{"driver-conditional": {Width: 11, Height: 14, Pixels: append([]byte(nil), raw[4:]...)}}}
	for _, scale := range []int{1, 2} {
		for _, level := range []byte{0, 1, 2, 3} {
			for _, camera := range []int{0, 7} {
				in := Inputs{CameraMode: camera, DamageOverlay: level, DamageOverlayPoints: [2][2]int16{{80, 60}, {260, 70}}}
				p := bytes.Repeat([]byte{0x17}, 64000*scale*scale)
				if err := DrawDamageOverlayScaled(p, a, in, scale); err != nil {
					t.Fatal(err)
				}
				want := bytes.Repeat([]byte{0x17}, len(p))
				n := 0
				if camera == 0 && level != 0 {
					n = 1
					if level == 2 {
						n = 2
					}
				}
				for _, point := range in.DamageOverlayPoints[:n] {
					for y := 0; y < 14; y++ {
						for x := 0; x < 11; x++ {
							v := raw[4+y*11+x]
							if v == 0 {
								continue
							}
							for dy := 0; dy < scale; dy++ {
								for dx := 0; dx < scale; dx++ {
									at := ((int(point[1])-7+y)*scale+dy)*(320*scale) + (int(point[0])-5+x)*scale + dx
									want[at] = v
								}
							}
						}
					}
				}
				if !bytes.Equal(p, want) {
					t.Fatalf("source crack placement differs scale%d level%d camera%d", scale, level, camera)
				}
				if !bytes.Equal(a.Sprites["driver-conditional"].Pixels, raw[4:]) {
					t.Fatal("draw mutated source sprite")
				}
			}
		}
	}
}

func TestPairedAmmoDamageUsesSourceFullRemainingQuantity(t *testing.T) {
	for _, c := range []struct {
		q               int16
		port, starboard byte
		want            string
	}{
		{37, 0, 0, "19 - 18"}, {37, 2, 0, "0 - 37"}, {37, 0, 2, "37 - 0"}, {37, 2, 2, "0 - 0"}, {38, 1, 0, "0 - 38"}, {-1, 0, 0, "0 - -1"},
	} {
		if got := primaryAmmunitionText(Station{Mode: 3, Quantity: c.q}, c.port, c.starboard); got != c.want {
			t.Fatal(got, c.want)
		}
	}
}
