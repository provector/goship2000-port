package menus

import (
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"image"
	"path/filepath"
	"strings"
)

type Screen string

const (
	ScreenOffice              Screen = "office"
	ScreenRoster              Screen = "roster"
	ScreenTheatre             Screen = "theatre"
	ScreenFolder              Screen = "folder_transition"
	ScreenMission             Screen = "mission_type"
	ScreenRecovery            Screen = "recovery"
	ScreenQuit                Screen = "quit_confirmation"
	ScreenStartup             Screen = "startup"
	ScreenInterstitial        Screen = "mission_loading"
	ScreenBriefing            Screen = "briefing"
	ScreenPapers              Screen = "papers"
	ScreenPaperTransition     Screen = "paper_transition"
	ScreenSettings            Screen = "preflight_settings"
	ScreenOutfit              Screen = "outfit"
	ScreenOrdnance            Screen = "ordnance"
	ScreenScene               Screen = "scene_preview"
	ScreenSceneInitialization Screen = "scene_initializing"
	ScreenDebrief             Screen = "debrief"
	ScreenFilms               Screen = "mission_films"
	ScreenReplay              Screen = "mission_replay"
	ScreenMissionFiles        Screen = "mission_files"
	ScreenMissionEditor       Screen = "mission_editor"
)

type RosterDraft struct {
	Replacement bool
	Slot        int
	Pilot       gamestate.Pilot
	Stage       int // name, emblem, unit; only finishing the unit commits the draft.
}

type FrontEnd struct {
	gps              preflightGPSConfig
	SimplifiedFields bool // Host display preference, including during film playback and practice.

	inputTrace                     *flightInputTrace
	RecordingLimit                 configuration.RecordingLimit `json:"-"`
	simulationProfile              string                       // Private validated launch interpretation; empty is new P4.
	enemyLaunchRules               *gamestate.EnemyRules        // Recorded R2.1 definitions; nil for historical films.
	filmIsolation                  *filmSessionIsolation
	filmPending                    *replay.Film
	rejectedCheck                  *rejectedFilmCheck
	filmDiagnostics                *replay.Temporary
	filmError                      string
	FilmDirectory                  string
	MissionDirectory               string
	SummaryDirectory               string // Active writable runtime; optional report is outside save/film journals.
	Files                          *MissionFiles
	Editor                         *MissionEditor
	editorAutoScroll               bool // Source E288: transient process UI owner; never career/document data.
	Library                        *FilmLibrary
	Viewer                         *ReplayViewer
	DevelopmentScenario            *DevelopmentScenario         `json:"-"` // Explicit dev-only setup/staging; nil in normal play.
	Runtime                        configuration.RuntimeOptions `json:"-"`
	audioCues                      []uint16                     // Semantic command queue, drained once by the native audio owner.
	endingSceneAudio               []sceneAudioCommand          // Pre-longjmp GS2 commands, drained once before GS result ownership.
	audioFocusPaused               bool                         // Device presentation ownership; never a gameplay input or saved value.
	Office                         *Office
	Menus                          *assetdata.MenuAssets
	Store                          *gamestate.SaveStore
	Screen                         Screen
	Highlight                      int
	Draft                          *RosterDraft
	rosterWorking                  *gamestate.SaveState
	Choices                        []gamestate.MissionType
	Recovery                       *gamestate.RecoverySnapshot
	ViewedPilotID                  string // Inspection/Phoenix owner; never permission to post or launch.
	ExitRequested                  bool
	Message                        string
	reportedDiagnostics            map[string]string
	recordingEvidenceLogged        [64]string // Operational inventory notices only; never saved or checkpointed.
	recordingEvidenceNext          uint8
	diagnosticStatus               string
	TransitionGroups               []int
	transitionNext, transitionHold int
	previousScreen                 Screen
	quitBackground                 []byte
	pointerX, pointerY             int
	pointerInside                  bool
	pointerCue                     *[2]int
	viewRevision                   uint64
	Startup                        *StartupPlayback
	Mission                        *MissionSession
	Rendering                      *RenderingPreview
	SceneEntry                     *sceneInitialization
	Debrief                        *DebriefView
	sceneEndConfirmation           bool
	MissionSeed                    *int64 `json:"-"`
	MissionDrawStream              []int  `json:"-"`
	retainedMissionObject          []byte
	editorCompilationMemory        *gamestate.AuthoredDocument
	missionPointer                 missionPointerState
	crewPointer                    missionPointerState
	keyboardHotspot                Screen // Office or briefing owns a keyboard-selected AREA.
	keyboardSelection              bool   // arrows own chooser highlight until real pointer activity
	resultWarning                  string
	missionSaveError               error
	fade                           *sceneFade
	FadeMilliseconds               int `json:"-"`
	quitFadeBackground             *image.RGBA
}

func NewFrontEnd(assets *assetdata.OfficeAssets, store *gamestate.SaveStore) (*FrontEnd, error) {
	if assets == nil || assets.Menus == nil || store == nil {
		return nil, fmt.Errorf("front end requires prepared menu assets and a save store")
	}
	c := &FrontEnd{MissionDirectory: filepath.Join(store.Directory, "missions"), FilmDirectory: filepath.Join(store.Directory, "replays"), SummaryDirectory: store.Directory, Menus: assets.Menus, Store: store, Screen: ScreenOffice, Highlight: -1, viewRevision: store.State().Revision}
	state := store.State().OfficeState()
	recovery, err := store.RecoveryCandidate()
	if err != nil {
		c.reportDiagnostic("roster", "Pre-mission backup unavailable: "+err.Error(), err)
	}
	state.SnapshotAvailable = recovery != nil
	c.Office, err = NewOffice(assets, state, 1)
	return c, err
}

// +4fh is retained as an opaque legacy selector, not renamed as campaign progress.
// New records have its established zero initializer; eligibility is independent
// of whether a preparation branch has enough supported inputs to run.
func missionChoices(p *gamestate.Pilot) ([]gamestate.MissionType, error) {
	if p == nil {
		return nil, nil
	}
	if p.Rank.Raw() <= 4 {
		return []gamestate.MissionType{gamestate.MissionTraining, gamestate.MissionSingle}, nil
	}
	value, err := gamestate.CampaignStage(p)
	if err != nil {
		return nil, err
	}
	if value != 0 {
		return []gamestate.MissionType{gamestate.MissionTraining, gamestate.MissionCampaign}, nil
	}
	return []gamestate.MissionType{gamestate.MissionTraining, gamestate.MissionSingle, gamestate.MissionFlight, gamestate.MissionCampaign, gamestate.MissionSimulated}, nil
}

func theatreLocked(p *gamestate.Pilot) (bool, error) {
	if p == nil || p.MissionType != gamestate.MissionCampaign {
		return false, nil
	}
	value, err := gamestate.CampaignStage(p)
	if err != nil {
		return false, err
	}
	return value != 0, nil
}

func (c *FrontEnd) RefreshOffice() error {
	c.AbandonFilmRecording()
	viewed := c.viewedOfficeState()
	state := viewed.OfficeState()
	var recovery *gamestate.RecoverySnapshot
	var err error
	if viewed.SelectedPilot() != nil {
		recovery, err = c.Store.RecoveryCandidateForPilot(viewed.SelectedPilotID)
	}
	backupProblem := err // Optional Phoenix data must not prevent inspecting a valid pilot.
	state.SnapshotAvailable = recovery != nil
	state.ComputerMode = c.Office.State.ComputerMode
	// Rebuild once on return: this clears old names/polygons without disturbing
	// immutable atlases or treating sparse CMPX groups as complete backgrounds.
	next, err := NewOffice(c.Office.Assets, state, 1)
	if err != nil {
		return err
	}
	c.Office = next
	c.viewRevision = c.Store.State().Revision
	c.Screen = ScreenOffice
	c.missionPointer.initialized = false
	c.keyboardSelection = false
	c.Highlight = -1
	c.Draft = nil
	c.rosterWorking = nil
	c.Message = ""
	if backupProblem != nil {
		c.reportDiagnostic("roster", "Pre-mission backup unavailable: "+backupProblem.Error(), backupProblem)
	}
	c.Mission = nil
	c.Rendering = nil
	c.SceneEntry = nil
	c.Debrief = nil
	c.BeginSceneFade(nil)
	if c.pointerCue != nil {
		c.Office.SetPointer(c.pointerCue[0], c.pointerCue[1], c.pointerInside)
	} else {
		c.Office.SetPointer(c.pointerX, c.pointerY, c.pointerInside)
	}
	return nil
}

func (c *FrontEnd) Tick() error {
	if err := c.syncOfficeView(); err != nil {
		return err
	}
	if c.Screen != ScreenQuit && c.fade != nil {
		c.fade.Tick++
		if c.fade.Tick >= c.fade.duration() {
			c.fade = nil
		}
		return nil // No hidden screen interaction or animation advances under a fade.
	}
	switch c.Screen {
	case ScreenMissionFiles, ScreenMissionEditor:
		if c.Screen == ScreenMissionEditor {
			c.advanceEditorStatus()
		}
		c.Office.advancePointer()
		return nil // Editor input/UI never advances flight or proposal clocks.
	case ScreenFilms:
		if c.Library != nil && c.Library.PressedTicks > 0 {
			c.Library.PressedTicks--
		}
		return nil
	case ScreenReplay:
		if c.Viewer != nil && c.Viewer.Practice && c.Viewer.Playback != nil && c.Viewer.Playback.Engine.Screen == ScreenScene {
			return c.Viewer.Playback.Engine.tickGPS()
		}
		return nil
	case ScreenDebrief:
		c.tickDebrief()
	case ScreenScene:
		return c.tickGPS() // Modal GPS input has a UI owner; flight/visual clocks remain paused.
	case ScreenSceneInitialization:
		return nil // Presentation acknowledgement and elapsed time own this stage.
	case ScreenInterstitial, ScreenBriefing, ScreenPapers, ScreenSettings, ScreenPaperTransition, ScreenOutfit, ScreenOrdnance:
		c.tickMission()
	case ScreenStartup:
		return nil // Startup uses elapsed seconds, independently of office ticks.
	case ScreenOffice:
		c.Office.Tick()
	case ScreenFolder:
		c.Office.advancePointer()
		if c.transitionHold > 0 {
			c.transitionHold--
			return nil
		}
		if c.transitionNext >= 32 {
			return c.openMission()
		}
		key := c.transitionNext
		if err := c.Office.ComposeGroup(key, 0, 0); err != nil {
			return err
		}
		c.TransitionGroups = append(c.TransitionGroups, key)
		c.transitionNext++
		// Modern elapsed ticks retain ordering and the special group17 pause. These
		// holds are presentation approximations, not recovered DOS wait durations.
		c.transitionHold = 3
		if key == 17 {
			c.transitionHold = 24
		}
	default:
		c.Office.advancePointer()
	}
	return nil
}

func (c *FrontEnd) syncOfficeView() error {
	// The scene is a derived cache only. Backend commits, including callers other
	// than a menu handler, invalidate it before the next update or input event.
	if c.Screen == ScreenOffice && c.viewRevision != c.Store.State().Revision {
		return c.RefreshOffice()
	}
	return nil
}

func (c *FrontEnd) openMission() error {
	c.Screen = ScreenMission
	c.missionPointer.initialized = false
	c.keyboardSelection = false
	var err error
	c.Choices, err = missionChoices(c.Store.State().SelectedPilot())
	if err != nil {
		return err
	}
	c.Highlight = -1
	p := c.Store.State().SelectedPilot()
	if p != nil {
		for i, m := range c.Choices {
			if m == p.MissionType {
				c.Highlight = i
			}
		}
	}
	c.seedMissionPointer()
	return nil
}

func (c *FrontEnd) Hit(x, y int) int {
	switch c.Screen {
	case ScreenInterstitial, ScreenBriefing, ScreenPapers, ScreenSettings, ScreenPaperTransition, ScreenOutfit, ScreenOrdnance:
		return c.missionHit(x, y)
	case ScreenMissionFiles:
		return c.missionFilesHit(x, y)
	case ScreenMissionEditor:
		return c.editorHit(x, y)
	case ScreenFilms:
		return c.filmHit(x, y)
	case ScreenReplay:
		return c.replayHit(x, y)
	case ScreenOffice:
		return c.Office.HoverKey
	case ScreenRoster:
		areas := c.Menus.Screens["ROSTER.SCN"].Areas
		for i := len(areas) - 1; i >= 0; i-- {
			a := areas[i]
			if a.Key >= 2 && a.Key <= 7 && insideRect(x, y, a.Rectangle) {
				return a.Key - 2
			}
		}
	case ScreenTheatre:
		for i := len(c.Menus.Theatres) - 1; i >= 0; i-- {
			if insideRect(x, y, c.Menus.Theatres[i].Rectangle) {
				return i
			}
		}
	case ScreenMission:
		for i := range c.Choices {
			if insideRect(x, y, missionChoiceRect(i)) {
				return i
			}
		}
	}
	return -1
}

func insideRect(x, y int, r [4]int) bool {
	return x >= r[0] && y >= r[1] && x < r[0]+r[2] && y < r[1]+r[3]
}

func (c *FrontEnd) Handle(a Actions) error {
	if err := c.syncOfficeView(); err != nil {
		return err
	}
	if check := c.rejectedCheck; check != nil {
		if !a.Close && !a.Quit {
			return nil
		}
		if check.published != nil {
			check.closeRequested = true
			return nil
		}
		// Preserve the sealed file and the unresolved sortie evidence before
		// handing the close request back to the ordinary quit dialog.
		c.rejectedCheck = nil
		if err := c.finishRejectedFilm(check.reason, check.sortie, check.playback.Film, fmt.Errorf("verification interrupted by close request; sealed recording retained")); err != nil {
			return err
		}
	}
	if a.FlightFocusLost {
		if c.Editor != nil {
			c.Editor.View.release()
		}
		if c.Files != nil {
			c.Files.PressedKey = 0
		}
	}
	moved := a.X != c.pointerX || a.Y != c.pointerY || a.Inside != c.pointerInside || a.PointerRelative && (a.PointerDX != 0 || a.PointerDY != 0)
	a.PointerMoved = moved // Sample before replacing the prior host coordinates.
	c.pointerX, c.pointerY, c.pointerInside = a.X, a.Y, a.Inside
	if moved {
		c.pointerCue = nil
		if c.Screen == ScreenOffice {
			c.Message = ""
		}
	}
	if c.pointerCue != nil {
		a.X, a.Y = c.pointerCue[0], c.pointerCue[1]
	}
	// Retain raw viewport coordinates above so leaving the chooser immediately
	// restores free logical movement, without warping/grabbing the OS pointer.
	a.X, a.Y = c.updateGamePointerAction(a)
	a = c.navigateHotspots(a)
	c.Office.SetPointer(a.X, a.Y, a.Inside)
	if c.Screen != ScreenOffice && c.Screen != ScreenFolder {
		c.Office.pointerStyle = 0
		if a.Inside && c.Hit(a.X, a.Y) >= 0 {
			c.Office.pointerStyle = 1
		}
		if c.Screen == ScreenMissionEditor && c.Editor != nil && c.Editor.Stage == "edit" && c.Editor.Held {
			c.Office.pointerStyle = 0 // Source held placement: arrow plus MBICONS, not menu finger.
		}
	}
	if c.Screen == ScreenQuit {
		if c.previousScreen == ScreenOrdnance && c.Mission != nil && c.Mission.Ordnance != nil {
			c.Mission.Ordnance.resetRowAnchor()
		}
		if c.previousScreen == ScreenScene && c.Rendering != nil {
			// Modal input must not leave a mast arrow latched after it was
			// released here. The next ordinary scene update reads held keys anew.
			c.Rendering.mastHeld = [4]bool{}
			c.Rendering.clearFlightInput()
		}
		// Native key and text queues can arrive separately (or Alt can suppress
		// text). Recognize the Y edge before generic key dismissal.
		if a.ConfirmQuit {
			if c.sceneEndConfirmation {
				return c.confirmSortieEnd()
			}
			c.ExitRequested = true
			c.AbandonFilmRecording()
			return c.Store.AbandonUnfinishedSorties("application quit before final acknowledgement")
		}
		for _, r := range a.Text {
			if r == 'y' || r == 'Y' {
				if c.sceneEndConfirmation {
					return c.confirmSortieEnd()
				}
				c.ExitRequested = true
				c.AbandonFilmRecording()
				return c.Store.AbandonUnfinishedSorties("application quit before final acknowledgement")
			}
		}
		if c.sceneEndConfirmation && c.Rendering != nil && c.Rendering.Initialized != nil && c.Rendering.Initialized.Sortie != nil && c.Rendering.Initialized.Sortie.Pending != "" {
			no := a.Cancel // native Escape-as-N policy; source naturally waits for Y/N.
			for _, v := range a.Text {
				if v == 'n' || v == 'N' {
					no = true
				}
			}
			if !no {
				return nil
			}
		}
		if a.Cancel || a.Selector1 || a.Selector2 || a.Backspace || a.Left || a.Right || a.Up || a.Down || a.ViewKey != 0 || a.ViewCycle != 0 || a.CycleArrangement || a.AnyKey || len(a.Text) > 0 {
			c.cancelNaturalSortieEnd()
			c.Screen = c.previousScreen
		}
		return nil
	}
	if (a.Quit || a.Close || a.EndMission) && c.Screen == ScreenScene && c.Rendering != nil && c.Rendering.GPS != nil {
		c.closeGPS()
	}
	if a.Quit || a.Close {
		// Modal cancellation preserves the draft/held object, not a held drag.
		if c.Editor != nil {
			c.Editor.View.release()
		}
		if c.Files != nil {
			c.Files.PressedKey = 0
		}
		c.sceneEndConfirmation = false
		c.quitFadeBackground = nil
		if c.fade != nil {
			c.quitFadeBackground = c.fadeSource()
		}
		c.quitBackground = c.frameBody()
		c.previousScreen = c.Screen
		c.Screen = ScreenQuit
		return nil
	}
	if a.EndMission && c.Screen == ScreenScene {
		c.quitFadeBackground = nil
		if c.fade != nil {
			c.quitFadeBackground = c.fadeSource()
		}
		c.quitBackground = c.frameBody()
		c.previousScreen = c.Screen
		c.sceneEndConfirmation = true
		c.Screen = ScreenQuit
		return nil
	}
	if c.missionSaveError != nil {
		// A failed multi-file commit may need journal reconciliation at restart.
		// Quit remains available, but no old or candidate mission can be acted on.
		return nil
	}
	if c.fade != nil {
		return nil // The input latch still consumes held presses during presentation.
	}
	if c.Screen == ScreenStartup {
		if a.Skip || a.Cancel {
			c.Startup.Skip()
			if c.Startup.Done {
				c.releaseStartup()
				return c.RefreshOffice()
			}
		}
		return nil
	}
	if c.Screen == ScreenMissionFiles {
		return c.handleMissionFiles(a)
	}
	if c.Screen == ScreenMissionEditor {
		return c.handleEditor(a)
	}
	if c.Screen == ScreenFilms {
		return c.handleFilms(a)
	}
	if c.Screen == ScreenReplay {
		return c.handleReplay(a)
	}
	if c.Screen == ScreenScene {
		return c.handleRenderingPreview(a)
	}
	if c.Screen == ScreenSceneInitialization {
		return nil
	}
	if c.Screen == ScreenDebrief {
		return c.handleDebrief(a)
	}
	// Editing owns printable input, Space and Enter before generic selectors.
	if c.Draft != nil {
		return c.edit(a)
	}
	if isMissionScreen(c.Screen) {
		return c.handleMission(a)
	}
	if a.Cancel {
		if c.Screen != ScreenOffice {
			// Roster, chooser and recovery Escape have explicit22h return arms.
			if c.Screen == ScreenRoster || c.Screen == ScreenTheatre || c.Screen == ScreenMission || c.Screen == ScreenRecovery {
				c.emitAudioCue(gs4Accepted)
			}
			return c.RefreshOffice()
		}
		return nil // Escape in the office is not quit.
	}
	if (c.Screen == ScreenMission || c.Screen == ScreenTheatre) && (moved || a.PointerPressed) {
		c.keyboardSelection = false
	}
	if a.Inside && (moved || a.PointerPressed) {
		if hit := c.Hit(a.X, a.Y); hit >= 0 {
			c.Highlight = hit
		}
	}
	switch c.Screen {
	case ScreenOffice:
		if !a.Selector1 {
			return nil
		}
		switch c.Office.HoverKey {
		case 4:
			return c.enterMission()
		case 1:
			c.emitAudioCue(gs4Accepted)
			c.Screen = ScreenRoster
			c.Highlight = 0
			state := c.Store.State()
			c.rosterWorking = &state
			for i, p := range state.Slots {
				if p != nil && p.ID == c.viewedOfficeState().SelectedPilotID {
					c.Highlight = i
				}
			}
		case 2, 3:
			p := c.Store.State().SelectedPilot()
			if p == nil || p.Status != gamestate.StatusActive || !c.viewingPostedPilot() {
				c.emitAudioCue(gs4Denied)
				c.pointerCue = &[2]int{184, 70}
				c.Office.SetPointer(184, 70, true)
				c.Message = ""
				return nil
			}
			if c.Office.HoverKey == 2 {
				locked, err := theatreLocked(p)
				if err != nil {
					return err
				}
				if locked {
					c.emitAudioCue(gs4Denied)
					c.pointerCue = &[2]int{184, 70}
					c.Office.SetPointer(184, 70, true)
					c.Message = ""
					return nil
				}
				c.emitAudioCue(gs4Accepted)
				c.Screen = ScreenTheatre
				c.Highlight = p.Theatre.Raw()
			} else {
				if _, err := missionChoices(p); err != nil {
					return err
				}
				c.emitAudioCue(gs4Accepted)
				c.Screen = ScreenFolder
				c.transitionNext = c.Office.OfficerGroup + 1
				if c.transitionNext < 0 {
					c.transitionNext = 0
				}
				c.transitionHold = 0
				c.TransitionGroups = nil
			}
		case 6:
			c.emitAudioCue(gs4Computer)
			c.Office.CycleComputerMode()
		case 8:
			c.emitAudioCue(gs4Entry)
			return c.openFilmLibrary(false, "", "")
		case 9:
			viewed := c.viewedOfficeState()
			if viewed.SelectedPilot() == nil {
				c.emitAudioCue(gs4Denied)
				return nil
			}
			snapshot, err := c.Store.RecoveryCandidateForPilot(viewed.SelectedPilotID)
			if err != nil {
				c.emitAudioCue(gs4Denied)
				c.reportDiagnostic("roster", "Pre-mission backup unavailable: "+err.Error(), err)
				return nil
			}
			if snapshot == nil {
				c.emitAudioCue(gs4Denied)
				c.reportStatus("roster", "No earlier saved roster is available.")
				return nil
			}
			c.emitAudioCue(gs4Entry)
			c.Recovery = snapshot
			c.Screen = ScreenRecovery

		case 0, 5, 7:
			if a.Inside {
				c.emitAudioCue(gs4Denied) // Original office invalid-action armO020a.
			}
		}
	case ScreenRoster:
		c.navigate(a, 6)
		if c.Highlight < 0 {
			return nil
		}
		state := c.rosterState()
		p := state.Slots[c.Highlight]
		for _, r := range a.Text {
			if r == 'e' || r == 'E' {
				return c.beginRosterDraft(false)
			}
		}
		if a.Selector2 {
			return c.beginRosterDraft(true)
		} else if a.Selector1 && p != nil {
			if p.Status != gamestate.StatusActive {
				// Inspection/Phoenix is independent of the unavailable source POST.
				c.ViewedPilotID = p.ID
				c.emitAudioCue(gs4Accepted)
				return c.RefreshOffice()
			}
			c.emitAudioCue(gs4Accepted)
			c.ViewedPilotID = ""
			state.SelectedPilotID = p.ID
			if err := c.Store.Commit(state); err != nil {
				return err
			}
			return c.RefreshOffice()
		}
	case ScreenTheatre:
		c.navigate(a, len(c.Menus.Theatres))
		if choice := c.activatedChoice(a, len(c.Menus.Theatres)); choice >= 0 {
			c.Highlight = choice
			state := c.Store.State()
			p := state.SelectedPilot()
			if p == nil {
				return fmt.Errorf("no selected pilot")
			}
			theatre, err := gamestate.TheatreFromRaw(c.Menus.Theatres[c.Highlight].Selector)
			if err != nil {
				return err
			}
			c.emitAudioCue(gs4Accepted)
			p.Theatre = theatre
			if err = c.Store.Commit(state); err != nil {
				return err
			}
			return c.RefreshOffice()
		} else if a.Selector1 && a.Inside {
			c.emitAudioCue(gs4Denied) // T013e rejects a nonpositive AREA selection.
		}
	case ScreenMission:
		c.navigate(a, len(c.Choices))
		if choice := c.activatedChoice(a, len(c.Choices)); choice >= 0 {
			c.Highlight = choice
			state := c.Store.State()
			p := state.SelectedPilot()
			if p == nil {
				return fmt.Errorf("no selected pilot")
			}
			c.emitAudioCue(gs4Accepted)
			p.MissionType = c.Choices[c.Highlight]
			if p.MissionType == gamestate.MissionCampaign {
				if p.Campaign != nil {
					// Typed campaign theatre supersedes its imported raw witness.
					p.Theatre = p.Campaign.Theatre
				} else if value, known := p.LegacyByte(0x4f); known && value != 0 {
					// Legacy selection copies the preserved theatre byte; actual
					// preparation still requires a matching complete cache import.
					raw, ok := p.LegacyByte(0x57)
					if !ok {
						return fmt.Errorf("campaign theatre source is unavailable")
					}
					theatre, err := gamestate.TheatreFromRaw(int(raw))
					if err != nil {
						return err
					}
					p.Theatre = theatre
				}
			}
			if err := c.Store.Commit(state); err != nil {
				return err
			}
			return c.RefreshOffice()
		}
	case ScreenRecovery:
		if a.Selector1 && c.Recovery != nil {
			c.emitAudioCue(gs4Accepted)
			if err := c.Store.RestoreForPilot(c.Recovery.State.SelectedPilotID, c.Recovery.ID); err != nil {
				c.reportDiagnostic("roster", err.Error(), err)
				return nil
			}
			c.Recovery = nil
			c.ViewedPilotID = "" // Recovery restores the target but does not post it.
			return c.RefreshOffice()
		}
	}
	return nil
}

func (c *FrontEnd) navigate(a Actions, count int) {
	if count <= 0 {
		return
	}
	if (a.Down || a.Right || a.Up || a.Left) && (c.Screen == ScreenMission || c.Screen == ScreenTheatre) {
		c.keyboardSelection = true
	}
	if a.Down || a.Right {
		c.Highlight = (c.Highlight + 1) % count
	}
	if a.Up || a.Left {
		c.Highlight = (c.Highlight - 1 + count) % count
	}
}

func (c *FrontEnd) activatedChoice(a Actions, count int) int {
	if !a.Selector1 {
		return -1
	}
	// A mouse edge always needs a real hit. Space uses the arrow selection only
	// while the keyboard owns it; a stationary pointer cannot steal it back.
	if !a.PointerPressed && c.keyboardSelection && c.Highlight >= 0 && c.Highlight < count {
		return c.Highlight
	}
	if a.Inside {
		if hit := c.Hit(a.X, a.Y); hit >= 0 && hit < count {
			return hit
		}
	}
	return -1
}

func (c *FrontEnd) edit(a Actions) error {
	d := c.Draft
	if a.Cancel {
		c.Draft = nil
		c.Message = ""
		return nil
	}
	if d.Stage == 1 {
		if a.Left {
			d.Pilot.Emblem = (d.Pilot.Emblem + 17) % 18
		}
		if a.Right {
			d.Pilot.Emblem = (d.Pilot.Emblem + 1) % 18
		}
		if a.Enter {
			d.Stage = 2
		}
		return nil
	}
	text := &d.Pilot.Name
	limit := 35
	if d.Stage == 2 {
		text = &d.Pilot.Squadron
		limit = 25
	}
	if (a.Backspace || a.Left) && len(*text) > 0 {
		*text = (*text)[:len(*text)-1]
	}
	if len(a.Text) > 0 || a.Backspace || a.Left {
		c.Message = ""
	}
	for _, r := range a.Text {
		if r < 32 || r > 127 || r == 32 && len(*text) == 0 || len(*text) >= limit {
			continue
		}
		next := *text + string(r)
		if c.Menus.Fonts[5].Measure([]byte(next)) <= 200 {
			*text = next
		}
	}
	if !a.Enter {
		return nil
	}
	if d.Stage == 0 {
		d.Pilot.Name = strings.TrimRight(d.Pilot.Name, " ")
		if d.Pilot.Name == "" {
			c.reportStatus("roster", "Enter a pilot name.")
			return nil
		}
		d.Stage = 1
		return nil
	}
	if d.Pilot.Squadron == "" {
		c.reportStatus("roster", "Enter a unit name.")
		return nil
	}
	if d.Replacement {
		return c.finishRosterReplacement(d)
	}
	state := c.Store.State()
	state.Slots[d.Slot] = &d.Pilot
	if err := c.Store.Commit(state); err != nil {
		return err
	}
	// Finishing the editor saves the unit but does not post it. Posting remains
	// Selector1 on the roster. Existing edits preserve rank/progression (safe port adaptation).
	c.Draft = nil
	c.Message = ""
	return nil
}
