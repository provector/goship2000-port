package navigation

import (
	"bytes"
	"encoding/binary"
	"gunship2000-assets/game/cockpit"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestGPSDamageOriginalArtworkAndSeverityBits(t *testing.T) {
	s, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional original")
	}
	if err != nil {
		t.Fatal(err)
	}
	a := &cockpit.Assets{Sprites: map[string]cockpit.Image{}, Opaque: map[string][]byte{}}
	for _, v := range []struct {
		name     string
		ip, w, h int
	}{{"gps-damage-body", 0x2257, 91, 67}, {"gps-damage-yellow", 0x2151, 7, 7}, {"gps-damage-red", 0x2186, 7, 7}} {
		at := 0x24290 + v.ip
		if binary.LittleEndian.Uint16(s[at:]) != uint16(v.w) || binary.LittleEndian.Uint16(s[at+2:]) != uint16(v.h) {
			t.Fatal("source dimensions", v.name)
		}
		a.Sprites[v.name] = cockpit.Image{Width: v.w, Height: v.h, Pixels: append([]byte(nil), s[at+4:at+4+v.w*v.h]...)}
	}
	positions := [10][2]int{{285, 22}, {241, 46}, {265, 8}, {263, 55}, {275, 50}, {275, 24}, {245, 25}, {315, 15}, {264, 38}, {275, 41}}
	a.Opaque["gps-damage-positions"] = append([]byte(nil), s[0x2940b:0x2940b+40]...)
	for i, p := range positions {
		if int(binary.LittleEndian.Uint16(a.Opaque["gps-damage-positions"][i*4:])) != p[0] || int(binary.LittleEndian.Uint16(a.Opaque["gps-damage-positions"][i*4+2:])) != p[1] {
			t.Fatal("source point", i)
		}
	}
	for _, severity := range []byte{0, 1, 2, 3, 0x81, 0xc2} {
		p := bytes.Repeat([]byte{0x19}, 64000)
		var d [10]byte
		for i := range d {
			d[i] = severity
		}
		if err := drawDamageDiagram(p, a, d); err != nil {
			t.Fatal(err)
		}
		want := bytes.Repeat([]byte{0x19}, 64000)
		paint := func(img cockpit.Image, x, y int) {
			for yy := 0; yy < img.Height; yy++ {
				for xx := 0; xx < img.Width; xx++ {
					v := img.Pixels[yy*img.Width+xx]
					if v != 0 && x+xx >= 0 && x+xx < 320 && y+yy >= 0 && y+yy < 200 {
						want[(y+yy)*320+x+xx] = v
					}
				}
			}
		}
		paint(a.Sprites["gps-damage-body"], 229, 0)
		if severity&3 != 0 {
			name := "gps-damage-yellow"
			if severity&3 == 2 {
				name = "gps-damage-red"
			}
			for _, point := range positions {
				paint(a.Sprites[name], point[0]-3, point[1]-3)
			}
		}
		if !bytes.Equal(p, want) {
			t.Fatalf("GPS source sprite/low2 composition differs severity%x", severity)
		}
	}
}
