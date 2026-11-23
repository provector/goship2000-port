package menus

import (
	"encoding/binary"
	"fmt"
	"gunship2000-assets/game/avionics"
	"gunship2000-assets/game/cockpit"
	"gunship2000-assets/game/diagnostics"
	"gunship2000-assets/game/render3d"
	"gunship2000-assets/game/world"
	"log/slog"
	"math"
	"strings"
)

func (p *RenderingPreview) initializeAvionics() {
	if p.Flight != nil {
		p.Flight.Avionics = avionics.New()
	}
}
func (p *RenderingPreview) contactEntities() (out [avionics.EntityCapacity]avionics.Entity) {
	if p == nil || p.Initialized == nil {
		return
	}
	for _, e := range p.Initialized.Entities {
		row := e.RuntimeRow
		if row < 0 || row >= len(out) {
			continue
		}
		out[row] = avionics.Entity{Present: true, Row: row, Kind: e.Kind, Participation: e.Participation, Pose: avionics.Pose{H0: e.Pose.H0, H1: e.Pose.H1, Height: int16(e.Pose.Height), Angles: e.Pose.Angles}, Object: e.Object.Raw, Control: e.ControlRaw, FlightSlot: e.FlightSlot}
	}
	return
}
func (p *RenderingPreview) advanceAvionics(random func() uint16, a *cockpit.Assets) error {
	if p.Flight == nil || p.Flight.Avionics == nil {
		return nil
	}
	r := p.Flight
	s := r.State
	body := avionics.Pose{H0: s.Pose.H0, H1: s.Pose.H1, Height: s.Pose.Height, Angles: s.Pose.Angles}
	head := body
	head.Height += 128
	if p.ViewControls.Mode == 7 {
		head.Angles = p.ViewControls.Camera.Angles
	}
	in := avionics.Input{Entities: p.contactEntities(), Player: body, Head: head, Mast: p.ViewControls.Mode == 7, Speed: s.IndicatedSpeed, Switches: p.Initialized.Switches, PlayerRow: p.Initialized.PlayerRow, DamageMask: r.Systems.DamageMasks[0], World: p.Initialized.World, Assets: p.Renderer.Assets, Random: random}
	in.ModernTail = p.p4Mechanics
	in.ProgressAwareTargeting = p.r8Mechanics
	in.DisableAutonomy = p.r1Mechanics && !p.wingmenAutonomy
	if p.p4Mechanics {
		in.SetControlTarget = func(actor int, target int8) error {
			if actor < 1 || actor > 4 || actor >= len(p.Initialized.Entities) || p.Initialized.Entities[actor].FlightSlot != actor {
				return fmt.Errorf("contact actor%d lacks actual CONTROL owner", actor)
			}
			p.Initialized.Entities[actor].ControlRaw[3] = byte(target)
			return nil
		}
		in.ContactOrder = func(actor, target int) error { return r.Combat.ContactOrder(actor, target, r.Notifications.Rows[0][0]) }
	}
	if err := r.Avionics.Advance(in); err != nil {
		return err
	}
	p.syncFollowerNavigation()
	p.publishAvionics()
	return nil
}

// advanceAvionicsDisplays runs after source camera publication, not while
// drawing a smooth frame. Source pixel/clip feedback always uses320x200/source.
func (p *RenderingPreview) advanceAvionicsDisplays(a *cockpit.Assets) error {
	if p.Flight == nil || p.Flight.Avionics == nil {
		return nil
	}
	r := p.Flight
	// Camera publication precedes the source HUD phase; mast display reprojection
	// must consume this frame's final camera, never the previous filter output.
	p.publishAvionics()
	// Source section2:2272 runs in the reached HUD phase for cameras0/7,
	// before reticle damage masking. The retained spoken row is not target zoom
	// history and must not advance on interpolated draws or clear on brief loss.
	if target := r.Avionics.Target; target > 0 && int(target) <= len(p.Initialized.Entities) {
		command, issued := r.AcquisitionSpeech.Advance(avionics.AcquisitionSpeechInput{
			Reached: p.Cockpit.HUD && (p.ViewControls.Mode == 0 || p.ViewControls.Mode == 7),
			Target:  target, Symbol: int16(p.Cockpit.TargetSymbol),
			Kind:    p.Initialized.Entities[int(target)-1].Kind,
			TargetX: p.Cockpit.TargetX, SpeechEnabled: r.SpeechEnabled,
		})
		if issued {
			r.sceneAudio = append(r.sceneAudio, sceneAudioCommand{event: command})
		}
	}
	s := r.State
	body := avionics.Pose{H0: s.Pose.H0, H1: s.Pose.H1, Height: s.Pose.Height, Angles: s.Pose.Angles}
	// Source display mutations are invoked only when the corresponding view/MFD
	// is reached. Extra smooth draws do not advance warning/zoom/contact state.
	if p.Cockpit.HUD && p.ViewControls.Mode == 0 && p.ViewControls.Arrangement != 2 && r.SecondaryMFD == 5 && r.Systems.DamageMasks[1]&0x40 == 0 {
		logicalFrame := append([]byte(nil), p.avionicsReference...)
		withoutContacts := p.Cockpit
		withoutContacts.ContactAvionics = nil
		if err := cockpit.Compose(logicalFrame, a, withoutContacts); err != nil {
			return err
		}
		if err := a.DrawThreatBackground(logicalFrame, p.Cockpit.AircraftID, r.Avionics.Threat.Zoom); err != nil {
			return err
		}
		// Only the established direct player shooter/target domains are admitted.
		// Unknown NPC control-row aliases are not inferred from scene allegiance.
		projectiles := avionics.ProjectileThreatInputs{Player: byte(p.Initialized.PlayerRow)}
		if r.Combat != nil {
			for i, slot := range r.Combat.Slots {
				projectiles.Slots[i] = avionics.ProjectileThreatInput{Flags: slot.Flags, Life: slot.Lifetime, Shooter: slot.Shooter, Target: slot.Target, Parameter: slot.Parameter, H0: slot.Pose.H0, H1: slot.Pose.H1}
			}
		}
		previousWarnings := r.Avionics.Threat.WarningFlags
		r.Avionics.AdvanceThreatFrame(body, p.Renderer.Assets, p.Cockpit.BlinkPhase, projectiles, avionics.ThreatFeedback{
			Contact: func(q avionics.ThreatPlot) bool {
				return a.DrawThreatContactPoint(logicalFrame, p.Cockpit.AircraftID, p.Cockpit.BlinkPhase, cockpit.ContactPoint{X: q.X, Y: q.Y, Colour: q.Colour, Attack: q.Attack})
			},
			Projectile: func(q avionics.ProjectileThreatPlot) bool {
				return a.DrawThreatProjectilePoint(logicalFrame, p.Cockpit.AircraftID, cockpit.ProjectilePoint{X: q.X, Y: q.Y, Cross: q.Cross})
			},
		})
		if p.threatFeedbackMechanics {
			flags := r.Avionics.Threat.WarningFlags
			// Source3CC5 tests semantic DE40, not speech DE80 or host mute.
			// Repeated reached HUD visits at one DA retain their own requests.
			if threatWarningTone(flags, p.Cockpit.BlinkPhase, 0x40) {
				r.sceneAudio = append(r.sceneAudio, sceneAudioCommand{event: 6})
			}
			if flags != previousWarnings && diagnostics.Enabled(slog.LevelDebug) {
				diagnostics.Debug("threat", "Warning display state changed", "step", r.Steps, "phase", p.Cockpit.BlinkPhase, "previous", previousWarnings, "flags", flags, "source", "1975:3C2B..3CDD")
			}
		}
	}
	if r.Avionics.Target > 0 && r.Systems.DamageMasks[0]&0x20 == 0 && ((p.ViewControls.Mode == 0 && p.ViewControls.Arrangement != 2 && r.PrimaryMFD == 5) || p.ViewControls.Mode == 7) {
		row := int(r.Avionics.Target) - 1
		e := p.contactEntities()[row]
		extent := p.entityExtent(row)
		old := r.Avionics.TargetCache
		// The cache is shared by target panels, not source recorder-memory aliases.
		if old.Target != r.Avionics.Target {
			distance := avionics.Metric(avionics.Delta(e.Pose.H0, body.H0), avionics.Delta(e.Pose.H1, body.H1))
			old.Target = r.Avionics.Target
			if distance > 0x800 {
				old.Zoom = 0x800
			} else {
				old.Zoom = distance << 2
			}
			r.Avionics.TargetCache = old
		}
		r.Avionics.TargetPanelZoom = old.Zoom
		clipFlags := byte(0)
		if p.Scene.GraphicsOptions&8 == 0 {
			if err := p.sourceTargetClip(a, &clipFlags); err != nil {
				return err
			}
		}
		r.Avionics.AdvanceTargetZoom(extent, p.Scene.GraphicsOptions&8 != 0, clipFlags)
	}
	p.publishAvionics()
	return nil
}
func (p *RenderingPreview) entityExtent(row int) int16 {
	if row < 0 || row >= len(p.Initialized.Entities) {
		return 0
	}
	e := p.Initialized.Entities[row]
	if e.ModelID >= 0 && e.ModelID < len(p.Renderer.Assets.Models) {
		if g := p.Renderer.Assets.Geometries[p.Renderer.Assets.Models[e.ModelID]]; g != nil {
			return g.Extent
		}
	}
	return 0
}
func (p *RenderingPreview) publishAvionics() {
	if p.Flight == nil || p.Flight.Avionics == nil {
		return
	}
	r := p.Flight
	a := r.Avionics
	// The source target-to-map fallback is a reached primary-panel action,
	// including a lock lost in this scan; exterior draws cannot change it.
	if a.Target == 0 && r.PrimaryMFD == 5 && p.Cockpit.HUD && p.Cockpit.CameraMode == 0 && p.Cockpit.Arrangement != 2 {
		r.PrimaryMFD = 0
		p.Cockpit.PrimaryMFD = 0
		p.Cockpit.PrimaryMap = true
	}
	if a.MFDTargetRequested {
		r.PrimaryMFD, r.SecondaryMFD = 5, 5
		a.MFDTargetRequested = false
		p.Cockpit.PrimaryMFD, p.Cockpit.SecondaryMFD = 5, 5
		p.Cockpit.PrimaryMap, p.Cockpit.SecondaryMap = false, false
	}
	p.Cockpit.MastEmpty = a.Target == 0
	p.publishTargeting()
	v := &cockpit.ContactAvionicsInputs{TargetActive: a.Target > 0, ThreatZoom: a.Threat.UsedZoom}
	for _, q := range a.Threat.Points {
		v.ThreatPoints = append(v.ThreatPoints, cockpit.ContactPoint{X: q.X, Y: q.Y, Colour: q.Colour, Attack: q.Attack})
	}
	for _, q := range a.Threat.Projectiles.Plots[:a.Threat.Projectiles.Count] {
		v.ProjectilePoints = append(v.ProjectilePoints, cockpit.ProjectilePoint{X: q.X, Y: q.Y, Cross: q.Cross})
	}
	if a.Target > 0 && int(a.Target) <= len(p.Initialized.Entities) {
		e := p.Initialized.Entities[int(a.Target)-1]
		s := r.State
		dx, dz := avionics.Delta(e.Pose.H0, s.Pose.H0), avionics.Delta(e.Pose.H1, s.Pose.H1)
		v.TargetBearing = -(world.SourceAngle(dz, dx, p.Renderer.Assets) - 0x4000)
	}
	if p.threatFeedbackMechanics && p.Cockpit.HUD && p.ViewControls.Mode == 0 && p.ViewControls.Arrangement != 2 && r.SecondaryMFD == 5 && r.Systems.DamageMasks[1]&0x40 == 0 {
		v.WarningFlags = a.Threat.WarningFlags
	}
	p.Cockpit.ContactAvionics = v
	if p.Cockpit.Map != nil {
		if p.p4Mechanics && p.Initialized.Support != nil {
			a.MapPhase(p.contactEntities(), p.Initialized.Support.Request != 0)
		}
		p.Cockpit.Map.Markers = p.ContactMapMarkers()
		p.publishMapRoutes()
	}
}

// publishMapRoutes projects the same authoritative route rows used by GPS and
// flight. Source015b:08a8 renders them after contacts; drawing never edits them.
func (p *RenderingPreview) publishMapRoutes() {
	p.syncFollowerNavigation()
	if p.Cockpit.Map == nil || p.Flight == nil || p.Initialized == nil {
		return
	}
	m := p.Cockpit.Map
	m.Observer = byte(p.Initialized.PlayerRow)
	m.RoutePhase = p.Cockpit.BlinkPhase
	if m.Mission != nil {
		// The source display/status words are shared with objective panels.
		// Future objective events update this logical snapshot, never drawing.
		m.Mission.Primary.Status = int16(p.Cockpit.Objectives[0].Status)
		m.Mission.Secondary.Status = int16(p.Cockpit.Objectives[1].Status)
		if p.Initialized.Sortie != nil {
			m.Mission.FARPLosses = p.Initialized.Sortie.FARPLosses
		}
	}
	for i, r := range p.Flight.Navigation {
		v := cockpit.MapRoute{Index: r.Index(), Mode: r.Raw[1], Group: int8(r.Raw[5])}
		for n := 1; n <= 3; n++ {
			pt := r.Point(n)
			v.Points[n-1] = cockpit.MapPoint{H0: pt.H0, H1: pt.H1}
		}
		for _, e := range p.Initialized.Entities {
			if e.FlightSlot == i {
				v.H0, v.H1 = e.Pose.H0, e.Pose.H1
				break
			}
		}
		m.Routes[i] = v
	}
	p.publishSupportStrike()
}

// ContactMapMarkers follows resident015b's admitted contact symbol/table domain.
// No scene Side label or unobserved live entity is turned into a detected mark.
func (p *RenderingPreview) ContactMapMarkers() []cockpit.MapMarker { return p.contactMapMarkers(false) }

func (p *RenderingPreview) contactMapMarkers(gps bool) []cockpit.MapMarker {
	if p.Flight == nil || p.Flight.Avionics == nil {
		return nil
	}
	var out []cockpit.MapMarker
	ptr := [12]uint16{0x6d4, 0x870, 0x885, 0x894, 0x6fa, 0x70d, 0x82b, 0x863, 0x83e, 0x6fa, 0x885, 0x70d}
	entities := p.contactEntities()
	for row, c := range p.Flight.Avionics.Contacts {
		if p.p4Mechanics && entities[row].Present && entities[row].Kind == 0x1b && entities[row].Participation == 1 {
			e := entities[row]
			// Only the four rescue reserves belong to downed flight members.
			reserve := row >= 5 && row <= 8 && len(p.Initialized.FlightRows) == 5 && p.Initialized.FlightRows[row-4] == row-4 && e.Object[10] == 0x1b
			out = append(out, cockpit.MapMarker{H0: e.Pose.H0, H1: e.Pose.H1, Symbol: "map-070d", Colour: 7, Heading: e.Pose.Angles[1], ObjectiveTriangle: gps && reserve})
			continue
		}
		if c.Flags&7 == 0 || c.Kind&0x40 != 0 {
			continue
		}
		// The support controller keeps its home pose after return. MapPhase
		// publishes position, but neither clears the contact nor copies yaw.
		// Project the live friendly aircraft without changing recorded contacts.
		if c.Kind == 0x11 {
			e := entities[row]
			if !e.Present || e.Kind != 0x11 || e.Participation != 1 {
				continue
			}
			c.H0, c.H1, c.Yaw = e.Pose.H0, e.Pose.H1, e.Pose.Angles[1]
		}
		kind := c.Kind & 15
		symbol := uint16(0)
		colour := byte(6)
		yaw := uint16(0)
		if c.Kind == 0x1c || c.Kind == 0x1d {
			symbol = 0x6ed
			colour = 0
		} else if c.Kind&0x20 != 0 {
			symbol = 0x6d4
			colour = 5
			if p.p4Mechanics && entities[row].Present && entities[row].Control[1] != 0 {
				colour = 9
			}
			yaw = c.Yaw
		} else {
			if int(kind) >= len(ptr) {
				continue
			}
			symbol = ptr[kind]
			if c.Kind&0x10 != 0 {
				colour = 7
			} else if c.Flags&7 == 2 {
				colour = 12
			}
			if kind < 3 {
				yaw = c.Yaw
			} else if kind == 4 || kind == 8 {
				yaw = c.Yaw & 0xc000
			}
		}
		triangle := false
		if gps && symbol == 0x70d && p.ResultTargets != nil && entities[row].Present && entities[row].Participation == 1 {
			for role, name := range [...]string{"primary", "secondary"} {
				goal, ok := p.Initialized.Objectives[name]
				status := binary.LittleEndian.Uint16(p.ResultTargets.Records[role+2][32:])
				if ok && goal.SourceRow == row && status == 2 {
					triangle = true
				}
			}
		}
		out = append(out, cockpit.MapMarker{H0: c.H0, H1: c.H1, Symbol: fmt.Sprintf("map-%04x", symbol), Colour: colour, Heading: yaw, ObjectiveTriangle: triangle})
	}
	return out
}
func (p *RenderingPreview) targetAnchor() *render3d.CameraAnchor {
	if p.Flight == nil || p.Flight.Avionics == nil || p.Flight.Avionics.Target == 0 {
		return nil
	}
	row := int(p.Flight.Avionics.Target) - 1
	if row < 0 || row >= len(p.Initialized.Entities) {
		return nil
	}
	e := p.Initialized.Entities[row]
	return &render3d.CameraAnchor{ID: e.ID, Camera: render3d.Camera{Pose: render3d.Pose{H0: e.Pose.H0, H1: e.Pose.H1, Height: e.Pose.Height}, Angles: e.Pose.Angles}, Extent: p.entityExtent(row)}
}

// drawTargetMFD renders a real acquired entity into its source panel. It only
// reads target history. The logical owner advances zoom once, never per draw.
func (p *RenderingPreview) drawTargetMFD(dst []byte, in cockpit.Inputs, a *cockpit.Assets, feedback ...*byte) error {
	return p.drawTargetView(dst, in, a, p.Scene, false, feedback...)
}
func (p *RenderingPreview) drawTargetView(dst []byte, in cockpit.Inputs, a *cockpit.Assets, display render3d.Scene, displayPanel bool, feedback ...*byte) error {
	if p.Flight == nil || p.Flight.Avionics == nil || p.Flight.Avionics.Target == 0 || in.Damage[0]&0x20 != 0 {
		return nil
	}
	if !(in.CameraMode == 0 && in.Arrangement != 2 && in.PrimaryMFD == 5) && in.CameraMode != 7 {
		return nil
	}
	target := p.targetAnchor()
	if target == nil {
		return nil
	}
	family, err := cockpit.Family(in.AircraftID)
	if err != nil {
		return err
	}
	def := a.Definitions[family]
	bounds := render3d.Rect{Left: int(def.Parameters[0x54/2]), Top: int(def.Parameters[0x56/2]), Right: int(def.Parameters[0x58/2]), Bottom: int(def.Parameters[0x5a/2])}
	center := render3d.Point{X: int(def.Parameters[0x50/2]), Y: (bounds.Top + bounds.Bottom) / 2}
	if in.CameraMode == 7 {
		bounds = render3d.Rect{Left: 69, Top: 132, Right: 251, Bottom: 196}
		center = render3d.Point{X: 160, Y: 164}
	}
	row := int(p.Flight.Avionics.Target) - 1
	instance := findInstance(p.instances, target.ID)
	if displayPanel && instance == nil {
		instance = findInstance(display.Instances, target.ID)
	}
	if instance == nil {
		return nil
	}
	s := p.Flight.State
	distance := avionics.Metric(avionics.Delta(target.Camera.H0, s.Pose.H0), avionics.Delta(target.Camera.H1, s.Pose.H1))
	yaw := world.SourceAngle(avionics.Delta(target.Camera.H1, s.Pose.H1), avionics.Delta(target.Camera.H0, s.Pose.H0), p.Renderer.Assets) - 0x4000
	pitch := world.SourceAngle((int16(target.Camera.Height)-s.Pose.Height)>>4, int16(distance), p.Renderer.Assets)
	zoom := int16(p.Flight.Avionics.TargetPanelZoom)
	if zoom <= 0 {
		return nil
	}
	camera := render3d.Camera{Pose: target.Camera.Pose, Angles: [3]uint16{pitch, yaw, 0}}
	camera.H0 += uint32(int32(world.SourceSinAmplitude(yaw, zoom<<1, p.Renderer.Assets.Sine)))
	camera.H1 -= uint32(int32(world.SourceCosAmplitude(yaw, zoom<<1, p.Renderer.Assets.Sine)))
	camera.Height += uint16(world.SourceSinAmplitude(-pitch, zoom<<1, p.Renderer.Assets.Sine))

	scene := p.Scene
	scene.SourceFillReference = scene.SourceFillReference || !displayPanel
	var owners []byte
	if p.collectMaterialMask {
		owners = p.skyMask
	}
	scene.SkyMask = nil
	scene.Camera = camera
	if scene.FloatingGeometry {
		scene.InstanceFractions = display.InstanceFractions
		for i := range display.Instances {
			if display.Instances[i].ID == target.ID {
				instance = &display.Instances[i]
				break
			}
		}
	}
	scene.View = render3d.View{Bounds: bounds, Center: center}
	scene.Environment = nil
	if p.Scene.GraphicsOptions&8 == 0 {
		scene.SkyMask = owners
		scene.View.DepthScale = 2
		angles := instance.Angles
		angles[1] -= s.Pose.Angles[1]
		result, e := p.Renderer.DrawModelPreview(scene, &p.Visual, dst, render3d.ModelPreview{ModelID: instance.ModelID, MutableModel: instance.MutableModel, Angles: angles, Translation: world.Vertex{0, 0, zoom}, ID: instance.ID})
		err = e
		if len(feedback) > 0 && feedback[0] != nil {
			*feedback[0] = result.HorizontalClipFlags
		}
		if err == nil && p.p4Mechanics && p.Flight.deathHistoryCount != 0 && p.Renderer.Assets.Combat != nil {
			for i := int(p.Flight.deathHistoryCount); i < 8; i++ {
				q := p.Flight.deathHistory[i]
				_, err = p.Renderer.DrawModelPreview(scene, &p.Visual, dst, render3d.ModelPreview{GeometryID: p.Renderer.Assets.Combat.DeathHistory[i], Translation: world.Vertex{int16(q.H0 - target.Camera.H0), int16(q.Height - target.Camera.Height), zoom}, ID: fmt.Sprintf("target-death-history-%d", i)})
				if err != nil {
					break
				}
			}
		}
	} else {
		// Source fullscene branch owns its own clipped target destination. The world
		// renderer clears its whole target, so only the source panel is copied back.
		if scene.FloatingGeometry {
			scene.Camera, scene.CameraFraction = floatPanelCamera(*instance, scene.InstanceFractions[target.ID], s.Pose.H0, s.Pose.H1, s.Pose.Height, zoom, camera)
		}
		if displayPanel {
			targetY := float64(int16(target.Camera.Height))
			if scene.FloatingGeometry {
				targetY = float64(int16(instance.Height)) + scene.InstanceFractions[target.ID].Position[1]
			}
			if err := clearTargetPanelTerrain(&scene, p.Renderer.Assets, targetY, zoom); err != nil {
				return err
			}
		}
		if p.p4Mechanics && scene.FloatingGeometry {
			scene.Instances = display.Instances
		} else if p.p4Mechanics {
			scene.Instances = append([]render3d.Instance(nil), p.Scene.Instances...)
			if displayPanel {
				scene = p.displayAirSupport(scene)
			}
		} else {
			scene.Instances = append([]render3d.Instance(nil), p.instances...)
		}
		if p.collectMaterialMask && strings.HasPrefix(scene.World, "EURO") {
			if len(p.targetBrownMask) != len(dst) {
				p.targetBrownMask = make([]byte, len(dst))
			}
			scene.SkyMask = p.targetBrownMask
		}
		layout := scene.Layout()
		if len(p.targetFrame) != len(dst) {
			p.targetFrame = make([]byte, len(dst))
		}
		scratch := p.targetFrame
		for y := layout.Y[bounds.Top]; y < layout.Y[bounds.Bottom+1]; y++ {
			lo, hi := y*layout.Width+layout.X[bounds.Left], y*layout.Width+layout.X[bounds.Right+1]
			copy(scratch[lo:hi], dst[lo:hi])
		}
		_, err = p.Renderer.Draw(scene, &p.Visual, scratch)
		if err == nil {
			w, _ := scene.Dimensions()

			for y := layout.Y[bounds.Top]; y < layout.Y[bounds.Bottom+1]; y++ {
				start, end := y*w+layout.X[bounds.Left], y*w+layout.X[bounds.Right+1]
				copy(dst[start:end], scratch[start:end])
				if len(owners) > 0 {
					clear(owners[start:end])
					if len(scene.SkyMask) > 0 {
						for i, v := range scene.SkyMask[start:end] {
							if groundOwnerAdmits(v, scratch[start+i]) {
								owners[start+i] = v
							}
						}
					}
				}
			}
		}
	}
	if err != nil {
		return err
	}
	// Exact target caption/destination remap is applied over logical interleaved
	// planes at scale2, keeping original glyph pixels and per-pixel destinations.
	name := strings.SplitN(string(p.Initialized.Entities[row].Object.Raw[15:27]), "\x00", 2)[0]
	role := ""
	objectFlags := binary.LittleEndian.Uint16(p.Initialized.Entities[row].Object.Raw[:])
	if objectFlags&0x10 != 0 {
		role = a.Literals["primary"]
	} else if objectFlags&0x20 != 0 {
		role = a.Literals["secondary"]
	}
	bearing := uint16(0x4000 - world.SourceAngle(avionics.Delta(target.Camera.H1, s.Pose.H1), avionics.Delta(target.Camera.H0, s.Pose.H0), p.Renderer.Assets))
	if len(owners) == 0 && scene.SkyMask != nil && p.collectMaterialMask {
		owners = scene.SkyMask
	}
	var sight *cockpit.Inputs
	if displayPanel && in.CameraMode == 0 {
		sight = &in
	}
	return targetCaptionCovered(dst, owners, bounds, name, in.SystemFlags, a, p.Scene.Scale(), distance<<1, bearing, role, in.CameraMode == 7, &p.overlayScratch, sight)
}
func targetCaption(dst []byte, bounds render3d.Rect, name string, flags byte, a *cockpit.Assets, scale int, rangeMetric, bearing uint16, role string, mast bool) error {
	return targetCaptionCovered(dst, nil, bounds, name, flags, a, scale, rangeMetric, bearing, role, mast, &cockpit.OverlayScratch{}, nil)
}
func targetCaptionCovered(dst, owners []byte, bounds render3d.Rect, name string, flags byte, a *cockpit.Assets, scale int, rangeMetric, bearing uint16, role string, mast bool, scratch *cockpit.OverlayScratch, sight *cockpit.Inputs) error {
	caption := func(plane []byte) error {
		if flags&1 == 0 {
			remap := a.ScenePresentation.ConfirmationRemap
			if len(remap) != 256 {
				return fmt.Errorf("target panel remap missing")
			}
			for y := bounds.Top; y <= bounds.Bottom; y++ {
				for x := bounds.Left; x <= bounds.Right; x++ {
					cockpit.PutLogical(plane, y*320+x, remap[plane[y*320+x]]+0x15)
				}
			}
		}
		clip := cockpit.Rect{Left: bounds.Left, Top: bounds.Top, Right: bounds.Right, Bottom: bounds.Bottom}
		nameY, roleY, dataY := bounds.Bottom-12, bounds.Top+2, bounds.Bottom-6
		rangeX, bearingX := bounds.Left+2, bounds.Right-18
		if mast {
			// Source2156..2218 installs a separate text clip0,120,320,200;
			// the sight's caption is one row, not primary-panel bottom offsets.
			clip = cockpit.Rect{Left: 0, Top: 120, Right: 319, Bottom: 199}
			nameY, roleY, dataY, rangeX, bearingX = 180, 172, 180, 100, 200
		}
		if err := a.Text(plane, name, 0, nameY, 31, 0x212, clip); err != nil {
			return err
		}
		if role != "" {
			if err := a.Text(plane, role, 0, roleY, 31, 0x212, clip); err != nil {
				return err
			}
		}
		if err := a.Text(plane, cockpit.RangeText(rangeMetric), rangeX, dataY, 31, 0x12, clip); err != nil {
			return err
		}
		if err := a.Text(plane, cockpit.BearingText(bearing), bearingX, dataY, 31, 0x12, clip); err != nil {
			return err
		}
		if sight != nil {
			return a.DrawPanelReticle(plane, *sight, clip)
		}
		return nil
	}
	area := cockpit.Rect{Left: bounds.Left, Top: bounds.Top, Right: bounds.Right, Bottom: bounds.Bottom}
	if mast {
		area = cockpit.Rect{Left: 0, Top: min(bounds.Top, 120), Right: 319, Bottom: 199}
	}
	return scratch.DrawRect(dst, owners, scale, area, caption)
}

// Use the reached target/ownship and the original panel distance metric and
// arm length. Only the display camera retains the fractional orbit offset.
func floatPanelCamera(target render3d.Instance, q render3d.PoseFraction, playerX, playerZ uint32, playerY, zoom int16, source render3d.Camera) (render3d.Camera, render3d.PoseFraction) {
	x := float64(target.H0) + q.Position[0]
	y := float64(int16(target.Height)) + q.Position[1]
	z := float64(target.H1) + q.Position[2]
	dx, dz := x-float64(playerX), z-float64(playerZ)
	r := math.Max(math.Abs(dx), math.Abs(dz)) + .5*math.Min(math.Abs(dx), math.Abs(dz))
	if r == 0 {
		return source, render3d.PoseFraction{}
	}
	yaw := math.Atan2(dz, dx) - math.Pi/2
	pitch := math.Atan2(y-float64(playerY), r)
	arm := float64(zoom) * 2
	x += arm * math.Sin(yaw)
	z -= arm * math.Cos(yaw)
	y -= arm * math.Sin(pitch)
	pose := render3d.Pose{H0: uint32(int64(math.Round(x))), H1: uint32(int64(math.Round(z))), Height: uint16(int64(math.Round(y)))}
	cam := render3d.Camera{Pose: pose}
	res := render3d.PoseFraction{Position: [3]float64{x - math.Round(x), y - math.Round(y), z - math.Round(z)}}
	for i, a := range [2]float64{pitch, yaw} {
		v := a * 65536 / (2 * math.Pi)
		cam.Angles[i] = uint16(int64(math.Round(v)))
		res.Angles[i] = v - math.Round(v)
	}
	return cam, res
}

// Raise only the displayed full-scene camera. The avionics clip probe keeps
// its original viewpoint, including when it looks through the back of a hill.
func clearTargetPanelTerrain(scene *render3d.Scene, assets *world.Bundle, targetY float64, zoom int16) error {
	cam := &scene.Camera
	ground, err := assets.TerrainHeight(scene.World, world.QueryPose{H0: cam.H0, H1: cam.H1, Height: int16(cam.Height)})
	if _, outside := err.(world.OutsideGridError); outside {
		return nil // No terrain under this orbit position.
	}
	if err != nil {
		return err
	}
	const clearance = 4 // native vertical units above the terrain plane
	floor := min(int(ground)+clearance, math.MaxInt16)
	height := float64(int16(cam.Height))
	if scene.FloatingGeometry {
		height += scene.CameraFraction.Position[1]
	}
	if height >= float64(floor) {
		return nil
	}
	// The horizontal orbit arm is twice the panel zoom. Keep the target
	// anchor centred while preserving the existing orbit position and yaw.
	pitch := math.Atan2(targetY-float64(floor), 2*float64(zoom)) * (65536 / (2 * math.Pi))
	cam.Height = uint16(floor)
	cam.Angles[0] = uint16(int32(math.Round(pitch)))
	if scene.FloatingGeometry {
		scene.CameraFraction.Position[1] = 0
		scene.CameraFraction.Angles[0] = pitch - math.Round(pitch)
	}
	return nil
}

// A shallow presentation copy is read-only during this probe. Its physical
// target is explicitly the reference size even when the window uses scale2.
func (p *RenderingPreview) sourceTargetClip(a *cockpit.Assets, flags *byte) error {
	probe := *p
	probe.collectMaterialMask = false
	probe.Scene.RenderScale = 1
	probe.Scene.FloatingGeometry = false
	probe.Scene.Resolution = ""
	probe.Scene.Instances = withoutPlayerDecoy(probe.Scene.Instances)
	return probe.drawTargetMFD(make([]byte, 64000), p.Cockpit, a, flags)
}

// advanceAvionicsReference retains one canonical320/source world destination
// at the merged flight/visual logical cadence. Display frames never call it.
// That keeps source destination-index feedback independent of user experiments.
func (p *RenderingPreview) advanceAvionicsReference(a *cockpit.Assets) error {
	if p.Inspection || p.Flight == nil {
		return nil
	}
	scene := p.Scene
	scene.FloatingGeometry = false
	scene.SourceFillReference = true
	scene.Instances = withoutPlayerDecoy(scene.Instances)
	scene.RenderScale = 1
	scene.Resolution = ""
	bottom, err := cockpit.Bottom(a, p.Cockpit.AircraftID, p.Cockpit.Arrangement)
	if err != nil {
		return err
	}
	if p.ViewControls != nil {
		if !p.ViewControls.UseArrangementViewport {
			bottom = 199
		}
	} else if p.Cockpit.CameraMode != 0 {
		bottom = 199
	}
	if len(p.avionicsReference) != 64000 {
		p.avionicsReference = make([]byte, 64000)
		p.avionicsReferenceBottom = 199
	}
	// Same source-backed viewport contraction as the existing reference renderer.
	if bottom < p.avionicsReferenceBottom {
		clear(p.avionicsReference[bottom*320:])
	}
	scene.View.Bounds.Bottom = bottom
	p.avionicsReferenceBottom = bottom
	_, err = p.Renderer.Draw(scene, &p.Visual, p.avionicsReference)
	return err
}
