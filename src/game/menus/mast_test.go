package menus

import (
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
)

// These are native generator/source equipment fixtures, not the photographed
// mission's unknown seed or natural promotion/campaign history.
func mastFixture(t *testing.T, aircraft int, family string) *FrontEnd {
	return mastProfileFixture(t, aircraft, family, replay.R12Compatibility)
}

func mastProfileFixture(t *testing.T, aircraft int, family, profile string) *FrontEnd {
	t.Helper()
	c := renderingController(t)
	c.simulationProfile = profile
	if profile == replay.R221Compatibility || profile == replay.R86Compatibility {
		// The old view fixture intentionally omits debrief resources. A new
		// complete-film witness uses the ordinary full prepared asset owner.
		c.Office.Assets, c.Menus = renderingAssets, renderingAssets.Menus
	}
	next := c.Store.State()
	p := next.SelectedPilot()
	p.Rank = gamestate.RankBrigadierGeneral
	seed := int64(1)
	p.MissionType = gamestate.MissionTraining
	p.Theatre = gamestate.TheatrePersianGulf
	if family != "land" {
		p.MissionType = gamestate.MissionFlight
		p.Theatre = gamestate.TheatreCentralEurope
		// P4 now retains the source Support producer. Seed1 selects active air
		// Support, whose controller is explicitly guarded. Seed10 independently
		// generates None, the same corridor family and a real three-row FARP.
		// Mechanism tests must not silently erase that active source selection.
		seed = 10
	}
	if family == "carrier" {
		p.Theatre = gamestate.TheatreAntarctica
		seed = 4
	}
	var err error
	p.SquadronPilots, err = gamestate.InitializeSquadron(p, next.Slots[:], c.Office.Assets.Missions.Preflight.SquadronNames, rand.New(rand.NewSource(42)).Intn)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Store.Commit(next); err != nil {
		t.Fatal(err)
	}
	if err = c.RefreshOffice(); err != nil {
		t.Fatal(err)
	}
	c.MissionSeed = &seed
	readyMission(t, c)
	if err = c.openOrdnance(0); err != nil {
		t.Fatal(err)
	}
	choose := func(action string, value *int) {
		t.Helper()
		for i, r := range c.Mission.Ordnance.Choices {
			if r.Enabled && r.Action == action && (value == nil || r.Value == *value) {
				missionClick(t, c, i)
				return
			}
		}
		t.Fatalf("no %s/%v", action, value)
	}
	choose("aircraft", nil)
	choose("replace", &aircraft)
	choose("back", nil)
	if ordnanceAcceptance(c.Mission.Ordnance.Working[:], 0) == "HELICOPTER IS UNARMED.  Add weapons." {
		choose("fits", nil)
		choose("mount", nil)
		choose("back", nil)
	}
	choose("back", nil)
	// A declared all-one-type flight avoids introducing a fourth aircraft type
	// when replacing the source generated leader. Other slots may be unarmed;
	// the actual ORDNANCE/whole-flight acceptance predicates still run.
	if family != "land" {
		for slot := 1; slot < 5; slot++ {
			if err = c.openOrdnance(slot); err != nil {
				t.Fatal(err)
			}
			choose("aircraft", nil)
			choose("replace", &aircraft)
			choose("back", nil)
			choose("back", nil)
		}
	}
	if c.Screen == ScreenOutfit {
		missionClick(t, c, 6)
	}
	if c.Screen != ScreenBriefing {
		t.Fatalf("fixture acceptance:%s %s", c.Screen, c.Message)
	}
	if profile == replay.R221Compatibility || profile == replay.R86Compatibility {
		// Explicit simulation profiles use the same frozen-world bank as
		// playback. Suppress new hostile decisions in this view-input fixture,
		// without erasing the bank or supplying initialized simulation state.
		c.Runtime.DisableEnemyAI = true
		switches, err := gamestate.LaunchSwitches(c.Mission.Data, c.Mission.Settings, c.Office.Assets.Missions.Generation.Tables.Initialization.CPGMapping)
		if err != nil {
			t.Fatal(err)
		}
		c.enemyLaunchRules, err = freezePreparedEnemyRules(c.Office.Assets.Missions, c.Mission.Data, switches, true)
		if err != nil {
			t.Fatal(err)
		}
	}
	startRendering(t, c)
	want := map[string]uint16{"land": 0, "carrier": 0x1000, "pass": 0x800}[family]
	if c.Rendering.Initialized.Switches&0x1800 != want {
		t.Fatalf("wrong source start family %s:%04x", family, c.Rendering.Initialized.Switches)
	}
	return c
}

func mastCapture(t *testing.T, c *FrontEnd, root, name string) {
	t.Helper()
	if root == "" {
		return
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	raw := c.Frame(false)
	img := image.NewPaletted(image.Rect(0, 0, 320, 200), c.Rendering.Palette)
	copy(img.Pix, raw)
	f, err := os.OpenFile(filepath.Join(root, name+".png"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()
	b, err := json.MarshalIndent(c.Rendering.Report(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, name+".json"), append(b, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	b, err = json.Marshal(c.Rendering.Scene)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, name+".scene.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestMastSourceCameraAndRoundTrips(t *testing.T) {
	for _, id := range []int{1, 3, 4} {
		for _, family := range []string{"land", "carrier", "pass"} {
			t.Run(fmt.Sprintf("aircraft%d_%s", id, family), func(t *testing.T) {
				c := mastFixture(t, id, family)
				p := c.Rendering
				mission, _ := json.Marshal(c.Mission.Data)
				initial, _ := json.Marshal(p.Initialized)
				saved := c.Store.State()
				if err := c.Handle(Actions{ViewKey: 2}); err != nil {
					t.Fatal(err)
				}
				name := fmt.Sprintf("aircraft%d-%s-mast", id, family)
				mastCapture(t, c, os.Getenv("GS2000_I1_MAST_REPORT"), name)
				// Independently read section1:1563..1579:313a==0 refreshes3ba2 from3b98,
				// then ADD [3baa],0080. This later writer follows the F2 key's initial copy.
				if p.Scene.Camera.Height != p.Initialized.Player.Height+0x80 {
					t.Errorf("missed later mast writer: eye%d player%d, want +128", p.Scene.Camera.Height, p.Initialized.Player.Height)
				}
				for _, key := range []int{1, 3, 4, 5, 7, 2} {
					if err := c.Handle(Actions{ViewKey: key}); err != nil {
						t.Fatal(err)
					}
				}
				for n := 0; n < 4; n++ {
					if err := c.Handle(Actions{CycleArrangement: true}); err != nil {
						t.Fatal(err)
					}
					c.Frame(false)
				}
				// Mast motion is presentation only and repeat draws do not advance it.
				if err := c.Handle(Actions{SceneDirections: [4]bool{true}}); err != nil {
					t.Fatal(err)
				}
				if err := c.StepRendering(1); err != nil {
					t.Fatal(err)
				}
				cam := p.Scene.Camera
				phase := p.Visual.Phase
				for n := 0; n < 3; n++ {
					p.Dirty = true
					c.Frame(false)
				}
				if p.Scene.Camera != cam || p.Visual.Phase != phase {
					t.Fatal("Draw advanced camera/phase")
				}
				after, _ := json.Marshal(c.Mission.Data)
				now, _ := json.Marshal(p.Initialized)
				if string(after) != string(mission) || string(now) != string(initial) || !reflect.DeepEqual(saved, c.Store.State()) {
					t.Fatal("views modified mission/entities/save")
				}
			})
		}
	}
}

func TestMastArrangementClearsAbandonedViewport(t *testing.T) {
	c := mastFixture(t, 1, "land")
	if err := c.Handle(Actions{ViewKey: 2}); err != nil {
		t.Fatal(err)
	}
	c.Frame(false)
	// A declared stale destination witness, not expected scene pixels. Source
	// Alt+S0704..0730 explicitly clears below the new aircraft-derived bottom.
	for i := range c.Rendering.worldFrame {
		c.Rendering.worldFrame[i] = 0x55
	}
	if err := c.Handle(Actions{CycleArrangement: true}); err != nil {
		t.Fatal(err)
	}
	c.Frame(false)
	bottom := c.Rendering.Scene.View.Bounds.Bottom
	if bottom >= 199 {
		t.Fatal("fixture did not reduce the viewport")
	}
	for at, v := range c.Rendering.worldFrame[(bottom+1)*320:] {
		if v != 0 {
			t.Fatalf("abandoned world row%d kept index%d", bottom+1+at/320, v)
		}
	}
}
