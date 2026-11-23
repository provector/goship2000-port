package menus

import (
	"bytes"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/internal/testfixture"
	"os"
	"reflect"
	"testing"
)

func TestActualLandCarrierContactServiceModal(t *testing.T) {
	for _, start := range []string{"land", "carrier"} {
		t.Run(start, func(t *testing.T) {
			c := mastFixture(t, 0, start)
			jFinishFade(t, c)
			m, p := c.Mission, c.Rendering
			r := p.Flight
			saved := c.Store.State()
			hash := m.Data.ObjectHash
			// Explicit synthetic readiness witness: source128-count cooldown's
			// last E0 is tested here; stage4/start128 sequencing has separate cases.
			r.State.FlightStage = 1
			r.Systems.Fuel[0] = 7
			for i := range p.Initialized.Stores.Ships[0].Stations {
				if p.Initialized.Stores.Ships[0].Stations[i].Present {
					p.Initialized.Stores.Ships[0].Stations[i].Ammunition = 0
				}
			}
			if e := c.advanceFlight(.1); e != nil {
				t.Fatal(e)
			}
			if r.State.FlightStage != 0 || !r.State.ValidContact || c.Screen != ScreenQuit || !c.sceneEndConfirmation || p.Initialized.Sortie.Pending != "base" {
				t.Fatalf("source guard path: stage%d contact%v screen%s sortie%+v terminal%+v", r.State.FlightStage, r.State.ValidContact, c.Screen, p.Initialized.Sortie, r.State.Terminal)
			}
			if r.Systems.Fuel[0] != p.Initialized.Stores.Blueprint[0].Fuel {
				t.Fatal("refill used new/default fuel instead of accepted blueprint")
			}
			if !reflect.DeepEqual(c.Store.State(), saved) || m.Data.ObjectHash != hash {
				t.Fatal("service mutated persistent/prepared mission")
			}
			frozen := r.State.Pose
			act(t, c, Actions{AnyKey: true, Text: []rune{'x'}})
			if c.Screen != ScreenQuit || r.State.Pose != frozen {
				t.Fatal("unrelated key dismissed natural Y/N modal")
			}
			act(t, c, Actions{Text: []rune{'n'}})
			if c.Screen != ScreenScene || r.State.FlightStage != 128 || p.Initialized.Sortie.Pending != "" || r.State.Pose != frozen {
				t.Fatal("N did not preserve view/contact and reset source cooldown")
			}
			// Re-present the same source-supported transient prompt, then answerY.
			r.State.FlightStage = 0
			if e := c.advanceFlight(.1); e != nil {
				t.Fatal(e)
			}
			if c.Screen != ScreenQuit {
				t.Fatal("base re-readiness missing")
			}
			act(t, c, Actions{ConfirmQuit: true})
			waitForResultVerification(t, c)
			if c.Screen != ScreenBriefing || c.Mission != m || p.Initialized.Sortie.Result == nil || p.Initialized.Sortie.Result.Reason != "base" {
				t.Fatal("natural Y changed session or lost transient result")
			}
			if !reflect.DeepEqual(c.Store.State(), saved) || m.Data.ObjectHash != hash {
				t.Fatal("natural end wrote an outcome")
			}
			if candidate, e := c.Store.RecoveryCandidate(); e != nil || candidate != nil {
				t.Fatal("natural end created recovery", e)
			}
		})
	}
}

func TestAllAircraftRuntimeStoreCountersThroughPreflight(t *testing.T) {
	for id := 0; id < 8; id++ {
		c := mastFixture(t, id, "land")
		jFinishFade(t, c)
		p := c.Rendering
		ship := p.Initialized.Stores.Ships[0]
		for i, s := range ship.Stations {
			got := p.Cockpit.Stations[i]
			if got.SystemID != int(s.CompactID) || got.Present != s.Present || got.Quantity != s.Ammunition || got.Name != s.ShortName {
				t.Fatalf("aircraft%d slot%d typed HUD mismatch%+v /%+v", id, i, s, got)
			}
		}
		row := c.Mission.Final.Aircraft[0]
		if row[0] != byte(id) || p.Initialized.Aircraft[0] != row {
			t.Fatal("store adaptation rewrote accepted aircraft")
		}
		if p.Cockpit.Defense == nil || p.Cockpit.Defense.Chaff != gamestate.RowWord(row[:], 31) || p.Cockpit.Defense.Flares != gamestate.RowWord(row[:], 33) {
			t.Fatal("defense counts disconnected")
		}
	}
}

func TestContactCounterAndTerminalOnlyReturn(t *testing.T) {
	c := mastFixture(t, 0, "land")
	jFinishFade(t, c)
	// Reenter through real accepted settings, not a manually supplied protection
	// bit. The diagnostic descent below isolates the source hard-contact arm.
	act(t, c, Actions{EndMission: true})
	act(t, c, Actions{ConfirmQuit: true})
	waitForResultVerification(t, c)
	c.Mission.Settings.Landings = 1
	startRendering(t, c)
	jFinishFade(t, c)
	p := c.Rendering
	r := p.Flight
	saved := c.Store.State()
	session := c.Mission
	if p.Initialized.Switches&0x20 != 0 {
		t.Fatal("realistic landing still protected")
	}
	r.State.Pose.Height = 3
	r.State.Vertical, r.State.Rotor = -20, 1
	steps := r.Steps
	if err := c.advanceFlight(.25); err != nil {
		t.Fatal(err)
	}
	term := r.State.Terminal
	if term == nil || (term.SourceCode != 2 && term.SourceCode != 8) || term.Countdown != int16(term.SourceCode)-1 || r.Steps != steps+1 {
		t.Fatalf("contact frame/count %+v steps%d", term, r.Steps-steps)
	}
	pose, countdown, fuel, phase := r.State.Pose, term.Countdown, append([]int16(nil), r.Systems.Fuel...), r.Systems.Countdown
	act(t, c, Actions{Quit: true})
	if err := c.AdvanceRendering(1); err != nil {
		t.Fatal(err)
	}
	if term.Countdown != countdown {
		t.Fatal("quit modal advanced terminal counter")
	}
	act(t, c, Actions{Cancel: true})
	if c.Screen != ScreenScene {
		t.Fatal("quit cancellation lost terminal scene")
	}
	for i := 0; i < 12 && c.Screen == ScreenScene; i++ {
		if err := c.AdvanceRendering(.1); err != nil {
			t.Fatal(err)
		}
	}
	waitForResultVerification(t, c)
	if term.Countdown != 0 || c.Screen != ScreenBriefing || p.Initialized.Sortie.Result == nil || p.Initialized.Sortie.Result.Reason != term.Kind {
		t.Fatal("terminal result/temporary return", term, p.Initialized.Sortie, c.Screen)
	}
	if r.State.Pose != pose || !reflect.DeepEqual(r.Systems.Fuel, fuel) || r.Systems.Countdown != phase || r.Steps != steps+1 {
		t.Fatal("terminal-only policy integrated physics/fuel")
	}
	if c.Mission != session || !reflect.DeepEqual(c.Store.State(), saved) {
		t.Fatal("terminal result wrote persistent outcome")
	}
	if candidate, e := c.Store.RecoveryCandidate(); e != nil || candidate != nil {
		t.Fatal("terminal return created recovery", e)
	}
}

func TestFuelOutAcceptedContactReturnsWithoutRefill(t *testing.T) {
	c := mastFixture(t, 0, "land")
	jFinishFade(t, c)
	p := c.Rendering
	r := p.Flight
	session := c.Mission
	saved := c.Store.State()
	r.Systems.Fuel[0] = 0
	r.Systems.Damage[9] = 1
	r.State.Pose.Height = 3
	r.State.Vertical, r.State.Rotor = -20, 1
	blueprint := p.Initialized.Stores.Blueprint[0]
	if err := c.advanceFlight(.1); err != nil {
		t.Fatal(err)
	}
	waitForResultVerification(t, c)
	if c.Screen != ScreenBriefing || p.Initialized.Sortie == nil || p.Initialized.Sortie.Result == nil || p.Initialized.Sortie.Result.Reason != "fuel-out-contact" {
		t.Fatal("fuel-out contact did not transfer", c.Screen, p.Initialized.Sortie, r.State.Terminal)
	}
	if r.State.Terminal != nil || r.Systems.Damage[9] != 1 || r.Systems.Fuel[0] != 0 || p.Initialized.Stores.Blueprint[0] != blueprint {
		t.Fatal("fuel-out contact was treated as refill/crash")
	}
	if c.Mission != session || !reflect.DeepEqual(c.Store.State(), saved) {
		t.Fatal("fuel-out end wrote persistent outcome")
	}
	if backup, e := c.Store.RecoveryCandidate(); e != nil || backup != nil {
		t.Fatal("fuel-out end created recovery", e)
	}
}

func TestOriginalEmptyBayThenSidewinderThroughOrdnance(t *testing.T) {
	b, e := os.ReadFile(testfixture.Path("installation/GS2000.DAT"))
	if e != nil {
		t.Fatal(e)
	}
	// Original Apache SHHDbyte9=2; WEAP record file17917 selects one
	// SIDEWNDR fit on mount1. WSHD1word2=1:2*1*1=2 runtime missiles.
	if b[17319+9] != 2 || !bytes.Equal(b[17917:17922], []byte{1, 1, 0, 1, 0x18}) || !bytes.Equal(b[33145+2:33145+4], []byte{1, 0}) {
		t.Fatal("original Apache/Sidewinder source witness changed")
	}
	c := mastFixture(t, 0, "land")
	jFinishFade(t, c)
	act(t, c, Actions{EndMission: true})
	act(t, c, Actions{ConfirmQuit: true})
	waitForResultVerification(t, c)
	if e = c.openOrdnance(0); e != nil {
		t.Fatal(e)
	}
	choose := func(action string, system, station int) {
		t.Helper()
		for i, v := range c.Mission.Ordnance.Choices {
			if v.Enabled && v.Action == action && (system < 0 || v.System == system) && (station < 0 || v.Station == station) {
				missionClick(t, c, i)
				return
			}
		}
		t.Fatalf("missing actual ORDNANCE choice %s/%d/%d", action, system, station)
	}
	for _, v := range c.Mission.Ordnance.Choices {
		if v.Action == "remove_menu" {
			choose("remove_menu", -1, -1)
			choose("remove_all", -1, -1)
			choose("back", -1, -1)
			break
		}
	}
	choose("fits", 1, -1)
	choose("mount", 1, 1)
	choose("back", -1, -1)
	choose("back", -1, -1)
	if c.Screen != ScreenBriefing {
		t.Fatal("actual acceptance failed", c.Screen, c.Message)
	}
	startRendering(t, c)
	jFinishFade(t, c)
	p := c.Rendering
	if p.Cockpit.Stations[1].SystemID != 1 || p.Cockpit.Stations[1].Name != "EMPTY" || p.Cockpit.Stations[1].Present {
		t.Fatal("first empty bay lost its source row", p.Cockpit.Stations)
	}
	s := p.Cockpit.Stations[2]
	if !s.Present || s.SystemID < 2 || s.Name != "SIDEWNDR" || s.Quantity != 2 {
		t.Fatal("source Sidewinder counter/row mismatch", s)
	}
}
