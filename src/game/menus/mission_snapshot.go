package menus

import (
	"fmt"
	"maps"
	"slices"

	"gunship2000-assets/game/cockpit"
	"gunship2000-assets/game/render3d"
	"gunship2000-assets/game/state"
	"gunship2000-assets/game/world"
)

// copyValue is for optional values whose fields have independent value
// semantics, or whose referenced data is explicitly detached by the caller.
func copyValue[T any](v *T) *T {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

type snapshotModels map[*world.MutableModel]*world.MutableModel

func (models snapshotModels) instances(rows []render3d.Instance) []render3d.Instance {
	out := slices.Clone(rows)
	for i := range out {
		if model := out[i].MutableModel; model != nil {
			// A presentation-only historical pose may retain an older model.
			if models[model] == nil {
				v := *model
				v.Vertices = slices.Clone(model.Vertices)
				v.Children = slices.Clone(model.Children)
				models[model] = &v
			}
			out[i].MutableModel = models[model]
		}
		out[i].GraphChild = copyValue(out[i].GraphChild)
	}
	return out
}

func copyCockpit(v cockpit.Inputs) cockpit.Inputs {
	v.Notifications = copyValue(v.Notifications)
	if v.Notifications != nil {
		v.Notifications.ObjectNames = slices.Clone(v.Notifications.ObjectNames)
	}
	v.Navigation = copyValue(v.Navigation)
	v.Route = copyValue(v.Route)
	v.Bars = copyValue(v.Bars)
	v.Map = copyValue(v.Map)
	if v.Map != nil {
		// The prepared world owns Grid and Atlas.
		v.Map.Markers = slices.Clone(v.Map.Markers)
		v.Map.Mission = copyValue(v.Map.Mission)
		v.Map.SupportStrike = copyValue(v.Map.SupportStrike)
	}
	v.TargetAim = copyValue(v.TargetAim)
	v.Defense = copyValue(v.Defense)
	v.ContactAvionics = copyValue(v.ContactAvionics)
	if v.ContactAvionics != nil {
		v.ContactAvionics.ThreatPoints = slices.Clone(v.ContactAvionics.ThreatPoints)
		v.ContactAvionics.ProjectilePoints = slices.Clone(v.ContactAvionics.ProjectilePoints)
	}
	return v
}

func copyFlight(r *FlightRuntime, m *state.InitializedMission, models snapshotModels) *FlightRuntime {
	f := *r
	f.State = m.PlayerFlight
	f.Combat = r.Combat.CopyForMission(m)
	f.Avionics = copyValue(r.Avionics)
	f.Avionics.Threat.Points = slices.Clone(f.Avionics.Threat.Points)
	f.filmTiming = nil
	f.terminalRGB6 = slices.Clone(r.terminalRGB6)
	f.TerminalPresentation.pixels = slices.Clone(r.TerminalPresentation.pixels)
	f.Defenses.R3 = copyValue(r.Defenses.R3)
	f.Systems.Fuel = slices.Clone(r.Systems.Fuel)
	f.Systems.Active = slices.Clone(r.Systems.Active)
	f.acceptedPlayerHits = slices.Clone(r.acceptedPlayerHits)
	for i := range f.acceptedPlayerHits {
		f.acceptedPlayerHits[i].developmentSeed = copyValue(f.acceptedPlayerHits[i].developmentSeed)
	}
	f.DamageEvents = slices.Clone(r.DamageEvents)
	for i := range f.DamageEvents {
		d := &f.DamageEvents[i]
		d.Input.developmentSeed = copyValue(d.Input.developmentSeed)
		d.Result.Audio = slices.Clone(d.Result.Audio)
		d.Second = copyValue(d.Second)
		if d.Second != nil {
			d.Second.Audio = slices.Clone(d.Second.Audio)
		}
	}
	f.CombatEvents = slices.Clone(r.CombatEvents)
	f.CopilotDefense = slices.Clone(r.CopilotDefense)
	for i := range f.CopilotDefense {
		d := &f.CopilotDefense[i].Result
		d.Deployments = slices.Clone(d.Deployments)
		d.SoundRequests = slices.Clone(d.SoundRequests)
	}
	f.TriggerTrace = slices.Clone(r.TriggerTrace)
	f.deathAngles = maps.Clone(r.deathAngles)
	f.explosionVertices = slices.Clone(r.explosionVertices)
	f.deathHistoryWorld = models.instances(r.deathHistoryWorld)
	f.sceneAudio, f.rotorCommands = nil, nil
	// Retain admitted controls, but never capture queued physical edges,
	// typematic deadlines or the host time origin belonging to another branch.
	q := &f.commandInput
	live, directions, trigger, pressed, jammers := copyValue(q.Live), q.Directions, q.Trigger, q.TriggerPressed, q.jammerHeld
	elapsed, remainder, momentary := q.elapsed, q.remainder, q.Momentary
	q.ResetTimeline()
	q.Live, q.Directions, q.Trigger, q.TriggerPressed, q.jammerHeld = live, directions, trigger, pressed, jammers
	q.elapsed, q.remainder, q.Momentary = elapsed, remainder, momentary
	return &f
}

// copyPointScene is called on the simulation owner at a completed operation.
// The copy has no recorder, career, playback capability or native renderer.
func (p *RenderingPreview) copyPointScene() (*RenderingPreview, error) {
	if p.Error != nil {
		return nil, p.Error
	}
	if p.Initialized == nil || p.Flight == nil || p.Flight.State == nil || p.Flight.State != p.Initialized.PlayerFlight || p.ResultTargets != p.Initialized.Targets || p.Flight.Combat == nil || p.Flight.Avionics == nil || p.GPS != nil || p.Flight.filmTiming != nil {
		return nil, fmt.Errorf("mission point needs a completed flight update and consistent mission owners")
	}
	n := *p
	m, copies := p.Initialized.CopyMissionState()
	models := snapshotModels(copies)
	n.Initialized, n.ResultTargets = m, m.Targets
	n.Flight = copyFlight(p.Flight, m, models)
	n.Renderer, n.sessionPoint, n.filmRecorder, n.filmPlayback, n.replayHashes = nil, nil, nil, nil, nil
	n.Visual, n.previousVisual = p.Visual.Clone(), p.previousVisual.Clone()
	n.PaletteState = copyValue(p.PaletteState)
	n.ViewControls = copyValue(p.ViewControls)
	n.Scene.Environment = p.Scene.Environment.Copy()
	n.Scene.InstanceFractions = maps.Clone(p.Scene.InstanceFractions)
	n.Scene.GroundOwners, n.Scene.SkyMask = nil, nil
	n.Scene.ExplosionVertices = slices.Clone(p.Scene.ExplosionVertices)
	if c := copyValue(p.Scene.MaterialCarrier); c != nil {
		c.CS, c.DS, c.Readable = slices.Clone(c.CS), slices.Clone(c.DS), slices.Clone(c.Readable)
		n.Scene.MaterialCarrier = c
	}
	n.instances, n.Scene.Instances = models.instances(p.instances), models.instances(p.Scene.Instances)
	n.Cockpit = copyCockpit(p.Cockpit)
	for _, v := range []*presentationSnapshot{&n.presentation.previous, &n.presentation.current} {
		v.Instances = models.instances(v.Instances)
		v.Cockpit = copyCockpit(v.Cockpit)
	}
	n.Palette = slices.Clone(p.Palette)
	n.avionicsReference = slices.Clone(p.avionicsReference)
	n.worldFrame = slices.Clone(p.worldFrame)
	n.worldSkyMask = slices.Clone(p.worldSkyMask)
	n.presentation.referenceBase = slices.Clone(p.presentation.referenceBase)
	n.Frame, n.targetFrame, n.skyMask, n.targetBrownMask = nil, nil, nil, nil
	n.overlayScratch = cockpit.OverlayScratch{}
	n.SkippedRows = slices.Clone(p.SkippedRows)
	n.Stats.Status = slices.Clone(p.Stats.Status)
	n.parkedDisplay.offsets = maps.Clone(p.parkedDisplay.offsets)
	n.Dirty = true
	return &n, nil
}
