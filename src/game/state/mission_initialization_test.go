package state

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/world"
	"os"
	"reflect"
	"testing"
)

func initializationAssets(t *testing.T) *assetdata.OfficeAssets {
	t.Helper()
	path := os.Getenv("GS2000_TEST_MENUS")
	if path == "" {
		t.Skip("set GS2000_TEST_MENUS to separately prepared StepI assets")
	}
	a, e := assetdata.LoadFrontEndAssets(path)
	if e != nil {
		t.Fatal(e)
	}
	return a
}
func initializedFlight(t *testing.T, a *assetdata.MissionAssets, m *MissionData) ([][36]byte, []FlightPilot) {
	t.Helper()
	rows, e := PrepareReadinessRows(a, m)
	if e != nil {
		t.Fatal(e)
	}
	p, e := NewPilot()
	if e != nil {
		t.Fatal(e)
	}
	p.Name = "Initialization fixture"
	p.Rank = RankBrigadierGeneral
	p.SquadronPilots = []SquadronPilot{{Name: "Fixture One", Rank: RankSecondLieutenant, Portrait: 1}, {Name: "Fixture Two", Rank: RankSecondLieutenant, Portrait: 2}, {Name: "Fixture Three", Rank: RankSecondLieutenant, Portrait: 3}, {Name: "Fixture Four", Rank: RankSecondLieutenant, Portrait: 4}}
	flight, e := InitFlightPilots(p, m)
	if e != nil {
		t.Fatal(e)
	}
	if e = RefreshFlightPresentation(flight, a, m); e != nil {
		t.Fatal(e)
	}
	return rows, flight
}
func TestMissionInitializationPreparedMatrix(t *testing.T) {
	a := initializationAssets(t)
	type report struct {
		Case          string
		Seed          int64
		Switches      uint16
		World         string
		Player        InitializedPose
		Rows, Visible int
		Contact       int16
		Kinds, Sides  map[string]int
		Excluded      map[string]int
		Error         string
	}
	reports := []report{}
	for _, kind := range []MissionType{MissionTraining, MissionSingle, MissionFlight} {
		for _, theatre := range saveTheatres {
			if kind == MissionTraining && theatre.Raw() > 1 {
				continue
			}
			for seed := int64(1); seed <= 3; seed++ {
				name := fmt.Sprintf("%s/%s/seed%d", kind, theatre, seed)
				t.Run(name, func(t *testing.T) {
					m, e := GenerateMission(a.Missions.Generation, MissionInputs{MissionType: kind, Theatre: theatre, Rank: RankBrigadierGeneral, PilotName: "Initialization fixture", Seed: seed})
					if e != nil {
						t.Fatal(e)
					}
					before, _ := json.Marshal(m)
					rows, flight := initializedFlight(t, a.Missions, m)
					switches, e := LaunchSwitches(m, PreflightSettings{}, a.Missions.Generation.Tables.Initialization.CPGMapping)
					if e != nil {
						t.Fatal(e)
					}
					got, e := InitializeMission(a.Missions, a.World, m, rows, flight, switches, 1234)
					r := report{Case: name, Seed: seed, Switches: switches, World: m.SourceWorld.TerrainName, Excluded: map[string]int{}, Kinds: map[string]int{}, Sides: map[string]int{}}
					if e != nil {
						r.Error = e.Error()
						reports = append(reports, r)
						t.Fatal(e)
					}
					r.Player, r.Rows = got.Player, len(got.Rows)
					r.Contact = got.InitialContactHeight
					for _, x := range got.Entities {
						r.Kinds[fmt.Sprintf("%02x", x.Kind)]++
						r.Sides[x.Side]++
						if x.Visible {
							r.Visible++
						} else {
							r.Excluded[x.Exclusion]++
						}
					}
					reports = append(reports, r)
					after, _ := json.Marshal(m)
					if !reflect.DeepEqual(before, after) {
						t.Fatal("initialization mutated generated mission")
					}
					if len(got.Entities) != len(m.Objects) || got.PlayerID != "mission-row-0" || len(got.FlightRows) != len(rows) || got.AutonomousDecisions {
						t.Fatal("population/identity/AI ownership")
					}
					for _, x := range got.Entities {
						if x.Visible && (x.ModelID < 0 || x.ModelID >= 230) {
							t.Fatal("visible invalid model")
						}
					}
					t.Logf("world%s rows%d visible%d player%+v switches%04x", r.World, r.Rows, r.Visible, r.Player, r.Switches)
				})
			}
		}
	}
	if out := os.Getenv("GS2000_INIT_REPORT"); out != "" {
		if _, e := os.Stat(out); !os.IsNotExist(e) {
			t.Fatal("choose new initialization report")
		}
		b, _ := json.MarshalIndent(reports, "", "  ")
		if e := os.WriteFile(out, append(b, '\n'), 0644); e != nil {
			t.Fatal(e)
		}
	}
}
func TestMissionInitializationLaunchTableAllAircraft(t *testing.T) {
	a := initializationAssets(t)
	m, e := GenerateMission(a.Missions.Generation, MissionInputs{MissionType: MissionTraining, Theatre: TheatrePersianGulf, Rank: RankBrigadierGeneral, Seed: 1})
	if e != nil {
		t.Fatal(e)
	}
	rows, flight := initializedFlight(t, a.Missions, m)
	// Independently read original SHIN/SHHD byte2; see StepI source vector.
	expected := []byte{9, 122, 123, 138, 59, 58, 139, 139}
	for id := 0; id < 8; id++ {
		row, e := InitializeReadinessRow(a.Missions, m, id)
		if e != nil {
			t.Fatal(e)
		}
		rows[0] = row
		got, e := launchObjects(a.Missions, m, rows, flight)
		if e != nil {
			t.Fatal(e)
		}
		if got[0].Raw[12] != expected[id] || got[0].Raw[10] != 0xff || got[0].Raw[14] != 0x4f {
			t.Fatalf("aircraft%d launch raw%v", id, got[0].Raw)
		}
		if got[0].Point() != m.Objects[0].Point() {
			t.Fatal("single primary incorrectly received formation offset")
		}
	}
}
func TestInitializationHeadingAndMalformedInputs(t *testing.T) {
	cases := []struct {
		Target MissionPoint
		Want   uint16
	}{{MissionPoint{32, 0}, 0xc000}, {MissionPoint{-32, 0}, 0x4000}, {MissionPoint{0, 32}, 0}, {MissionPoint{0, -32}, 0x8000}, {MissionPoint{32, 32}, 0}}
	for _, c := range cases {
		if got := initializedHeading(MissionPoint{}, c.Target, false); got != c.Want {
			t.Fatalf("heading %v %04x", c.Target, got)
		}
	}
	if initializedHeading(MissionPoint{}, MissionPoint{-32, -100}, true) != 0 {
		t.Fatal("carrier start heading")
	}
	a := initializationAssets(t)
	m, e := GenerateMission(a.Missions.Generation, MissionInputs{MissionType: MissionTraining, Theatre: TheatrePersianGulf, Rank: RankBrigadierGeneral, Seed: 2})
	if e != nil {
		t.Fatal(e)
	}
	rows, flight := initializedFlight(t, a.Missions, m)
	m.Objects[10].Raw[12] = 255
	before, _ := json.Marshal(m)
	if _, e = InitializeMission(a.Missions, a.World, m, rows, flight, 0, 1234); e == nil {
		t.Fatal("invalid consumed primary model accepted")
	}
	after, _ := json.Marshal(m)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("failed init changed source")
	}
	// Word packing is independent of format prose; native row quantity must remain.
	if binary.LittleEndian.Uint16(rows[0][29:]) == 0 {
		t.Fatal("fixture has no prepared fuel")
	}
}

func TestMissionInitializationStartFamilies(t *testing.T) {
	a := initializationAssets(t)
	found := map[string]bool{}
	// This is a bounded native-generator fixture search, not original runtime
	// history. First source-selected land/carrier/pass-through starts are checked.
	for _, theatre := range saveTheatres {
		for seed := int64(1); seed <= 100 && len(found) < 3; seed++ {
			m, err := GenerateMission(a.Missions.Generation, MissionInputs{MissionType: MissionFlight, Theatre: theatre, Rank: RankBrigadierGeneral, Seed: seed})
			if err != nil {
				t.Logf("fixture generation unavailable %s/seed%d:%v", theatre, seed, err)
				continue
			}
			switches, err := LaunchSwitches(m, PreflightSettings{}, a.Missions.Generation.Tables.Initialization.CPGMapping)
			if err != nil {
				t.Fatal(err)
			}
			family := "land"
			if switches&0x1000 != 0 {
				family = "carrier"
			} else if switches&0x800 != 0 {
				family = "pass-through"
			}
			if found[family] {
				continue
			}
			rows, flight := initializedFlight(t, a.Missions, m)
			got, err := InitializeMission(a.Missions, a.World, m, rows, flight, switches, 1234)
			if err != nil {
				t.Fatalf("%s %s/seed%d:%v", family, theatre, seed, err)
			}
			// Source DATA2582 starts as zero words and the player loader writes
			// only Kindff/control participation. Even pass-through E2=120 must
			// not become the distinct projectile-acceleration dynamic+6 input.
			player := got.Entities[got.PlayerRow]
			if player.RawMotion != 0 || binary.LittleEndian.Uint16(player.DynamicRaw[6:]) != 0 {
				t.Fatalf("%s copied main E2 into source-retained dynamic speed: %+v", family, player)
			}
			if family == "pass-through" {
				// Correct launch ON0 now sets40. Source3792 averages target300,
				// cold height0 and support+2;391e's later floor can clear motion.
				wantHeight, wantMotion := int16(300), int16(120)
				if switches&0x40 != 0 {
					wantHeight = int16(300+got.InitialContactHeight+2) >> 1
				}
				if wantHeight < got.InitialContactHeight+2 {
					wantHeight, wantMotion = got.InitialContactHeight+2, 0
				}
				if int16(got.Player.Height) != wantHeight || got.InitialForwardMotion != wantMotion || got.Player.Angles[0] != 0xf400 {
					t.Fatalf("pass-through source controls height%d/%d motion%d/%d", got.Player.Height, wantHeight, got.InitialForwardMotion, wantMotion)
				}
			} else if int16(got.Player.Height) != got.InitialContactHeight+2 {
				t.Fatal("ground/deck footprint contact+2")
			}
			t.Logf("%s %s seed%d world%s switches%04x contact%d player%+v", family, theatre, seed, got.World, switches, got.InitialContactHeight, got.Player)
			found[family] = true
		}
	}
	if len(found) != 3 {
		t.Fatalf("bounded fixtures missing a start family:%v", found)
	}
}

func TestInitializationContactAfterHeightControl(t *testing.T) {
	// Explicit synthetic flat500 region, independently hand-evaluated plane:
	// -(0*x+0*z-500)/1=500. This tests source391e→3a2e→3b86 after
	// pass-through37ac, rather than supplying a captured initialized pose.
	w := &world.Bundle{Sine: make([]int16, 1025), Worlds: map[string]*world.World{}, Geometries: map[string]*world.Geometry{}}
	w.Sine[1024] = 16384
	w.Worlds["fixture"] = &world.World{Grid: make([]byte, 4096), Cells: make([]world.Cell, 64)}
	for i := range w.Worlds["fixture"].Cells {
		w.Worlds["fixture"].Cells[i].Geometry = "flat"
	}
	w.Geometries["flat"] = &world.Geometry{Heights: []world.HeightRegion{{Points: [][2]int16{{-4096, -4096}, {-4096, 4096}, {4096, 4096}, {4096, -4096}, {-4096, -4096}}, Plane: world.Plane{0, 1, 0, -500}}}}
	for _, flags := range []uint16{0, 0x800, 0x840} {
		m := &InitializedMission{World: "fixture", Player: InitializedPose{H0: 3*8192 + 4096, H1: 3*8192 + 4096}, Switches: flags, InitialHeightTarget: 300, InitialForwardMotion: 120}
		if err := m.initializePlayerContact(w, nil); err != nil {
			t.Fatal(err)
		}
		if m.Player.Height != 502 || m.InitialContactHeight != 500 || m.InitialForwardMotion != 0 {
			t.Fatalf("flags%04x hill contact %+v", flags, m)
		}
	}
}
