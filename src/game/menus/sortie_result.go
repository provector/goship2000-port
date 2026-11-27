package menus

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/diagnostics"
	"gunship2000-assets/game/flight"
	gamestate "gunship2000-assets/game/state"
)

type fuelReceipt struct {
	actor string
	step  uint64
	phase uint16
	value int16
}

type sortieLaunch struct {
	fuel                   [5]fuelReceipt
	Start                  gamestate.SortieStart
	Tables                 gamestate.SortieTables
	Coefficient            int16
	Recommended, Accepted  []byte
	Kinds                  [2]int16
	Targets                [5][62]byte // Immutable GS launch carrier, separate from live GS2 goals.
	LaunchPose             gamestate.InitializedPose
	CampaignDraft          *gamestate.CampaignState
	DevelopmentSeed        *uint32 // Named development scenarios only, never normal launch.
	RetainedCampaignSHA256 string  `json:",omitempty"` // R5 Training's independent immutable owner.
}

func resultTables(a *assetdata.DebriefAssets) gamestate.SortieTables {
	return gamestate.SortieTables{SourceProfile: assetdata.IslandIce469085.Name,
		RankThresholds: a.RankThresholds, MedalThresholds: a.MedalThresholds,
		MedalPenaltyCredits: a.MedalPenaltyCredits, RankNames: a.RankNames, MedalNames: a.MedalNames}
}

func resultDigest(value any) (string, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return assetdata.Hash(b), nil
}

// Result admission does not remove previously supported flight/preparation.
// Missing producers leave an explicit no-career boundary, not fabricated facts.
func (c *FrontEnd) beginSortieResult(final *FinalPreflightConfiguration) error {
	s, p := c.Mission, c.Rendering
	c.endingSceneAudio = nil
	s.ResultLaunch = nil
	s.ResultUnavailable = ""
	p.ResultTargets = nil
	p.Initialized.Targets = nil
	// TARGET62 is mission state for every supported flight, independently of
	// career admission. Prepare once from the accepted source handoff, then copy
	// for this sortie so subsequent play never edits preparation/briefing data.
	targetErr := gamestate.PrepareMissionTargets(c.Office.Assets.Missions, final.Mission)
	if targetErr == nil {
		targets := gamestate.CloneMissionTargets(final.Mission.Targets)
		if p.Initialized.SafeGameplay {
			targets.InitObjectiveRevisions()
		}
		p.ResultTargets = targets
		p.Initialized.Targets = targets
		p.Flight.Combat.Primary.DisplayKind = int(binary.LittleEndian.Uint16(targets.Records[2][32:]))
		p.Flight.Combat.Primary.Required = int16(binary.LittleEndian.Uint16(targets.Records[2][50:]))
		p.Flight.Combat.Secondary.DisplayKind = int(binary.LittleEndian.Uint16(targets.Records[3][32:]))
		p.Flight.Combat.Secondary.Required = int16(binary.LittleEndian.Uint16(targets.Records[3][50:]))
	}
	unsupported := func(reason string) error { s.ResultUnavailable = reason; return nil }
	if c.Office.Assets.Debrief == nil {
		return unsupported("prepared player debrief resources unavailable")
	}
	// Authored Simulation uses ordinary result ownership; isolated Replay Resume
	// never reaches this Store-capable entry (initializeSortieCapability).
	// Unknown imported career fields are a result capability boundary, not
	// a reason to remove the already supported preparation/flight path.
	pilot := c.Store.State().SelectedPilot()
	if pilot == nil || pilot.Statistics == nil || len(pilot.Statistics.AwardCounts) != 8 || len(pilot.LegacyRaw) != gamestate.LegacyRecordSize {
		return unsupported("complete retained pilot progression fields are unavailable")
	}
	if len(final.Aircraft) < 1 || len(final.Aircraft) > 5 || len(p.Initialized.Stores.Ships) != len(final.Aircraft) || p.Initialized.PlayerRow != 0 || len(p.Initialized.Flight) != len(final.Aircraft) {
		return unsupported("complete fresh player/crew actor prefix is required")
	}
	if len(final.Mission.Objects) != 80 || len(s.InitialAircraftIDs) != len(final.Aircraft) {
		return unsupported("complete80-object and original aircraft-selector snapshots required")
	}
	training := final.Mission.Inputs.MissionType == gamestate.MissionTraining
	safeTraining := p.r5Mechanics && training && p.Initialized.Switches&0x400 != 0 && s.CampaignDraft == nil
	if training && (s.CampaignDraft != nil || pilot.Campaign != nil) && !safeTraining {
		return unsupported("Training with retained active Campaign crossmode ownership is excluded")
	}
	if pilot.Campaign != nil && s.CampaignDraft == nil && !safeTraining {
		// GS's cache writer tests retained stage, not just mission type4.
		// The ordinary chooser prevents this crossmode; do not silently ignore
		// its cache owner if an externally supplied pilot nevertheless has it.
		return unsupported("retained Campaign without an accepted launch draft has unresolved crossmode ownership")
	}
	if targetErr != nil {
		return unsupported(targetErr.Error())
	}
	tables := resultTables(c.Office.Assets.Debrief)
	if err := tables.Validate(); err != nil {
		return unsupported(err.Error())
	}
	coefficient, _, err := preflightDifficulty(c.Office.Assets.Missions, final.Mission, final.Settings)
	if err != nil {
		return unsupported(err.Error())
	}
	accepted := make([]byte, len(final.Aircraft))
	for i, row := range final.Aircraft {
		accepted[i] = row[0]
	}
	// Includes routes and retained import provenance, not only visible objects.
	missionHash, err := resultDigest(final.Mission)
	if err != nil {
		return err
	}
	launchHash, err := resultDigest(struct {
		Aircraft    [][36]byte
		Settings    gamestate.PreflightSettings
		Coefficient int16
		Recommended []byte
		Switches    uint16
	}{final.Aircraft, final.Settings, coefficient, s.InitialAircraftIDs, p.Initialized.Switches})
	if err != nil {
		return err
	}
	policy := ""
	if p.r5Mechanics {
		policy = gamestate.SafeGameplayPolicy
	}
	start, err := c.Store.BeginSortie(gamestate.SortieStart{RetireByMissions: p.independentManeuvers, SourceProfile: assetdata.IslandIce469085.Name, MissionDigest: missionHash, LaunchDigest: launchHash, ClockPolicy: flight.NativeClockPolicy, GameplayPolicy: policy})
	if err != nil {
		return err
	}
	s.ResultLaunch = &sortieLaunch{Start: *start, Tables: tables, Coefficient: coefficient, Recommended: append([]byte(nil), s.InitialAircraftIDs...), Accepted: accepted,
		Kinds: [2]int16{int16(final.Mission.Objectives["primary"].DisplayKind), int16(final.Mission.Objectives["secondary"].DisplayKind)}}
	if p.filmRecorder != nil && p.filmRecorder.Spool != nil {
		p.filmRecorder.Spool.SetSessionID(start.ID)
	}
	s.ResultLaunch.LaunchPose = p.Initialized.Player
	s.ResultLaunch.Targets = p.ResultTargets.Records
	if safeTraining && start.PilotBefore.Campaign != nil {
		s.ResultLaunch.RetainedCampaignSHA256, err = resultDigest(start.PilotBefore.Campaign)
		if err != nil {
			return err
		}
	}
	if s.CampaignDraft != nil {
		raw, err := json.Marshal(s.CampaignDraft)
		if err != nil {
			return err
		}
		var draft gamestate.CampaignState
		if err := json.Unmarshal(raw, &draft); err != nil {
			return err
		}
		s.ResultLaunch.CampaignDraft = &draft
	}
	c.rememberSortieFuel()
	return nil
}

// Keep the last mutually confirmed fuel reading on the launch that owns it.
// This also captures service and the same-update fuel damage writer.
func (c *FrontEnd) rememberSortieFuel() {
	if c.Mission == nil || c.Mission.ResultLaunch == nil || c.Rendering == nil {
		return
	}
	l, p := c.Mission.ResultLaunch, c.Rendering
	m, r := p.Initialized, p.Flight
	if m == nil || r == nil || m.Stores == nil {
		return
	}
	for slot, row := range m.FlightRows {
		if slot >= len(l.fuel) || row < 0 || row >= len(m.Entities) || slot >= len(m.Stores.Ships) || slot >= len(r.Systems.Fuel) {
			continue
		}
		e, ship := m.Entities[row], m.Stores.Ships[slot]
		if e.RuntimeRow == row && e.FlightSlot == slot && ship.Fuel == r.Systems.Fuel[slot] {
			l.fuel[slot] = fuelReceipt{actor: e.ID, step: r.Steps, phase: r.ElapsedDA, value: ship.Fuel}
		}
	}
}

func (c *FrontEnd) confirmedFuel(slot int) (int16, bool) {
	l, p := c.Mission.ResultLaunch, c.Rendering
	m, r := p.Initialized, p.Flight
	if slot < 0 || slot >= len(l.fuel) || slot >= len(l.Accepted) || slot >= len(m.FlightRows) || m.Stores == nil || slot >= len(m.Stores.Ships) {
		return 0, false
	}
	row := m.FlightRows[slot]
	if row < 0 || row >= len(m.Entities) {
		return 0, false
	}
	e, ship, receipt := m.Entities[row], m.Stores.Ships[slot], l.fuel[slot]
	ok := e.RuntimeRow == row && e.FlightSlot == slot && e.ID != "" && ship.AircraftID == int(l.Accepted[slot]) && receipt.actor == e.ID && receipt.step == r.Steps && receipt.phase == r.ElapsedDA && receipt.value == ship.Fuel
	return receipt.value, ok
}

// Complete only independently confirmed missing terminal words before sealing
// the final film checkpoint. The diagnostic context retains the original rows.
func (c *FrontEnd) recoverEndingFuel() bool {
	l, p := c.Mission.ResultLaunch, c.Rendering
	if c.filmIsolation != nil || l == nil || len(p.Flight.Systems.Fuel) >= len(l.Accepted) || len(l.Accepted) > 5 {
		return false
	}
	c.rememberResultEvidence()
	fuel := append([]int16(nil), p.Flight.Systems.Fuel...)
	for i := len(fuel); i < len(l.Accepted); i++ {
		v, ok := c.confirmedFuel(i)
		if !ok {
			if f := p.filmRecorder; f != nil {
				f.failRecording("terminal fuel", fmt.Errorf("unverified fuel for aircraft %d; preserving recording prefix", i))
				c.filmError = f.Error
			}
			return true
		}
		fuel = append(fuel, v)
	}
	c.qualifyResult(fmt.Sprintf("Missing terminal fuel rows %d..%d. Recovered synchronized fuel %v at step %d.", len(p.Flight.Systems.Fuel), len(fuel)-1, fuel[len(p.Flight.Systems.Fuel):], p.Flight.Steps))
	p.Flight.Systems.Fuel = fuel
	return true
}

func (c *FrontEnd) qualifyResult(detail string) {
	if c.resultWarning != "" {
		c.resultWarning += "; "
	}
	c.resultWarning += detail
	diagnostics.Warn("career", "Qualified mission result", "detail", detail)
}

func (c *FrontEnd) freezeSortieFacts() (gamestate.SortieFacts, error) {
	s, p := c.Mission, c.Rendering
	if s == nil || s.Data == nil || p == nil || p.Flight == nil || s.ResultLaunch == nil || p.ResultTargets == nil {
		return gamestate.SortieFacts{}, fmt.Errorf("sortie has no admitted immutable launch state")
	}
	l, r, m := s.ResultLaunch, p.Flight, p.Initialized
	if m == nil || m.Stores == nil || r.Combat == nil || len(m.Entities) < 80 || m.PlayerRow < 0 || m.PlayerRow >= len(m.Entities) || len(l.Accepted) != 1 && len(l.Accepted) != 5 || len(m.Aircraft) != len(l.Accepted) {
		return gamestate.SortieFacts{}, fmt.Errorf("incomplete aircraft, stores, fuel or world facts at mission end")
	}
	if r.Failure != "" {
		return gamestate.SortieFacts{}, fmt.Errorf("unsupported simulation state: %s", r.Failure)
	}
	if p.corridorNavigation {
		for row := 1; row < 5; row++ {
			if r.Systems.UnsupportedFollowerEmptyBurn&(1<<row) == 0 && (m.FollowerLimits[row] == "" || r.Combat.OnlyWaypointDiagnostic(row)) {
				continue
			}
			reason := m.FollowerLimits[row]
			if reason == "" {
				reason = "fuel exhaustion lacks a validated failure transition"
			}
			return gamestate.SortieFacts{}, fmt.Errorf("aircraft %d outcome unresolved: %s", row, reason)
		}
	} else if r.Systems.UnsupportedFollowerEmptyBurn != 0 {
		// New formation guards can precede depletion. Keep historical films' exact
		// interpretation while naming each reached missing owner for new play.
		if r.Combat != nil && r.Combat.FormationEnabled() {
			detail := ""
			for row := 1; row <= 4; row++ {
				if r.Systems.UnsupportedFollowerEmptyBurn&(1<<uint(row)) != 0 && m.FollowerLimits[row] != "" {
					detail += fmt.Sprintf("; actor%d: %s", row, m.FollowerLimits[row])
				}
			}
			return gamestate.SortieFacts{}, fmt.Errorf("nonplayer successor is unsupported (ordinary actor mask%02x%s); no healthy return or loss was invented", r.Systems.UnsupportedFollowerEmptyBurn, detail)
		}
		return gamestate.SortieFacts{}, fmt.Errorf("nonplayer fuel-empty successor is unsupported (ordinary actor mask%02x); no healthy return or loss was invented", r.Systems.UnsupportedFollowerEmptyBurn)
	}
	if m.Sortie == nil || m.Sortie.Result == nil {
		return gamestate.SortieFacts{}, fmt.Errorf("sortie has not reached an ending boundary")
	}
	if p.ResultTargets.ProgressFailure != "" {
		return gamestate.SortieFacts{}, fmt.Errorf("%s", p.ResultTargets.ProgressFailure)
	}
	end := m.Sortie.Result
	reason := end.Reason
	flags := end.SourceStatus
	if reason == "Alt+E" {
		reason = "voluntary"
		flags &^= 0x80
	}
	if reason == "base" && (s.Data.Scene == 2 || s.Data.Scene == 5) {
		reason = "carrier"
	}
	returned := func(row int) (byte, bool) {
		if row == m.PlayerRow {
			return flags, true
		}
		return 0, false
	}
	if m.ObjectiveRecovered(0, returned) {
		flags |= 1
	}
	if m.ObjectiveRecovered(1, returned) {
		flags |= 2
	}
	f := gamestate.SortieFacts{SoloBuilder: s.Data.SoloBuilder, SortieID: l.Start.ID, SourceProfile: assetdata.IslandIce469085.Name, MissionType: s.Data.Inputs.MissionType,
		ActorCount: len(m.Aircraft), SourceMode: 0, Reason: reason, SourceFlags: flags, ElapsedDA: r.ElapsedDA,
		Coefficient: l.Coefficient, SelectorMultisetMatch: gamestate.SortieSelectorMultisetMatch(l.Recommended, l.Accepted),
		RecommendedSelectors: append([]byte(nil), l.Recommended...), AcceptedSelectors: append([]byte(nil), l.Accepted...),
		LaunchSwitches: m.Switches, ObjectCount: 80, ObjectCredits: r.Combat.Credit, LaunchObjectiveKinds: l.Kinds,
		InactiveHeaderPolicy: gamestate.SortieFreshInactiveHeaders, PlayerTerminal: r.State.Terminal != nil || r.Systems.Damage[9] != 0}
	f.FinalPose = m.Entities[m.PlayerRow].Pose
	f.FinalPose.H0, f.FinalPose.H1, f.FinalPose.Height = end.Player.H0, end.Player.H1, uint16(end.Player.Height)
	f.FinalPose.Angles = end.Player.Angles

	for i := range p.ResultTargets.Records {
		f.Targets = append(f.Targets, p.ResultTargets.Records[i])
	}
	if len(m.Entities) < 80 {
		return f, fmt.Errorf("final object pool is incomplete")
	}
	for i := 0; i < 80; i++ {
		f.ObjectValues[i] = int16(binary.LittleEndian.Uint16(m.Entities[i].Object.Raw[27:29]))
		f.FinalObjects[i] = m.Entities[i].Object.Raw
	}
	air := int(l.Accepted[0])
	definition := c.Office.Assets.Missions.AircraftDefinitions[air]
	if len(definition) != 252 {
		return f, fmt.Errorf("final aircraft score definition missing")
	}
	f.PlayerAircraftValue = int8(definition[6])
	if f.ActorCount > 1 {
		f.InactiveHeaderPolicy = gamestate.SortieAcceptedFlightHeaders
	}
	for i := 1; i < len(l.Accepted); i++ {
		d := c.Office.Assets.Missions.AircraftDefinitions[int(l.Accepted[i])]
		if len(d) != 252 {
			return f, fmt.Errorf("follower%d aircraft score definition missing", i)
		}
		f.InactiveAircraftValues[i-1] = int8(d[6])
	}
	if err := c.freezeResultContext(&f); err != nil {
		return f, err
	}
	if len(l.Recommended) < len(l.Accepted) {
		// Neither an incomplete recommendation nor its absence establishes the
		// selector-match bonus. Accepted aircraft and all other factors stay valid.
		if len(s.InitialAircraftIDs) == len(l.Accepted) {
			f.RecommendedSelectors = append([]byte(nil), s.InitialAircraftIDs...)
			f.SelectorMultisetMatch = gamestate.SortieSelectorMultisetMatch(f.RecommendedSelectors, l.Accepted)
			c.qualifyResult("Recovered missing recommended aircraft from this mission's retained briefing.")
		} else {
			f.ResultContext.RecommendationUnknown = true
			f.RecommendedSelectors = []byte{}
			f.SelectorMultisetMatch = false
			c.qualifyResult("Aircraft recommendation incomplete. Omitted the unverified selector-match bonus.")
		}
	}
	if err := gamestate.FinalizeSortieFacts(&f); err != nil {
		return f, err
	}
	if p.p4Mechanics {
		if err := gamestate.FinalizeAssignedObjectives(&f); err != nil {
			return f, err
		}
	}
	// Cargo recovery is owned by the assigned actor's actual final return flags.
	returnedAll := func(row int) (byte, bool) {
		for _, a := range f.ResultContext.Actors {
			if m.FlightRows[a.Actor] == row {
				return a.Flags, true
			}
		}
		return 0, false
	}
	if m.ObjectiveRecovered(0, returnedAll) {
		f.SourceFlags |= 1
	}
	if m.ObjectiveRecovered(1, returnedAll) {
		f.SourceFlags |= 2
	}
	f.ResultContext.Actors[0].Flags = f.SourceFlags
	return f, f.Validate()
}

// This is the termination barrier. No queued ticks/projectiles/events can run
// while the immutable proposal or its UI awaits confirmation/restart.
func (c *FrontEnd) finishSortieResult() error {
	// A recorded step stops at the mechanical boundary. Its separately recorded
	// ending transaction owns clear/finalize; do not apply it early in playback.
	if c.filmIsolation != nil && c.filmIsolation.Mode == "playback" && c.Rendering != nil && c.Rendering.filmPlayback != nil && c.Rendering.filmPlayback.current != nil {
		return nil
	}
	// A delayed ending callback cannot reopen/abandon a frozen proposal or an
	// already acknowledged office. Only the live scene/its confirmation owns it.
	if c.Screen != ScreenScene && c.Screen != ScreenQuit {
		return nil
	}
	if c.Rendering == nil || c.Mission == nil {
		return nil
	}
	p := c.Rendering
	if c.filmIsolation == nil && (p.Initialized == nil || p.Flight == nil || p.Flight.State == nil || p.Flight.Combat == nil) {
		reason := fmt.Errorf("reached ending lacks initialized aircraft, flight or combat facts")
		if f := p.filmRecorder; f != nil {
			f.failRecording("missing ending facts", reason)
			c.filmError = f.Error
		}
		id := ""
		if l := c.Mission.ResultLaunch; l != nil {
			id = l.Start.ID
		}
		c.rememberResultEvidence()
		return c.rejectSortieResult(reason, id)
	}
	if p.Flight != nil && p.Flight.Combat != nil {
		p.Flight.Combat.FlushDiagnostics()
	}
	fuelEvidence := c.recoverEndingFuel()
	c.filmFinishStep()
	p.clearFlightInput()
	if p.Flight != nil {
		p.Flight.Clock.ResetElapsed()
		p.Flight.Paused = true
	}
	p.Paused = true
	if m := p.Initialized; m != nil && m.Sortie != nil && m.Sortie.Result != nil && p.Flight != nil {
		end := m.Sortie.Result
		end.PlayerDamage = gamestate.PlayerDamage12(&p.Flight.Systems)
		end.EncodedHealth = gamestate.EncodedDamageHealth(end.PlayerDamage)
		end.DamageKnown = true
	}
	if p.Initialized != nil {
		p.Initialized.FinalizePlayerObjectives()
	}
	c.filmFinishEnding()
	if !fuelEvidence {
		c.rememberResultEvidence()
	} else if c.filmError != "" {
		c.qualifyResult(c.filmError)
	}
	if c.filmIsolation != nil {
		return nil
	}
	c.sceneEndConfirmation = false
	c.fade = nil
	if c.Mission.ResultLaunch == nil {
		why := c.Mission.ResultUnavailable
		return c.rejectSortieResult(fmt.Errorf("%s", why), "")
	}
	c.endingSceneAudio = append(c.endingSceneAudio, p.takeSceneAudio()...)
	c.Screen = ScreenDebrief // Also freezes the scene if a staging I/O error needs restart.
	facts, err := c.freezeSortieFacts()
	if err != nil {
		id := c.Mission.ResultLaunch.Start.ID
		return c.rejectSortieResult(err, id)
	}
	proposal, err := gamestate.ProposeSortie(c.Mission.ResultLaunch.Start.PilotBefore, facts, c.Mission.ResultLaunch.Tables)
	if err != nil {
		return c.rejectSortieResult(err, facts.SortieID)
	}
	diagnostics.RememberWarning(facts.SortieID, c.resultWarning)
	if err = c.Store.StageSortieResult(facts.SortieID, proposal); err != nil {
		diagnostics.Report(facts.SortieID+"/stage", err.Error(), "Your mission result could not be saved.")
		_ = c.failPreparationSave("result staging", err)
		return nil
	}
	if c.resultWarning != "" {
		diagnostics.ResultAwaitingAcknowledgment()
	}
	diagnostics.Phase("debrief awaiting acknowledgment")
	diagnostics.Info("career", "Result staged; final acknowledgment required")
	return c.enterSortieDebrief()
}
