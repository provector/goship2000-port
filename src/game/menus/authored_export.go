package menus

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	gamestate "gunship2000-assets/game/state"
)

// Export is an authored configuration conversion. It neither adopts playback
// state nor acquires its parent's career/successor capabilities.
func (c *FrontEnd) exportFilmToMission() error {
	l := c.Library
	if l == nil || l.Selected < 0 || l.Selected >= len(l.Entries) {
		return fmt.Errorf("select a film to Export")
	}
	entry := l.Entries[l.Selected]
	f, e := l.Store.LoadEntry(entry)
	if e != nil {
		return e
	}
	var launch filmLaunch
	d := json.NewDecoder(bytes.NewReader(f.Launch))
	d.DisallowUnknownFields()
	if e = d.Decode(&launch); e != nil {
		return fmt.Errorf("film launch is unavailable: %w", e)
	}
	m := launch.Final.Mission
	if len(launch.GeneratedSource) > 0 {
		compatible, err := filmMatchesInstalledResources(f, c.Office.Assets)
		if err != nil {
			return err
		}
		if !compatible {
			return fmt.Errorf("the selected film needs a different resource profile")
		}
		var groups gamestate.GenerationIndex
		if err := json.Unmarshal(launch.GeneratedSource, &groups); err != nil {
			return fmt.Errorf("invalid generated placement description: %w", err)
		}
		if m == nil {
			return fmt.Errorf("film initial mission unavailable")
		}
		if err := gamestate.PrepareMissionTargets(c.Office.Assets.Missions, m); err != nil {
			return err
		}
		doc, err := gamestate.NewGeneratedDocument(c.editorData(), &groups, m, entry.Title)
		if err != nil {
			return err
		}
		doc.Aircraft = append([][36]byte(nil), launch.Final.Aircraft...)
		doc.Compiled.Revision = gamestate.AuthoredRevision(doc)
		return c.publishExport(doc)
	}
	if m == nil || (m.Inputs.MissionType != gamestate.MissionFlight && m.Inputs.MissionType != gamestate.MissionSimulated) {
		return fmt.Errorf("this historical film lacks the initial formation and route descriptions required for editing")
	}
	if (len(launch.Final.Aircraft) != 5 || len(launch.Final.Flight) != 5) && !(launch.AuthoredSource != nil && m.SoloBuilder && len(launch.Final.Aircraft) == 1 && len(launch.Final.Flight) == 1) || launch.Adjustment != nil {
		return fmt.Errorf("Export requires five actual aircraft/crew and a represented clean launch")
	}
	// Validate exact resources, operation framing and the pristine initial owner.
	// Film End/world damage/GPS changes are never consulted for conversion.
	var q *FilmPlayback
	if f.Legacy() {
		q, e = loadSelectedFilmPlayback(c.Office.Assets, l.Store, entry, c.Runtime)
	} else {
		q, e = NewFilmPlayback(c.Office.Assets, f, c.Runtime)
	}
	if e != nil {
		return e
	}
	initial := q.Preview()
	if initial == nil || initial.Initialized == nil || initial.ResultTargets == nil {
		return fmt.Errorf("film source TARGET initialization is unavailable")
	}
	copy, e := cloneFilmFinal(&launch.Final)
	if e != nil {
		return e
	}
	copy.Mission.Targets = gamestate.CloneMissionTargets(initial.ResultTargets)
	if f.SafeGameplayMechanics() {
		// R5's completed-goal lineage belongs to the live sortie/result, not
		// the authored preparation compared with its saved source document.
		// Project only the owned reconstruction; keep film/viewer untouched.
		copy.Mission.Targets.ObjectiveRevisions = nil
	}
	var doc *gamestate.AuthoredDocument
	if launch.AuthoredSource != nil {
		doc, e = gamestate.NewAuthoredFromFilmSource(c.Office.Assets.Missions.Generation, launch.AuthoredSource, copy.Mission, entry.Title)
		if errors.Is(e, gamestate.ErrAuthoredSourceReadOnly) {
			doc, e = gamestate.NewAuthoredFromMission(c.Office.Assets.Missions.Generation, copy.Mission, entry.Title, uint32(copy.Mission.Inputs.Seed))
		}
	} else {
		doc, e = gamestate.NewAuthoredFromMission(c.Office.Assets.Missions.Generation, copy.Mission, entry.Title, uint32(copy.Mission.Inputs.Seed))
	}
	if e != nil {
		return e
	}
	doc.Aircraft = append([][36]byte(nil), copy.Aircraft...)
	doc.Compiled.Revision = gamestate.AuthoredRevision(doc)
	return c.publishExport(doc)
}
func (c *FrontEnd) publishExport(doc *gamestate.AuthoredDocument) error {
	if _, e := (gamestate.AuthoredStore{Directory: c.MissionDirectory}).SaveExport(doc); e != nil {
		return e
	}
	c.Library.Status = "Exported " + doc.Title + " to Mission Files."
	if doc.Initial != nil {
		c.Library.Status += " Historical layout: conditions editable; group provenance unavailable."
	}
	c.emitAudioCue(gs4Accepted)
	return nil
}
