package render3d

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"gunship2000-assets/internal/testfixture"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gunship2000-assets/game/world"
)

func sourceCameraTables(t *testing.T) *world.Bundle {
	t.Helper()
	p := filepath.Clean(testfixture.Path("installation/GS2.GS2"))
	b, e := os.ReadFile(p)
	if os.IsNotExist(e) {
		t.Skip("optional original camera byte witness unavailable")
	}
	if e != nil {
		t.Fatal(e)
	}
	if fmt.Sprintf("%x", sha256.Sum256(b)) != "1a1db96070adb8c23e1eb499b449cc147f9fb0fd7b47a65e88c4c5603a40e209" {
		t.Fatal("unsupported GS2 source")
	}
	a := &world.Bundle{Sine: make([]int16, 1025), AngleBases: make([]uint16, 8), Atan: make([]uint16, 4096)}
	for i := range a.Sine {
		a.Sine[i] = int16(binary.LittleEndian.Uint16(b[0x4bc50+2*i:]))
	}
	for i := range a.AngleBases {
		a.AngleBases[i] = binary.LittleEndian.Uint16(b[0x4c452+2*i:])
	}
	for i := range a.Atan {
		a.Atan[i] = binary.LittleEndian.Uint16(b[0x4c462+2*i:])
	}
	return a
}

func TestCameraOriginalSourceDispatchAndTables(t *testing.T) {
	a := sourceCameraTables(t)
	b, e := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if e != nil {
		t.Fatal(e)
	}
	check := func(base, ip int, want ...byte) {
		t.Helper()
		if !reflect.DeepEqual(b[base+ip:base+ip+len(want)], want) {
			t.Fatalf("source operand mismatch %x+%x", base, ip)
		}
	}
	// Named section1, not section0 at the same original CS. These bytes establish
	// F2's serialized-selector gate and exact visible-camera word operations.
	check(0xd8b0, 0x912, 0x24, 7)
	check(0xd8b0, 0x916, 0xfe, 0xc8, 0x74, 0x0d)
	check(0xd8b0, 0x91a, 0x80, 0xf9, 3, 0x74, 8, 0x80, 0xf9, 5)
	check(0xd8b0, 0x15ee, 0xf7, 0xd8, 0xd1, 0xf8)
	check(0xd8b0, 0x1621, 0xa1, 0xcc, 0, 0xeb, 0xca)
	check(0xd8b0, 0x704, 0x8e, 6, 0xfe, 0x32, 0x26, 0x8a, 0x1e, 0xcf, 0x0a, 0xfe, 0xc3, 0x83, 0xe3, 3)
	check(0xd8b0, 0xbdd, 0x8e, 6, 2, 0x33, 0x26, 0xc7, 6, 0xd1, 0x0a, 4, 0)
	check(0xd8b0, 0x1238, 0x83, 0x7e, 0xec, 0x50, 0x75, 5, 0xc7, 0x46, 0xec, 1, 0)
	check(0xd8b0, 0x1270, 0x83, 0x7e, 0xec, 1, 0x77, 5, 0xc7, 0x46, 0xec, 0x4f, 0)
	check(0xd8b0, 0x124b, 0x26, 0x80, 0xbf, 0x83, 0x25, 1)
	check(0xd8b0, 0x1253, 0x26, 0xf6, 0x87, 0x82, 0x25, 0x60)
	check(0xd8b0, 0xb94, 0x26, 0xf6, 6, 0x76, 0x0b, 0x20)
	check(0x61bc0, 0x32ea, 0x2d, 0x27)
	check(0x3df0, 0x1ef5, 0x99, 0x3b, 0xda, 0x74, 8)
	check(0x3df0, 0x1f02, 0x0c, 1)
	wantDispatch := []uint16{0x978, 0x99e, 0x9c8, 0x9f2, 0xa4e, 0xa8c, 0xafe, 0x775, 0x775, 0x775, 0x775, 0x1296, 0x1226}
	for i, want := range wantDispatch {
		got := binary.LittleEndian.Uint16(b[0xd8b0+0x13d0+2*i:])
		if got != want {
			t.Fatalf("F-key source slot%d=%x", i, got)
		}
	}
	for _, q := range []struct {
		at, n int
		hash  string
	}{{0x4c452, 16, "d57d75d4bd8f68eb1f48d2cfa9a467f5b363573077a2733cc47dd1dafb85bb78"}, {0x4c462, 8192, "d0db330f9d3ad1f470017c285111b4dcc1529c38e3b8c041ea4284f9ad58dafd"}} {
		if fmt.Sprintf("%x", sha256.Sum256(b[q.at:q.at+q.n])) != q.hash {
			t.Fatal("angle table source identity")
		}
	}
	if a.Atan[256] != 651 || a.Atan[4095] != 8190 {
		t.Fatal("independent raw angle witnesses")
	}
}

func TestCameraDifferenceAndAngleHandCases(t *testing.T) {
	for _, q := range []struct {
		a, b uint32
		want int16
	}{{0, 0, 1}, {100, 0, 101}, {0, 100, -99}, {65536, 0, 32753}, {0, 65536, -32753}, {0, 0xffffffff, 1}, {0x80000000, 0, -32753}} {
		if got := SourceCameraDifference(q.a, q.b); got != q.want {
			t.Fatalf("difference(%x,%x)=%d want%d", q.a, q.b, got, q.want)
		}
	}
	a := sourceCameraTables(t)
	// Original sign words plus independently indexed source table values. In
	// particular equal/zero vectors use index4095, not ideal atan2's zero.
	for _, q := range []struct {
		y, x int16
		want uint16
	}{{0, 1, 0}, {1, 0, 0x4000}, {0, -1, 0x8000}, {-1, 0, 0xc000}, {1, 1, 0x1ffe}, {0, 0, 0x1ffe}, {-1, -1, 0x9ffe}, {16, 256, 651}, {-16, 256, 0xfd75}, {256, 16, 0x3d75}} {
		if got := SourceCameraAngle(q.y, q.x, a); got != q.want {
			t.Fatalf("angle(%d,%d)=%04x want%04x", q.y, q.x, got, q.want)
		}
	}
}

func cameraInputFixture() CameraInputs {
	return CameraInputs{Player: CameraAnchor{ID: "player", Camera: Camera{Pose: Pose{H0: 10000, H1: 10000, Height: 100}, Angles: [3]uint16{0x2000, 0x2000, 0x1000}}}, AircraftSelector: 0, GroundHeight: func(Pose) (int16, error) { return 0, nil }}
}

func TestCameraInternalSidesMastAndSourceArrangements(t *testing.T) {
	in := cameraInputFixture()
	before := in
	c := NewCameraController(in)
	if c.Mode != 0 || c.Camera.Height != 132 {
		t.Fatal("initial source eye offset")
	}
	for _, q := range []struct {
		key    int
		angles [3]uint16
	}{{1, in.Player.Camera.Angles}, {3, [3]uint16{0x1000, 0x6000, 0xf000}}, {4, [3]uint16{0xf000, 0xe000, 0x1000}}} {
		changed, e := c.Select(q.key, false, in, nil)
		if e != nil || !changed {
			t.Fatal(e)
		}
		if c.Camera.Height != 132 || c.Camera.Angles != q.angles {
			t.Fatalf("key%d %+v", q.key, c.Camera)
		}
	}
	// NEG8000 wraps before SAR; both source side branches yieldc000 here.
	in.Player.Camera.Angles[0] = 0x8000
	for _, key := range []int{3, 4} {
		_, e := c.Select(key, false, in, nil)
		if e != nil || c.Camera.Angles[2] != 0xc000 {
			t.Fatal("word NEG/SAR boundary", e)
		}
	}
	if _, e := c.Select(2, false, in, nil); e != nil {
		t.Fatal(e)
	}
	if c.Mode == 7 {
		t.Fatal("Apache non-mast selector admitted")
	}
	for _, selector := range []uint8{1, 3, 5} {
		in.AircraftSelector = selector
		mast := in.Player.Camera
		mast.Height += 0x80 // Later source156a..1579 writer, independently checked below.
		if changed, e := c.Select(2, false, in, nil); e != nil || !changed || c.Mode != 7 || c.Camera != mast {
			t.Fatal("source mast selector/snapshot", selector, e)
		}
	}
	if !c.MastControls(4, -3) || c.Camera.Angles[0] != 0xffa0 || c.Camera.Angles[1] != 0x1fc0 || c.Camera.Angles[2] != 0 {
		t.Fatal("source raw mast controller")
	}
	for i := 0; i < 4; i++ {
		c.CycleArrangement()
		if c.Arrangement != uint8((i+1)&3) {
			t.Fatal("arrangement cycle")
		}
	}
	if before.Player != in.Player { // Only this test's explicit angle change is allowed.
		in.Player.Camera.Angles[0] = 0x2000
		if before.Player != in.Player {
			t.Fatal("camera mutated player")
		}
	}
}

func TestCameraChaseRemoteAndFlightHistory(t *testing.T) {
	a := sourceCameraTables(t)
	in := cameraInputFixture()
	in.Player.Camera.Angles = [3]uint16{}
	c := NewCameraController(in)
	if changed, e := c.Select(5, false, in, a); e != nil || !changed {
		t.Fatal(e)
	}
	// First chase update: average100/132=>116+50=166; cos(0)*128=128.
	if c.Mode != 3 || c.Camera.H0 != 10000 || c.Camera.H1 != 9872 || c.Camera.Height != 166 || c.Camera.Angles != [3]uint16{0xfc18, 0, 0} {
		t.Fatalf("source first chase %+v", c)
	}
	if changed, e := c.Select(7, false, in, a); e != nil || !changed || c.Mode != 5 || c.Camera.H0 != 10000 || c.Camera.H1 != 10000 || c.Camera.Height != 484 {
		t.Fatal("remote entry", e, c)
	}
	if !c.remoteValid {
		t.Fatal("source remote offsets not retained")
	}
	in.FlightEnabled = true
	in.Flight = []CameraAnchor{{ID: "wing1", Camera: in.Player.Camera, Extent: 32}, {ID: "wing2", Camera: in.Player.Camera, Extent: 32}}
	if changed, e := c.Select(5, true, in, a); e != nil || !changed || c.TrackedID != "wing1" {
		t.Fatal("first flight chase", e)
	}
	if _, e := c.Select(5, true, in, a); e != nil || c.TrackedID != "wing2" {
		t.Fatal("repeat cycle", e)
	}
	if _, e := c.Select(1, false, in, a); e != nil {
		t.Fatal(e)
	}
	if _, e := c.Select(5, true, in, a); e != nil || c.TrackedID != "wing2" {
		t.Fatal("F1 then flight chase retains source selected slot", e)
	}
	if _, e := c.Select(5, true, in, a); e != nil || c.TrackedID != "wing1" {
		t.Fatal("source wrap", e)
	}
}

func TestCameraGuardsFailureOwnershipAndRenderIndependence(t *testing.T) {
	a := sourceCameraTables(t)
	in := cameraInputFixture()
	c := NewCameraController(in)
	before := *c
	for _, q := range []struct {
		key   int
		shift bool
	}{{2, false}, {6, false}, {8, false}, {5, true}, {8, true}, {10, false}, {1, true}} {
		changed, e := c.Select(q.key, q.shift, in, a)
		if e != nil || changed || *c != before {
			t.Fatal("unavailable/non-camera key changed view", q, e)
		}
	}
	// F9 has a source viewport write before rejecting an absent projectile; it
	// must not discard the current anchor/mode or accidentally enter missile view.
	if changed, e := c.Select(9, false, in, a); e != nil || !changed || c.Mode != before.Mode || c.Camera != before.Camera || c.UseArrangementViewport {
		t.Fatal("guarded F9 viewport", e)
	}
	c.CycleArrangement()
	c.Arrangement = before.Arrangement
	before = *c
	noGround := in
	noGround.GroundHeight = nil
	if changed, e := c.Select(5, false, noGround, a); e == nil || changed || *c != before {
		t.Fatal("failed camera partially committed")
	}
	c.Mode = 3
	before = *c
	// The presentation loop must call Tick explicitly; copying/drawing camera
	// values cannot advance history. Independent logical schedules then agree.
	left, right := *c, *c
	for frame := 0; frame < 120; frame++ {
		_ = left.Camera
	}
	if left != before {
		t.Fatal("read advanced camera")
	}
	for tick := 0; tick < 12; tick++ {
		if e := left.Tick(in, a); e != nil {
			t.Fatal(e)
		}
	}
	for frame := 0; frame < 240; frame++ {
		_ = right.Camera
		if frame%20 == 19 {
			if e := right.Tick(in, a); e != nil {
				t.Fatal(e)
			}
		}
	}
	if left != right {
		t.Fatal("camera depends on read/draw frequency")
	}
}

func TestCameraMastKeyboardSourceAndHeldLogicalUpdates(t *testing.T) {
	a := sourceCameraTables(t)
	b, e := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if e != nil {
		t.Fatal(e)
	}
	// Actual scan0x47..0x51 table from resident071d, not guessed arrow signs.
	if got := b[0x77d0+0xe4 : 0x77d0+0xef]; !reflect.DeepEqual(got, []byte{0x0a, 8, 9, 0, 2, 0, 1, 0, 6, 4, 5}) {
		t.Fatal("original keyboard scan mask")
	}
	if got := b[0x77d0+0x61e : 0x77d0+0x621]; !reflect.DeepEqual(got, []byte{0x83, 0xeb, 0x10}) {
		t.Fatal("vertical source step")
	}
	if got := b[0xd8b0+0x27c : 0xd8b0+0x285]; !reflect.DeepEqual(got, []byte{0xa1, 0x34, 0x13, 0xc1, 0xe0, 5, 0xa3, 0x76, 0x35}) {
		t.Fatal("mast pitch reader")
	}
	in := cameraInputFixture()
	in.AircraftSelector = 5
	c := NewCameraController(in)
	if _, e := c.Select(2, false, in, a); e != nil {
		t.Fatal(e)
	}
	c.MastKeyboard(true, false, true, false)
	if c.Camera.Angles != [3]uint16{0xfe00, 0x2100, 0} {
		t.Fatal("source arrow steps", c.Camera.Angles)
	}
	c.MastKeyboard(true, false, true, false)
	if c.Camera.Angles != [3]uint16{0xfe00, 0x2200, 0} {
		t.Fatal("mast axes should be consumed, not accumulate", c.Camera.Angles)
	}
	c.MastKeyboard(true, true, true, true)
	if c.Camera.Angles != [3]uint16{0, 0x2200, 0} {
		t.Fatal("opposing/reset mast inputs")
	}
	if _, e := c.Select(1, false, in, a); e != nil {
		t.Fatal(e)
	}
	before := *c
	if c.MastKeyboard(true, false, false, false) || *c != before {
		t.Fatal("mast controls escaped mast view")
	}
}

func TestCameraTargetRecipesAndSourceExtent(t *testing.T) {
	a := sourceCameraTables(t)
	in := cameraInputFixture()
	in.Player.Camera.Angles = [3]uint16{}
	target := CameraAnchor{ID: "target", Camera: Camera{Pose: Pose{H0: 10000, H1: 11024, Height: 100}}, Extent: 32}
	in.Target = &target
	c := NewCameraController(in)
	if changed, e := c.Select(6, false, in, a); e != nil || !changed {
		t.Fatal(e)
	}
	// dy0, horizontal((1>>1)+(1025>>1))=512; headingtableindex3=7,
	// SAR2=>1; source sine quantization discards that low angle before*384.
	if c.Camera != (Camera{Pose: Pose{10000, 9616, 356}, Angles: [3]uint16{0, 1, 0}}) {
		t.Fatal("source tactical hand case", c.Camera)
	}
	c = NewCameraController(in)
	if changed, e := c.Select(8, false, in, a); e != nil || !changed {
		t.Fatal(e)
	}
	// Reverse source yaw0->e000 (signedworddifference8000 SAR2).
	// Q14sin512=11585; -11585*256 SAR14=-182, positive=>181.
	if c.Camera != (Camera{Pose: Pose{9818, 10843, 164}, Angles: [3]uint16{0, 0xe000, 0}}) {
		t.Fatal("source reverse tactical hand case", c.Camera)
	}
	c = NewCameraController(in)
	if _, e := c.Select(8, true, in, a); e != nil || c.Mode != 4 || c.TrackedID != "target" {
		t.Fatal("ShiftF8 is source mode4 target chase, not mode8", e)
	}
	in.Target = nil
	before := *c
	if e := c.Tick(in, a); e == nil || *c != before {
		t.Fatal("lost target should preserve state for explicit unavailable handling")
	}
}

func TestCameraMode4PageCyclingFiltersAndFiniteFailure(t *testing.T) {
	a := sourceCameraTables(t)
	in := cameraInputFixture()
	in.Player.Camera.Angles = [3]uint16{}
	in.Player.Camera.H0 = 20000
	in.Player.Camera.H1 = 20000
	in.FlightEnabled = true
	in.Flight = []CameraAnchor{{ID: "flight1", Camera: in.Player.Camera, Extent: 32}}
	in.Candidates = []CameraCandidate{
		{CameraAnchor: in.Flight[0], SourceIndex: 1, Type: 0x21, Participation: 1},
		{CameraAnchor: CameraAnchor{ID: "group", Camera: in.Player.Camera, Extent: 32}, SourceIndex: 2, Type: 0, Participation: 0},
		{CameraAnchor: CameraAnchor{ID: "hidden", Camera: in.Player.Camera, Extent: 32}, SourceIndex: 3, Type: 0x40, Participation: 1},
		{CameraAnchor: CameraAnchor{ID: "unit", Camera: in.Player.Camera, Extent: 32}, SourceIndex: 4, Type: 0, Participation: 1},
		{CameraAnchor: CameraAnchor{ID: "last", Camera: in.Player.Camera, Extent: 32}, SourceIndex: 79, Type: 0, Participation: 1},
	}
	c := NewCameraController(in)
	if _, e := c.Select(5, true, in, a); e != nil {
		t.Fatal(e)
	}
	// First mode4update from132/100 and extent32:116+16+32=164.
	if c.Camera.Height != 164 || c.Camera.H1 != 19936 || c.Camera.Angles[1] != 160 {
		t.Fatal("tracked source extent/difference hand case", c.Camera)
	}
	snapshot := *c
	if _, e := c.Select(9, false, in, a); e != nil || c.Mode != snapshot.Mode || c.TrackedID != snapshot.TrackedID || c.Camera != snapshot.Camera {
		t.Fatal("guarded F9 discarded a tracked anchor", e)
	}
	if changed, e := c.CycleTracked(1, in, a); e != nil || !changed || c.TrackedID != "unit" {
		t.Fatal("PageUp filters", e)
	}
	if changed, e := c.CycleTracked(-1, in, a); e != nil || !changed || c.TrackedID != "last" {
		t.Fatal("PageDown source <=1 wrap", e)
	}
	in.Candidates[3].Participation = 0
	in.Candidates[4].Participation = 0
	before := *c
	if changed, e := c.CycleTracked(1, in, a); e == nil || changed || *c != before {
		t.Fatal("empty source scan must terminate without mutation")
	}
}
