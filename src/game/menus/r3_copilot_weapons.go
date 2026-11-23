package menus

import (
	"fmt"
	"gunship2000-assets/game/combat"
	"gunship2000-assets/game/diagnostics"
	"log/slog"
)

// advanceCopilotWeapons is source0975:58F8..595B at the late1FAC visit.
// The new interpretation's caller supplies a fresh AB9/B0B input after target
// and view publication. B0B==1 already carries the selected weapon's source
// class, projected-angle and strict range predicates. There is no station choice
// or separate AI policy here. Fire retains559E's ammunition/damage/cooldown/RNG.
func (p *RenderingPreview) advanceCopilotWeapons(in combat.Input) error {
	if p.Initialized.Switches&0x10 == 0 || !in.E0 || in.Phase&7 != 0 || in.Target == 0 || !in.TargetAvailable {
		return nil
	}
	row := int(in.Target) - 1
	if row >= len(p.Initialized.Entities) {
		return fmt.Errorf("copilot target%d outside initialized source domain", in.Target)
	}
	target := p.Initialized.Entities[row]
	if target.Kind&0x30 != 0 || target.Participation != 1 {
		return nil
	}
	r := p.Flight
	if r.Combat.NextSlot > 15 {
		return fmt.Errorf("copilot current projectile slot%d outside0..15", r.Combat.NextSlot)
	}
	//5950 reads anchor+4, the lifetime byte (the normalized payload starts
	// at anchor+2). It tests the CURRENT AC9 slot, before559E increments AC9.
	// A manual release earlier in this visit therefore inhibits this late arm.
	if r.Combat.Slots[r.Combat.NextSlot].Lifetime != 0 {
		return nil
	}
	before := len(r.Combat.Events)
	if err := r.Combat.Fire(in); err != nil {
		return fmt.Errorf("copilot fire: %w", err)
	}
	if diagnostics.Enabled(slog.LevelDebug) {
		for _, event := range r.Combat.Events[before:] {
			if event.Kind == "launch" {
				diagnostics.Debug("copilot", "Automatic selected weapon released", "step", r.Steps, "phase", in.Phase, "target", int(in.Target)-1, "station", in.SelectedStation, "slot", event.Slot, "source", "0975:58F8..595B -> 559E")
			}
		}
	}
	//559E's ordinary launch/mass/salvo/audio publication applies immediately;
	// movement waits for the next ascending projectile pass.
	p.collectCombatEvents()
	return nil
}
