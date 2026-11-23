package menus

import (
	"fmt"
	gamestate "gunship2000-assets/game/state"
)

func restoreCampaignPreparation(s *MissionSession, draft *gamestate.CampaignState) error {
	if draft == nil || draft.Suspended == nil {
		return nil
	}
	x := draft.Suspended
	if len(s.Flight) != 5 {
		return fmt.Errorf("Campaign restore requires freshly initialized authoritative flight members")
	}
	current := [5]gamestate.FlightPilot{}
	seen := [5]bool{}
	for _, pilot := range s.Flight {
		if pilot.SourceIndex < 0 || pilot.SourceIndex >= len(current) || seen[pilot.SourceIndex] {
			return fmt.Errorf("Campaign restore authoritative flight has invalid source slots")
		}
		current[pilot.SourceIndex] = pilot
		seen[pilot.SourceIndex] = true
	}
	if err := draft.Validate(); err != nil {
		return err
	}
	flight := make([]gamestate.FlightPilot, len(x.Flight))
	for i, assignment := range x.Flight {
		flight[i] = current[assignment.SourceIndex]
		flight[i].Group, flight[i].Leader = assignment.Group, assignment.Leader
	}
	s.ReadinessRows = append([][36]byte(nil), x.Aircraft...)
	s.InitialAircraftIDs = append([]byte(nil), x.InitialAircraftIDs...)
	s.Flight = flight
	s.Settings = x.Settings
	s.SecondaryControl = x.SecondaryControl
	s.ReadinessError = nil
	s.SettingsError = nil
	draft.Suspended = nil
	draft.Phase = gamestate.CampaignActive
	return nil
}

func campaignSessionState(p *gamestate.Pilot, s *MissionSession) (*gamestate.CampaignState, error) {
	if p == nil || s == nil || s.Data == nil || p.ID != s.PilotID || s.Data.Inputs.MissionType != gamestate.MissionCampaign || s.CampaignDraft == nil {
		return nil, fmt.Errorf("Campaign session/pilot does not match")
	}
	return gamestate.CloneCampaign(s.CampaignDraft)
}

func campaignSuspend(p *gamestate.Pilot, s *MissionSession) error {
	draft, err := campaignSessionState(p, s)
	if err != nil {
		return err
	}
	if s.SettingsError != nil || s.ReadinessError != nil || len(s.ReadinessRows) != 5 || len(s.InitialAircraftIDs) != 5 || len(s.Flight) != 5 {
		return fmt.Errorf("Campaign cannot suspend incomplete settings/flight data")
	}
	draft.Phase = gamestate.CampaignSuspended
	assignments := make([]gamestate.FlightAssignment, len(s.Flight))
	for i, pilot := range s.Flight {
		assignments[i] = gamestate.FlightAssignment{pilot.SourceIndex, pilot.Group, pilot.Leader}
	}
	draft.Suspended = &gamestate.CampaignSuspension{Mission: gamestate.CampaignSnapshot(s.Data), Aircraft: append([][36]byte(nil), s.ReadinessRows...), InitialAircraftIDs: append([]byte(nil), s.InitialAircraftIDs...), Flight: assignments, Settings: s.Settings, SecondaryControl: s.SecondaryControl}
	if err = draft.Validate(); err != nil {
		return err
	}
	p.Campaign = draft
	return nil
}

func campaignDecline(p *gamestate.Pilot, s *MissionSession) error {
	draft, err := campaignSessionState(p, s)
	if err != nil {
		return err
	}
	if err = gamestate.CampaignPenalty(p, 300); err != nil {
		return err
	}
	draft.TargetCounter = int(byte(draft.TargetCounter + 1))
	draft.ResultCounters[1] = int(byte(draft.ResultCounters[1] + 1))
	draft.Phase = gamestate.CampaignActive
	draft.Suspended = nil
	p.Campaign = draft
	return nil
}

func campaignAbort(p *gamestate.Pilot, s *MissionSession) error {
	if _, err := campaignSessionState(p, s); err != nil {
		return err
	}
	if err := gamestate.CampaignPenalty(p, 500); err != nil {
		return err
	}
	p.Campaign = nil
	p.MissionType = gamestate.MissionFlight
	p.LegacyRaw[0x4f] = 0
	return nil
}
