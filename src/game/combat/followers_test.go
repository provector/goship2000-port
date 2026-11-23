package combat

import (
	"encoding/binary"
	"gunship2000-assets/game/flight"
	gamestate "gunship2000-assets/game/state"
	"testing"
)

func followerFixture(t *testing.T) (*State, Input, *flight.Systems) {
	s, in := fixture(t, 0)
	m := s.init
	m.Entities = make([]gamestate.InitializedEntity, 80)
	m.Rows = make([]gamestate.MissionObject, 80)
	m.FlightRows = []int{0, 1, 2, 3, 4}
	m.Targets = &gamestate.MissionTargets{}
	m.Stores.Ships = make([]gamestate.RuntimeAircraft, 5)
	m.Stores.Blueprint = make([]gamestate.RuntimeAircraft, 5)
	m.Stores.Damage = make([][10]byte, 5)
	m.Stores.FreshShipPadding = true
	s.assets.Models = []string{"flat", "flat", "flat"}
	for i := range m.Entities {
		m.Entities[i] = gamestate.InitializedEntity{RuntimeRow: i, SourceRow: i, Kind: 0x40, Participation: 0, ReplacementModelID: 2, Pose: gamestate.InitializedPose{H0: 0x40000, H1: 0x40000, Height: 400}}
	}
	for i := 0; i < 5; i++ {
		m.Rows[i].Raw[10] = 0x20
		m.Entities[i].Object = m.Rows[i]
		m.Entities[i].Kind = 0x20
		m.Entities[i].Participation = 1
		m.Entities[i].ControlRaw[1] = 1
		m.Entities[i].Pose.H0 += uint32(i * 4096)
		skill := byte(0)
		m.Stores.Ships[i] = gamestate.RuntimeAircraft{Flags: 6, Fuel: 100, Proficiency: &skill}
		m.Stores.Blueprint[i] = m.Stores.Ships[i]
	}
	m.Entities[0].Kind, m.Entities[0].Participation = 0xff, 0
	for i := 5; i < 9; i++ {
		e := &m.Entities[i]
		e.Kind = 0x1b
		e.Object.Raw[10] = 0x1b
		e.Exclusion = "source follower-status reserve has no initial participation"
	}
	binary.LittleEndian.PutUint32(m.Targets.Records[0][42:], 0x10000)
	binary.LittleEndian.PutUint32(m.Targets.Records[0][46:], 0x10000)
	sys := flight.NewSystems([]int16{100, 0, 100, 100, 100}, []bool{true, true, true, true, true})
	sys.Countdown = 0
	in.Random = func() uint16 { return 127 }
	return s, in, &sys
}
func TestFollowerFailureToSourcePositionedPickup(t *testing.T) {
	s, in, sys := followerFixture(t)
	m := s.init
	e := &m.Entities[1]
	for _, off := range []int{18, 28, 38} {
		binary.LittleEndian.PutUint16(e.ControlRaw[off:], 0xbeef)
	}
	sys.AdvanceWithFollowers(true, 16, s.FollowerFuelContext(in.Random))
	if m.Stores.Damage[1][9] != 2 || !m.FollowerFailureKnown[1] || sys.UnsupportedFollowerEmptyBurn != 0 {
		t.Fatal("real oldempty did not own source609")
	}
	if _, err := s.AdvanceFollowers(in, sys); err != nil {
		t.Fatal(err)
	}
	if e.Kind != 0x26 || e.Participation != 1 || e.Pose.Height != 350 {
		t.Fatalf("old20 callback must retain its own geometry this visit: %+v", e)
	}
	for _, off := range []int{18, 28, 38} {
		if binary.LittleEndian.Uint16(e.ControlRaw[off:]) != 0xbeef {
			t.Fatal("source waypoint tail lost")
		}
	}
	for i := 0; i < 100 && m.Entities[5].Participation == 0; i++ {
		if _, err := s.AdvanceFollowers(in, sys); err != nil {
			t.Fatal(err)
		}
	}
	reserve := &m.Entities[5]
	if e.Kind != 0x20 || e.Participation != 0 || e.ControlRaw[1] != 0 || m.Stores.Damage[1][9] != 0 || reserve.Participation != 1 || reserve.Pose.H0 == 0 {
		t.Fatal("descent/contact/reserve successor missing", e, reserve)
	}
	notices := events(s, "notification")
	found := false
	for _, n := range notices {
		found = found || n.Notification == 22
	}
	if !found {
		t.Fatal("sourceSOS22 missing")
	}
	// An actual near/slow player approach picks up the source-positioned actor.
	m.Entities[0].Pose = reserve.Pose
	m.Entities[0].Pose.Height = 0
	before := m.Stores.Ships[0].Flags
	s.AdvanceObjectives(in)
	if reserve.Kind != 0x41 || reserve.DynamicRaw[3] != 0 || m.Stores.Ships[0].Flags != before || s.Results[3].Kind != 6 {
		t.Fatal("pickup changed cargo or omitted actual carrier/event")
	}
	seq := s.Results[3].Sequence
	s.AdvanceObjectives(in)
	if s.Results[3].Sequence != seq {
		t.Fatal("duplicate pickup credit")
	}
}
func TestFollowerSharedBurnBoundariesAndPrefix(t *testing.T) {
	for _, tc := range []struct {
		height int16
		skill  byte
		draw   uint16
		want   byte
		calls  int
	}{{2, 0, 127, 2, 0}, {3, 0, 0, 1, 1}, {3, 25, 25, 1, 1}, {3, 25, 26, 2, 1}, {3, 128, 127, 1, 1}} {
		s, _, sys := followerFixture(t)
		s.init.Entities[1].Pose.Height = uint16(tc.height)
		*s.init.Stores.Ships[1].Proficiency = tc.skill
		calls := 0
		sys.AdvanceWithFollowers(true, 16, s.FollowerFuelContext(func() uint16 { calls++; return tc.draw }))
		if s.init.Stores.Damage[1][9] != tc.want || calls != tc.calls {
			t.Fatal(tc, s.init.Stores.Damage[1], calls)
		}
	}
	s, in, sys := followerFixture(t)
	s.init.Rows[1].Raw[10] = 0
	sys.Fuel[2] = 0
	sys.AdvanceWithFollowers(true, 16, s.FollowerFuelContext(in.Random))
	if s.init.Stores.Damage[2][9] != 0 || sys.UnsupportedFollowerEmptyBurn != 0 {
		t.Fatal("fuel crossed prefix gap")
	}
}
func TestFollowerDisabledRegisterQuirkAndNoSecondVisit(t *testing.T) {
	s, in, sys := followerFixture(t)
	m := s.init
	m.Entities[2].ControlRaw[5] = 1
	sys.AdvanceWithFollowers(true, 16, s.FollowerFuelContext(in.Random))
	s.AdvanceFollowers(in, sys)
	if m.Entities[1].Kind != 0x20 || m.Entities[2].Kind != 0x20 || m.Entities[2].ControlRaw[1] != 2 || m.Entities[1].Pose.Height != 350 {
		t.Fatal("source AX leader quirk or same-visit redispatch changed")
	}
}
func TestRetaskActualSameCallLocal(t *testing.T) {
	s, in, _ := followerFixture(t)
	m := s.init
	m.Entities[0].Pose.Height = 0
	primary := &m.Targets.Records[2]
	binary.LittleEndian.PutUint16(primary[32:], 2)
	binary.LittleEndian.PutUint32(primary[34:], m.Entities[0].Pose.H0)
	binary.LittleEndian.PutUint32(primary[38:], m.Entities[0].Pose.H1)
	m.Entities[17].Kind = 0x15
	m.Entities[17].Participation = 0
	m.Entities[17].Object.Raw[0] = 0x10
	m.Entities[22].Kind = 0x15
	m.Entities[22].Object.Raw[0] = 0x20
	binary.LittleEndian.PutUint16(m.Targets.Records[3][32:], 3)
	binary.LittleEndian.PutUint16(m.Targets.Records[3][52:], 1)
	m.Targets.Records[4][61] = 0xa5
	s.AdvanceObjectives(in)
	if m.Targets.ProgressFailure != "" || m.Entities[17].Kind != 0x41 || m.Entities[22].Kind != 0x15 || m.Targets.Records[3][61] != 0xa5 {
		t.Fatal("defined BP-4 wrote secondary row or lost full62", m.Targets.ProgressFailure)
	}
	// A final exhausted scan must replace an earlier valid same-call provenance.
	trace := primaryScanTrace{executed: true, finalIndex: 17}
	s.matchCargoObject(0, &trace)
	if !trace.executed || trace.finalIndex != 80 {
		t.Fatal("exhausted scan did not override previous local")
	}
}

func TestReservedPoseCardinalsAndPickupStrictBounds(t *testing.T) {
	for _, tc := range []struct {
		heading          uint16
		dx, dz           int32
		offsetX, offsetZ int32
	}{{0, 0, -16, 0, -256}, {0x4000, 16, 0, 256, 0}, {0x8000, 0, 16, 0, 256}, {0xc000, -16, 0, -256, 0}} {
		s, _, _ := followerFixture(t)
		m := s.init
		p := m.Entities[0].Pose
		e := &m.Entities[5]
		e.Participation = 1
		e.Pose.H0 = p.H0 + uint32(tc.dx)
		e.Pose.H1 = p.H1 + uint32(tc.dz)
		e.Pose.Angles[1] = tc.heading
		basis := m.Entities[1].Pose
		if err := s.advanceReservedPose(5); err != nil {
			t.Fatal(err)
		}
		if e.Pose.Angles[1] != tc.heading || e.Pose.H0 != basis.H0+uint32(tc.offsetX) || e.Pose.H1 != basis.H1+uint32(tc.offsetZ) {
			t.Fatalf("source cardinal %+v gave %+v", tc, e.Pose)
		}
	}
	for _, tc := range []struct {
		distance      uint32
		height, speed int16
		pick          bool
	}{{127, 15, 15, true}, {128, 15, 15, false}, {127, 16, 15, false}, {127, 15, 16, false}, {127, -1, -1, true}} {
		s, in, _ := followerFixture(t)
		m := s.init
		e := &m.Entities[5]
		e.Participation = 1
		e.Pose = m.Entities[0].Pose
		e.Pose.H0 += tc.distance * 16
		m.Entities[0].Pose.Height = uint16(tc.height)
		in.PlayerIndicatedSpeed = tc.speed
		m.Stores.Ships[0].Flags = 6 | 0x18 // Reserve pickup has NO ordinary upper-cargo-empty gate.
		s.advancePlayerReservePickup(in)
		if (e.Kind == 0x41) != tc.pick || m.Stores.Ships[0].Flags != 6|0x18 {
			t.Fatal("strict signed pickup/cargo ownership", tc, e.Kind, m.Stores.Ships[0].Flags)
		}
	}
}

func TestFollowerFuelUnsupportedGuardAndPositiveDepletion(t *testing.T) {
	s, in, sys := followerFixture(t)
	m := s.init
	sys.Fuel[1] = 2
	m.Stores.Damage[1][3] = 1
	sys.AdvanceWithFollowers(true, 16, s.FollowerFuelContext(in.Random))
	if sys.Fuel[1] != 0 || m.Stores.Damage[1][9] != 0 || m.FollowerFailureKnown[1] {
		t.Fatal("partial603 old2->0 created failure on positive branch")
	}
	sys.Countdown = 0
	m.Entities[1].Kind = 0x24
	sys.AdvanceWithFollowers(true, 16, s.FollowerFuelContext(in.Random))
	if sys.UnsupportedFollowerEmptyBurn != 2 || m.Stores.Damage[1][9] != 0 {
		t.Fatal("unclosed caller lost its explicit sticky guard")
	}
}

func TestFollowerGroundContactNextVisitInitializesWithoutCallback(t *testing.T) {
	s, in, sys := followerFixture(t)
	m := s.init
	e := &m.Entities[1]
	m.FollowerFailureKnown[1] = true
	e.Kind = 0x26
	e.Participation = 1
	e.Pose.Height = 1
	m.Stores.Damage[1][9] = 2
	if _, err := s.AdvanceFollowers(in, sys); err != nil {
		t.Fatal(err)
	}
	if e.Participation != 0 || e.Kind != 0x20 || m.Entities[5].Participation != 1 {
		t.Fatal("contact did not publish P0 and same-phase reserve")
	}
	if _, err := s.AdvanceFollowers(in, sys); err != nil {
		t.Fatal(err)
	}
	if e.Participation != 1 || e.Pose.Height != 2 || e.RawMotion != 0 || m.Stores.Damage[1][9] != 0 {
		t.Fatal("sourceP0 reinitialized incorrectly or received a second callback")
	}
}
