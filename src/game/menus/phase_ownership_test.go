package menus

import (
	"encoding/json"
	"gunship2000-assets/game/flight"
	"testing"
)

// Supplied phase boundary, separate from the natural hostile-release sorties.
func TestPhysicalLatchExpirySkippedConsumerAndHUD(t *testing.T) {
	p, in, _ := weaponFixture(t, 0x0e10)
	r := p.Flight
	p.copilotMechanics = true
	r.Defenses = flight.NewDefenses(30, 30)
	r.Defenses.EnableR3()
	r.Combat.SetPersistentPlayerThreat(true)
	r.Combat.IncomingThreat = 0x80 // already accumulated by an earlier projectile
	// An empty following projectile pass cannot erase a skipped physical latch.
	if err := r.Combat.Step(in); err != nil {
		t.Fatal(err)
	}
	for _, v := range []struct {
		e0     bool
		phase  byte
		damage byte
		mask   uint16
	}{
		{true, 3, 0, 8}, {false, 4, 0, 8}, {true, 4, 0x40, 8}, {true, 4, 0, 0},
	} {
		r.Systems.DamageMasks[0], p.Initialized.Switches = v.damage, v.mask
		if err := p.advanceCopilotDefense(v.e0, v.phase, 2, nil); err != nil {
			t.Fatal(err)
		}
		if r.Combat.IncomingThreat != 0x80 || r.Defenses.Flares != 30 || r.Defenses.FlareTimer != 0 {
			t.Fatal("skipped consumer changed latch/stores", v, r.Defenses)
		}
	}
	r.Systems.DamageMasks[0], p.Initialized.Switches = 0, 8
	if err := p.advanceCopilotDefense(true, 4, 2, nil); err != nil {
		t.Fatal(err)
	}
	if r.Combat.IncomingThreat != 0 || r.Defenses.Flares != 27 || r.Defenses.FlareTimer != 10 || p.Cockpit.Defense.Flares != 27 {
		t.Fatal("late consume/publication", r.Defenses, p.Cockpit.Defense)
	}
	if len(r.sceneAudio) != 1 || len(r.CopilotDefense) != 1 {
		t.Fatal("once-only cue/receipt", r.sceneAudio, r.CopilotDefense)
	}
	if err := p.advanceCopilotDefense(true, 6, 2, nil); err != nil {
		t.Fatal(err)
	}
	if r.Defenses.Flares != 27 || len(r.sceneAudio) != 1 {
		t.Fatal("absent request repeated spending")
	}
	// Explicitly retained reader profile owns the old per-pass reset.
	r.Combat.SetPersistentPlayerThreat(false)
	r.Combat.IncomingThreat = 0x80
	if err := r.Combat.Step(in); err != nil || r.Combat.IncomingThreat != 0 {
		t.Fatal("historical reset", err)
	}
}

func TestFocusCleanupPreservesAutomaticEffectiveState(t *testing.T) {
	p, _, _ := weaponFixture(t, 0x0e10)
	p.copilotMechanics = true
	r := p.Flight
	r.Defenses = flight.NewDefenses(0, 0)
	r.Defenses.EnableR3()
	r.Combat.IncomingThreat = 0xc0
	p.Initialized.Switches = 8
	draws := 0
	if err := p.advanceCopilotDefense(true, 2, 2, func() uint16 { draws++; return 5 }); err != nil {
		t.Fatal(err)
	}
	if err := r.Defenses.SetR3ManualHeld([2]bool{true, true}, 0, nil); err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(r.Defenses)
	p.clearFlightInput()
	if r.Failure != "" || r.Defenses.R3.ManualHeld != [2]bool{} || r.Defenses.IRActive != 64 || r.Defenses.RadarSignature != 6 || p.Cockpit.Defense.IRActive != 64 || p.Cockpit.Defense.RadarSignature != 6 || draws != 1 {
		t.Fatal("focus cleanup erased automatic intent/effective HUD", string(before), r.Defenses, p.Cockpit.Defense, r.Failure)
	}
	// Damage suppresses publication immediately while preserving charge/owners.
	r.Systems.DamageMasks[0] = 0x40
	p.publishDefenseState()
	if p.Cockpit.Defense.IRActive != 0 || p.Cockpit.Defense.RadarSignature != 0 || r.Defenses.IRCharge != 64 || r.Defenses.R3.Automatic != [2]bool{true, true} {
		t.Fatal("equipment owner/publication", r.Defenses, p.Cockpit.Defense)
	}
}
