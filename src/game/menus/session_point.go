package menus

import (
	"bytes"
	"fmt"
	"maps"
	"slices"

	"gunship2000-assets/game/replay"
)

type missionPoint struct {
	scene      *RenderingPreview
	point      *replay.SessionPoint
	recorder   filmRecorder
	owner      *MissionSession
	launch     *sortieLaunch
	fuel       [5]fuelReceipt
	checkpoint map[string]string
}

func (c *FrontEnd) pointAvailable() bool {
	if c == nil || c.Screen != ScreenScene || c.filmIsolation != nil || c.Mission == nil {
		return false
	}
	p := c.Rendering
	if p == nil || p.Flight == nil || p.Flight.State == nil || p.Initialized == nil || p.GPS != nil || p.filmTransaction || p.filmPlayback != nil || p.Flight.State.Terminal != nil || (p.Initialized.Sortie != nil && (p.Initialized.Sortie.Result != nil || p.Initialized.Sortie.Pending != "")) {
		return false
	}
	r := p.filmRecorder
	return r != nil && r.Spool != nil && !r.Sealed && r.Error == "" && r.Pending == nil && len(r.Events) == 0
}

// Keep the identity of the live writer and launch film. The previous input and
// boundary row belong to the frozen point and must survive repeated loads.
func (r *filmRecorder) pointRecorder() filmRecorder {
	out := *r
	out.PreviousControl = bytes.Clone(r.PreviousControl)
	out.Events = nil
	out.lastOperation.Payload = bytes.Clone(r.lastOperation.Payload)
	out.lastOperation.Checkpoint = maps.Clone(r.lastOperation.Checkpoint)
	out.lastOperation.Events = slices.Clone(r.lastOperation.Events)
	out.lastFocusReset, out.focusResetValid = focusReset{}, false
	return out
}

func (c *FrontEnd) saveOrLoadPoint() error {
	if !c.pointAvailable() {
		return fmt.Errorf("save point unavailable at this time")
	}
	live := c.Rendering
	mark := live.sessionPoint
	if mark == nil {
		r := live.filmRecorder
		if c.Mission.Initialized != live.Initialized || r.Ordinal != live.Flight.Steps {
			return fmt.Errorf("mission and recorder have different flight positions")
		}
		point, err := r.Spool.Point()
		if err != nil {
			return err
		}
		if point.EventCount() != r.EventIndex {
			return fmt.Errorf("mission and recorder have different event positions")
		}
		scene, err := live.copyPointScene()
		if err != nil {
			return err
		}
		checkpoint := live.FilmCheckpoint()
		if group := compareFilmCheckpoint(checkpoint, scene.FilmCheckpoint()); group != "" {
			return fmt.Errorf("copied mission differs in %s", group)
		}
		recorded := r.lastOperation.Checkpoint
		if r.Spool.Count() == 0 {
			recorded = r.launchCheckpoint
		}
		if group := compareFilmCheckpoint(recorded, checkpoint); group != "" {
			return fmt.Errorf("current mission differs from recorded boundary in %s", group)
		}
		mark = &missionPoint{scene: scene, point: point, recorder: r.pointRecorder(), owner: c.Mission, launch: c.Mission.ResultLaunch, checkpoint: checkpoint}
		if mark.launch != nil {
			mark.fuel = mark.launch.fuel
		}
		live.sessionPoint = mark
		return nil
	}
	if mark.owner != c.Mission || mark.launch != c.Mission.ResultLaunch || mark.recorder.Spool != live.filmRecorder.Spool || mark.recorder.Film != live.filmRecorder.Film {
		return fmt.Errorf("saved point belongs to another mission or recorder")
	}
	p, err := mark.scene.copyPointScene()
	if err != nil {
		return err
	}
	if group := compareFilmCheckpoint(mark.checkpoint, p.FilmCheckpoint()); group != "" {
		return fmt.Errorf("saved mission differs in %s", group)
	}
	r := mark.recorder.pointRecorder()
	p.filmRecorder, p.sessionPoint, p.Renderer = &r, mark, live.Renderer
	p.installFilmObserver()
	if p.Scene.RenderScale != live.Scene.RenderScale || p.Scene.Resolution != live.Scene.Resolution || p.Scene.FloatingGeometry != live.Scene.FloatingGeometry || p.presentation.mode != live.presentation.mode {
		p.presentation.referenceDirty = true
	}
	p.Scene.RenderScale, p.Scene.Resolution, p.Scene.FloatingGeometry = live.Scene.RenderScale, live.Scene.Resolution, live.Scene.FloatingGeometry
	p.presentation.mode = live.presentation.mode
	p.Frame = make([]byte, p.Scene.TargetPixels())
	if len(p.worldFrame) != len(p.Frame) {
		p.worldFrame = make([]byte, len(p.Frame))
		p.presentation.referenceDirty = true
	}
	input := &p.Flight.commandInput
	input.samples = []flightInputSample{{}}
	input.LogicalSampling = p.Flight.Compression.Request != 0
	p.Flight.requireRelease, p.Flight.triggerNeedsRelease = true, true
	if err = mark.point.Restore(r.Spool); err != nil {
		if replay.RecordingFailurePhase(err, "") == "restore-write-position" {
			live.filmRecorder.failRecording("restore-write-position", err)
		}
		return err
	}
	// No fallible operation follows truncation. Publish the prepared scene on
	// the window owner, retaining the original live sortie's commit authority.
	c.Rendering, c.Mission.Initialized = p, p.Initialized
	if mark.launch != nil {
		mark.launch.fuel = mark.fuel
	}
	return nil
}
