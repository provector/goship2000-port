package menus

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"os"
	"reflect"
	"strings"
	"testing"
)

// Ordinary generated data, with a labelled supplied Captain/prior demerit.
// Termination poses below are supplied boundary states, not original witnesses.
func nResultController(t *testing.T, profiles ...string) *FrontEnd {
	t.Helper()
	dir := os.Getenv("GS2000_TEST_MISSIONS")
	if dir == "" {
		t.Skip("prepared-only integration opt in")
	}
	c := missionController(t)
	a, err := assetdata.LoadFrontEndAssets(dir)
	if err != nil {
		t.Fatal(err)
	}
	c.Office.Assets = a
	if len(profiles) != 0 {
		c.simulationProfile = profiles[0]
	}
	saved := c.Store.State()
	p := saved.SelectedPilot()
	p.Rank, p.MissionType, p.Theatre = gamestate.RankCaptain, gamestate.MissionSingle, gamestate.TheatrePersianGulf
	binary.LittleEndian.PutUint16(p.LegacyRaw[0x59:], 300)
	if err = c.Store.Commit(saved); err != nil {
		t.Fatal(err)
	}
	found := false
	for seed := int64(1); seed <= 128; seed++ {
		s, e := prepareMissionSession(a.Missions, c.Store.State().SelectedPilot(), seed, nil, nil)
		if e == nil && s.Data.Objectives["primary"].DisplayKind == 3 && s.Data.Objectives["secondary"].DisplayKind == 3 {
			c.MissionSeed = &seed
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no ordinary destruction-objective fixture")
	}
	readyMission(t, c)
	if len(profiles) != 0 {
		switches, err := gamestate.LaunchSwitches(c.Mission.Data, c.Mission.Settings, a.Missions.Generation.Tables.Initialization.CPGMapping)
		if err != nil {
			t.Fatal(err)
		}
		c.enemyLaunchRules, err = freezePreparedEnemyRules(a.Missions, c.Mission.Data, switches, c.Runtime.DisableEnemyAI)
		if err != nil {
			t.Fatal(err)
		}
	}
	startRendering(t, c)
	if c.Mission.ResultLaunch == nil {
		t.Fatal("result admission failed", c.Mission.ResultUnavailable)
	}
	return c
}

func TestFreezeNaturalReturnVoluntaryAndEndingBarrier(t *testing.T) {
	c := nResultController(t)
	p := c.Rendering
	r := p.Flight
	launch := c.Mission.ResultLaunch
	// Literal captured inputs, with ordinary current-generation targets retained.
	launch.Coefficient = 6
	launch.Accepted = append([]byte(nil), launch.Recommended...)
	r.Combat.Score = 0
	r.ElapsedDA = 128
	p.Initialized.Sortie = &gamestate.SortieState{Status: 0xc0, Pending: "base"}
	if !p.Initialized.AnswerNaturalEnd(true) {
		t.Fatal("natural boundary rejected")
	}
	c.takeAudioCues() // Discard already-consumed preparation events in this headless harness.
	originalTargets := p.ResultTargets.Records
	f, err := c.freezeSortieFacts()
	if err != nil {
		t.Fatal(err)
	}
	if f.Reason != "base" && f.Reason != "carrier" {
		t.Fatal(f.Reason)
	}
	if f.SourceFlags != 0xc0 || f.ElapsedDA != 128 || !reflect.DeepEqual(f.Targets, originalTargets[:]) {
		t.Fatal("frozen launch/opaque target ownership", f)
	}
	for i, e := range p.Initialized.Entities[:80] {
		if f.ObjectValues[i] != int16(binary.LittleEndian.Uint16(e.Object.Raw[27:29])) {
			t.Fatalf("OBJECT decimal27 value row%d", i)
		}
	}
	if err = c.finishSortieResult(); err != nil {
		t.Fatal(err)
	}
	pending, err := c.Store.PendingSortie()
	if err != nil || pending == nil {
		t.Fatal(err)
	}
	if c.Screen != ScreenDebrief || c.Rendering != nil || pending.Proposal.Score.Score != 30 || pending.Proposal.Score.NormalizedFlags != 0x80 {
		t.Fatalf("captured arithmetic/end barrier: screen%s result%+v", c.Screen, pending.Proposal.Score)
	}
	digest := pending.Proposal.FactsDigest
	for i := 0; i < 30; i++ {
		c.Frame(false)
		if err = c.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	again, _ := c.Store.PendingSortie()
	if again.Proposal.FactsDigest != digest || r.Combat.Score != 0 {
		t.Fatal("debrief draw/tick advanced frozen combat")
	}
	requireAudioCues(t, c, 0x1c)
	// The first key reveals text only; it cannot commit the next page.
	act(t, c, Actions{Enter: true})
	if c.Debrief.Page != "jump" {
		t.Fatal("skip consumed wrong page")
	}
	act(t, c, Actions{Enter: true})
	if c.Debrief.Page != "feedback" {
		t.Fatal("missing final crew-chief page")
	}
	if c.Office.Assets.Debrief.Text[0x51c4] != "Press a key to Continue." {
		t.Fatal("original final-page footer not prepared")
	}
	if len(c.CurrentPalette()) != 256 {
		t.Fatal("feedback palette missing")
	}
	rgba, err := c.RenderRGBA(false)
	if err != nil || rgba.Bounds().Dx() != 320 {
		t.Fatal("feedback output", err)
	}
	act(t, c, Actions{Enter: true})
	act(t, c, Actions{Enter: true})
	if c.Screen != ScreenFilms {
		t.Fatal("Step O post-ack library missing", c.Screen)
	}
	if err := c.activateFilmControl(18); err != nil {
		t.Fatal(err)
	}
	if c.Screen != ScreenOffice {
		t.Fatal(c.Screen, c.Message)
	}
	q := c.Store.State().SelectedPilot()
	if q.Statistics.LastMissionScore != 30 || int16(binary.LittleEndian.Uint16(q.LegacyRaw[0x59:])) != 500 {
		t.Fatal("source-witness persisted fields", q.Statistics)
	}
	once := c.Store.State()
	if err = c.RefreshOffice(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(once, c.Store.State()) {
		t.Fatal("office refresh reapplied result")
	}
}

func TestVoluntaryNeverAddsReturnAndDisabledEngineIsNotFinalLoss(t *testing.T) {
	c := nResultController(t)
	p := c.Rendering
	p.Initialized.Sortie = &gamestate.SortieState{Status: 0x80}
	p.Initialized.EndSortieForQoL()
	f, err := c.freezeSortieFacts()
	if err != nil {
		t.Fatal(err)
	}
	if f.SourceFlags&0x80 != 0 || f.Reason != "voluntary" {
		t.Fatal("Alt+E invented safe return", f)
	}
	before := c.Store.State()
	p.Flight.Systems.Damage[9] = 1
	if err = c.finishSortieResult(); err != nil {
		t.Fatal(err)
	}
	if c.Screen != ScreenDebrief || !reflect.DeepEqual(before.SelectedPilot(), c.Store.State().SelectedPilot()) {
		t.Fatal("pre-ack disabled-engine facts mutated career")
	}
	pending, err := c.Store.PendingSortie()
	if err != nil || pending == nil || pending.Proposal.Result == nil {
		t.Fatal("real normalized proposal missing", err)
	}
	if pending.Proposal.Facts.SourceFlags&0x90 != 0 {
		t.Fatal("engine609 fabricated safe return/death")
	}

}

func TestCountdownCurrentCountWrapAndQualifiedExpiry(t *testing.T) {
	// This assertion covers the retained BP-4 guard, not R5's current-row repair.
	c := nResultController(t, replay.R4Compatibility)
	p := c.Rendering
	binary.LittleEndian.PutUint16(p.ResultTargets.Records[3][32:], 3)
	binary.LittleEndian.PutUint16(p.ResultTargets.Records[3][50:], 100)
	word := p.ResultTargets.Records[3][52:54]
	binary.LittleEndian.PutUint16(word, 2)
	phase := func() { p.advanceCombatObjectives(p.combatInput(6, 1, nil)) }
	phase()
	if binary.LittleEndian.Uint16(word) != 1 {
		t.Fatal("pre-expiry current secondary omitted")
	}
	for i := range p.Initialized.Entities {
		p.Initialized.Entities[i].Object.Raw[0] &^= 0x40
	}
	p.Initialized.Entities[70].Object.Raw[0] |= 0x20
	p.Initialized.Entities[70].Kind = 0x15
	phase()
	if p.ResultTargets.ProgressFailure == "" {
		t.Fatal("unresolved BP-4 actor fabricated")
	}
	p.ResultTargets.ProgressFailure = ""
	binary.LittleEndian.PutUint16(word, 0)
	phase()
	if binary.LittleEndian.Uint16(word) != 0xffff {
		t.Fatal("source0 wrap lost")
	}
	binary.LittleEndian.PutUint16(word, 1)
	p.Initialized.Entities[70].Object.Raw[0] = 0x40
	p.Initialized.Entities[70].Kind = 0x1c
	phase()
	if p.ResultTargets.ProgressFailure != "" || binary.LittleEndian.Uint16(word) != 0 {
		t.Fatal("alternate wreck did not retain current goal")
	}
}

func TestDebriefSourceTextBoundariesAndImmutableDraw(t *testing.T) {
	c := nResultController(t)
	p := c.Rendering
	p.Initialized.EndSortieForQoL()
	if err := c.finishSortieResult(); err != nil {
		t.Fatal(err)
	}
	font := c.Menus.Fonts[3]
	if got := debriefWrap(font, "air-to-ground", font.Measure([]byte("air-to-"))); !reflect.DeepEqual(got, []string{"air-to-", "ground"}) {
		t.Fatal("source hyphen split", got)
	}
	if got := debriefWrap(font, "AB", font.Measure([]byte("AB"))); len(got) != 1 {
		t.Fatal("equal width wrongly wraps", got)
	}
	ys, h := debriefBaselines(27, 7)
	if ys[24] != 168 || ys[25] != 164 || ys[26] != 171 || h != 171 {
		t.Fatal("original bank baseline constrain", ys, h)
	}
	frame := c.Frame(false)
	a := c.Frame(false)
	if !bytes.Equal(frame, a) {
		t.Fatal("debrief rendering mutates state")
	}
	for _, page := range []string{"jump", "feedback"} {
		c.enterDebriefPage(page)
		c.Debrief.Tick = 120
		if _, err := c.RenderRGBA(false); err != nil {
			t.Fatal(page, err)
		}
	}
}

func TestSuppliedCommissionAndTrainingControllerComposition(t *testing.T) {
	c := nResultController(t)
	p := c.Rendering
	p.Initialized.Sortie = &gamestate.SortieState{Status: 0x81, Pending: "base"}
	p.Initialized.AnswerNaturalEnd(true)
	facts, err := c.freezeSortieFacts()
	if err != nil {
		t.Fatal(err)
	}
	facts.SourceFlags = 0x81
	facts.RawScore = 250
	facts.ResultContext.DynamicKinds[79] = 0x1c
	facts.ObjectValues[79] = 250
	binary.LittleEndian.PutUint16(facts.FinalObjects[79][27:], 250)
	facts.Coefficient = 10
	facts.LaunchSwitches = 2
	facts.SelectorMultisetMatch = false
	facts.RecommendedSelectors = []byte{byte((int(facts.AcceptedSelectors[0]) + 1) % 8)}
	before := c.Mission.ResultLaunch.Start.PilotBefore
	encoded, _ := json.Marshal(before)
	var supplied gamestate.Pilot
	if err = json.Unmarshal(encoded, &supplied); err != nil {
		t.Fatal(err)
	}
	supplied.Rank = gamestate.RankCW2
	supplied.Statistics.CareerScore = 2850
	binary.LittleEndian.PutUint16(supplied.LegacyRaw[0x59:], 0)
	proposal, err := gamestate.ProposeSortie(&supplied, facts, c.Mission.ResultLaunch.Tables)
	if err != nil {
		t.Fatal(err)
	}
	if !proposal.Commission || proposal.CommanderRank != 6 {
		t.Fatalf("independent supplied commission case %+v", proposal)
	}
	// Renderer-only supplied state: persistence choice/restart tests separately use
	// genuine bound Begin/Stage/Set/Commit APIs, never this diagnostic proposal.
	c.AbandonFilmRecording()
	c.Rendering = nil
	c.Screen = ScreenDebrief
	c.Debrief = &DebriefView{Pending: &gamestate.PendingSortie{Proposal: proposal}, Pilot: proposal.AcceptedPilot, Variant: 1}
	for _, page := range []string{"commander", "jump", "feedback"} {
		c.enterDebriefPage(page)
		c.Debrief.Tick = 120
		if _, err = c.RenderRGBA(false); err != nil {
			t.Fatal(page, err)
		}
	}
	supplied.Rank = gamestate.RankWOC
	supplied.MissionType = gamestate.MissionTraining
	facts.MissionType = gamestate.MissionTraining
	facts.LaunchSwitches = 0x400
	facts.RawScore = 100
	facts.ObjectValues[79] = 100
	binary.LittleEndian.PutUint16(facts.FinalObjects[79][27:], 100)
	facts.SourceFlags = 0x80
	facts.LaunchObjectiveKinds = [2]int16{}
	training, err := gamestate.ProposeSortie(&supplied, facts, c.Mission.ResultLaunch.Tables)
	if err != nil {
		t.Fatal(err)
	}
	if training.CommanderRank != 1 || training.Commission || training.Awards[7] != 1 || training.Score.CareerApplied {
		t.Fatal("Training separate commander/NDS", training)
	}
	c.Debrief = &DebriefView{Pending: &gamestate.PendingSortie{Proposal: training}, Pilot: training.AcceptedPilot}
	for _, page := range []string{"commander", "jump", "feedback"} {
		c.enterDebriefPage(page)
		c.Debrief.Tick = 120
		if _, err = c.RenderRGBA(false); err != nil {
			t.Fatal(page, err)
		}
	}

}

func TestTrainingCommanderAndCommitSkipsCrewChief(t *testing.T) {
	c := missionController(t)
	a, err := assetdata.LoadFrontEndAssets(os.Getenv("GS2000_TEST_MISSIONS"))
	if err != nil {
		t.Fatal(err)
	}
	c.Office.Assets = a
	before := c.Store.State().SelectedPilot()
	if before.MissionType != gamestate.MissionTraining || before.Rank != gamestate.RankWOC {
		t.Fatal("fixture must be new Training WOC")
	}
	start, err := c.Store.BeginSortie(gamestate.SortieStart{SourceProfile: assetdata.IslandIce469085.Name, MissionDigest: strings.Repeat("1", 64), LaunchDigest: strings.Repeat("2", 64), ClockPolicy: flight.NativeClockPolicy})
	if err != nil {
		t.Fatal(err)
	}
	facts := gamestate.SortieFacts{SortieID: start.ID, SourceProfile: start.SourceProfile, MissionType: gamestate.MissionTraining, ActorCount: 1, Reason: "base", SourceFlags: 0x80, RawScore: 100, Coefficient: 6, LaunchSwitches: 0x400, ObjectCount: 80, Targets: make([][62]byte, 5), RecommendedSelectors: []byte{0}, AcceptedSelectors: []byte{0}, SelectorMultisetMatch: true, InactiveHeaderPolicy: gamestate.SortieFreshInactiveHeaders}
	proposal, err := gamestate.ProposeSortie(start.PilotBefore, facts, resultTables(a.Debrief))
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Store.StageSortieResult(start.ID, proposal); err != nil {
		t.Fatal(err)
	}
	store := c.Store
	if err := c.enterSortieDebrief(); err != nil {
		t.Fatal(err)
	}
	resumed := c
	if resumed.Screen != ScreenDebrief || resumed.Debrief.Page != "commander" {
		t.Fatal("current flight skipped source commander")
	}

	if err = resumed.BeginStartup(nil); err != nil {
		t.Fatal(err)
	}
	if resumed.Startup != nil {
		t.Fatal("startup stole pending choice input")
	}
	// First input only reveals text. Warrant advancement has no refusal branch.
	act(t, resumed, Actions{Enter: true})
	act(t, resumed, Actions{Enter: true})
	if resumed.Debrief.Page != "jump" {
		t.Fatal("commander did not lead to JUMP")
	}
	act(t, resumed, Actions{Enter: true})
	act(t, resumed, Actions{Enter: true})
	if resumed.Screen != ScreenFilms || resumed.Debrief != nil {
		t.Fatal("Training must skip HAPPY/SAD and reach the post-ack library")
	}
	if err := resumed.activateFilmControl(18); err != nil {
		t.Fatal(err)
	}
	if resumed.Screen != ScreenOffice {
		t.Fatal("Training library EXIT did not return office", resumed.Screen)
	}
	q := store.State().SelectedPilot()
	if q.Rank != gamestate.RankWO1 || q.MissionType != gamestate.MissionSingle || q.Statistics.AwardCounts[7] != 1 || q.Statistics.MissionCampaignCounter != before.Statistics.MissionCampaignCounter || q.Statistics.CareerScore != before.Statistics.CareerScore {
		t.Fatal("Training commander/NDS versus protected career", q)
	}
}

func TestRetiredProgressionUsesOriginalRosterLabel(t *testing.T) {
	if got := rosterStatusLabel(&gamestate.Pilot{Status: gamestate.StatusRetired, LegacyStatus: 2}); got != "Retired" {
		t.Fatal(got)
	}
}

func TestUnfinishedEntryRestartsAtOfficeWithoutAward(t *testing.T) {
	c := nResultController(t)
	before := c.Mission.ResultLaunch.Start.PilotBefore
	store, err := gamestate.OpenSaveStore(c.Store.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := NewFrontEnd(c.Office.Assets, store)
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Screen != ScreenOffice || restarted.Debrief != nil {
		t.Fatal("forced interrupted-session UI")
	}
	if !reflect.DeepEqual(before, store.State().SelectedPilot()) {
		t.Fatal("unfinished sortie changed pilot")
	}
	if _, err = restarted.RenderRGBA(false); err != nil {
		t.Fatal(err)
	}
	if err := c.Store.CommitSortie(c.Mission.ResultLaunch.Start.ID); err == nil {
		t.Fatal("stale result callback awarded")
	}
	if backup, err := store.RecoveryCandidate(); err != nil || backup != nil {
		t.Fatal("launch created Phoenix", err)
	}
}

func TestFinalFactsIndependentOfPresentationSchedule(t *testing.T) {
	var reference *gamestate.SortieFacts
	for _, tc := range []struct {
		mode       string
		scale, fps int
	}{{"smooth", 1, 30}, {"smooth", 2, 144}, {"stepped", 1, 60}, {"stepped", 2, 30}} {
		t.Run(fmt.Sprintf("%s/%d/%d", tc.mode, tc.scale, tc.fps), func(t *testing.T) {
			c := nResultController(t)
			p := c.Rendering
			seed := uint32(1) // Identical supplied front-end seed isolates presentation from native sortie identity.
			c.Mission.ResultLaunch.DevelopmentSeed = &seed
			p.Scene.RenderScale = tc.scale
			p.presentation.mode = tc.mode
			p.resetPresentation()
			for frame := 0; frame < tc.fps/2; frame++ {
				if err := c.AdvanceRendering(1 / float64(tc.fps)); err != nil {
					t.Fatal(err)
				}
				c.Frame(false)
			}
			p.Initialized.EndSortieForQoL()
			f, err := c.freezeSortieFacts()
			if err != nil {
				t.Fatal(err)
			}
			f.SortieID = "" // Native transaction identity intentionally uses independent crypto/rand.
			if reference == nil {
				reference = &f
			} else if !reflect.DeepEqual(*reference, f) {
				t.Fatalf("authoritative final snapshot changed with display schedule: want%+v got%+v", *reference, f)
			}
		})
	}
}

func TestUnknownImportedCareerKeepsPreviouslySupportedFlight(t *testing.T) {
	c := renderingController(t)
	startRendering(t, c) // Minimal legacy controller deliberately has no debrief admission.
	full, err := assetdata.LoadFrontEndAssets(os.Getenv("GS2000_TEST_MISSIONS"))
	if err != nil {
		t.Fatal(err)
	}
	c.Office.Assets.Debrief = full.Debrief
	state := c.Store.State()
	state.SelectedPilot().Statistics = nil
	if err = c.Store.Commit(state); err != nil {
		t.Fatal(err)
	}
	rendering := c.Rendering
	if err = c.beginSortieResult(c.Mission.Final); err != nil {
		t.Fatal("unknown career blocked flight", err)
	}
	if c.Mission.ResultLaunch != nil || c.Mission.ResultUnavailable == "" || c.Rendering != rendering || c.Screen != ScreenScene {
		t.Fatal("incomplete pilot manufactured result or lost flight")
	}
	if pending, err := c.Store.PendingSortie(); err != nil || pending != nil {
		t.Fatal("incomplete pilot acquired journal", pending, err)
	}
}
