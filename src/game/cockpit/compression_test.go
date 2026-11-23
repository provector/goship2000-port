package cockpit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"gunship2000-assets/internal/testfixture"
	"os"
	"path/filepath"
	"testing"
)

func sourceDigitMask(t *testing.T) []byte {
	t.Helper()
	var b []byte
	var err error
	if root := os.Getenv("GS2000_TEST_MISSIONS"); root != "" {
		b, err = os.ReadFile(filepath.Join(root, "world/cockpit/raw/digits-zero-mask.bin"))
	} else {
		var original []byte
		original, err = os.ReadFile(testfixture.Path("installation/GS2.GS2"))
		if err == nil {
			b = append([]byte(nil), original[0x41429:0x41429+240]...)
		}
	}
	if os.IsNotExist(err) {
		t.Skip("source digit bytes or complete prepared bundle required")
	}
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 240 || fmt.Sprintf("%x", sha256.Sum256(b)) != "e5a6c90f53f98b7006b6187b8e0ff9e12262b6490afec20149edb3b9c6f3ad7d" {
		t.Fatal("not the independently witnessed source digit mask")
	}
	return b
}

func TestCompressionOriginalCallerAndMasks(t *testing.T) {
	original, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err == nil {
		want, _ := hex.DecodeString("68020126ff36f30a6a086a109a8d197f03")
		if !bytes.Equal(original[0xf9ad:0xf9ad+len(want)], want) {
			t.Fatal("original AF3/style/y8/x16 caller changed")
		}
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	source := sourceDigitMask(t)
	a := hudTestAssets()
	a.Sprites["mast-view"] = Image{Width: 211, Height: 73, Pixels: make([]byte, 211*73)}
	a.Atlases["digits-zero-mask"] = Image{Width: 4, Height: 60, Pixels: source}
	for _, scale := range []int{1, 2} {
		for _, view := range []int{0, 3, 6, 7} {
			for _, damaged := range []byte{0, 0x80} {
				in := Inputs{AircraftID: 0, CameraMode: view, HUD: true, MastEmpty: true, MissionCounter: 1234, HUDScale: 128, FuelMaximum: 100, Damage: [10]byte{damaged}}
				base := make([]byte, 64000*scale*scale)
				for i := range base {
					base[i] = byte(40 + i%17)
				}
				baseline := append([]byte(nil), base...)
				if err := ComposeScaled(baseline, a, in, scale); err != nil {
					t.Fatal(err)
				}
				for _, request := range []int16{0, 2, 4, 6, 8, 10, 12, 14, 16} {
					t.Run(fmt.Sprintf("scale%d/view%d/damage%d/request%d", scale, view, damaged, request), func(t *testing.T) {
						dst := append([]byte(nil), base...)
						in.CompressionRequest = request
						if err := ComposeScaled(dst, a, in, scale); err != nil {
							t.Fatal(err)
						}
						// Expected pixels come straight from the verified original atlas and the
						// literal source16,8/4x6/two-digit operation, not Number's own output.
						width := 320 * scale
						for y := 0; y < 200*scale; y++ {
							for x := 0; x < width; x++ {
								i := y*width + x
								want := baseline[i]
								if x >= 16*scale && x < 24*scale && y >= 8*scale && y < 14*scale {
									want = base[i]
									if request != 0 {
										column := (x/scale - 16) / 4
										digit := int(request) / 10
										if column == 1 {
											digit = int(request) % 10
										}
										v := source[digit*24+(y/scale-8)*4+(x/scale-16)%4]
										if v != 0 {
											want = v
										}
									}
								}
								if dst[i] != want {
									t.Fatalf("pixel%d,%d got%d expected%d", x, y, dst[i], want)
								}
							}
						}
						again := append([]byte(nil), base...)
						if err := ComposeScaled(again, a, in, scale); err != nil {
							t.Fatal(err)
						}
						if !bytes.Equal(dst, again) {
							t.Fatal("repeat composition changed source masks")
						}
					})
				}
			}
		}
	}
}
