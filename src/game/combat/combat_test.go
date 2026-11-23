package combat

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gunship2000-assets/internal/testfixture"
	"os"
	"strings"
	"testing"

	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/game/world"
)

func sourceTables(t *testing.T) *world.Bundle {
	t.Helper()
	b, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional original source table fixture")
	}
	if err != nil {
		t.Fatal(err)
	}
	if got := sha256.Sum256(b); hex.EncodeToString(got[:]) != "1a1db96070adb8c23e1eb499b449cc147f9fb0fd7b47a65e88c4c5603a40e209" {
		t.Fatal("unsupported source fixture")
	}
	a := &world.Bundle{Sine: make([]int16, 1025), AngleBases: make([]uint16, 8), Atan: make([]uint16, 4096), Worlds: map[string]*world.World{}, Geometries: map[string]*world.Geometry{}}
	for i := range a.Sine {
		a.Sine[i] = int16(binary.LittleEndian.Uint16(b[0x4bc50+2*i:]))
	}
	for i := range a.AngleBases {
		a.AngleBases[i] = binary.LittleEndian.Uint16(b[0x4c452+2*i:])
	}
	for i := range a.Atan {
		a.Atan[i] = binary.LittleEndian.Uint16(b[0x4c462+2*i:])
	}
	a.Worlds["flat"] = &world.World{Grid: make([]byte, 4096), Cells: make([]world.Cell, 64)}
	for i := range a.Worlds["flat"].Cells {
		a.Worlds["flat"].Cells[i].Geometry = "flat"
	}
	a.Geometries["flat"] = &world.Geometry{Heights: []world.HeightRegion{{Points: [][2]int16{{-4096, -4096}, {4096, -4096}, {4096, 4096}, {-4096, 4096}, {-4096, -4096}}, Plane: world.Plane{0, 1, 0, 0}}}}
	return a
}

func fixture(t *testing.T, flags uint16) (*State, Input) {
	t.Helper()
	var w gamestate.RuntimeWeapon
	w.RawWSHDID = 4
	w.CompactID = 2
	binary.LittleEndian.PutUint16(w.Raw[:], flags)
	w.Raw[4], w.Raw[5], w.Raw[6], w.Raw[7] = 100, 100, 100, 30
	binary.LittleEndian.PutUint16(w.Raw[11:], 7)
	m := &gamestate.InitializedMission{World: "flat", PlayerRow: 0, Stores: &gamestate.RuntimeStores{Definitions: []gamestate.RuntimeWeapon{w}, Ships: []gamestate.RuntimeAircraft{{Stations: [4]gamestate.RuntimeStation{{Present: true, CompactID: 2, Ammunition: 200, RawWSHDID: 4}, {Present: true, CompactID: 2, Ammunition: 20, RawWSHDID: 4, Mode: 1}}}}}}
	for i := 0; i < 4; i++ {
		e := gamestate.InitializedEntity{RuntimeRow: i, SourceRow: i, Participation: 1, Kind: 3, FlightSlot: -1, Pose: gamestate.InitializedPose{H0: 0x40000, H1: 0x40000 + uint32(i*2000), Height: 512}}
		e.Object.Raw[31] = 0
		binary.LittleEndian.PutUint16(e.Object.Raw[29:], 251)
		e.Object.Raw[0] = 0x10
		m.Entities = append(m.Entities, e)
	}
	m.Entities[0].FlightSlot = 0
	m.Entities[0].Kind = 0xff
	m.Entities[0].Object.Raw[10] = 0x20
	s, err := New(m, sourceTables(t), &gamestate.MissionData{Objectives: map[string]gamestate.MissionObjective{"primary": {RawKind: 1, DisplayKind: 3, Count: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	return s, Input{DF: 2, SelectedStation: 1, Salvo: 1, AudioEnabled: true, Random: func() uint16 { return 1 }}
}

func events(s *State, kind string) []Event {
	var a []Event
	for _, e := range s.Events {
		if e.Kind == kind {
			a = append(a, e)
		}
	}
	return a
}

func TestOriginalCombatOperands(t *testing.T) {
	b, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional original")
	}
	if err != nil {
		t.Fatal(err)
	}
	for ip, want := range map[int]string{0x559e: "c81e0000", 0x55fd: "2683bf050000", 0x56aa: "26833f10", 0x5738: "26832f14", 0x57b4: "268b85f100", 0x8d5a: "8a46f22ae46bd818", 0x9960: "837eee18", 0x9aad: "2680bf832501", 0x9c06: "26833ee81503"} {
		x, _ := hex.DecodeString(want)
		if !bytes.Equal(b[0xd8b0+ip:][:len(x)], x) {
			t.Fatalf("section1 operand%04x changed", ip)
		}
	}
	if !bytes.Equal(b[0x3df0+0x1ee6:][:31], []byte{0x55, 0x8b, 0xec, 0x8b, 0x46, 0x06, 0x8b, 0x5e, 0x08, 0x2b, 0x46, 0x0a, 0x1b, 0x5e, 0x0c, 0x99, 0x3b, 0xda, 0x74, 0x08, 0x8b, 0xc3, 0x99, 0x8b, 0xc2, 0x35, 0xf0, 0x7f, 0x0c, 0x01, 0x5d}) {
		t.Fatal("difference helper arithmetic changed")
	}
}

func TestDistinctDifferenceAndSourceMovement(t *testing.T) {
	for _, v := range []struct {
		a, b uint32
		want int16
	}{{100, 100, 1}, {110, 100, 11}, {100, 110, -9}, {0x40000, 0, -0 + 32753}, {0, 0x40000, -32753}} {
		if got := Difference(v.a, v.b); got != v.want {
			t.Fatalf("1ee6%+v =>%d", v, got)
		}
	}
	s, _ := fixture(t, 2)
	for _, v := range []struct {
		pitch, yaw uint16
		dx, dy, dz int32
	}{{0, 0, 0, 0, 64}, {0, 0x4000, -64, 0, 0}, {0x4000, 0, 0, 64, 64}} {
		p := gamestate.InitializedPose{H0: 0x40000, H1: 0x40000, Height: 512, Angles: [3]uint16{v.pitch, v.yaw, 0}}
		s.move(&p, 64)
		if int32(p.H0)-0x40000 != v.dx || int32(p.Height)-512 != v.dy || int32(p.H1)-0x40000 != v.dz {
			t.Fatalf("source0c6c %+v =>%+v", v, p)
		}
	}
}

func TestFireGatesAllocationAndDamageOrdering(t *testing.T) {
	s, in := fixture(t, 0x0e10)
	ship := &s.init.Stores.Ships[0]
	ship.Stations[1].Ammunition = 0
	if err := s.Fire(in); err != nil {
		t.Fatal(err)
	}
	if s.NextSlot != 0 || len(events(s, "launch")) != 0 {
		t.Fatal("empty fired")
	}
	ship.Stations[1].Ammunition = 5
	in.GearFlags = 1
	s.DrainEvents()
	if err := s.Fire(in); err != nil {
		t.Fatal(err)
	}
	if s.NextSlot != 0 || events(s, "sound")[0].Command != 3 {
		t.Fatal("bay gate")
	}
	in.GearFlags = 0
	s.DrainEvents()
	s.Slots[0].Lifetime = 124
	if err := s.Fire(in); err != nil {
		t.Fatal(err)
	}
	if s.NextSlot != 0 {
		t.Fatal("cooldown boundary")
	}
	s.Slots[0].Lifetime = 123
	s.Slots[1].Lifetime = 1
	s.DrainEvents()
	if err := s.Fire(in); err != nil {
		t.Fatal(err)
	}
	if s.NextSlot != 1 || ship.Stations[1].Ammunition != 5 || len(events(s, "busy")) != 1 {
		t.Fatal("busy slot searched or spent")
	}
	s, in = fixture(t, 2)
	in.SelectedStation = 0
	in.Damage[4] = 1
	draws := 0
	in.Random = func() uint16 { draws++; return 1 }
	if err := s.Fire(in); err != nil {
		t.Fatal(err)
	}
	if draws != 1 || s.NextSlot != 1 || s.init.Stores.Ships[0].Stations[0].Ammunition != 200 || len(events(s, "launch")) != 0 {
		t.Fatal("damaged cannon mutation/order")
	}
}

func TestSalvoAndBurstAreSourcePayloads(t *testing.T) {
	for _, salvo := range []int16{1, 2, 4} {
		s, in := fixture(t, 0x1e21)
		s.init.Stores.Ships[0].Stations[1].Mode = 3
		in.Salvo = salvo
		if err := s.Fire(in); err != nil {
			t.Fatal(err)
		}
		if s.NextSlot != 2 || s.init.Stores.Ships[0].Stations[1].Ammunition != 20-2*salvo || len(events(s, "launch")) != 2 {
			t.Fatalf("salvo%d", salvo)
		}
		for _, slot := range []int{1, 2} {
			p := s.Slots[slot]
			if p.Lifetime != 131 || p.Parameter != salvo || p.Template != map[int16]int{1: 1, 2: 2, 4: 3}[salvo] || p.Packed != 11 {
				t.Fatalf("source salvo payload %+v", p)
			}
		}
	}
	s, in := fixture(t, 2)
	in.SelectedStation = 0
	s.init.Stores.Definitions[0].RawWSHDID = 16
	if err := s.Fire(in); err != nil {
		t.Fatal(err)
	}
	if s.init.Stores.Ships[0].Stations[0].Ammunition != 180 || len(events(s, "launch")) != 1 {
		t.Fatal("burst was20 projectiles")
	}
	if err := s.Step(in); err != nil {
		t.Fatal(err)
	}
	if s.Slots[1].Lifetime != 32 || len(events(s, "shake")) != 1 || events(s, "sound")[0].Command != 7 {
		t.Fatal("cannon first step")
	}
}

func TestGuidedAttachmentTargetDomainsAndAudio(t *testing.T) {
	s, in := fixture(t, 0x288)
	in.Target = 0
	in.Contacts[2].Flags = 2
	in.Contacts[2].Kind = 1
	in.Contacts[1].Flags = 2
	in.Contacts[1].Kind = 3
	if err := s.Fire(in); err != nil {
		t.Fatal(err)
	}
	if s.Slots[1].Target != 2 {
		t.Fatal("IR stable class filter")
	}
	for i := 0; i < 4; i++ {
		before := s.Slots[1].Pose
		if err := s.Step(in); err != nil {
			t.Fatal(err)
		}
		if s.Slots[1].Pose != before {
			t.Fatal("attached guided moved")
		}
	}
	if s.Slots[1].Lifetime != 127 || events(s, "sound")[0].Command != 0x18 {
		t.Fatal("guided initial cadence")
	}
	if err := s.Step(in); err != nil {
		t.Fatal(err)
	}
	if s.Slots[1].Lifetime != 126 || s.Slots[1].Speed != 75 {
		t.Fatal("guided acceleration")
	}
	snd := events(s, "sound")
	if snd[len(snd)-1].Command != 8 {
		t.Fatal("guided release cue")
	}
	s, in = fixture(t, 0xc48)
	in.Contacts[2].Flags = 2
	in.Contacts[2].Kind = 3
	in.Contacts[2].AttackFlags = 0x20
	if err := s.Fire(in); err != nil {
		t.Fatal(err)
	}
	if s.Slots[1].Target != 0 {
		t.Fatal("lowbyte20 confused with highbyte20")
	}
	s.Slots[s.LastSlot].Lifetime = 123
	in.Contacts[2].AttackFlags = 0x2000
	if err := s.Fire(in); err != nil {
		t.Fatal(err)
	}
	if s.Slots[2].Target != 2 {
		t.Fatal("radar contact filter")
	}
}

func TestDamageOneDrawThresholdAndExactlyOnceCredit(t *testing.T) {
	s, in := fixture(t, 2)
	draws := 0
	in.Random = func() uint16 { draws++; return 1 }
	if err := s.ApplyStationaryHit(1, 0, 2, 2, in); err != nil {
		t.Fatal(err)
	}
	if draws != 1 || s.init.Entities[1].Participation != 1 || binary.LittleEndian.Uint16(s.init.Entities[1].DynamicRaw[4:]) != 250 {
		t.Fatal("one randomized amount reused count2")
	}
	if err := s.ApplyStationaryHit(1, 0, 2, 1, in); err != nil {
		t.Fatal(err)
	}
	if draws != 2 || s.init.Entities[1].Participation != 2 || s.Primary.Destroyed != 1 || s.Primary.Achieved || s.Credit[0] != 0x10 {
		t.Fatal("destruction/count/credit")
	}
	s.DrainEvents()
	if err := s.ApplyStationaryHit(1, 0, 2, 1, in); err != nil {
		t.Fatal(err)
	}
	if draws != 2 || len(s.Events) != 0 {
		t.Fatal("duplicate destruction credit/RNG")
	}
	if err := s.ApplyStationaryHit(2, 0, 2, 3, in); err != nil {
		t.Fatal(err)
	}
	if !s.Primary.Achieved || s.Primary.Destroyed != 2 || events(s, "notification")[0].Notification != 0x10 {
		t.Fatal("transformed count halved or wrong displaykind")
	}
	if s.Results[3].Kind != 1 || s.Results[3].Target != 2 || s.Results[3].Sequence != 2 {
		t.Fatal("result queue")
	}
}

func TestFamilyCollisionAndPlayerZeroBoundary(t *testing.T) {
	s, in := fixture(t, 0x1e21)
	e := &s.init.Entities[1]
	e.Pose = s.init.Entities[0].Pose
	e.Pose.H1 += 5
	s.Slots[1] = Projectile{Flags: 0x1e21, Lifetime: 49, Packed: 9, Shooter: 0, Target: 1, Speed: 100, Parameter: 2, Pose: s.init.Entities[0].Pose}
	if err := s.Step(in); err != nil {
		t.Fatal(err)
	}
	if s.Slots[1].Lifetime != 0 || s.Slots[1].Packed != 4 || s.Slots[1].Pose.H1 != e.Pose.H1 {
		t.Fatal("designated rocket collision must precede move")
	}
	s, in = fixture(t, 2)
	s.Slots[1] = Projectile{Flags: 2, Lifetime: 30, Packed: 8, Shooter: 0, Target: 0, Speed: 600, Pose: s.init.Entities[0].Pose}
	in.DF = 0
	draws := 0
	in.Random = func() uint16 { draws++; return 0 }
	if err := s.Step(in); err != nil {
		t.Fatal(err)
	}
	if s.Slots[1].Lifetime != 0 || len(events(s, "unsupported")) != 1 || s.init.Entities[0].Participation != 1 || draws != 0 {
		t.Fatal("family2 zero/player domain silently generalized")
	}
}

func TestUpperDomainAndRepeatedSnapshotPurity(t *testing.T) {
	s, in := fixture(t, 2)
	s.init.Stores.Ships[0].Stations[1].CompactID = 16
	before, _ := json.Marshal(s.init.Stores)
	if err := s.Fire(in); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatal("upperdomain accepted")
	}
	after, _ := json.Marshal(s.init.Stores)
	if !bytes.Equal(before, after) {
		t.Fatal("upperdomain changed stores")
	}
	s.Slots[23].Packed = 2
	s.Slots[23].Pose.Height = 51
	for i := 0; i < 5; i++ {
		_ = s.Slots[23].Raw()
	}
	if s.Slots[23].Packed != 2 {
		t.Fatal("snapshot decrementedtail")
	}
	if err := s.Step(in); err != nil {
		t.Fatal(err)
	}
	if s.Slots[23].Packed != 1 {
		t.Fatal("pool is not24slots")
	}
}

func TestPlayerFamiliesCompleteStationaryTrajectories(t *testing.T) {
	for _, family := range []struct {
		flags   uint16
		raw     int
		station int
	}{{0x0e10, 4, 1}, {0x1e21, 7, 1}, {0x0e02, 16, 0}, {0x0e01, 14, 0}} {
		s, in := fixture(t, family.flags)
		in.SelectedStation = family.station
		in.DF = 6
		in.Target = 2
		in.TargetAvailable = true
		s.init.Stores.Definitions[0].RawWSHDID = family.raw
		s.init.Stores.Ships[0].Stations[family.station].RawWSHDID = family.raw
		s.init.Entities[1].Pose.H1 = 0x41000
		draws := 0
		in.Random = func() uint16 { draws++; return 1 }
		var trace []any
		for shot := 0; shot < 3; shot++ {
			// Previous projectiles have reached their source collision/lifetime;
			// no reset of allocation/history is needed to fire the next shot.
			if err := s.Fire(in); err != nil {
				t.Fatal(err)
			}
			slot := int(s.LastSlot)
			launched := s.Slots[slot].Lifetime != 0
			if !launched {
				t.Fatalf("family%d shot%d rejected", family.flags&3, shot)
			}
			for tick := 0; tick < 140 && s.Slots[slot].Lifetime != 0; tick++ {
				in.Phase++
				if err := s.Step(in); err != nil {
					t.Fatal(err)
				}
				trace = append(trace, struct {
					Shot, Tick    int
					Projectile    Projectile
					Victim        int16
					Participation byte
				}{shot, tick, s.Slots[slot], int16(binary.LittleEndian.Uint16(s.init.Entities[1].DynamicRaw[4:])), s.init.Entities[1].Participation})
			}
			if s.Slots[slot].Lifetime != 0 {
				t.Fatal("source lifetime failed to terminate")
			}
		}
		//100+25 source perturbation gives125 per impact: three successful
		// singleton impacts exceed251; each hit consumes exactly one RNG word.
		if s.init.Entities[1].Participation != 2 || draws != 3 || s.Results[3].Sequence != 1 {
			t.Fatalf("family%d trajectory draw%d victim%+v", family.flags&3, draws, s.init.Entities[1])
		}
		if root := os.Getenv("GS2000_M_COMBAT_TRACE"); root != "" {
			if err := os.MkdirAll(root, 0755); err != nil {
				t.Fatal(err)
			}
			name := fmt.Sprintf("%s/raw-%02d.json", root, family.raw)
			f, err := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				t.Fatal(err)
			}
			b, _ := json.MarshalIndent(struct {
				Meaning string
				Raw     int
				Flags   uint16
				Trace   any
				Events  any
			}{"Supplied flat query/target state using original trig and checked source operands, not original CPU trajectory", family.raw, family.flags, trace, s.Events}, "", "  ")
			if _, err = f.Write(b); err != nil {
				t.Fatal(err)
			}
			if err = f.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestDamageClassDomainAndOneDrawAmount(t *testing.T) {
	for _, v := range []struct {
		class byte
		want  int
		draws int
	}{{0, 10, 1}, {1, 20, 1}, {2, 40, 1}, {3, 40, 1}, {255, 0, 0}} {
		s, in := fixture(t, 2)
		s.init.Stores.Definitions[0].Raw[4], s.init.Stores.Definitions[0].Raw[5], s.init.Stores.Definitions[0].Raw[6] = 10, 20, 40
		s.init.Entities[1].Object.Raw[31] = v.class
		n := 0
		in.Random = func() uint16 { n++; return 2 }
		if err := s.ApplyStationaryHit(1, 0, 2, 1, in); err != nil {
			t.Fatal(err)
		}
		if got := int(binary.LittleEndian.Uint16(s.init.Entities[1].DynamicRaw[4:])); got != v.want || n != v.draws {
			t.Fatalf("class%d got%d draws%d", v.class, got, n)
		}
	}
}

func TestPostAttemptSalvoUsesSignedRemainingAmmo(t *testing.T) {
	for _, tc := range []struct {
		ammo           int16
		busy, cooldown bool
		want           int16
	}{
		{11, false, false, 2}, {7, false, false, 1}, {3, false, false, 1}, {7, true, false, 2}, {-1, true, false, 1}, {7, false, true, 0}, {0, false, false, 0},
	} {
		s, in := fixture(t, 0x1e21)
		in.Salvo = 4
		s.init.Stores.Ships[0].Stations[1].Ammunition = tc.ammo
		if tc.busy {
			s.Slots[1].Lifetime = 1
		}
		if tc.cooldown {
			s.Slots[0].Lifetime = 128
		}
		if err := s.Fire(in); err != nil {
			t.Fatal(err)
		}
		ev := events(s, "salvo")
		if tc.want == 0 {
			if len(ev) != 0 {
				t.Fatalf("early return %+v emitted%+v", tc, ev)
			}
		} else if len(ev) != 1 || ev[0].Value != tc.want {
			t.Fatalf("post attempts %+v =>%+v", tc, ev)
		}
	}
}

func TestCollateralHeightWordOverflowUsesSignedJGE(t *testing.T) {
	// Source90f9 CMPAX,40 /90fc JGE rejects +64 but accepts ABS(8000),
	// which remains signed -32768. Designated-target branches are unsigned.
	b, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b[0xd8b0+0x90f9:][:5], []byte{0x3d, 0x40, 0x00, 0x7d, 0x08}) {
		t.Fatal("signed source collateral bound changed")
	}
	for _, height := range []uint16{576, 0x8200} {
		s, in := fixture(t, 0x1e21)
		s.init.Entities[1].Pose.H0, s.init.Entities[1].Pose.H1 = 0x40000, 0x40000
		s.init.Entities[1].Pose.Height = height
		s.Slots[1] = Projectile{Flags: 0x1e21, Lifetime: 49, Packed: 9, Speed: 100, Parameter: 1, Pose: s.init.Entities[0].Pose}
		if err := s.Step(in); err != nil {
			t.Fatal(err)
		}
		got := len(events(s, "impact")) != 0
		if got != (height == 0x8200) {
			t.Fatalf("height%04x impact%v", height, got)
		}
	}
}

func TestNoTargetCannonSourceFirstBurstBoundary(t *testing.T) {
	s, in := fixture(t, 2)
	s.init.Stores.Definitions[0].RawWSHDID = 16
	in.SelectedStation = 0
	in.DF = 6
	in.Phase = 0
	if err := s.Fire(in); err != nil {
		t.Fatal(err)
	}
	slot := s.LastSlot
	if err := s.Step(in); err != nil {
		t.Fatal(err)
	}
	if s.Slots[slot].Lifetime != 32 {
		t.Fatal("first source burst must only reset lifetime")
	}
	if err := s.Step(in); err != nil {
		t.Fatal(err)
	}
	// A level source pose moves45 native units after speed50→75, but the
	// direct row0 test is within its75 bound. This is the explicit unsupported
	// player boundary, not evidence that original zero means absent target.
	if s.Slots[slot].Lifetime != 0 || s.Slots[slot].Packed != 0 || len(events(s, "unsupported")) != 1 || len(events(s, "impact")) != 0 {
		t.Fatalf("no-target boundary%+v events%+v", s.Slots[slot], s.Events)
	}
}
