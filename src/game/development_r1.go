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

// configureCommanderAcceptance supplies only a fresh commissioned pilot and
// source-initialized crew. Mission generation, loadout, starting pose, orders,
// objective eligibility and results remain ordinary production decisions.
// Explicit dev command only; this never opens the player's career directory.
func configureCommanderAcceptance(c *menuui.FrontEnd, app string) error {
	path := filepath.Join(app, "dev-output", "scenario.json")
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		return fmt.Errorf("refusing to replace retained commander fixture metadata: %s", path)
	}
	next := c.Store.State()
	p := next.SelectedPilot()
	if p == nil {
		return fmt.Errorf("commander fixture requires a disposable pilot")
	}
	p.Name, p.Rank = "Pad Commander", gamestate.RankCaptain
	p.MissionType, p.Theatre = gamestate.MissionFlight, gamestate.TheatrePersianGulf
	p.Statistics = &gamestate.PilotStatistics{LastMissionScore: 300, BestMissionScore: 600, CareerScore: 7900, MissionCampaignCounter: 12, PhoenixCounter: -1, AwardCounts: []int8{0, 0, 0, 0, 1, 2, 0, 1}}
	p.LegacyRaw[0x54], p.LegacyRaw[0x55], p.LegacyRaw[0x56] = 7, 3, 1
	binary.LittleEndian.PutUint16(p.LegacyRaw[0x59:], 300)
	binary.LittleEndian.PutUint32(p.LegacyRaw[0x6e:], 12)
	binary.LittleEndian.PutUint32(p.LegacyRaw[0x72:], 7900)
	p.Preflight = &gamestate.PreflightSettings{CPGControls: 2}
	var err error
	p.SquadronPilots, err = gamestate.InitializeSquadron(p, next.Slots[:], c.Office.Assets.Missions.Preflight.SquadronNames, rand.New(rand.NewSource(44)).Intn)
	if err != nil {
		return err
	}
	if err = c.Store.Commit(next); err != nil {
		return err
	}
	if err = c.RefreshOffice(); err != nil {
		return err
	}
	m, err := gamestate.GenerateMission(c.Office.Assets.Missions.Generation, gamestate.MissionInputs{MissionType: p.MissionType, Theatre: p.Theatre, Rank: p.Rank, PilotName: p.Name, Seed: 75})
	if err != nil {
		return err
	}
	if m.World != "GULF4.WRL" || m.ObjectHash != "d2d60fcb00b85efb2dc05a07c6ed2c2bc868f66364fce6c7b2a618e67c621544" || m.Conditions.Flags&0x800 != 0 || m.Objectives["primary"].DisplayKind != 4 {
		return fmt.Errorf("R1 commander seed75 source witness changed")
	}
	crew, err := gamestate.InitFlightPilots(p, m)
	if err != nil {
		return err
	}
	if err = gamestate.RefreshFlightPresentation(crew, c.Office.Assets.Missions, m); err != nil {
		return err
	}
	rows, err := gamestate.PrepareReadinessRows(c.Office.Assets.Missions, m)
	if err != nil {
		return err
	}
	if len(rows) != 5 || rows[1][0] != 4 {
		return fmt.Errorf("R1 default scout loadout changed")
	}
	meta := map[string]any{"name": "r1-pad-commander", "scratch_save": c.Store.Directory, "seed": 75, "world": m.World, "object_sha256": m.ObjectHash, "conditions": m.Conditions, "objectives": m.Objectives, "squadron": p.SquadronPilots, "flight": crew, "readiness": rows, "scope": "Supplied fresh Captain/history and crew only; ordinary generated Flight, no pose, target, objective-completion or result adjustment", "instructions": "Door -> Flight briefing. Retain the source default loadout: ZULU2 is already an eligible Kiowa scout. Accept all aircraft and settings, start rotor; keep player on pad. Use GPS orders and return to cockpit between them. Primary is sector recon near actual BTR-60. Only eligible scout aircraft within original range earn recon credit."}
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	fmt.Printf("R1 supplied fresh commander setup: %s\n", path)
	return nil
}
