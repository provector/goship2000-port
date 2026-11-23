package menus

import (
	"fmt"
	gamestate "gunship2000-assets/game/state"
)

// Authored launch has a normal current-pilot result capability. The editor and
// document store never possess the isolated practice engine or generate a new
// random layout. The detached saved realization is the accepted layout owner.
func (c *FrontEnd) beginAuthoredMission(doc *gamestate.AuthoredDocument) error {
	if c.Editor != nil && c.Editor.SupportCancelled != nil && doc != nil && c.Editor.Document.ID == doc.ID {
		return fmt.Errorf("choose None or a Support option before BEGIN")
	}
	if err := gamestate.AuthoredExecutionReadyP5(c.Office.Assets.Missions.Generation, doc); err != nil {
		return err
	}
	p := c.Store.State().SelectedPilot()
	if p == nil || p.Status != gamestate.StatusActive || !c.viewingPostedPilot() || p.Rank.Raw() < 5 || p.MissionType != gamestate.MissionSimulated {
		return fmt.Errorf("Simulation requires the posted active commissioned pilot")
	}
	if p.Campaign != nil {
		return fmt.Errorf("Simulation with retained Campaign ownership is unavailable")
	}
	if doc == nil || doc.Compiled == nil {
		return fmt.Errorf("SAVE a valid mission before BEGIN")
	}
	a := c.Office.Assets.Missions
	w, err := gamestate.AuthoredWorld(a.Generation, doc.World)
	if err != nil {
		return err
	}
	if w.Theatre != p.Theatre.Raw() {
		return fmt.Errorf("select this mission's theatre before BEGIN")
	}
	prev, known := p.LegacyByte(0x5b)
	if !known {
		return fmt.Errorf("retained preparation context unavailable")
	}
	in := gamestate.MissionInputs{MissionType: gamestate.MissionSimulated, Theatre: p.Theatre, Rank: p.Rank, PilotName: p.Name, Seed: int64(doc.Seed), PreviousScenario: int(int8(prev))}
	data, err := gamestate.CompileAuthoredMission(a.Generation, doc, in)
	if err != nil {
		return err
	}
	source, err := gamestate.CloneAuthored(doc)
	if err != nil {
		return fmt.Errorf("freeze authored source: %w", err)
	}
	view, err := NewBriefingView(a, data, p, int64(doc.Seed)^0x61c8864680b583eb)
	if err != nil {
		return err
	}
	s := &MissionSession{AuthoredSource: source, Data: data, View: view, PilotID: p.ID, interstitialTicks: 35}
	s.Settings, s.SettingsError = gamestate.PreflightFromPilot(p)
	s.ReadinessRows, s.ReadinessError = gamestate.PrepareReadinessRows(a, data)
	if s.ReadinessError != nil {
		return s.ReadinessError
	}
	if len(doc.Aircraft) != 0 {
		s.ReadinessRows, err = authoredLoadout(a, data, doc.Aircraft, p.Rank)
		if err != nil {
			return err
		}
	}
	for _, r := range s.ReadinessRows {
		s.InitialAircraftIDs = append(s.InitialAircraftIDs, r[0])
	}
	s.Flight, err = gamestate.InitFlightPilots(p, data)
	if err != nil {
		return err
	}
	if err = gamestate.RefreshFlightPresentation(s.Flight, a, data); err != nil {
		return err
	}
	c.publishMission(s)
	return nil
}
