//go:build devtools

package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	menuui "gunship2000-assets/game/menus"
	gamestate "gunship2000-assets/game/state"
	"math/rand"
	"os"
	"path/filepath"
)

// Explicit disposable setup only. No document, objective, pose or result is
// supplied: authoring and flight still use the ordinary production controllers.
func configureEditorAcceptance(c *menuui.FrontEnd, app string) error {
	next := c.Store.State()
	p := next.SelectedPilot()
	if p == nil {
		return fmt.Errorf("editor fixture requires a disposable pilot")
	}
	p.Name = "Editor Captain"
	p.Rank = gamestate.RankCaptain
	p.MissionType = gamestate.MissionSimulated
	p.Theatre = gamestate.TheatreCentralEurope
	p.Statistics = &gamestate.PilotStatistics{LastMissionScore: 300, BestMissionScore: 600, CareerScore: 7900, MissionCampaignCounter: 12, PhoenixCounter: -1, AwardCounts: []int8{0, 0, 0, 0, 1, 2, 0, 1}}
	p.LegacyRaw[0x54], p.LegacyRaw[0x55], p.LegacyRaw[0x56] = 7, 5, 1
	binary.LittleEndian.PutUint16(p.LegacyRaw[0x59:], 300)
	binary.LittleEndian.PutUint32(p.LegacyRaw[0x6e:], 12)
	binary.LittleEndian.PutUint32(p.LegacyRaw[0x72:], 7900)
	rng := rand.New(rand.NewSource(44))
	var e error
	p.SquadronPilots, e = gamestate.InitializeSquadron(p, next.Slots[:], c.Office.Assets.Missions.Preflight.SquadronNames, rng.Intn)
	if e != nil {
		return e
	}
	if e = c.Store.Commit(next); e != nil {
		return e
	}
	if e = c.RefreshOffice(); e != nil {
		return e
	}
	v := map[string]any{"scratch_save": c.Store.Directory, "profile": "Step P supplied established Captain/history; source-initialized squadron; empty owned mission files", "supplied_history": true, "supplied_pose": false, "supplied_goals": false, "theatre": "Central Europe", "rank": "Captain", "seed": 44}
	b, _ := json.MarshalIndent(v, "", "  ")
	path := filepath.Join(app, "dev-output", "editor-profile.json")
	if _, e = os.Stat(path); !os.IsNotExist(e) {
		return fmt.Errorf("refusing to replace retained editor profile")
	}
	if e = os.WriteFile(path, append(b, '\n'), 0600); e != nil {
		return e
	}
	fmt.Printf("Step P disposable editor setup: %s\n", path)
	return nil
}
