package menus

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"math/rand"
	"os"
	"reflect"
	"strings"
	"testing"
)

// Genuine new generation/preflight with actual follower cargo accepted through
// ORDNANCE. Later poses/controller2 are labelled supplied approaches, not flight.
func logisticsController(t *testing.T, seed int64, aircraft, loadActor int, profiles ...string) *FrontEnd {
	t.Helper()
	c := missionController(t)
	a, err := assetdata.LoadFrontEndAssets(os.Getenv("GS2000_TEST_MISSIONS"))
	if err != nil {
		t.Fatal(err)
	}
	c.Office.Assets, c.Menus = a, a.Menus
	if len(profiles) != 0 {
		c.simulationProfile = profiles[0]
	}
	next := c.Store.State()
	p := next.SelectedPilot()
	p.Rank, p.MissionType, p.Theatre = gamestate.RankCaptain, gamestate.MissionFlight, gamestate.TheatrePersianGulf
	p.SquadronPilots, err = gamestate.InitializeSquadron(p, next.Slots[:], a.Missions.Preflight.SquadronNames, rand.New(rand.NewSource(42)).Intn)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Store.Commit(next); err != nil {
		t.Fatal(err)
	}
	if err = c.RefreshOffice(); err != nil {
		t.Fatal(err)
	}
	c.MissionSeed = &seed
	readyMission(t, c)
	choose := func(action string, value int) {
		t.Helper()
		for i, r := range c.Mission.Ordnance.Choices {
			if r.Enabled && r.Action == action && (value < 0 || r.Value == value) {
				missionClick(t, c, i)
				return
			}
		}
		t.Fatalf("missing actual ordnance%s/%d", action, value)
	}
	for actor := 0; actor < 5; actor++ {
		if err = c.openOrdnance(actor); err != nil {
			t.Fatal(err)
		}
		choose("aircraft", -1)
		choose("replace", aircraft)
		choose("back", -1)
		if actor == 0 && ordnanceAcceptance(c.Mission.Ordnance.Working[:], 0) == "HELICOPTER IS UNARMED.  Add weapons." {
			choose("fits", -1)
			choose("mount", -1)
			choose("back", -1)
		}
		if actor == loadActor {
			found := false
			for i, r := range c.Mission.Ordnance.Choices {
				w := a.Missions.WeaponDefinitions[r.System]
				if r.Enabled && r.Action == "remove" && len(w) == 214 && gamestate.RowWord(w, 4)&0x4100 == 0x4100 {
					found = true
					break
				}
				if r.Enabled && r.Action == "mount" && len(w) == 214 && gamestate.RowWord(w, 4)&0x4100 == 0x4100 {
					missionClick(t, c, i)
					found = true
					break
				}
			}
			if !found {
				t.Fatal("actual follower cargo mount absent", c.Mission.Ordnance.Choices, c.Mission.Ordnance.Working)
			}
		}
		choose("back", -1)
	}
	if c.Screen == ScreenOutfit {
		missionClick(t, c, 6)
	}
	if c.Screen != ScreenBriefing {
		t.Fatal(c.Screen, c.Message)
	}
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
	jFinishFade(t, c)
	if !c.Rendering.Flight.Combat.LogisticsEnabled() || c.Mission.ResultLaunch == nil {
		t.Fatal("fresh P4 source launch absent", c.Mission.ResultUnavailable)
	}
	// Staging follower position is not part of the clean-launch film ABI. Keep
	// this explicitly a mechanics/publication witness, not a replayable flight.
	c.AbandonFilmRecording()
	c.Rendering.filmRecorder = nil
	c.DevelopmentScenario = &DevelopmentScenario{Name: "p4-supplied-follower-approach", Stage: "explicit test staging"}
	return c
}

func TestFreshFollowerLogisticsResultCommitAndRestart(t *testing.T) {
	for _, tc := range []struct {
		name           string
		seed           int64
		aircraft, kind int
		load           bool
	}{{"pickup", 38, 5, 2, false}, {"delivery", 1, 5, 1, true}, {"recon", 10, 4, 4, false}} {
		t.Run(tc.name, func(t *testing.T) {
			loadActor := -1
			if tc.load {
				loadActor = 1
			}
			c := logisticsController(t, tc.seed, tc.aircraft, loadActor)
			p, r := c.Rendering, c.Rendering.Flight
			m := p.Initialized
			if int(binary.LittleEndian.Uint16(m.Targets.Records[2][32:])) != tc.kind {
				t.Fatal("independent seed/goal changed")
			}
			if err := c.developmentScenarioPose(m.Targets.Records[0], true, 2, 1, 0); err != nil {
				t.Fatal(err)
			}
			e := &m.Entities[1]
			e.Pose.H0 = binary.LittleEndian.Uint32(m.Targets.Records[2][34:])
			e.Pose.H1 = binary.LittleEndian.Uint32(m.Targets.Records[2][38:])
			e.Pose.Height = 2
			e.RawMotion = 0
			e.Kind, e.Participation, e.ControlRaw[1] = 0x20, 1, 2
			// Runtime controller2 keeps a real accepted aircraft at the supplied low
			// approach. No TARGET/cargo/assignment/result value is pre-completed.
			beforeState := c.Store.State()
			before, _ := json.Marshal(beforeState.SelectedPilot())
			if err := c.Handle(Actions{Cancel: true}); err != nil {
				t.Fatal(err)
			}
			if err := c.AdvanceRendering(.12); err != nil {
				t.Fatal(err)
			}
			if m.Targets.Records[2][32] != 0 {
				t.Fatal("reached follower objective omitted", r.Steps, m.ObjectiveLimits, m.FollowerLimits)
			}
			if tc.kind == 2 && (m.ObjectiveAssignments[0] != 2 || m.Stores.Ships[1].Flags&0xf8 != 0x18 || m.ActorFlags[1]&1 == 0) {
				t.Fatal("actual follower pickup owner absent")
			}
			if after, _ := json.Marshal(c.Store.State().SelectedPilot()); !bytes.Equal(before, after) {
				t.Fatal("progress changed career before finalack")
			}
			// Separate supplied base approach; ordinary contact/rotor return owner
			// and result normalizer perform all final flags and durable decisions.
			if err := c.developmentScenarioPose(m.Targets.Records[0], true, 2, 1, 0); err != nil {
				t.Fatal(err)
			}
			e.Pose.H0 = r.State.Pose.H0
			e.Pose.H1 = r.State.Pose.H1
			e.Pose.Height = 2
			e.ControlRaw[1] = 1
			e.ControlRaw[5] = 0
			e.LeaderRow = 0
			scenarioSafeReturn(t, c)
			proposal := c.Debrief.Pending.Proposal
			if proposal.Facts.SourceFlags&1 == 0 || proposal.Facts.ResultContext.Actors[1].Flags&0x80 == 0 {
				t.Fatal("actual assigned actor lost normalized recovery", proposal.Facts.SourceFlags, proposal.Facts.ResultContext.Actors[1])
			}
			if after, _ := json.Marshal(c.Store.State().SelectedPilot()); !bytes.Equal(before, after) {
				t.Fatal("debrief published early")
			}
			id := c.Debrief.Pending.Start.ID
			t.Logf("journal/pending revision %d→%d while canonical pilot unchanged", beforeState.Revision, c.Store.State().Revision)
			pages := loopAcknowledge(t, c)
			committed := c.Store.State()
			reopened, err := gamestate.OpenSaveStore(c.Store.Directory, "")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(committed, reopened.State()) {
				t.Fatal("restart changed committed source result")
			}
			if err = reopened.CommitSortie(id); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(committed, reopened.State()) {
				t.Fatal("duplicate result")
			}
			t.Logf("supplied approaches; genuine seed%d accepted aircraft%d/follower1 kind%d; steps%d; pages%v; normalizedflags%x; commit%s", tc.seed, tc.aircraft, tc.kind, r.Steps, pages, proposal.Score.NormalizedFlags, id)
		})
	}
}

func TestUnknownFollowerStillPreservesCareerAndPhoenix(t *testing.T) {
	// R5 replaces this alias; keep the historical rejection/career contract here.
	c := logisticsController(t, 38, 5, -1, replay.R4Compatibility)
	m := c.Rendering.Initialized
	r := c.Rendering.Flight
	if err := c.developmentScenarioPose(m.Targets.Records[0], true, 2, 1, 0); err != nil {
		t.Fatal(err)
	}
	e := &m.Entities[1]
	e.Kind = 0x25
	e.Participation = 1
	e.ControlRaw[1] = 5
	e.ControlRaw[3] = 0
	child := &m.Entities[2]
	child.Kind = 0x20
	child.ControlRaw[1] = 1
	child.ControlRaw[5] = 1
	child.ControlRaw[3] = 11
	before, _ := json.Marshal(c.Store.State())
	phoenix, _ := c.Store.RecoveryCandidate()
	if err := c.Handle(Actions{Cancel: true}); err != nil {
		t.Fatal(err)
	}
	if err := c.AdvanceRendering(.12); err != nil {
		t.Fatal(err)
	}
	if r.Systems.UnsupportedFollowerEmptyBurn&2 == 0 || !strings.Contains(m.FollowerLimits[1], "pre-row0") {
		t.Fatal("actual undefined alias was not guarded", m.FollowerLimits)
	}
	m.EndSortieForQoL()
	if err := c.finishSortieResult(); err != nil {
		t.Fatal(err)
	}
	if c.Screen == ScreenDebrief || !strings.Contains(c.Message, "nonplayer successor is unsupported") {
		t.Fatal("unsupported mechanic became an award", c.Screen, c.Message)
	}
	if after, _ := json.Marshal(c.Store.State()); !bytes.Equal(before, after) {
		t.Fatal("guard changed career")
	}
	afterPhoenix, _ := c.Store.RecoveryCandidate()
	if !reflect.DeepEqual(phoenix, afterPhoenix) {
		t.Fatal("guard changed independent Phoenix")
	}
}
