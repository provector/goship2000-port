package combat

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestDevelopmentFormationRebaseUsesOnlySourceP0(t *testing.T) {
	s, _, sys := formationFixture(t)
	m := s.init
	m.Entities[0].Pose.H0 += 100000
	m.Entities[0].Pose.H1 -= 100000
	m.Entities[0].Pose.Height = 128
	m.Entities[4].ControlRaw[5], m.Entities[4].DynamicRaw[2] = 3, 0x11
	m.Offsets[1] = [][2]int16{{0, -400}}
	for i := 1; i < 5; i++ {
		m.Entities[i].DynamicRaw[4] = 33
		m.Entities[i].RawMotion = int16(10 + i)
	}
	stores, _ := json.Marshal(m.Stores)
	systems, _ := json.Marshal(sys)
	rng, credit, results := m.RandomState, s.Credit, s.Results
	n, err := s.StageDevelopmentFormation()
	if err != nil || n != 4 {
		t.Fatal(n, err)
	}
	base := m.Entities[0].Pose
	for i, off := range [][2]int16{{400, -200}, {-400, -200}, {0, -400}, {0, -800}} {
		e := m.Entities[i+1]
		if e.Pose.H0 != base.H0+uint32(int32(off[0])) || e.Pose.H1 != base.H1+uint32(int32(off[1])) || e.Pose.Height != 128 || e.Pose.Angles != base.Angles || e.Participation != 1 || e.Kind != 0x21 || e.RawMotion != int16(11+i) || e.DynamicRaw[4] != 0 || e.DynamicRaw[5] != 0 {
			t.Fatal("source P0 must initialize ascending actual leaders without P1", i+1, e)
		}
	}
	afterStores, _ := json.Marshal(m.Stores)
	afterSystems, _ := json.Marshal(sys)
	if string(stores) != string(afterStores) || string(systems) != string(afterSystems) || m.RandomState != rng || s.Credit != credit || s.Results != results {
		t.Fatal("fixture construction changed fuel/damage/RNG/credit/result owners")
	}
}

func TestDevelopmentFormationPreservesFailureAndRejectsAtomically(t *testing.T) {
	s, _, _ := formationFixture(t)
	m := s.init
	m.Entities[2].Participation = 2
	m.Stores.Damage[2][9], m.FollowerFailureKnown[2] = 1, true
	failed := m.Entities[2]
	m.Entities[0].Pose.H0 += 100000
	n, err := s.StageDevelopmentFormation()
	if err != nil || n != 3 || !reflect.DeepEqual(failed, m.Entities[2]) || !m.FollowerFailureKnown[2] || m.Stores.Damage[2][9] != 1 {
		t.Fatal("staging revived an actual failure", n, err, m.Entities[2])
	}
	for _, reason := range []string{"offset", "sticky", "carrier", "historical"} {
		s, _, _ = formationFixture(t)
		m = s.init
		m.Entities[0].Pose.H0 += 100000
		switch reason {
		case "offset":
			m.Entities[2].DynamicRaw[2] = 0
		case "sticky":
			m.FollowerLimits[2] = "preserved unsupported query"
		case "carrier":
			m.Switches, m.Entities[0].Pose.Height = 0x1000, 0x16a
		case "historical":
			s.SetFormationEnabled(false)
		}
		before := append([]byte(nil), mustStageJSON(t, m)...)
		n, err := s.StageDevelopmentFormation()
		if reason == "historical" {
			if err != nil || n != 0 {
				t.Fatal("historical interpretation changed", n, err)
			}
		} else if err == nil || n != 0 {
			t.Fatal("unsupported staged constructor was admitted", reason, n, err)
		}
		if string(before) != string(mustStageJSON(t, m)) {
			t.Fatal("failed staging partially published or cleared state", reason)
		}
	}
}

func mustStageJSON(t *testing.T, value any) []byte {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
