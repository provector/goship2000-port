package state

import (
	"encoding/binary"
	"encoding/json"
	"reflect"
	"testing"
)

func TestVictimWingSignedStoresAndLegacy(t *testing.T) {
	for _, v := range []int{0, 1, 3} {
		for _, port := range []bool{true, false} {
			for _, quantity := range []int16{0, 19, 21, -1, -21} {
				for _, modern := range []bool{false, true} {
					m, c := damageFixture(t, 2)
					m.Stores.Ships = make([]RuntimeAircraft, 5)
					m.Stores.Damage = make([][10]byte, 5)
					m.Entities = make([]InitializedEntity, 5)
					for i := range m.Stores.Ships {
						skill := byte(127)
						m.Stores.Ships[i] = RuntimeAircraft{Flags: 2, Fuel: 99, Proficiency: &skill}
						m.Entities[i] = InitializedEntity{RuntimeRow: i, SerializedAircraftSelector: 2}
						m.Stores.Ships[i].Stations[1] = RuntimeStation{CompactID: 2, Mode: 3, Ammunition: 25}
					}
					m.Stores.Ships[v].Stations[1].Ammunition = quantity
					m.Stores.VictimWingStores = modern
					m.Stores.FreshShipPadding = true
					w := RuntimeWeapon{CompactID: 2}
					binary.LittleEndian.PutUint16(w.Raw[11:], 3)
					m.Stores.Definitions = []RuntimeWeapon{w}
					if e := m.Stores.EnableNPCDamage(make([]byte, 80)); e != nil {
						t.Fatal(e)
					}
					before := append([]RuntimeAircraft(nil), m.Stores.Ships...)
					m.Stores.Blueprint = append([]RuntimeAircraft(nil), before...)
					word := uint16(51)
					if !port {
						word = 146
					}
					draws := 0
					rng := func() uint16 { draws++; return word }
					if v == 0 {
						c.Random = rng
						if _, e := m.ApplyAcceptedPlayerHit(c); e != nil {
							t.Fatal(e)
						}
					} else {
						if _, e := m.ApplyAcceptedNPCHit(NPCHitContext{VictimRow: v, Random: rng}); e != nil {
							t.Fatal(e)
						}
					}
					if draws != 1 {
						t.Fatal("draw count", draws)
					}
					for i := range m.Stores.Ships {
						want := before[i].Stations[1].Ammunition
						if modern && i == v || !modern && i == 3 {
							want >>= 1
						}
						if m.Stores.Ships[i].Stations[1].Ammunition != want {
							t.Fatal(v, port, quantity, modern, i, want, m.Stores.Ships[i])
						}
					}
					if v == 0 {
						want := int16(8000)
						if modern || v == 3 {
							want -= (quantity - (quantity >> 1)) * 3
						}
						if *c.MassTerm != want {
							t.Fatal("victim mass", quantity, modern, *c.MassTerm, want)
						}
					}
					// Opposite-side hit empties own stores and never halves another row.
					if port {
						word = 146
					} else {
						word = 51
					}
					priorOther := m.Stores.Ships[(v+1)%5]
					if v == 0 {
						if _, e := m.ApplyAcceptedPlayerHit(c); e != nil {
							t.Fatal(e)
						}
					} else {
						if _, e := m.ApplyAcceptedNPCHit(NPCHitContext{VictimRow: v, Random: rng}); e != nil {
							t.Fatal(e)
						}
					}
					if m.Stores.Ships[v].Stations[1].Ammunition != 0 || !reflect.DeepEqual(priorOther, m.Stores.Ships[(v+1)%5]) {
						t.Fatal("second side ownership")
					}
					d := m.Stores.Damage[v]
					if v == 0 {
						d = c.Systems.Damage
					}
					if e := m.Stores.Refill(v, &d); e != nil {
						t.Fatal(e)
					}
					if m.Stores.Ships[v].Stations[1].Ammunition != 0 {
						t.Fatal("service healed wing")
					}
					raw, _ := json.Marshal(m.Stores)
					var restored RuntimeStores
					if e := json.Unmarshal(raw, &restored); e != nil || restored.VictimWingStores != modern {
						t.Fatal("checkpoint policy", e)
					}
				}
			}
		}
	}
}

func TestVictimWingComposesWithoutCriticalReadChange(t *testing.T) {
	m, c := nPCFixture(t, 2, 51, 127)
	m.Stores.VictimWingStores = true
	m.Stores.Damage[1][0] = 2
	*m.Stores.Ships[1].Proficiency = 127
	*m.Stores.Ships[3].Proficiency = 25
	m.Stores.Ships[1].Stations[0] = RuntimeStation{CompactID: 2, Mode: 3, Ammunition: 21}
	m.Stores.Ships[3].Stations[0] = RuntimeStation{CompactID: 2, Mode: 3, Ammunition: 25}
	r, e := m.ApplyAcceptedNPCHit(c)
	if e != nil || r.RNGCalls != 2 || m.Stores.Ships[1].Stations[0].Ammunition != 10 || m.Stores.Ships[3].Stations[0].Ammunition != 25 || m.Stores.Damage[1][9] != 2 {
		t.Fatal("R6 write/R5 threshold", r, e, m.Stores.Damage[1])
	}
}

func TestVictimWingActualInitializedLoadout(t *testing.T) {
	a := initializationAssets(t)
	rows, _, _ := legalWeaponRows(t)
	m, e := GenerateMission(a.Missions.Generation, MissionInputs{MissionType: MissionFlight, Theatre: TheatrePersianGulf, Rank: RankCaptain, Seed: 1})
	if e != nil {
		t.Fatal(e)
	}
	stores, e := BuildRuntimeStores(a.Missions, m, rows, true)
	if e != nil {
		t.Fatal(e)
	}
	stores.VictimWingStores = true
	v, c := damageFixture(t, 0, 51, 51)
	v.Stores = stores
	v.Entities[0].SerializedAircraftSelector = int(stores.Ships[0].Flags & 7)
	before := append([]RuntimeAircraft(nil), stores.Ships...)
	if _, e = v.ApplyAcceptedPlayerHit(c); e != nil {
		t.Fatal(e)
	}
	for i, st := range before[0].Stations {
		if st.CompactID > 0 && st.Mode == 3 && stores.Ships[0].Stations[i].Ammunition != st.Ammunition>>1 {
			t.Fatal("real paired station", i)
		}
	}
	if !reflect.DeepEqual(before[1:], stores.Ships[1:]) {
		t.Fatal("foreign aircraft changed")
	}
}
