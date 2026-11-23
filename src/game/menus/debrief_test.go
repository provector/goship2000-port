package menus

import (
	"bytes"
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

// Renderer-only labelled state. Result construction, event bookkeeping and
// persistent acceptance are exercised by the real preparation/transaction tests.
func debriefDisplay(t *testing.T) *FrontEnd {
	t.Helper()
	a := preparedUI(t)
	pilot := &gamestate.Pilot{Name: "Test Leader", Rank: gamestate.RankCaptain, Squadron: "Test Squadron Name", LegacyRaw: make([]byte, gamestate.LegacyRecordSize), Statistics: &gamestate.PilotStatistics{AwardCounts: make([]int8, 8)}}
	p := &gamestate.SortieProposal{PilotBefore: pilot, AcceptedPilot: pilot, Success: true, Result: &gamestate.SortieResultDetails{CrewPro: true, RescuerActor: -1, MusicCue: 0x1b}}
	p.Facts.MissionType = gamestate.MissionFlight
	p.Facts.ResultContext = &gamestate.SortieResultContext{Actors: make([]gamestate.SortieActorFacts, 5)}
	for i := 1; i <= 4; i++ {
		actor := &p.Facts.ResultContext.Actors[i]
		actor.Actor, actor.SourceIndex, actor.RankRaw = i, i, 1
		actor.Name = a.Missions.Preflight.SquadronNames[i-1]
		actor.Callsign = []string{"ECHO 2", "ECHO 3", "PAPA 4", "PAPA 5"}[i-1]
		actor.Group = gamestate.BoolInt(i >= 3)
		actor.FlightLeader = i == 3
		actor.StoredRaw[26+7], actor.StoredRaw[34], actor.StoredRaw[36] = 1, 1, byte(i)
		p.Result.Crew = append(p.Result.Crew, gamestate.SortieCrewMetric{Actor: i, SourceIndex: i, Name: actor.Name, Rank: 1, Flight: 100, Attack: 50, Defense: 100, Mean: 83})
	}
	c := &FrontEnd{Office: &Office{Assets: a}, Menus: a.Menus, Screen: ScreenDebrief, Debrief: &DebriefView{Pending: &gamestate.PendingSortie{Proposal: p}, Pilot: pilot}}
	return c
}

func TestDebriefSourcePageOrderingAndRenderingIsolation(t *testing.T) {
	c := debriefDisplay(t)
	p := c.Debrief.Pending.Proposal
	p.Score.NormalizedFlags = 0x20
	p.Result.Campaign = &gamestate.CampaignResultProposal{Concluded: true, Verdict: 1, SortieSuccess: false}
	if page := c.debriefAfterCommander(c.Debrief); page != "rescue" {
		t.Fatal(page)
	}
	c.enterDebriefPage("rescue")
	c.Debrief.Tick = 0
	if err := c.handleDebrief(Actions{Enter: true}); err != nil {
		t.Fatal(err)
	}
	if c.Debrief.Page != "rescue" || c.Debrief.Tick != 324 {
		t.Fatal("skip must not acknowledge next page")
	}
	if err := c.handleDebrief(Actions{Enter: true}); err != nil {
		t.Fatal(err)
	}
	if c.Debrief.Page != "campaign" || c.Debrief.Scene != "WIN.SCN" {
		t.Fatal("Campaign conclusion must precede JUMP")
	}
	c.Debrief.Tick = 120
	if err := c.handleDebrief(Actions{Enter: true}); err != nil {
		t.Fatal(err)
	}
	if c.Debrief.Page != "jump" {
		t.Fatal("missing player awards")
	}
	c.Debrief.Tick = 120
	if err := c.handleDebrief(Actions{Enter: true}); err != nil {
		t.Fatal(err)
	}
	if c.Debrief.Page != "crew" {
		t.Fatal("genuine Flight crew page missing")
	}
	if err := c.handleDebrief(Actions{Cancel: true}); err != nil || c.Debrief.Page != "crew" {
		t.Fatal("Escape is not shared quit110 or award cancellation")
	}
	if err := c.handleDebrief(Actions{Enter: true}); err != nil || c.Debrief.Page != "feedback" {
		t.Fatal("sourceAccept failed", err)
	}
	p.Result.Loss = true
	p.Score.NormalizedFlags = 0x10
	if page := c.debriefAfterCommander(c.Debrief); page != "loss" {
		t.Fatal("loss must bypass awards/crew/Campaign", page)
	}
	before, _ := json.Marshal(p)
	for _, page := range []string{"loss", "rescue", "campaign", "crew"} {
		c.enterDebriefPage(page)
		c.Debrief.Tick = 400
		for i := 0; i < 4; i++ {
			frame := c.debriefFrame()
			if len(frame) != 64000 {
				t.Fatal(page)
			}
		}
	}
	after, _ := json.Marshal(p)
	if !bytes.Equal(before, after) {
		t.Fatal("draw advanced proposal, RNG or choices")
	}
}

func TestDebriefPreparedSceneCompositionAndCaptures(t *testing.T) {
	c := debriefDisplay(t)
	p := c.Debrief.Pending.Proposal
	p.Result.Campaign = &gamestate.CampaignResultProposal{Concluded: true, Verdict: 1, SortieSuccess: true}
	p.Awards[6] = 1
	out := os.Getenv("GS2000_TEST_N4_FRAMES")
	if out != "" {
		if err := os.MkdirAll(out, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []struct {
		page            string
		flags           byte
		style, x, width int
	}{{"loss", 0x10, 5, 6, 200}, {"loss", 8, 5, 6, 200}, {"rescue", 0x20, 0, 6, 300}, {"campaign", 0x80, 0, 16, 280}, {"crew", 0x80, 0, 0, 0}} {
		p.Score.NormalizedFlags = q.flags
		c.enterDebriefPage(q.page)
		if q.page == "rescue" {
			for i := 0; i < 324; i++ {
				c.tickDebrief()
			}
		} else {
			c.Debrief.Tick = 160
		}
		frame := c.debriefFrame()
		if len(frame) != 64000 || len(c.CurrentPalette()) != 256 {
			t.Fatal(q.page, "missing source bank/palette")
		}
		if q.page != "crew" {
			original := c.Office.Assets.Debrief.Images[c.Office.Assets.Debrief.Scenes[c.Debrief.Scene].Background]
			if frame[10*320+10] != original[10*320+10] {
				t.Fatal(q.page, "unrelated artwork erased")
			}
		} else {
			// Source four panel fields beginx+4,y3 and100/50/100 starty72.
			for i := 0; i < 4; i++ {
				if frame[9*320+i*80+4] != byte([]int{9, 9, 7, 7}[i]) {
					t.Fatal("source group colour/header boundary", i)
				}
			}
			if bytes.Count(frame[72*320:90*320], []byte{10}) == 0 {
				t.Fatal("real source metric glyphs absent")
			}
		}
		if out != "" {
			name := q.page
			if q.page == "loss" && q.flags == 8 {
				name = "mia"
			}
			palette := c.Office.Assets.Debrief.Scenes[c.Debrief.Scene].Palette
			if err := writeIndexedFrame(filepath.Join(out, name+".png"), frame, c.Office.Assets.Debrief.Palettes[palette], c.Office.Assets.Root, map[string]any{"scope": "labelled supplied native renderer state; source artwork/operands, not original-frame equivalence", "page": q.page, "flags": q.flags, "scene": c.Debrief.Scene, "palette": palette}); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Actual source KIA artwork remains transparent at index0; the existing
	// prepared LORI API is tested independently of a hand-redrawn bugler.
	a := c.Office.Assets.Debrief
	frame := bytes.Repeat([]byte{23}, 64000)
	if err := a.Lori(frame, "KIA.SCN", 0, 0, 223, 104); err != nil {
		t.Fatal(err)
	}
	if !assetdata.OfficeRectFits([4]int{223, 104, 34, 42}) {
		t.Fatal("source rectangle")
	}
}

func TestRescueOriginalPaletteCycleAndBankOwnership(t *testing.T) {
	c := debriefDisplay(t)
	a := c.Office.Assets.Debrief
	original := append(color.Palette(nil), a.Palettes["RESCUE.PAL"]...)
	c.enterDebriefPage("rescue")
	for i := 0; i < 4; i++ {
		c.tickDebrief()
	}
	for i := 0; i < 256; i++ {
		from := i
		if i >= 16 && i < 32 {
			from = 16 + (i-16)/4*4 + (i-16+3)%4
		}
		if c.CurrentPalette()[i] != original[from] {
			t.Fatalf("source four-triplet rotate index%d", i)
		}
		if a.Palettes["RESCUE.PAL"][i] != original[i] {
			t.Fatal("mutable palette escaped page owner")
		}
	}
	for i := 0; i < 12; i++ {
		c.tickDebrief()
	}
	for i := 0; i < 256; i++ {
		if c.CurrentPalette()[i] != original[i] {
			t.Fatal("four source rotations must restore palette")
		}
	}
	before := append([]byte(nil), c.Debrief.Clean...)
	for i := 0; i < 8; i++ {
		c.Frame(false)
	}
	if !bytes.Equal(before, c.Debrief.Clean) {
		t.Fatal("redraw advanced CMPX animation")
	}
	c.enterDebriefPage("loss")
	if c.Debrief.Palette != nil || c.Debrief.Clean != nil {
		t.Fatal("palette/composite state leaked to next controller")
	}
}
