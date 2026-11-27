package menus

import (
	"bytes"
	"fmt"
	"io"
	"time"

	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/replay"
)

type pointReconstruction struct {
	mark                 *missionPoint
	reader               *replay.PointReader
	playback             *FilmPlayback
	saving               bool
	clock                flight.Clock
	paused, flightPaused bool
}

func (c *FrontEnd) reconstructPoint(saved *pointReconstruction) (*pointReconstruction, error) {
	if !c.pointAvailable() {
		return nil, fmt.Errorf("save point unavailable at this time")
	}
	p := c.Rendering
	mark := p.sessionPoint
	saving := saved == nil
	if !saving && saved.mark != mark {
		return nil, fmt.Errorf("different reference point")
	}
	if saving {
		r := p.filmRecorder
		point, e := r.Spool.Point()
		if e != nil {
			return nil, e
		}
		mark = &missionPoint{point: point, recorder: *r, owner: c.Mission, launch: c.Mission.ResultLaunch, checkpoint: p.FilmCheckpoint()}
		mark.recorder.PreviousControl = bytes.Clone(r.PreviousControl)
		mark.recorder.focusResetValid = false
		if mark.launch != nil {
			mark.fuel = mark.launch.fuel
		}
	}
	if c.Mission != mark.owner || mark.launch != c.Mission.ResultLaunch {
		return nil, fmt.Errorf("mission owner changed")
	}
	q, e := newFilmPlayback(c.Office.Assets, mark.recorder.Film, c.Runtime, true)
	if e != nil {
		return nil, e
	}
	w := &pointReconstruction{mark: mark, reader: mark.point.Reader(), playback: q, saving: saving, clock: p.Flight.Clock, paused: p.Paused, flightPaused: p.Flight.Paused}
	if !saving {
		w.clock, w.paused, w.flightPaused = saved.clock, saved.paused, saved.flightPaused
	}
	return w, nil
}

// Advance on the window owner in short slices. Until commit, the live controller,
// recorder and career still belong to the previous scene.
func (w *pointReconstruction) advance(c *FrontEnd, budget time.Duration) (done bool, err error) {
	defer func() {
		if v := recover(); v != nil {
			done = true
			err = fmt.Errorf("could not reconstruct recording point: %v", v)
		}
	}()
	until := time.Now().Add(budget)
	q := w.playback
	for {
		o, e := w.reader.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return true, e
		}
		steps := q.reachedSteps
		ended := false
		if e = validateFilmOperation(q.Film, o, &steps, &ended); e != nil {
			return true, e
		}
		if ended {
			return true, fmt.Errorf("mission already ended at recording point")
		}
		if e = q.applyOperation(o); e != nil {
			return true, e
		}
		q.Preview().takeSceneAudio()
		if time.Now().After(until) {
			return false, nil
		}
	}
	mark := w.mark
	p := q.Preview()
	if q.EventCount != mark.recorder.EventIndex || q.reachedSteps != mark.recorder.Ordinal {
		return true, fmt.Errorf("recorded point counters differ")
	}
	if group := compareFilmCheckpoint(mark.checkpoint, p.FilmCheckpoint()); group != "" {
		return true, fmt.Errorf("recorded point differs in %s", group)
	}
	if w.saving {
		c.Rendering.sessionPoint = mark
		return true, nil
	}
	if !c.pointAvailable() || c.Rendering.sessionPoint != mark || c.Mission != mark.owner || c.Mission.ResultLaunch != mark.launch {
		return true, fmt.Errorf("mission no longer owns this point")
	}
	if e := mark.point.Restore(c.Rendering.filmRecorder.Spool); e != nil {
		return true, e
	}
	r := mark.recorder
	p.filmPlayback = nil
	p.replayHashes = nil
	p.filmRecorder = &r
	p.sessionPoint = mark
	p.installFilmObserver()
	c.Rendering = p
	c.Mission.Initialized = p.Initialized
	if mark.launch != nil {
		mark.launch.fuel = mark.fuel
	}
	p.takeSceneAudio()
	// Keep the reached state (including axes, compression, keyboard ownership
	// and defenses) intact at A. Deliver physical releases as ordinary input
	// on the next logical step. The replay engine has no live OS event queue.
	input := &p.Flight.commandInput
	input.samples = []flightInputSample{{}}
	input.started = false
	input.LogicalSampling = p.Flight.Compression.Request != 0
	p.Flight.requireRelease = true
	p.Flight.triggerNeedsRelease = true
	p.Flight.Clock = w.clock
	p.Paused, p.Flight.Paused = w.paused, w.flightPaused
	p.Dirty = true
	return true, nil
}
