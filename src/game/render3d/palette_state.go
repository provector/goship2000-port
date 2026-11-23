package render3d

import (
	"fmt"
	"gunship2000-assets/game/world"
	"image/color"
)

// PaletteState models the selected VGA bank pointer separately from the DAC
// display. Cycle operations mutate the selected bank; the Antarctic pulse
// derives temporary DAC entries from it without corrupting its base colours.
// New scenes own new copies; normal camera changes retain this state.
type PaletteState struct {
	banks        [2][768]byte
	display      [768]byte
	active       int
	lastPhase    uint64
	advanced     bool
	pulseVisible bool
}

func NewPaletteState(worldRGB6, nightRGB6 []byte) (*PaletteState, error) {
	if len(worldRGB6) != 768 || len(nightRGB6) != 768 {
		return nil, fmt.Errorf("scene palette requires two768byte RGB6 banks")
	}
	p := &PaletteState{}
	copy(p.banks[0][:], worldRGB6)
	copy(p.banks[0][39:42], p.banks[0][45:48])
	copy(p.banks[1][:], nightRGB6)
	copy(p.display[:], p.banks[0][:])
	return p, nil
}

func (r *Renderer) NewPaletteState(scene Scene, nightRGB6 []byte) (*PaletteState, error) {
	if r == nil || r.Assets == nil || len(scene.World) < 4 {
		return nil, fmt.Errorf("scene world palette unavailable")
	}
	bank := scene.PaletteBank
	if bank == "" {
		bank = "DAY"
	}
	id := scene.World[:4] + bank
	p, ok := r.Assets.Palettes[id+".PAL"]
	if !ok {
		p, ok = r.Assets.Palettes[id]
	}
	if !ok {
		return nil, fmt.Errorf("prepared palette%s unavailable", id)
	}
	return NewPaletteState(p.RGB6, nightRGB6)
}

// Toggle compiles common key0590→VGA01fe argument0. Eligibility is the source
// NIGHT0200 flag only; no settings, visibility or detection state is changed.
func (p *PaletteState) Toggle(switches uint16) bool {
	if p == nil || switches&0x200 == 0 {
		return false
	}
	p.active ^= 1
	copy(p.display[:], p.banks[p.active][:])
	return true
}

func (p *PaletteState) Night() bool { return p != nil && p.active == 1 }

// Advance is called once per logical frame after weather ownership, never by
// drawing. phase is an ownership token, not an elapsed-time or render counter.
// A repeated token cannot recursively rotate the selected bank.
func (p *PaletteState) Advance(graphics byte, phase uint64, environment *EnvironmentState) {
	if p == nil || (p.advanced && p.lastPhase == phase) {
		return
	}
	p.lastPhase, p.advanced = phase, true
	bank := p.banks[p.active][:]
	if environment != nil {
		if environment.PulseVisible && environment.Phase&1 == 0 {
			copy(p.display[:], bank)
			environment.applyPalette(p.display[:])
		} else if p.pulseVisible && !environment.PulseVisible {
			copy(p.display[:], bank)
		}
		p.pulseVisible = environment.PulseVisible
	}
	if graphics&0x10 != 0 {
		// VGA01b4: rotate six RGB triples154..159 left, update that DAC slice.
		first := [3]byte{bank[462], bank[463], bank[464]}
		copy(bank[462:477], bank[465:480])
		copy(bank[477:480], first[:])
		copy(p.display[462:480], bank[462:480])
	}
}

// RGB6 and Colours return private copies; presentation cannot mutate bank
// history. GPS's temporary world-bank installation remains a future owner.
func (p *PaletteState) RGB6() []byte {
	if p == nil {
		return nil
	}
	return append([]byte(nil), p.display[:]...)
}
func (p *PaletteState) Colours() color.Palette {
	if p == nil {
		return nil
	}
	return (world.Palette{RGB6: p.display[:]}).Colours()
}
