package menus

import (
	"bytes"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/diagnostics"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPostDebriefDiagnosticLogsWithoutInterruptingSavedFilm(t *testing.T) {
	for _, logging := range []bool{false, true} {
		t.Run(map[bool]string{false: "off", true: "on"}[logging], func(t *testing.T) {
			var log bytes.Buffer
			settings := configuration.DefaultLogging()
			settings.ShowLog, settings.Level = logging, "DEBUG"
			logger := diagnostics.Start(t.TempDir(), settings, diagnostics.Options{Console: &log})
			defer logger.Close()
			c := developmentScenarioController(t, "o-short-campaign")
			before := c.Store.State().SelectedPilot().Statistics.MissionCampaignCounter
			// Named candidate is a supplied inventory-only test file, deliberately
			// not a complete film and never admitted by the playable-film reader.
			candidate := ".film-" + strings.Repeat("d", 32) + ".tmp.sealed"
			if err := os.MkdirAll(c.FilmDirectory, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(c.FilmDirectory, candidate), []byte("supplied diagnostic candidate; no ending"), 0600); err != nil {
				t.Fatal(err)
			}
			proposal := campaignNaturalBaseEnd(t, c)
			ackToLibrary(t, c)
			if c.Library.Notice != "" || len(c.Library.Entries) != 1 || c.Store.State().SelectedPilot().Statistics.MissionCampaignCounter != before+1 {
				t.Fatal("inventory diagnostic interrupted film/result", c.Library.Notice)
			}
			if logging && (!strings.Contains(log.String(), "Retained recording evidence") || !strings.Contains(log.String(), "Explicit strict verification/recovery is required")) {
				t.Fatal("actual diagnostic cause was lost", log.String())
			}
			if !logging && log.Len() != 0 {
				t.Fatal("logging off wrote diagnostics")
			}
			film, err := c.Library.Store.Load(c.Library.Entries[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			reader, err := NewFilmPlayback(c.Office.Assets, film, configuration.Defaults())
			if err != nil {
				t.Fatal(err)
			}
			for !reader.Done {
				if err = reader.Step(); err != nil {
					t.Fatal(err)
				}
			}
			committed := c.Store.State()
			if err = c.commitDebriefResult(); err != nil {
				t.Fatal(err)
			}
			restart, err := gamestate.OpenSaveStore(c.Store.Directory, "")
			if err != nil {
				t.Fatal(err)
			}
			if err = restart.CommitSortie(proposal.Facts.SortieID); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(committed, restart.State()) {
				t.Fatal("duplicate/restart changed committed result")
			}
			if err = c.openFilmLibrary(false, "", ""); err != nil {
				t.Fatal(err)
			}
			if c.Library.Notice != "" || strings.Count(log.String(), "Retained recording evidence") > 1 {
				t.Fatal("unchanged diagnostic repeated")
			}
		})
	}
}

func TestRealRecorderFailureKeepsLogCauseAndCareer(t *testing.T) {
	var log bytes.Buffer
	logger := diagnostics.Start(t.TempDir(), configuration.Logging{ShowLog: true, Level: "DEBUG", FileName: "unused.txt", FileLimitMB: 1}, diagnostics.Options{Console: &log})
	defer logger.Close()
	c := developmentScenarioController(t, "o-short-campaign")
	before := c.Store.State().SelectedPilot().Statistics.MissionCampaignCounter
	if err := c.Rendering.filmRecorder.Spool.Close(); err != nil {
		t.Fatal(err)
	}
	campaignNaturalBaseEnd(t, c)
	ackToLibrary(t, c)
	if c.Library.Notice != "" || c.Store.State().SelectedPilot().Statistics.MissionCampaignCounter != before+1 || !strings.Contains(log.String(), "Mission film not saved") || !strings.Contains(log.String(), "spool closed") {
		t.Fatal("recording-only failure lost cause or valid result", log.String())
	}
}
