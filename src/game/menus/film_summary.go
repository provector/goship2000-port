package menus

import (
	"fmt"
	"gunship2000-assets/game/replay"
)

func (p *RenderingPreview) filmInitialKinds() (out [80]byte) {
	for i, e := range p.Initialized.Entities {
		if i >= 80 {
			break
		}
		out[i] = e.Kind
	}
	return
}
func (p *RenderingPreview) frozenFilmSummary(kinds [80]byte, score int16) *replay.Summary {
	if p == nil || p.Flight == nil || p.Initialized.Sortie == nil || p.Initialized.Sortie.Result == nil {
		return nil
	}
	s := &replay.Summary{Score: score, Elapsed: p.Flight.ElapsedDA, Kinds: kinds, Credit: p.Flight.Combat.Credit, Flags: p.Initialized.Sortie.Result.SourceStatus}
	s.Primary = s.Flags&1 != 0
	s.Secondary = s.Flags&2 != 0
	s.TotalKills = replay.CountKills(s.Kinds, s.Credit)
	return s
}
func (q *FilmPlayback) validateFilmSummary() error {
	if group := compareFilmCheckpoint(q.Film.End.Checkpoint, q.Preview().FilmCheckpoint()); group != "" {
		return fmt.Errorf("ending %s checkpoint differs", group)
	}
	got := q.Preview().frozenFilmSummary(q.initialKinds, q.Film.End.Score)
	if got == nil {
		return fmt.Errorf("missing final summary owners")
	}
	if want := q.Film.End.Summary; want != nil && filmHash(want) != filmHash(got) {
		return fmt.Errorf("frozen film summary differs")
	}
	q.summary = got
	return nil
}
func (q *FilmPlayback) Summary() (*replay.Summary, bool) {
	if q == nil {
		return nil, false
	}
	if q.summary != nil {
		s := *q.summary
		return &s, true
	}
	if q.Film.End.Summary != nil {
		s := *q.Film.End.Summary
		return &s, true
	}
	return nil, false
}
