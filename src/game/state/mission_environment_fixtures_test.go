package state

import (
	"encoding/json"
	"gunship2000-assets/game/render3d"
	"os"
	"testing"
)

func TestInitializedWeatherFixturesFromGeneration(t *testing.T) {
	a := initializationAssets(t)
	type witness struct {
		Theatre    Theatre
		Seed       int64
		World      string
		Conditions MissionConditions
		Switches   uint16
		Bank       string
		Player     InitializedPose
		Rows       int
	}
	fixtures := map[string]witness{}
	// Fixed successful source-generation seeds: neither a runtime retry nor
	// weather overrides. Preserve unrelated placement-limit failures separately.
	for _, theatre := range []Theatre{TheatrePersianGulf, TheatreAntarctica, TheatrePhilippines} {
		for _, seed := range []int64{1, 4, 24} {
			m, e := GenerateMission(a.Missions.Generation, MissionInputs{MissionType: MissionSingle, Theatre: theatre, Rank: RankBrigadierGeneral, Seed: seed})
			if e != nil {
				t.Fatalf("fixed fixture candidate%s/seed%d:%v", theatre, seed, e)
			}
			switches, e := LaunchSwitches(m, PreflightSettings{Visibility: 1}, a.Missions.Generation.Tables.Initialization.CPGMapping)
			if e != nil {
				t.Fatal(e)
			}
			bank := render3d.EnvironmentPalette(switches)
			key := string(theatre) + "/" + bank
			if _, ok := fixtures[key]; ok {
				continue
			}
			rows, flight := initializedFlight(t, a.Missions, m)
			init, e := InitializeMission(a.Missions, a.World, m, rows, flight, switches, uint16(seed))
			if e != nil {
				t.Fatal(e)
			}
			if init.Switches&0x300 != m.Conditions.Flags&0x300 {
				t.Fatal("realistic preference rerolled weather")
			}
			before, _ := json.Marshal(init)
			env := &render3d.EnvironmentState{Switches: init.Switches, Prefix: init.World[:4], RNG: init.RandomState}
			for i := 0; i < 520; i++ {
				env.Step(a.World.GraphicsOptions)
			}
			after, _ := json.Marshal(init)
			if string(before) != string(after) {
				t.Fatal("presentation changed actors/weather")
			}
			fixtures[key] = witness{theatre, seed, init.World, m.Conditions, switches, bank, init.Player, len(init.Entities)}
			t.Logf("%s seed%d world%s sourceflags%04x bank%s", theatre, seed, init.World, m.Conditions.Flags, bank)
		}
	}
	for _, bank := range []string{"DAY", "GREY", "DARK"} {
		found := false
		for _, f := range fixtures {
			if f.Bank == bank {
				found = true
			}
		}
		if !found {
			t.Fatal("missing deterministic lighting fixture", bank)
		}
	}
	if out := os.Getenv("GS2000_ENVIRONMENT_REPORT"); out != "" {
		b, _ := json.MarshalIndent(fixtures, "", "  ")
		f, e := os.OpenFile(out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			t.Fatal(e)
		}
		defer f.Close()
		if _, e = f.Write(append(b, '\n')); e != nil {
			t.Fatal(e)
		}
	}
}
