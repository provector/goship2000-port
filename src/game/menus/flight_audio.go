package menus

import "gunship2000-assets/game/flight"

type sceneAudioCommand struct {
	event, argument uint16
	firingAircraft  string // Remote friendly aircraft weapon constructors use a camera-owned voice.
}

func (p *RenderingPreview) firingAircraft(slot int, event uint16) string {
	switch event {
	case 7, 8, 9, 0x18: // cannon/rocket release and guided ignition/release
	default:
		return ""
	}
	if slot < 0 || slot >= len(p.Flight.Combat.Slots) {
		return ""
	}
	shooter := int(p.Flight.Combat.Slots[slot].Shooter)
	if shooter < len(p.Initialized.Entities) && shooter != p.Initialized.PlayerRow && p.Initialized.Entities[shooter].Kind == 0x11 {
		return p.Initialized.Entities[shooter].ID
	}
	for _, row := range p.Initialized.FlightRows {
		if row == shooter && row != p.Initialized.PlayerRow {
			return p.Initialized.Entities[row].ID
		}
	}
	return ""
}
func (p *RenderingPreview) followedAircraft() string {
	if p == nil || p.Inspection || p.GPS != nil || p.ViewControls == nil {
		return ""
	}
	switch p.ViewControls.Mode {
	case 4, 10: // external tracked aircraft and reverse tactical (target-anchored)
		return p.ViewControls.TrackedID
	}
	return ""
}

// defenseAudio restores the post-success source0FD0/10DC callers. The manual
// leaf has no timer/cooldown test: another accepted release replaces the same
// source row. Host effects gain and view are separate from the source DE gate.
// Historical launch interpretations keep their prior absent callers/metadata.
func (p *RenderingPreview) defenseAudio(action flight.DefenseAction, accepted bool) {
	if p == nil || p.Flight == nil || !(p.p7Mechanics || p.practiceFeedback) || !accepted {
		return
	}
	switch action {
	case flight.LaunchChaff:
		p.Flight.sceneAudio = append(p.Flight.sceneAudio, sceneAudioCommand{event: 0x0a})
	case flight.LaunchFlare:
		p.Flight.sceneAudio = append(p.Flight.sceneAudio, sceneAudioCommand{event: 0x2a})
	}
}

// Rotor requests preserve original ASOUND.GS2 command0bh arguments. They are
// emitted by source semantic callers, never by drawing or blanket collective
// sampling: autopilot/hover may change AAF without requesting a new sound.
func (p *RenderingPreview) requestRotor(argument uint16) {
	if p == nil || p.Flight == nil {
		return
	}
	p.Flight.rotorCommands = append(p.Flight.rotorCommands, argument)
	p.Flight.sceneAudio = append(p.Flight.sceneAudio, sceneAudioCommand{event: 0x0b, argument: argument})
}

// takeSceneAudio preserves source call order when several logical quanta are
// accepted by one host update. Fixed effects share the retained rotor driver.
func (p *RenderingPreview) takeSceneAudio() []sceneAudioCommand {
	if p == nil || p.Flight == nil {
		return nil
	}
	r := p.Flight
	out := r.sceneAudio
	r.sceneAudio, r.rotorCommands = nil, nil
	return out
}

func (p *RenderingPreview) requestCollectiveRotor() {
	if p == nil || p.Flight == nil || p.Flight.State == nil {
		return
	}
	p.requestRotor(p.collectiveRotorPitch())
}

func (p *RenderingPreview) collectiveRotorPitch() uint16 {
	r := p.Flight
	if r.State.Rotor == 0 || len(r.Systems.Fuel) == 0 || r.Systems.Fuel[0] <= 0 {
		return 0
	}
	return runningRotorPitch(r.State.Collective)
}

// Zero is the mixer shutdown command. A running rotor at collective zero or one
// uses the same idle voice as engine start.
func runningRotorPitch(collective int16) uint16 {
	return uint16(max(1, collective>>1))
}

// TakeRotorCommands hands ordered logical events to the independent audio
// sample-clock owner. Host effects mute does not alter these source events.
func (p *RenderingPreview) TakeRotorCommands() []uint16 {
	if p == nil || p.Flight == nil {
		return nil
	}
	commands := p.Flight.rotorCommands
	p.Flight.rotorCommands = nil
	kept := p.Flight.sceneAudio[:0]
	for _, q := range p.Flight.sceneAudio {
		if q.event != 0x0b {
			kept = append(kept, q)
		}
	}
	p.Flight.sceneAudio = kept
	return commands
}

func (p *RenderingPreview) collectiveRotorCommand(key int) {
	r := p.Flight
	if key == 0 || key == 1 {
		// Selected source profile DE20 is enabled. Native mute is a separate
		// output preference, not a mutation of the emulated semantic gate.
		if len(r.Systems.Fuel) > 0 && r.Systems.Fuel[0] > 0 {
			p.requestCollectiveRotor()
		}
	} else if key == 2 {
		if r.State.Rotor == 0 {
			p.requestRotor(0)
		} else if len(r.Systems.Fuel) > 0 && r.Systems.Fuel[0] > 0 {
			// Restart the retained driver at idle even when the axis or height
			// controller supplies collective without a keyboard sound command.
			p.requestRotor(1)
		}
	}
}

func (p *RenderingPreview) fuelAudio(burn flight.FuelBurn) {
	r := p.Flight
	if !burn.PlayerActive {
		return
	}
	if burn.PreviousFuel > 0 {
		if r.SpeechEnabled {
			switch burn.PreviousFuel {
			case 24:
				r.sceneAudio = append(r.sceneAudio, sceneAudioCommand{event: 0x33})
			case 10:
				r.sceneAudio = append(r.sceneAudio, sceneAudioCommand{event: 0x3c})
			}
		}
		return
	}
	if r.SpeechEnabled && r.State.RadarAltitude > 2 {
		r.sceneAudio = append(r.sceneAudio, sceneAudioCommand{event: 0x3d})
		r.SpeechEnabled = false // Source03fc XORs shared DE80 only on this path.
	}
	// The selected source profile admits DE20. Host effects mute is output
	// gain, not this semantic gate. RotorAE1, terminal state and result flags
	// are not changed by the ordinary fuel-empty caller.
	p.requestRotor(0)
}
