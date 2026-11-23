package combat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"gunship2000-assets/internal/testfixture"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	gamestate "gunship2000-assets/game/state"
)

type retainedPose struct {
	Code   byte   `json:"code"`
	Cursor uint16 `json:"cursor"`
	Motion int16  `json:"motion"`
	Phase  byte   `json:"phase"`
	Queued int16  `json:"queued"`
	Row    int    `json:"row"`
	X      uint32 `json:"x"`
	Yaw    uint16 `json:"yaw"`
	Z      uint32 `json:"z"`
}

type retainedTick struct {
	Draws int            `json:"draws"`
	Poses []retainedPose `json:"poses"`
	RNG   uint16         `json:"rng"`
	Tick  int            `json:"tick"`
}

// Retained Step50 source-derived ordinary MT-LB and Train states; no private
// campaign state. The JSON and expected trajectory are immutable witnesses.
func retainedMovement(t *testing.T, name string) (*State, int, []int, []retainedTick) {
	t.Helper()
	root := testfixture.Path("ground-movement")
	b, err := os.ReadFile(filepath.Join(root, name+"-initial.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"ordinary": "bee4a4a31a64a4b043cac97027ce1faf17b86c51ad34db23281f4496c55be09d", "train": "293d63ca3d67b1037c76f40bc1ed23cdd6f9c95385c81cac909fd6487a792960"}[name]
	hash := sha256.Sum256(b)
	if hex.EncodeToString(hash[:]) != want {
		t.Fatal("retained initial fixture identity", name)
	}
	var a struct {
		Initialized gamestate.InitializedMission
		Mission     gamestate.MissionData
		Header      int `json:"header_row"`
	}
	if err = json.Unmarshal(b, &a); err != nil {
		t.Fatal(err)
	}
	if name == "train" {
		a.Header = 71
	}
	// Only route/header visits run here. Reuse exact source trigonometric tables;
	// no terrain, actor sensing, AI scheduler or live flight is inferred.
	s, err := New(&a.Initialized, sourceTables(t), &a.Mission)
	if err != nil {
		t.Fatal(err)
	}
	s.SetMissionActorsEnabled(true)
	s.SetTrainEnabled(true)
	s.SetEnemyBehaviorEnabled(true)
	var defs [16][8]byte
	for i := range defs {
		defs[i][0] = byte(i)
	}
	if err = s.ConfigureStationaryEnemyAI(defs, false); err != nil {
		t.Fatal(err)
	}
	children := []int{}
	for i, e := range a.Initialized.Entities {
		if e.LeaderRow == a.Header {
			children = append(children, i)
		}
	}
	b, err = os.ReadFile(filepath.Join(root, name+"-isolated.json"))
	if err != nil {
		t.Fatal(err)
	}
	var expected []retainedTick
	if err = json.Unmarshal(b, &expected); err != nil {
		t.Fatal(err)
	}
	return s, a.Header, children, expected
}

func movementTick(s *State, h int, children []int, tick, draws int, rng uint16) retainedTick {
	r := retainedTick{Tick: tick, Draws: draws, RNG: rng}
	for _, row := range append([]int{h}, children...) {
		e := &s.init.Entities[row]
		r.Poses = append(r.Poses, retainedPose{e.DynamicRaw[2], s.route.Cursor[row], e.RawMotion, e.Participation, s.route.Queued[row], row, e.Pose.H0, e.Pose.Angles[1], e.Pose.H1})
	}
	return r
}

func TestGroundNoContactKeepsMTLBAndTrainTrajectory(t *testing.T) {
	for _, name := range []string{"ordinary", "train"} {
		t.Run(name, func(t *testing.T) {
			s, h, children, expected := retainedMovement(t, name)
			rng, draws := uint16(0xace1), 0
			in := Input{DF: 6, Random: func() uint16 {
				draws++
				if rng&1 != 0 {
					rng = rng>>1 ^ 0xb400
				} else {
					rng >>= 1
				}
				return rng
			}}
			arrival := -1
			for tick, want := range expected {
				if tick > 0 {
					if err := s.advanceRouteHeader(h, in); err != nil {
						t.Fatalf("tick%d header: %v", tick, err)
					}
					for _, row := range children {
						var err error
						if name == "train" {
							err = s.advanceTrainRoute(row, in)
						} else {
							err = s.advanceGroundRoute(row, in)
						}
						if err != nil {
							t.Fatalf("tick%d actor%d: %v", tick, row, err)
						}
					}
				}
				got := movementTick(s, h, children, tick, draws, rng)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("first divergence %s tick%d\ngot %+v\nwant%+v", name, tick, got, want)
				}
				if arrival < 0 && s.route.Cursor[h] == 2 {
					arrival = tick
				}
			}
			if name == "ordinary" && (h != 39 || len(children) != 3 || children[0] != 40 || arrival != 819) {
				t.Fatal("EURO7 seed8 PATH39/form83 MT-LB waypoint", h, children, arrival)
			}
		})
	}
}

func TestGroundOddSelectionDetachesBeforeOffsetMovementAndJitter(t *testing.T) {
	for _, whyNoShot := range []string{"unarmed", "cooldown", "range", "busy"} {
		t.Run(whyNoShot, func(t *testing.T) {
			s, h, _, _ := retainedMovement(t, "ordinary")
			row := 41 // actual odd formation member0x21; row40 is even0x20.
			e := &s.init.Entities[row]
			e.Object.Raw[11] = 0
			if whyNoShot != "unarmed" {
				e.Object.Raw[11] = 0x22
				s.enemyDefinitions[2] = [8]byte{2, 100, 7, 0, 1, 0, 0xff, 0x7f}
				s.init.Entities[0].Pose = e.Pose
				s.init.Entities[0].Pose.H1 += 1024
				e.Pose.Angles[1] = 0
			}
			if whyNoShot == "cooldown" {
				e.DynamicRaw[3] = 7
			}
			if whyNoShot == "range" {
				s.enemyDefinitions[2][6], s.enemyDefinitions[2][7] = 0, 0
			}
			if whyNoShot == "busy" {
				s.Slots[21].Lifetime = 9
			}
			// The original doesn't access formation offsets after the odd AX!=0
			// successor. Prove the dispatcher doesn't require an unread operand.
			s.init.Offsets = nil
			before := e.Pose
			draws := 0
			in := Input{DF: 6, E0: true, Random: func() uint16 { draws++; return 0 }}
			in.Contacts[row].Flags = 2
			s.init.Entities[0].ControlRaw[1] = 1
			s.init.Entities[h].DynamicRaw[2] |= 0x80
			if err := s.advanceGroundRoute(row, in); err != nil {
				t.Fatal(err)
			}
			if e.DynamicRaw[2] != 0 || e.Pose.H0 != before.H0 || e.Pose.H1 != before.H1 || s.init.Entities[h].DynamicRaw[2]&0x80 != 0 || draws != 1 || len(events(s, "launch")) != 0 {
				t.Fatal("BE3D selected successor before BE65/BF5F", whyNoShot, e, draws, s.Events)
			}
			// Next ordinary independent visit makes a fresh decision and cannot
			// reattach or resume jitter movement without a new source route.
			in.Contacts[row].Flags = 0
			if err := s.advanceGroundRoute(row, in); err != nil {
				t.Fatal(err)
			}
			if e.DynamicRaw[2] != 0 || e.Pose.H0 != before.H0 || e.Pose.H1 != before.H1 || draws != 1 {
				t.Fatal("detached next C056 visit", e, draws)
			}
		})
	}
}

func TestGroundEvenSelectionKeepsFormationAndTwoJitterDraws(t *testing.T) {
	s, _, _, _ := retainedMovement(t, "ordinary")
	e := &s.init.Entities[40]
	e.Object.Raw[11] = 0
	in := Input{DF: 6}
	in.Contacts[40].Flags = 2
	s.init.Entities[0].ControlRaw[1] = 1
	draws := sequence(t, &in, 0, 4, 4)
	before := e.Pose
	if err := s.advanceGroundRoute(40, in); err != nil {
		t.Fatal(err)
	}
	if *draws != 3 || e.DynamicRaw[2] != 0x20 || e.Pose.H0 != before.H0 || e.Pose.H1 != before.H1 || len(events(s, "launch")) != 0 {
		t.Fatal("selected even retains BE65 and both BF5F/BF90 draws", *draws, e)
	}
}

func TestGroundFirstSelectedVisitDivergesFromMovingMTLBControl(t *testing.T) {
	control, h, children, _ := retainedMovement(t, "ordinary")
	engaged, _, _, _ := retainedMovement(t, "ordinary")
	states := []*State{control, engaged}
	rng := [2]uint16{0xace1, 0xace1}
	draws := [2]int{}
	inputs := [2]Input{}
	for i := range inputs {
		n := i
		inputs[i] = Input{DF: 6, Random: func() uint16 {
			draws[n]++
			if rng[n]&1 != 0 {
				rng[n] = rng[n]>>1 ^ 0xb400
			} else {
				rng[n] >>= 1
			}
			return rng[n]
		}}
	}
	step := func(which int) {
		s, in := states[which], inputs[which]
		if err := s.advanceRouteHeader(h, in); err != nil {
			t.Fatal(err)
		}
		for _, row := range children {
			if err := s.advanceGroundRoute(row, in); err != nil {
				t.Fatal(err)
			}
		}
	}
	for tick := 1; tick <= 200; tick++ {
		step(0)
		step(1)
	}
	if !reflect.DeepEqual(control.init.Entities, engaged.init.Entities) || rng[0] != rng[1] || draws[0] != draws[1] {
		t.Fatal("uncontacted preparation diverged")
	}
	if engaged.route.Queued[41] != -1 {
		t.Fatal("encounter fixture must enter ordinary BE3D, not queued-leg bypass")
	}
	before := engaged.init.Entities[41].Pose
	inputs[1].Contacts[41].Flags = 2
	engaged.init.Entities[0].ControlRaw[1] = 1
	engaged.init.Entities[41].Object.Raw[11] = 0 // selected but unarmed: detach still required.
	oldDraws := draws
	step(0)
	step(1)
	a, b := control.init.Entities[41], engaged.init.Entities[41]
	if b.DynamicRaw[2] != 0 || b.Pose != before || a.DynamicRaw[2] != 0x21 || a.Pose == before {
		t.Fatal("first divergent visit201 must be BE5C, no movement", a, b, before)
	}
	if control.init.Entities[h].Pose != engaged.init.Entities[h].Pose || control.route.Cursor[h] != engaged.route.Cursor[h] {
		t.Fatal("detach cannot move or retask the PATH header")
	}
	if draws[0]-oldDraws[0] != 6 || draws[1]-oldDraws[1] != 5 {
		t.Fatal("one selection draw replaces exactly two odd-member jitter draws", oldDraws, draws)
	}
	inputs[1].Contacts[41].Flags = 0
	for tick := 202; tick <= 211; tick++ {
		step(0)
		step(1)
	}
	if engaged.init.Entities[41].Pose != before || engaged.init.Entities[41].DynamicRaw[2] != 0 || control.init.Entities[41].Pose == before {
		t.Fatal("later independent C056 visits retain detached position and normal route continues")
	}
}
