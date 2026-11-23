package state

import (
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func missionReadinessAssets(t *testing.T) *assetdata.MissionAssets {
	t.Helper()
	ready := os.Getenv("GS2000_TEST_MISSIONS")
	if ready == "" {
		ready = testfixture.Path("prepared-preflight")
	}
	if _, err := os.Stat(ready); err == nil {
		a, err := assetdata.LoadMissionAssets(ready)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	path := testfixture.Path("prepared-missions")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("source-backed prepared test witness absent")
	}
	raw, err := os.ReadFile(path + "/GS2000.DAT/FIELD.SCN.missions.json")
	if err != nil {
		t.Fatal(err)
	}
	var metadata assetdata.MissionMetadata
	if err = json.Unmarshal(raw, &metadata); err != nil {
		t.Fatal(err)
	}
	a := &assetdata.MissionAssets{Generation: &metadata.Generation, AircraftDefinitions: metadata.AircraftDefinitions, WeaponDefinitions: metadata.WeaponDefinitions}
	return a
}

func TestSourceReadinessManualVectors(t *testing.T) {
	a := missionReadinessAssets(t)
	var world *assetdata.MissionWorld
	for i := range a.Generation.Worlds {
		w := &a.Generation.Worlds[i]
		if w.Theatre == 1 && w.Suffix == 4 {
			world = w
		}
	}
	if world == nil {
		t.Fatal("original Training Gulf world missing")
	}
	sh := a.AircraftDefinitions[0]
	if sh[4] != 2 || sh[7] != 10 || sh[9] != 2 || RowWord(sh, 10) != 6700 || RowWord(sh, 12) != 2550 || world.THHD[0x1b] != 10 {
		t.Fatal("manual vector source identity changed")
	}
	for _, v := range []struct {
		temperature int8
		capacity    int16
	}{{68, 6135}, {100, 5515}, {20, 5665}} {
		m := &MissionData{SourceWorld: world, FlightTemplate: 3, Inputs: MissionInputs{MissionType: MissionTraining}, Conditions: MissionConditions{Temperature: v.temperature}}
		rows, err := PrepareReadinessRows(a, m)
		if err != nil {
			t.Fatal(err)
		}
		r := rows[0][:]
		if RowWord(r, 23) != v.capacity || RowWord(r, 25) != 4350 || RowWord(r, 27) != 2550 || RowWord(r, 29) != 2550 {
			t.Fatalf("temperature%d manual source vector mismatch: capacity%d total%d original%d remaining%d", v.temperature, RowWord(r, 23), RowWord(r, 25), RowWord(r, 27), RowWord(r, 29))
		}
		if RowWord(r, 21) != 1200 || RowWord(r, 12) != 8 || RowWord(r, 14) != 14 || RowWord(r, 8) != 140 {
			t.Fatal("descriptor or source fixed-pylon term differs")
		}
		if err = PreflightReadiness(rows); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSourceReadinessTheatreCapacityOffset(t *testing.T) {
	a := missionReadinessAssets(t)
	// Original DS:b62f-b614=1bh. Europe byte0dh is the P in EUROPE;
	// using that name byte accidentally removes80% of a Kiowa's base capacity.
	for _, vector := range []struct {
		theatre, sourceByte, capacity int
	}{{0, 0, 1630}, {1, 10, 1460}, {2, 10, 1460}, {3, 20, 1290}} {
		var world *assetdata.MissionWorld
		for i := range a.Generation.Worlds {
			if a.Generation.Worlds[i].Theatre == vector.theatre {
				world = &a.Generation.Worlds[i]
				break
			}
		}
		if world == nil || int(world.THHD[0x1b]) != vector.sourceByte {
			t.Fatalf("theatre%d source capacity byte differs", vector.theatre)
		}
		m := &MissionData{SourceWorld: world, FlightTemplate: 13, Inputs: MissionInputs{MissionType: MissionFlight}, Conditions: MissionConditions{Temperature: 59}}
		rows, err := PrepareReadinessRows(a, m)
		if err != nil {
			t.Fatal(err)
		}
		r := rows[3][:]
		if r[0] != 4 || RowWord(r, 23) != int16(vector.capacity) || RowWord(r, 25) != 1022 || RowWord(r, 29) != 712 {
			t.Fatalf("theatre%d Kiowa source calculation: capacity%d load%d fuel%d", vector.theatre, RowWord(r, 23), RowWord(r, 25), RowWord(r, 29))
		}
		if vector.theatre == 0 && PreflightRatio(RowWord(r, 25), RowWord(r, 23)) != 62 {
			t.Fatal("Europe screenshot-compatible template should retain full fuel and62% Kiowa load")
		}
		// Restriction lists constrain new choices; a retained template load is
		// not stripped merely because the same system/aircraft is unavailable.
		m.CampaignUnavailableAircraft, m.CampaignUnavailableSystems = []int{4}, []int{9, 0}
		denied, err := PrepareReadinessRows(a, m)
		if err != nil || string(denied[3][:]) != string(r) {
			t.Fatal("availability lists stripped or rerolled the source template")
		}
	}
}

func TestSourceReadinessFirstClass2Override(t *testing.T) {
	a := missionReadinessAssets(t)
	m := &MissionData{SourceWorld: &a.Generation.Worlds[0], FlightTemplate: 12, Inputs: MissionInputs{MissionType: MissionFlight}, Conditions: MissionConditions{Temperature: 45}}
	for _, raw := range []int{1, 2} {
		m.RawE278 = raw
		rows, err := PrepareReadinessRows(a, m)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, r := range rows {
			if r[1] != 2 {
				continue
			}
			if !found {
				found = true
				wantID, want35 := byte(20), byte(2)
				if raw == 2 {
					wantID, want35 = 21, 1
				}
				if r[5] != wantID || r[35] != want35 || RowWord(r[:], 16) != 1 {
					t.Fatal("first class2 source override mismatch")
				}
			} else if r[35] != 0 {
				t.Fatal("override applied beyond first class2")
			}
		}
		if !found {
			t.Fatal("source template12 contains no class2 aircraft")
		}
	}
}

func TestSourceReadinessGuards(t *testing.T) {
	for _, v := range []struct{ n, d, w int16 }{{100, 100, 100}, {101, 100, 101}, {-101, 100, -101}, {99, 0, 0}, {-5, 2, -250}, {5, -2, -250}} {
		if got := PreflightRatio(v.n, v.d); got != v.w {
			t.Fatalf("signed ratio%v got%d", v, got)
		}
	}
	rows := make([][36]byte, 5)
	for i := range rows {
		rows[i][0] = byte(i % 3)
		RowPut(rows[i][:], 23, 100)
		RowPut(rows[i][:], 25, 100)
	}
	if err := PreflightReadiness(rows); err != nil {
		t.Fatal(err)
	}
	rows[4][0] = 4
	if PreflightReadiness(rows) == nil {
		t.Fatal("four distinct types accepted")
	}
	if err := PreflightReadiness(rows[:4]); err != nil {
		t.Fatal("type count guard applies only to full five-row flight", err)
	}
	RowPut(rows[0][:], 25, 101)
	if PreflightReadiness(rows[:1]) == nil {
		t.Fatal("ratio101 accepted")
	}
}

func TestMissionFreshLocalSeeds(t *testing.T) {
	a := missionReadinessAssets(t)
	for _, mission := range []MissionType{MissionTraining, MissionSingle, MissionFlight} {
		for ti := 0; ti < 4; ti++ {
			if mission == MissionTraining && ti > 1 {
				continue
			}
			for seed := int64(1); seed <= 20; seed++ {
				t.Run(fmt.Sprintf("%s/theatre%d/seed%d", mission, ti, seed), func(t *testing.T) {
					m, err := GenerateMission(a.Generation, MissionInputs{MissionType: mission, Theatre: saveTheatres[ti], Rank: RankCaptain, PilotName: "Test Pilot Name", Seed: seed})
					if err != nil {
						t.Fatal(err)
					}
					if _, err = PrepareReadinessRows(a, m); err != nil {
						t.Fatal(err)
					}
					if len(m.Objects) < 80 || len(m.Objects) > 81 {
						t.Fatal("unexpected accepted count", len(m.Objects))
					}
				})
			}
		}
	}
}
