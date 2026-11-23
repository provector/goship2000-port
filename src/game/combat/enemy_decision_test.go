package combat

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"

	gamestate "gunship2000-assets/game/state"
)

func TestDecisionSourceOperands(t *testing.T) {
	_ = sourceTables(t) // full supported GS2 SHA checked before byte operands.
	b, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	for ip, encoded := range map[int]string{
		0xa0d7: "c746d20008",             // fallback initial unsigned nearest0800.
		0xa0f2: "3b46d2730a",             // equal distance does not replace the earlier row.
		0xa127: "2683bf670b007dd2",       // signed OBJECT+27 must be negative.
		0xa13f: "6b5ee61826f687822510",   // actual DYN kind bit10.
		0xa221: "9a38207f03a801",         // one common selection draw, low bit.
		0xa2bd: "2680bf852500",           // selected target reaches cooldown after draw.
		0xa3cc: "263947067703",           // definition range must be strictly greater.
		0xa406: "3bc87703",               // cone equality rejects.
		0xa433: "26888f8525",             // cooldown precedes the occupied-slot read.
		0xa43c: "26ff06cb0a26833ecb0a18", // increment ring then wrap24.
		0xa450: "266b1ecb0a1a",           // selected slot only; no free-slot search.
		0xa5f9: "2680bc822501750bd1e0",   // double guided speed only for actual kind1.
		0xa69c: "b800f8262b878a25c1f802", // wrapped signed pitch correction.
		0xa7ae: "26f7bf6a2e",             // quality uses signed divisor table.
		0xbe3d: "9a102cbb1083c4023d0000", // BE3D uses selected return, not release.
		0xbe51: "26f684842501",           // odd formation code.
		0xbe5c: "26c684842500e9eb01",     // detach skips offset/move/jitter successors.
		0xbf5f: "9a38207f03250700",       // ordinary first movement jitter draw.
	} {
		want, _ := hex.DecodeString(encoded)
		if !bytes.Equal(b[0xd8b0+ip:][:len(want)], want) {
			t.Fatalf("GS2 canonical0975:%04X operand changed", ip)
		}
	}
	if !bytes.Equal(b[0x487f0+0x2e6a:][:8], []byte{50, 0, 20, 0, 10, 0, 5, 0}) {
		t.Fatal("quality steering divisors")
	}
}

// These supplied-state cases encode the source A070 branch boundaries, rather
// than requiring a lucky ordinary encounter. Natural launches are separate.
func decisionFixture(t *testing.T) (*State, Input) {
	t.Helper()
	s, in, _ := attackFixture(t)
	s.SetEnemyBehaviorEnabled(true)
	for len(s.init.Entities) < 82 {
		i := len(s.init.Entities)
		s.init.Entities = append(s.init.Entities, gamestate.InitializedEntity{RuntimeRow: i, Kind: 0x40})
	}
	in.ContactState = nil
	return s, in
}

func TestDecisionSourceSelection(t *testing.T) {
	for _, tc := range []struct {
		name                               string
		kind, flags, observedKind, control byte
		want                               int
	}{
		{"player-row-zero-is-a-target", 2, 2, 0, 1, 0},
		{"no-observation", 2, 0, 0x11, 1, -1},
		{"ordinary-inactive-observer", 2, 2, 0, 0, -1},
		{"mast-not-normal-observer", 2, 0xfa, 0, 1, -1},
		{"nonair-exact-kind11-alternative", 2, 0xfa, 0x11, 0, 31},
		{"nonair-kind12-is-not11", 2, 0xfa, 0x12, 0, -1},
		{"heli-does-not-take-nonair-alternative", 0, 0xfa, 0x11, 0, -1},
		{"plane-does-not-take-nonair-alternative", 1, 0xfa, 0x11, 0, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, in := decisionFixture(t)
			s.init.Entities[1].Kind = tc.kind
			s.init.Entities[0].ControlRaw[1] = tc.control
			in.Contacts[1].Flags, in.Contacts[1].Kind, in.Contacts[1].AttackFlags = tc.flags, tc.observedKind, 0xffff
			contacts := in.Contacts
			in.ContactState = &contacts
			s.init.Entities[1].DynamicRaw[3] = 9
			in.E0 = false
			draws := sequence(t, &in, 0)
			got, selected, err := s.enemyDecision(1, in)
			if err != nil || got != tc.want || selected != (tc.want >= 0) {
				t.Fatal(got, selected, err)
			}
			wantDraws := 0
			if selected {
				wantDraws = 1
			}
			if *draws != wantDraws || s.stationaryEnemy.NextSlot != 20 || s.init.Entities[1].DynamicRaw[3] != 9 {
				t.Fatal(*draws, s.stationaryEnemy, s.init.Entities[1].DynamicRaw[3])
			}
			if !selected && contacts[1].AttackFlags != 0 {
				t.Fatal("no-target must clear attack flags", contacts[1])
			}
		})
	}
}

func TestDecisionAirFallbackStrictDistanceSignedScoreAndStableTies(t *testing.T) {
	for _, tc := range []struct {
		name        string
		distance    uint32
		score       uint16
		kind, phase byte
		row, want   int
	}{
		{"strict-near-boundary", 0x7ff, 0xffff, 0x10, 1, 5, 5},
		{"equal-0800-rejected", 0x800, 0xffff, 0x10, 1, 5, -1},
		{"signed-most-negative-score", 0x100, 0x8000, 0x11, 1, 5, 5},
		{"zero-score-rejected", 0x100, 0, 0x10, 1, 5, -1},
		{"positive-score-rejected", 0x100, 0x7fff, 0x10, 1, 5, -1},
		{"mask20-rejected", 0x100, 0xffff, 0x30, 1, 5, -1},
		{"mask40-rejected", 0x100, 0xffff, 0x50, 1, 5, -1},
		{"without-bit10-rejected", 0x100, 0xffff, 2, 1, 5, -1},
		{"inactive-rejected", 0x100, 0xffff, 0x10, 0, 5, -1},
		{"wreck-rejected", 0x100, 0xffff, 0x10, 2, 5, -1},
		{"last-actual-row-admitted", 0x100, 0xffff, 0x10, 1, 79, 79},
		{"beyond-80-not-scanned", 0x100, 0xffff, 0x10, 1, 80, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, air := range []byte{0, 1} {
				s, in := decisionFixture(t)
				e := &s.init.Entities[1]
				e.Kind, e.DynamicRaw[3] = air, 9
				in.Contacts[1].Flags = 0
				candidate := &s.init.Entities[tc.row]
				candidate.Kind, candidate.Participation, candidate.Pose = tc.kind, tc.phase, e.Pose
				candidate.Pose.H1 += tc.distance * 16
				binary.LittleEndian.PutUint16(candidate.Object.Raw[27:], tc.score)
				in.Random = func() uint16 { return 0 }
				got, selected, err := s.enemyDecision(1, in)
				if err != nil || got != tc.want || selected != (tc.want >= 0) {
					t.Fatal(air, got, selected, err)
				}
			}
		})
	}
	// A0F2 unsigned JB retains the first equal distance in ascending actor order.
	s, in := decisionFixture(t)
	s.init.Entities[1].Kind, s.init.Entities[1].DynamicRaw[3] = 0, 9
	in.Contacts[1].Flags = 0
	for _, row := range []int{5, 6} {
		e := &s.init.Entities[row]
		e.Kind, e.Participation, e.Pose = 0x10, 1, s.init.Entities[1].Pose
		e.Pose.H1 += 0x400 * 16
		binary.LittleEndian.PutUint16(e.Object.Raw[27:], 0xffff)
	}
	in.Random = func() uint16 { return 0 }
	got, _, err := s.enemyDecision(1, in)
	if err != nil || got != 5 {
		t.Fatal(got, err)
	}
}

func TestDecisionSelectionIsNotProjectileRelease(t *testing.T) {
	for _, tc := range []struct {
		name            string
		raw, cool       byte
		e0, disabled    bool
		draws, cooldown int
	}{
		{"unarmed-still-selects", 0, 0, true, false, 1, 0},
		{"cooldown-frozen-off-E0", 0x22, 3, false, false, 1, 3},
		{"cooldown-counts-on-E0", 0x22, 3, true, false, 1, 2},
		{"disabled-decision-does-not-consume", 0x22, 3, true, true, 0, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, in := decisionFixture(t)
			s.init.Entities[1].Object.Raw[11], s.init.Entities[1].DynamicRaw[3] = tc.raw, tc.cool
			s.enemyAutonomyDisabled, in.E0 = tc.disabled, tc.e0
			draws, callbacks := 0, 0
			in.Random = func() uint16 { draws++; return 0 }
			in.HostileTarget = func(a, target int) {
				callbacks++
				if a != 1 || target != 0 {
					t.Fatal(a, target)
				}
			}
			_, selected, err := s.enemyDecision(1, in)
			if err != nil || selected == tc.disabled || draws != tc.draws || int(s.init.Entities[1].DynamicRaw[3]) != tc.cooldown || s.stationaryEnemy.NextSlot != 20 || len(events(s, "launch")) != 0 {
				t.Fatal(selected, err, draws, s.init.Entities[1], s.stationaryEnemy)
			}
			if callbacks != tc.draws {
				t.Fatal(callbacks, tc.draws)
			}
		})
	}
}

func TestDecisionRangeConeAndBusyOrdering(t *testing.T) {
	for _, tc := range []struct {
		name           string
		kind           byte
		attack         uint16
		distance       uint32
		yaw            uint16
		busy, wantShot bool
	}{
		{"range-one-below", 2, 1, 735, 0x8000, false, true},
		{"range-equality", 2, 1, 736, 0x8000, false, false},
		{"gun-cone-one-below", 3, 1, 64, 0x3ff, false, true},
		{"gun-cone-equality", 3, 1, 64, 0x400, false, false},
		{"guided-cone-one-below", 3, 0, 64, 0x7ff, false, true},
		{"guided-cone-equality", 3, 0, 64, 0x800, false, false},
		{"kind2-ignores-cone", 2, 1, 64, 0x8000, false, true},
		{"guided-busy-does-not-draw-bypass", 3, 0, 64, 0, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, in := decisionFixture(t)
			e := &s.init.Entities[1]
			e.Kind, e.Pose.Angles[1] = tc.kind, tc.yaw
			s.init.Entities[0].Pose = e.Pose
			s.init.Entities[0].Pose.H1 += tc.distance * 16
			binary.LittleEndian.PutUint16(s.enemyDefinitions[2][4:], tc.attack)
			if tc.busy {
				s.Slots[21].Lifetime = 9
			}
			draws := 0
			in.Random = func() uint16 { draws++; return 0 }
			_, selected, err := s.enemyDecision(1, in)
			if err != nil || !selected || (len(events(s, "launch")) == 1) != tc.wantShot {
				t.Fatal(selected, err, s.Events)
			}
			wantDraws := 1
			if tc.wantShot && tc.attack&1 == 0 {
				wantDraws++
			}
			if draws != wantDraws {
				t.Fatal("conditional bypass RNG", draws, wantDraws)
			}
			if tc.wantShot || tc.busy {
				if s.stationaryEnemy.NextSlot != 21 || e.DynamicRaw[3] != 3 || e.Pose.Angles[1] != 0 {
					t.Fatal("writers precede occupancy", s.stationaryEnemy, e)
				}
			} else if s.stationaryEnemy.NextSlot != 20 || e.DynamicRaw[3] != 0 {
				t.Fatal("rejection cannot allocate/cool down", s.stationaryEnemy, e)
			}
			if tc.busy && (s.Slots[21].Lifetime != 9 || s.Slots[22].Lifetime != 0) {
				t.Fatal("busy ring cannot search next free slot", s.Slots[21:23])
			}
		})
	}
}

func TestDecisionKind1SpeedNibbleAndBypassBoundary(t *testing.T) {
	for _, kind := range []byte{0, 1, 2, 3} {
		for _, gun := range []bool{false, true} {
			for _, bypass := range []uint16{186, 187} {
				s, in := decisionFixture(t)
				e := &s.init.Entities[1]
				e.Kind, e.Pose.Angles[1], e.Object.Raw[11] = kind, 0, 0x12
				s.enemyDefinitions[1] = [8]byte{1, 77, 5, 186, 0, 0, 23, 0}
				if gun {
					s.enemyDefinitions[1][4] = 1
				}
				s.stationaryEnemy.NextSlot = 23
				s.Slots[20].Pose.Angles[2] = 0xface
				draws := sequence(t, &in, 0, bypass)
				_, selected, err := s.enemyDecision(1, in)
				if err != nil || !selected || len(events(s, "launch")) != 1 {
					t.Fatal(kind, gun, bypass, selected, err, s.Events)
				}
				p := s.Slots[20]
				wantSpeed := int16(102)
				if kind == 1 {
					wantSpeed = 204
				}
				if gun {
					wantSpeed = 306
				}
				wantDraws := 2
				if gun {
					wantDraws = 1
				}
				if p.Speed != wantSpeed || p.Pose.Angles[2] != 0xface || *draws != wantDraws || e.DynamicRaw[3] != 5 || (p.Flags&0x8000 != 0) != (!gun && bypass > 186) {
					t.Fatal(kind, gun, bypass, p, *draws)
				}
			}
		}
	}
	// Odd selection takes the low nibble, not a second random weapon choice.
	s, in := decisionFixture(t)
	s.init.Entities[1].Object.Raw[11] = 0x02
	draws := sequence(t, &in, 1)
	_, _, err := s.enemyDecision(1, in)
	if err != nil || *draws != 1 || s.Slots[21].Packed != 230 {
		t.Fatal(err, *draws, s.Slots[21])
	}
}

func TestDecisionSourceQualitySteeringWords(t *testing.T) {
	// Literal outcomes from A656..A7BE (SAR signed words; all arithmetic wraps).
	for _, tc := range []struct {
		name                               string
		kind                               byte
		distance                           uint16
		yaw, targetYaw, pitch, roll, phase uint16
		speed                              int16
		wantYaw, wantRoll                  [4]uint16
		wantPitch                          uint16
		wantSpeed                          [4]int16
	}{
		{"heli-normal", 0, 0x401, 0x1000, 0x1000, 0x100, 0x2001, 0, 100, [4]uint16{3072, 3072, 2048, 2048}, [4]uint16{1024, 1024, 2048, 2048}, 0x100, [4]int16{82, 82, 82, 82}},
		{"heli-roll-threshold-equality", 0, 0x401, 0x200, 0x200, 0x100, 0x2001, 0, 100, [4]uint16{384, 384, 256, 256}, [4]uint16{128, 128, 256, 256}, 0x100, [4]int16{115, 115, 115, 115}},
		{"heli-distance-0100-equality", 0, 0x100, 0x1000, 0x1000, 0x100, 0x2001, 0, 100, [4]uint16{0x1000, 0x1000, 0x1000, 0x1000}, [4]uint16{0x2001, 0x2001, 0x2001, 0x2001}, 0x100, [4]int16{115, 115, 115, 115}},
		{"heli-2000-enters-generic", 0, 0x2000, 0x1000, 0, 0x100, 0x2001, 0, 100, [4]uint16{4015, 3892, 3687, 3277}, [4]uint16{0x2001, 0x2001, 0x2001, 0x2001}, 0x100, [4]int16{100, 100, 100, 100}},
		{"plane-quality", 1, 0x401, 0x2000, 0, 0x123, 0, 0, 400, [4]uint16{8064, 7936, 7680, 7168}, [4]uint16{4096, 4096, 4096, 4096}, 0x123, [4]int16{400, 400, 400, 400}},
		{"plane-0400-equality", 1, 0x400, 0x2000, 0, 0x123, 0x321, 0, 400, [4]uint16{0x2000, 0x2000, 0x2000, 0x2000}, [4]uint16{0x321, 0x321, 0x321, 0x321}, 0x123, [4]int16{400, 400, 400, 400}},
		{"emergency-0800-equality-damps", 0, 0x800, 0x8000, 0, 0x100, 0x2001, 0, 100, [4]uint16{0x8000, 0x8000, 0x8000, 0x8000}, [4]uint16{0x1000, 0x1000, 0x1000, 0x1000}, 0xfec0, [4]int16{115, 115, 115, 115}},
		{"emergency-near-turn", 0, 0x801, 0x8000, 0, 0x100, 0x2001, 0, 100, [4]uint16{0x7c00, 0x7c00, 0x7c00, 0x7c00}, [4]uint16{0xfc00, 0xfc00, 0xfc00, 0xfc00}, 0xfec0, [4]int16{115, 115, 115, 115}},
		{"emergency-1000-far-turn", 0, 0x1000, 0x8000, 0, 0x100, 0x2001, 0, 100, [4]uint16{0x7800, 0x7800, 0x7800, 0x7800}, [4]uint16{0xf800, 0xf800, 0xf800, 0xf800}, 0xfec0, [4]int16{115, 115, 115, 115}},
		{"emergency-high-phase-flips-turn", 0, 0x801, 0x8000, 0, 0x100, 0x2001, 0x100, 100, [4]uint16{0x8c00, 0x8c00, 0x8c00, 0x8c00}, [4]uint16{0xc00, 0xc00, 0xc00, 0xc00}, 0xfec0, [4]int16{115, 115, 115, 115}},
		{"phase40-prevents-emergency", 0, 0x801, 0x8000, 0, 0x100, 0x2001, 0x40, 100, [4]uint16{0xa000, 0xa000, 0xc000, 0xc000}, [4]uint16{0xe000, 0xe000, 0xc000, 0xc000}, 0x100, [4]int16{82, 82, 82, 82}},
		{"distance0200-prevents-emergency", 0, 0x200, 0x8000, 0, 0x100, 0x2001, 0, 100, [4]uint16{0xa000, 0xa000, 0xc000, 0xc000}, [4]uint16{0xe000, 0xe000, 0xc000, 0xc000}, 0x100, [4]int16{82, 82, 82, 82}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for quality := 0; quality < 4; quality++ {
				s, in := decisionFixture(t)
				e := &s.init.Entities[1]
				e.Kind, e.RawMotion, e.Pose.Angles = tc.kind, tc.speed, [3]uint16{tc.pitch, tc.yaw, tc.roll}
				s.init.Entities[0].Pose.Angles[1] = tc.targetYaw
				s.init.Switches = s.init.Switches&^3 | uint16(quality)
				in.Phase = tc.phase
				s.enemyNoReleaseSteering(1, 0, 0, tc.distance, in)
				if e.Pose.Angles != [3]uint16{tc.wantPitch, tc.wantYaw[quality], tc.wantRoll[quality]} || e.RawMotion != tc.wantSpeed[quality] {
					t.Fatal(quality, e.Pose.Angles, e.RawMotion)
				}
			}
		})
	}
}
