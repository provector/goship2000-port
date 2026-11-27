package menus

import (
	"encoding/json"
	"fmt"
	"image/color"
	"log/slog"
	"strings"
	"time"

	"gunship2000-assets/game/cockpit"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/navigation"
	"gunship2000-assets/game/render3d"
	"gunship2000-assets/game/replay"
	"gunship2000-assets/game/world"
)

// Viewer state is deliberately outside the recorded camera/controller. Original
// view selection is also simulation input; replacing it would change the film.
type ReplayViewer struct {
	Target                              bool
	df                                  uint8
	Playback                            *FilmPlayback
	Stopped, AudioReset, Practice       bool
	Fast, Next, Demo, Map               bool
	DemoSkip                            int16 // Source DEMO BP-local; distinct from live AF3 and VCR Fast.
	MapDetails                          bool
	Selected, Actor, PointerX, PointerY int
	nextEvent                           uint64
	credit                              time.Duration
	yaw                                 uint16
	pitch                               int16
	camera                              render3d.Camera
	cameraKnown                         bool
	previousCamera                      render3d.Camera
	previousInstances                   []render3d.Instance
	alpha                               float64
	frame                               []byte
	skyMask                             []byte
	overlayScratch                      cockpit.OverlayScratch
	eventCursor                         int
	events                              []replay.Event
}

func NewReplayViewer(p *FilmPlayback) *ReplayViewer {
	v := &ReplayViewer{Playback: p, AudioReset: true, Selected: 2, PointerX: 78, PointerY: 174, yaw: 0xc000, df: 10}
	v.updateCamera()
	v.previousCamera = v.camera
	if p != nil && p.Preview() != nil {
		v.previousInstances = append([]render3d.Instance(nil), p.Preview().instances...)
	}
	v.alpha = 1
	return v
}
func (c *FrontEnd) replayHit(x, y int) int {
	if c.Office == nil || c.Office.Assets.Replay == nil {
		return -1
	}
	return c.Office.Assets.Replay.RegionHit(x, y)
}
func (c *FrontEnd) handleReplay(a Actions) error {
	v := c.Viewer
	if v == nil || v.Playback == nil {
		return nil
	}
	if v.Practice {
		if err := v.Playback.HandlePractice(a); err != nil {
			return err
		}
		if v.Playback.Done {
			return c.returnFilmLibrary()
		}
		return nil
	}
	if a.Close || a.Quit || a.Cancel {
		return c.returnFilmLibrary()
	}
	if v.Demo {
		if a.CompressionReset {
			v.DemoSkip = 0
			v.credit = 0
		}
		if a.CompressionIncrease && v.DemoSkip < 16 {
			v.DemoSkip += 2
			v.credit = 0
		}
	}
	if a.FlightFocusLost {
		v.Stopped = true
		v.credit = 0
	}
	if a.Inside {
		if a.PointerRelative {
			v.PointerX = max(0, min(v.PointerX+a.PointerDX/2, 300))
			v.PointerY = max(168, min(v.PointerY+a.PointerDY/2, 190))
		} else {
			v.PointerX = max(0, min(a.X, 300))
			v.PointerY = max(168, min(a.Y, 190))
		}
	}
	selected := -1
	if a.MenuTab {
		v.Selected = (v.Selected + 1) % 18
		selected = -2
	}
	if a.MenuBackTab {
		v.Selected = (v.Selected + 17) % 18
		selected = -2
	}
	if selected == -2 {
		r := c.Office.Assets.Replay.ViewerRegions[v.Selected]
		v.PointerX = (r.X1 + r.X2) / 2
		v.PointerY = (r.Y1 + r.Y2) / 2
		v.PointerX = min(v.PointerX, 300)
		c.pointerCue = &[2]int{v.PointerX, v.PointerY}
	}
	if a.PointerPressed && a.Inside {
		selected = c.replayHit(v.PointerX, v.PointerY)
	} else if a.Enter {
		selected = v.Selected
	}
	if a.Up {
		selected = 5
	}
	if a.Down {
		selected = 6
	}
	if a.Left {
		selected = 7
	}
	if a.Right {
		selected = 8
	}
	if a.ReplayResume {
		selected = 16
	}
	if a.ReplayDemo {
		selected = 17
	}
	if a.ReplayMap {
		selected = 15
	}
	for _, r := range a.Text {
		switch r {
		case 'r', 'R':
			selected = 0
		case 's', 'S':
			selected = 1
		case 'p', 'P':
			selected = 2
		case 'n', 'N':
			selected = 3
		case 'f', 'F':
			selected = 4
		case 't', 'T':
			selected = 14
		case 'm', 'M':
			if !a.ReplayMap {
				v.MapDetails = !v.MapDetails
			}
		case '1', '2', '3', '4', '5':
			selected = 9 + int(r-'1')
		}
	}
	if selected >= 0 {
		return c.activateReplayControl(selected)
	}
	return nil
}
func (c *FrontEnd) activateReplayControl(n int) error {
	v := c.Viewer
	if v == nil || n < 0 || n >= 18 {
		return nil
	}
	p := v.Playback.Preview()
	v.Selected = n
	switch n {
	case 0:
		pb, e := RewindFilmPlayback(c.Office.Assets, v.Playback, c.Runtime)
		if e != nil {
			c.reportDiagnostic("replay", "Cannot rewind this film.", nil)
			return nil
		}
		next := NewReplayViewer(pb)
		next.AudioReset = true
		c.Viewer = next
		c.Message = "" // A successfully rebuilt film owns no error from the old playback.
	case 1:
		v.Stopped = true
		v.Fast = false
		v.Next = false
		v.credit = 0
	case 2:
		v.Stopped = false
		v.Fast = false
		v.Next = false
		v.credit = 0
		v.previousCamera = v.camera
		v.previousInstances = append(v.previousInstances[:0], p.instances...)
	case 3:
		v.Stopped = false
		v.Fast = true
		v.Next = true
		v.nextEvent = v.Playback.EventCount
		v.credit = 0
	case 4:
		v.Stopped = false
		v.Fast = true
		v.Next = false
		v.credit = 0
	case 5:
		v.pitch = max(-16384, v.pitch-(int16(v.df)<<6))
		v.updateCamera()
	case 6:
		v.pitch = min(4096, v.pitch+(int16(v.df)<<6))
		v.updateCamera()
	case 7:
		v.yaw -= uint16(v.df) << 6
		v.updateCamera()
	case 8:
		v.yaw += uint16(v.df) << 6
		v.updateCamera()
	case 9, 10, 11, 12, 13:
		row := n - 9
		if p != nil && row < len(p.Initialized.Entities) && (row == 0 || p.Initialized.Entities[row].Object.Raw[10]&0x20 != 0) {
			v.Actor = row
			v.Target = false
			v.Map = false
			v.Demo = false
			v.updateCamera()
		}
	case 14:
		if p != nil && !v.Target {
			target := int(p.Flight.Avionics.Target)
			if v.Actor > 0 && v.Actor < len(p.Initialized.Entities) && int8(p.Initialized.Entities[v.Actor].ControlRaw[3]) > 0 {
				target = int(int8(p.Initialized.Entities[v.Actor].ControlRaw[3]))
			}
			if target > 0 && target <= len(p.Initialized.Entities) {
				v.Actor = target - 1
				v.Target = true
				v.Map = false
				v.Demo = false
				v.updateCamera()
			}
		}
	case 15:
		v.Map = !v.Map
		v.Demo = false
	case 16:
		if v.Playback.Divergence != "" {
			c.reportDiagnostic("replay", "Resume is unavailable after a film mismatch.", nil)
			return nil
		}
		if e := v.Playback.Resume(); e != nil {
			c.reportDiagnostic("replay", "Resume is unavailable at this film boundary.", nil)
			return nil
		}
		v.Practice = true
		v.Stopped = false
		v.Fast = false
		v.Next = false
		v.credit = 0
		// Resume adopts the existing retained GS2 driver, including its cursor,
		// row priorities and pause state. Resetting it here loses future0bh calls.
	case 17:
		if v.Stopped {
			return nil
		} // Resident05a2 admits DEMO only in mode1.
		v.Demo = !v.Demo
		v.Map = false
		v.Fast, v.Next = false, false
		v.DemoSkip = 0 // Source normal/demo invocation initializes its own BP-local.
		v.credit = 0
	}
	return nil
}

// Source section3:161a..1787 follows the selected actor with word-angle
// arithmetic and prepared Q14 lookup, without assigning logical ViewControls.
func (v *ReplayViewer) updateCamera() {
	p := v.Playback.Preview()
	if p == nil || v.Actor < 0 || v.Actor >= len(p.Initialized.Entities) {
		return
	}
	e := p.Initialized.Entities[v.Actor]
	a := p.Renderer.Assets
	extent := int16(0)
	if e.ModelID >= 0 && e.ModelID < len(a.Models) {
		if g := a.Geometries[a.Models[e.ModelID]]; g != nil {
			extent = g.Extent
		}
	}
	if !v.cameraKnown {
		v.camera = render3d.Camera{Pose: render3d.Pose{H0: e.Pose.H0, H1: e.Pose.H1, Height: e.Pose.Height}, Angles: e.Pose.Angles}
		v.cameraKnown = true
	}
	v.previousCamera = v.camera
	yaw := v.camera.Angles[1] - uint16(int16(v.camera.Angles[1]-e.Pose.Angles[1]-v.yaw-0x4000)>>2)
	radius := world.SourceCosAmplitude(uint16(v.pitch), extent<<2, a.Sine)
	v.camera.H0 = uint32((int32(v.camera.H0)+int32(e.Pose.H0))>>1) + uint32(int32(world.SourceSinAmplitude(yaw, radius, a.Sine)))
	v.camera.H1 = uint32((int32(v.camera.H1)+int32(e.Pose.H1))>>1) - uint32(int32(world.SourceCosAmplitude(yaw, radius, a.Sine)))
	v.camera.Height = uint16(int16(e.Pose.Height) + world.SourceSinAmplitude(uint16(v.pitch), -(extent<<3), a.Sine))
	// Original section3:16f6..1710 calls resident314c, including model
	// support, before the horizontal clamps at1713 onward. Terrain-only
	// sampling can leave the observational camera at carrier-deck level.
	if h, e := a.ContactHeight(p.Scene.World, world.QueryPose{H0: v.camera.H0, H1: v.camera.H1, Height: int16(v.camera.Height)}, p.Initialized.ContactObjects()); e == nil && int16(v.camera.Height) < h+32 {
		v.camera.Height = uint16(h + 64)
	}
	v.camera.H0 = max(0x4000, min(v.camera.H0, 0x7c000))
	v.camera.H1 = max(0x4000, min(v.camera.H1, 0x7c000))
	v.camera.Angles = [3]uint16{uint16(v.pitch), yaw, 0}
}
func (c *FrontEnd) AdvanceReplay(seconds float64) error {
	if c.rejectedCheck != nil {
		return c.advanceRejectedFilm()
	}
	v := c.Viewer
	if c.Screen != ScreenReplay || v == nil || v.Playback == nil {
		return nil
	}
	if v.Practice {
		if e := v.Playback.AdvancePractice(seconds); e != nil {
			return e
		}
		if v.Playback.Done {
			return c.returnFilmLibrary()
		}
		return nil
	}
	if v.Stopped || v.Playback.Divergence != "" {
		return nil
	}
	if seconds < 0 {
		seconds = 0
	}
	if seconds > 0.25 {
		seconds = 0.25
	}
	rate := 1
	if v.Demo && v.DemoSkip != 0 {
		rate = int(v.DemoSkip) + 1
	} else if v.Fast {
		rate = 33
	} // Named native draw-skip mapping: ordinary steps, never multiplied DF/pitch.
	p := v.Playback.Preview()
	if p.Flight.terminalAwaitingAck {
		return nil
	}
	if p.Flight.State.Terminal != nil {
		rate = 1
		v.Fast = false
		v.Next = false
		v.DemoSkip = 0
	}
	v.credit += time.Duration(seconds*float64(time.Second)) * time.Duration(rate)
	// Old films can contain thousands of modal boundaries between adjacent
	// flight steps. Spend spare fast-forward time checking those boundaries,
	// yielding between complete operations for the next input/present cycle.
	var deadline time.Time
	if v.Fast {
		deadline = time.Now().Add(8 * time.Millisecond)
	}
	for work := 0; !v.Playback.Done; work++ {
		if work >= 16 && (!v.Fast || work >= 256 || time.Now().After(deadline)) {
			break
		}
		o, readErr := v.Playback.Film.OperationAt(v.Playback.Cursor)
		if readErr != nil {
			v.Stopped = true
			c.Message = "Film cannot be read."
			c.recordDiagnostic(slog.LevelWarn, "replay", c.Message, v.Playback.fail(readErr))
			return nil
		}
		logical := o.Kind == "step" || o.Kind == "terminal"
		if logical && (work >= 16 || v.credit < flight.StepDuration()) {
			break
		}
		if logical {
			v.credit -= flight.StepDuration()
			v.previousInstances = append(v.previousInstances[:0], p.instances...)
		}
		before := v.Playback.EventCount
		if e := v.Playback.Step(); e != nil {
			v.Stopped = true
			c.Message = "Film differs from the supported simulation. Playback stopped."
			c.recordDiagnostic(slog.LevelWarn, "replay", c.Message, e)
			return nil
		}
		if logical {
			var consumed filmStep
			_ = json.Unmarshal(o.Payload, &consumed)
			v.df = consumed.Timing.DF
			// Following each verified pose keeps the spectator camera in step with
			// moving actors during fast-forward, including a stop at the next event.
			v.updateCamera()
		}
		if v.Next && v.Playback.EventCount > v.nextEvent && v.Playback.EventCount > before {
			v.Next = false
			v.Fast = false
			v.Selected = 2
			v.credit = 0
			break
		}
		if p.Flight.State.Terminal != nil || p.Flight.terminalAwaitingAck {
			v.Fast = false
			v.Next = false
			v.DemoSkip = 0
			v.credit = 0
			break
		}
	}
	// Bounded work drops presentation debt, never operations or recorded state.
	if v.credit > 16*flight.StepDuration() {
		v.credit = 0
	}
	v.alpha = float64(v.credit) / float64(flight.StepDuration())
	if v.alpha > 1 {
		v.alpha = 1
	}
	if v.Fast || p.Flight.State.Terminal != nil {
		v.alpha = 1
	}
	// Adjacent poses only: this is the same presentation-only latency policy
	// as normal native flight. It never restores film clock or input state.
	p.presentation.alpha = v.alpha
	// DEMO uses the ordinary renderer too. Explicit reference operations own
	// Visual; this private bounded fraction only displays its adjacent states.
	p.visualTime = int64(v.alpha * float64(time.Second))
	p.Dirty = true
	if v.Playback.Done && v.Playback.Divergence == "" {
		return c.returnFilmLibrary()
	}
	return nil
}
func (c *FrontEnd) replayPalette() color.Palette {
	if c.Viewer != nil && c.Viewer.Playback != nil {
		v := c.Viewer
		if v.Practice {
			return v.Playback.Engine.CurrentPalette()
		}
		p := v.Playback.Preview()
		if r := p.Flight; r.TerminalPresentation.Active && len(r.terminalRGB6) == 768 {
			return (world.Palette{RGB6: r.terminalRGB6}).Colours()
		}
		if p.PaletteState != nil {
			return p.PaletteState.Colours()
		}
		scene := p.Scene
		scene.Camera = v.presentationCamera(p)
		if pal, e := p.Renderer.Palette(scene, &p.Visual); e == nil {
			return pal
		}
		return p.Palette
	}
	return nil
}
func (c *FrontEnd) replayFrame(pointer bool) []byte {
	v := c.Viewer
	if v == nil || v.Playback == nil {
		return make([]byte, 64000)
	}
	p := v.Playback.Preview()
	if v.Practice {
		return v.Playback.Engine.Frame(pointer)
	}
	scene := p.Scene
	scale := scene.Scale()
	if len(v.frame) != scene.TargetPixels() {
		v.frame = make([]byte, scene.TargetPixels())
	}
	if v.Demo || p.Flight.TerminalPresentation.Active {
		recorded := v.Playback.Engine.Frame(false)
		if len(recorded) == len(v.frame) {
			copy(v.frame, recorded)
		} else if p.GPS != nil && len(recorded) == 64000 {
			// GPS is source-coordinate modal artwork, not a low-resolution3D
			// substitute. Ordinary simulation likewise uses this logical UI.
			layout := scene.Layout()
			for y := layout.Top; y < layout.Y[200]; y++ {
				for x := 0; x < layout.Width; x++ {
					v.frame[y*layout.Width+x] = recorded[layout.LogicalY[y]*320+layout.LogicalX[x]]
				}
			}
		} else {
			c.reportDiagnostic("replay", "Film view unavailable.", nil)
			clear(v.frame)
		}
	} else {
		scene.Camera = v.presentationCamera(p)
		if scene.FloatingGeometry && p.presentation.mode == "smooth" && p.Flight.State.Terminal == nil {
			scene.CameraFraction = poseFraction(v.previousCamera.Pose, v.camera.Pose, scene.Camera.Pose, v.previousCamera.Angles, v.camera.Angles, scene.Camera.Angles, v.alpha)
		}
		if strings.HasPrefix(scene.World, "EURO") {
			if len(v.skyMask) != len(v.frame) {
				v.skyMask = make([]byte, len(v.frame))
			}
			scene.SkyMask = v.skyMask
		} else {
			v.skyMask = nil
		}
		// Source0:155b..1588 sets center113,82 and the original viewer
		// viewport2,3..225,163. This is not the full cockpit projection.
		scene.View = render3d.View{Bounds: render3d.Rect{Left: 2, Top: 3, Right: 225, Bottom: 163}, Center: render3d.Point{X: 113, Y: 82}}
		// Rebuild the render list locally: the recorded cockpit may omit ownship.
		scene.Instances = nil
		ids := map[string]bool{}
		for _, in := range p.instances {
			visible := true
			for _, e := range p.Initialized.Entities {
				if e.ID == in.ID && e.ID != p.Initialized.PlayerID && (e.Kind == 255 || e.Kind&0x40 != 0 || e.Participation == 0) {
					visible = false
					break
				}
			}
			if visible {
				if p.presentation.mode == "smooth" && p.Flight.State.Terminal == nil {
					for _, before := range v.previousInstances {
						if before.ID == in.ID && before.ModelID == in.ModelID {
							current := in
							in.Pose = lerpPose(before.Pose, in.Pose, v.alpha)
							in.Angles = lerpAngles(before.Angles, in.Angles, v.alpha)
							if scene.FloatingGeometry {
								retainInstanceFraction(&scene, before, current, in, v.alpha)
							}
							break
						}
					}
				}
				scene.Instances = append(scene.Instances, in)
				ids[in.ID] = true
			}
		}
		for _, in := range p.Scene.Instances {
			if !ids[in.ID] {
				found := false
				for _, base := range p.instances {
					if in.ID == base.ID {
						found = true
						break
					}
				}
				if !found {
					scene.Instances = append(scene.Instances, in)
				}
			}
		}
		// Use the same private rotor/component presentation as the live view.
		// Playback credit supplies interpolation, never simulation input.
		parkingFocus := ""
		if v.Actor >= 0 && v.Actor < len(p.Initialized.Entities) {
			parkingFocus = p.Initialized.Entities[v.Actor].ID
		}
		scene = p.displayAirSupport(scene)
		scene, _ = p.parkedPresentation(scene, parkingFocus)
		if v.Actor >= 0 && v.Actor < len(p.Initialized.Entities) {
			id := p.Initialized.Entities[v.Actor].ID
			for _, focus := range scene.Instances {
				if focus.ID == id {
					scene.Camera = p.Renderer.ObserverCamera(scene, focus)
					break
				}
			}
		}
		visual, _ := p.presentationVisual(scene, v.alpha, p.presentation.mode == "smooth" && p.Flight.State.Terminal == nil)
		if _, e := p.Renderer.Draw(scene, &visual, v.frame); e != nil {
			c.reportDiagnostic("replay", "Film view unavailable.", nil)
		}
	}
	// DEMO is the source normal cockpit loop, not the loose external VCR
	// panel pasted over its instruments. Its skip local has a separate caller.
	if v.Demo {
		if p.Flight.State.Terminal == nil && !p.Flight.TerminalPresentation.Active {
			_ = p.overlayScratch.Draw(v.frame, p.skyMask, scale, func(logical []byte) error { c.drawReplayDemo(logical, 1); return nil })
		}
		return v.frame
	}
	a := c.Office.Assets.Replay
	var mapImage []byte
	if v.Map {
		routes := p.Flight.Navigation
		var poses [5]navigation.Point
		var active [5]bool
		for i, row := range p.Initialized.FlightRows {
			if i >= 5 || row < 0 || row >= len(p.Initialized.Entities) {
				continue
			}
			e := p.Initialized.Entities[row]
			poses[i] = navigation.Point{H0: e.Pose.H0, H1: e.Pose.H1}
			active[i] = e.Participation != 0
		}
		g := navigation.New(&routes, poses, active)
		for i, row := range p.Initialized.FlightRows {
			if i < 5 && row >= 0 && row < len(p.Initialized.Entities) {
				g.Headings[i] = p.Initialized.Entities[row].Pose.Angles[1]
			}
		}
		tmp := make([]byte, 64000)
		if e := g.Draw(tmp, p.Renderer.Assets.Worlds[p.Scene.World], c.Office.Assets.Cockpit); e == nil {
			mapImage = tmp
		}
	}
	composeViewer := func(dst []byte) error {
		if mapImage != nil {
			for y := 3; y <= 163; y++ {
				copy(dst[y*320+2:y*320+226], mapImage[y*320+2:y*320+226])
			}
		}
		if len(a.ViewerPanel) == 64000 {
			for y := 0; y < 200; y++ {
				for x := 0; x < 320; x++ {
					if x < 2 || x > 225 || y < 3 || y > 163 {
						cockpit.PutLogical(dst, y*320+x, a.ViewerPanel[y*320+x])
					}
				}
			}
		}
		// Active button state replaces source palette12 only; surrounding artwork stays.
		selected := 2
		if v.Stopped {
			selected = 1
		} else if v.Next {
			selected = 3
		} else if v.Fast {
			selected = 4
		}
		a.ReplaceRegionIndices(dst, selected, 12, 7)
		// Source3:7a8..7e5 has a separate available-target ink14 carrier.
		// It is not an additional selected transport or actor button.
		if !v.Target {
			target := int(p.Flight.Avionics.Target)
			if v.Actor > 0 && v.Actor < len(p.Initialized.Entities) && int8(p.Initialized.Entities[v.Actor].ControlRaw[3]) > 0 {
				target = int(int8(p.Initialized.Entities[v.Actor].ControlRaw[3]))
			}
			if target > 0 && target <= len(p.Initialized.Entities) {
				a.ReplaceRegionIndices(dst, 14, 12, 14)
			}
		}
		if v.Actor >= 0 && v.Actor < 5 {
			if !v.Target {
				a.ReplaceRegionIndices(dst, 9+v.Actor, 12, 7)
			}
		}
		if v.Target {
			a.ReplaceRegionIndices(dst, 14, 12, 7)
		}
		if v.Map {
			a.ReplaceRegionIndices(dst, 15, 12, 7)
		}
		if v.Demo {
			a.ReplaceRegionIndices(dst, 17, 12, 7)
		}
		if ca := c.Office.Assets.Cockpit; ca != nil {
			elapsed := p.Cockpit.MissionCounter
			_ = ca.Number(dst, a.Styles.ClockMinutes[0], a.Styles.ClockMinutes[1], int16(elapsed/360), a.Styles.ClockStyle)
			_ = ca.Number(dst, a.Styles.ClockSeconds[0], a.Styles.ClockSeconds[1], int16((elapsed/6)%60), a.Styles.ClockStyle)
			c.drawReplaySummary(dst)
			c.drawReplayEvents(dst)
			c.drawReplayStats(dst)
			if c.Message != "" {
				for y := 64; y < 164; y++ {
					for x := 234; x < 316; x++ {
						cockpit.PutLogical(dst, y*320+x, 8)
					}
				}
				_ = ca.Text(dst, c.Message, 234, 70, 31, 0x102, cockpit.Rect{234, 64, 315, 163})
			}
		}
		if pointer {
			a.DrawViewerPointer(dst, v.PointerX, v.PointerY)
		}
		return nil
	}
	var owners []byte
	if !v.Map {
		owners = v.skyMask
	}
	_ = v.overlayScratch.Draw(v.frame, owners, scale, composeViewer)
	return v.frame
}
func (c *FrontEnd) drawReplayEvents(dst []byte) {
	v := c.Viewer
	a := c.Office.Assets.Replay
	ca := c.Office.Assets.Cockpit
	for v.eventCursor < v.Playback.Cursor {
		op, e := v.Playback.Film.OperationAt(v.eventCursor)
		if e != nil {
			break
		}
		v.events = append(v.events, op.Events...)
		if len(v.events) > 4 {
			v.events = v.events[len(v.events)-4:]
		}
		v.eventCursor++
	}
	events := v.events
	for y := 64; y < 164; y++ {
		for x := 234; x < 316; x++ {
			cockpit.PutLogical(dst, y*320+x, 8)
		}
	}
	name := func(row byte) string {
		p := v.Playback.Preview()
		if int(row) < len(p.Initialized.Entities) {
			return stringTrimZero(p.Initialized.Entities[row].Object.Raw[15:27])
		}
		return ""
	}
	for i, e := range events {
		text := ""
		switch e.Kind {
		case 1:
			text = name(e.Target) + a.Text["destroyed_by"] + name(e.Actor)
		case 2:
			text = name(e.Actor) + a.Text["takes_hit"]
			damage := func(n byte) string { return a.Text[fmt.Sprintf("damage_%d", n)] }
			if e.Target&0xf0 != 0 {
				text += damage(e.Target >> 4)
				if e.Target&15 != 0 {
					text += a.Text["and"] + damage(e.Target&15)
				}
			} else {
				text += damage(e.Target)
			}
		case 3:
			text = name(e.Actor) + a.Text["goes_in"]
		case 4:
			text = a.Text["primary_achieved"]
		case 5:
			text = a.Text["secondary_achieved"]
		case 6:
			text = name(e.Target) + a.Text["rescued_by"] + name(e.Actor)
		}
		y := 64 + 24*i
		clip := cockpit.Rect{232, y, 315, min(y+23, 163)}
		_ = ca.Text(dst, a.Text["event_time"], 232, y, 2, 2, clip)
		_ = ca.Text(dst, fmt.Sprintf("%02d", e.Sequence), 256, y, 31, 2, clip)
		_ = ca.Text(dst, fmt.Sprintf("%02d:%02d", e.Timestamp/360, (e.Timestamp/6)%60), 289, y, 31, 2, clip)
		_ = ca.Text(dst, text, 234, y+6, 10, 0x102, clip)
	}
}

// Source3:1ac0..1bef uses indicated player speed, an independently retained
// follower motion word, unsigned heading conversion, and signed height units.
func (c *FrontEnd) drawReplayStats(dst []byte) {
	v := c.Viewer
	if v.Map && !v.MapDetails || v.Demo || v.Actor < 0 {
		return
	}
	p := v.Playback.Preview()
	if v.Actor >= len(p.Initialized.Entities) {
		return
	}
	e := p.Initialized.Entities[v.Actor]
	// Viewer3:1a9b calls MZ-resident0000:0c2a (file122a), not
	// overlay0:c2a: its first operation centers the actual actor name.
	_ = c.Office.Assets.Cockpit.Text(dst, stringTrimZero(e.Object.Raw[15:27]), 112, 142, 31, 0x832, cockpit.Rect{2, 3, 225, 163})
	// Source B56 minus OBJECT base B4C is byte0Ah, not the row's flags0.
	if e.Object.Raw[10]&0x20 == 0 {
		return
	}
	speed := e.RawMotion
	if v.Actor == p.Initialized.PlayerRow {
		speed = p.Flight.State.IndicatedSpeed
	} else if speed != 0 {
		speed += int16(p.Flight.Clock.ReplayState().Phase & 7)
	}
	a := c.Office.Assets.Replay
	ca := c.Office.Assets.Cockpit
	values := []struct {
		key   string
		x     int
		value int
	}{{"airspeed", 8, int(speed)}, {"heading", 88, int(uint16(-e.Pose.Angles[1]) / 182)}, {"altitude", 156, int((int32(int16(e.Pose.Height)-2) * 25) >> 5)}}
	for _, v := range values {
		_ = ca.Text(dst, a.Text[v.key]+fmt.Sprint(v.value), v.x, 8, 31, 0x32, cockpit.Full)
	}
}

// presentationCamera applies the same previous-to-current latency policy as
// ordinary smooth simulation. Palette and world drawing share this exact pose.
func (v *ReplayViewer) presentationCamera(p *RenderingPreview) render3d.Camera {
	camera := v.camera
	if p.presentation.mode == "smooth" && p.Flight.State.Terminal == nil {
		camera.Pose = lerpPose(v.previousCamera.Pose, v.camera.Pose, v.alpha)
		camera.Angles = lerpAngles(v.previousCamera.Angles, v.camera.Angles, v.alpha)
	}
	return camera
}

func (c *FrontEnd) drawReplayDemo(dst []byte, scale int) {
	v := c.Viewer
	p := v.Playback.Preview()
	if p.Flight.State.Terminal != nil || p.Flight.TerminalPresentation.Active {
		return
	}
	a := c.Office.Assets.Cockpit
	text := c.Office.Assets.Replay.Text
	// GS2 section1:2233: hexadecimal masks on recorded DA, not host time.
	da := p.Cockpit.MissionCounter
	if da&0x100 != 0 && da&0x80 == 0 && da&0x04 != 0 {
		_ = a.TextScaled(dst, text["demo_title"], 160, 64, 14, 0x832, cockpit.Full, scale)
		_ = a.TextScaled(dst, text["demo_copyright"], 160, 76, 14, 0x832, cockpit.Full, scale)
		_ = a.TextScaled(dst, "GoShip 2000 port by _provector '26", 160, 176, 14, 0x832, cockpit.Full, scale)
	}
	if v.DemoSkip != 0 {
		_ = cockpit.LogicalPass(dst, scale, func(logical []byte) error { return a.Number(logical, 4, 8, v.DemoSkip, 0x102) })
	}
}

func (c *FrontEnd) drawReplaySummary(dst []byte) {
	summary, ok := c.Viewer.Playback.Summary()
	if !ok {
		return
	} // Old films expose no invented summary before full verification.
	a, ca := c.Office.Assets.Replay, c.Office.Assets.Cockpit
	clip := cockpit.Rect{232, 16, 319, 63}
	for _, v := range []struct {
		text string
		y    int
	}{
		{a.Text["mission_score"] + fmt.Sprint(summary.Score), 16},
		{a.Text["time_taken"] + fmt.Sprintf("%d:%02d", summary.Elapsed/360, (summary.Elapsed/6)%60), 24},
		{a.Text["total_kills"] + fmt.Sprint(summary.TotalKills), 32},
	} {
		_ = ca.Text(dst, v.text, 232, v.y, 7, 0x0002, clip)
	}
	y := 40
	if summary.Primary {
		_ = ca.Text(dst, a.Text["primary_achieved"], 232, y, 7, 0x0002, clip)
		y += 8
	}
	if summary.Secondary {
		_ = ca.Text(dst, a.Text["secondary_achieved"], 232, y, 7, 0x0002, clip)
	}
}
