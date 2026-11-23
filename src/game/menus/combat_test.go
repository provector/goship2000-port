package menus

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"gunship2000-assets/game/combat"
	"gunship2000-assets/game/flight"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestEndingAndReentryDiscardTransientCombatOnly(t *testing.T) {
	c := armedMission(t)
	p := c.Rendering
	revision, mission := c.Store.State().Revision, c.Mission.Data.ObjectHash
	ammo := p.Initialized.Stores.Ships[0].Stations[0].Ammunition
	c.Handle(Actions{WeaponTriggerValid: true})
	c.Handle(Actions{WeaponTriggerValid: true, WeaponTriggerHeld: true, InputTime: time.Millisecond})
	if err := c.AdvanceRendering(.1); err != nil {
		t.Fatal(err)
	}
	if p.Flight.Combat.Slots[p.Flight.Combat.LastSlot].Lifetime == 0 {
		t.Fatal("fixture needs active projectile")
	}
	if err := c.Handle(Actions{EndMission: true}); err != nil {
		t.Fatal(err)
	}
	if err := c.Handle(Actions{ConfirmQuit: true}); err != nil {
		t.Fatal(err)
	}
	waitForResultVerification(t, c)
	if c.Screen != ScreenBriefing || c.Store.State().Revision != revision || c.Mission.Data.ObjectHash != mission {
		t.Fatal("ending wrote outcome or regenerated mission")
	}
	startRendering(t, c)
	jFinishFade(t, c)
	r := c.Rendering.Flight
	if r.Combat == p.Flight.Combat || r.Combat.Slots != ([combat.SlotCount]combat.Projectile{}) || r.Combat.Score != 0 || r.Combat.Results != ([4]combat.Result{}) {
		t.Fatal("old projectile/result state leaked into reentry")
	}
	if c.Rendering.Initialized.Stores.Ships[0].Stations[0].Ammunition != ammo || c.Store.State().Revision != revision || c.Mission.Data.ObjectHash != mission {
		t.Fatal("transient consumption persisted to accepted blueprint/save")
	}
}

func TestSalvoHUDSeesPostFireReductionInSameUpdate(t *testing.T) {
	c := armedMission(t)
	p, r := c.Rendering, c.Rendering.Flight
	p.Initialized.SelectedStation = 1
	p.Initialized.Stores.Ships[0].Stations[1].Ammunition = 15
	r.RocketSalvo, p.Cockpit.RocketSalvo = 4, 4
	c.Handle(Actions{WeaponTriggerValid: true})
	c.Handle(Actions{InputTime: time.Millisecond, WeaponTriggerValid: true, WeaponTriggerHeld: true})
	if err := c.AdvanceRendering(.1); err != nil {
		t.Fatal(err)
	}
	// Paired station launches use4+4, leaving7.5886 requires at least2*AC1,
	// so the same source frame lowers4→2 before the later HUD consumer.
	if p.Initialized.Stores.Ships[0].Stations[1].Ammunition != 7 || r.RocketSalvo != 2 || p.Cockpit.RocketSalvo != 2 {
		t.Fatal("post-fire AC1 publication", r.RocketSalvo, p.Cockpit.RocketSalvo)
	}
}

func TestStartHeldTriggerCannotFireThroughAPUHandoff(t *testing.T) {
	c := armedMission(t)
	p := c.Rendering
	ammo := p.Initialized.Stores.Ships[0].Stations[0].Ammunition
	// Newly created flight owner has seen no released trigger sample yet.
	if err := c.Handle(Actions{WeaponTriggerValid: true, WeaponTriggerHeld: true}); err != nil {
		t.Fatal(err)
	}
	if err := c.AdvanceRendering(.2); err != nil {
		t.Fatal(err)
	}
	if p.Initialized.Stores.Ships[0].Stations[0].Ammunition != ammo {
		t.Fatal("held Start/menu activation fired on handoff")
	}
	c.Handle(Actions{InputTime: 200 * time.Millisecond, WeaponTriggerValid: true})
	c.AdvanceRendering(.1)
	c.Handle(Actions{InputTime: 300 * time.Millisecond, WeaponTriggerValid: true, WeaponTriggerHeld: true})
	if err := c.AdvanceRendering(.1); err != nil {
		t.Fatal(err)
	}
	if p.Initialized.Stores.Ships[0].Stations[0].Ammunition != ammo-20 {
		t.Fatal("fresh firing press after handoff release suppressed")
	}
}

func TestProjectileExpirySnapsPresentationWithoutResettingFlight(t *testing.T) {
	c := armedMission(t)
	p, r := c.Rendering, c.Rendering.Flight
	c.Handle(Actions{WeaponTriggerValid: true})
	r.Combat.NextSlot, r.Combat.LastSlot = 1, 1
	r.Combat.Slots[1] = combat.Projectile{Lifetime: 1, Template: 1, Pose: p.Initialized.Player}
	if err := c.Handle(Actions{ViewKey: 9}); err != nil {
		t.Fatal(err)
	}
	if p.ViewControls.Mode != 6 {
		t.Fatal("live F9 guard")
	}
	r.Combat.Slots[1].Lifetime, r.Combat.Slots[1].Packed = 0, 1
	steps := r.Steps
	if err := c.AdvanceRendering(.1); err != nil {
		t.Fatal(err)
	}
	if p.ViewControls.Mode != 0 || r.Steps != steps+1 {
		t.Fatal("expiry changed logical scheduling", p.ViewControls.Mode, r.Steps)
	}
	if p.presentation.previous.Camera != p.presentation.current.Camera {
		t.Fatal("projectile return interpolated across a discontinuity")
	}
	q := p.takeSceneAudio()
	if len(q) != 2 || q[0].event != 0x17 || q[1].event != 0xb {
		t.Fatal("F9/rotor handoff order", q)
	}
}

func TestNaturalStationaryTargetDestructionAndWreck(t *testing.T) {
	c := armedMission(t)
	p := c.Rendering
	r := p.Flight
	// This retained Step M leaf fixture pins its historical actor interpretation:
	// one source damage draw, before kind21 movement was admitted. P.3/P.4
	// tests separately verify shared formation/logistics draws and full combat.
	r.Combat.SetFormationEnabled(false)
	r.Combat.SetLogisticsEnabled(false)
	row := -1
	for i, e := range p.Initialized.Entities {
		if i > 4 && e.Participation == 1 && e.Object.Raw[0]&0x10 != 0 && e.Kind == 4 && int16(binary.LittleEndian.Uint16(e.Object.Raw[29:])) > 0 {
			row = i
			break
		}
	}
	if row < 0 {
		t.Fatal("natural primary stationary target missing")
	}
	e := p.Initialized.Entities[row]
	original, replacement := e.ModelID, e.ReplacementModelID
	value := int16(binary.LittleEndian.Uint16(e.Object.Raw[27:]))
	initialScore := r.Combat.Score
	// Explicit supplied firing pose, not a natural first frame: the target's
	// generated identity/placement/threshold, actual world and accepted stores
	// remain untouched. Independent source leaves govern trajectory and damage.
	r.State.Pose = flight.Pose{H0: e.Pose.H0, H1: e.Pose.H1 - 3000, Height: int16(e.Pose.Height + 256)}
	p.publishFlight()
	p.Initialized.SelectedStation = 2
	r.Avionics.Target = uint16(row + 1)
	p.Cockpit.TargetSymbol = 1
	before, _ := json.Marshal(c.Mission.Data)
	if err := p.ViewControls.Tick(p.cameraInputs(), p.Renderer.Assets); err != nil {
		t.Fatal(err)
	}
	p.publishView()
	p.resetPresentation()
	mastCapture(t, c, os.Getenv("GS2000_M_DESTRUCTION_CAPTURE"), "supplied-pose-natural-target-before")
	draws := 0
	random := func() uint16 { draws++; return 1 }
	in := p.combatInput(6, 1, random)
	if err := r.Combat.Fire(in); err != nil {
		t.Fatal(err)
	}
	hit, wreck, pending := false, false, false
	for n := 1; n <= 100; n++ {
		in.Phase = uint16(n)
		if err := p.advanceCombat(in); err != nil {
			t.Fatal(err)
		}
		p.publishView()
		p.filterSceneInstances()
		p.advanceCombatNotifications(true)
		p.resetPresentation()
		c.Frame(false)
		if p.Error != nil {
			t.Fatal(p.Error)
		}
		v := p.Initialized.Entities[row]
		if v.Kind == 0x1c && v.Participation == 0 {
			pending = true
			for _, in := range p.Scene.Instances {
				if in.ID == v.ID {
					t.Fatal("8899 participation0 was drawn before wreck initialization")
				}
			}
		}
		if !hit && v.Participation == 2 {
			hit = true
			mastCapture(t, c, os.Getenv("GS2000_M_DESTRUCTION_CAPTURE"), "supplied-pose-natural-target-hit")
		}
		if v.Kind == 0x1c && v.Participation == 4 {
			wreck = true
			mastCapture(t, c, os.Getenv("GS2000_M_DESTRUCTION_CAPTURE"), "supplied-pose-natural-target-wreck")
			break
		}
	}
	if !hit || !pending || !wreck || draws != 1 {
		t.Fatal("natural hit/death lifecycle", hit, pending, wreck, draws)
	}
	if p.Initialized.Entities[row].ModelID != replacement || original == replacement {
		t.Fatal("source replacement visual was not applied")
	}
	if r.Combat.Results[3].Kind != 1 || int(r.Combat.Results[3].Target) != row {
		t.Fatal("exactly-once transient result", r.Combat.Results)
	}
	// Independent0975:9BF4..9BFD word reader: this generated target contributes its
	// actual signed OBJECT value once, not a value copied from an older seed.
	wantScore := int16(uint16(initialScore) + uint16(value))
	credit := r.Combat.Credit[row>>1]
	if row&1 == 0 {
		credit &= 15
	} else {
		credit >>= 4
	}
	if r.Combat.Score != wantScore || credit != 1 {
		t.Fatal("actual target signed value/player destruction credit", value, initialScore, r.Combat.Score, credit)
	}
	t.Logf("supplied-pose leaf: world%s seed%d row%d value%d score%d->%d credit%d damageDraws%d", p.Initialized.World, c.Mission.Data.Inputs.Seed, row, value, initialScore, r.Combat.Score, credit, draws)
	if r.Avionics.Contacts[row].Kind != 0x1c || r.Avionics.Contacts[row].Flags&7 != 1 {
		t.Fatal("DD92 wreck observation")
	}
	after, _ := json.Marshal(c.Mission.Data)
	if !bytes.Equal(before, after) {
		t.Fatal("destruction rewrote immutable mission papers")
	}
	results := r.Combat.Results
	if err := r.Combat.ApplyStationaryHit(row, 0, p.Initialized.Stores.Ships[0].Stations[2].CompactID, 1, in); err != nil {
		t.Fatal(err)
	}
	if draws != 1 || r.Combat.Results != results {
		t.Fatal("repeated hit credited dead object")
	}
}

// Real accepted loadout transactions on the retained natural mission. This is
// reusable only by tests; production never substitutes a diagnostic population.
func armedMission(t *testing.T) *FrontEnd {
	t.Helper()
	c := mastFixture(t, 0, "pass")
	jFinishFade(t, c)
	if err := c.returnRenderingBriefing(); err != nil {
		t.Fatal(err)
	}
	if err := c.openOrdnance(0); err != nil {
		t.Fatal(err)
	}
	choose := func(action string, system, station int) {
		t.Helper()
		for i, v := range c.Mission.Ordnance.Choices {
			if v.Enabled && v.Action == action && (system < 0 || v.System == system) && (station < 0 || v.Station == station) {
				missionClick(t, c, i)
				return
			}
		}
		t.Fatalf("missing accepted choice %s/%d/%d", action, system, station)
	}
	for _, v := range []struct{ system, station int }{{7, 0}, {4, 1}} {
		choose("fits", v.system, -1)
		choose("mount", v.system, v.station)
		choose("back", -1, -1)
	}
	choose("back", -1, -1)
	if c.Screen == ScreenOutfit {
		missionClick(t, c, 6)
	}
	if c.Screen != ScreenBriefing {
		t.Fatalf("source loadout acceptance: %s %s", c.Screen, c.Message)
	}
	startRendering(t, c)
	jFinishFade(t, c)
	return c
}

func TestAcceptedMixedWeaponLoadout(t *testing.T) {
	c := armedMission(t)
	s := c.Rendering.Initialized.Stores.Ships[0]
	if s.Stations[0].RawWSHDID != 16 || s.Stations[1].RawWSHDID != 7 || s.Stations[2].RawWSHDID != 4 {
		t.Fatal("accepted weapon domains", s.Stations)
	}
	for _, i := range []int{0, 1, 2} {
		if s.Stations[i].Ammunition <= 0 {
			t.Fatal("empty test loadout", i)
		}
	}
}

func TestNormalPreflightHeldTriggerAndSuspension(t *testing.T) {
	c := mastFixture(t, 0, "land")
	jFinishFade(t, c)
	p, r := c.Rendering, c.Rendering.Flight
	if err := c.Handle(Actions{WeaponTriggerValid: true}); err != nil {
		t.Fatal(err)
	}
	p.Initialized.SelectedStation = 0
	ammo := p.Initialized.Stores.Ships[0].Stations[0].Ammunition
	if ammo < 20 {
		t.Fatal("natural Apache cannon fixture has no ammunition")
	}
	if err := c.Handle(Actions{InputTime: time.Millisecond, WeaponTriggerValid: true, WeaponTriggerHeld: true}); err != nil {
		t.Fatal(err)
	}
	if err := c.AdvanceRendering(.1); err != nil {
		t.Fatal(err)
	}
	if got := p.Initialized.Stores.Ships[0].Stations[0].Ammunition; got != ammo-20 {
		t.Fatal("source559e twenty-round burst", got, ammo)
	}
	var commands []uint16
	for _, q := range p.takeSceneAudio() {
		commands = append(commands, q.event)
	}
	found := false
	for _, q := range commands {
		found = found || q == 7
	}
	if !found {
		t.Fatal("86e6 first cannon update did not emit actual ASOUND07", commands)
	}
	raw, rng, updates := p.combatRaw(), p.Initialized.RandomState, r.Combat.Updates
	for range 4 {
		p.Dirty = true
		c.Frame(false)
	}
	if !bytes.Equal(raw, p.combatRaw()) || rng != p.Initialized.RandomState || updates != r.Combat.Updates || len(p.takeSceneAudio()) != 0 {
		t.Fatal("drawing advanced combat/audio/RNG")
	}
	if err := c.Handle(Actions{InputTime: 2 * time.Millisecond, WeaponTriggerValid: true}); err != nil {
		t.Fatal(err)
	}
	if err := c.Handle(Actions{FlightPause: true}); err != nil {
		t.Fatal(err)
	}
	if err := c.AdvanceRendering(2); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, p.combatRaw()) || updates != r.Combat.Updates {
		t.Fatal("pause advanced projectile state")
	}
	if err := c.Handle(Actions{AnyKey: true}); err != nil {
		t.Fatal(err)
	}
	if err := c.Handle(Actions{WeaponTriggerValid: true}); err != nil {
		t.Fatal(err)
	}
	if err := c.Handle(Actions{GPSOpen: true}); err != nil {
		t.Fatal(err)
	}
	if p.GPS == nil {
		t.Fatal("GPS did not open")
	}
	for range 20 {
		if err := c.AdvanceRendering(.1); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(raw, p.combatRaw()) || updates != r.Combat.Updates {
		t.Fatal("GPS advanced combat")
	}
	c.closeGPS()
	if err := c.Handle(Actions{WeaponTriggerValid: true, WeaponTriggerHeld: true}); err != nil {
		t.Fatal(err)
	}
	if err := c.AdvanceRendering(.1); err != nil {
		t.Fatal(err)
	}
	if got := p.Initialized.Stores.Ships[0].Stations[0].Ammunition; got != ammo-20 {
		t.Fatal("GPS return fired a stale held key", got)
	}
}

func TestCombatTimedInputIndependentOfPresentation(t *testing.T) {
	type witness struct {
		Flight, Initialized, Combat []byte
		Mass                        int16
		Events                      any
	}
	var want *witness
	for _, mode := range []string{"smooth", "stepped"} {
		for _, scale := range []int{1, 2} {
			for _, fps := range []int{30, 60, 144} {
				t.Run(fmt.Sprintf("%s-%d-%d", mode, scale, fps), func(t *testing.T) {
					c := armedMission(t)
					p, r := c.Rendering, c.Rendering.Flight
					// Supplied hover pose near an unchanged generated target. This
					// schedule exercises actual damage/credit RNG, not only empty fire.
					e := p.Initialized.Entities[17]
					r.State.Pose = flight.Pose{H0: e.Pose.H0, H1: e.Pose.H1 - 3000, Height: int16(e.Pose.Height + 256)}
					r.State.Forward, r.State.Rotor, r.State.Collective, r.State.Hover, r.State.HeightControl, r.State.HeightTarget, r.State.CruiseSpeed = 0, 1, 16, 1, 1, int16(e.Pose.Height+256), 0
					r.Avionics.Target = 18
					p.publishFlight()
					if err := p.ViewControls.Tick(p.cameraInputs(), p.Renderer.Assets); err != nil {
						t.Fatal(err)
					}
					p.publishView()
					p.Scene.RenderScale = scale
					p.presentation.mode = mode
					p.Frame, p.worldFrame = make([]byte, p.Scene.TargetPixels()), make([]byte, p.Scene.TargetPixels())
					p.resetPresentation()
					p.Initialized.SelectedStation = 0
					if err := c.Handle(Actions{WeaponTriggerValid: true}); err != nil {
						t.Fatal(err)
					}
					// Timestamped physical-state changes are supplied independently of
					// host draws.559e is polled once per authoritative update while held.
					r.commandInput.SampleAt(100*time.Millisecond, Actions{WeaponTriggerValid: true, WeaponTriggerHeld: true})
					r.commandInput.SampleAt(650*time.Millisecond, Actions{WeaponTriggerValid: true})
					last := time.Duration(0)
					for n := 1; n <= fps*3; n++ {
						now := time.Duration(n) * time.Second / time.Duration(fps)
						if err := c.AdvanceRendering(float64(now-last) / float64(time.Second)); err != nil {
							t.Fatal(err)
						}
						last = now
						p.Dirty = true
						c.Frame(false)
						if p.Error != nil {
							t.Fatal(p.Error)
						}
					}
					f, _ := json.Marshal(r.State)
					destroyed := false
					for _, e := range r.CombatEvents {
						destroyed = destroyed || e.Kind == "destroyed"
					}
					if !destroyed {
						t.Fatal("fixture did not reach damage/destruction")
					}
					i, _ := json.Marshal(p.Initialized)
					v := witness{f, i, p.combatRaw(), r.MassTerm, r.CombatEvents}
					if want == nil {
						want = &v
					} else if !reflect.DeepEqual(*want, v) {
						t.Fatal("presentation changed authoritative combat/flight/RNG/event ordering")
					}
				})
			}
		}
	}
}

func TestTriggerTimestampReleaseAndModalReset(t *testing.T) {
	q := flightCommandInput{}
	q.SampleAt(10*time.Millisecond, Actions{WeaponTriggerValid: true, WeaponTriggerHeld: true})
	q.SampleAt(110*time.Millisecond, Actions{WeaponTriggerValid: true})
	q.NextAt(9 * time.Millisecond)
	if q.Trigger {
		t.Fatal("future trigger observed")
	}
	q.NextAt(100 * time.Millisecond)
	if !q.Trigger {
		t.Fatal("held trigger lost")
	}
	q.NextAt(200 * time.Millisecond)
	if q.Trigger {
		t.Fatal("release queued as repeated firing")
	}
	q.SampleAt(210*time.Millisecond, Actions{WeaponTriggerValid: true, WeaponTriggerHeld: true})
	q.NextAt(300 * time.Millisecond)
	q.Reset()
	q.NextAt(time.Second)
	if q.Trigger {
		t.Fatal("modal/focus reset retained firing")
	}
}
