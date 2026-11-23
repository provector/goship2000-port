package menus

import (
	"fmt"
	"gunship2000-assets/game/render3d"
	"gunship2000-assets/game/world"
)

func (p *RenderingPreview) cameraInputs() render3d.CameraInputs {
	m := p.Initialized
	anchor := func(row int) render3d.CameraAnchor {
		e := m.Entities[row]
		extent := int16(0)
		if e.ModelID >= 0 && e.ModelID < len(p.Renderer.Assets.Models) {
			if g := p.Renderer.Assets.Geometries[p.Renderer.Assets.Models[e.ModelID]]; g != nil {
				extent = g.Extent
			}
		}
		return render3d.CameraAnchor{ID: e.ID, Camera: render3d.Camera{Pose: render3d.Pose{H0: e.Pose.H0, H1: e.Pose.H1, Height: e.Pose.Height}, Angles: e.Pose.Angles}, Extent: extent}
	}
	in := render3d.CameraInputs{Player: anchor(m.PlayerRow), AircraftSelector: uint8(m.Entities[m.PlayerRow].SerializedAircraftSelector), SourceFloorRules: p.cameraContactMechanics,
		GroundHeight: func(pose render3d.Pose) (int16, error) {
			// Source camera floor callers16bb/17b5/1942/1c62/1dfd/1f65
			// use314c. Historical interpretations retain their old31e3 owner.
			if p.cameraContactMechanics {
				return p.Renderer.Assets.ContactHeight(m.World, world.QueryPose{H0: pose.H0, H1: pose.H1, Height: int16(pose.Height)}, m.ContactObjects())
			}
			return p.Renderer.Assets.TerrainHeight(m.World, world.QueryPose{H0: pose.H0, H1: pose.H1, Height: int16(pose.Height)})
		}}
	if len(m.Rows) > 1 {
		in.FlightEnabled = m.Rows[1].Raw[10]&0x20 != 0
	}
	for _, row := range m.FlightRows {
		if row != m.PlayerRow && m.Entities[row].Participation != 0 {
			in.Flight = append(in.Flight, anchor(row))
		}
	}
	for row, entity := range m.Entities {
		in.Candidates = append(in.Candidates, render3d.CameraCandidate{CameraAnchor: anchor(row), SourceIndex: entity.RuntimeRow, Type: entity.Kind, Participation: entity.Participation})
	}
	// Acquired contact owner supplies a target; mission objectives are never
	// silently promoted to locks. Projectile history remains deferred.
	in.Target = p.targetAnchor()
	if r := p.Flight; r != nil && r.Combat != nil {
		b := r.Combat
		q := b.Slots[b.NextSlot]
		anchor := func(slot int) render3d.CameraAnchor {
			v := b.Slots[slot]
			return render3d.CameraAnchor{ID: fmt.Sprintf("projectile-%d-%d", slot, r.projectileGeneration[slot]), Camera: render3d.Camera{Pose: render3d.Pose{H0: v.Pose.H0, H1: v.Pose.H1, Height: v.Pose.Height}, Angles: v.Pose.Angles}}
		}
		v := &render3d.ProjectileCameraInputs{Eligible: q.Lifetime != 0 && q.Flags&3 != 3, LastActive: b.Slots[b.LastSlot].Lifetime != 0, Current: anchor(int(b.NextSlot)), Lifetime: q.Lifetime, Packed: q.Packed}
		flags, _, known := p.selectedAimDefinition()
		previous := int((b.NextSlot - 1) & 15)
		if known && flags&0x1000 != 0 && b.Slots[previous].Lifetime != 0 {
			a := anchor(previous)
			v.Previous = &a
		}
		in.ProjectileView = v
	}
	return in
}

func (p *RenderingPreview) publishView() {
	if p.ViewControls == nil || p.Inspection {
		return
	}
	p.Scene.Camera = p.ViewControls.Camera
	p.Cockpit.CameraMode = int(p.ViewControls.Mode)
	p.Cockpit.Arrangement = int(p.ViewControls.Arrangement)
	p.Dirty = true
}

func (p *RenderingPreview) filterSceneInstances() {
	p.Scene.Instances = p.Scene.Instances[:0]
	for _, in := range p.instances {
		hidden := false
		for _, e := range p.Initialized.Entities {
			if e.ID == in.ID && e.ID != p.Initialized.PlayerID && (e.Kind == 0xff || e.Kind&0x40 != 0 || e.Participation == 0) {
				hidden = true
				break
			}
		}
		// Raw10bb:8899 (canonicalfcf9) skips every ordinary P0, not only
		// wrecks, and FF/40-bit kinds. Ownship has its separate camera owner.
		// Retain metadata: a later source P1/P4 can register the same header.
		if hidden {
			continue
		}
		// Modes0/1/2 skip the source common ownship registration. Mast7 does
		// register it; the camera, not entity deletion, controls this distinction.
		mode := p.ViewControls.Mode
		if in.ID == p.Initialized.PlayerID && (mode < 3 || mode == 8) {
			continue
		}
		p.Scene.Instances = append(p.Scene.Instances, in)
		if p.Flight != nil && p.Flight.Combat != nil && p.Renderer.Assets.Combat != nil {
			for _, e := range p.Initialized.Entities {
				if e.ID != in.ID || (e.Participation != 2 && e.Participation != 3) {
					continue
				}
				if e.Participation == 3 {
					p.Scene.Instances = append(p.Scene.Instances, render3d.Instance{ID: e.ID + "-impact", Pose: in.Pose, Impact: true})
				}
				p.Scene.Instances = append(p.Scene.Instances, render3d.Instance{ID: e.ID + "-explosion", Pose: in.Pose, GeometryID: p.Renderer.Assets.Combat.Explosion, Explosion: true})
				break
			}
		}
	}
	p.Scene.Instances = append(p.Scene.Instances, p.combatInstances()...)
	if p.p4Mechanics && p.Flight != nil {
		p.Scene.Instances = append(p.Scene.Instances, p.Flight.deathHistoryWorld...)
	}
	if r := p.Flight; r != nil && r.TerminalPresentation.Active && r.TerminalPresentation.Exterior && !r.TerminalPresentation.Completed {
		pose := r.State.Pose
		p.Scene.Instances = append(p.Scene.Instances, render3d.Instance{ID: "terminal-header0048", Impact: true,
			Pose: render3d.Pose{H0: pose.H0, H1: pose.H1, Height: uint16(pose.Height)}})
	}
	p.refreshPlayerDecoyScene()
}

func (c *FrontEnd) handleSceneView(a Actions) error {
	p := c.Rendering
	if p == nil || p.ViewControls == nil {
		return fmt.Errorf("initialized camera state missing")
	}
	p.mastHeld = a.SceneDirections
	if p.Inspection {
		return nil
	}
	if a.CycleArrangement {
		p.ViewControls.CycleArrangement()
		p.publishView()
		p.resetPresentation()
	}
	if a.ViewKey != 0 {
		previousMode := p.ViewControls.Mode
		changed, err := p.ViewControls.Select(a.ViewKey, a.ViewShift, p.cameraInputs(), p.Renderer.Assets)
		if err != nil {
			c.reportDiagnostic("view", err.Error(), err)
			return err
		}
		if changed {
			if p.r1Mechanics && a.ViewKey == 5 && a.ViewShift {
				for _, e := range p.Initialized.Entities {
					if e.ID == p.ViewControls.TrackedID && e.FlightSlot > 0 && e.FlightSlot < 5 {
						p.Flight.OrderRecipient = e.FlightSlot
						break
					}
				}
			}
			if previousMode != 6 && p.ViewControls.Mode == 6 && a.ViewKey == 9 {
				p.Flight.sceneAudio = append(p.Flight.sceneAudio, sceneAudioCommand{event: 0x17})
			}
			if a.ViewKey == 1 && !a.ViewShift {
				// Only forward-view08b6 falls through08fb/080a to0bh(AAF SAR1).
				// The other view handlers do not reissue or reset the rotor stream.
				p.requestCollectiveRotor()
			}
			p.publishView()
			p.resetPresentation()
		}
	}
	if a.ViewCycle != 0 {
		changed, err := p.ViewControls.CycleTracked(a.ViewCycle, p.cameraInputs(), p.Renderer.Assets)
		if err != nil {
			c.reportDiagnostic("view", err.Error(), err)
			return err
		}
		if changed {
			p.publishView()
			p.resetPresentation()
		}
	}
	return nil
}
