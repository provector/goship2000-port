package menus

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"math/rand"
	"os"
	"reflect"
	"testing"
)

// Real generation/ORDNANCE/settings/launch with supplied Captain/squadron history.
// Tests supply an approach pose later; they are not naturally flown witnesses.
func objectiveController(t *testing.T, seed int64, aircraft int, load bool) *FrontEnd {
	t.Helper()
	c := missionController(t)
	a, err := assetdata.LoadFrontEndAssets(os.Getenv("GS2000_TEST_MISSIONS"))
	if err != nil {
		t.Fatal(err)
	}
	c.Office.Assets, c.Menus = a, a.Menus
	next := c.Store.State()
	p := next.SelectedPilot()
	p.Rank, p.MissionType, p.Theatre = gamestate.RankCaptain, gamestate.MissionFlight, gamestate.TheatrePersianGulf
	p.SquadronPilots, err = gamestate.InitializeSquadron(p, next.Slots[:], a.Missions.Preflight.SquadronNames, rand.New(rand.NewSource(42)).Intn)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Store.Commit(next); err != nil {
		t.Fatal(err)
	}
	if err := c.RefreshOffice(); err != nil {
		t.Fatal(err)
	}
	c.MissionSeed = &seed
	readyMission(t, c)
	choose := func(action string, value int) {
		t.Helper()
		for i, r := range c.Mission.Ordnance.Choices {
			if r.Enabled && r.Action == action && (value < 0 || r.Value == value) {
				missionClick(t, c, i)
				return
			}
		}
		t.Fatalf("no admitted ORDNANCE action%s/%d", action, value)
	}
	for slot := 0; slot < 5; slot++ {
		if err := c.openOrdnance(slot); err != nil {
			t.Fatal(err)
		}
		choose("aircraft", -1)
		choose("replace", aircraft)
		choose("back", -1)
		if slot == 0 && ordnanceAcceptance(c.Mission.Ordnance.Working[:], 0) == "HELICOPTER IS UNARMED.  Add weapons." {
			choose("fits", -1)
			choose("mount", -1)
			choose("back", -1)
		}
		if slot == 0 && load {
			found := false
			for i, r := range c.Mission.Ordnance.Choices {
				w := a.Missions.WeaponDefinitions[r.System]
				if r.Enabled && r.Action == "mount" && len(w) == 214 && gamestate.RowWord(w, 4)&0x4100 == 0x4100 {
					missionClick(t, c, i)
					found = true
					break
				}
			}
			if !found {
				t.Fatal("source CARGO option missing")
			}
		}
		choose("back", -1)
	}
	if c.Screen == ScreenOutfit {
		missionClick(t, c, 6)
	}
	if c.Screen != ScreenBriefing {
		t.Fatal("loadout did not accept", c.Screen, c.Message)
	}
	startRendering(t, c)
	jFinishFade(t, c)
	if c.Mission.Data.Inputs.MissionType != gamestate.MissionFlight || c.Mission.ResultLaunch == nil {
		t.Fatal("genuine Flight/source result boundary absent", c.Mission.ResultUnavailable)
	}
	if c.Rendering.ResultTargets == nil || c.Rendering.ResultTargets != c.Rendering.Initialized.Targets {
		t.Fatal("missing shared live TARGET carrier")
	}
	return c
}
func objectiveApproach(t *testing.T, c *FrontEnd) {
	t.Helper()
	p := c.Rendering
	r := p.Flight
	target := p.ResultTargets.Records[2]
	r.State.Pose.H0 = binary.LittleEndian.Uint32(target[34:])
	r.State.Pose.H1 = binary.LittleEndian.Uint32(target[38:])
	r.State.Pose.Height = 2
	r.State.Pose.Angles = [3]uint16{}
	r.State.Forward, r.State.Vertical, r.State.Lateral, r.State.IndicatedSpeed = 0, 0, 0, 0
	r.State.Hover, r.State.HeightControl, r.State.Collective, r.State.Rotor, r.State.FlightStage = 0, 0, 0, 0, 128
	p.clearFlightInput()
	p.publishFlight()
	p.resetPresentation()
}
func TestGenuineFlightPlayerMechanicsAndGPSDrop(t *testing.T) {
	for _, tc := range []struct {
		name      string
		seed      int64
		air, kind int
		load      bool
	}{{"pickup", 38, 5, 2, false}, {"rescue", 5, 5, 2, false}, {"delivery", 1, 5, 1, true}, {"recon", 10, 4, 4, false}} {
		t.Run(tc.name, func(t *testing.T) {
			c := objectiveController(t, tc.seed, tc.air, tc.load)
			p := c.Rendering
			r := p.Flight
			if int(binary.LittleEndian.Uint16(p.ResultTargets.Records[2][32:])) != tc.kind {
				t.Fatal("genuine seed changed objective family")
			}
			beforeSave := c.Store.State()
			beforeMission, _ := json.Marshal(c.Mission.Data)
			mass := r.MassTerm
			objectiveApproach(t, c)
			if tc.kind == 4 {
				r.State.Pose.Height = 500
				p.publishFlight()
			}
			// Current source movement publication through the actual logical controller.
			if err := c.AdvanceRendering(.12); err != nil {
				t.Fatal(err)
			}
			if p.ResultTargets.Records[2][32] != 0 {
				t.Fatalf("goal not completed: kind%d pose%+v ship%02x limits%v", tc.kind, r.State.Pose, p.Initialized.Stores.Ships[0].Flags, p.Initialized.ObjectiveLimits)
			}
			if !reflect.DeepEqual(beforeSave, c.Store.State()) {
				t.Fatal("Flight mechanics wrote career")
			}
			if tc.kind == 2 {
				if p.Initialized.ObjectiveAssignments[0] != 1 || p.Initialized.Stores.Ships[0].Flags&0xf8 != 0x18 || r.MassTerm != mass {
					t.Fatal("pickup owner/hold/mass")
				}
				if err := c.openGPS(); err != nil {
					t.Fatal(err)
				}
				// Route rollback cannot restore dropped cargo; the GPS transaction is separate.
				if err := c.handleGPS(Actions{GPSKey: "f"}); err != nil {
					t.Fatal(err)
				}
				frozenSteps, rng, fuel := r.Steps, p.Initialized.RandomState, r.Systems.Fuel[0]
				if err := c.handleGPS(Actions{GPSKey: "c"}); err != nil {
					t.Fatal(err)
				}
				if r.MassTerm != mass-4000 || p.Initialized.Stores.Ships[0].Flags&0xf8 != 0 || r.Steps != frozenSteps || rng != p.Initialized.RandomState || fuel != r.Systems.Fuel[0] {
					t.Fatal("GPS drop ownership or timer mutation")
				}
				if err := c.handleGPS(Actions{Cancel: true}); err != nil {
					t.Fatal(err)
				}
				if p.Initialized.Stores.Ships[0].Flags&0xf8 != 0 {
					t.Fatal("route rollback restored cargo")
				}
				if err := c.handleGPS(Actions{Cancel: true}); err != nil {
					t.Fatal(err)
				}
			}
			// Source62 preparation and immutable briefing data never mutate during play.
			afterMission, _ := json.Marshal(c.Mission.Data)
			if string(beforeMission) != string(afterMission) {
				t.Fatal("mechanics changed prepared mission")
			}
			p.Initialized.EndSortieForQoL()
			if err := c.finishSortieResult(); err != nil {
				t.Fatal(err)
			}
			if c.Screen != ScreenDebrief || !reflect.DeepEqual(beforeSave.SelectedPilot(), c.Store.State().SelectedPilot()) {
				t.Fatal("Flight ending manufactured result", c.Screen, c.Message)
			}
		})
	}
}
func TestMissionMechanicsIndependentOfPresentationSchedule(t *testing.T) {
	var expected []byte
	for _, tc := range []struct {
		mode       string
		scale, fps int
	}{{"smooth", 1, 30}, {"smooth", 2, 144}, {"stepped", 1, 60}, {"stepped", 2, 30}} {
		t.Run(fmt.Sprintf("%s/%d/%d", tc.mode, tc.scale, tc.fps), func(t *testing.T) {
			c := objectiveController(t, 38, 5, false)
			p := c.Rendering
			r := p.Flight
			objectiveApproach(t, c)
			p.Scene.RenderScale = tc.scale
			p.presentation.mode = tc.mode
			p.resetPresentation()
			for frame := 0; frame < tc.fps/2; frame++ {
				if err := c.AdvanceRendering(1 / float64(tc.fps)); err != nil {
					t.Fatal(err)
				}
				c.Frame(false)
			}
			raw, err := json.Marshal(struct {
				Targets *gamestate.MissionTargets
				Ships   []gamestate.RuntimeAircraft
				Owner   [2]uint16
				Mass    int16
				RNG     uint16
				Flight  any
				Results any
				Steps   uint64
			}{p.ResultTargets, p.Initialized.Stores.Ships, p.Initialized.ObjectiveAssignments, r.MassTerm, p.Initialized.RandomState, r.State, r.Combat.Results, r.Steps})
			if err != nil {
				t.Fatal(err)
			}
			if expected == nil {
				expected = raw
			} else if string(raw) != string(expected) {
				t.Fatal("authoritative objective/flight/stores/RNG differs with presentation")
			}
		})
	}
}

func TestCargoGPSContextAndSourceComponentEdge(t *testing.T) {
	c := objectiveController(t, 38, 5, false)
	p, r := c.Rendering, c.Rendering.Flight
	objectiveApproach(t, c)
	if err := c.AdvanceRendering(.12); err != nil {
		t.Fatal(err)
	}
	if err := c.openGPS(); err != nil {
		t.Fatal(err)
	}
	// Source B080 reaches the fourth literal after pickup18; render that real
	// producer state before testing C's overview/selected-player ownership.
	c.Frame(false)
	if p.Error != nil {
		t.Fatal(p.Error)
	}
	mass, flags := r.MassTerm, p.Initialized.Stores.Ships[0].Flags
	for _, context := range []string{"detail", "nonplayer"} {
		p.GPS.Low, p.GPS.Selected = context == "detail", 0
		if context == "nonplayer" {
			p.GPS.Selected = 1
		}
		if err := c.handleGPS(Actions{GPSKey: "c"}); err != nil {
			t.Fatal(err)
		}
		if r.MassTerm != mass || p.Initialized.Stores.Ships[0].Flags != flags {
			t.Fatal("C escaped its source/native context", context)
		}
	}
	p.GPS.Low, p.GPS.Selected = false, 0
	if err := c.handleGPS(Actions{GPSKey: "c"}); err != nil {
		t.Fatal(err)
	}
	if r.MassTerm != mass-4000 || p.GPS.CargoReleaseAvailable {
		t.Fatal("drop priority/legend")
	}
	// The normally generated Blackhawk uses model58, not the separately gated
	// source A6 cargo model. It must not receive an invented component mutation.
	for _, in := range p.instances {
		if in.ID == p.Initialized.PlayerID && in.GraphChild != nil {
			t.Fatal("cargo edge was generalized to a different source model")
		}
	}
	// Labelled supplied model-domain case: source b58==A6 admits graphRoot+9.
	p.Initialized.Entities[p.Initialized.PlayerRow].Object.Raw[12] = 0xa6
	for i := range p.instances {
		if p.instances[i].ID == p.Initialized.PlayerID {
			p.instances[i].ModelID = 166
		}
	}
	if err := c.handleGPS(Actions{GPSKey: "c"}); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, in := range p.instances {
		if in.ID != p.Initialized.PlayerID {
			continue
		}
		found = true
		// Independent OBJFILE A6/root19 byte+9 is packed child44, node4/side0.
		g := in.GraphChild
		if in.ModelID != 166 || g == nil || g.Node != 4 || g.Side != 0 || g.Expected != 44 || g.Child != 255 {
			t.Fatal("source cargo component was not isolated to the actor", in.ModelID, g)
		}
	}
	if !found {
		t.Fatal("actual Blackhawk instance missing")
	}
	if err := c.handleGPS(Actions{GPSKey: "c"}); err != nil {
		t.Fatal(err)
	}
	if r.MassTerm != mass-4000 {
		t.Fatal("empty repeat drop changed mass twice")
	}
}
