package menus

import (
	"encoding/json"
	"fmt"
	"gunship2000-assets/game/diagnostics"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"log/slog"
	"time"
)

// A rejected score may still have a complete film. Strict reconstruction runs
// on the ordinary UI thread in short batches so the window keeps polling input.
type rejectedFilmCheck struct {
	published      <-chan rejectionWrite
	closeRequested bool
	reason         error
	sortie         string
	playback       *FilmPlayback
}

func (c *FrontEnd) rejectSortieResult(reason error, id string) error {
	diagnostics.Report(id+"/rejection", reason.Error(), "Your mission result could not be saved.")
	film, err := c.sealedRecording()
	if err == nil && film != nil {
		diagnostics.Info("replay", "Verifying complete recording before unscored publication")
		var q *FilmPlayback
		q, err = NewFilmPlayback(c.Office.Assets, film, c.Runtime)
		if err == nil {
			c.rejectedCheck = &rejectedFilmCheck{reason: reason, sortie: id, playback: q}
			c.Library = &FilmLibrary{Notice: "Career result unavailable. Checking the recording for unscored playback."}
			c.Screen = ScreenFilms
			return nil
		}
	}
	return c.finishRejectedFilm(reason, id, film, err)
}

func (c *FrontEnd) advanceRejectedFilm() error {
	check := c.rejectedCheck
	q := check.playback
	if check.published != nil {
		select {
		case result := <-check.published:
			c.rejectedCheck = nil
			if result.persistErr != nil {
				return c.failPreparationSave("retaining unresolved sortie", result.persistErr)
			}
			if result.adopt != nil {
				result.adopt()
			}
			if e := c.showRejectedFilm(check.reason, check.sortie, q.Film, result.filmErr); e != nil {
				return e
			}
			if check.closeRequested {
				return c.Handle(Actions{Close: true})
			}
			return nil
		default:
			return nil
		}
	}
	deadline := time.Now().Add(7 * time.Millisecond)
	for i := 0; i < 32; i++ {
		err := q.Step()
		if err == nil && q.Done {
			done := make(chan rejectionWrite, 1)
			check.published = done
			c.Library.Notice = "Recording verified. Saving the unscored film."
			store := replay.Store{Root: c.FilmDirectory}
			film := q.Film
			why := check.reason.Error()
			career := c.Store
			raw, freezeErr := c.rejectedEvidence(check.sortie, why, film.Header.ID)
			id := check.sortie
			go func() {
				result := rejectionWrite{persistErr: freezeErr}
				result.filmErr = publishVerifiedUnscored(store, film, why)
				if id != "" && result.persistErr == nil {
					if result.filmErr != nil && raw != nil {
						var fields map[string]json.RawMessage
						result.persistErr = json.Unmarshal(raw, &fields)
						if result.persistErr == nil {
							fields["FilmID"] = json.RawMessage(`""`)
							raw, result.persistErr = json.Marshal(fields)
						}
					}
					if result.persistErr == nil {
						result.adopt, result.persistErr = career.ArchiveRejectedSortie(id, why, raw)
					}
				}
				done <- result
			}()
			return nil
		}
		if err != nil {
			c.rejectedCheck = nil
			return c.finishRejectedFilm(check.reason, check.sortie, q.Film, err)
		}
		if time.Now().After(deadline) {
			break
		}
	}
	percent := 100 * q.Cursor / q.Film.OperationCount()
	c.Library.Notice = fmt.Sprintf("Career result unavailable. Verifying recording: %d%%", percent)
	return nil
}

func publishVerifiedUnscored(store replay.Store, film *replay.Film, why string) error {
	var filmErr error
	filmErr = store.PublishUnscored(film, why)
	if filmErr == nil {
		// The PLAY notice describes a committed managed entry and its readable
		// sidecar, not an in-memory recorder or a temporary spool.
		var reopened *replay.Film
		reopened, filmErr = store.LoadEntry(replay.Entry{ID: film.Header.ID})
		if filmErr == nil {
			_, filmErr = store.Unscored(reopened)
		}
	}
	return filmErr
}

type rejectionWrite struct {
	filmErr, persistErr error
	adopt               func()
}

func (c *FrontEnd) rejectedEvidence(id, reason, published string) ([]byte, error) {
	p := c.Rendering
	if id == "" || p == nil || !p.corridorNavigation || c.Mission.ResultLaunch == nil {
		return nil, nil
	}
	var sys *flight.Systems
	var elapsed *uint16
	var credit *[40]byte
	if r := p.Flight; r != nil {
		sys = &r.Systems
		elapsed = &r.ElapsedDA
		if r.Combat != nil {
			credit = &r.Combat.Credit
		}
	}
	return json.Marshal(struct {
		Version        int
		Reason, FilmID string
		Launch         *sortieLaunch
		Mission        *gamestate.InitializedMission
		Systems        *flight.Systems
		Elapsed        *uint16
		Credit         *[40]byte
	}{1, reason, published, c.Mission.ResultLaunch, p.Initialized, sys, elapsed, credit})
}

func (c *FrontEnd) finishRejectedFilm(reason error, id string, film *replay.Film, filmErr error) error {
	if id != "" {
		published := ""
		if film != nil && filmErr == nil {
			published = film.Header.ID
		}
		raw, e := c.rejectedEvidence(id, reason.Error(), published)
		if e == nil {
			var adopt func()
			adopt, e = c.Store.ArchiveRejectedSortie(id, reason.Error(), raw)
			if e == nil {
				adopt()
			}
		}
		if e != nil {
			return c.failPreparationSave("retaining unresolved sortie", e)
		}
	}
	return c.showRejectedFilm(reason, id, film, filmErr)
}
func (c *FrontEnd) showRejectedFilm(reason error, id string, film *replay.Film, filmErr error) error {
	published := ""
	if film != nil {
		if filmErr != nil && c.filmDiagnostics != nil {
			c.filmDiagnostics.RecordFailure("verify-unscored", uint64(film.OperationCount()), film.End.Steps, filmErr)
		}

		if filmErr == nil {
			published = film.Header.ID
			diagnostics.Info("replay", "Complete unscored film preserved after career rejection", "film", published)
		} else if c.filmDiagnostics != nil {
			c.filmDiagnostics.RecordFailure("publish-unscored", uint64(film.OperationCount()), film.End.Steps, filmErr)
		}
	}
	diagnostics.Warn("career", "Sortie ended without a career award", "reason", reason)
	c.Mission.ResultLaunch = nil
	if err := c.returnRenderingBriefing(); err != nil {
		return err
	}
	notice := "CAREER RESULT NOT SAVED\n" + reason.Error() + "\n"
	if published != "" {
		notice += "Complete unscored film retained. In Film Library select Unscored Mission and press PLAY. No career award was applied."
	} else if filmErr != nil {
		notice += "Recording unavailable for complete playback: " + filmErr.Error() + "\nAny valid prefix and first failure remain in the film directory for explicit diagnostic recovery."
		c.recordDiagnostic(slog.LevelWarn, "replay", "Recording retained for explicit recovery; no complete film published", filmErr)
	} else {
		notice += "No mission recording was available."
	}
	c.Message = "No career changes: " + reason.Error()
	// Preserve this exact accepted preparation for retry; ordinary openFilmLibrary
	// refreshes the office and clears it. This return token is presentation-only.
	if id != "" && filmPresentationReady(c.Office.Assets) {
		c.Library = &FilmLibrary{Store: replay.Store{Root: c.FilmDirectory}, Focus: 1, Return: filmReturn{RetainedBriefing: true}, Notice: notice}
		c.Screen = ScreenFilms
		c.Viewer = nil
		if err := c.refreshFilmList(published); err != nil {
			return err
		}
	}
	return nil
}

func (c *FrontEnd) filmNoticeLines(text string) []string {
	font := c.Menus.Fonts[3]
	out := []string{}
	line := ""
	for _, r := range text {
		if r == '\n' {
			out = append(out, line)
			line = ""
			continue
		}
		next := line + string(r)
		if line != "" && font.Measure([]byte(next)) > 284 {
			out = append(out, line)
			line = string(r)
		} else {
			line = next
		}
	}
	return append(out, line)
}
func (c *FrontEnd) drawFilmNotice(frame []byte) {
	l := c.Library
	if l == nil || l.Notice == "" {
		return
	}
	// Native recording/persistence information is separate from original bottom
	// hints and from source debrief outcomes. It cannot acknowledge a career.
	for y := 10; y < 190; y++ {
		for x := 10; x < 310; x++ {
			frame[y*320+x] = 0
		}
	}
	lines := c.filmNoticeLines(l.Notice)
	pages := max(1, (len(lines)+16)/17)
	l.NoticePage = max(0, min(l.NoticePage, pages-1))
	clip := [4]int{14, 14, 292, 172}
	c.filmText(frame, 3, "RECORDING / CAREER STATUS", 16, 15, 15, clip)
	for n := 0; n < 17 && l.NoticePage*17+n < len(lines); n++ {
		c.filmText(frame, 3, lines[l.NoticePage*17+n], 16, 30+n*8, 15, clip)
	}
	if c.rejectedCheck != nil {
		return
	}
	c.filmText(frame, 3, fmt.Sprintf("%d/%d ARROWS: PAGE   ENTER/ESC: RETURN", l.NoticePage+1, pages), 16, 176, 15, clip)
}

// An unrelated window/backend exit must wait for the active career writer
// before the outer startup owner releases the saves lease.
func (c *FrontEnd) finishPendingRejectionWrite() {
	if check := c.rejectedCheck; check != nil && check.published != nil {
		result := <-check.published
		c.rejectedCheck = nil
		if result.persistErr != nil {
			diagnostics.Error("career", "Preserving unresolved sortie on shutdown failed", "cause", result.persistErr)
		} else if result.adopt != nil {
			result.adopt()
		}
	}
}
