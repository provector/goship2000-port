package menus

import (
	"encoding/binary"
	"fmt"
	gamestate "gunship2000-assets/game/state"
)

// freezeResultContext is the one boundary between live mission owners and the
// pure GS finalizer. Reserved rescue objects are not participating crew rows.
func (c *FrontEnd) freezeResultContext(f *gamestate.SortieFacts) error {
	s, p := c.Mission, c.Rendering
	m, r, l := p.Initialized, p.Flight, s.ResultLaunch
	ships, err := m.Stores.SourceShipRows()
	if err != nil {
		return err
	}
	if s.Data.SourceWorld == nil || len(s.Data.SourceWorld.SNHD) == 0 || len(m.Stores.Ships) != f.ActorCount {
		return fmt.Errorf("missing scenario or aircraft return facts")
	}
	if len(m.Entities) < 80 || len(m.FlightRows) != f.ActorCount || len(m.Flight) != f.ActorCount || len(m.CrewPerformance) != f.ActorCount {
		return fmt.Errorf("result actor/world provenance incomplete")
	}
	ctx := &gamestate.SortieResultContext{RetireByMissions: l.Start.RetireByMissions, Version: 1, LaunchPose: l.LaunchPose, Seed: gamestate.ResultSeed(l.Start.ID), SeedPolicy: gamestate.ResultSeedPolicy}
	if p.r5Mechanics {
		ctx.GameplayPolicy = gamestate.SafeGameplayPolicy
		ctx.RetainedCampaignSHA256 = l.RetainedCampaignSHA256
	}
	ctx.MissionRecovery = &gamestate.MissionRecoveryContext{Policy: gamestate.MissionRecoveryPolicy,
		LaunchTargets: append([][62]byte(nil), l.Targets[:]...), ObjectiveAssignments: m.ObjectiveAssignments,
		Candidates: l.Start.RecoveryRoster}
	if p.r5Mechanics {
		ctx.MissionRecovery.Policy = gamestate.OptionalMissionRecoveryPolicy
		if m.Targets != nil && m.Targets.ObjectiveRevisions != nil {
			revisions := *m.Targets.ObjectiveRevisions
			ctx.MissionRecovery.ObjectiveRevisions = &revisions
		}
	}
	// Source3be5 is the high SWITCHES byte, including pass-through0800.
	ctx.SetupFlags = byte(f.LaunchSwitches >> 8)
	ctx.SelectedScenario = byte(s.Data.SourceWorld.Suffix)
	ctx.ScenarioOrdinal = byte(s.Data.SourceWorld.SNHD[0])
	if l.DevelopmentSeed != nil {
		ctx.Seed = *l.DevelopmentSeed
		ctx.SeedPolicy = "development-supplied-CRT-seed-v1"
	}
	for i, e := range m.Entities[:80] {
		ctx.DynamicKinds[i], ctx.WorldPoses[i] = e.Kind, e.Pose
	}
	for i, row := range m.FlightRows {
		if row < 0 || row >= len(m.Entities) || !m.CrewPerformance[i].SkillKnown {
			return fmt.Errorf("actor%d has unsupported runtime/proficiency provenance", i)
		}
		e, crew := m.Entities[row], m.Flight[i]
		if p.resilientFlight && e.ControlCarrierOnly {
			return fmt.Errorf("actor%d active CONTROL owner unavailable", i)
		}
		a := gamestate.SortieActorFacts{Actor: i, SourceIndex: crew.SourceIndex, Name: crew.Name, RankRaw: crew.Rank.Raw(), AcceptedSelector: l.Accepted[i], Kind: e.Kind,
			Control: e.ControlRaw[1], Leader: e.LeaderRow, Pose: e.Pose, Ship: ships[i], StoredRaw: crew.StoredRaw, RawA: m.CrewPerformance[i].RawA, RawB: m.CrewPerformance[i].RawB,
			Callsign: crew.Callsign, Group: crew.Group, FlightLeader: crew.Leader}
		if p.r5Mechanics && a.Control == 1 {
			// Freeze the actual CONTROL+5 consumed by formation, not a cached
			// initialization identity after a later controller transition.
			a.Leader = int(int8(e.ControlRaw[5]))
		}
		if e.RuntimeRow != row || e.FlightSlot != i || e.ID == "" || m.Stores.Ships[i].AircraftID != int(l.Accepted[i]) {
			return fmt.Errorf("actor%d identity does not match its accepted aircraft", i)
		}
		var fuel int16
		if i < len(r.Systems.Fuel) {
			fuel = r.Systems.Fuel[i]
		} else {
			if recovered, ok := c.confirmedFuel(i); ok {
				fuel = recovered
				c.qualifyResult(fmt.Sprintf("Missing fuel row for %s at step %d. Recovered %d from its synchronized aircraft store.", e.ID, r.Steps, fuel))
			} else {
				// Fuel is informational at this boundary. No scorer or return
				// classifier consumes this word. Minus one means unavailable.
				fuel = -1
				c.qualifyResult(fmt.Sprintf("Missing fuel row for %s at step %d. Current fuel unverified and recorded as unknown.", e.ID, r.Steps))
			}
		}
		binary.LittleEndian.PutUint16(a.Ship[23:25], uint16(fuel))
		if i == 0 {
			f.FinalFuel = fuel
		}
		if i == 0 {
			a.Pose = f.FinalPose
			a.Damage = gamestate.PlayerDamage12(&r.Systems)
			a.Flags = f.SourceFlags
		} else if i < len(m.Stores.Damage) {
			if p.p4Mechanics && i < len(m.ActorFlags) {
				a.Flags = m.ActorFlags[i]
			}
			if p.wingmanDamageMechanics {
				if m.Stores.NPCDamage == nil {
					return fmt.Errorf("actor%d full wingman damage owner missing", i)
				}
				a.Damage = m.Stores.Damage12(i)
			} else {
				copy(a.Damage[2:], m.Stores.Damage[i][:])
			}
		} else {
			return fmt.Errorf("actor%d health owner missing", i)
		}
		ctx.Actors = append(ctx.Actors, a)
	}
	if f.ActorCount == 5 {
		ctx.Version = 2 // New actual reserve carrier facts; never reinterpret old v1.
		for actor := 1; actor < 5; actor++ {
			row := actor + 4
			if row >= len(m.Entities) || m.Entities[row].RuntimeRow != row || m.Entities[row].Object.Raw[10] != 0x1b || m.FlightRows[actor] != actor {
				return fmt.Errorf("reserved rescue actor%d lacks source identity", actor)
			}
			e := m.Entities[row]
			ctx.ReservedRescues = append(ctx.ReservedRescues, gamestate.SortieReservedRescue{Actor: actor, Kind: e.Kind, Carrier: e.DynamicRaw[3]})
		}
	}
	if p.p4Mechanics {
		scene := byte(s.Data.Scene)
		ctx.Scene = &scene
		ctx.Version = 3
	}
	if c.Office.Assets.Missions.Preflight != nil {
		ctx.ReplacementNames = append([]string(nil), c.Office.Assets.Missions.Preflight.SquadronNames...)
	}
	for _, pilot := range c.Store.State().Slots {
		if pilot != nil {
			ctx.OccupiedNames = append(ctx.OccupiedNames, pilot.Name)
			for _, crew := range pilot.SquadronPilots {
				ctx.OccupiedNames = append(ctx.OccupiedNames, crew.Name)
			}
		}
	}
	if l.CampaignDraft != nil {
		stage := byte(1)
		if l.CampaignDraft.Phase == gamestate.CampaignSuspended {
			return fmt.Errorf("launch retained a suspended draft without accepted restoration")
		}
		ctx.Campaign = &gamestate.CampaignResultInput{LaunchDraft: l.CampaignDraft, Scenario: s.Data.Scenario, World: s.Data.World, Ordinal: int(s.Data.SourceWorld.SNHD[0]), SourceStage: stage, LaunchHigh: byte(m.Switches >> 8), CurrentMission: f.MissionType}
	} else if f.MissionType == gamestate.MissionCampaign {
		return fmt.Errorf("Campaign result has no accepted launch draft")
	}
	f.ResultContext = ctx
	return nil
}
