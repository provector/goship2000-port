package menus

import (
	"gunship2000-assets/game/render3d"
	"gunship2000-assets/game/world"
)

const playerDecoyInstanceID = "player-shared-decoy"

// playerDecoyInstance reads the final source-owned shared state. The selector
// exists only in R3+ recorded ownership; older films keep their former visuals.
// Changing camera/viewed actor neither moves this position nor creates effects
// for NPC contact-time defense, which has no descriptor producer.
func (p *RenderingPreview) playerDecoyInstance() (render3d.Instance, bool) {
	if p.Flight == nil || p.Renderer == nil || p.Renderer.Assets == nil {
		return render3d.Instance{}, false
	}
	d := p.Flight.Defenses
	if d.R3 == nil || d.FlareTimer|d.ChaffTimer == 0 {
		return render3d.Instance{}, false
	}
	var geometry string
	switch d.R3.StatusPointer {
	case 0x3078:
		geometry = world.PlayerFlareGeometry
	case 0x30ce:
		geometry = world.PlayerChaffGeometry
	default:
		return render3d.Instance{}, false // No guess for an unknown imported selector.
	}
	if p.Renderer.Assets.Geometries[geometry] == nil {
		return render3d.Instance{}, false // Parser-only fixtures have no GS2 carrier.
	}
	return render3d.Instance{ID: playerDecoyInstanceID, GeometryID: geometry,
		Pose: render3d.Pose{H0: d.Decoy.H0, H1: d.Decoy.H1, Height: uint16(d.Decoy.Height)}}, true
}

// refreshPlayerDecoyScene is a presentation publication after the late shared
// drift. It changes no defense/counter/RNG/checkpoint state. The stable identity
// permits ordinary interpolation and the replay viewer's nonactor merge.
func (p *RenderingPreview) refreshPlayerDecoyScene() {
	instances := p.Scene.Instances[:0]
	for _, in := range p.Scene.Instances {
		if in.ID != playerDecoyInstanceID {
			instances = append(instances, in)
		}
	}
	if in, ok := p.playerDecoyInstance(); ok {
		instances = append(instances, in)
	}
	p.Scene.Instances = instances
}

// The canonical source destination/clip feedback is part of historical film
// checkpoints. This new painting consumer must not change that logical input.
// Displayed full-world/MFD/replay scenes retain the effect normally.
func withoutPlayerDecoy(instances []render3d.Instance) []render3d.Instance {
	for i, in := range instances {
		if in.ID == playerDecoyInstanceID {
			out := append([]render3d.Instance(nil), instances[:i]...)
			for _, tail := range instances[i+1:] {
				if tail.ID != playerDecoyInstanceID {
					out = append(out, tail)
				}
			}
			return out
		}
	}
	return instances
}
