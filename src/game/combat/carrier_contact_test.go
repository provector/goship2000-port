package combat

import (
	"os"
	"path/filepath"
	"testing"

	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/game/world"
)

func TestFormationClearsCarrierIslandWithRotorAndTail(t *testing.T) {
	dir := os.Getenv("GS2000_TEST_MISSIONS")
	if dir == "" {
		t.Skip("set GS2000_TEST_MISSIONS for the actual carrier and Apache geometry")
	}
	b, err := world.Load(filepath.Join(dir, "world"))
	if err != nil {
		t.Fatal(err)
	}
	ship := gamestate.InitializedEntity{ModelID: 64, Visible: true, Pose: gamestate.InitializedPose{H0: 28302, H1: 438747, Height: 65535}}
	m := &gamestate.InitializedMission{Entities: []gamestate.InitializedEntity{ship, {ModelID: 9, FlightSlot: 1}}, Switches: 0x1000}
	s := &State{assets: b, init: m}
	s.EnableCarrierClearance()
	// Reached formation poses from the historical Arctic Fleet recording.
	// From the deck centreline the helicopter's origin clears the island at
	// x=200 in both cases. Rotors and tail extend past it at these headings.
	for _, tc := range []struct{ x, z, y, heading, minimumCorrection int }{{93, -640, 417, 20116, 65}, {54, -388, 387, 9161, 47}, {93, -640, 1000, 20116, 0}, {0, -400, 362, 0, 0}} {
		p := gamestate.InitializedPose{H0: ship.Pose.H0 + uint32(tc.x), H1: ship.Pose.H1 + uint32(tc.z), Height: uint16(tc.y), Angles: [3]uint16{0, uint16(tc.heading), 0}}
		e := gamestate.InitializedEntity{FlightSlot: 1, ModelID: 9, Visible: true, Kind: 0x21, Participation: 1, Pose: p}
		s.clearCarrierContact(&e, p)
		shift := int(p.H0) - int(e.Pose.H0)
		if shift < tc.minimumCorrection || tc.minimumCorrection == 0 && shift != 0 || e.Pose.H1 != p.H1 || e.Pose.Height != p.Height || e.Pose.Angles != p.Angles {
			t.Fatalf("pose %+v moved %+v, lateral clearance %d", p, e.Pose, shift)
		}
		stable := e.Pose
		s.clearCarrierContact(&e, stable)
		if e.Pose != stable {
			t.Fatal("resting contact drift", stable, e.Pose)
		}
		t.Logf("local (%d,%d) at height %d heading %04x moved %d units toward open deck", tc.x, tc.z, tc.y, tc.heading, shift)
	}
}
