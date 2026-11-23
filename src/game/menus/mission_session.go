package menus

import (
	"encoding/binary"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/diagnostics"
	gamestate "gunship2000-assets/game/state"
	"log/slog"
	"time"
)

type MissionSession struct {
	gps                preflightGPS
	AuthoredSource     *gamestate.AuthoredDocument
	Data               *gamestate.MissionData
	View               *BriefingView
	PilotID            string
	Page               int
	Settings           gamestate.PreflightSettings
	SettingsError      error
	SettingsChanged    bool
	ReadinessRows      [][36]byte
	InitialAircraftIDs []byte
	ReadinessError     error
	Flight             []gamestate.FlightPilot
	Outfit             *OutfitView
	Ordnance           *OrdnanceView
	Cockpit            CockpitState
	settingsHover      int // Source prior AREA for the exceptional APU leave cue.
	SettingsHelp       SettingsHelpState
	SecondaryControl   byte
	Initialized        *gamestate.InitializedMission
	Final              *FinalPreflightConfiguration
	ResultLaunch       *sortieLaunch
	ResultUnavailable  string
	CampaignDraft      *gamestate.CampaignState
	interstitialTicks  int
	PaperTransition    *paperTransition
	PreflightWarning   string // Gameplay validation, not a native diagnostic.
}

func isMissionScreen(s Screen) bool {
	return s == ScreenInterstitial || s == ScreenBriefing || s == ScreenPapers || s == ScreenSettings || s == ScreenPaperTransition || s == ScreenOutfit || s == ScreenOrdnance || s == ScreenSceneInitialization
}

func (c *FrontEnd) startMission(seed int64, stream []int) error {
	p := c.Store.State().SelectedPilot()
	if p == nil || p.Status != gamestate.StatusActive || !c.viewingPostedPilot() {
		c.emitAudioCue(gs4Denied)
		c.pointerCue = &[2]int{184, 70}
		c.Office.SetPointer(184, 70, true)
		return nil
	}
	// O0064..00ab selects the door cue from the original active/theatre guard.
	// Port-only missing-asset/editor diagnostics retain their existing order.
	if p.MissionType == gamestate.MissionTraining && p.Theatre.Raw() > 1 {
		c.emitAudioCue(gs4Denied)
	} else {
		c.emitAudioCue(gs4Door)
	}
	if c.Office.Assets.Missions == nil {
		c.reportDiagnostic("preparation", "Briefing assets are not prepared.", nil)
		return nil
	}
	if p.MissionType == gamestate.MissionSimulated {
		return c.openMissionFiles()
	}
	if p.MissionType == gamestate.MissionTraining && p.Theatre.Raw() > 1 {
		c.reportStatus("preparation", "Training requires Europe or Persian Gulf.")
		return nil
	}
	s, err := prepareMissionSession(c.Office.Assets.Missions, p, seed, stream, c.retainedMissionObject, c.safeGameplayProfile(), c.r6Profile())
	if err != nil {
		c.reportDiagnostic("preparation", "Mission preparation is unavailable. See terminal.", nil)
		return err
	}
	c.publishMission(s)
	return nil
}

// Build a candidate without changing the controller or save. Campaign decline
// must finish both preparation and persistence before publishing this session.
func prepareMissionSession(assets *assetdata.MissionAssets, p *gamestate.Pilot, seed int64, stream []int, retained []byte, safeGameplay ...bool) (*MissionSession, error) {
	previous, known := p.LegacyByte(0x5b)
	if p.MissionType != gamestate.MissionTraining && p.MissionType != gamestate.MissionCampaign && !known {
		return nil, fmt.Errorf("previous scenario state is unavailable")
	}
	in := gamestate.MissionInputs{MissionType: p.MissionType, Theatre: p.Theatre, Rank: p.Rank, PilotName: p.Name, Seed: seed, DrawStream: stream, PreviousScenario: int(int8(previous)), RetainedObject: retained}
	safe := len(safeGameplay) != 0 && safeGameplay[0]
	if safe && p.MissionType == gamestate.MissionTraining {
		// Each Training entry owns a fresh preparation epoch. The retained
		// Campaign's rows, metadata and suspension are not scratch inputs.
		in.RetainedObject = nil
	}
	var data *gamestate.MissionData
	var draft *gamestate.CampaignState
	var err error
	if p.MissionType == gamestate.MissionCampaign {
		data, draft, err = gamestate.CampaignStart(assets, p, seed, stream, retained, safe, len(safeGameplay) > 1 && safeGameplay[1])
	} else {
		data, err = gamestate.GenerateMission(assets.Generation, in)
	}
	if err != nil {
		return nil, err
	}
	view, err := NewBriefingView(assets, data, p, seed^0x61c8864680b583eb)
	if err != nil {
		return nil, err
	}
	s := &MissionSession{Data: data, View: view, PilotID: p.ID, interstitialTicks: 35, CampaignDraft: draft}
	s.Settings, s.SettingsError = gamestate.PreflightFromPilot(p)
	s.ReadinessRows, s.ReadinessError = gamestate.PrepareReadinessRows(assets, data)
	for _, r := range s.ReadinessRows {
		s.InitialAircraftIDs = append(s.InitialAircraftIDs, r[0])
	}
	s.Flight, err = gamestate.InitFlightPilots(p, data)
	if err != nil {
		return nil, err
	}
	if err = gamestate.RefreshFlightPresentation(s.Flight, assets, data); err != nil {
		return nil, err
	}
	if err = restoreCampaignPreparation(s, draft); err != nil {
		return nil, err
	}
	if err = gamestate.RefreshFlightPresentation(s.Flight, assets, data); err != nil {
		return nil, err
	}
	return s, nil
}

func (c *FrontEnd) publishMission(s *MissionSession) {
	from := c.fadeSource()
	c.Mission = s
	c.Screen = ScreenInterstitial
	c.BeginSceneFade(from)
	c.Highlight = -1
	c.Message = ""
}

func (c *FrontEnd) nextMissionSeed() int64 {
	seed := time.Now().UnixNano()
	if c.MissionSeed != nil {
		seed = *c.MissionSeed
	}
	return seed
}

func (c *FrontEnd) enterMission() error {
	return c.startMission(c.nextMissionSeed(), c.MissionDrawStream)
}

func (c *FrontEnd) tickMission() {
	c.Office.advancePointer()
	if c.Screen == ScreenPaperTransition {
		c.tickPaperTransition()
		return
	}
	if c.Screen == ScreenInterstitial {
		c.Mission.interstitialTicks--
		if c.Mission.interstitialTicks <= 0 {
			from := c.fadeSource()
			c.Screen = ScreenBriefing
			c.BeginSceneFade(from)
		}
	} else if c.Screen == ScreenBriefing {
		c.Mission.View.Tick()
	} else if c.Screen == ScreenOutfit {
		c.Mission.Outfit.Tick()
	} else if c.Screen == ScreenSettings {
		s := c.Mission
		s.SettingsHelp.Tick()
		if err := s.Cockpit.Tick(c.Office.Assets.Missions, s.Data, s.Settings); err != nil {
			c.reportDiagnostic("preparation", err.Error(), err)
		}
		if s.Cockpit.Ready && s.Final == nil && c.gpsAllowsStart() {
			final, err := finalPreflight(s)
			if err != nil {
				if !c.showPreflightRejection(err) {
					c.reportDiagnostic("preparation", err.Error(), err)
				}
			} else {
				s.PreflightWarning = ""
				s.Final = final
				if err := c.beginRenderingInitialization(final); err != nil {
					c.Message = "Scene preview unavailable. See terminal."
					// Keep the accepted preparation intact; no simulation or save
					// boundary is crossed when a visual resource is unsupported.
					c.noteDiagnosticStatus(slog.LevelWarn, "Scene preview initialization rejected", err, "accepted preparation retained; simulation not entered")
				}
			}
		}
	}
}

func (c *FrontEnd) missionHit(x, y int) int {
	if c.Mission == nil {
		return -1
	}
	switch c.Screen {
	case ScreenBriefing:
		return c.Mission.View.Hit(x, y)
	case ScreenPapers:
		return c.Mission.View.PaperHit(c.Mission.Page, x, y)
	case ScreenSettings:
		return settingsHit(c.Office.Assets.Missions, x, y)
	case ScreenOutfit:
		if c.Mission.Outfit != nil {
			return c.Mission.Outfit.Hit(x, y)
		}
	case ScreenOrdnance:
		if c.Mission.Ordnance != nil {
			return c.Mission.Ordnance.Hit(x, y)
		}
	}
	return -1
}

func (c *FrontEnd) handleMission(a Actions) error {
	s := c.Mission
	if s == nil {
		return fmt.Errorf("mission screen has no session")
	}
	if s.PaperTransition != nil {
		return nil // Input edges are consumed while moving; global quit is handled first.
	}
	if c.Screen == ScreenOutfit || c.Screen == ScreenOrdnance {
		return c.handleLoadout(a)
	}
	hit := -1
	if a.Inside {
		hit = c.missionHit(a.X, a.Y)
	}
	leaveAPU := c.Screen == ScreenSettings && s.settingsHover == 9 && hit != 9 && s.Cockpit.APU != 0
	if c.Screen == ScreenSettings {
		s.SettingsHelp.Observe(s.settingsHover, hit, a.Selector1 && !a.Cancel && hit > 0)
		s.settingsHover = hit
	}
	if a.Cancel {
		switch c.Screen {
		case ScreenPapers:
			c.emitAudioCue(gs4Accepted)
			c.beginPaperTransition(paperClosing)
		case ScreenSettings:
			// C024c copies choices back even on Escape. The port also persists
			// this commit immediately, rather than waiting for a later DOS save.
			if s.SettingsChanged {
				next := c.Store.State()
				p := next.SelectedPilot()
				if p == nil || p.ID != s.PilotID {
					return fmt.Errorf("selected pilot changed while editing settings")
				}
				if err := gamestate.CopyPilotPreflight(p, s.Settings); err != nil {
					return err
				}
				if err := c.Store.Commit(next); err != nil {
					return c.failPreparationSave("settings", err)
				}
				s.SettingsChanged = false
			}
			c.Screen = ScreenBriefing
			s.SecondaryControl = s.Cockpit.Knobs[1]
			s.View.ResumeScene()
			c.Message = ""
		}
		return nil // Tent Escape has no direct office branch.
	}
	if c.Screen == ScreenSettings && hit == 8 {
		c.handleGPSInput(a)
	}
	if !a.Selector1 {
		if leaveAPU {
			c.emitAudioCue(gs4Computer)
		}
		return nil
	}
	switch c.Screen {
	case ScreenBriefing:
		switch hit {
		case 1:
			c.emitAudioCue(gs4Accepted)
			return c.openOutfit()
		case 2:
			c.emitAudioCue(gs4Accepted)
			c.beginPaperTransition(paperOpening)
		case 3:
			if s.SettingsError != nil {
				c.emitAudioCue(gs4Denied)
				c.reportDiagnostic("preparation", "Switch preferences are unavailable. See terminal.", nil)
				return s.SettingsError
			}
			if s.ReadinessError != nil {
				c.emitAudioCue(gs4Denied)
				c.reportDiagnostic("preparation", "Readiness data is unavailable. See terminal.", nil)
				return s.ReadinessError
			}
			if err := gamestate.PreflightReadiness(s.ReadinessRows); err != nil {
				c.emitAudioCue(gs4Denied)
				c.Message = briefingReadinessWarning(s.ReadinessRows)
				return nil
			}
			c.emitAudioCue(gs4Accepted)
			c.Screen = ScreenSettings
			s.PreflightWarning = ""
			s.Cockpit.Enter()
			c.enterPreflightGPS()
			s.SettingsHelp = SettingsHelpState{}
			s.settingsHover = -1
			s.Final = nil
			c.Message = ""
		default:
			c.emitAudioCue(gs4Denied)
		}
	case ScreenPapers:
		switch hit {
		case 3:
			c.emitAudioCue(gs4Page)
			c.beginPaperTransition(paperNext)
		case 4:
			c.emitAudioCue(gs4Accepted)
			c.beginPaperTransition(paperClosing)
		case 5:
			return c.declineMission()
		case 6:
			return c.campaignResult("suspend")
		case 7:
			return c.campaignResult("abort")
		}
	case ScreenSettings:
		if hit >= 1 && hit <= 7 {
			if hit <= 2 {
				c.emitAudioCue(gs4Switch)
			} else {
				c.emitAudioCue(gs4Computer)
			}
			key := hit - 1
			values := s.Settings.Values()
			count := gamestate.PreflightChoiceCounts[key]
			s.Settings.Set(key, (values[key]+1)%count)
			s.SettingsChanged = true
			c.Message = ""
			s.Final = nil
		} else if hit == 9 {
			if !c.gpsAllowsStart() {
				message := "APU locked. Authenticate at the GPS console."
				if s.gps.choice.Challenge == "" {
					message = "APU locked. Check gps_codes.json or turn protection Off."
				}
				c.gpsReject(message)
				return nil
			}
			c.emitAudioCue(gs4Computer)
			s.Cockpit.APU = (s.Cockpit.APU + 1) % 3
			s.Cockpit.Ready = false
			s.Cockpit.readyTicks = 0
			s.Final = nil
			c.Message = ""
		} else if hit == 10 || hit == 11 {
			c.emitAudioCue(gs4Knob)
			s.Cockpit.Knobs[hit-10] ^= 1
			c.Message = ""
		}
	}
	if leaveAPU {
		// C0627 is an explicit hover-change exception: it follows any new
		// control's own cue, rather than adding generic hover sounds.
		c.emitAudioCue(gs4Computer)
	}
	return nil
}

func (c *FrontEnd) declineMission() error {
	s := c.Mission
	if s == nil {
		return fmt.Errorf("no mission to decline")
	}
	next := c.Store.State()
	p := next.SelectedPilot()
	if p == nil || p.ID != s.PilotID {
		return fmt.Errorf("selected pilot changed during mission preparation")
	}
	if s.Data.Inputs.MissionType == gamestate.MissionCampaign {
		return c.campaignResult("decline")
	}
	if s.Data.Inputs.MissionType != gamestate.MissionTraining {
		if len(p.LegacyRaw) != gamestate.LegacyRecordSize {
			return fmt.Errorf("decline needs the preserved opaque source counter; no state was saved")
		}
		// A1e3e5 is a conditional signed16 addition, not a clamp or a named
		// progression statistic. Keep the original opaque storage distinction.
		value := int16(binary.LittleEndian.Uint16(p.LegacyRaw[0x59:]))
		if value < 3000 {
			value += 300
		}
		binary.LittleEndian.PutUint16(p.LegacyRaw[0x59:], uint16(value))
		p.LegacyRaw[0x5c] = p.LegacyRaw[0x5b]
		p.LegacyRaw[0x5b] = byte(s.Data.SourceWorld.Suffix)
	}
	if len(p.LegacyRaw) == gamestate.LegacyRecordSize {
		p.LegacyRaw[0x58] = s.Data.SourceWorld.SNHD[0]
	}
	if s.SettingsChanged {
		copy := s.Settings
		p.Preflight = &copy
	}
	if err := copyPreparationPreferences(p, s); err != nil {
		return err
	}
	if err := c.Store.Commit(next); err != nil {
		return err
	}
	if len(s.Data.Objects) > 0 {
		c.retainedMissionObject = append([]byte(nil), s.Data.Objects[0].Raw[:]...)
	}
	return c.RefreshOffice()
}

func (c *FrontEnd) missionFrame() []byte {
	s := c.Mission
	hit := -1
	if c.pointerInside {
		hit = c.missionHit(c.pointerX, c.pointerY)
	}
	if c.keyboardHotspot == c.Screen {
		hit = c.missionHit(c.Office.PointerX, c.Office.PointerY)
	}
	switch c.Screen {
	case ScreenPaperTransition:
		return c.paperTransitionFrame()
	case ScreenInterstitial:
		return s.View.InterstitialFrame()
	case ScreenBriefing:
		frame := s.View.FrameForHit(hit)
		// Original 1a79:2030 AREA-entry callback paints the rejection on hover3;
		// recomposition must retain it while the pointer remains stationary.
		if hit == 3 {
			c.drawPreflightWarning(frame, briefingReadinessWarning(s.ReadinessRows))
		}
		return frame
	case ScreenPapers:
		return s.View.PaperFrameForHit(s.Page, hit)
	case ScreenSettings:
		frame := renderCockpitWithHelp(c.Office.Assets.Missions, s.Settings, s.Cockpit, hit, s.SettingsHelp)
		c.drawRadioLCD(frame)
		if hit == 8 {
			c.drawGPSMonitor(frame)
		}
		c.drawPreflightWarning(frame, s.PreflightWarning)
		return frame
	case ScreenOutfit:
		return s.Outfit.Frame()
	case ScreenOrdnance:
		return s.Ordnance.Frame()
	}
	return make([]byte, 64000)
}

// Preferences and stored wingmen copy back at outer result/save boundaries,
// never on a local OUTFIT/ORDNANCE return or animation tick.
func copyPreparationPreferences(p *gamestate.Pilot, s *MissionSession) error {
	if s.SettingsChanged {
		if err := gamestate.CopyPilotPreflight(p, s.Settings); err != nil {
			return err
		}
	}
	if len(s.Flight) == 5 {
		wingmen, err := gamestate.SquadronFromFlight(s.Flight)
		if err != nil {
			return err
		}
		p.SquadronPilots = wingmen
	}
	return nil
}

func (c *FrontEnd) campaignResult(result string) error {
	s := c.Mission
	if s == nil {
		return fmt.Errorf("no Campaign preparation")
	}
	next := c.Store.State()
	p := next.SelectedPilot()
	if p == nil || p.ID != s.PilotID {
		return fmt.Errorf("selected pilot changed during Campaign preparation")
	}
	var err error
	switch result {
	case "suspend":
		err = campaignSuspend(p, s)
	case "abort":
		err = campaignAbort(p, s)
	case "decline":
		err = campaignDecline(p, s)
	default:
		return fmt.Errorf("unknown Campaign result")
	}
	if err != nil {
		return err
	}
	if err = copyPreparationPreferences(p, s); err != nil {
		return err
	}
	retained := append([]byte(nil), c.retainedMissionObject...)
	if len(s.Data.Objects) > 0 {
		retained = append([]byte(nil), s.Data.Objects[0].Raw[:]...)
	}
	var candidate *MissionSession
	if result == "decline" {
		candidate, err = prepareMissionSession(c.Office.Assets.Missions, p, c.nextMissionSeed(), c.MissionDrawStream, retained, c.safeGameplayProfile(), c.r6Profile())
		if err != nil {
			c.reportDiagnostic("preparation", "Next mission unavailable. Nothing saved; current preparation retained.", nil)
			return fmt.Errorf("Campaign decline not applied: %w", err)
		}
	}
	if err = c.Store.Commit(next); err != nil {
		return c.failPreparationSave("Campaign "+result, err)
	}
	c.retainedMissionObject = retained
	if result == "decline" {
		c.publishMission(candidate)
		return nil
	}
	return c.RefreshOffice()
}

func (c *FrontEnd) failPreparationSave(operation string, err error) error {
	// A journal may already contain part of a multi-file replacement. Do not
	// expose either preparation as actionable until restart reconciles it.
	c.missionSaveError = err
	c.Mission = nil
	c.Screen = ScreenOffice
	c.fade = nil
	c.Message = "Save incomplete. Restart required; preparation closed."
	diagnostics.Error("career", "Persistence boundary failed; restart required", "operation", operation, "status", c.Message, "error", err)
	return fmt.Errorf("%s save incomplete; restart to reconcile saved files: %w", operation, err)
}
