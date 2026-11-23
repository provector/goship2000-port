package menus

import (
	"fmt"
	"gunship2000-assets/game/cockpit"
	"gunship2000-assets/game/diagnostics"
	"gunship2000-assets/game/flight"
	"log/slog"
)

// CopilotDefenseWitness is a bounded diagnostic receipt, excluded from film
// checkpoints. Automatic state itself is in Defenses and the physical AEB latch.
type CopilotDefenseWitness struct {
	Step                 uint64
	Phase                byte
	Threats              uint16
	ChaffBefore, Chaff   int16
	FlaresBefore, Flares int16
	Result               flight.R3DefenseResult
}

func (p *RenderingPreview) advanceCopilotDefense(e0 bool, phase, df byte, random func() uint16) error {
	r := p.Flight
	before := r.Defenses
	requests := r.Combat.IncomingThreat // Never OR physical held buttons into AEB.
	out, err := r.Defenses.ApplyR3Automatic(flight.R3DefenseRequest{
		Switches: p.Initialized.Switches, Damage: r.Systems.DamageMasks[0], E0: e0,
		Phase: phase, Threats: requests, Pose: r.State.Pose, DF: df,
		Angles: p.Renderer.Assets, SoundEnabled: true, // source DE40; host mute is separate
	}, random)
	if err != nil {
		return fmt.Errorf("late player copilot defense: %w", err)
	}
	if out.ConsumedAEB {
		r.Combat.IncomingThreat = 0 // Sole reached5B62 clear; skipped visits retain it.
	}
	for _, cue := range out.SoundRequests {
		r.sceneAudio = append(r.sceneAudio, sceneAudioCommand{event: uint16(cue)})
	}
	changed := len(out.Deployments) != 0 || before.R3.Automatic != r.Defenses.R3.Automatic
	if changed {
		r.CopilotDefense = append(r.CopilotDefense, CopilotDefenseWitness{r.Steps, phase, requests, before.Chaff, r.Defenses.Chaff, before.Flares, r.Defenses.Flares, out})
		if len(r.CopilotDefense) > 64 {
			r.CopilotDefense = r.CopilotDefense[len(r.CopilotDefense)-64:]
		}
		if diagnostics.Enabled(slog.LevelDebug) {
			diagnostics.Debug("copilot", "Automatic defense state changed", "step", r.Steps, "phase", phase, "physical_threat", requests,
				"chaff_before", before.Chaff, "chaff", r.Defenses.Chaff, "flares_before", before.Flares, "flares", r.Defenses.Flares,
				"ir_active", r.Defenses.IRActive, "radar_token", r.Defenses.RadarSignature, "source", "0975:595C..5B62")
		}
	}
	p.publishDefenseState() // New stores/timers/effective words precede this HUD.
	return nil
}

func (p *RenderingPreview) publishDefenseState() {
	r, m := p.Flight, p.Initialized
	if r == nil || m == nil || m.Stores == nil || len(m.Stores.Ships) == 0 {
		return
	}
	d := r.Defenses.EffectiveR3(r.Systems.DamageMasks[0])
	m.Stores.Ships[0].Chaff, m.Stores.Ships[0].Flares = d.Chaff, d.Flares
	p.Cockpit.Defense = &cockpit.DefenseInputs{Chaff: d.Chaff, Flares: d.Flares, ChaffTimer: d.ChaffTimer, FlareTimer: d.FlareTimer, IRActive: int16(d.IRActive), RadarSignature: int16(d.RadarSignature)}
}
