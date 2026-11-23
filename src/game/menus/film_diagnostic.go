package menus

import "gunship2000-assets/game/flight"

// Failure evidence is bounded, read-only, and belongs to the viewer's console,
// not the film, pilot, or normal small source text panel. No recovery is inferred.
type FilmFailureDiagnostic struct {
	FilmID, Profile, Error string
	Operation              int
	ReachedSteps, Events   uint64
	Clock                  flight.ReplayClockState
	RNG                    uint16
	Kind                   string
	Input                  string
	Expected, Actual       map[string]string
	Modal                  *filmModal
}

func (q *FilmPlayback) captureFilmFailure(e error) *FilmFailureDiagnostic {
	d := &FilmFailureDiagnostic{Operation: q.Cursor, ReachedSteps: q.reachedSteps, Events: q.EventCount, Error: e.Error()}
	if q.Film != nil {
		d.FilmID = q.Film.Header.ID
		d.Profile = q.Film.Header.Simulation
		if q.Cursor >= 0 && q.Cursor < q.Film.OperationCount() {
			o, _ := q.Film.OperationAt(q.Cursor)
			d.Kind = o.Kind
			d.Expected = o.Checkpoint
			b := o.Payload
			if len(b) > 4096 {
				b = b[:4096]
			}
			d.Input = string(b)
		} else {
			d.Expected = q.Film.End.Checkpoint
		}
	}
	if p := q.Preview(); p != nil && p.Flight != nil {
		d.Clock = p.Flight.Clock.ReplayState()
		d.RNG = p.Initialized.RandomState
		d.Actual = p.FilmCheckpoint()
		if !p.filmLegacy {
			m := p.filmModalState()
			d.Modal = &m
		}
	}
	return d
}
