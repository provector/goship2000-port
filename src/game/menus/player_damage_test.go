package menus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"reflect"
	"testing"
)

func playerDamageResume(t *testing.T, c *FrontEnd) {
	t.Helper()
	if err := c.Handle(Actions{Cancel: true}); err != nil {
		t.Fatal(err)
	}
	if err := c.AdvanceRendering(.12); err != nil {
		t.Fatal(err)
	}
}

func TestAcceptedDamageReachesFlightCockpitGPSAndResultBoundary(t *testing.T) {
	c := developmentScenarioController(t, "damage")
	p, r := c.Rendering, c.Rendering.Flight
	before := c.Store.State()
	if len(r.acceptedPlayerHits) != 3 {
		t.Fatal("named accepted-hit plan missing")
	}
	playerDamageResume(t, c)
	// Independent fixture input: Apache selection uses its second draw. Supplied
	// seeds0140/0010/0320 produce selection80/4/200, then mask20 and crack132,58.
	want := [12]byte{0x21, 0, 0, 0, 1, 0, 0, 0, 0, 0, 1, 0}
	if got := gamestate.PlayerDamage12(&r.Systems); got != want || r.Systems.EngineTargets[0] != 7 || r.DamageOverlay[0] != [2]int16{132, 58} || r.DamageHealth != 229 {
		t.Fatal("source damage consumers", got, r.Systems.EngineTargets, r.DamageOverlay, r.DamageHealth)
	}
	if len(r.DamageEvents) != 3 || r.Combat.Results[3].Kind != 2 || r.Combat.Results[3].Target != 11 {
		t.Fatal("accepted hit/event2 ownership")
	}
	if r.Systems.Damage != p.Initialized.Stores.Damage[p.Initialized.PlayerRow] || p.Cockpit.Damage[0] != 0x21 || p.Cockpit.Damage[4] != 1 || p.Cockpit.DamageOverlay != 1 {
		t.Fatal("authoritative display/store publication")
	}
	if err := c.openGPS(); err != nil {
		t.Fatal(err)
	}
	panel := *p.GPS.Panel
	if !panel.DamageKnown || panel.Damage[0] != 0x21 || panel.Damage[4] != 1 || panel.Damage[9] != 0 {
		t.Fatal("GPS ten-byte source domain", panel.Damage)
	}
	rng, steps, events := p.Initialized.RandomState, r.Steps, len(r.DamageEvents)
	first := append([]byte(nil), c.Frame(false)...)
	for i := 0; i < 8; i++ {
		if err := c.AdvanceRendering(.1); err != nil {
			t.Fatal(err)
		}
		c.Frame(false)
	}
	if p.Error != nil || p.Initialized.RandomState != rng || r.Steps != steps || len(r.DamageEvents) != events || *p.GPS.Panel != panel || !bytes.Equal(first, c.Frame(false)) {
		t.Fatal("GPS advanced damage/RNG or changed immutable snapshot", p.Error)
	}
	if err := c.handleGPS(Actions{Cancel: true}); err != nil {
		t.Fatal(err)
	}
	// Damage still affects the original flight model and instrument producer;
	// no tuning or separate renderer position is used.
	for i := 0; i < 24; i++ {
		if err := c.AdvanceRendering(.1); err != nil {
			t.Fatal(err)
		}
	}
	if r.Systems.Engines[0] != 7 || r.Systems.EngineTargets[1] != 15 || r.State.Terminal != nil {
		t.Fatal("engine ordinal target did not reach its live consumer", r.Systems.Engines)
	}
	p.Initialized.EndSortieForQoL()
	if err := c.finishSortieResult(); err != nil {
		t.Fatal(err)
	}
	end := p.Initialized.Sortie.Result
	if c.Screen != ScreenDebrief || !reflect.DeepEqual(before.SelectedPilot(), c.Store.State().SelectedPilot()) || !end.DamageKnown || end.PlayerDamage != want || end.EncodedHealth != 229 || end.SourceStatus&0x80 != 0 {
		t.Fatal("damaged outcome not staged/frozen", c.Screen, c.Message, end)
	}
	r.Systems.Damage[2] = 2
	if end.PlayerDamage != want {
		t.Fatal("final damage facts alias mutable systems")
	}
}

func TestFuelDamageTerminalOrderFreezeAndNoCareer(t *testing.T) {
	c := developmentScenarioControllerProfile(t, "crash", replay.V45Compatibility)
	p, r := c.Rendering, c.Rendering.Flight
	before := c.Store.State()
	playerDamageResume(t, c)
	if r.State.Terminal == nil || r.State.Terminal.Status != 0x10 || r.State.Terminal.SourceCode != 0 || r.State.Terminal.Countdown != 3 || r.TerminalPresentation.PixelCount != 8191 || r.Systems.Damage[3] != 2 {
		t.Fatal("fuel hit/status/countdown ownership", r.State.Terminal, r.TerminalPresentation)
	}
	pose, updates := r.State.Pose, r.Combat.Updates
	rng := p.Initialized.RandomState
	first := append([]byte(nil), c.Frame(false)...)
	c.Frame(false)
	if p.Error != nil || !bytes.Equal(first, c.Frame(false)) || p.Initialized.RandomState != rng {
		t.Fatal("terminal redraw consumed state", p.Error)
	}
	for i := 0; i < 4 && c.Screen == ScreenScene; i++ {
		if err := c.AdvanceRendering(.12); err != nil {
			t.Fatal(err)
		}
	}
	end := p.Initialized.Sortie.Result
	if c.Screen != ScreenDebrief || end == nil || end.Reason != "damage-fuel" || end.SourceStatus&0x3c != 0x10 || end.SourceStatus&0x80 != 0 || !end.DamageKnown || !reflect.DeepEqual(before.SelectedPilot(), c.Store.State().SelectedPilot()) || r.Combat.Updates != updates || r.State.Pose != pose {
		t.Fatal("terminal manufactured loss/career or advanced projectiles", c.Screen, end)
	}
	if r.TerminalPresentation.Frames != 4 || r.TerminalPresentation.RNGCalls != 68263 || r.State.Terminal.Countdown != 0 {
		t.Fatal("literal32767/n terminal sequence", r.TerminalPresentation)
	}
	if err := p.QueueAcceptedPlayerHit(AcceptedPlayerHit{VictimRow: 0}); err == nil {
		t.Fatal("ended sortie accepted a future hit")
	}
	for i := 0; i < 4; i++ {
		if err := c.AdvanceRendering(.5); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(before.SelectedPilot(), c.Store.State().SelectedPilot()) || p.Initialized.Sortie.Result != end || r.Combat.Updates != updates {
		t.Fatal("repeat ending advanced credit/commit")
	}
}

func TestAcceptedHitBoundaryAndProtectionUsesSharedRNG(t *testing.T) {
	c := developmentScenarioController(t, "damage")
	p, r := c.Rendering, c.Rendering.Flight
	r.acceptedPlayerHits = nil
	if err := p.QueueAcceptedPlayerHit(AcceptedPlayerHit{VictimRow: 1}); err == nil {
		t.Fatal("wrong actor accepted")
	}
	if err := p.QueueAcceptedPlayerHit(AcceptedPlayerHit{VictimRow: 0, ProjectileFlags: 0x80}); err == nil {
		t.Fatal("unrecovered incoming flags accepted")
	}
	p.Initialized.Switches |= 0x400
	p.Initialized.RandomState = 0x100
	p.Scene.Environment.RNG = 0x100
	if err := p.QueueAcceptedPlayerHit(AcceptedPlayerHit{VictimRow: 0, ProjectileFlags: 0x40, Source: "labelled supplied accepted family3 consumer"}); err != nil {
		t.Fatal(err)
	}
	if err := p.advanceAcceptedPlayerHits(6); err != nil {
		t.Fatal(err)
	}
	//7468 returns0 without RNG;95ca nevertheless draws once for flag40 (<200),
	// then its second7468 call is also suppressed. No event2 for packed return0.
	if p.Initialized.RandomState != 0x80 || r.DamageEvents[0].Second == nil || r.DamageEvents[0].PackedCode != 0 || r.Combat.Results[3].Kind != 0 || gamestate.PlayerDamage12(&r.Systems) != [12]byte{} {
		t.Fatal("protection versus caller RNG/event ownership", r.DamageEvents)
	}
}

func TestPowerLoss609WarnsOnceAndReachesOriginalFlight(t *testing.T) {
	c := developmentScenarioController(t, "power-loss")
	p, r := c.Rendering, c.Rendering.Flight
	before := c.Store.State()
	playerDamageResume(t, c)
	if r.Systems.Damage[2] != 2 || r.Systems.Damage[7] != 2 || r.Systems.Damage[9] != 1 || r.Systems.EngineTargets != [2]int16{} || r.SpeechEnabled || r.State.Terminal != nil || r.DamageHealth != 203 {
		t.Fatal("dual engine/609 is not the separate fuel-status countdown", r.Systems, r.State.Terminal)
	}
	count := 0
	for _, q := range p.takeSceneAudio() {
		if q.event == 0x3d {
			count++
		}
	}
	if count != 1 {
		t.Fatal("source3d warning count", count)
	}
	if err := c.AdvanceRendering(.3); err != nil {
		t.Fatal(err)
	}
	if r.State.HeightControl != 0 || r.State.Hover != 0 {
		t.Fatal("609 did not reach original flight control/lift consumer")
	}
	for _, q := range p.takeSceneAudio() {
		if q.event == 0x3d {
			t.Fatal("warning repeated without a producer")
		}
	}
	if !reflect.DeepEqual(before, c.Store.State()) {
		t.Fatal("mechanical power loss wrote a career outcome")
	}
	// Supplied accepted support after the real damage events exercises the
	// shared609 contact consumer, without relabelling positive fuel as fuel-out.
	r.State.ValidContact = true
	if serviced, err := p.Initialized.AdvanceService(c.Mission.Data, &r.Systems); err != nil || serviced || p.Initialized.Sortie.Result == nil || p.Initialized.Sortie.Result.Reason != "disabled-contact" {
		t.Fatal("disabled contact was refuelled or mislabeled", serviced, err, p.Initialized.Sortie)
	}
	if err := c.finishSortieResult(); err != nil {
		t.Fatal(err)
	}
	if c.Screen != ScreenDebrief || !reflect.DeepEqual(before.SelectedPilot(), c.Store.State().SelectedPilot()) {
		t.Fatal("609 contact fabricated persistent loss")
	}
}

func TestDamageAndTerminalIndependentOfPresentation(t *testing.T) {
	for _, name := range []string{"damage", "crash"} {
		var expected []byte
		for _, tc := range []struct {
			mode       string
			scale, fps int
		}{{"smooth", 1, 30}, {"smooth", 2, 144}, {"stepped", 1, 60}, {"stepped", 2, 30}} {
			t.Run(fmt.Sprintf("%s/%s/%d/%d", name, tc.mode, tc.scale, tc.fps), func(t *testing.T) {
				c := developmentScenarioController(t, name)
				p, r := c.Rendering, c.Rendering.Flight
				p.Scene.RenderScale = tc.scale
				p.presentation.mode = tc.mode
				p.resetPresentation()
				if err := c.Handle(Actions{Cancel: true}); err != nil {
					t.Fatal(err)
				}
				for frame := 0; frame < tc.fps/2; frame++ {
					if err := c.AdvanceRendering(1 / float64(tc.fps)); err != nil {
						t.Fatal(err)
					}
					c.Frame(false)
				}
				got, err := json.Marshal(struct {
					State   flight.State
					Damage  [12]byte
					Mass    int16
					RNG     uint16
					Events  []PlayerHitWitness
					End     *gamestate.TransientSortieResult
					Combat  uint64
					Elapsed uint16
				}{*r.State, gamestate.PlayerDamage12(&r.Systems), r.MassTerm, p.Initialized.RandomState, r.DamageEvents, p.Initialized.Sortie.Result, r.Combat.Updates, r.ElapsedDA})
				if err != nil {
					t.Fatal(err)
				}
				if expected == nil {
					expected = got
				} else if !bytes.Equal(expected, got) {
					t.Fatal("authoritative damage/flight/mission/RNG changed with presentation")
				}
			})
		}
	}
}

func TestDestroyedFuelSupplyContinuesToActualContact(t *testing.T) {
	for _, mode := range []uint16{0, 4, 0x20, 0x24} {
		c := developmentScenarioController(t, "crash")
		p, r := c.Rendering, c.Rendering.Flight
		p.Initialized.Switches = (p.Initialized.Switches &^ 0x24) | mode
		r.State.Pose.Height += 120
		r.State.RadarAltitude += 120
		r.State.ValidContact = false
		playerDamageResume(t, c)
		if !p.correctiveFlight || r.Systems.Damage[3] != 2 || r.State.Terminal != nil {
			t.Fatal(mode, r.Systems.Damage, r.State.Terminal)
		}
		for i := 0; i < 500 && c.Screen == ScreenScene; i++ {
			if err := c.AdvanceRendering(.11); err != nil {
				t.Fatal(mode, err)
			}
		}
		if p.Initialized.Sortie.Result == nil || p.Initialized.Sortie.Result.Reason == "damage-fuel" {
			t.Fatal(mode, "failed to reach physical contact", r.State.Pose, r.State.Vertical, r.State.Terminal)
		}
		t.Logf("switches=%x actual outcome=%s", mode, p.Initialized.Sortie.Result.Reason)
	}
}
