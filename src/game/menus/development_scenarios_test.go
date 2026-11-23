package menus

import (
	"encoding/binary"
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/flight"
	gamestate "gunship2000-assets/game/state"
	"os"
	"reflect"
	"testing"
)

func developmentScenarioController(t *testing.T, name string) *FrontEnd {
	t.Helper()
	return developmentScenarioControllerProfile(t, name, "")
}

func developmentScenarioControllerProfile(t *testing.T, name, profile string) *FrontEnd {
	t.Helper()
	c := missionController(t)
	c.simulationProfile = profile
	if profile != "" {
		// Recorded-profile constructors require the immutable launch bank. Build
		// the same named preparation normally, then supply only its frozen bank.
		anchor := developmentScenarioController(t, name)
		c.enemyLaunchRules = anchor.Rendering.Initialized.EnemyRules
		anchor.AbandonFilmRecording()
	}
	a, err := assetdata.LoadFrontEndAssets(os.Getenv("GS2000_TEST_MISSIONS"))
	if err != nil {
		t.Fatal(err)
	}
	c.Office.Assets, c.Menus = a, a.Menus
	if _, err := c.ConfigureDevelopmentScenario(name); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestDevelopmentScenarioPreparedAndPausedBoundary(t *testing.T) {
	for _, name := range DevelopmentScenarioNames {
		t.Run(name, func(t *testing.T) {
			c := developmentScenarioController(t, name)
			p, r, meta := c.Rendering, c.Rendering.Flight, c.DevelopmentScenario
			if c.Screen != ScreenScene || !p.Paused || !r.Paused || r.Steps != 0 || meta == nil || meta.Name != name {
				t.Fatal("scenario did not stop at paused fresh logical boundary")
			}
			if r.State.Terminal != nil || r.Systems.Damage != [10]byte{} || r.Systems.DamageMasks != [2]byte{} {
				t.Fatal("scenario pre-edited subsystem damage or terminal outcome")
			}
			if p.Initialized.ObjectiveAssignments != [2]uint16{} || p.ResultTargets.Records[2][32] == 0 {
				t.Fatal("scenario pre-completed objective")
			}
			if p.Initialized.Entities[p.Initialized.PlayerRow].Pose.H0 != r.State.Pose.H0 || p.Cockpit.Height != uint16(r.State.Pose.Height) || p.Scene.Camera != p.ViewControls.Camera || !reflect.DeepEqual(p.presentation.previous, p.presentation.current) {
				t.Fatal("pose/camera/instrument/interpolation ownership disagrees")
			}
			profile, _ := developmentProfile(name)
			if c.Mission.Data.Inputs.MissionType != profile.mission || c.Mission.ResultLaunch == nil {
				t.Fatal("genuine mission type/admitted real actor facts", c.Mission.ResultUnavailable)
			}
			before, _ := json.Marshal(struct {
				Flight  flight.State
				Targets *gamestate.MissionTargets
				RNG     uint16
				Fuel    []int16
			}{*r.State, p.ResultTargets, p.Initialized.RandomState, r.Systems.Fuel})
			if err := c.AdvanceRendering(3); err != nil {
				t.Fatal(err)
			}
			c.Frame(false)
			if p.Error != nil {
				t.Fatal(p.Error)
			}
			after, _ := json.Marshal(struct {
				Flight  flight.State
				Targets *gamestate.MissionTargets
				RNG     uint16
				Fuel    []int16
			}{*r.State, p.ResultTargets, p.Initialized.RandomState, r.Systems.Fuel})
			if string(before) != string(after) || r.Steps != 0 {
				t.Fatal("paused display completed mechanics or consumed state")
			}
		})
	}
}

func TestDevelopmentObjectiveScenariosResumeRealMechanics(t *testing.T) {
	for _, name := range []string{"pickup", "rescue", "delivery", "recon"} {
		t.Run(name, func(t *testing.T) {
			c := developmentScenarioController(t, name)
			p := c.Rendering
			before := c.Store.State()
			mass := p.Flight.MassTerm
			if err := c.Handle(Actions{Cancel: true}); err != nil {
				t.Fatal(err)
			}
			if err := c.AdvanceRendering(.12); err != nil {
				t.Fatal(err)
			}
			if p.ResultTargets.Records[2][32] != 0 || !reflect.DeepEqual(before, c.Store.State()) || p.Initialized.Sortie != nil && p.Initialized.Sortie.Result != nil {
				t.Fatal("real objective path not completed or invented career/result")
			}
			if name == "pickup" || name == "rescue" {
				if p.Initialized.Stores.Ships[0].Flags&0xf8 != 0x18 || p.Initialized.ObjectiveAssignments[0] != 1 || p.Flight.MassTerm != mass {
					t.Fatal("real pickup owner/hold/no immediate mass producer")
				}
			} else if name == "delivery" && p.Initialized.Stores.Ships[0].Flags&0xf8 != 0 {
				t.Fatal("real delivery retained cargo")
			}
		})
	}
}

func TestDevelopmentFARPAndReturnUseContactAndRotor(t *testing.T) {
	for _, name := range []string{"farp", "return"} {
		t.Run(name, func(t *testing.T) {
			c := developmentScenarioController(t, name)
			p := c.Rendering
			if name == "farp" {
				if c.DevelopmentScenario.Seed != 10 || c.Mission.Data.Support != nil || c.Mission.Data.World != "EURO9.WRL" || c.Mission.Data.Scenario != "CENT_03.SNR" || c.Mission.Data.ObjectHash != "1d183f7d1dc557c7a678862d2713d929d88dc7cb7dae053e45914f96009911f7" {
					t.Fatal("FARP fixture must retain its declared ordinary generated None-Support realization", c.DevelopmentScenario.Seed, c.Mission.Data.Support, c.Mission.Data.ObjectHash)
				}
				if target := p.Initialized.Targets.Records[1]; binary.LittleEndian.Uint16(target[32:]) != 3 || int16(binary.LittleEndian.Uint16(target[50:])) != 3 {
					t.Fatal("FARP fixture lost actual source TARGET presence/service limit", target)
				}
			}
			before := c.Store.State()
			if err := c.Handle(Actions{Cancel: true}); err != nil {
				t.Fatal(err)
			}
			if err := c.AdvanceRendering(.12); err != nil {
				t.Fatal(err)
			}
			if p.Flight.State.Rotor != 1 || c.Screen != ScreenScene || p.Initialized.Sortie != nil && p.Initialized.Sortie.Pending != "" {
				t.Fatal("fixture ended before actual rotor-off action")
			}
			if err := c.Handle(Actions{}); err != nil {
				t.Fatal(err)
			}
			if err := c.Handle(Actions{FlightCommands: []int{2}}); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 40 && c.Screen == ScreenScene && (p.Initialized.Sortie == nil || p.Initialized.Sortie.Services == 0); i++ {
				if err := c.AdvanceRendering(.12); err != nil {
					t.Fatal(err)
				}
			}
			if p.Initialized.Sortie == nil || p.Initialized.Sortie.Services == 0 || !reflect.DeepEqual(before, c.Store.State()) {
				t.Fatal("real contact/rotor service not reached or committed career")
			}
			if name == "farp" {
				if c.Screen != ScreenScene || p.Initialized.Sortie.Pending != "" || p.Flight.Systems.Fuel[0] != p.Initialized.Stores.Blueprint[0].Fuel {
					t.Fatal("FARP semantics")
				}
			} else if c.Screen != ScreenQuit || p.Initialized.Sortie.Pending != "base" {
				t.Fatal("natural base prompt not reached", c.Screen, p.Initialized.Sortie)
			}
		})
	}
}

// A direct independent generator call pins the fixture selection before the
// development approach supplies contact pose/fuel. The Support producer itself
// must distinguish old seed1's active Air from seed10's None.
func TestDevelopmentFARPFixtureRetainsSourceSupportDecision(t *testing.T) {
	c := missionController(t)
	in := gamestate.MissionInputs{MissionType: gamestate.MissionFlight, Theatre: gamestate.TheatreCentralEurope, Rank: gamestate.RankBrigadierGeneral, PilotName: "Test Pilot Name", Seed: 1}
	old, err := gamestate.GenerateMission(c.Office.Assets.Missions.Generation, in)
	if err != nil {
		t.Fatal(err)
	}
	if old.Support == nil || old.Support.Mode != 2 {
		t.Fatal("old fixture must expose the actual active Air producer", old.Support)
	}
	if err := gamestate.ValidateSupportLaunch(old); err == nil {
		t.Fatal("active Air execution restriction was bypassed")
	}
	in.Seed = 10
	next, err := gamestate.GenerateMission(c.Office.Assets.Missions.Generation, in)
	if err != nil {
		t.Fatal(err)
	}
	if err := gamestate.PrepareMissionTargets(c.Office.Assets.Missions, next); err != nil {
		t.Fatal(err)
	}
	if next.Support != nil || next.ObjectHash != "1d183f7d1dc557c7a678862d2713d929d88dc7cb7dae053e45914f96009911f7" {
		t.Fatal("declared seed must generate None rather than deleting active Support", next.Support, next.ObjectHash)
	}
	target := next.Targets.Records[1]
	if binary.LittleEndian.Uint16(target[32:]) != 3 || int16(binary.LittleEndian.Uint16(target[50:])) != 3 {
		t.Fatal("source FARP TARGET not retained", target)
	}
}

func TestDevelopmentPickupReturnStagesAfterRealPickup(t *testing.T) {
	c := developmentScenarioController(t, "pickup-return")
	p, meta := c.Rendering, c.DevelopmentScenario
	before := c.Store.State()
	query := meta.Pose
	if err := c.Handle(Actions{Cancel: true}); err != nil {
		t.Fatal(err)
	}
	if err := c.AdvanceRendering(.25); err != nil {
		t.Fatal(err)
	}
	if meta.Stage != "loaded-return" || !p.Paused || !p.Flight.Paused || p.Flight.Steps != 1 || p.Initialized.Stores.Ships[0].Flags&0xf8 != 0x18 || p.Initialized.ObjectiveAssignments[0] != 1 || p.ResultTargets.Records[2][32] != 0 {
		t.Fatal("observer did not follow one actual pickup update", meta.Stage, p.Flight.Steps)
	}
	if p.Flight.State.Pose == query || p.Flight.State.Pose.H0 != binary.LittleEndian.Uint32(p.ResultTargets.Records[0][42:]) || !reflect.DeepEqual(before, c.Store.State()) {
		t.Fatal("development second stage/result ownership")
	}
	if p.Initialized.Sortie != nil && p.Initialized.Sortie.Result != nil {
		t.Fatal("observer pre-manufactured recovery")
	}
	if err := c.Handle(Actions{Cancel: true}); err != nil {
		t.Fatal(err)
	}
	if err := c.Handle(Actions{}); err != nil {
		t.Fatal(err)
	}
	if err := c.Handle(Actions{FlightCommands: []int{2}}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40 && c.Screen == ScreenScene; i++ {
		if err := c.AdvanceRendering(.12); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen != ScreenQuit || p.Initialized.Sortie == nil || p.Initialized.Sortie.Pending != "base" {
		t.Fatal("real loaded return failed contact/rotor predicate")
	}
	if err := c.Handle(Actions{ConfirmQuit: true}); err != nil {
		t.Fatal(err)
	}
	if c.Screen != ScreenDebrief || p.Initialized.Sortie.Result == nil || p.Initialized.Sortie.Result.SourceStatus&0x81 != 0x81 || !reflect.DeepEqual(before.SelectedPilot(), c.Store.State().SelectedPilot()) {
		t.Fatal("safe loaded return facts or admitted Flight staging boundary")
	}
}
