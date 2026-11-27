package menus

import (
	"bytes"
	"encoding/binary"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func captureCareerPage(t *testing.T, c *FrontEnd, name string) {
	t.Helper()
	if out := os.Getenv("GS2000_TEST_N4_FRAMES"); out != "" {
		a := c.Office.Assets.Debrief
		palette := a.Scenes[c.Debrief.Scene].Palette
		if _, v, ok := c.frozenFeedbackMedia(); ok {
			palette = v
		}
		if err := writeIndexedFrame(filepath.Join(out, name+".png"), c.Frame(false), a.Palettes[palette], c.Office.Assets.Root, nil); err != nil {
			t.Fatal(err)
		}
	}
}

func TestHonorRibbonCoversPocket(t *testing.T) {
	c := debriefDisplay(t)
	c.enterDebriefPage("jump")
	v := c.Debrief
	base := *v.Pilot
	stats := *base.Statistics
	stats.AwardCounts = append([]int8(nil), stats.AwardCounts...)
	base.Statistics = &stats
	v.Pilot = &base
	v.Tick = 200
	neighbor := c.Frame(false)
	stats.AwardCounts[0]++
	base.Name = "Provector"
	got := c.Frame(false)
	a := c.Office.Assets.Debrief
	for _, s := range a.Scenes["JUMP.SCN"].Sprites {
		if s.Key != 2 {
			continue
		}
		for y := 0; y < s.Source[3]; y++ {
			for x := 0; x < s.Source[2]; x++ {
				ink := a.Images["RANKS.PIC"][(s.Source[1]+y)*320+s.Source[0]+x]
				if ink != 0 && got[(61+y)*320+178+x] != ink {
					t.Fatal("pocket overwrote ribbon", x, y)
				}
			}
		}
	}
	captureCareerPage(t, c, "medal-of-honor")
	stats.AwardCounts[0]--
	base.Name = c.Debrief.Pending.Proposal.PilotBefore.Name
	if !bytes.Equal(neighbor, c.Frame(false)) {
		t.Fatal("neighboring award presentation changed")
	}
}

func TestPositiveCareerEndingThroughDebrief(t *testing.T) {
	for _, tc := range []struct {
		name   string
		rank   gamestate.Rank
		before int32
		ending string
	}{
		{"below", gamestate.RankCaptain, 48, ""},
		{"instructor", gamestate.RankCaptain, 149, "Fort Rucker"},
		{"general", gamestate.RankColonel, 12, "Pentagon"},
		{"both", gamestate.RankColonel, 149, "Pentagon"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := authoredUIController(t)
			c.simulationProfile = ""
			st := c.Store.State()
			pilot := st.SelectedPilot()
			pilot.MissionType = gamestate.MissionSingle
			pilot.Theatre = gamestate.TheatrePersianGulf
			if tc.rank == gamestate.RankColonel {
				pilot.MissionType = gamestate.MissionFlight
			}
			pilot.Rank = tc.rank
			pilot.LegacyRaw[0x54] = byte(tc.rank.Raw())
			pilot.Statistics.MissionCampaignCounter = tc.before
			binary.LittleEndian.PutUint32(pilot.LegacyRaw[0x6e:], uint32(tc.before))
			if tc.rank == gamestate.RankColonel {
				pilot.Statistics.CareerScore = 28490
				binary.LittleEndian.PutUint32(pilot.LegacyRaw[0x72:], 28490)
			}
			if e := c.Store.Commit(st); e != nil {
				t.Fatal(e)
			}
			if e := c.RefreshOffice(); e != nil {
				t.Fatal(e)
			}
			seed := int64(10)
			c.MissionSeed = &seed
			readyMission(t, c)
			if tc.rank == gamestate.RankColonel {
				o := c.Mission.Data.Objectives["primary"]
				if o.DisplayKind != 4 {
					t.Fatalf("recon scenario needed: %v", o.DisplayKind)
				}
				if e := c.openOrdnance(0); e != nil {
					t.Fatal(e)
				}
				for _, action := range []string{"aircraft", "replace", "back", "fits", "mount", "back", "back"} {
					found := false
					for i, v := range c.Mission.Ordnance.Choices {
						if v.Enabled && v.Action == action && (action != "replace" || v.Value == 4) {
							if e := c.developmentScenarioClick(i); e != nil {
								t.Fatal(e)
							}
							found = true
							break
						}
					}
					if !found {
						t.Fatal(action, c.Mission.Ordnance.Choices)
					}
				}
			}
			if c.Screen == ScreenOutfit {
				if e := c.developmentScenarioClick(6); e != nil {
					t.Fatal(e)
				}
			}
			if c.Screen != ScreenBriefing {
				t.Fatal("loadout", c.Screen, c.Message)
			}
			startRendering(t, c)
			jFinishFade(t, c)
			p := c.Rendering
			resultSeed := uint32(1)
			c.Mission.ResultLaunch.DevelopmentSeed = &resultSeed
			row, offset, above := 0, 42, int16(2)
			if tc.rank == gamestate.RankColonel {
				row, offset, above = 2, 34, 200
			}
			target := p.ResultTargets.Records[row]
			a := FilmApproach{H0: binary.LittleEndian.Uint32(target[offset:]), H1: binary.LittleEndian.Uint32(target[offset+4:]), AboveSupport: above, Rotor: 1, Stage: 128}
			if e := c.ApplyFilmInitialAdjustment(FilmInitialAdjustment{Label: "Career threshold approach", Approach: &a}); e != nil {
				t.Fatal(e)
			}
			useComparedMissionPoint(t, c)
			tick := func() {
				if e := c.AdvanceRendering(.12); e != nil {
					t.Fatal(e)
				}
			}
			for i := 0; i < 10; i++ {
				tick()
			}
			useComparedMissionPoint(t, c)
			if c.Rendering.Flight.Steps != 0 {
				t.Fatal("discarded continuation survived")
			}
			for i := 0; i < 12; i++ {
				tick()
			}
			if c.Screen == ScreenScene {
				act(t, c, Actions{EndMission: true})
			}
			act(t, c, Actions{ConfirmQuit: true})
			if c.Debrief == nil {
				t.Fatal(c.Screen, c.Message)
			}
			prop := c.Debrief.Pending.Proposal
			if tc.rank == gamestate.RankColonel && prop.AcceptedPilot.Rank != gamestate.RankBrigadierGeneral {
				t.Fatalf("no promotion: flags %x score %d kind %v rank %v loss %v", prop.Score.NormalizedFlags, prop.Score.Score, prop.Facts.LaunchObjectiveKinds, prop.AcceptedPilot.Rank, prop.Result.Loss)
			}
			feedback := false
			for i := 0; i < 25 && c.Screen == ScreenDebrief; i++ {
				c.Debrief.Tick = 400
				if c.Debrief.Page == "crew" {
					c.Debrief.CrewSelection = len(c.Debrief.Pending.Proposal.Result.Recommendations)
				}
				if c.Debrief.Page == "feedback" {
					feedback = true
					_, msg := c.feedbackPresentation()
					if tc.ending != "" && !strings.Contains(msg, tc.ending) {
						t.Fatal(msg)
					}
					t.Logf("page %s text %q music %x score %d total %d rank %v status %v packed %d", c.Debrief.Scene, msg, prop.Result.MusicCue, prop.Score.Score, prop.AcceptedPilot.Statistics.CareerScore, prop.AcceptedPilot.Rank, prop.AcceptedPilot.Status, prop.AcceptedPilot.Statistics.MissionCampaignCounter)
					captureCareerPage(t, c, tc.name)
				}
				act(t, c, Actions{Enter: true})
				act(t, c, Actions{})
			}
			if !feedback || c.Screen != ScreenFilms {
				t.Fatal(c.Screen, c.Message)
			}
			committed := c.Store.State()
			q := committed.SelectedPilot()
			if q == nil {
				q = committed.Slots[0]
			}
			if (q.Status == gamestate.StatusRetired) != (tc.ending != "") || q.Statistics.MissionCampaignCounter != tc.before+1 {
				t.Fatal(q.Status, q.Statistics)
			}
			if e := c.Store.CommitSortie(prop.Facts.SortieID); e != nil {
				t.Fatal(e)
			}
			filmAction(t, c, 15)
			for !c.Viewer.Playback.Done {
				if e := c.Viewer.Playback.Step(); e != nil {
					t.Fatal(e)
				}
			}
			reopened, e := gamestate.OpenSaveStore(c.Store.Directory, "")
			if e != nil || !reflect.DeepEqual(committed, reopened.State()) {
				t.Fatal("repeat acknowledgement/restart/playback changed roster", e)
			}
		})
	}
}
