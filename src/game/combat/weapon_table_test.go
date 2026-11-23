package combat

import (
	"encoding/hex"
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"os"
	"testing"
)

func TestLegal17FollowerHellfireReleaseUpdateAndDamage(t *testing.T) {
	assets, err := assetdata.LoadFrontEndAssets(os.Getenv("GS2000_TEST_MENUS"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../../test-data/weapon-table/rows.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct{ Rows []string }
	if err = json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	rows := make([][36]byte, len(f.Rows))
	for i, h := range f.Rows {
		b, e := hex.DecodeString(h)
		if e != nil || len(b) != 36 {
			t.Fatal(e)
		}
		copy(rows[i][:], b)
	}
	stores, err := gamestate.BuildRuntimeStores(assets.Missions, &gamestate.MissionData{}, rows, true)
	if err != nil {
		t.Fatal(err)
	}
	s, in := fixture(t, 2)
	s.init.Stores = stores
	s.init.SafeGameplay = true
	for len(s.init.Entities) < 6 {
		i := len(s.init.Entities)
		e := s.init.Entities[1]
		e.RuntimeRow, e.SourceRow = i, i
		s.init.Entities = append(s.init.Entities, e)
	}
	actor, target := &s.init.Entities[4], &s.init.Entities[5]
	actor.Kind, actor.Pose.H0, actor.Pose.H1, actor.Pose.Angles[1] = 0x20, 0x40000, 0x40000, 0
	actor.ControlRaw[3], actor.ControlRaw[4] = 6, 1
	target.Pose.H0, target.Pose.H1 = 0x40000, 0x44000
	s.init.CrewPerformance = make([]gamestate.SourceCrewPerformance, 5)
	for i := range stores.Ships {
		skill := byte(127)
		stores.Ships[i].Proficiency = &skill
		s.init.CrewPerformance[i] = gamestate.SourceCrewPerformance{RawB: skill, SkillKnown: true}
	}
	draws := 0
	in.Random = func() uint16 { draws++; return 127 }
	in.E0 = true
	if err = s.advanceHealthyFollower(4, in); err != nil {
		t.Fatal(err)
	}
	p := &s.Slots[19]
	if p.Lifetime == 0 || p.Packed>>2 != 16 || p.Target != 5 || stores.Ships[4].Stations[2].Ammunition != 1 || draws != 1 {
		t.Fatal("actual upper-table release", *p, stores.Ships[4].Stations, draws)
	}
	before := p.Pose
	for i := 0; i < 5; i++ {
		if err = s.Step(in); err != nil {
			t.Fatal(err)
		}
	}
	if p.Lifetime == 0 || p.Pose == before || p.Packed>>2 != 16 {
		t.Fatal("projectile did not preserve/move actual upper definition", *p)
	}
	// The real impact consumer decodes that same packed definition; no
	// supplied damage flag or replacement definition is introduced.
	target.Participation = 1
	p.Pose = target.Pose
	p.Lifetime = 120
	p.Speed = 600
	if err = s.Step(in); err != nil {
		t.Fatal(err)
	}
	if len(events(s, "impact")) != 1 || len(events(s, "damage")) != 1 || draws != 2 {
		t.Fatal("upper definition did not reach actual impact/damage", p, draws, s.Events)
	}
	if err = stores.Refill(4, &stores.Damage[4]); err != nil || stores.Ships[4].Stations[2].Ammunition != 2 {
		t.Fatal("upper ammunition refill", err)
	}
}
