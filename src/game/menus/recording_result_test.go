package menus

import (
	"bytes"
	"encoding/json"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/replay"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestUnadmittedPreviewRetainsOriginalBriefing(t *testing.T) {
	c := renderingController(t)
	mission := c.Mission
	before := c.Store.State()
	startRendering(t, c)
	jFinishFade(t, c)
	if c.Mission.ResultLaunch != nil {
		t.Fatal("fixture unexpectedly admits a career journal")
	}
	if err := c.Handle(Actions{EndMission: true}); err != nil {
		t.Fatal(err)
	}
	if err := c.Handle(Actions{ConfirmQuit: true}); err != nil {
		t.Fatal(err)
	}
	for c.rejectedCheck != nil {
		if err := c.AdvanceReplay(.016); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen != ScreenBriefing || c.Mission != mission || !reflect.DeepEqual(before, c.Store.State()) {
		t.Fatal("unadmitted preview changed its original return or career", c.Screen)
	}
	if err := c.finishSortieResult(); err != nil {
		t.Fatal(err)
	}
	if c.Screen != ScreenBriefing || !reflect.DeepEqual(before, c.Store.State()) {
		t.Fatal("delayed preview ending acquired career authority")
	}
}

func TestCareerRejectionRetainsCompleteStrictFilmAndBriefing(t *testing.T) {
	c := developmentScenarioController(t, "o-short-campaign")
	jFinishFade(t, c)
	before, _ := json.Marshal(c.Store.State().SelectedPilot())
	mission := c.Mission
	sessionID := mission.ResultLaunch.Start.ID
	// Labelled career-only fault injection: the actual flight/recording inputs,
	// checkpoint and ending remain untouched. This is not the private incident.
	c.Mission.ResultLaunch.Tables.SourceProfile = "diagnostic unknown progression table"
	if e := c.Handle(Actions{Cancel: true}); e != nil {
		t.Fatal(e)
	}
	if e := c.AdvanceRendering(.12); e != nil {
		t.Fatal(e)
	}
	if e := c.Handle(Actions{EndMission: true}); e != nil {
		t.Fatal(e)
	}
	if e := c.Handle(Actions{ConfirmQuit: true}); e != nil {
		t.Fatal(e)
	}
	if c.rejectedCheck == nil || len(c.Library.Entries) != 0 {
		t.Fatal("published before strict verification")
	}
	if err := c.Handle(Actions{Selector1: true, Cancel: true}); err != nil || c.rejectedCheck == nil {
		t.Fatal("input escaped verification", err)
	}
	for c.rejectedCheck != nil {
		if err := c.AdvanceReplay(.016); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen != ScreenFilms || c.Library == nil || len(c.Library.Entries) != 1 || !c.Library.Entries[0].Unscored || c.Library.Notice == "" {
		t.Fatal("rejection discarded completefilm", c.Screen, c.Message, c.Library)
	}
	after, _ := json.Marshal(c.Store.State().SelectedPilot())
	if !bytes.Equal(before, after) {
		t.Fatal("rejected career changed pilot")
	}
	if p, e := c.Store.PendingSortie(); e != nil || p != nil {
		t.Fatal("rejected result remains pending", p, e)
	}
	id := c.Library.Entries[0].ID
	saved, err := os.ReadFile(filepath.Join(c.Store.Directory, "unresolved-results", sessionID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		Reason, FilmID string
		Launch         *sortieLaunch
		Systems        json.RawMessage
		Mission        json.RawMessage
	}
	if err = json.Unmarshal(saved, &receipt); err != nil || receipt.Launch == nil || receipt.Launch.Start.ID != sessionID || receipt.FilmID != id || receipt.Reason == "" || len(receipt.Mission) == 0 || len(receipt.Systems) == 0 {
		t.Fatal("missing unresolved result evidence", err)
	}

	f, e := c.Library.Store.Load(id)
	if e != nil {
		t.Fatal(e)
	}
	if f.End.Score != 0 || f.End.Summary == nil || f.End.Summary.Score != 0 {
		t.Fatal("unknowncareerrescored film")
	}
	raw, e := replay.Marshal(f)
	if e != nil {
		t.Fatal(e)
	}
	sealed, e := c.SealRecordedFilm()
	if e != nil {
		t.Fatal(e)
	}
	original, _ := replay.Marshal(sealed)
	if !bytes.Equal(raw, original) {
		t.Fatal("sealedbyteschanged")
	}
	q, e := NewFilmPlayback(c.Office.Assets, f, configuration.Defaults())
	if e != nil {
		t.Fatal(e)
	}
	for !q.Done {
		if e = q.Step(); e != nil {
			t.Fatal(e)
		}
	}
	publishedPath := publishedFilmPath(t, c, id)
	beforeAck, err := os.ReadFile(publishedPath)
	if err != nil {
		t.Fatal(err)
	}
	if e = c.Handle(Actions{Enter: true}); e != nil {
		t.Fatal(e)
	}
	if c.Library.Notice != "" {
		t.Fatal("notice didnotdismiss")
	}
	if e = c.finishSortieResult(); e != nil {
		t.Fatal(e)
	}
	if e = c.commitDebriefResult(); e != nil {
		t.Fatal(e)
	}
	if e = c.exitFilmLibrary(); e != nil {
		t.Fatal(e)
	}
	if c.Screen != ScreenBriefing || c.Mission != mission {
		t.Fatal("rejection lost offered preparation")
	}
	disk, e := os.ReadFile(publishedPath)
	if e != nil || !bytes.Equal(beforeAck, disk) {
		t.Fatal("repeatackmodifiedfilm", e)
	}
	rows, e := (replay.Store{Root: c.FilmDirectory}).List()
	if e != nil || len(rows) != 1 || !rows[0].Unscored {
		t.Fatal("restartunscored admission", rows, e)
	}
	after, _ = json.Marshal(c.Store.State().SelectedPilot())
	if !bytes.Equal(before, after) {
		t.Fatal("repeatackawarded career")
	}
}

func TestClosingUnscoredVerificationKeepsSealedEvidence(t *testing.T) {
	c := developmentScenarioController(t, "o-short-campaign")
	jFinishFade(t, c)
	before, err := json.Marshal(c.Store.State().SelectedPilot())
	if err != nil {
		t.Fatal(err)
	}
	id := c.Mission.ResultLaunch.Start.ID
	c.Mission.ResultLaunch.Tables.SourceProfile = "unavailable progression table"
	for _, action := range []Actions{{Cancel: true}, {EndMission: true}, {ConfirmQuit: true}} {
		if err := c.Handle(action); err != nil {
			t.Fatal(err)
		}
	}
	if c.rejectedCheck == nil {
		t.Fatal("missing verification")
	}
	if err := c.Handle(Actions{Close: true}); err != nil {
		t.Fatal(err)
	}
	if c.rejectedCheck != nil || c.Screen != ScreenQuit {
		t.Fatal("close did not return to the quit prompt", c.Screen)
	}
	if _, err := os.Stat(filepath.Join(c.Store.Directory, "unresolved-results", id+".json")); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(c.FilmDirectory, "*.sealed"))
	if err != nil || len(files) != 1 {
		t.Fatal("lost private sealed recording", files, err)
	}
	entries, err := (replay.Store{Root: c.FilmDirectory}).List()
	if err != nil || len(entries) != 0 {
		t.Fatal("unverified recording published", entries, err)
	}
	after, err := json.Marshal(c.Store.State().SelectedPilot())
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("close changed career", err)
	}
}
