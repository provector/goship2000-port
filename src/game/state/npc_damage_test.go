package state

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func nPCFixture(t *testing.T, selector byte, words ...uint16) (*InitializedMission, NPCHitContext) {
	t.Helper()
	s := &RuntimeStores{FreshShipPadding: true, Ships: make([]RuntimeAircraft, 5), Damage: make([][10]byte, 5)}
	m := &InitializedMission{PlayerRow: 0, Switches: 3, Stores: s, Entities: make([]InitializedEntity, 5)}
	for i := range s.Ships {
		p := byte(10 + i)
		s.Ships[i] = RuntimeAircraft{Flags: selector, Fuel: 99, Proficiency: &p}
		m.Entities[i].RuntimeRow, m.Entities[i].SerializedAircraftSelector = i, int(selector)
	}
	w := RuntimeWeapon{CompactID: 2}
	w.Raw[21] = 99
	s.Definitions = []RuntimeWeapon{w}
	stat := make([]byte, 80)
	stat[0x1d], stat[0x3b] = 3, 6
	if err := s.EnableNPCDamage(stat); err != nil {
		t.Fatal(err)
	}
	n := 0
	c := NPCHitContext{VictimRow: 1, Random: func() uint16 {
		if n >= len(words) {
			t.Fatalf("unjustified extra NPC RNG draw%d beyond original fixture%v", n, words)
		}
		v := words[n]
		n++
		return v
	}}
	return m, c
}

func TestNPCFullMaskDamageAndQuality(t *testing.T) {
	for _, tc := range []struct {
		q       uint16
		words   []uint16
		applied bool
	}{{0, []uint16{75}, false}, {0, []uint16{76, 0}, true}, {1, []uint16{37}, false}, {1, []uint16{38, 0}, true}, {2, []uint16{0}, true}, {3, []uint16{0}, true}} {
		m, c := nPCFixture(t, 2, tc.words...)
		m.Switches = tc.q
		out, err := m.ApplyAcceptedNPCHit(c)
		if err != nil || out.Applied != tc.applied || out.RNGCalls != uint32(len(tc.words)) {
			t.Fatal(tc, out, err)
		}
		want := [12]byte{}
		health := uint16(255)
		if tc.applied {
			want[0], health = 1, 242
			if out.Bucket != 0 || out.ReturnCode != 1 || !out.NotifyDamage || out.DirectWreck {
				t.Fatal(out)
			}
		}
		if got := m.Stores.Damage12(1); got != want || EncodedDamageHealth(got) != health || m.Stores.Damage[1] != [10]byte{} {
			t.Fatal("NPC mask must not mutate player/physical state", got, health, m.Stores.Damage)
		}
	}
}

func TestNPCProtectionTerminalAndFreshOwner(t *testing.T) {
	for _, mode := range []string{"Training", "terminal", "player", "missing-owner", "selector"} {
		m, c := nPCFixture(t, 2)
		c.Random = func() uint16 { t.Fatal("early NPC guard consumed RNG"); return 0 }
		switch mode {
		case "Training":
			m.Switches |= 0x400
		case "terminal":
			m.Stores.Damage[1][9] = 1
		case "player":
			c.VictimRow = 0
		case "missing-owner":
			m.Stores.NPCDamage = nil
		case "selector":
			m.Entities[1].SerializedAircraftSelector = 3
		}
		out, err := m.ApplyAcceptedNPCHit(c)
		if out.Applied || out.RNGCalls != 0 || (err != nil) != (mode == "player" || mode == "missing-owner" || mode == "selector") {
			t.Fatal(mode, out, err)
		}
	}
}

func TestNPCBucketRerollsAndNoPlayerEffects(t *testing.T) {
	for _, tc := range []struct {
		selector byte
		words    []uint16
		bucket   int16
	}{
		{2, []uint16{190, 0}, 0},          // NPC crack bucket must reroll, never draw overlay coordinates.
		{5, []uint16{102, 0}, 0},          // single-engine remap6->10 then reroll.
		{3, []uint16{172, 0}, 0},          // engine remap9->10 then reroll.
		{0, []uint16{0, 190, 190, 44}, 2}, // Apache extra selection on every attempt.
		{1, []uint16{10, 44}, 2},
	} {
		m, c := nPCFixture(t, tc.selector, tc.words...)
		out, err := m.ApplyAcceptedNPCHit(c)
		if err != nil || out.Bucket != tc.bucket || out.RNGCalls != uint32(len(tc.words)) || m.Stores.Damage12(0) != [12]byte{} {
			t.Fatal(tc, out, err)
		}
	}
	// Entire-byte INC, and only low2==2 rejects during selection.
	m, c := nPCFixture(t, 2, 0)
	m.Stores.NPCDamage.Masks[1][0] = 0x80
	out, err := m.ApplyAcceptedNPCHit(c)
	if err != nil || !out.Applied || m.Stores.NPCDamage.Masks[1][0] != 0x81 {
		t.Fatal(out, err, m.Stores.Damage12(1))
	}
}

func TestNPCFuelAndCriticalLiteralAliases(t *testing.T) {
	m, c := nPCFixture(t, 2, 95)
	m.Stores.Damage[1][3] = 1 // whole603 becomes2, direct replacement belongs to combat.
	out, err := m.ApplyAcceptedNPCHit(c)
	if err != nil || !out.DirectWreck || out.ReturnCode != 6 || m.Stores.Ships[1].Fuel != 98 || m.Stores.Damage[1][9] != 0 {
		t.Fatal(out, err, m.Stores.Damage[1], m.Stores.Ships[1].Fuel)
	}
	// Existing physical602 critical: selecting mask bucket0 reads SHIP0 skill,
	// not victim1. Equality chooses6091; a greater draw chooses6092.
	for _, roll := range []uint16{55, 56} {
		m, c := nPCFixture(t, 2, 0, roll)
		m.Stores.Damage[1][2] = 2
		*m.Stores.Ships[0].Proficiency = 55 // changed after launch, so use the live byte.
		*m.Stores.Ships[1].Proficiency = 127
		out, err := m.ApplyAcceptedNPCHit(c)
		want := byte(1)
		if roll > 55 {
			want = 2
		}
		if err != nil || out.RNGCalls != 2 || m.Stores.Damage[1][9] != want {
			t.Fatal("literal bucket alias/equality", roll, out, err, m.Stores.Damage[1])
		}
	}
	for _, tc := range []struct {
		value  uint16
		before int
		roll   uint16
		want   byte
	}{
		{102, 4, 6, 1}, // full bucket6 -> STAT3B=6.
		{102, 4, 7, 2},
		{172, 7, 99, 1}, // full bucket9 -> actual compact2[21]=99.
		{172, 7, 100, 2},
	} {
		m, c := nPCFixture(t, 2, tc.value, tc.roll)
		m.Stores.Damage[1][tc.before] = 1
		// Make an actual602 critical for the STAT case;607 itself becomes
		// critical in the compact2 case.
		if tc.value == 102 {
			m.Stores.Damage[1][2] = 2
		}
		out, err := m.ApplyAcceptedNPCHit(c)
		if err != nil || out.RNGCalls != 2 || m.Stores.Damage[1][9] != tc.want {
			t.Fatal(tc, out, err, m.Stores.Damage12(1))
		}
	}
	m, c = nPCFixture(t, 2, 128)
	m.Stores.Damage[1][5] = 1 // physical605 index5 -> full bucket7.
	out, err = m.ApplyAcceptedNPCHit(c)
	if err != nil || out.Bucket != 7 || out.RNGCalls != 1 || m.Stores.Damage[1][9] != 2 {
		t.Fatal("tail critical descent", out, err, m.Stores.Damage[1])
	}
}

func TestNPCStationDamageForeignSHIP3AndNoMass(t *testing.T) {
	for _, port := range []bool{true, false} {
		value := uint16(51)
		if !port {
			value = 146
		}
		m, c := nPCFixture(t, 2, value)
		m.Stores.Ships[1].Stations[0] = RuntimeStation{CompactID: 2, Mode: 3, Ammunition: 101}
		m.Stores.Ships[3].Stations[0] = RuntimeStation{CompactID: 2, Mode: 3, Ammunition: 77}
		out, err := m.ApplyAcceptedNPCHit(c)
		if err != nil || !out.Applied || m.Stores.Ships[1].Stations[0].Ammunition != 101 || m.Stores.Ships[3].Stations[0].Ammunition != 38 {
			t.Fatal(port, out, err, m.Stores.Ships)
		}
		// Destroying the opposite side then clears victim's remaining mode3
		// ammunition rather than halving SHIP3 again.
		other := uint16(146)
		if !port {
			other = 51
		}
		c.Random = func() uint16 { return other }
		out, err = m.ApplyAcceptedNPCHit(c)
		if err != nil || m.Stores.Ships[1].Stations[0].Ammunition != 0 || m.Stores.Ships[3].Stations[0].Ammunition != 38 {
			t.Fatal(out, err, m.Stores.Ships)
		}
	}
}

func TestNPCOptionalHistoricalPayloadAndAliasGuard(t *testing.T) {
	old := RuntimeStores{Damage: [][10]byte{{}}}
	b, err := json.Marshal(old)
	if err != nil || strings.Contains(string(b), "NPCDamage") {
		t.Fatal("new optional owner changed old checkpoint", string(b), err)
	}
	m, c := nPCFixture(t, 2, 172, 10)
	m.Stores.Damage[1][7] = 1
	m.Stores.NPCDamage.Known[9] = false
	before := m.Stores.Damage12(1)
	out, err := m.ApplyAcceptedNPCHit(c)
	if err != nil || out.Applied || !strings.Contains(out.Limit, "bucket9") || out.RNGCalls != 2 || m.Stores.Damage12(1) != before {
		t.Fatal("unknown literal alias cannot be replaced by zero/skill", out, err, m.Stores.Damage12(1))
	}
	encoded, err := json.Marshal(m.Stores)
	if err != nil {
		t.Fatal(err)
	}
	var restored RuntimeStores
	if err := json.Unmarshal(encoded, &restored); err != nil || !reflect.DeepEqual(restored.NPCDamage, m.Stores.NPCDamage) {
		t.Fatal("full masks/alias provenance lost across snapshot", err)
	}
}
