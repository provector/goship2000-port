package state

import (
	"bytes"
	"gunship2000-assets/internal/testfixture"
	"os"
	"reflect"
	"testing"
)

func TestRuntimeCompactDefinitionSourceFields(t *testing.T) {
	// Hand operands: WSHD0x0e03 becomes0x0e01;2 MINIPOD mounts at1500
	// rounds/mount become3000. This is not a mass/quantity formula change.
	w := make([]byte, 214)
	RowPut(w, 2, 1500)
	RowPut(w, 4, 0x0e03)
	copy(w[17:25], "MINIPOD")
	if runtimeFlags(w) != 0xe01 {
		t.Fatal("combined flag transform")
	}
	r := RuntimeStores{Ships: []RuntimeAircraft{{Flags: 0xa6}}, Blueprint: []RuntimeAircraft{{Flags: 0x25, Stations: [4]RuntimeStation{{Present: true, CompactID: 2, Ammunition: 11, Mode: 3}}, Fuel: 100}}}
	damage := [10]byte{0, 2, 1, 1, 1, 1, 0, 1, 1, 1}
	if err := r.Refill(0, &damage); err != nil {
		t.Fatal(err)
	}
	if r.Ships[0].Flags != 0xa5 || r.Ships[0].Stations[0].Ammunition != 5 || damage[9] != 0 || damage[2] != 1 || damage[1] != 2 {
		t.Fatal(r, damage)
	}
}

func TestRefillDamageSource601606(t *testing.T) {
	for _, tc := range []struct {
		mode        byte
		left, right byte
		want        int16
	}{{3, 2, 0, 5}, {3, 0, 2, 5}, {3, 2, 2, 0}, {2, 2, 0, 0}, {1, 2, 0, 11}, {1, 0, 2, 0}, {2, 0, 2, 11}, {3, 1, 1, 11}} {
		r := RuntimeStores{Ships: []RuntimeAircraft{{}}, Blueprint: []RuntimeAircraft{{Stations: [4]RuntimeStation{{CompactID: 2, Ammunition: 11, Mode: tc.mode}}}}}
		d := [10]byte{}
		d[1], d[6] = tc.left, tc.right
		if err := r.Refill(0, &d); err != nil {
			t.Fatal(err)
		}
		if r.Ships[0].Stations[0].Ammunition != tc.want {
			t.Fatal(tc, r.Ships[0])
		}
	}
}

func TestRuntimeStoresAcceptedPathPrepared(t *testing.T) {
	a := initializationAssets(t)
	for id := 0; id < 8; id++ {
		m, e := GenerateMission(a.Missions.Generation, MissionInputs{MissionType: MissionTraining, Theatre: TheatreCentralEurope, Rank: RankBrigadierGeneral, PilotName: "Stores", Seed: 1})
		if e != nil {
			t.Fatal(e)
		}
		row, e := InitializeReadinessRow(a.Missions, m, id)
		if e != nil {
			t.Fatal(e)
		}
		// STINGER0 and SIDEWNDR1 are valid raw IDs. Install both on a bounded
		// synthetic accepted row to check ID domains, independent of eligibility.
		row[3], row[4] = 0, 1
		RowPut(row[:], 12, 2)
		RowPut(row[:], 14, 1)
		rows := [][36]byte{row}
		before := append([][36]byte(nil), rows...)
		got, e := BuildRuntimeStores(a.Missions, m, rows)
		if e != nil {
			t.Fatal(id, e)
		}
		if !reflect.DeepEqual(rows, before) {
			t.Fatal("accepted rows mutated")
		}
		found := [2]bool{}
		for _, s := range got.Ships[0].Stations {
			if s.Present && (s.RawWSHDID == 0 || s.RawWSHDID == 1) {
				found[s.RawWSHDID] = true
				if s.CompactID < 2 || s.ShortName == "" {
					t.Fatal(s)
				}
			}
		}
		if !found[0] || !found[1] {
			t.Fatal(id, got.Ships[0])
		}
		if int8(row[2]) < 0 && (got.Ships[0].Stations[0].CompactID != 0 || got.Ships[0].Stations[0].Present || got.Ships[0].Stations[1].RawWSHDID != 0) {
			t.Fatal("compact writer's absent cannon0 must retain slot0 before real STINGER slot1", id, got.Ships[0])
		}
	}
}

func TestRuntimeStoresLaterWriterAndScaledPod(t *testing.T) {
	a := initializationAssets(t)
	bank, e := os.ReadFile(testfixture.Path("installation/GS2000.DAT"))
	if e != nil {
		t.Fatal(e)
	}
	// Independently retained catalogue range: WSHD13 payload file35809.
	// ID13/rank1, multiplier1500 and flags0e03 are original little-endian bytes.
	if !bytes.Equal(bank[35809:35815], []byte{13, 1, 0xdc, 5, 3, 0x0e}) {
		t.Fatal("source MINIPOD WSHD witness changed")
	}
	b, e := os.ReadFile(testfixture.Path("installation/GS.GS2"))
	if e != nil {
		t.Fatal(e)
	}
	for ip, w := range map[int][]byte{0x858: {0xc6, 0x87, 0xea, 0xba, 0}, 0x354: {0x80, 0x7e, 0xdc, 0, 0x7c, 0x1b}, 0x2f4: {0xc6, 0x46, 0xdf, 0}, 0x3eb: {0x80, 0x7a, 0xdd, 0, 0x7e, 0x35}} {
		if !bytes.Equal(b[0x32e70+ip:][:len(w)], w) {
			t.Fatalf("GS1c89 later writer%04x", ip)
		}
	}
	m, e := GenerateMission(a.Missions.Generation, MissionInputs{MissionType: MissionTraining, Theatre: TheatreCentralEurope, Rank: RankBrigadierGeneral, Seed: 1})
	if e != nil {
		t.Fatal(e)
	}
	row, e := InitializeReadinessRow(a.Missions, m, 2)
	if e != nil {
		t.Fatal(e)
	}
	row[3] = 13
	RowPut(row[:], 12, 2)
	r, e := BuildRuntimeStores(a.Missions, m, [][36]byte{row})
	if e != nil {
		t.Fatal(e)
	}
	pod := r.Ships[0].Stations[1]
	if !pod.Present || pod.RawWSHDID != 13 || pod.Ammunition != 3000 || pod.Flags != 0x0e01 || pod.Mode != 3 {
		t.Fatal("source2*1500,0e03→0e01,dual mode3", pod)
	}
	if r.Ships[0].Stations[0].Ammunition != RowWord(row[:], 21) {
		t.Fatal("cannon rounds multiplied twice")
	}
	// EMPTY is compact1 and therefore passes03eb's selector>0 test. It
	// occupies a row even with zero ammo; it is not an absent physical bay.
	row[3] = 0
	RowPut(row[:], 12, 0)
	row[4] = 1
	RowPut(row[:], 14, 1)
	r, e = BuildRuntimeStores(a.Missions, m, [][36]byte{row})
	if e != nil {
		t.Fatal(e)
	}
	if r.Ships[0].Stations[1].CompactID != 1 || r.Ships[0].Stations[1].ShortName != "EMPTY" || r.Ships[0].Stations[2].RawWSHDID != 1 || !r.Ships[0].Stations[2].Present {
		t.Fatal("empty bay must reserve row before next mounted system", r.Ships[0].Stations)
	}
	// Class2 cargo remains header state; serializer02f4 removes it from ammo.
	for air, def := range a.Missions.AircraftDefinitions {
		if def[1] != 2 {
			continue
		}
		row, e = InitializeReadinessRow(a.Missions, m, air)
		if e != nil {
			t.Fatal(e)
		}
		row[5], row[35] = 20, 2
		RowPut(row[:], 16, 1)
		m.RawE278 = 1
		r, e = BuildRuntimeStores(a.Missions, m, [][36]byte{row})
		if e != nil {
			t.Fatal(e)
		}
		if r.Ships[0].Flags&0x18 != 0x10 {
			t.Fatal("squadron header flag", air, r.Ships[0].Flags)
		}
		for _, st := range r.Ships[0].Stations {
			if st.Present && st.RawWSHDID == 20 {
				t.Fatal("squadron cargo reported as ammunition")
			}
		}
	}
}
