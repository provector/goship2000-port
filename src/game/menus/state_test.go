package menus

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// This is the device-to-controller route used by renderer_window, not act(),
// which intentionally resets pointer anchors for older semantic-only tests.
func stateWindowInput(t *testing.T, c *FrontEnd, input *windowInput, b Buttons) Actions {
	t.Helper()
	v := officeViewport(1280, 720, false)
	px := v.X + (float64(b.X)+.5)*v.Width/320
	py := v.Y + (float64(b.Y)+.5)*v.Height/200
	x, y, inside := v.LogicalPoint(px, py)
	b.X, b.Y, b.Inside = x, y, b.Inside && inside
	a := input.ReadViewport(b, v)
	if err := c.Handle(a); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestMissionInputOwnershipThroughWindowWrapper(t *testing.T) {
	c := sourceFrontEnd(t, true)
	if err := c.openMission(); err != nil {
		t.Fatal(err)
	}
	input := &windowInput{}
	b := Buttons{X: 100, Y: 82, Inside: true}
	stateWindowInput(t, c, input, b)
	b.Down = true
	stateWindowInput(t, c, input, b)
	if c.Highlight != 1 {
		t.Fatal("arrow did not visibly highlight Single")
	}
	stateWindowInput(t, c, input, b)
	if c.Highlight != 1 {
		t.Fatal("held arrow navigated again")
	}
	b.Down = false
	stateWindowInput(t, c, input, b)
	b.Space = true
	stateWindowInput(t, c, input, b)
	if c.Screen != ScreenOffice || c.Store.State().SelectedPilot().MissionType != gamestate.MissionSingle {
		t.Fatal("stationary pointer stole keyboard highlight", c.Screen)
	}
	revision := c.Store.State().Revision
	a := stateWindowInput(t, c, input, b)
	if a.Selector1 || c.Store.State().Revision != revision {
		t.Fatal("held selector crossed the transition")
	}
	b.Space = false
	stateWindowInput(t, c, input, b)
	if err := c.openMission(); err != nil {
		t.Fatal(err)
	}
	stateWindowInput(t, c, input, b)
	b.Up = true
	stateWindowInput(t, c, input, b)
	b.Up = false
	stateWindowInput(t, c, input, b)
	// A genuine mouse press takes ownership, even if keyboard navigation last
	// highlighted a different row. Relative delta enters Single's row.
	b.Y, b.LeftMouse = 92, true
	stateWindowInput(t, c, input, b)
	if c.Screen != ScreenOffice || c.Store.State().SelectedPilot().MissionType != gamestate.MissionSingle {
		t.Fatal("mouse click did not select the actual hit")
	}
}

func TestTheatreBlankAndKeyboardOwnership(t *testing.T) {
	c := sourceFrontEnd(t, true)
	c.Screen, c.Highlight = ScreenTheatre, 0
	input := &windowInput{}
	before := c.Store.State()
	b := Buttons{X: 10, Y: 10, Inside: true, LeftMouse: true}
	stateWindowInput(t, c, input, b)
	if c.Screen != ScreenTheatre || !reflect.DeepEqual(before, c.Store.State()) {
		t.Fatal("blank theatre click committed previous highlight")
	}
	b.Inside, b.LeftMouse = false, false
	stateWindowInput(t, c, input, b)
	b.LeftMouse = true
	stateWindowInput(t, c, input, b)
	if c.Screen != ScreenTheatre || !reflect.DeepEqual(before, c.Store.State()) {
		t.Fatal("outside viewport click committed")
	}
	b.Inside, b.LeftMouse, b.Down = true, false, true
	stateWindowInput(t, c, input, b)
	b.Down = false
	stateWindowInput(t, c, input, b)
	b.Space = true
	stateWindowInput(t, c, input, b)
	if c.Screen != ScreenOffice || c.Store.State().SelectedPilot().Theatre != gamestate.TheatrePersianGulf {
		t.Fatal("keyboard selection did not commit visible theatre")
	}
}

func TestWindowWrapperResetsOnlyOnViewportOrFocusChange(t *testing.T) {
	w := &windowInput{}
	v := officeViewport(960, 720, false)
	b := Buttons{X: 100, Y: 82, Inside: true}
	if !w.ReadViewport(b, v).PointerReset || w.ReadViewport(b, v).PointerReset {
		t.Fatal("stationary input continually reset pointer anchors")
	}
	if !w.ReadViewport(b, officeViewport(1280, 720, false)).PointerReset {
		t.Fatal("resize did not reset anchor")
	}
	b.Inside = false
	if !w.ReadViewport(b, v).PointerReset {
		t.Fatal("focus loss did not reset anchor")
	}
}

func TestRosterLabelsAndConservativePosting(t *testing.T) {
	// Independent bounded native table DS0c26, reached through DS1a94 by A244ff.
	for raw, want := range []string{"Replacement", "Active", "Retired", "MIA", "KIA"} {
		p := &gamestate.Pilot{Status: gamestate.StatusLegacyUnknown, LegacyStatus: byte(raw)}
		if raw == 0 {
			p.Status = gamestate.StatusInactive
		} else if raw == 1 {
			p.Status = gamestate.StatusActive
		}
		if got := rosterStatusLabel(p); got != want {
			t.Fatalf("native status%d label %q want %q", raw, got, want)
		}
	}
	if got := rosterStatusLabel(&gamestate.Pilot{Status: gamestate.StatusLegacyUnknown, LegacyStatus: 255}); got != "Unknown" {
		t.Fatal("invented label outside original bounded table", got)
	}
	c := sourceFrontEnd(t, true)
	next := c.Store.State()
	next.SelectedPilot().Status = gamestate.StatusInactive
	if err := c.Store.Commit(next); err != nil {
		t.Fatal(err)
	}
	c.Screen, c.Highlight = ScreenRoster, 0
	before := c.Store.State()
	if err := c.Handle(Actions{Selector1: true}); err != nil {
		t.Fatal(err)
	}
	// N.5 admits inspection/Phoenix independently of POST. The duty identity
	// and canonical unavailable status must remain unchanged by that inspection.
	if c.Screen != ScreenOffice || c.ViewedPilotID != before.SelectedPilotID || !reflect.DeepEqual(before, c.Store.State()) {
		t.Fatal("inspection changed posting eligibility or canonical pilot")
	}
}

func TestTypedCampaignHeadingSurvivesResume(t *testing.T) {
	c := missionController(t)
	setPreflightPilot(t, c, gamestate.MissionCampaign)
	readyMission(t, c)
	if err := c.campaignResult("suspend"); err != nil {
		t.Fatal(err)
	}
	for _, restart := range []bool{false, true} {
		if restart {
			var err error
			c.Store, err = gamestate.OpenSaveStore(c.Store.Directory, "")
			if err != nil {
				t.Fatal(err)
			}
			if err = c.RefreshOffice(); err != nil {
				t.Fatal(err)
			}
		}
		p := c.Store.State().SelectedPilot()
		if p.LegacyRaw[0x4f] != 0 || c.rosterCampaignLine(p) != "Campaign underway" {
			t.Fatal("typed suspended Campaign did not override raw witness")
		}
		p.Campaign.Phase = gamestate.CampaignActive
		p.Campaign.Suspended = nil
		if c.rosterCampaignLine(p) != "Campaign underway" {
			t.Fatal("typed active Campaign missing heading")
		}
		p.Statistics = nil
		if c.rosterCampaignLine(p) != "Campaign underway" {
			t.Fatal("unavailable statistics hid known Campaign state")
		}
	}
	readyMission(t, c)
	if c.Mission.CampaignDraft == nil {
		t.Fatal("resume lost Campaign")
	}
}

func TestCampaignDeclineGenerationFailureRetainsCurrentState(t *testing.T) {
	c := missionController(t)
	setPreflightPilot(t, c, gamestate.MissionCampaign)
	readyMission(t, c)
	c.Screen = ScreenPapers
	c.Mission.Page = 3
	s := c.Mission
	before := c.Store.State()
	disk := append([]byte(nil), mustReadSaveBytes(t, filepath.Join(c.Store.Directory, "roster.json"))...)
	objects := s.Data.ObjectHash
	// Explicit invalid bounded draw is a deterministic preparation failure,
	// not a claim this is a valid native RNG stream or Campaign history.
	c.MissionDrawStream = []int{-1}
	if err := c.campaignResult("decline"); err == nil {
		t.Fatal("invalid draw stream unexpectedly prepared")
	}
	if c.Mission != s || c.Screen != ScreenPapers || c.Mission.Page != 3 || objects != c.Mission.Data.ObjectHash || !reflect.DeepEqual(before, c.Store.State()) || !bytes.Equal(disk, mustReadSaveBytes(t, filepath.Join(c.Store.Directory, "roster.json"))) {
		t.Fatal("failed candidate changed current mission or committed state")
	}
	if c.missionSaveError != nil || !strings.Contains(c.Message, "Nothing saved") {
		t.Fatal("generation failure incorrectly blocked safe retry", c.Message)
	}
	c.MissionDrawStream = nil
	if err := c.campaignResult("decline"); err != nil {
		t.Fatal("safe retry failed", err)
	}
	if c.Mission == s || c.Screen != ScreenInterstitial || c.Store.State().Revision != before.Revision+1 {
		t.Fatal("successful decline did not commit/publish exactly once")
	}
	if backup, err := c.Store.RecoveryCandidate(); err != nil || backup != nil {
		t.Fatal("decline manufactured recovery backup", err)
	}
}

func TestCampaignSaveFailureClosesActionablePreparation(t *testing.T) {
	for _, result := range []string{"decline", "suspend", "abort"} {
		t.Run(result, func(t *testing.T) {
			c := missionController(t)
			setPreflightPilot(t, c, gamestate.MissionCampaign)
			readyMission(t, c)
			before := c.Store.State()
			disk := append([]byte(nil), mustReadSaveBytes(t, filepath.Join(c.Store.Directory, "roster.json"))...)
			// Block a real journal publish in this disposable save directory.
			if err := os.Mkdir(filepath.Join(c.Store.Directory, ".pending-commit.json"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := c.campaignResult(result); err == nil {
				t.Fatal("save unexpectedly succeeded")
			}
			if c.Mission != nil || c.Screen != ScreenOffice || c.missionSaveError == nil || !reflect.DeepEqual(before, c.Store.State()) || !bytes.Equal(disk, mustReadSaveBytes(t, filepath.Join(c.Store.Directory, "roster.json"))) {
				t.Fatal("save failure published a session or changed acknowledged state")
			}
			if err := c.Handle(Actions{X: 285, Y: 75, Inside: true, Selector1: true, PointerPressed: true}); err != nil || c.Mission != nil {
				t.Fatal("blocked save failure reopened preparation", err)
			}
			if err := c.Handle(Actions{Quit: true}); err != nil || c.Screen != ScreenQuit {
				t.Fatal("blocked preparation cannot quit", err)
			}
			if err := c.Handle(Actions{Text: []rune{'N'}}); err != nil || c.Screen != ScreenOffice || c.missionSaveError == nil {
				t.Fatal("quit cancellation removed failure lock", err)
			}
		})
	}
}

func TestCampaignPendingCommitRequiresRestart(t *testing.T) {
	c := missionController(t)
	setPreflightPilot(t, c, gamestate.MissionCampaign)
	readyMission(t, c)
	next := c.Store.State()
	if err := campaignDecline(next.SelectedPilot(), c.Mission); err != nil {
		t.Fatal(err)
	}
	next.Revision++
	files, err := stateFilesForTest(next)
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.ToSlash(filepath.Join("pilots", next.SelectedPilotID+".json"))
	// Synthesize a valid interrupted journal, not filesystem crash durability.
	journal := testSaveCommit{SchemaVersion: 1, Operations: []testSaveOperation{{Path: name, Before: assetdata.Hash(mustReadSaveBytes(t, filepath.Join(c.Store.Directory, filepath.FromSlash(name)))), After: files[name]}, {Path: "roster.json", Before: assetdata.Hash(mustReadSaveBytes(t, filepath.Join(c.Store.Directory, "roster.json"))), After: files["roster.json"]}}}
	raw, _ := json.Marshal(journal)
	if err := os.WriteFile(filepath.Join(c.Store.Directory, ".pending-commit.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := c.campaignResult("decline"); err == nil || c.Mission != nil || c.missionSaveError == nil {
		t.Fatal("pending journal left an actionable mission", err)
	}
	reopened, err := gamestate.OpenSaveStore(c.Store.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if reopened.State().Revision != next.Revision || reopened.State().SelectedPilot().Campaign == nil {
		t.Fatal("restart did not reconcile journal")
	}
}

func TestKnownCampaignBoundariesDoNotCommitDecline(t *testing.T) {
	for _, tc := range []struct {
		theatre  gamestate.Theatre
		stop     int
		closedPH bool
	}{{gamestate.TheatrePhilippines, 4, true}, {gamestate.TheatreCentralEurope, 8, false}} {
		t.Run(string(tc.theatre), func(t *testing.T) {
			c := missionController(t)
			// Retain the old byte-layout/hash boundary; R5 tests paired metadata.
			c.simulationProfile = replay.R4Compatibility
			setPreflightPilot(t, c, gamestate.MissionCampaign)
			next := c.Store.State()
			next.SelectedPilot().Theatre = tc.theatre
			if err := c.Store.Commit(next); err != nil {
				t.Fatal(err)
			}
			if err := c.RefreshOffice(); err != nil {
				t.Fatal(err)
			}
			readyMission(t, c)
			for seed := 2; seed <= tc.stop; seed++ {
				value := int64(seed)
				c.MissionSeed = &value
				old := c.Mission
				state := c.Store.State()
				files, err := stateFilesForTest(state)
				if err != nil {
					t.Fatal(err)
				}
				err = c.campaignResult("decline")
				if seed < tc.stop {
					if err != nil || c.Mission == old {
						t.Fatalf("supported preceding preparation%d: %v", seed, err)
					}
					continue
				}
				if tc.closedPH {
					// Step38 independently reached this exact history/hash with
					// opaque PHIL3 row23 provenance154. Actual FMHD consumers
					// remain guarded; the previous eager-append error is obsolete.
					if err != nil || c.Mission == old || c.Mission.Data.ObjectHash != "6d9aaf1a33699014844d2d921571ae5bf54422e720667dc1f2c02bee9319715f" || c.Mission.Data.Objects[23].Formation != 154 || len(c.Mission.Data.SourceWorld.FMHD)/39 != 154 {
						t.Fatal("Step38 closed PH preparation4 did not retain its independent source history", err)
					}
					after := c.Store.State()
					if after.Revision != state.Revision+1 || after.SelectedPilot().Campaign.ResultCounters[1] != old.CampaignDraft.ResultCounters[1]+1 || !reflect.DeepEqual(after.SelectedPilot().Statistics, state.SelectedPilot().Statistics) {
						t.Fatal("closed continuation did not publish precisely one decline, or invented sortie results")
					}
					priorPenalty := int16(binary.LittleEndian.Uint16(state.SelectedPilot().LegacyRaw[0x59:]))
					if got := int16(binary.LittleEndian.Uint16(after.SelectedPilot().LegacyRaw[0x59:])); got != priorPenalty+300 {
						t.Fatal("source decline penalty changed while removing eager provenance guard", got)
					}
					if c.missionSaveError != nil {
						t.Fatal(c.missionSaveError)
					}
					continue
				}
				if err == nil || c.Mission != old || !reflect.DeepEqual(state, c.Store.State()) {
					t.Fatal("CAM-01 boundary published changed history", err)
				}
				for name, raw := range files {
					if !bytes.Equal(raw, mustReadSaveBytes(t, filepath.Join(c.Store.Directory, filepath.FromSlash(name)))) {
						t.Fatal("failed continuation changed disk", name)
					}
				}
				if c.missionSaveError != nil || !strings.Contains(c.Message, "Nothing saved") {
					t.Fatal("supported error boundary ownership", c.Message)
				}
				t.Logf("CAM-01 preparation%d seed%d remains a safe error: %v", seed, seed, err)
			}
		})
	}
}

func TestInvalidCampaignSaveDoorPathIsNonDestructive(t *testing.T) {
	c := missionController(t)
	setPreflightPilot(t, c, gamestate.MissionCampaign)
	readyMission(t, c)
	next := c.Store.State()
	p := next.SelectedPilot()
	var err error
	p.Campaign, err = gamestate.CloneCampaign(c.Mission.CampaignDraft)
	if err != nil {
		t.Fatal(err)
	}
	p.Campaign.Retained.Provenance[0] = 255
	if err = c.Store.Commit(next); err != nil {
		t.Fatal(err)
	}
	c.Store, err = gamestate.OpenSaveStore(c.Store.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if err = c.RefreshOffice(); err != nil {
		t.Fatal(err)
	}
	state := c.Store.State()
	files, _ := stateFilesForTest(state)
	if err = c.startMission(18, nil); err == nil || c.Mission != nil || c.Screen != ScreenOffice {
		t.Fatal("GEN-02 malformed editable state reached mission", err)
	}
	if !reflect.DeepEqual(state, c.Store.State()) {
		t.Fatal("rejected Campaign changed backend")
	}
	for name, raw := range files {
		if !bytes.Equal(raw, mustReadSaveBytes(t, filepath.Join(c.Store.Directory, filepath.FromSlash(name)))) {
			t.Fatal("rejected Campaign changed disk", name)
		}
	}
}
