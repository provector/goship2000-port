package render3d

import (
	"bytes"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestSourceCameraContactCallOperands(t *testing.T) {
	_ = sourceCameraTables(t) // Independently hash-binds these raw operand checks.
	raw, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	for _, ip := range []int{0x16bb, 0x17b5, 0x1942, 0x1c62, 0x1dfd, 0x1f65} {
		if !bytes.Equal(raw[0xd8b0+ip:][:5], []byte{0x9a, 0x4c, 0x31, 0x7f, 0x03}) {
			t.Fatalf("ordinary camera%04x no longer source314c", ip)
		}
	}
	if !bytes.Equal(raw[0x21b90+0x16f9:][:5], []byte{0x9a, 0x4c, 0x31, 0x7f, 0x03}) {
		t.Fatal("replay camera source314c")
	}
	// Tactical/reverse take the direct16d2 store, skipping16cf's extra32.
	for at, want := range map[int][]byte{0x1e08: {0x05, 0x20, 0x00, 0xe9, 0x65, 0x01}, 0x1f70: {0x05, 0x40, 0x00}, 0x1f7c: {0xe9, 0x53, 0xf7}} {
		if !bytes.Equal(raw[0xd8b0+at:][:len(want)], want) {
			t.Fatalf("source direct floor store%04x", at)
		}
	}
}

func TestOrdinaryFloorCallersKeepTheirDistinctRecipes(t *testing.T) {
	a := sourceCameraTables(t)
	for _, current := range []bool{false, true} {
		for _, ground := range []int16{0, 360} {
			for _, mode := range []uint8{3, 4, 5, 6, 9, 10} {
				in := cameraInputFixture()
				in.Player.Camera.Pose = Pose{H0: 0x20000, H1: 0x20000}
				in.SourceFloorRules = current
				calls := 0
				in.GroundHeight = func(Pose) (int16, error) { calls++; return ground, nil }
				target := in.Player
				target.ID, target.Extent = "target", 64
				in.Target = &target
				in.ProjectileView = &ProjectileCameraInputs{LastActive: true, Lifetime: 1, Current: target}
				c := NewCameraController(in)
				c.Mode, c.TrackedID = mode, target.ID
				in.Player.Camera.Height, target.Camera.Height = uint16(ground-1000), uint16(ground-1000)
				in.Target = &target
				in.ProjectileView.Current = target
				c.Camera.Height = uint16(ground - 1000)
				c.remoteValid, c.remoteDelta = true, [3]int16{0, 0, 512}
				if err := c.Tick(in, a); err != nil {
					t.Fatal(mode, err)
				}
				want := uint16(ground + 36) // Chase/remote: q+4, then32.
				if mode == 6 {
					want = uint16(ground + 68) // Projectile: q+4, then64.
				}
				if mode == 9 {
					want = uint16(ground + 64)
					if current {
						want = uint16(ground + 32) // Tactical direct q+32.
					}
				}
				if mode == 10 {
					want = uint16(ground + 96)
					if current {
						want = uint16(ground + 64) // Reverse direct q+64.
					}
				}
				if calls != 1 || c.Camera.Height != want {
					t.Fatal("camera floor caller recipe", current, ground, mode, calls, c.Camera.Height, want)
				}
			}
		}
	}
}

func TestCameraFloorStrictSignedThresholds(t *testing.T) {
	for _, recipe := range [][2]int16{{4, 32}, {4, 64}, {32, 0}, {64, 0}} {
		for _, ground := range []int16{-1, 0, 360, 32760} {
			// Expected source word sums are explicit: wrap ADD before signed CMP.
			boundary := int16(uint16(ground) + uint16(recipe[0]))
			raised := int16(uint16(boundary) + uint16(recipe[1]))
			for _, offset := range []int16{-1, 0, 1} {
				height := boundary + offset
				c := &CameraController{Mode: 3, Camera: Camera{Pose: Pose{Height: uint16(height)}}}
				in := CameraInputs{GroundHeight: func(Pose) (int16, error) { return ground, nil }}
				if err := c.floor(in, recipe[0], recipe[1]); err != nil {
					t.Fatal(err)
				}
				want := height
				if offset == -1 {
					want = raised
				}
				if got := int16(c.Camera.Height); got != want {
					t.Fatal(recipe, ground, offset, got, want)
				}
			}
		}
	}
}
