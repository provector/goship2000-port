package menus

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
)

// Four retained public seeds only. These are fresh native-generated Campaigns,
// not authored imports, a supplied live actor, or a personal suspended sortie.
// Seed2 supplies a real Su-25; the other three independently prove that an
// unsatisfied ingress allowance leaves kind1 absent without blocking Campaign.
func TestGeneratedCampaignIngressColdActivation(t *testing.T) {
	assets := os.Getenv("GS2000_TEST_MISSIONS")
	if assets == "" {
		t.Skip("prepared original Campaign assets required")
	}
	a, err := assetdata.LoadFrontEndAssets(assets)
	if err != nil {
		t.Fatal(err)
	}
	expectedWorld := map[int64]string{1: "EURO7.WRL", 2: "EURO7.WRL", 3: "EURO9.WRL", 4: "EURO8.WRL"}
	expectedAllowanceDraw := map[int64]int{1: 74, 2: 27, 3: 73, 4: 50}
	for _, seed := range []int64{1, 2, 3, 4} {
		t.Run(fmt.Sprint("seed", seed), func(t *testing.T) {
			pilot, err := gamestate.NewPilot()
			if err != nil {
				t.Fatal(err)
			}
			pilot.Name = "R23 Campaign"
			pilot.Rank = gamestate.RankCaptain
			pilot.MissionType = gamestate.MissionCampaign
			pilot.Theatre = gamestate.TheatreCentralEurope
			pilot.Preflight = &gamestate.PreflightSettings{CPGControls: 2, EnemyQuality: 3}
			pilot.LegacyRaw[0x54], pilot.LegacyRaw[0x55], pilot.LegacyRaw[0x56] = 7, 5, 1
			pilot.LegacyRaw[0x5d], pilot.LegacyRaw[0x5f] = 3, 2
			binary.LittleEndian.PutUint32(pilot.LegacyRaw[0x72:], 0)
			supplied := gamestate.EmptySaveState()
			supplied.Slots[0] = pilot
			supplied.SelectedPilotID = pilot.ID
			pilot.SquadronPilots, err = gamestate.InitializeSquadron(pilot, supplied.Slots[:], a.Missions.Preflight.SquadronNames, rand.New(rand.NewSource(44)).Intn)
			if err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			store := &gamestate.SaveStore{Directory: filepath.Join(root, "saves")}
			if err = os.MkdirAll(store.Directory, 0700); err != nil {
				t.Fatal(err)
			}
			if err = store.Initialize(supplied); err != nil {
				t.Fatal(err)
			}
			c, err := NewFrontEnd(a, store)
			if err != nil {
				t.Fatal(err)
			}
			c.simulationProfile = ""
			c.Runtime.DisableEnemyAI = false
			c.Runtime.DisableWingmenAI = false
			c.FilmDirectory = filepath.Join(root, "films")
			c.MissionDirectory = filepath.Join(root, "missions")
			if err = c.startMission(seed, nil); err != nil {
				t.Fatal("ordinary Campaign preparation", err)
			}
			if c.Mission == nil || c.Mission.CampaignDraft == nil || c.Mission.Data.Inputs.MissionType != gamestate.MissionCampaign {
				t.Fatal("not normal Campaign entry")
			}
			m := c.Mission.Data
			if m.World != expectedWorld[seed] {
				t.Fatal("Campaign scenario", m.World)
			}
			sawAllowance := false
			for _, draw := range m.Draws {
				if draw.Site == "A1a017" {
					sawAllowance = true
					if draw.Value != expectedAllowanceDraw[seed] {
						t.Fatal("Campaign allowance draw", draw)
					}
				}
			}
			if !sawAllowance {
				t.Fatal("missing original generation allowance")
			}
			air := []int{}
			for row, o := range m.Objects {
				if row >= 9 && o.Raw[10] == 1 {
					air = append(air, row)
				}
			}
			if seed != 2 {
				if len(air) != 0 {
					t.Fatal("inactive Campaign unexpectedly admitted ingress", air)
				}
				t.Logf("normal Campaign seed%d %s A1A017=%d correctly no kind1; hostile helicopters remain separately source-selected", seed, m.World, expectedAllowanceDraw[seed])
				return
			}
			if len(air) != 1 || air[0] != 41 {
				t.Fatal("Campaign kind1 producer", air)
			}
			raw := [32]byte{8, 0, 139, 48, 5, 0, 0, 48, 0, 0, 1, 246, 86, 86, 152, 83, 117, 45, 50, 53, 0, 0, 0, 0, 0, 0, 0, 50, 0, 70, 0, 2}
			if m.Objects[41].Raw != raw || m.Objects[41].Formation != 116 || m.ObjectHash != "56cea47d5b8d34b70b7f61fc84a0ee773141f5286ce55be2b42f339bb480e28c" {
				t.Fatal("source selected Su25 realization", m.Objects[41], m.ObjectHash)
			}
			for n := 0; n < 100 && c.Screen != ScreenBriefing; n++ {
				if err = c.Tick(); err != nil {
					t.Fatal(err)
				}
			}
			if c.Screen != ScreenBriefing {
				t.Fatal("Campaign briefing", c.Screen, c.Message)
			}
			startRendering(t, c)
			jFinishFade(t, c)
			defer c.AbandonFilmRecording()
			p := c.Rendering
			e := p.Initialized.Entities[41]
			if e.Kind != 1 || e.Participation != 0 || e.RawMotion != 0 || e.Pose.Height != 0 || e.Pose.H0 != 340107 || e.Pose.H1 != 12288 {
				t.Fatal("cold Campaign actor", e)
			}
			retained := false
			for _, instance := range p.instances {
				if instance.ID == e.ID {
					retained = true
				}
			}
			if !retained {
				t.Fatal("cold Campaign render header omitted")
			}
			c.Handle(Actions{})
			c.Handle(Actions{FlightCommands: []int{2}})
			if err = c.advanceFlight(.1); err != nil {
				t.Fatal(err)
			}
			first := p.Initialized.Entities[41]
			if first.Participation != 1 || first.Pose.H0 != e.Pose.H0 || first.Pose.H1 != e.Pose.H1 || first.Pose.Height != 1024 || first.RawMotion != 400 {
				t.Fatal("Campaign first source P0 visit", first)
			}
			c.Frame(false)
			visible := false
			for _, instance := range p.Scene.Instances {
				if instance.ID == e.ID {
					visible = true
				}
			}
			if !visible {
				t.Fatal("active Campaign aircraft missing renderer")
			}
			if err = c.advanceFlight(.1); err != nil {
				t.Fatal(err)
			}
			second := p.Initialized.Entities[41]
			if second.Pose.H0 == first.Pose.H0 && second.Pose.H1 == first.Pose.H1 {
				t.Fatal("Campaign second visit did not move", second)
			}
			if p.Error != nil || p.Flight.Failure != "" {
				t.Fatal(p.Error, p.Flight.Failure)
			}
			t.Logf("normal Campaign seed2 EURO7 F116 row41 model86 attackF6 cold%+v first%+v second%+v contact%+v", e.Pose, first.Pose, second.Pose, p.Flight.Avionics.Contacts[41])
		})
	}
}
