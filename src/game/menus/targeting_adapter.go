package menus

import (
	"encoding/binary"
	"gunship2000-assets/game/avionics"
	"gunship2000-assets/game/cockpit"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/render3d"
	"gunship2000-assets/game/world"
)

// selectedAimDefinition keeps compact0/1 source prefix records separate from raw
// weapon IDs0/1. GS1c89:04e8 clears each24-byte prefix, then writes only its name
// at+15. Unknown definitions never inherit those zero flags/range.
func (p *RenderingPreview) selectedAimDefinition() (flags, threshold uint16, valid bool) {
	if p.Initialized.Stores == nil || len(p.Initialized.Stores.Ships) == 0 {
		return
	}
	selected := p.Initialized.SelectedStation
	if selected < 0 || selected > 3 {
		return
	}
	p.Cockpit.SelectedStation = selected
	id := p.Initialized.Stores.Ships[0].Stations[selected].CompactID
	if id <= 1 {
		return 0, 0, true
	}
	w, err := p.Initialized.Stores.LookupDefinition(id)
	if err == nil {
		return binary.LittleEndian.Uint16(w.Raw[:]), binary.LittleEndian.Uint16(w.Raw[9:]), true
	}
	return
}

// projectTargetPose compiles00db:0032..016c/02bc..03d0. Shift each horizontal
// dword before subtracting; the height subtraction instead wraps as a word.
// Projection is logical320, independent of the selected physical render scale.
func projectTargetPose(target, viewer flight.Pose, tables *world.Bundle) (x, y, depth int16, valid bool) {
	relative := world.Vertex{avionics.Delta(target.H0, viewer.H0), (target.Height - viewer.Height) >> 4, avionics.Delta(target.H1, viewer.H1)}
	transformed := render3d.Transform(relative, render3d.Rotation(viewer.Angles, tables.Sine, true))
	depth = int16(transformed[2] >> 16)
	if depth <= 4 || depth >= 0x4000 {
		return
	}
	// Compiler long-division helper returns a32-bit quotient; retained AX is the
	// lowword. This is not the raster leaf's16-bit IDIV overflow contract.
	x = int16((transformed[0] >> 8) / int32(depth))
	y = -int16((transformed[1] >> 8) / int32(depth))
	return x, y, depth, true
}

func (p *RenderingPreview) publishTargeting() {
	in := &p.Cockpit
	in.TargetSymbol = 0
	in.TargetAim = nil
	flags, threshold, hasDefinition := p.selectedAimDefinition()
	if hasDefinition {
		in.TargetAim = &cockpit.TargetAimInputs{WeaponFlags: flags, TargetActive: p.Flight.Avionics.Target != 0, CenterX: int16(p.Scene.View.Center.X), CenterY: int16(p.Scene.View.Center.Y), ClipBottom: int16(p.Scene.View.Bounds.Bottom)}
	}
	row := int(p.Flight.Avionics.Target) - 1
	if row < 0 || row >= len(p.Initialized.Entities) {
		return
	}
	e := p.Initialized.Entities[row]
	target := flight.Pose{H0: e.Pose.H0, H1: e.Pose.H1, Height: int16(e.Pose.Height), Angles: e.Pose.Angles}
	body := p.Flight.State.Pose
	x, y, depth, ok := projectTargetPose(target, body, p.Renderer.Assets)
	if in.TargetAim != nil {
		in.TargetAim.RadiusAdjust = int8(uint16(depth) >> 8)
	}
	if ok {
		in.TargetX, in.TargetY = x, y
		if hasDefinition {
			contact := p.Flight.Avionics.Contacts[row]
			distance := avionics.TargetMetric(avionics.Delta(target.H0, body.H0), avionics.Delta(target.H1, body.H1)) >> 5
			in.TargetSymbol = avionics.TargetSymbol(true, x, y, distance, threshold, flags, contact.Kind, contact.AttackFlags)
		}
	}
	// Source313a is clear in the reached mast branch. Body geometry still owns
	// weapon eligibility; only the later display projection uses the actual camera
	// including its recovered final mast-height writer (not a guessed+128 pose).
	if p.ViewControls.Mode == 7 {
		c := p.ViewControls.Camera
		view := flight.Pose{H0: c.H0, H1: c.H1, Height: int16(c.Height), Angles: c.Angles}
		x, y, depth, ok = projectTargetPose(target, view, p.Renderer.Assets)
		if in.TargetAim != nil {
			in.TargetAim.RadiusAdjust = int8(uint16(depth) >> 8)
		}
		if !ok {
			in.TargetSymbol = 0
			return
		}
		in.TargetX, in.TargetY = x, y
		if hasDefinition && in.TargetSymbol == 0 {
			in.TargetSymbol = 3
		}
	}
}
