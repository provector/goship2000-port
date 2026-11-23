package render3d

import (
	"bytes"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestProjectileCameraSourceOperands(t *testing.T) {
	b, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	for ip, want := range map[int][]byte{0x1ad7: {0x26, 0x6b, 0x1e, 0xc7, 0x0a, 0x1a}, 0x1b15: {0x26, 0xa1, 0xc9, 0x0a}, 0x1c50: {0x03, 0x06, 0x8c, 0x3b, 0x99, 0x2b, 0xc2, 0xd1, 0xf8, 0x05, 0x40, 0}, 0x1c7c: {0xc7, 0x06, 0x70, 0x35, 0x60, 0xf0}} {
		if !bytes.Equal(b[0xd8b0+ip:][:len(want)], want) {
			t.Fatalf("source section1:%04x", ip)
		}
	}
}

func TestProjectileCameraPairHeightYawAndTailReturn(t *testing.T) {
	a := sourceCameraTables(t)
	in := CameraInputs{GroundHeight: func(Pose) (int16, error) { return 0, nil }, Player: CameraAnchor{Camera: Camera{Pose: Pose{H0: 0x40000, H1: 0x40000, Height: 100}}}}
	c := NewCameraController(in)
	c.Camera.Height = 100
	c.Camera.Angles = [3]uint16{12, 0, 37}
	in.ProjectileView = &ProjectileCameraInputs{Eligible: true, LastActive: true, Lifetime: 40, Packed: 9, Current: CameraAnchor{Camera: Camera{Pose: Pose{H0: 0x40000, H1: 0x40000, Height: 101}, Angles: [3]uint16{0, 0x800, 0}}}, Previous: &CameraAnchor{Camera: Camera{Pose: Pose{H0: 0x40002, H1: 0x40004, Height: 800}}}}
	ok, err := c.Select(9, false, in, a)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	// Pairmean H0=40001,H1=40002; yaw0 givesoffset0,-128.
	// Height is current101+old100, trunc/2+64=164, not pairmean height.
	if c.Mode != 6 || c.Camera.H0 != 0x40001 || c.Camera.H1 != 0x3ff82 || c.Camera.Height != 164 || c.Camera.Angles != [3]uint16{0xf060, 0x100, 37} {
		t.Fatalf("camera%+v", c)
	}
	frozen := c.Camera
	in.ProjectileView.LastActive = false
	in.ProjectileView.Lifetime = 0
	in.ProjectileView.Packed = 1
	if err = c.Tick(in, a); err != nil {
		t.Fatal(err)
	}
	if c.Camera != frozen || c.Mode != 6 {
		t.Fatal("tail did not retain camera")
	}
	in.ProjectileView.Packed = 0
	if err = c.Tick(in, a); err != nil {
		t.Fatal(err)
	}
	if c.Mode != 0 || c.UseArrangementViewport || c.Camera.Pose != in.Player.Camera.Pose {
		t.Fatalf("return%+v", c)
	}
	if c.Camera.Angles != frozen.Angles {
		t.Fatal("position return must not restore angles")
	}
}

func TestProjectileReturnMastGateIsNotViewportSelection(t *testing.T) {
	a := sourceCameraTables(t)
	for _, mode := range []uint8{0, 3, 7} {
		for _, viewport := range []bool{false, true} {
			in := CameraInputs{Player: CameraAnchor{Camera: Camera{Pose: Pose{H0: 0x40000, H1: 0x40000, Height: 100}}}, GroundHeight: func(Pose) (int16, error) { return 0, nil }, ProjectileView: &ProjectileCameraInputs{Eligible: true, Lifetime: 40, Current: CameraAnchor{Camera: Camera{Pose: Pose{H0: 0x40000, H1: 0x41000, Height: 100}}}}}
			c := NewCameraController(in)
			c.Mode, c.UseArrangementViewport = mode, viewport
			if _, err := c.Select(9, false, in, a); err != nil {
				t.Fatal(err)
			}
			in.ProjectileView.Lifetime = 0
			if err := c.Tick(in, a); err != nil {
				t.Fatal(err)
			}
			want := uint8(0)
			if mode == 7 {
				want = 7
			}
			if c.Mode != want || c.UseArrangementViewport {
				t.Fatal("source313a gate/F0 full viewport confused", mode, viewport, c.Mode, c.UseArrangementViewport)
			}
		}
	}
	// Independent raw producers: F1 writes313a=1; F9 computes return from
	// CMP313a,1/SBB/AND7 and never restores F0 in1a68..1aca.
	b, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	for ip, want := range map[int][]byte{0x8b6: {0xc7, 6, 0x3a, 0x31, 1, 0}, 0xafe: {0xc7, 6, 0xf0, 0, 0xc7, 0}, 0xb49: {0x83, 0x3e, 0x3a, 0x31, 1, 0x1b, 0xc0, 0x25, 7, 0}} {
		if !bytes.Equal(b[0xd8b0+ip:][:len(want)], want) {
			t.Fatalf("source%04x operand mismatch", ip)
		}
	}
}
