package state

import (
	"encoding/json"
	"reflect"
	"testing"
)

func criticalFixture(t *testing.T, victim int, selector byte, words ...uint16) (*InitializedMission, NPCHitContext) {
	t.Helper()
	m, c := nPCFixture(t, selector, words...)
	c.VictimRow = victim
	m.Stores.VictimCriticalProficiency, m.Stores.VictimWingStores = true, true
	for i := range m.Entities {
		m.Entities[i].FlightSlot, m.Entities[i].Kind = i, 0x20
	}
	return m, c
}

func TestCritical536ConsumerCases(t *testing.T) {
	count := 0
	check := func(v, bucket int, own, other, compact, roll byte) {
		t.Helper()
		selection := [...]uint16{0, 26, 44, 51, 77, 95, 102, 128, 146, 172}
		m, c := criticalFixture(t, v, 2, selection[bucket], uint16(roll))
		for i := range m.Stores.Ships {
			*m.Stores.Ships[i].Proficiency = other
		}
		*m.Stores.Ships[v].Proficiency = own
		m.Stores.NPCDamage.BucketBytes[5], m.Stores.NPCDamage.BucketBytes[6] = other, other
		m.Stores.NPCDamage.BucketBytes[9] = compact
		m.Stores.Definitions[0].Raw[21] = compact
		critical := 2
		if bucket == 4 {
			critical = 0
		}
		m.Stores.Damage[v][critical] = 2
		out, err := m.ApplyAcceptedNPCHit(c)
		want := byte(1)
		if roll&127 > own {
			want = 2
		}
		if err != nil || !out.Applied || out.Limit != "" || out.RNGCalls != 2 || m.Stores.Damage[v][9] != want || out.Bucket != int16(bucket) {
			t.Fatal(v, bucket, own, other, compact, roll, out, err, m.Stores.Damage12(v), want)
		}
		count++
	}
	for v := 1; v <= 4; v++ {
		for _, own := range []byte{0, 25, 50, 126, 127, 128, 255} {
			for _, roll := range []byte{0, 25, 26, 50, 51, 126, 127} {
				for _, other := range []byte{0, 255} {
					check(v, 0, own, other, 99, roll)
				}
			}
		}
		for _, b := range []int{0, 1, 2, 3, 4, 6, 7, 8, 9} {
			for _, other := range []byte{0, 255} {
				for _, compact := range []byte{0, 255} {
					check(v, b, 25, other, compact, 50)
				}
			}
		}
	}
	if count != 536 {
		t.Fatal(count)
	}
}

func TestCriticalVictimValidationAndNoAliasRequirement(t *testing.T) {
	for _, invalid := range []string{"proficiency", "row", "slot", "kind", "selector"} {
		m, c := criticalFixture(t, 1, 2)
		c.Random = func() uint16 { t.Fatal("invalid victim consumed RNG"); return 0 }
		switch invalid {
		case "proficiency":
			m.Stores.Ships[1].Proficiency = nil
		case "row":
			m.Entities[1].RuntimeRow = 2
		case "slot":
			m.Entities[1].FlightSlot = 2
		case "kind":
			m.Entities[1].Kind = 1
		case "selector":
			m.Entities[1].SerializedAircraftSelector = -1
		}
		before, _ := json.Marshal(m)
		out, err := m.ApplyAcceptedNPCHit(c)
		after, _ := json.Marshal(m)
		if err == nil || out.RNGCalls != 0 || !reflect.DeepEqual(before, after) {
			t.Fatal(invalid, out, err)
		}
	}
	m, c := criticalFixture(t, 1, 2, 0, 26)
	*m.Stores.Ships[1].Proficiency = 25
	m.Stores.Damage[1][2] = 2
	// Neither an unknown compact alias nor incomplete unrelated SHIP rows is
	// an input to the selected victim's critical decision.
	m.Stores.NPCDamage.Known = [10]bool{}
	m.Stores.Ships[3].Proficiency = nil
	out, err := m.ApplyAcceptedNPCHit(c)
	if err != nil || !out.Applied || out.Limit != "" || m.Stores.Damage[1][9] != 2 {
		t.Fatal(out, err)
	}
}

func TestCriticalSelectorsClassesAndWingComposition(t *testing.T) {
	for selector := byte(0); selector < 8; selector++ {
		for kind := byte(0x20); kind <= 0x26; kind++ {
			for _, word := range []uint16{51, 146} {
				words := []uint16{word, 50}
				if selector < 2 {
					words = []uint16{190, word, 50}
				}
				m, c := criticalFixture(t, 3, selector, words...)
				m.Entities[3].Kind = kind
				*m.Stores.Ships[3].Proficiency = 25
				m.Stores.Damage[3][2] = 2
				m.Stores.Ships[3].Stations[1] = RuntimeStation{CompactID: 2, Mode: 3, Ammunition: -21}
				m.Stores.Ships[1].Stations[1] = RuntimeStation{CompactID: 2, Mode: 3, Ammunition: 41}
				out, err := m.ApplyAcceptedNPCHit(c)
				if err != nil || !out.Applied || out.RNGCalls != uint32(len(words)) || m.Stores.Damage[3][9] != 2 || m.Stores.Ships[3].Stations[1].Ammunition != -11 || m.Stores.Ships[1].Stations[1].Ammunition != 41 {
					t.Fatal(selector, kind, word, out, err)
				}
			}
		}
	}
}

func TestCriticalProficiencyInitializationRefillCheckpoint(t *testing.T) {
	m, c := criticalFixture(t, 1, 2, 0, 26)
	m.Stores.Blueprint = append([]RuntimeAircraft(nil), m.Stores.Ships...)
	crew := []FlightPilot{{SourceIndex: 0}, {SourceIndex: 1, Rank: RankWOC, AwardCounts: make([]int8, 8)}}
	if _, err := InitializeCrewPerformance(m.Stores, crew, 2); err != nil {
		t.Fatal(err)
	}
	*m.Stores.Ships[1].Proficiency = 127
	var damage [10]byte
	if err := m.Stores.Refill(1, &damage); err != nil || *m.Stores.Ships[1].Proficiency != 25 {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(m)
	var restored InitializedMission
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if !restored.Stores.VictimCriticalProficiency || *restored.Stores.Ships[1].Proficiency != 25 {
		t.Fatal("checkpoint lost runtime policy/skill")
	}
	restored.Stores.Damage[1][2] = 2
	out, err := restored.ApplyAcceptedNPCHit(c)
	if err != nil || !out.Applied || restored.Stores.Damage[1][9] != 2 {
		t.Fatal(out, err)
	}
}
