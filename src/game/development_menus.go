//go:build devtools

package main

import (
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	menuui "gunship2000-assets/game/menus"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
)

func newDevelopmentFrontEnd(paths ApplicationPaths, assets *assetdata.OfficeAssets, state gamestate.OfficeState, seed uint64) (*menuui.FrontEnd, error) {
	parent := filepath.Join(paths.Directory, "dev-output")
	if _, err := assetdata.GeneratedPath(filepath.Join(parent, ".scratch-path-check"), assets.Root); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(parent, 0755); err != nil {
		return nil, err
	}
	directory, err := os.MkdirTemp(parent, "menu-save-")
	if err != nil {
		return nil, err
	}
	// Never open the normal save or auto-import a personal roster in dev mode.
	// Scratch saves are retained for inspecting the scenario and its backups.
	store, err := gamestate.OpenManagedCareerAt(directory, "", paths.Directory, paths.install(), false)
	if err != nil {
		return nil, err
	}
	pilot, err := gamestate.NewPilot()
	if err != nil {
		return nil, err
	}
	pilot.Name, pilot.Squadron, pilot.Emblem = state.PilotName, state.Squadron, state.Emblem
	pilot.Rank, err = gamestate.RankFromRaw(state.Rank)
	if err != nil {
		return nil, err
	}
	pilot.MissionType, err = gamestate.MissionTypeFromRaw(state.MissionType)
	if err != nil {
		return nil, err
	}
	pilot.Theatre, err = gamestate.TheatreFromRaw(state.Theatre)
	if err != nil {
		return nil, err
	}
	if !state.Active {
		pilot.Status = gamestate.StatusInactive
	}
	saved := store.State()
	saved.Slots[0] = pilot
	saved.SelectedPilotID = pilot.ID
	if err = store.Commit(saved); err != nil {
		return nil, err
	}
	controller, err := menuui.NewFrontEnd(assets, store)
	if err != nil {
		return nil, err
	}
	controller.FilmDirectory = filepath.Join(directory, "replays")
	controller.MissionDirectory = filepath.Join(directory, "missions")
	controller.SummaryDirectory = directory // Diagnostic sorties must not replace the ordinary runtime's report.
	controller.Office.State.ComputerMode = state.ComputerMode
	controller.Office, err = menuui.NewOffice(assets, controller.Office.State, seed)
	fmt.Printf("Development scratch save: %s\n", directory)
	return controller, err
}

// No player menu calls this. It stands in for the future mission boundary so
// recovery can be inspected without fabricating a mission in normal progress.
func prepareDevelopmentRecovery(c *menuui.FrontEnd) error {
	s := c.Store.State()
	p := s.SelectedPilot()
	if p == nil {
		return fmt.Errorf("recovery fixture needs a selected scratch pilot")
	}
	p.Statistics = &gamestate.PilotStatistics{LastMissionScore: 101, BestMissionScore: 202, CareerScore: 303, MissionCampaignCounter: 207, PhoenixCounter: -1}
	if err := c.Store.Commit(s); err != nil {
		return err
	}
	if err := c.Store.PrepareMissionSnapshot(); err != nil {
		return err
	}
	s = c.Store.State()
	s.SelectedPilot().Statistics.CareerScore = 1000
	s.SelectedPilot().Theatre = gamestate.TheatrePhilippines
	if err := c.Store.Commit(s); err != nil {
		return err
	}
	return c.RefreshOffice()
}
