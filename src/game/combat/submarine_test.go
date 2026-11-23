package combat

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"reflect"
	"testing"

	"gunship2000-assets/game/avionics"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/game/world"
)

func submarineFixture(t *testing.T, code byte) (*State, Input, *int) {
	t.Helper()
	s, in := groundFixture(t)
	s.enemyBehaviorEnabled = true
	g := &world.Geometry{ID: "sub", Vertices: []world.Vertex{{0, 0, 0}, {20, 160, 0}, {0, 1, 0}, {0, 256, 0}}, Planes: make([]world.Plane, 32), Children: make([][2]int, 32), Root: 13}
	for i := range g.Children {
		g.Children[i] = [2]int{255, 255}
	}
	g.Children[13] = [2]int{24, 0}
	s.assets.Models = []string{"sub", "replacement"}
	s.assets.Geometries["sub"], s.assets.Geometries["replacement"] = g, &world.Geometry{ID: "replacement"}
	s.init.Offsets[1] = [][2]int16{{100, 0}, {200, 0}, {300, 0}}
	for _, row := range []int{10, 11} {
		e := &s.init.Entities[row]
		e.Kind, e.Participation, e.ModelID, e.ReplacementModelID = 0x0a, 0, 0, 1
		e.Pose = gamestate.InitializedPose{H0: 0x40000 + 200, H1: 0x40000, Height: 123}
		e.DynamicRaw[2] = code
		e.Object.Raw[10], e.Object.Raw[11] = 0x0a, 0
		e.LeaderRow = 9
	}
	if err := s.init.InitializeSubmarineModels(s.assets); err != nil {
		t.Fatal(err)
	}
	var definitions [16][8]byte
	for i := range definitions {
		definitions[i][0] = byte(i)
	}
	if err := s.ConfigureStationaryEnemyAI(definitions, false); err != nil {
		t.Fatal(err)
	}
	in.DF, in.E0 = 1, true
	draws := new(int)
	in.Random = func() uint16 { *draws++; return 4 }
	return s, in, draws
}

func TestSubmarineOriginalOperands(t *testing.T) {
	b, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	if got := sha256.Sum256(b); hex.EncodeToString(got[:]) != "1a1db96070adb8c23e1eb499b449cc147f9fb0fd7b47a65e88c4c5603a40e209" {
		t.Fatal("unsupported original fixture")
	}
	for ip, raw := range map[int]string{
		0xc15d: "b83200", 0xc282: "b80000", 0xc2b2: "a0df002ae4d1e050",
		0xc2e1: "26ff848825", 0xc2f1: "b041", 0xc307: "03d9d1e32688874306",
		0xc31b: "2683bf882580", 0xc339: "c6471cff", 0xc348: "2683bf88259c", 0xc366: "c6471b1f",
		0xc3ba: "2683780400", 0xc3c4: "26ff4804", 0xc3d0: "2683bf882564", 0xc401: "26ff4804",
		0xc41e: "26f687420602", 0xc431: "26c787882560ff", 0xc5e8: "9a102cbb10",
		0xc5fc: "26f684842501", 0xc607: "26c684842500", 0xc70a: "9a38207f03", 0xc735: "9a38207f03",
		0xc817: "268b87882526ff8f8825", 0xc821: "3d0000", 0xc862: "26c68782251c", 0xc868: "26c78786259cff",
	} {
		want, err := hex.DecodeString(raw)
		if err != nil || !bytes.Equal(b[0xd8b0+ip:][:len(want)], want) {
			t.Fatalf("source0975:%04x operand mismatch", ip)
		}
	}
}

func TestSubmarineColdIndependentGroupedAndQueuedBoundaries(t *testing.T) {
	for _, code := range []byte{0, 0x11} {
		s, in, draws := submarineFixture(t, code)
		e, leader := &s.init.Entities[10], &s.init.Entities[9]
		leader.Pose.Height = 42
		if err := s.advanceSubmarine(10, in); err != nil {
			t.Fatal(err)
		}
		wantMotion, wantHeight := int16(0), uint16(123)
		if code != 0 {
			wantMotion, wantHeight = 50, 42
		}
		if e.Participation != 1 || e.RawMotion != wantMotion || e.Pose.Height != wantHeight || s.route.Cursor[10] != 0 || s.route.Queued[10] != -1 || *draws != 0 {
			t.Fatal("cold visit", code, e, *draws)
		}
		if code == 0 {
			old := e.Pose
			if err := s.advanceSubmarine(10, in); err != nil || e.Pose != old || *draws != 0 || len(events(s, "launch")) != 0 {
				t.Fatal("independent submarine must not borrow stationary gun/air motion", e, err)
			}
		} else {
			if leader.RawMotion != 50 {
				t.Fatal("group P0 must set actual header speed", leader.RawMotion)
			}
			leader.DynamicRaw[2] = 0x81
			if err := s.advanceSubmarine(10, in); err != nil || *draws != 2 || leader.DynamicRaw[2] != 1 || len(events(s, "launch")) != 0 {
				t.Fatal("C5E8 no-target then two source formation draws", e, *draws, err)
			}
		}
	}
	for _, dx := range []int32{98, 100} {
		s, in, draws := submarineFixture(t, 0x11)
		e := &s.init.Entities[10]
		if err := s.advanceSubmarine(10, in); err != nil {
			t.Fatal(err)
		}
		e.Pose.H0, e.Pose.H1 = uint32(262144-dx), 262144
		s.route.Queued[10] = 0
		if err := s.advanceSubmarine(10, in); err != nil {
			t.Fatal(err)
		}
		if (s.route.Queued[10] == -1) != (dx == 98) || *draws != 0 {
			t.Fatal("strict motion>unsigned half-distance", dx, e, s.route.Queued[10], *draws)
		}
	}
}

func TestSubmarineContactDivePhasesAndDisablePolicy(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		s, in, draws := submarineFixture(t, 0)
		s.enemyAutonomyDisabled = disabled
		e := &s.init.Entities[10]
		if err := s.advanceSubmarine(10, in); err != nil {
			t.Fatal(err)
		}
		model, other := s.init.SubmarineModels[10], s.init.SubmarineModels[11]
		oldOther := append([]world.Vertex(nil), other.Vertices...)
		contacts := [80]avionics.Contact{}
		contacts[10].Flags = 2
		in.ContactState = &contacts
		old := e.Pose
		if err := s.advanceSubmarine(10, in); err != nil || e.RawMotion != -160 || e.Pose != old || *draws != 0 {
			t.Fatal("contact visit starts dive without movement/draw", disabled, e, err)
		}
		in.E0, in.DF = false, 10
		if err := s.advanceSubmarine(10, in); err != nil || e.RawMotion != -160 || e.Pose.H1 != old.H1+20 || model.Vertices[1][1] != 160 {
			t.Fatal("non-E0 still moves2*DF but not model/countdown", e, err)
		}
		in.E0 = true
		for _, c := range []struct {
			before, after int16
			y             int16
			children      [2]int
		}{{-160, -159, 158, [2]int{24, 0}}, {-129, -128, 158, [2]int{24, 255}}, {-101, -100, 158, [2]int{31, 255}}, {-2, -1, 156, [2]int{31, 255}}, {-1, 0, 156, [2]int{31, 255}}} {
			e.RawMotion = c.before
			if err := s.advanceSubmarine(10, in); err != nil || e.RawMotion != c.after || model.Vertices[1][1] != c.y || model.Children[13] != c.children || *draws != 0 {
				t.Fatal("negative phase boundary", disabled, c, e, model, *draws, err)
			}
		}
		if e.Kind != 0x41 || e.Visible || contacts[10].Kind != 0x41 || !reflect.DeepEqual(other.Vertices, oldOther) || s.assets.Geometries["sub"].Vertices[1][1] != 160 {
			t.Fatal("deactivate/current contact/private isolation", e, contacts[10], model, other)
		}
	}
}

func TestSubmarineDeathKeepsOldCounterAndClearsOnlyItsPrivateModel(t *testing.T) {
	s, in, _ := submarineFixture(t, 0x11)
	e := &s.init.Entities[10]
	e.Participation, e.RawMotion = 2, 1
	if err := s.advanceSubmarine(10, in); err != nil || e.Participation != 3 || e.RawMotion != 1 {
		t.Fatal("P2 only advances P3", e, err)
	}
	if err := s.advanceSubmarine(10, in); err != nil || e.RawMotion != 0 || e.Kind != 0x0a {
		t.Fatal("old-counter equality is not post-decrement equality", e, err)
	}
	if err := s.advanceSubmarine(10, in); err != nil || e.Kind != 0x1c || e.Participation != 0 || e.ModelID != 1 || s.init.SubmarineModels[10] != nil || s.init.SubmarineModels[11] == nil {
		t.Fatal("replacement/cleanup", e, err)
	}
	if s.init.Entities[9].Participation != 1 {
		t.Fatal("old-test death must not invent header deactivation")
	}
}

func TestSubmarineSuppliedC5E8BoundarySelectedWithoutShot(t *testing.T) {
	// This supplied mid-routine boundary deliberately skips C41E's earlier
	// contact guard. In an ordinary full visit Flags02 starts submergence first.
	// It proves C5E8's true result means selected, not a released submarine shot.
	for _, code := range []byte{0x11, 0x12} {
		s, in, draws := submarineFixture(t, code)
		if err := s.advanceSubmarine(10, in); err != nil {
			t.Fatal(err)
		}
		in.Contacts[10].Flags = 2
		s.init.Entities[s.init.PlayerRow].ControlRaw[1] = 1
		e := &s.init.Entities[10]
		before := e.Pose
		if err := s.advanceSubmarineFormation(10, in); err != nil || len(events(s, "launch")) != 0 {
			t.Fatal("unarmed selected boundary", e, s.Events, err)
		}
		if code&1 != 0 {
			if e.DynamicRaw[2] != 0 || e.Pose != before || *draws != 1 {
				t.Fatal("C607 odd detach skips both motion and jitter draws", e, *draws)
			}
			in.Contacts[10].Flags = 0
			if err := s.advanceSubmarine(10, in); err != nil || e.Pose != before || *draws != 1 {
				t.Fatal("detached next ordinary visit must not invent pursuit", e, *draws, err)
			}
			in.Contacts[10].Flags = 2
			if err := s.advanceSubmarine(10, in); err != nil || e.RawMotion != -160 || *draws != 1 {
				t.Fatal("later real contact still owns its source submergence", e, *draws, err)
			}
		} else if e.DynamicRaw[2] != code || *draws != 3 {
			t.Fatal("even selected member keeps group and both jitter draws", e, *draws)
		}
	}
}
