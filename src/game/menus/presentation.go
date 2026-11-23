package menus

import (
	"gunship2000-assets/game/cockpit"
	"gunship2000-assets/game/render3d"
	"math"
	"strings"
)

// Presentation snapshots never replace InitializedMission or the flight state.
// Smooth mode displays previous→current, adding one logical tick of latency.
type presentationSnapshot struct {
	Camera    render3d.Camera
	Instances []render3d.Instance
	Cockpit   cockpit.Inputs
}
type scenePresentation struct {
	previous, current presentationSnapshot
	ready             bool
	mode              string
	alpha             float64
	referenceBase     []byte // destination before the latest reference frame
	referenceDirty    bool
}

// Contact membership is not a camera discontinuity: the source strict contact
// test can alternate while an aircraft stays at the same support height. Reset
// on every such transition collapsed mast/ownship view interpolation to logical
// updates. Smooth presentation instead interpolates the actual reached camera
// endpoints, including ordinary landing corrections. Real world-boundary jumps
// still snap player-owned cameras; a separately tracked wingman's history does
// not depend on the player's boundary. Projectile returns always change owner.
// This is display history only, never a flight/contact or recorded-state rule.
func (p *RenderingPreview) presentationDiscontinuity(projectileReturn, playerContact, playerBoundary bool) bool {
	if projectileReturn {
		return true
	}
	if !playerBoundary {
		return false
	}
	if p.ViewControls != nil && p.ViewControls.Mode == 4 && p.ViewControls.TrackedID != "" && p.Initialized != nil && p.ViewControls.TrackedID != p.Initialized.PlayerID {
		return false
	}
	return true
}

func (p *RenderingPreview) snapshotPresentation() presentationSnapshot {
	s := presentationSnapshot{Camera: p.Scene.Camera, Instances: append([]render3d.Instance(nil), p.Scene.Instances...), Cockpit: p.Cockpit}
	if s.Cockpit.Map != nil {
		m := *s.Cockpit.Map
		s.Cockpit.Map = &m
	}
	if s.Cockpit.Bars != nil {
		b := *s.Cockpit.Bars
		s.Cockpit.Bars = &b
	}
	return s
}
func (p *RenderingPreview) resetPresentation() {
	if p.ViewControls != nil && !p.Inspection {
		p.filterSceneInstances()
	}
	s := p.snapshotPresentation()
	p.presentation.previous, p.presentation.current = s, s
	p.presentation.ready = true
	p.presentation.alpha = 1
	p.previousVisual = p.Visual.Clone()
	p.presentation.referenceDirty = true
	p.Dirty = true
}
func (p *RenderingPreview) beginPresentationStep() {
	if !p.presentation.ready {
		p.resetPresentation()
	}
	p.presentation.previous = p.presentation.current
}
func (p *RenderingPreview) endPresentationStep() {
	p.presentation.current = p.snapshotPresentation()
	p.presentation.referenceDirty = true
	p.Dirty = true
}
func lerpWord(a, b int16, t float64) int16 {
	return int16(math.Round(float64(a) + (float64(b)-float64(a))*t))
}
func lerpAngle(a, b uint16, t float64) uint16 {
	return a + uint16(int16(math.Round(float64(int16(b-a))*t)))
}
func lerpPosition(a, b uint32, t float64) uint32 {
	return uint32(math.Round(float64(a) + (float64(b)-float64(a))*t))
}
func lerpPose(a, b render3d.Pose, t float64) render3d.Pose {
	return render3d.Pose{H0: lerpPosition(a.H0, b.H0, t), H1: lerpPosition(a.H1, b.H1, t), Height: uint16(lerpWord(int16(a.Height), int16(b.Height), t))}
}
func lerpAngles(a, b [3]uint16, t float64) (o [3]uint16) {
	for i := range o {
		o[i] = lerpAngle(a[i], b[i], t)
	}
	return
}
func poseFraction(a, b, rounded render3d.Pose, aa, ba, angles [3]uint16, t float64) (q render3d.PoseFraction) {
	remainder := func(start, end, value float64) float64 { return start - value + (end-start)*t }
	q.Position = [3]float64{remainder(float64(a.H0), float64(b.H0), float64(rounded.H0)), remainder(float64(int16(a.Height)), float64(int16(b.Height)), float64(int16(rounded.Height))), remainder(float64(a.H1), float64(b.H1), float64(rounded.H1))}
	for i := range aa {
		q.Angles[i] = float64(int16(ba[i]-aa[i]))*t - float64(int16(angles[i]-aa[i]))
	}
	return
}

func retainInstanceFraction(scene *render3d.Scene, prev, cur, in render3d.Instance, t float64) {
	q := poseFraction(prev.Pose, cur.Pose, in.Pose, prev.Angles, cur.Angles, in.Angles, t)
	if q != (render3d.PoseFraction{}) {
		if scene.InstanceFractions == nil {
			scene.InstanceFractions = make(map[string]render3d.PoseFraction)
		}
		scene.InstanceFractions[in.ID] = q
	}
}

func (p *RenderingPreview) presentationInputs() (render3d.Scene, cockpit.Inputs) {
	scene, in := p.Scene, p.Cockpit
	h := &p.presentation
	if h.mode != "smooth" || p.Inspection || !h.ready {
		return scene, in
	}
	a, b := h.previous, h.current
	t := math.Max(0, math.Min(1, h.alpha))
	scene.Camera = render3d.Camera{Pose: lerpPose(a.Camera.Pose, b.Camera.Pose, t), Angles: lerpAngles(a.Camera.Angles, b.Camera.Angles, t)}
	if scene.FloatingGeometry {
		scene.CameraFraction = poseFraction(a.Camera.Pose, b.Camera.Pose, scene.Camera.Pose, a.Camera.Angles, b.Camera.Angles, scene.Camera.Angles, t)
	}
	scene.Instances = append([]render3d.Instance(nil), b.Instances...)
	for i := range scene.Instances {
		if i < len(a.Instances) && a.Instances[i].ID == b.Instances[i].ID {
			scene.Instances[i].Pose = lerpPose(a.Instances[i].Pose, b.Instances[i].Pose, t)
			scene.Instances[i].Angles = lerpAngles(a.Instances[i].Angles, b.Instances[i].Angles, t)
			if scene.FloatingGeometry {
				retainInstanceFraction(&scene, a.Instances[i], b.Instances[i], scene.Instances[i], t)
			}
		}
	}
	// Only continuous display values interpolate. Stores, warnings, modes, gear,
	// palette phase and damage always use the current authoritative snapshot.
	in.Height = uint16(lerpWord(int16(a.Cockpit.Height), int16(b.Cockpit.Height), t))
	in.Heading = lerpAngle(a.Cockpit.Heading, b.Cockpit.Heading, t)
	in.Pitch = lerpAngle(a.Cockpit.Pitch, b.Cockpit.Pitch, t)
	in.Bank = lerpAngle(a.Cockpit.Bank, b.Cockpit.Bank, t)
	in.Vertical = lerpWord(a.Cockpit.Vertical, b.Cockpit.Vertical, t)
	in.Lateral = lerpWord(a.Cockpit.Lateral, b.Cockpit.Lateral, t)
	in.HUDScale = lerpWord(a.Cockpit.HUDScale, b.Cockpit.HUDScale, t)
	in.AttitudeMatrix = [3]int16{render3d.Sine(in.Pitch, p.Renderer.Assets.Sine), render3d.Sine(in.Bank+0x4000, p.Renderer.Assets.Sine), render3d.Sine(in.Bank, p.Renderer.Assets.Sine)}
	if in.Map != nil && a.Cockpit.Map != nil && b.Cockpit.Map != nil {
		m := *in.Map
		m.H0 = lerpPosition(a.Cockpit.Map.H0, b.Cockpit.Map.H0, t)
		m.H1 = lerpPosition(a.Cockpit.Map.H1, b.Cockpit.Map.H1, t)
		m.Heading = in.Heading
		in.Map = &m
	}
	return scene, in
}

// Reference destination advances only on a logical state change. Extra display
// draws start from this same pre-frame destination, never their prior output.
func (p *RenderingPreview) commitReferenceFrame(a *cockpit.Assets) error {
	if !p.presentation.referenceDirty && p.presentation.ready {
		return nil
	}
	bottom, err := cockpit.Bottom(a, p.Cockpit.AircraftID, p.Cockpit.Arrangement)
	if err != nil {
		return err
	}
	if p.ViewControls != nil && !p.Inspection {
		p.filterSceneInstances()
		if !p.ViewControls.UseArrangementViewport {
			bottom = 199
		}
	} else if p.Cockpit.CameraMode != 0 {
		bottom = 199
	}
	if bottom < p.Scene.View.Bounds.Bottom && len(p.worldFrame) == p.Scene.TargetPixels() {
		clear(p.worldFrame[p.Scene.Layout().Y[bottom]*p.Scene.Layout().Width:])
	}
	p.Scene.View.Bounds.Bottom = bottom
	size := p.Scene.TargetPixels()
	if len(p.worldFrame) != size {
		p.worldFrame = make([]byte, size)
	}
	if len(p.Frame) != size {
		p.Frame = make([]byte, size)
	}
	if len(p.presentation.referenceBase) != size {
		p.presentation.referenceBase = make([]byte, size)
	}
	copy(p.presentation.referenceBase, p.worldFrame)
	scene := p.Scene
	if strings.HasPrefix(scene.World, "EURO") {
		if len(p.worldSkyMask) != size {
			p.worldSkyMask = make([]byte, size)
		}
		// Smooth drawing immediately renders the displayed scene with its own
		// exact coverage plane. This hidden indexed feedback frame still needs
		// its pixels, but not a second material/sky attribution traversal.
		// Stepped display consumes worldFrame directly and retains its plane.
		if p.presentation.mode != "smooth" || p.Inspection {
			scene.SkyMask = p.worldSkyMask
		} else {
			scene.SkyMask = nil
		}
	} else {
		p.worldSkyMask = nil
	}
	p.Stats, err = p.Renderer.Draw(scene, &p.Visual, p.worldFrame)
	p.presentation.referenceDirty = false
	return err
}
