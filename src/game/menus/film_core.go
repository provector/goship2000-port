package menus

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gunship2000-assets/game/diagnostics"
	"io"
	goruntime "runtime"

	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/combat"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/flightcontrol"
	"gunship2000-assets/game/navigation"
	"gunship2000-assets/game/render3d"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
)

// The isolated controller deliberately has no Store. Only these mechanical
// entry points are used; result/office/generation capabilities are absent.
type filmSessionIsolation struct {
	Mode string
	Done bool
}
type filmLaunch struct {
	GeneratedSource json.RawMessage             `json:",omitempty"`
	EnemyRules      *gamestate.EnemyRules       `json:",omitempty"`
	AuthoredSource  *gamestate.AuthoredDocument `json:",omitempty"`
	Adjustment      *FilmInitialAdjustment      `json:",omitempty"`
	Final           FinalPreflightConfiguration
	Runtime         configuration.RuntimeOptions
	Initial         map[string]string
}
type filmInput struct {
	Jammers          *[2]bool                 `json:",omitempty"`
	Live             *flightcontrol.LiveInput `json:",omitempty"`
	Command          int
	HasCommand       bool
	Aux              []int
	Directions       [4]bool
	Trigger, Pressed bool
}
type filmControl struct {
	View           render3d.CameraState
	Navigation     [5]navigation.Route
	Speech         bool
	Compression    int16
	HUD            bool
	Palette        render3d.ReplayPaletteState
	OrderRecipient int `json:",omitempty"`
}
type filmStep struct {
	Ordinal uint64
	Timing  flight.Timing
	Input   filmInput
	Control *filmControl `json:",omitempty"`
}
type filmBoundary struct {
	Support    *navigation.SupportCommand `json:",omitempty"`
	Count      int                        `json:",omitempty"`
	Kind       string
	Action     Actions      `json:",omitempty"`
	Hit        *filmHit     `json:",omitempty"`
	GPSTick    *filmGPSTick `json:",omitempty"`
	GPSRelease *bool        `json:",omitempty"`
}
type filmHit struct {
	Victim int
	Flags  uint16
	Source string
	Seed   *uint16
}

// focusReset is session-local. Only consecutive paused focus polls may reuse it.
type focusReset struct {
	operations int
	steps      uint64
	gps        *navigation.GPS
	release    bool
	control    filmControl
	defenses   flight.Defenses
	ownership  flight.DefenseOwnership
	damage     [2]byte
}
type filmRecorder struct {
	lastOperation   replay.Operation
	lastFocusReset  focusReset
	focusResetValid bool
	Spool           *replay.Temporary
	Film            *replay.Film
	Pending         *replay.Operation
	Ordinal         uint64
	PreviousControl []byte
	Events          []replay.Event
	EventIndex      uint64
	Error           string
	Sealed          bool
	InitialKinds    [80]byte
}
type FilmPlayback struct {
	reachedSupport         *navigation.SupportCommand
	reachedCheckpoint      map[string]string
	FailureDiagnostic      *FilmFailureDiagnostic
	Engine                 *FrontEnd
	Film                   *replay.Film
	Cursor                 int
	Done                   bool
	Divergence             string
	EventCount             uint64
	Recovery               *replay.RecoveryReceipt
	recoveryVerified       bool
	initialKinds           [80]byte
	summary                *replay.Summary
	verifiedSummaryContent string // Immutable body bound to a fully checked historical summary.
	Practice               bool
	current                *filmStep
	reached                bool
	terminal               bool
	events                 []replay.Event
	reachedSteps           uint64
}

func filmJSON(v any) []byte { b, _ := json.Marshal(v); return b }
func filmHash(v any) string { b := filmJSON(v); h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func filmResourceIdentity(a *assetdata.OfficeAssets) (string, error) {
	if a == nil {
		return "", fmt.Errorf("film needs prepared resource ownership")
	}
	return assetdata.LogicalResourceProfile(a.Root)
}
func cloneFilmFinal(f *FinalPreflightConfiguration) (*FinalPreflightConfiguration, error) {
	if f == nil {
		return nil, fmt.Errorf("film needs accepted launch")
	}
	b, e := json.Marshal(f)
	if e != nil {
		return nil, e
	}
	if len(b) > 8<<20 {
		return nil, fmt.Errorf("film launch exceeds supported size")
	}
	var out FinalPreflightConfiguration
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e = d.Decode(&out); e != nil {
		return nil, e
	}
	return &out, nil
}
func (p *RenderingPreview) filmControl() filmControl {
	r := p.Flight
	control := filmControl{View: p.ViewControls.ReplayState(), Navigation: r.Navigation, Speech: r.SpeechEnabled, Compression: r.Compression.Request, HUD: p.Cockpit.HUD, Palette: p.PaletteState.ReplayState()}
	if p.r1Mechanics {
		control.OrderRecipient = r.OrderRecipient
	}
	return control
}

// FilmCheckpoint is finite owner state, not a mutable process/Store snapshot.
// Host debt, UI pauses, diagnostics, interpolation and sound transport excluded.
func (p *RenderingPreview) FilmCheckpoint() map[string]string {
	r := p.Flight
	logical := struct {
		Parameters                                               flight.Parameters
		Systems                                                  flight.Systems
		Defenses                                                 flight.Defenses
		Axes                                                     flightcontrol.Axes
		Low, Primary, Secondary, Mass, Wind, WindStrength, Salvo int16
		Flags                                                    byte
		Speech                                                   bool
		Health                                                   uint16
		Overlay                                                  [2][2]int16
		Steps                                                    uint64
		Phase                                                    uint16
		Clock                                                    flight.ReplayClockState
	}{r.Parameters, r.Systems, r.Defenses, r.Axes, r.LowLimit, r.PrimaryMFD, r.SecondaryMFD, r.MassTerm, int16(r.WindHeading), r.WindStrength, r.RocketSalvo, r.SystemFlags, r.SpeechEnabled, r.DamageHealth, r.DamageOverlay, r.Steps, r.ElapsedDA, r.Clock.ReplayState()}
	out := map[string]string{"flight": filmHash(struct {
		State   *flight.State
		Runtime any
	}{r.State, struct {
		Logical          any
		Notifications    combat.NotificationState
		Terminal         terminalPresentation
		Directions       [4]bool
		Trigger, Pressed bool
	}{logical, r.Notifications, r.TerminalPresentation, r.commandInput.Directions, r.commandInput.Trigger, r.commandInput.TriggerPressed}}), "mission": filmHash(p.Initialized), "combat": filmHash(r.Combat.FilmState()), "navigation": filmHash(struct {
		Routes   [5]navigation.Route
		Avionics any
		View     render3d.CameraState
		Speech   any
	}{r.Navigation, r.Avionics, p.ViewControls.ReplayState(), r.AcquisitionSpeech}), "environment": filmHash(p.Scene.Environment), "pending-hit": filmHash(filmPendingHits(r.acceptedPlayerHits)), "palette": filmHash(p.PaletteState.ReplayState()), "visual": filmHash(p.Visual), "reference": filmHash(struct {
		Buffer []byte
		Bottom int
	}{p.avionicsReference, p.avionicsReferenceBottom})}
	if p.p4Mechanics {
		out["appearance"] = filmHash(p.p4AppearanceCheckpoint())
	}
	if p.r1Mechanics {
		out["orders-r1"] = filmHash(struct {
			Autonomy  bool
			Recipient int
		}{p.wingmenAutonomy, r.OrderRecipient})
	}
	if !p.filmLegacy {
		out["modal"] = filmHash(p.filmModalState())
	}
	if p.resilientFlight {
		out["collective-owner"] = filmHash(r.collectiveOwner)
	}
	if r.commandInput.Live != nil {
		out["controls-q3"] = filmHash(struct {
			Input *flightcontrol.LiveInput
			State flightcontrol.LiveState
		}{r.commandInput.Live, r.liveControls})
	}
	return out
}
func compareFilmCheckpoint(want, got map[string]string) string {
	if len(want) != len(got) {
		return "owner-domain"
	}
	if v, ok := want["collective-owner"]; ok && (v == "" || v != got["collective-owner"]) {
		return "collective-owner"
	}
	if v, ok := want["modal"]; ok && (v == "" || v != got["modal"]) {
		return "modal"
	}
	if v, ok := want["appearance"]; ok && (v == "" || v != got["appearance"]) {
		return "appearance"
	}
	if v, ok := want["controls-q3"]; ok && (v == "" || v != got["controls-q3"]) {
		return "controls-q3"
	}
	for _, g := range []string{"flight", "mission", "combat", "navigation", "environment", "reference", "pending-hit", "palette", "visual"} {
		if want[g] == "" || want[g] != got[g] {
			return g
		}
	}
	return ""
}
func (c *FrontEnd) beginFilmRecording(final *FinalPreflightConfiguration) {
	c.filmPending = nil
	c.filmDiagnostics = nil
	c.filmError = ""
	if c.filmIsolation == nil && c.Rendering != nil && final != nil && final.Mission != nil {
		diagnostics.SetMission(final.Mission.World, "", "flight preparation", 0)
	}
	if c.filmIsolation != nil || c.Rendering == nil || c.FilmDirectory == "" {
		return
	}
	diagnostics.SetMission(final.Mission.World, "", "starting recording", 0)
	if !filmPresentationReady(c.Office.Assets) {
		c.filmError = "native replay requires Preparation25; career behavior unchanged"
		c.Message = c.filmError
		c.noteRecordingStatus("Optional recording unavailable", fmt.Errorf("%s", c.filmError), "live sortie/career behavior unchanged")
		return
	}
	final, e := cloneFilmFinal(final)
	if e != nil {
		c.Message = "Film recording unavailable: " + e.Error()
		c.noteRecordingStatus("Recording launch clone failed", e, "live sortie continues")
		return
	}
	id, e := replay.NewID()
	if e != nil {
		c.Message = "Film recording unavailable"
		c.noteRecordingStatus("Recording identity unavailable", e, "live sortie continues")
		return
	}
	resources, e := filmResourceIdentity(c.Office.Assets)
	if e != nil {
		c.Message = e.Error()
		c.noteRecordingStatus("Recording resource identity rejected", e, "live sortie continues; no supported film produced")
		return
	}
	p := c.Rendering
	initial := p.FilmCheckpoint()
	recordedRuntime := c.Runtime.Effective()
	recordedRuntime.FloatingGeometry = false
	recordedRuntime.SimulationResolution = "" // Physical display choice is local to the viewer.
	launch := filmLaunch{EnemyRules: p.Initialized.EnemyRules, Final: *final, Runtime: recordedRuntime, Initial: initial}
	if c.r83Profile() {
		launch.Runtime.AvoidancePolicy = configuration.AvoidanceLegacy
		if p.Flight.State.CorrectedAvoidance {
			launch.Runtime.AvoidancePolicy = configuration.AvoidanceCorrected
		}
	}
	if p.p5Mechanics && c.Mission != nil && c.Mission.AuthoredSource != nil && c.Mission.AuthoredSource.Initial == nil {
		source, err := gamestate.CloneAuthored(c.Mission.AuthoredSource)
		if err != nil || len(filmJSON(source)) > 4<<20 {
			c.filmError = "authored source exceeds the bounded recording carrier"
			return
		}
		launch.AuthoredSource = source
	} else if c.Mission != nil && c.Mission.Data != nil {
		if groups := c.Mission.Data.GenerationIndex(); groups != nil {
			launch.GeneratedSource = filmJSON(groups)
		} else if c.Mission.AuthoredSource == nil {
			diagnostics.Warn("replay", "Initial preparation lacks generator group provenance", "world", final.Mission.World, "export", "historical layout limits apply")
		}
	}
	f := &replay.Film{Header: replay.Header{Version: replay.Version, ID: id, Title: "Last Mission", Simulation: replay.P4Compatibility, InputSchema: replay.Q32InputCompatibility, InitializationSchema: "accepted-native-launch-v1", ClockPolicy: flight.NativeClockPolicy, Resources: resources, Build: filmBuildIdentity(), Platform: goruntime.GOOS + "/" + goruntime.GOARCH, World: p.Initialized.World}, Launch: filmJSON(launch)}
	if p.p5Mechanics {
		f.Header.Simulation = replay.P5Compatibility
	}
	if p.p6Mechanics {
		f.Header.Simulation = replay.P6Compatibility
	}
	if p.p7Mechanics {
		f.Header.Simulation = replay.P7Compatibility
	}
	if p.r1Mechanics {
		f.Header.Simulation = replay.R1Compatibility
		if p.r11Mechanics {
			f.Header.Simulation = replay.R11Compatibility
		}
		if p.cameraContactMechanics {
			f.Header.Simulation = replay.R11CameraCompatibility
		}
		if p.routeAvoidanceMechanics {
			f.Header.Simulation = replay.R11AvoidanceCompatibility
		}
		if p.waypointReleaseMechanics {
			f.Header.Simulation = replay.R11WaypointReleaseCompatibility
		}
		if p.navigationMechanics {
			f.Header.Simulation = replay.R11NavigationCompatibility
		}
		if p.wingmanEngagementCompression {
			f.Header.Simulation = replay.R12Compatibility
		}
		f.Header.InputSchema = replay.R1InputCompatibility
		if c.simulationProfile == "" {
			f.Header.InputSchema = replay.RouteConfirmationInput
		}
	}
	if final.Mission.Inputs.MissionType == gamestate.MissionSimulated {
		f.Header.Simulation = replay.P4AuthoredCompatibility
		if p.p5Mechanics {
			f.Header.Simulation = replay.P5AuthoredCompatibility
		}
		if p.p6Mechanics {
			f.Header.Simulation = replay.P6AuthoredCompatibility
		}
		if p.p7Mechanics {
			f.Header.Simulation = replay.P7AuthoredCompatibility
		}
		if p.r1Mechanics {
			f.Header.Simulation = replay.R1AuthoredCompatibility
			if p.r11Mechanics {
				f.Header.Simulation = replay.R11AuthoredCompatibility
			}
			if p.cameraContactMechanics {
				f.Header.Simulation = replay.R11CameraAuthoredCompatibility
			}
			if p.routeAvoidanceMechanics {
				f.Header.Simulation = replay.R11AvoidanceAuthoredCompatibility
			}
			if p.waypointReleaseMechanics {
				f.Header.Simulation = replay.R11WaypointReleaseAuthoredCompatibility
			}
			if p.navigationMechanics {
				f.Header.Simulation = replay.R11NavigationAuthoredCompatibility
			}
			if p.wingmanEngagementCompression {
				f.Header.Simulation = replay.R12AuthoredCompatibility
			}
		}
	}
	if len(final.Flight) > 0 {
		f.Header.Pilot = final.Flight[0].Name
	}
	if p.stationaryEnemyMechanics {
		f.Header.Simulation = replay.R21Compatibility
		if final.Mission.Inputs.MissionType == gamestate.MissionSimulated {
			f.Header.Simulation = replay.R21AuthoredCompatibility
		}
	}
	if p.wingmanDamageMechanics {
		f.Header.Simulation = replay.R22Compatibility
		if final.Mission.Inputs.MissionType == gamestate.MissionSimulated {
			f.Header.Simulation = replay.R22AuthoredCompatibility
		}
	}
	if p.fieldCorrectionsMechanics {
		f.Header.Simulation = replay.R221Compatibility
		if final.Mission.Inputs.MissionType == gamestate.MissionSimulated {
			f.Header.Simulation = replay.R221AuthoredCompatibility
		}
	}
	if p.enemyBehaviorMechanics {
		f.Header.Simulation = replay.R23Compatibility
		if final.Mission.Inputs.MissionType == gamestate.MissionSimulated {
			f.Header.Simulation = replay.R23AuthoredCompatibility
		}
	}
	if p.threatFeedbackMechanics {
		f.Header.Simulation = replay.R231Compatibility
		if final.Mission.Inputs.MissionType == gamestate.MissionSimulated {
			f.Header.Simulation = replay.R231AuthoredCompatibility
		}
	}
	if p.copilotMechanics {
		f.Header.Simulation = replay.R3Compatibility
		if final.Mission.Inputs.MissionType == gamestate.MissionSimulated {
			f.Header.Simulation = replay.R3AuthoredCompatibility
		}
	}
	if p.r4Mechanics {
		f.Header.Simulation = replay.R4Compatibility
		if final.Mission.Inputs.MissionType == gamestate.MissionSimulated {
			f.Header.Simulation = replay.R4AuthoredCompatibility
		}
	}
	if p.r5Mechanics {
		f.Header.Simulation = replay.R5Compatibility
		if final.Mission.Inputs.MissionType == gamestate.MissionSimulated {
			f.Header.Simulation = replay.R5AuthoredCompatibility
		}
	}
	if p.r6Mechanics {
		f.Header.Simulation = replay.R6Compatibility
		if final.Mission.Inputs.MissionType == gamestate.MissionSimulated {
			f.Header.Simulation = replay.R6AuthoredCompatibility
		}
	}
	if p.r8Mechanics {
		f.Header.Simulation = replay.R8Compatibility
		if final.Mission.Inputs.MissionType == gamestate.MissionSimulated {
			f.Header.Simulation = replay.R8AuthoredCompatibility
		}
	}
	if c.r83Profile() {
		f.Header.Simulation = replay.R83Compatibility
		if final.Mission.Inputs.MissionType == gamestate.MissionSimulated {
			f.Header.Simulation = replay.R83AuthoredCompatibility
		}
	}
	if c.r86Profile() {
		f.Header.Simulation = replay.R86Compatibility
		if final.Mission.Inputs.MissionType == gamestate.MissionSimulated {
			f.Header.Simulation = replay.R86AuthoredCompatibility
		}
	}
	if c.t5Profile() {
		f.Header.Simulation = replay.T5Compatibility
		if final.Mission.Inputs.MissionType == gamestate.MissionSimulated {
			f.Header.Simulation = replay.T5AuthoredCompatibility
		}
	}
	if c.exitWaypointProfile() {
		f.Header.Simulation = replay.W31Compatibility
		if final.Mission.Inputs.MissionType == gamestate.MissionSimulated {
			f.Header.Simulation = replay.W31AuthoredCompatibility
		}
	} else if c.correctiveFlightProfile() {
		f.Header.Simulation = replay.W3Compatibility
		if final.Mission.Inputs.MissionType == gamestate.MissionSimulated {
			f.Header.Simulation = replay.W3AuthoredCompatibility
		}
	} else if c.enginePowerProfile() {
		f.Header.Simulation = replay.V45Compatibility
		if final.Mission.Inputs.MissionType == gamestate.MissionSimulated {
			f.Header.Simulation = replay.V45AuthoredCompatibility
		}
	} else if c.resilientFlightProfile() {
		f.Header.Simulation = replay.U26Compatibility
		if final.Mission.Inputs.MissionType == gamestate.MissionSimulated {
			f.Header.Simulation = replay.U26AuthoredCompatibility
		}
	} else if c.wingmanRouteEditProfile() {
		f.Header.Simulation = replay.U25Compatibility
		if final.Mission.Inputs.MissionType == gamestate.MissionSimulated {
			f.Header.Simulation = replay.U25AuthoredCompatibility
		}
	} else if c.lowCityProfile() {
		f.Header.Simulation = replay.U24Compatibility
		if final.Mission.Inputs.MissionType == gamestate.MissionSimulated {
			f.Header.Simulation = replay.U24AuthoredCompatibility
		}
	}
	spool, err := (replay.Store{Root: c.FilmDirectory}).BeginTemporaryWithLimit(f, c.RecordingLimit)
	if err != nil {
		c.filmError = err.Error()
		c.Message = "Film recording unavailable; sortie career behavior unchanged."
		c.noteRecordingStatus("Recording spool unavailable", err, "live sortie/career behavior unchanged")
		return
	}
	if c.Mission != nil && c.Mission.ResultLaunch != nil {
		spool.SetSessionID(c.Mission.ResultLaunch.Start.ID)
	}
	diagnostics.SetMission(final.Mission.World, f.Header.ID, "flight", 0)
	diagnostics.Info("recording", "Recording started", "id", f.Header.ID)
	c.filmDiagnostics = spool
	p.filmRecorder = &filmRecorder{Film: f, Spool: spool, PreviousControl: filmJSON(p.filmControl()), InitialKinds: p.filmInitialKinds()}
	p.installFilmObserver()
}
func (p *RenderingPreview) installFilmObserver() {
	if p.Flight == nil || p.Flight.Combat == nil {
		return
	}
	p.Flight.Combat.ResultObserver = func(v combat.Result) {
		if p.filmRecorder != nil && !p.filmRecorder.Sealed && p.filmRecorder.Error == "" {
			r := p.filmRecorder
			r.EventIndex++
			r.Events = append(r.Events, replay.Event{Index: r.EventIndex, Kind: v.Kind, Actor: v.Actor, Target: v.Target, Sequence: v.Sequence, Timestamp: v.Timestamp})
		}
		if p.filmPlayback != nil {
			q := p.filmPlayback
			q.EventCount++
			q.events = append(q.events, replay.Event{Index: q.EventCount, Kind: v.Kind, Actor: v.Actor, Target: v.Target, Sequence: v.Sequence, Timestamp: v.Timestamp})
		}
	}
}
func (c *FrontEnd) filmBeginStep(t flight.Timing, terminal bool, command int, has bool, aux []int) (int, bool, []int, error) {
	p := c.Rendering
	r := p.Flight
	if q := p.filmPlayback; q != nil && !q.Practice {
		if q.current == nil || q.current.Timing != t || q.terminal != terminal {
			return 0, false, nil, fmt.Errorf("film timing/phase witness differs")
		}
		in := q.current.Input
		if in.Live != nil && (!replay.ContinuousInput(q.Film.Header.InputSchema) || !in.Live.Valid()) {
			return 0, false, nil, fmt.Errorf("unsupported or invalid continuous input operand")
		}
		if !replay.HeldInput(q.Film.Header.InputSchema) && (in.Jammers != nil || in.Live != nil && (in.Live.Mode != "" || in.Live.Slider)) {
			return 0, false, nil, fmt.Errorf("new input fields in historical film")
		}
		r.commandInput.Momentary = replay.HeldInput(q.Film.Header.InputSchema)
		if in.Jammers != nil {
			r.commandInput.jammerHeld = *in.Jammers
		}
		r.commandInput.Live = in.Live
		r.commandInput.Directions = in.Directions
		r.commandInput.Trigger = in.Trigger
		r.commandInput.TriggerPressed = in.Pressed
		if q.current.Control != nil {
			return 0, false, nil, fmt.Errorf("unversioned corrective control state is not supported")
		}
		q.reached = true
		return in.Command, in.HasCommand, in.Aux, nil
	}
	if f := p.filmRecorder; f != nil && !f.Sealed && f.Error == "" {
		c.filmFinishStep()
		if f.Error != "" {
			return command, has, aux, nil
		}
		f.Ordinal++
		v := filmStep{Ordinal: f.Ordinal, Timing: t, Input: filmInput{Jammers: &r.commandInput.jammerHeld, Live: r.commandInput.Live, Command: command, HasCommand: has, Aux: append([]int(nil), aux...), Directions: r.commandInput.Directions, Trigger: r.commandInput.Trigger, Pressed: r.commandInput.TriggerPressed}}
		ctrl := p.filmControl()
		b := filmJSON(ctrl)
		if !bytes.Equal(b, f.PreviousControl) {
			f.failRecording("input", fmt.Errorf("Unsupported direct control-state change outside recorded transactions"))
			return command, has, aux, nil
		}
		kind := "step"
		if terminal {
			kind = "terminal"
		}
		f.Pending = &replay.Operation{Kind: kind, Payload: filmJSON(v)}
	}
	return command, has, aux, nil
}
func (c *FrontEnd) filmFinishStep() {
	p := c.Rendering
	if p != nil && p.filmPlayback != nil {
		q := p.filmPlayback
		if q.current != nil && q.reachedCheckpoint == nil {
			q.reachedCheckpoint = p.FilmCheckpoint()
		}
	}
	if p == nil || p.filmRecorder == nil {
		return
	}
	f := p.filmRecorder
	if f.Pending == nil {
		return
	}
	if f.Error != "" {
		f.Pending = nil
		f.Events = nil
		return
	}
	f.Pending.Checkpoint = p.FilmCheckpoint()
	f.Pending.Events = f.Events
	f.Events = nil
	pending := *f.Pending
	f.Pending = nil
	f.appendOperation(pending)
	f.PreviousControl = filmJSON(p.filmControl())
}
func (c *FrontEnd) recordFilmBoundary(b filmBoundary) {
	p := c.Rendering
	if p == nil || p.filmTransaction || p.filmRecorder == nil || p.filmRecorder.Sealed || p.filmPlayback != nil {
		return
	}
	c.filmFinishStep()
	f := p.filmRecorder
	if f.Error != "" {
		return
	}
	f.appendOperation(replay.Operation{Kind: "boundary", Payload: filmJSON(b), Checkpoint: p.FilmCheckpoint(), Events: f.Events})
	f.Events = nil
	f.PreviousControl = filmJSON(p.filmControl())
}
func (c *FrontEnd) filmFinishEnding() {
	p := c.Rendering
	if p == nil {
		return
	}
	if c.filmIsolation != nil {
		c.filmIsolation.Done = true
		return
	}
	if f := p.filmRecorder; f != nil && !f.Sealed {
		c.filmFinishStep()
		if f.Error != "" {
			c.filmError = f.Error
			f.Sealed = true
			if f.Spool != nil {
				_ = f.Spool.Close()
			}
			return
		}
		reason := "voluntary"
		if p.Initialized.Sortie != nil && p.Initialized.Sortie.Result != nil {
			reason = p.Initialized.Sortie.Result.Reason
		}
		f.appendOperation(replay.Operation{Kind: "end", Payload: filmJSON(struct{ Reason string }{reason}), Checkpoint: p.FilmCheckpoint(), Events: f.Events})
		f.Events = nil
		if f.Error != "" {
			c.filmError = f.Error
			f.Sealed = true
			return
		}
		f.Film.End = replay.End{Reason: reason, Steps: f.Ordinal, Checkpoint: p.FilmCheckpoint(), Summary: p.frozenFilmSummary(f.InitialKinds, 0)}
		f.Film.Header.Complete = true
		f.Sealed = true
		if f.Spool != nil {
			if e := f.Spool.Seal(f.Film); e != nil {
				f.failRecording("seal", e)
				c.filmError = f.Error
				return
			}
		}
		if f.Error == "" {
			c.filmPending = f.Film
		}
	}
}

// SealRecordedFilm returns the legacy, inspectable snapshot to explicit diagnostic callers.
func (c *FrontEnd) SealRecordedFilm() (*replay.Film, error) {
	f, e := c.sealedRecording()
	if e != nil || f == nil {
		return f, e
	}
	return f.Snapshot()
}
func (c *FrontEnd) sealedRecording() (*replay.Film, error) {
	if c.filmError != "" {
		return nil, fmt.Errorf("%s", c.filmError)
	}
	if c.filmPending != nil {
		return c.filmPending, c.filmPending.Validate()
	}
	p := c.Rendering
	if p == nil || p.filmRecorder == nil {
		return nil, nil
	}
	f := p.filmRecorder
	if f.Error != "" {
		return nil, fmt.Errorf("%s", f.Error)
	}
	if !f.Sealed {
		return nil, fmt.Errorf("film has not reached termination")
	}
	return f.Film, f.Film.Validate()
}

func NewFilmPlayback(a *assetdata.OfficeAssets, f *replay.Film, options configuration.RuntimeOptions) (out *FilmPlayback, err error) {
	defer func() {
		if v := recover(); v != nil {
			out = nil
			err = fmt.Errorf("film initialization has invalid source owners: %v", v)
		}
	}()
	if a == nil || a.World == nil || a.Cockpit == nil || a.Missions == nil {
		return nil, fmt.Errorf("film needs complete prepared simulation resources")
	}
	if e := f.Validate(); e != nil {
		return nil, e
	}
	compatible, e := filmMatchesInstalledResources(f, a)
	if e != nil {
		return nil, e
	}
	if !compatible || f.Header.ClockPolicy != flight.NativeClockPolicy {
		return nil, fmt.Errorf("film needs a different resource or clock profile")
	}
	var launch filmLaunch
	d := json.NewDecoder(bytes.NewReader(f.Launch))
	d.DisallowUnknownFields()
	if e = d.Decode(&launch); e != nil {
		return nil, e
	}
	if e = d.Decode(new(any)); e != io.EOF {
		return nil, fmt.Errorf("trailing launch data")
	}
	if f.R83Mechanics() && !launch.Runtime.AvoidancePolicy.Valid() {
		return nil, fmt.Errorf("R8.3 film lacks a valid avoidance policy")
	}
	if !f.Legacy() && f.End.Summary == nil {
		return nil, fmt.Errorf("current film needs frozen source summary")
	}
	if !f.BlockStorage() {
		if e = validateFilmOperations(f); e != nil {
			return nil, e
		}
	}
	final, e := cloneFilmFinal(&launch.Final)
	if e != nil {
		return nil, e
	}
	if final.Mission == nil || final.Mission.SourceWorld == nil || len(final.Mission.Objects) != 80 || len(final.Aircraft) != 1 && len(final.Aircraft) != 5 {
		return nil, fmt.Errorf("film launch owner incomplete")
	}
	if final.Mission.Inputs.MissionType == gamestate.MissionSimulated && !f.Authored() {
		return nil, fmt.Errorf("authored Simulation needs its explicit initialization profile")
	}
	if launch.AuthoredSource != nil {
		if !f.P5Mechanics() || !f.Authored() || len(filmJSON(launch.AuthoredSource)) > 4<<20 {
			return nil, fmt.Errorf("authored source needs the bounded P5 authored interpretation")
		}
		if e = gamestate.ValidateAuthoredFilmSource(a.Missions.Generation, launch.AuthoredSource, final.Mission); e != nil {
			return nil, fmt.Errorf("film authored source: %w", e)
		}
	}
	engine := &FrontEnd{simulationProfile: f.Header.Simulation, Office: &Office{Assets: a}, Runtime: launch.Runtime, filmIsolation: &filmSessionIsolation{Mode: "playback"}, Screen: ScreenScene, Mission: &MissionSession{Data: final.Mission, Final: final, Flight: final.Flight, ReadinessRows: final.Aircraft, PilotID: final.PilotID, Settings: final.Settings}}
	if f.StationaryEnemyMechanics() {
		if launch.EnemyRules == nil || launch.EnemyRules.Disabled != launch.Runtime.DisableEnemyAI {
			return nil, fmt.Errorf("recorded hostile policy is incomplete or inconsistent")
		}
		engine.enemyLaunchRules = launch.EnemyRules
	} else if launch.EnemyRules != nil {
		return nil, fmt.Errorf("historical film cannot introduce hostile definitions")
	}
	if e = engine.constructSimulation(final); e != nil {
		return nil, e
	}
	p := engine.Rendering
	// Constructor owners are identical. Select the recorded simulation rules
	// before any tick: historical films omit the P.3 formation/RNG visits.
	p.Flight.Combat.SetFormationEnabled(f.FormationMechanics())
	p.Flight.Combat.SetLogisticsEnabled(f.P4Mechanics())
	p.p4Mechanics = f.P4Mechanics()
	p.p5Mechanics = f.P5Mechanics()
	p.Flight.Combat.SetMissionActorsEnabled(p.p5Mechanics)
	p.p6Mechanics = f.P6Mechanics()
	p.Flight.Combat.SetTrainEnabled(p.p6Mechanics)
	p.p7Mechanics = f.P7Mechanics()
	p.Flight.Combat.SetFeedbackEnabled(p.p7Mechanics)
	p.r11Mechanics = f.R11Mechanics()
	p.r4Mechanics = f.ReadyRepairsMechanics()
	p.r6Mechanics = f.R6Mechanics()
	p.r8Mechanics = f.R8Mechanics()
	p.r5Mechanics = f.SafeGameplayMechanics()
	p.cameraContactMechanics = f.CameraContactMechanics()
	p.routeAvoidanceMechanics = f.RouteAvoidanceMechanics()
	p.waypointReleaseMechanics = f.WaypointReleaseMechanics()
	p.navigationMechanics = f.NavigationMechanics()
	p.wingmanEngagementCompression = f.WingmanEngagementCompression()
	p.r1Mechanics = f.R1Mechanics()
	p.wingmenAutonomy = !p.r1Mechanics || !launch.Runtime.DisableWingmenAI
	p.Flight.Combat.SetOrdersEnabled(p.r1Mechanics)
	p.Flight.Combat.SetSafeFlightOrdersEnabled(p.r5Mechanics)
	p.Flight.Combat.SetDispatcherScratchEnabled(p.r11Mechanics)
	p.Flight.Combat.SetRouteAvoidanceEnabled(p.routeAvoidanceMechanics)
	p.Flight.Combat.SetWaypointReleaseEnabled(p.waypointReleaseMechanics)
	p.Flight.Combat.SetNavigationEnabled(p.navigationMechanics)
	p.Flight.Combat.SetWingmenAutonomyEnabled(p.wingmenAutonomy)
	p.Flight.Combat.SetPersistentPlayerThreat(f.PlayerCopilotMechanics())
	p.filmLegacy = f.Legacy()
	p.Flight.Notifications.LegacyO = f.Legacy()
	if launch.Adjustment != nil {
		if e = p.applyFilmInitial(*launch.Adjustment, a); e != nil {
			return nil, e
		}
	}
	if group := compareFilmCheckpoint(launch.Initial, p.FilmCheckpoint()); group != "" {
		return nil, fmt.Errorf("film initial %s state differs", group)
	}
	// Presentation resolution/mode belongs to the viewer, never construction.
	applyReplayPresentationOptions(p, engine, options)
	pb := &FilmPlayback{Engine: engine, Film: f, initialKinds: p.filmInitialKinds()}
	if f.End.Summary != nil && f.End.Summary.Kinds != pb.initialKinds {
		return nil, fmt.Errorf("film summary launch kinds differ")
	}
	p.filmPlayback = pb
	p.installFilmObserver()
	return pb, nil
}
func (q *FilmPlayback) Preview() *RenderingPreview {
	if q == nil || q.Engine == nil {
		return nil
	}
	return q.Engine.Rendering
}
func (q *FilmPlayback) fail(e error) error {
	q.FailureDiagnostic = q.captureFilmFailure(e)
	q.Divergence = fmt.Sprintf("Film stopped at operation%d: %v", q.Cursor, e)
	q.Done = true
	q.reachedCheckpoint = nil
	q.current = nil
	return fmt.Errorf("%s", q.Divergence)
}
func (q *FilmPlayback) Step() (err error) {
	defer func() {
		if v := recover(); v != nil {
			err = q.fail(fmt.Errorf("invalid isolated film operation: %v", v))
		}
	}()
	if q == nil || q.Done || q.Practice {
		return nil
	}
	if q.Cursor >= q.Film.OperationCount() {
		if q.Film.BlockStorage() && q.Cursor == 0 {
			return q.fail(fmt.Errorf("empty logical stream"))
		}
		q.Done = true
		return nil
	}
	if e := q.applyVerifiedRecovery(); e != nil {
		return q.fail(e)
	}
	o, e := q.Film.OperationAt(q.Cursor)
	if e != nil {
		return q.fail(e)
	}
	if q.Film.BlockStorage() {
		steps := q.reachedSteps
		ended := false
		if e := validateFilmOperation(q.Film, o, &steps, &ended); e != nil {
			return q.fail(e)
		}
		if ended != (q.Cursor+1 == q.Film.OperationCount()) || ended && steps != q.Film.End.Steps {
			return q.fail(fmt.Errorf("incomplete logical stream"))
		}
	}
	q.reachedCheckpoint = nil
	q.reachedSupport = nil
	defer func() { q.reachedCheckpoint = nil; q.current = nil }()
	p := q.Preview()
	q.events = nil
	q.reached = false
	switch o.Kind {
	case "step", "terminal":
		var v filmStep
		if e := json.Unmarshal(o.Payload, &v); e != nil {
			return q.fail(e)
		}
		if v.Ordinal != q.reachedSteps+1 {
			return q.fail(fmt.Errorf("logical ordinal differs"))
		}
		q.current = &v
		q.terminal = o.Kind == "terminal"
		t := p.Flight.Clock.ReplayTiming()
		if t != v.Timing {
			return q.fail(fmt.Errorf("recorded timing differs from source counter"))
		}
		p.Flight.filmTiming = &t
		p.Flight.Paused = false
		p.Paused = false
		p.Flight.terminalAwaitingAck = false
		if e := q.Engine.advanceFlight(0); e != nil {
			return q.fail(e)
		}
		if !q.reached {
			return q.fail(fmt.Errorf("logical operation was not reached"))
		}
		q.current = nil
		q.reachedSteps++
	case "boundary":
		var b filmBoundary
		if e := json.Unmarshal(o.Payload, &b); e != nil {
			return q.fail(e)
		}
		if e := q.Engine.applyFilmBoundary(b); e != nil {
			return q.fail(e)
		}
	case "end":
		var b struct{ Reason string }
		if e := json.Unmarshal(o.Payload, &b); e != nil {
			return q.fail(e)
		}
		if p.Initialized.Sortie == nil || p.Initialized.Sortie.Result == nil {
			if p.Initialized.Sortie != nil && p.Initialized.Sortie.Pending != "" {
				p.Initialized.AnswerNaturalEnd(true)
			} else {
				p.Initialized.EndSortieForQoL()
			}
		}
		if e := q.Engine.finishSortieResult(); e != nil {
			return q.fail(e)
		}
	default:
		return q.fail(fmt.Errorf("unknown operation"))
	}
	actual := q.reachedCheckpoint
	if actual == nil {
		actual = p.FilmCheckpoint()
	}
	if group := compareFilmCheckpoint(o.Checkpoint, actual); group != "" {
		return q.fail(fmt.Errorf("authoritative %s checkpoint differs", group))
	}
	if !bytes.Equal(filmJSON(o.Events), filmJSON(q.events)) {
		return q.fail(fmt.Errorf("source event sequence differs"))
	}
	q.Cursor++
	if q.Cursor == q.Film.OperationCount() {
		if e := q.validateFilmSummary(); e != nil {
			return q.fail(e)
		}
		q.Done = true
	}
	return nil
}
func (q *FilmPlayback) Resume() error {
	if q == nil || q.Divergence != "" || q.Engine == nil || q.Recovery != nil && !q.recoveryVerified {
		return fmt.Errorf("this film cannot resume")
	}
	p := q.Preview()
	if p.Initialized.Sortie != nil && p.Initialized.Sortie.Result != nil || p.Flight.State.Terminal != nil {
		return fmt.Errorf("film has already reached termination")
	}
	q.Practice = true
	q.Done = false
	q.Engine.filmIsolation.Mode = "practice"
	q.Engine.filmIsolation.Done = false
	p.filmPlayback = nil
	// Only future feedback belongs to practice. The verified reconstruction,
	// logical film interpretation and all reached state remain untouched.
	if q.Engine.Office != nil && q.Engine.Office.Assets != nil && q.Engine.Office.Assets.Audio.SupportsDefenseReleaseAudio() {
		p.practiceFeedback = true
		p.Flight.Combat.SetFeedbackEnabled(true)
	}
	if p.GPS != nil {
		p.GPS.UseSourceSupportInterlocks()
	}
	p.filmLegacy = false
	p.Flight.Notifications.LegacyO = false
	p.clearFlightInput()
	p.Flight.Compression.Reset()
	p.Flight.Paused = false
	p.Paused = false
	return nil
}
func (q *FilmPlayback) HandlePractice(a Actions) error {
	if !q.Practice {
		return nil
	}
	c := q.Engine
	if a.Quit || a.Close {
		c.filmIsolation.Done = true
		q.Done = true
		return nil
	}
	if a.EndMission {
		if c.Rendering.GPS != nil {
			c.closeGPS()
		}
		// The isolated confirmation owns the same saved framebuffer as live Alt+E.
		// It must never borrow the parent library or invoke its persistent dispatch.
		c.quitFadeBackground = nil
		if c.fade != nil {
			c.quitFadeBackground = c.fadeSource()
		}
		c.quitBackground = c.frameBody()
		c.sceneEndConfirmation = true
		c.previousScreen = ScreenScene
		c.Screen = ScreenQuit
		c.Rendering.clearFlightInput()
		return nil
	}
	if c.Screen == ScreenQuit {
		if a.ConfirmQuit {
			return c.confirmSortieEnd()
		}
		for _, r := range a.Text {
			if r == 'y' || r == 'Y' {
				return c.confirmSortieEnd()
			}
		}
		if a.AnyKey || a.Cancel || a.Selector1 {
			c.cancelNaturalSortieEnd()
			c.Screen = ScreenScene
			c.Rendering.clearFlightInput()
		}
		return nil
	}
	return c.handleFlight(a)
}
func (q *FilmPlayback) AdvancePractice(seconds float64) error {
	if !q.Practice || q.Done {
		return nil
	}
	if q.Engine.Screen == ScreenScene {
		if e := q.Engine.AdvanceRendering(seconds); e != nil {
			return e
		}
	}
	q.Done = q.Engine.filmIsolation.Done
	return nil
}
func (c *FrontEnd) applyFilmBoundary(b filmBoundary) error {
	switch b.Kind {
	case "control":
		a := b.Action
		r := c.Rendering.Flight
		if a.CompressionIncrease {
			r.Compression.Increase()
			r.commandInput.LogicalSampling = true
		}
		if a.CompressionReset {
			r.Compression.Reset()
			r.Clock.ResetElapsed()
		}
		if a.SpeechToggle {
			r.SpeechEnabled = !r.SpeechEnabled
		}
		if a.NightVisionToggle && c.Rendering.PaletteState != nil {
			c.Rendering.PaletteState.Toggle(c.Rendering.Initialized.Switches)
		}
		c.Rendering.Cockpit.CompressionRequest = r.Compression.Request
		return c.handleSceneView(a)
	case "natural-end-cancel":
		p := c.Rendering
		if p == nil || p.Initialized == nil || p.Initialized.Sortie == nil || p.Initialized.Sortie.Pending == "" || p.Initialized.PlayerFlight == nil {
			return fmt.Errorf("natural-end cancellation lacks reached pending source prompt")
		}
		p.Initialized.AnswerNaturalEnd(false)
		return nil
	case "reference":
		if b.Count < 1 || b.Count > 16 {
			return fmt.Errorf("film reference count outside bound")
		}
		return c.StepRendering(b.Count)
	case "gps-tick":
		return c.applyFilmGPSTick(b.GPSTick)
	case "gps-release":
		if c.Rendering.GPS == nil || !c.Rendering.Flight.gpsNeedsRelease {
			return fmt.Errorf("GPS release not pending")
		}
		c.Rendering.Flight.gpsNeedsRelease = false
		return nil
	case "gps-support":
		if b.Support == nil || c.Rendering.filmPlayback == nil {
			return fmt.Errorf("missing ordered support command")
		}
		q := c.Rendering.filmPlayback
		if !q.Film.SupportInput() {
			return fmt.Errorf("unsupported support input profile")
		}
		q.reachedSupport = nil
		if e := c.handleGPS(b.Action); e != nil {
			return e
		}
		if !bytes.Equal(filmJSON(b.Support), filmJSON(q.reachedSupport)) {
			return fmt.Errorf("resolved support command differs")
		}
		return nil
	case "gps":
		return c.handleGPS(b.Action)
	case "gps-open":
		return c.openGPS()
	case "gps-close":
		c.closeGPS()
		return nil
	case "reset":
		if b.GPSRelease != nil {
			c.Rendering.Flight.gpsNeedsRelease = *b.GPSRelease
		}
		c.Rendering.clearFlightInput()
		return nil
	case "hit":
		if b.Hit == nil {
			return fmt.Errorf("missing accepted-hit input")
		}
		h := b.Hit
		return c.Rendering.QueueAcceptedPlayerHit(AcceptedPlayerHit{VictimRow: h.Victim, ProjectileFlags: h.Flags, Source: h.Source, developmentSeed: h.Seed})
	default:
		return fmt.Errorf("unknown film transaction %q", b.Kind)
	}
}

func (r *FlightRuntime) nextFilmOrClock() (flight.Timing, bool) {
	if r.filmTiming != nil {
		v := *r.filmTiming
		r.filmTiming = nil
		return v, true
	}
	return r.Clock.PopScheduled()
}
func (c *FrontEnd) initializeSortieCapability(final *FinalPreflightConfiguration) error {
	if c.filmIsolation == nil {
		return c.beginSortieResult(final)
	}
	if c.Store != nil {
		return fmt.Errorf("isolated film must not own a save store")
	}
	p := c.Rendering
	if e := gamestate.PrepareMissionTargets(c.Office.Assets.Missions, final.Mission); e != nil {
		return e
	}
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
	return nil
}
func (p *RenderingPreview) recordFilmReset(pausedFocus bool) {
	f := p.filmRecorder
	if p.filmTransaction || f == nil || f.Sealed || f.Pending != nil || p.Initialized.Sortie != nil && p.Initialized.Sortie.Result != nil {
		return
	}
	release := p.Flight.gpsNeedsRelease
	if pausedFocus && f.Error == "" {
		r := p.Flight
		state := focusReset{operations: f.operationCount(), steps: r.Steps, gps: p.GPS, release: release, control: p.filmControl(), defenses: r.Defenses, damage: r.Systems.DamageMasks}
		if state.defenses.R3 != nil {
			state.ownership = *state.defenses.R3
			state.defenses.R3 = nil
		}
		if f.focusResetValid && f.lastFocusReset == state {
			return
		}
		f.lastFocusReset = state
		f.focusResetValid = true
	} else {
		f.focusResetValid = false
	}
	f.appendOperation(replay.Operation{Kind: "boundary", Payload: filmJSON(filmBoundary{Kind: "reset", GPSRelease: &release}), Checkpoint: p.FilmCheckpoint()})
	if f.focusResetValid {
		f.lastFocusReset.operations = f.operationCount()
	}
	f.PreviousControl = filmJSON(p.filmControl())
}

func (f *filmRecorder) operationCount() int {
	if f.Spool != nil {
		return f.Spool.Count()
	}
	return len(f.Film.Operations)
}
func (f *filmRecorder) appendOperation(o replay.Operation) {
	// Paused host polls can release already released input. Retain the first
	// real reset, but not another identical no-op with every owner unchanged.
	// A source step, event or other boundary always prevents this coalescing.
	if o.Kind == "boundary" && len(o.Events) == 0 && isFilmReset(o.Payload) && f.operationCount() > 0 {
		prev := f.lastOperation
		if f.Spool == nil {
			prev = f.Film.Operations[f.operationCount()-1]
		}
		if prev.Kind == "boundary" && len(prev.Events) == 0 && bytes.Equal(prev.Payload, o.Payload) && bytes.Equal(filmJSON(prev.Checkpoint), filmJSON(o.Checkpoint)) {
			return
		}
	}
	if f.Error != "" || f.Sealed {
		return
	}
	if f.operationCount() >= replay.MaxOperations {
		f.failRecording("operation-count", fmt.Errorf("Film operation capacity reached (%d)", replay.MaxOperations))
		return
	}
	if f.Spool != nil {
		if e := f.Spool.Append(o); e != nil {
			f.failRecording("append", e)
			return
		}
	}
	if f.Spool == nil {
		f.Film.Operations = append(f.Film.Operations, o)
	} else {
		f.lastOperation = o
	}
}
func (f *filmRecorder) failRecording(phase string, err error) {
	if f == nil || f.Error != "" || err == nil {
		return
	}
	f.Error = err.Error()
	diagnostics.Warn("replay", "Recording stopped after first failure; valid career remains independent", "phase", phase, "session", f.Film.Header.ID, "operation", f.operationCount()+1, "step", f.Ordinal, "error", err)
	f.Pending = nil
	f.Events = nil
	if f.Spool != nil {
		f.Spool.RecordFailure(phase, uint64(f.operationCount()+1), f.Ordinal, err)
		_ = f.Spool.Close()
	}
}

// AbandonFilmRecording only closes this uncommitted film spool. It does not
// publish/delete films or touch career, consumed IDs, Campaign or Phoenix.
func (c *FrontEnd) AbandonFilmRecording() {
	if c.Rendering != nil && c.Rendering.filmRecorder != nil {
		f := c.Rendering.filmRecorder
		if f.Spool != nil {
			if e := f.Spool.Close(); e != nil {
				f.failRecording("abandon-flush", e)
			}
		}
	}
}

func decodeFilmPayload(raw []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return e
	}
	if e := d.Decode(new(any)); e != io.EOF {
		return fmt.Errorf("trailing operation input")
	}
	return nil
}
func validateFilmOperations(f *replay.Film) error {
	var steps uint64
	ended := false
	for _, o := range f.Operations {
		if e := validateFilmOperation(f, o, &steps, &ended); e != nil {
			return e
		}
	}
	if !ended || steps != f.End.Steps {
		return fmt.Errorf("incomplete logical stream")
	}
	return nil
}

func validateFilmOperation(f *replay.Film, o replay.Operation, steps *uint64, ended *bool) error {
	if *ended {
		return fmt.Errorf("operations after end")
	}
	groups := []string{"flight", "mission", "combat", "navigation", "environment", "reference", "pending-hit", "palette", "visual"}
	if f.P4Mechanics() {
		groups = append(groups, "appearance")
	}
	if f.R1Mechanics() {
		groups = append(groups, "orders-r1")
	}
	if !f.Legacy() {
		groups = append(groups, "modal")
	}
	if f.EnginePowerMechanics() || f.Header.Simulation == replay.U26Compatibility || f.Header.Simulation == replay.U26AuthoredCompatibility {
		groups = append(groups, "collective-owner")
	}
	if _, ok := o.Checkpoint["controls-q3"]; ok && replay.ContinuousInput(f.Header.InputSchema) {
		groups = append(groups, "controls-q3")
	}
	if len(o.Checkpoint) != len(groups) {
		return fmt.Errorf("unsupported checkpoint owner domain")
	}
	for _, g := range groups {
		v := o.Checkpoint[g]
		b, e := hex.DecodeString(v)
		if e != nil || len(b) != 32 {
			return fmt.Errorf("invalid %s checkpoint", g)
		}
	}
	switch o.Kind {
	case "step", "terminal":
		var v filmStep
		if e := decodeFilmPayload(o.Payload, &v); e != nil {
			return e
		}
		if replay.HeldInput(f.Header.InputSchema) && v.Input.Jammers == nil {
			return fmt.Errorf("v7 held-jammer operand missing")
		}
		if v.Input.Live != nil {
			if !replay.ContinuousInput(f.Header.InputSchema) || !v.Input.Live.Valid() || o.Checkpoint["controls-q3"] == "" {
				return fmt.Errorf("unsupported continuous input/checkpoint domain")
			}
		}
		if !replay.HeldInput(f.Header.InputSchema) && (v.Input.Jammers != nil || v.Input.Live != nil && (v.Input.Live.Mode != "" || v.Input.Live.Slider)) {
			return fmt.Errorf("new input operand in historical film")
		}
		(*steps)++
		if v.Ordinal != *steps || v.Control != nil || len(v.Input.Aux) > 32 || v.Input.Command < 0 || (v.Input.Command & ^0x1ff) != 0 || (v.Input.Command&0xff) >= len(FlightKeys) || v.Input.Command&0x100 != 0 && (v.Input.Command&0xff) >= 2 {
			return fmt.Errorf("invalid logical input")
		}
		for _, a := range v.Input.Aux {
			if (a < 0 || a > 10) && !(f.ReadyRepairsMechanics() && (a == 11 || a == 12)) && !(f.R1Mechanics() && a >= 0x108 && a < 0x128) {
				return fmt.Errorf("invalid auxiliary input")
			}
		}
	case "boundary":
		var b filmBoundary
		if e := decodeFilmPayload(o.Payload, &b); e != nil {
			return e
		}
		if !f.Legacy() {
			if b.Kind != "gps-support" && b.Support != nil || b.Kind != "reference" && b.Count != 0 || b.Kind != "hit" && b.Hit != nil || b.Kind != "gps-tick" && b.GPSTick != nil || b.Kind != "reset" && b.GPSRelease != nil {
				return fmt.Errorf("unexpected film transaction carrier")
			}
			if b.Kind == "reset" && b.GPSRelease == nil {
				return fmt.Errorf("missing reset release witness")
			}
			if len(b.Action.GPSKey) > 8 || len(b.Action.Text) > 128 {
				return fmt.Errorf("modal action outside bounded input domain")
			}
		}
		switch b.Kind {
		case "control", "gps", "gps-open", "gps-close", "reset":
			if f.Legacy() && b.GPSRelease != nil {
				return fmt.Errorf("new reset in legacy film")
			}
		case "natural-end-cancel":
			if (f.Header.InputSchema != replay.InputCompatibility && !f.SupportInput()) || !bytes.Equal(filmJSON(b.Action), filmJSON(Actions{})) || len(o.Events) != 0 {
				return fmt.Errorf("unsupported natural-end cancellation transaction profile/carrier")
			}
		case "gps-support":
			if !f.SupportInput() || b.Support == nil || b.Support.Point.H0 > 0x7ffff || b.Support.Point.H1 > 0x7ffff {
				return fmt.Errorf("unsupported support transaction carrier/profile")
			}
		case "gps-tick":
			if f.Legacy() || b.GPSTick == nil {
				return fmt.Errorf("unsupported GPS tick profile")
			}
			if err := b.GPSTick.validate(); err != nil {
				return err
			}
		case "gps-release":
			if f.Legacy() {
				return fmt.Errorf("unsupported GPS release profile")
			}
		case "reference":
			if b.Count < 1 || b.Count > 16 {
				return fmt.Errorf("invalid reference count")
			}
		case "hit":
			if b.Hit == nil || b.Hit.Victim != 0 || b.Hit.Flags&^uint16(0x40) != 0 || len(b.Hit.Source) > 512 {
				return fmt.Errorf("invalid accepted hit")
			}
		default:
			return fmt.Errorf("unsupported boundary kind")
		}
	case "end":
		var v struct{ Reason string }
		if e := decodeFilmPayload(o.Payload, &v); e != nil {
			return e
		}
		if len(v.Reason) > 256 {
			return fmt.Errorf("invalid ending")
		}
		*ended = true
	}
	return nil
}

func filmPendingHits(in []AcceptedPlayerHit) []filmHit {
	var out []filmHit
	for _, h := range in {
		out = append(out, filmHit{h.VictimRow, h.ProjectileFlags, h.Source, h.developmentSeed})
	}
	return out
}
