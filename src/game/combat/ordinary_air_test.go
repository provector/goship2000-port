package combat

import "testing"

func TestOrdinaryHelicopterSourceInitialMotionAndFormation(t *testing.T) {
	s, in := groundFixture(t)
	e := &s.init.Entities[10]
	leader := &s.init.Entities[9]
	e.Kind, e.Participation = 0, 0
	leader.Pose.Height = 1337
	if err := s.advanceOrdinaryHelicopter(10, in); err != nil {
		t.Fatal(err)
	}
	if e.Participation != 1 || e.RawMotion != 40 || leader.RawMotion != 40 || e.Pose.Height != 1337 || e.Pose.H0 != leader.Pose.H0+100 {
		t.Fatal("B05A..B1AA actual P0", e, leader)
	}
	e.Pose.H0 = leader.Pose.H0
	draws := 0
	in.Random = func() uint16 {
		draws++
		if draws == 1 {
			return 7
		}
		return 0
	}
	if err := s.advanceOrdinaryHelicopter(10, in); err != nil {
		t.Fatal(err)
	}
	if draws != 2 || e.Pose.H0 != leader.Pose.H0+25 || e.Pose.H1 != leader.Pose.H1-1 || e.Pose.Height != 1337 {
		t.Fatal("B4D7..B5C3 two draws/SAR2/inherited height", draws, e)
	}
}

func TestOrdinaryAircraftActualObserverMovementAndLatch(t *testing.T) {
	s, in := groundFixture(t)
	in.DF = 1
	e := &s.init.Entities[10]
	e.Kind, e.Participation = 1, 0
	if err := s.advanceOrdinaryAircraft(10, in); err != nil {
		t.Fatal(err)
	}
	if e.Pose.Height != 1024 || e.RawMotion != 400 || e.Participation != 1 {
		t.Fatal("B6F2 initializer", e)
	}
	e.Pose.H0, e.Pose.H1 = 262144, 262144
	player := &s.init.Entities[s.init.PlayerRow]
	player.Pose.H0, player.Pose.H1, player.Pose.Height = 262144, 278144, 896
	in.Random = func() uint16 { t.Fatal("B6E0 no-attack policy consumed RNG"); return 0 }
	if err := s.advanceOrdinaryAircraft(10, in); err != nil {
		t.Fatal(err)
	}
	if e.Pose.H0 != 262144 || e.Pose.H1 != 262184 || e.Pose.Height != 1024 || e.Pose.Angles[0] != 0 {
		t.Fatal("actual593C straight movement", e)
	}
	// CONTACT bit2 identifies observer2; AB9 intentionally points elsewhere.
	in.Target = 9
	live := in.Contacts
	in.ContactState = &live
	live[10].Flags = 2 | 2<<3 // previous actor wrote live contact after snapshot
	s.init.Entities[2].Pose = player.Pose
	player.Pose.H1 = 240000   // observer2 and player now have independent sightlines
	e.Pose.Angles[1] = 0x4000 // exact no-turn equality; wanted angle0
	if err := s.advanceOrdinaryAircraft(10, in); err != nil {
		t.Fatal(err)
	}
	if e.DynamicRaw[4] != 0 || e.Pose.Angles[1] != 0x3c00 || e.Pose.Angles[2] != 0x1000 {
		t.Fatal("B94F unsigned strict greater-than4000", e)
	}
	e.Pose.Angles[1] = 0x4001
	if err := s.advanceOrdinaryAircraft(10, in); err != nil {
		t.Fatal(err)
	}
	if e.DynamicRaw[4] != 1 {
		t.Fatal("B99F source home latch", e)
	}
}
func TestOrdinaryHelicopterQueuedTerrainAndNoAttackArm(t *testing.T) {
	s, in := groundFixture(t)
	e := &s.init.Entities[10]
	e.Kind = 0
	e.RawMotion = 40
	s.route.Queued[10] = 0
	old := e.Pose
	if err := s.advanceOrdinaryHelicopter(10, in); err != nil {
		t.Fatal(err)
	}
	h, err := s.assets.PairedTerrainHeight(s.init.World, deathQuery(e.Pose), deathQuery(old))
	if err != nil {
		t.Fatal(err)
	}
	if e.Pose.Height != uint16(h+256) || s.route.Queued[10] != 0 {
		t.Fatal("B2F1..B38B moved-height reader", e, h)
	}
	e.Pose.H0, e.Pose.H1 = 262145, 262144
	priorHeight := e.Pose.Height
	if err := s.advanceOrdinaryHelicopter(10, in); err != nil {
		t.Fatal(err)
	}
	if e.Pose.H0 != 262144 || e.Pose.H1 != 262144 || e.Pose.Height != priorHeight || s.route.Queued[10] != -1 {
		t.Fatal("B272 arrival does not recalculate height", e)
	}
	e.DynamicRaw[2] = 0
	e.Pose.Angles[2] = 0xff00
	old = e.Pose
	in.Random = func() uint16 { t.Fatal("A070=false ungrouped arm consumed draw"); return 0 }
	if err := s.advanceOrdinaryHelicopter(10, in); err != nil {
		t.Fatal(err)
	}
	old.Angles[2] = 0xff80
	if e.Pose != old {
		t.Fatal("B697 damping only", e.Pose, old)
	}
}
func TestOrdinaryHelicopterFinitePathAndDeath(t *testing.T) {
	s, in := groundFixture(t)
	e := &s.init.Entities[10]
	e.Kind, e.Participation = 0, 0
	s.init.Entities[9].Participation = 0 // actual constructor enters header P0 before child P0
	in.Random = func() uint16 { return 4 }
	for i := 0; i < 300; i++ {
		if _, err := s.advanceLogisticsFollowers(in, nil); err != nil {
			t.Fatal(i, err)
		}
		if s.init.Entities[9].Participation == 4 {
			break
		}
	}
	if s.init.Entities[9].Participation != 4 || e.DynamicRaw[2] != 0 {
		t.Fatal("finite actual PATH did not terminate", e, s.init.Entities[9])
	}
	e.Participation = 2
	if err := s.advanceOrdinaryHelicopter(10, in); err != nil {
		t.Fatal(err)
	}
	if e.Pose.Angles[0] != 0xfe00 {
		t.Fatal("B6A7 uses source29E8 fall", e)
	}
}
