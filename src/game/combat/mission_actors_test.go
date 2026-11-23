package combat

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"gunship2000-assets/game/avionics"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func airFixture(t *testing.T) (*State, Input) {
	s, in := artilleryFixture(t)
	s.SetMissionActorsEnabled(true)
	sup := s.init.Support
	sup.Configuration.Mode = 2
	sup.Configuration.Rows = []int{79}
	sup.Configuration.PrefixRaw = [2]byte{23, 24}
	sup.Availability = 0x12
	sup.Request = 255
	w := s.init.Stores.Definitions[0]
	w.RawWSHDID = 23
	binary.LittleEndian.PutUint16(w.Raw[:], 2)
	w.Raw[9] = 100
	s.init.Stores.Definitions = []gamestate.RuntimeWeapon{w}
	w.CompactID = 3
	w.RawWSHDID = 24
	binary.LittleEndian.PutUint16(w.Raw[:], 0)
	s.init.Stores.Definitions = append(s.init.Stores.Definitions, w)
	for i := range s.init.Entities {
		s.init.Entities[i].Kind = 0xff
		s.init.Entities[i].Participation = 0
	}
	e := &s.init.Entities[79]
	e.Kind = 0x11
	e.Participation = 0
	e.Pose = gamestate.InitializedPose{H0: 0x40000, H1: 0x40000}
	binary.LittleEndian.PutUint32(e.Object.Raw[2:], 0x40000)
	binary.LittleEndian.PutUint32(e.Object.Raw[6:], 0x40000)
	s.init.Entities[9].Kind = 3
	s.init.Entities[9].Participation = 1
	s.init.Entities[9].Pose = gamestate.InitializedPose{H0: 0x40000, H1: 0x41000, Height: 512}
	sup.Target = gamestate.InitializedPose{H0: 0x40000, H1: 0x44000}
	in.E0 = true
	in.ObserveSupport = func(actor, target int) (bool, error) {
		if actor != 79 || target != 9 {
			t.Fatal(actor, target)
		}
		return true, nil
	}
	return s, in
}
func TestAirSupportActivationFireReturnAndNoExtraDraws(t *testing.T) {
	s, in := airFixture(t)
	draws := 0
	in.Random = func() uint16 { draws++; return 0 }
	if err := s.UpdateAirSupport(79, in); err != nil {
		t.Fatal(err)
	}
	e := &s.init.Entities[79]
	if e.Participation != 1 || e.Pose.Height != 1536 || e.RawMotion != 400 || s.init.Support.Request != 255 {
		t.Fatal(e, s.init.Support)
	}
	if err := s.UpdateAirSupport(79, in); err != nil {
		t.Fatal(err)
	}
	slot := int((byte(s.NextSlot) - byte((254&0x30)>>4) + 8) & 15)
	p := s.Slots[slot]
	if s.init.Support.Request != 254 || p.Lifetime != 131 || p.Shooter != 79 || p.Target != 9 || p.Packed != 11 || draws != 0 {
		t.Fatal(p, s.init.Support, draws)
	}
	// Reaching64 switches the actual dynamic+3 return owner, not participation.
	s.init.Support.Request = 65
	e.Pose = gamestate.InitializedPose{H0: 0x40000, H1: 0x40000, Height: 1536}
	if err := s.UpdateAirSupport(79, in); err != nil {
		t.Fatal(err)
	}
	if e.DynamicRaw[3] != 1 || s.init.Support.Request != 64 {
		t.Fatal(e.DynamicRaw, s.init.Support)
	}
	// Actual source home/behind cone <512 clears request and enters dormantP0.
	s.init.Entities[9].Participation = 0
	e.Pose = gamestate.InitializedPose{H0: 0x40000, H1: 0x40110, Height: 1536}
	e.Pose.Angles[1] = 0
	if err := s.UpdateAirSupport(79, in); err != nil {
		t.Fatal(err)
	}
	if e.Participation != 0 || s.init.Support.Request != 0 {
		t.Fatal(e, s.init.Support)
	}
	if err := s.UpdateAirSupport(79, in); err != nil || e.Participation != 0 {
		t.Fatal(err, e)
	}
}
func TestAirSupportE0SightlineAndBusySlot(t *testing.T) {
	for _, tc := range []struct {
		e0, visible, busy bool
		want              uint16
		shot              bool
	}{{false, true, false, 255, false}, {true, false, false, 255, false}, {true, true, true, 254, false}, {true, true, false, 254, true}} {
		s, in := airFixture(t)
		s.UpdateAirSupport(79, in)
		in.E0 = tc.e0
		in.ObserveSupport = func(int, int) (bool, error) { return tc.visible, nil }
		slot := 5
		if tc.busy {
			s.Slots[slot].Lifetime = 5
		}
		if err := s.UpdateAirSupport(79, in); err != nil {
			t.Fatal(err)
		}
		if s.init.Support.Request != tc.want || (s.Slots[slot].Lifetime == 131) != tc.shot {
			t.Fatal(tc, s.init.Support, s.Slots[slot])
		}
	}
	s, in := airFixture(t)
	s.SetMissionActorsEnabled(false)
	if s.UpdateAirSupport(79, in) == nil {
		t.Fatal("old profile admitted new actor")
	}
}
func fireFixture(t *testing.T) (*State, Input) {
	s, in := logisticsFollowerFixture(t)
	s.SetMissionActorsEnabled(true)
	e := &s.init.Entities[1]
	e.Kind = 0x21
	e.Participation = 1
	e.ControlRaw[3] = 10
	e.ControlRaw[4] = 1
	e.Pose = gamestate.InitializedPose{H0: 0x40000, H1: 0x40000, Height: 512}
	e.RawMotion = 96
	for len(s.init.Entities) < 10 {
		s.init.Entities = append(s.init.Entities, gamestate.InitializedEntity{RuntimeRow: len(s.init.Entities)})
	}
	target := &s.init.Entities[9]
	target.RuntimeRow = 9
	target.Kind = 3
	target.Participation = 1
	target.Pose = gamestate.InitializedPose{H0: 0x40000, H1: 0x41000, Height: 512}
	target.Object.Raw[31] = 0
	binary.LittleEndian.PutUint16(target.Object.Raw[29:], 251)
	w := s.init.Stores.Definitions[0]
	w.CompactID = 2
	w.RawWSHDID = 4
	binary.LittleEndian.PutUint16(w.Raw[:], 0x0800)
	w.Raw[4] = 100
	binary.LittleEndian.PutUint16(w.Raw[9:], 100)
	w.Raw[7] = 30
	s.init.Stores.Definitions = []gamestate.RuntimeWeapon{w}
	p := byte(25)
	s.init.Stores.Ships[1].Proficiency = &p
	s.init.Stores.Ships[1].Stations = [4]gamestate.RuntimeStation{{}, {Present: true, CompactID: 2, Mode: 1, Ammunition: 10}}
	s.init.CrewPerformance = make([]gamestate.SourceCrewPerformance, 5)
	s.init.CrewPerformance[1].SkillKnown = true
	s.init.CrewPerformance[1].RawB = 25
	in.E0 = true
	in.Random = func() uint16 { return 127 }
	return s, in
}
func TestHealthyFollowerBodyShotAmmoProficiencyAndCooldown(t *testing.T) {
	s, in := fireFixture(t)
	draws := 0
	in.Random = func() uint16 { draws++; return 127 }
	if err := s.advanceHealthyFollower(1, in); err != nil {
		t.Fatal(err)
	}
	p := s.Slots[16]
	if p.Lifetime != 131 || p.Shooter != 1 || p.Target != 9 || p.Pose.Height != 528 || p.Speed != 146 || draws != 1 || s.init.Stores.Ships[1].Stations[1].Ammunition != 9 || s.init.Entities[1].DynamicRaw[3] != 7 || s.init.CrewPerformance[1].RawB != 19 {
		t.Fatal(p, draws, s.init.Stores.Ships[1], s.init.CrewPerformance[1])
	}
	// Busy slot, no E0 and cooldown preserve accepted ammunition/RNG.
	in.E0 = false
	s.advanceHealthyFollower(1, in)
	if draws != 1 || s.init.Entities[1].DynamicRaw[3] != 7 {
		t.Fatal(draws)
	}
	in.E0 = true
	s.advanceHealthyFollower(1, in)
	if draws != 1 || s.init.Entities[1].DynamicRaw[3] != 6 {
		t.Fatal(draws)
	}
}
func TestHealthyFollowerMissStandbyAndMissingOperands(t *testing.T) {
	s, in := fireFixture(t)
	in.Random = func() uint16 { return 0 }
	if err := s.advanceHealthyFollower(1, in); err != nil {
		t.Fatal(err)
	}
	if s.Slots[16].Target != 0 || s.Slots[16].Parameter != 0 || s.init.CrewPerformance[1].RawB != 82 {
		t.Fatal(s.Slots[16], s.init.CrewPerformance[1])
	}
	s, in = fireFixture(t)
	s.init.Entities[1].ControlRaw[4] = 0
	s.advanceHealthyFollower(1, in)
	if s.Slots[16].Lifetime != 0 || s.init.Entities[1].DynamicRaw[3] != 50 {
		t.Fatal("standby", s.init.Entities[1])
	}
	s, in = fireFixture(t)
	s.init.Stores.Ships[1].Proficiency = nil
	if s.advanceHealthyFollower(1, in) == nil || s.init.Stores.Ships[1].Stations[1].Ammunition != 10 {
		t.Fatal("absent skill invented")
	}
}

func TestAirSupportRealProjectileImpactAndUnstagedReturn(t *testing.T) {
	s, in := airFixture(t)
	s.assets.Models = []string{"flat"}
	target := &s.init.Entities[9]
	target.Object.Raw[27] = 100
	binary.LittleEndian.PutUint16(target.Object.Raw[29:], 100)
	seenLaunch, seenDead, seenReturn := false, false, false
	for tick := 0; tick < 20000; tick++ {
		in.E0 = tick%6 == 0
		in.Phase = uint16(tick)
		if err := s.UpdateAirSupport(79, in); err != nil {
			t.Fatal(tick, err)
		}
		for _, e := range s.DrainEvents() {
			if e.Kind == "launch" {
				seenLaunch = true
			}
		}
		if err := s.Step(in); err != nil {
			t.Fatal(tick, err)
		}
		for _, e := range s.DrainEvents() {
			if e.Kind == "destroyed" && e.Actor == 79 && e.Target == 9 {
				seenDead = true
			}
		}
		if seenLaunch && s.init.Entities[79].Participation == 0 && s.init.Support.Request == 0 {
			seenReturn = true
			t.Logf("Air command->realimpact->sourcehome completed at step%d score%d", tick, s.Score)
			break
		}
	}
	if !seenLaunch || !seenDead || !seenReturn || s.Credit[4]>>4 != 7 {
		t.Fatalf("launch%v destruction%v return%v credit%02x air%+v request%d", seenLaunch, seenDead, seenReturn, s.Credit[4], s.init.Entities[79], s.init.Support.Request)
	}
}
func TestHealthyFollowerRealGuidedImpactCreditsActualActor(t *testing.T) {
	s, in := fireFixture(t)
	s.assets.Models = []string{"flat"}
	target := &s.init.Entities[9]
	target.Object.Raw[27] = 100
	binary.LittleEndian.PutUint16(target.Object.Raw[29:], 100)
	for tick := 0; tick < 1000 && target.Participation == 1; tick++ {
		in.E0 = tick%6 == 0
		in.Phase = uint16(tick)
		if err := s.advanceHealthyFollower(1, in); err != nil {
			t.Fatal(err)
		}
		if err := s.Step(in); err != nil {
			t.Fatal(err)
		}
	}
	if target.Participation != 2 || s.Credit[4]>>4 != 2 || s.Score != 100 {
		t.Fatal("actual source hit/credit", target.Participation, s.Credit, s.Score)
	}
}

// A supplied flat ridge surrogate (body below terrain, mast above) exercises the
// actual player ray, five-draw assignment tail and subsequent healthy consumer.
// It is not a naturally flown ridge encounter or an original runtime witness.
func TestMastRaySharedTailThenHealthyBodyFireAndHistoricalControl(t *testing.T) {
	for _, modern := range []bool{false, true} {
		s, shot := fireFixture(t)
		s.SetMissionActorsEnabled(modern)
		s.init.Entities[1].FlightSlot = 1
		s.init.Entities[1].ControlRaw[3] = 0
		s.init.Entities[1].ControlRaw[1] = 1
		s.init.Entities[1].ControlRaw[5] = 0
		s.init.Entities[1].Object.Raw[10] = 0x21
		s.init.Entities[10] = s.init.Entities[9]
		s.init.Entities[10].RuntimeRow = 10
		s.init.Entities[10].Pose.H1 += 512
		a := avionics.New()
		body := avionics.Pose{H0: 0x40000, H1: 0x40000, Height: -64}
		head := body
		head.Height += 128
		in := avionics.Input{Player: body, Head: head, Mast: true, World: "flat", Assets: s.assets, ModernTail: true}
		for row, e := range s.init.Entities {
			in.Entities[row] = avionics.Entity{Present: true, Row: row, Kind: e.Kind, Participation: e.Participation, Pose: avionics.Pose{H0: e.Pose.H0, H1: e.Pose.H1, Height: int16(e.Pose.Height), Angles: e.Pose.Angles}, FlightSlot: e.FlightSlot, Object: e.Object.Raw, Control: e.ControlRaw}
		}
		in.Entities[0].Control[3] = 255
		// Only one source flight actor participates in this isolated handoff.
		in.Entities[2].Object[10] = 0
		draws := 0
		in.Random = func() uint16 { draws++; return 0 }
		in.SetControlTarget = func(actor int, target int8) error { s.init.Entities[actor].ControlRaw[3] = byte(target); return nil }
		if err := a.Advance(in); err != nil {
			t.Fatal(err)
		}
		if a.BodyQuery.Status != 0 || a.HeadQuery.Status == 0 || a.Contacts[9].Flags != 0xfa || s.init.Entities[1].ControlRaw[3] != 11 || a.Contacts[10].Flags != 0xfa || draws != 5 {
			t.Fatalf("mast→tail body%+v head%+v contact%+v control%d draws%d", a.BodyQuery, a.HeadQuery, a.Contacts[9], s.init.Entities[1].ControlRaw[3], draws)
		}
		// This is the same dispatch used at the original healthy callback phase.
		if modern {
			if err := s.advanceHealthyFollower(1, shot); err != nil {
				t.Fatal(err)
			}
		} else {
			s.disableFollower(1)
		}
		if modern && (s.Slots[16].Shooter != 1 || s.Slots[16].Target != 10 || s.Slots[16].Lifetime != 131) {
			t.Fatal("shared actual assignment failed body launch", s.Slots[16])
		}
		if !modern && s.Slots[16].Lifetime != 0 {
			t.Fatal("historical profile acquired healthy fire")
		}
	}
}

func TestOriginalAirAndHealthyFireOperands(t *testing.T) {
	_ = sourceTables(t) // Hash-bound executable and exact source angle tables.
	raw, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	for ip, want := range map[int]string{
		0xb079: "26c687832501", 0xb07f: "26c78798250004", 0xb1b5: "26c78788256400", 0xb268: "2639858825", 0xb381: "050001", 0xb4d7: "9a38207f03250700", 0xb697: "8e066433", 0xb69f: "26d1bf8e25",
		0xb707: "26c78798250004", 0xb70e: "26c78788259001", 0xb75f: "26f687420602", 0xb775: "a03c59", 0xb94f: "3d0040", 0xb9e1: "8b46fcc1f802", 0xba5c: "8b46e0050001",
		0xc8b5: "8e0664336b5ede18", 0xc8ca: "26c78798250006", 0xc902: "26c687832501", 0xcc6d: "c606ae3540", 0xcd5e: "9a1606db00", 0xcd92: "24fe0c06", 0xcdbd: "26ff0ea20a", 0xcde3: "8e069e33268a0ea20a", 0xcf7c: "268b878825f7e9b90a0099f7f9",
		0xa7e6: "c8220000", 0xa929: "2680bf8b0400", 0xa9f4: "3d00407e05", 0xabb7: "8b46e8c1e805263987ef0072ab", 0xad1b: "f7e72bd2f7f1", 0xad24: "3d60007c03", 0xad74: "2680bf040000", 0xad7f: "2680bf050000", 0xaeaf: "8bf39a38207f03257f00", 0xaf21: "e880cf", 0xaf3c: "9a946d7509", 0x8205: "26a1c10a",
	} {
		literal, e := hex.DecodeString(want)
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(raw[0xd8b0+ip:][:len(literal)], literal) {
			t.Fatalf("canonical0975:%04x operand differs", ip)
		}
	}
}
func TestHealthyFollowerBoundaryAndDeadAimRestore(t *testing.T) {
	s, in := fireFixture(t)
	e := &s.init.Entities[1]
	e.Pose.Angles[0] = 123
	s.init.Entities[9].Participation = 2
	if err := s.advanceHealthyFollower(1, in); err != nil {
		t.Fatal(err)
	}
	if e.Pose.Angles[0] != 123 || s.Slots[16].Lifetime != 0 || s.init.Stores.Ships[1].Stations[1].Ammunition != 10 {
		t.Fatal("AF44 dead-target restore/ammo", e.Pose, s.Slots[16])
	}
	for _, tc := range []struct {
		distance uint32
		want     bool
	}{{63, false}, {64, true}, {3040, true}, {3072, false}} {
		s, in = fireFixture(t)
		s.init.Entities[9].Pose.H1 = s.init.Entities[1].Pose.H1 + tc.distance*16
		// proficiency100/range100: source chance95 at3040,96 at3072; both within the independent range gate.
		if tc.distance >= 3000 {
			v := byte(100)
			s.init.Stores.Ships[1].Proficiency = &v
		}
		if err := s.advanceHealthyFollower(1, in); err != nil {
			t.Fatal(err)
		}
		if (s.Slots[16].Lifetime != 0) != tc.want {
			t.Fatal("source min64/chance96", tc, s.Slots[16])
		}
	}
}
func TestAirSupportActualDeathDispatchAndActivationVisibility(t *testing.T) {
	s, in := airFixture(t)
	e := &s.init.Entities[79]
	e.Exclusion = "source airborne reserve awaits AA2 activation; autonomous activation suppressed"
	if err := s.UpdateAirSupport(79, in); err != nil {
		t.Fatal(err)
	}
	if !e.Visible || e.Exclusion != "" {
		t.Fatal("AA2 activation did not release reserve presentation", e)
	}
	e.Participation = 2
	e.Pose.Height = 2
	e.ReplacementModelID = 0
	for tick := 0; tick < 50 && e.Kind != 0x1c; tick++ {
		if _, err := s.advanceLogisticsFollowers(in, nil); err != nil {
			t.Fatal(err)
		}
	}
	if e.Kind != 0x1c || e.Participation != 0 {
		t.Fatal("real source fall/contact/wreck", e)
	}
	rows, err := s.advanceLogisticsFollowers(in, nil)
	if err != nil || e.Participation != 4 || len(rows) != 1 || rows[0] != 79 {
		t.Fatal("wreck observation phase", e, rows, err)
	}
}

func TestRetainedSupportUsesActualCompactCarrierWithoutFakePrefix(t *testing.T) {
	s, in := artilleryFixture(t)
	s.SetMissionActorsEnabled(true)
	cfg := &s.init.Support.Configuration
	cfg.Profile = gamestate.SupportRetainedInitializationProfile
	cfg.Mode = 0
	cfg.PrefixRaw = [2]byte{}
	cfg.RetainedRows = nil
	for _, row := range cfg.Rows {
		s.init.Entities[row].Object.Raw[10] = 0x19
		cfg.RetainedRows = append(cfg.RetainedRows, s.init.Entities[row].Object)
	}
	s.init.Stores.Definitions[0].RawWSHDID = 4 // Actual accepted player weapon; no raw25 fictional prefix.
	s.init.Support.Request = 16
	if err := gamestate.ValidateSupportRuntimeDefinitions(cfg, s.init.Stores); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateArtillery(78, in); err != nil {
		t.Fatal(err)
	}
	if s.Slots[1].Lifetime != 32 || s.Slots[1].Packed>>2 != 2 || cfg.Mode != 0 || cfg.PrefixRaw != ([2]byte{}) {
		t.Fatal("retained rawMode0 sourcecompact2", s.Slots[1], cfg)
	}
	s.SetMissionActorsEnabled(false)
	if err := s.UpdateArtillery(78, in); err == nil {
		t.Fatal("old profile admitted retained-memory controller")
	}
	s, in = airFixture(t)
	cfg = &s.init.Support.Configuration
	cfg.Profile = gamestate.SupportRetainedInitializationProfile
	cfg.Mode = 1
	cfg.PrefixRaw = [2]byte{25, 0}
	s.init.Entities[79].Object.Raw[10] = 0x11
	cfg.RetainedRows = []gamestate.MissionObject{s.init.Entities[79].Object}
	s.init.Stores.Definitions[0].RawWSHDID = 25
	s.init.Stores.Definitions[1].RawWSHDID = 4
	if err := gamestate.ValidateSupportRuntimeDefinitions(cfg, s.init.Stores); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateAirSupport(79, in); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateAirSupport(79, in); err != nil {
		t.Fatal(err)
	}
	if s.Slots[5].Lifetime != 131 || s.Slots[5].Shooter != 79 || s.Slots[5].Packed>>2 != 2 {
		t.Fatal("actual mixed-prefix Air", s.Slots[5])
	}
	s.init.Stores.Definitions = s.init.Stores.Definitions[:1]
	if err := gamestate.ValidateSupportRuntimeDefinitions(cfg, s.init.Stores); err == nil {
		t.Fatal("undefinedcompact3 invented for retained Air")
	}
}
func TestHealthySignedABSMinDoesNotInventSlowdown(t *testing.T) {
	s, in := fireFixture(t)
	s.init.Entities[1].Pose.Angles[1] = 0x8000
	if err := s.advanceHealthyFollower(1, in); err != nil {
		t.Fatal(err)
	}
	if s.init.Entities[1].RawMotion != 96 || s.Slots[16].Lifetime != 0 {
		t.Fatal("signed JLE atABS8000 then unsigned firing cone", s.init.Entities[1], s.Slots[16])
	}
}
