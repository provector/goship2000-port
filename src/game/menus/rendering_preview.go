package menus

import (
	"encoding/binary"
	"fmt"
	"gunship2000-assets/game/diagnostics"
	"image/color"
	"math"
	"time"

	"gunship2000-assets/game/cockpit"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/navigation"
	"gunship2000-assets/game/render3d"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/game/world"
)

// This cadence is a native inspection policy, not an original simulation rate.
// Mission time, unit poses and instrument values never advance with this clock.
const DefaultPreviewVisualRate = configuration.PreviewVisualRate

type RenderingPreview struct {
	sessionPoint                 *missionPoint
	replayHashes                 *replayHashCache
	targetFrame                  []byte
	r4Mechanics                  bool // R4 caller-written avoidance and logical recall/jettison.
	r6Mechanics                  bool // R6 launch-frozen victim ammunition ownership.
	correctiveFlight             bool
	enginePower                  bool
	resilientFlight              bool
	lowCityClearance             bool
	obstacleClearance            bool
	corridorNavigation           bool
	routeAcquisitionRetry        bool
	independentManeuvers         bool
	carrierBerths                [5]navigation.Point
	admitExitWaypoint            bool
	r8Mechanics                  bool // R8 launch-frozen critical read and target revalidation.
	r5Mechanics                  bool // R5 explicit state policies; frozen before construction.
	fieldCorrectionsMechanics    bool // R221 launch-frozen mast/logistics interpretation.
	threatFeedbackMechanics      bool // R231 sourcewarning lamps/audio and reviewed damage assist.
	copilotMechanics             bool // R3 late58F2 and explicit manual/automatic defense ownership.
	enemyBehaviorMechanics       bool // R23 all six hostile callers, cold ingress and fixed/submarine owners.
	wingmanDamageMechanics       bool // R22 per-victim defense/full damage; chosen before launch/checkpoints.
	stationaryEnemyMechanics     bool
	filmRecorder                 *filmRecorder
	filmLegacy                   bool // Strict historical O interpretation, not a save/presentation policy.
	formationMechanics           bool
	p4Mechanics                  bool // Launch-selected source contact/feedback interpretation.
	p6Mechanics                  bool // Launch-selected P6 Train execution.
	p7Mechanics                  bool // Launch-selected P7 supported feedback producers.
	r11Mechanics                 bool // Shared dispatcher scratch provenance; frozen at launch.
	navigationMechanics          bool // R11v5 bounded native waypoint/return paths.
	wingmanEngagementCompression bool // R12: accepted healthy wingman release limits accelerated time.
	waypointReleaseMechanics     bool // R11v4 heading-only kind23 release; original kind22 return.
	routeAvoidanceMechanics      bool // R11v3 native ordered-avoidance release debounce.
	cameraContactMechanics       bool // R11v2 source camera314c floors; frozen before camera initialization.
	r1Mechanics                  bool // Source orders/cache/route phases selected before initialization.
	wingmenAutonomy              bool // Frozen native policy; independent of player CP/G.
	practiceFeedback             bool // Current output policy after verified isolated takeover only.
	p5Mechanics                  bool // Launch-selected P5 mission-runtime interpretation.
	filmTransaction              bool
	filmPlayback                 *FilmPlayback
	GPS                          *navigation.GPS
	previousVisual               render3d.VisualState
	visualTime                   int64
	hostTimeFraction             float64
	presentation                 scenePresentation
	PaletteState                 *render3d.PaletteState
	Renderer                     *render3d.Renderer
	Scene                        render3d.Scene
	Visual                       render3d.VisualState
	Cockpit                      cockpit.Inputs
	Palette                      color.Palette
	Frame                        []byte
	avionicsReference            []byte // Canonical source-sized logical display feedback; never presentation output.
	avionicsReferenceBottom      int
	worldFrame                   []byte // Retained world destination excludes cockpit/HUD overlay pixels.
	worldSkyMask, skyMask        []byte // Presentation-only source sky pair/coverage planes.
	targetBrownMask              []byte // Private full-scene target-MFD material ownership; never logical feedback.
	collectMaterialMask          bool
	overlayScratch               cockpit.OverlayScratch

	Stats              render3d.Stats
	Paused             bool
	pauseUISeconds     float64
	Rate               float64
	clock              tickAccumulator
	Dirty              bool
	Error              error
	DiagnosticPolicy   string
	MissionObjectHash  string
	SourceMissionWorld string
	SkippedRows        []string
	instances          []render3d.Instance
	mastHeld           [4]bool
	Initialized        *gamestate.InitializedMission
	ViewControls       *render3d.CameraController
	Inspection         bool
	Flight             *FlightRuntime
	playerShake        playerShakePresentation   // Private VGA display-start state; never canonical feedback/checkpoints.
	parkedDisplay      parkedDisplayState        // Private model spacing; canonical positions and feedback stay original.
	ResultTargets      *gamestate.MissionTargets // View alias of Initialized.Targets; not career admission.
}

func (c *FrontEnd) renderingPresented() bool {
	return c.Screen == ScreenScene || c.Screen == ScreenQuit && c.previousScreen == ScreenScene
}

// The same committed MissionSession supplies this view. There is no second
// generator, outcome, recovery snapshot, cache import or save transaction here.
func (c *FrontEnd) enterRenderingPreview(final *FinalPreflightConfiguration) error {
	// Freeze typed accepted input before any mutable launch consumer. Film I/O
	// must never prevent a supported live launch.
	frozen, filmErr := cloneFilmFinal(final)
	if err := c.constructSimulation(final); err != nil {
		return err
	}
	if c.Rendering.r1Mechanics {
		diagnostics.Info("wingmen", "Sortie rules frozen", "autonomy", c.Rendering.wingmenAutonomy, "orders", true, "hostile_decisions", c.Rendering.Initialized.EnemyRules != nil && !c.Rendering.Initialized.EnemyRules.Disabled)
	}
	if filmErr == nil {
		c.beginFilmRecording(frozen)
	} else {
		diagnostics.Warn("replay", "Launch recording snapshot unavailable; live simulation continues", "error", filmErr)
	}
	return nil
}

// constructSimulation is shared by live and isolated films. Only the live
// capability below owns settings and result persistence.
func (c *FrontEnd) constructSimulation(final *FinalPreflightConfiguration) error {
	if final == nil || final.Mission == nil || final.Mission.SourceWorld == nil || len(final.Aircraft) == 0 {
		return fmt.Errorf("render preview needs an accepted preparation")
	}
	if c.Office.Assets.World == nil || c.Office.Assets.Cockpit == nil {
		return fmt.Errorf("world/cockpit preparation is missing; rebuild supported assets")
	}
	m := final.Mission
	r5 := c.safeGameplayProfile()
	r4 := r5 || c.simulationProfile == "" || c.simulationProfile == replay.R4Compatibility || c.simulationProfile == replay.R4AuthoredCompatibility
	copilot := r4 || c.simulationProfile == replay.R3Compatibility || c.simulationProfile == replay.R3AuthoredCompatibility
	feedback := copilot || c.simulationProfile == replay.R231Compatibility || c.simulationProfile == replay.R231AuthoredCompatibility
	enemyBehavior := feedback || c.simulationProfile == replay.R23Compatibility || c.simulationProfile == replay.R23AuthoredCompatibility
	fieldCorrections := enemyBehavior || c.simulationProfile == replay.R221Compatibility || c.simulationProfile == replay.R221AuthoredCompatibility
	wingmanDamage := fieldCorrections || c.simulationProfile == replay.R22Compatibility || c.simulationProfile == replay.R22AuthoredCompatibility
	enemyMechanics := wingmanDamage || c.simulationProfile == replay.R21Compatibility || c.simulationProfile == replay.R21AuthoredCompatibility
	engagementCompression := enemyMechanics || c.simulationProfile == replay.R12Compatibility || c.simulationProfile == replay.R12AuthoredCompatibility
	navigationPolicy := engagementCompression || c.simulationProfile == replay.R11NavigationCompatibility || c.simulationProfile == replay.R11NavigationAuthoredCompatibility
	waypointRelease := navigationPolicy || c.simulationProfile == replay.R11WaypointReleaseCompatibility || c.simulationProfile == replay.R11WaypointReleaseAuthoredCompatibility
	avoidance := waypointRelease || c.simulationProfile == replay.R11AvoidanceCompatibility || c.simulationProfile == replay.R11AvoidanceAuthoredCompatibility
	cameraContact := avoidance || c.simulationProfile == replay.R11CameraCompatibility || c.simulationProfile == replay.R11CameraAuthoredCompatibility
	r11 := cameraContact || c.simulationProfile == replay.R11Compatibility || c.simulationProfile == replay.R11AuthoredCompatibility
	r1 := r11 || c.simulationProfile == replay.R1Compatibility || c.simulationProfile == replay.R1AuthoredCompatibility
	p7 := r1 || c.simulationProfile == replay.P7Compatibility || c.simulationProfile == replay.P7AuthoredCompatibility
	p6 := p7 || c.simulationProfile == replay.P6Compatibility || c.simulationProfile == replay.P6AuthoredCompatibility
	p5 := p6 || c.simulationProfile == replay.P5Compatibility || c.simulationProfile == replay.P5AuthoredCompatibility
	validator := gamestate.ValidateSupportLaunch
	if p5 {
		validator = gamestate.ValidateSupportLaunchP5
	}
	if err := validator(m); err != nil {
		return err
	}
	renderer, err := render3d.New(c.Office.Assets.World)
	if err != nil {
		return err
	}
	wid := m.SourceWorld.TerrainName
	if c.Office.Assets.World.GraphicsMode != 0 {
		return fmt.Errorf("render preview supports selected VGA mode0, source graphics mode%d is unsupported", c.Office.Assets.World.GraphicsMode)
	}
	if _, ok := c.Office.Assets.World.Worlds[wid]; !ok {
		return fmt.Errorf("selected mission terrain %s is not prepared", wid)
	}
	if c.Office.Assets.Missions.Generation == nil || c.Office.Assets.Missions.Generation.Tables.Initialization == nil {
		return fmt.Errorf("mission launch tables missing; rebuild supported assets")
	}
	tables := c.Office.Assets.Missions.Generation.Tables.Initialization
	switches, err := gamestate.LaunchSwitches(m, final.Settings, tables.CPGMapping)
	if err != nil {
		return err
	}
	var enemyRules *gamestate.EnemyRules
	if enemyMechanics {
		if c.simulationProfile == "" {
			enemyRules, err = freezePreparedEnemyRules(c.Office.Assets.Missions, m, switches, c.Runtime.DisableEnemyAI)
		} else {
			enemyRules = c.enemyLaunchRules
			err = enemyRules.Validate(m.World, switches)
			if err == nil {
				var expected *gamestate.EnemyRules
				expected, err = freezePreparedEnemyRules(c.Office.Assets.Missions, m, switches, enemyRules.Disabled)
				if err == nil && expected.Raw != enemyRules.Raw {
					err = fmt.Errorf("recorded enemy bank differs from selected immutable world")
				}
			}
		}
		if err != nil {
			return fmt.Errorf("hostile launch rules: %w", err)
		}
	}
	// One copied initialized world, derived from this accepted preparation. The
	// separate source-style LFSR starts from a recorded native seed policy; it
	// never consumes generation draws or edits the papers' immutable population.
	initialized, err := gamestate.InitializeMission(c.Office.Assets.Missions, c.Office.Assets.World, m, final.Aircraft, final.Flight, switches, uint16(m.Inputs.Seed), r5)
	if err != nil {
		return fmt.Errorf("mission initialization: %w", err)
	}
	initialized.Stores.VictimWingStores = c.r6Profile()
	initialized.Stores.VictimCriticalProficiency = c.r8Profile()
	initialized.EnemyRules = enemyRules
	if enemyBehavior {
		if err := initialized.InitializeEnemyBehaviors(c.Office.Assets.World); err != nil {
			return fmt.Errorf("hostile initialization: %w", err)
		}
	}
	formation, modern := launchSimulationPolicy(c.simulationProfile)
	p := &RenderingPreview{p7Mechanics: p7, p6Mechanics: p6, p5Mechanics: p5, p4Mechanics: modern, formationMechanics: formation, Renderer: renderer, Initialized: initialized, Frame: make([]byte, 64000), worldFrame: make([]byte, 64000), Rate: DefaultPreviewVisualRate, Dirty: true,
		MissionObjectHash: m.ObjectHash, SourceMissionWorld: m.World,
		DiagnosticPolicy: "Source-backed launch/first-participation initialization; autonomous decisions and motion integration disabled. Native visual counter cadence and low16 mission-seed LFSR policy are explicit; unimplemented instrument producers remain raw initial state."}
	p.Scene = render3d.Scene{World: wid, View: render3d.FullView(), GraphicsOptions: c.Office.Assets.World.GraphicsOptions, PaletteBank: render3d.EnvironmentPalette(switches),
		Environment: &render3d.EnvironmentState{Prefix: wid[:4], Switches: switches, RNG: initialized.RandomState}}
	p.r11Mechanics = r11
	p.cameraContactMechanics = cameraContact
	p.routeAvoidanceMechanics = avoidance
	p.waypointReleaseMechanics = waypointRelease
	p.navigationMechanics = navigationPolicy
	p.wingmanEngagementCompression = engagementCompression
	p.r1Mechanics = r1
	p.enemyBehaviorMechanics = enemyBehavior
	p.threatFeedbackMechanics = feedback
	p.copilotMechanics = copilot
	p.r4Mechanics = r4
	p.r6Mechanics = c.r6Profile()
	p.r8Mechanics = c.r8Profile()
	p.corridorNavigation = c.r86Profile()
	p.routeAcquisitionRetry = c.routeAcquisitionProfile()
	p.independentManeuvers = c.maneuverProfile()
	if p.independentManeuvers && initialized.Switches&0x1000 != 0 {
		// The initializer places grounded carrier crews along the deck centreline.
		// Retain those actual berths before flight moves the aircraft.
		objects := initialized.ContactObjects()
		for _, e := range initialized.Entities {
			if e.FlightSlot < 0 || e.FlightSlot >= len(p.carrierBerths) || e.Pose.Height != 0x16a || !e.Visible {
				continue
			}
			floor, ok := p.parkingFloor(e.Pose.H0, e.Pose.H1, objects)
			if ok && floor == 0x168 {
				p.Scene.DeckCrew[e.FlightSlot] = e.ID
				p.carrierBerths[e.FlightSlot] = navigation.Point{H0: e.Pose.H0, H1: e.Pose.H1}
			}
		}
	}
	p.admitExitWaypoint = c.exitWaypointProfile()
	p.correctiveFlight = c.correctiveFlightProfile()
	p.enginePower = c.enginePowerProfile()
	p.resilientFlight = c.resilientFlightProfile()
	p.lowCityClearance = c.lowCityProfile()
	p.obstacleClearance = c.t5Profile()
	p.r5Mechanics = r5
	p.stationaryEnemyMechanics = enemyMechanics
	p.wingmanDamageMechanics = wingmanDamage
	p.fieldCorrectionsMechanics = fieldCorrections
	p.wingmenAutonomy = !r1 || !c.Runtime.DisableWingmenAI
	for _, entity := range initialized.Entities {
		if !entity.Visible && !(enemyBehavior && entity.Exclusion == "" && (entity.Kind == 0 || entity.Kind == 1 || entity.Kind == 0x0a)) {
			p.SkippedRows = append(p.SkippedRows, fmt.Sprintf("row%d: %s", entity.SourceRow, entity.Exclusion))
			continue
		}
		callback := uint8(binary.LittleEndian.Uint16(entity.Object.Raw[:]) >> 8)
		if callback > 7 {
			return fmt.Errorf("row%d unsupported model callback%d", entity.SourceRow, callback)
		}
		p.instances = append(p.instances, render3d.Instance{ID: entity.ID, ModelID: entity.ModelID, MutableModel: initialized.SubmarineModels[entity.RuntimeRow],
			Pose: render3d.Pose{H0: entity.Pose.H0, H1: entity.Pose.H1, Height: entity.Pose.Height}, Angles: entity.Pose.Angles,
			Callback: callback, Oriented: binary.LittleEndian.Uint16(entity.Object.Raw[:])&8 != 0})
	}
	p.ViewControls = render3d.NewCameraController(p.cameraInputs())
	p.Scene.Camera = p.ViewControls.Camera
	row := final.Aircraft[0]
	aircraft := int(int8(row[0]))
	if _, err = cockpit.Family(aircraft); err != nil {
		return err
	}
	pose := initialized.Player
	p.Cockpit = cockpit.Inputs{AircraftID: aircraft, Arrangement: 0, CameraMode: 0, HUD: true, MastEmpty: true,
		Height: pose.Height, Heading: pose.Angles[1], Pitch: pose.Angles[0], Bank: pose.Angles[2],
		AttitudeMatrix:  [3]int16{render3d.Sine(pose.Angles[0], c.Office.Assets.World.Sine), render3d.Sine(pose.Angles[2]+0x4000, c.Office.Assets.World.Sine), render3d.Sine(pose.Angles[2], c.Office.Assets.World.Sine)},
		SelectedStation: initialized.SelectedStation, HUDVertical: initialized.InitialViewWords[4], HUDScale: initialized.InitialViewWords[5],
		Fuel: initialized.InitialFuelPercent, FuelMaximum: 100}

	// Unimplemented dynamic gauge producers remain at cold state; they do not
	// report a running engine or integrated acceleration. Pose, source-initial
	// HUD words, fuel and stores have their explicit accepted/initialized owners.
	if p.Cockpit.FuelMaximum <= 0 {
		return fmt.Errorf("accepted aircraft has no supported positive fuel divisor")
	}
	w := c.Office.Assets.World.Worlds[wid]
	// Source-initialized AD7/AD9 selectors: map0 and target-camera5. No
	// invented contact/target image is supplied for the latter.
	p.Cockpit.PrimaryMap = initialized.InitialViewWords[2] == 0
	p.Cockpit.SecondaryMap = initialized.InitialViewWords[3] == 0
	p.Cockpit.Map = &cockpit.MapInputs{Grid: w.Grid, Atlas: w.TileAtlas, H0: initialized.Player.H0, H1: initialized.Player.H1, DrawOwnship: true}
	if err = p.initializeFlight(c.Office.Assets, m); err != nil {
		return err
	}
	runtimeOptions := c.Runtime.Effective()
	if c.r83Profile() {
		if !runtimeOptions.AvoidancePolicy.Valid() {
			return fmt.Errorf("invalid launch avoidance policy %q", runtimeOptions.AvoidancePolicy)
		}
		p.Flight.State.CorrectedAvoidance = runtimeOptions.AvoidancePolicy == configuration.AvoidanceCorrected
	}
	p.Scene.RenderScale = runtimeOptions.SimulationRenderScale
	p.Scene.FloatingGeometry = runtimeOptions.FloatingGeometry
	p.Scene.Resolution = runtimeOptions.SimulationResolution
	p.Frame = make([]byte, p.Scene.TargetPixels())
	p.worldFrame = make([]byte, p.Scene.TargetPixels())
	p.Flight.commandInput.Delay = time.Duration(runtimeOptions.CollectiveRepeatDelayMS) * time.Millisecond
	p.Flight.commandInput.Interval = time.Duration(runtimeOptions.CollectiveRepeatIntervalMS) * time.Millisecond
	p.Flight.commandInput.Capacity = runtimeOptions.CommandQueueCapacity
	p.presentation.mode = runtimeOptions.PresentationMode
	p.resetPresentation()
	p.PaletteState, err = p.Renderer.NewPaletteState(p.Scene, c.Office.Assets.Cockpit.NightPalette)
	if err != nil {
		return err
	}
	if err = p.advanceAvionicsReference(c.Office.Assets.Cockpit); err != nil {
		return err
	}
	if err = p.draw(c.Office.Assets.Cockpit); err != nil {
		return err
	}
	if c.filmIsolation == nil && c.Mission.SettingsChanged {
		next := c.Store.State()
		pilot := next.SelectedPilot()
		if pilot == nil || pilot.ID != final.PilotID {
			return fmt.Errorf("selected pilot changed before preview acceptance")
		}
		if err := gamestate.CopyPilotPreflight(pilot, final.Settings); err != nil {
			return err
		}
		if err := c.Store.Commit(next); err != nil {
			return c.failPreparationSave("settings acceptance", err)
		}
		c.Mission.SettingsChanged = false
	}
	c.Mission.Initialized = initialized
	c.Rendering = p
	if err := c.initializeSortieCapability(final); err != nil {
		return c.failPreparationSave("sortie acceptance", err)
	}
	if c.Mission.ResultUnavailable != "" {
		diagnostics.Warn("career", "Flight admitted without result capability", "reason", c.Mission.ResultUnavailable)
	}
	p.publishObjectiveDisplays()
	c.Screen = ScreenScene
	c.Message = ""
	return nil
}

func (p *RenderingPreview) draw(a *cockpit.Assets) (err error) {
	if !p.Dirty {
		return p.Error
	}
	defer func() { p.Error = err; p.Dirty = false; p.collectMaterialMask = false }()
	p.collectMaterialMask = true
	clear(p.targetBrownMask)
	if err = p.commitReferenceFrame(a); err != nil {
		return err
	}
	scene, in := p.presentationInputs()
	parkingFocus := ""
	if p.Cockpit.CameraMode == 4 && p.ViewControls != nil {
		parkingFocus = p.ViewControls.TrackedID
	}
	scene, parkedModels := p.parkedPresentation(scene, parkingFocus)
	if !p.Inspection && (p.Cockpit.CameraMode == 3 || p.Cockpit.CameraMode == 4) {
		focusID := p.Initialized.PlayerID
		if p.Cockpit.CameraMode == 4 && p.ViewControls != nil {
			focusID = p.ViewControls.TrackedID
		}
		for _, focus := range scene.Instances {
			if focus.ID == focusID {
				scene.Camera = p.Renderer.ObserverCamera(scene, focus)
				break
			}
		}
	}
	if len(p.worldSkyMask) == len(p.Frame) {
		if len(p.skyMask) != len(p.Frame) {
			p.skyMask = make([]byte, len(p.Frame))
		}
		copy(p.skyMask, p.worldSkyMask)
	} else {
		p.skyMask = nil
	}
	if scene.Environment != nil {
		in.HeadingPerturbation = scene.Environment.CompassShown
	}
	visual, nativeRotors := p.presentationVisual(scene, float64(p.visualTime)/float64(time.Second), p.presentation.mode == "smooth")
	if (p.presentation.mode == "smooth" || nativeRotors || parkedModels || len(scene.Instances) != len(p.Scene.Instances)) && !p.Inspection {
		copy(p.Frame, p.presentation.referenceBase)
		if len(p.skyMask) > 0 {
			scene.SkyMask = p.skyMask
		}
		p.Stats, err = p.Renderer.Draw(scene, &visual, p.Frame)
		if err != nil {
			return err
		}
	} else {
		copy(p.Frame, p.worldFrame)
	}
	if err = p.composeSceneOverlaysAt(p.Frame, in, a, scene); err != nil {
		return err
	}
	if err = p.drawPlayerShake(); err != nil {
		return err
	}
	if p.PaletteState != nil {
		p.Palette = p.PaletteState.Colours()
	} else {
		p.Palette, err = p.Renderer.Palette(scene, &p.Visual)
	}
	if r := p.Flight; r != nil && r.TerminalPresentation.Active && len(r.terminalRGB6) == 768 {
		p.Palette = (world.Palette{RGB6: r.terminalRGB6}).Colours()
	}
	return err
}

// presentationVisual is shared by live and external replay drawing. Its phase
// and fraction are private presentation values; neither a viewer camera nor
// its redraws may advance the canonical animation arena or flight state.
func (p *RenderingPreview) presentationVisual(scene render3d.Scene, alpha float64, interpolate bool) (render3d.VisualState, bool) {
	visual := p.Visual.Clone()
	phase := float64(p.Visual.Phase)
	if interpolate && !p.Inspection {
		visual = p.Renderer.SmoothVisual(&p.previousVisual, &p.Visual, alpha)
		phase += alpha
	}
	if !p.Inspection {
		return p.Renderer.NativeRingRotors(scene, &visual, phase, p.nativeRotorRunning())
	}
	return visual, false
}

// Rotor display ownership follows the actual admitted flight rows. The player
// has an explicit engine switch; live followers use the source callback while
// registered. Recovery/death visibility remains owned by filterSceneInstances.
func (p *RenderingPreview) nativeRotorRunning() map[string]bool {
	if p.Flight == nil || p.Flight.State == nil || p.Initialized == nil {
		return nil
	}
	running := make(map[string]bool, 5)
	for _, row := range p.Initialized.FlightRows {
		if row < 0 || row >= len(p.Initialized.Entities) {
			continue
		}
		e := p.Initialized.Entities[row]
		on := e.Participation == 1
		if e.ID == p.Initialized.PlayerID {
			on = p.Flight.State.Rotor != 0 || p.threatFeedbackMechanics && p.Flight.State.RadarAltitude > 2
		}
		running[e.ID] = on
	}
	return running
}

func (p *RenderingPreview) composeSceneOverlays(dst []byte, in cockpit.Inputs, a *cockpit.Assets) error {
	return p.composeSceneOverlaysAt(dst, in, a, p.Scene)
}
func (p *RenderingPreview) composeSceneOverlaysAt(dst []byte, in cockpit.Inputs, a *cockpit.Assets, display render3d.Scene) error {
	var owners []byte
	if p.collectMaterialMask {
		owners = p.skyMask
	}
	scale := p.Scene.Scale()
	if err := p.overlayScratch.Draw(dst, owners, scale, func(logical []byte) error { return cockpit.Compose(logical, a, in) }); err != nil {
		return err
	}
	if err := p.drawTargetView(dst, in, a, display, true); err != nil {
		return err
	}
	if in.Notifications.Visible() || in.CameraMode == 0 && in.DamageOverlay != 0 || in.HUD && in.CameraMode == 7 && !in.MastEmpty && in.Damage[0]&0x20 == 0 {
		if err := p.overlayScratch.Draw(dst, owners, scale, func(logical []byte) error { return p.composeAfterTargetOverlays(logical, in, a) }); err != nil {
			return err
		}
	}
	if r := p.Flight; r != nil && r.TerminalPresentation.Active {
		return r.TerminalPresentation.drawCovered(dst, owners, scale)
	}
	return nil
}
func (p *RenderingPreview) composeAfterTargetOverlays(dst []byte, in cockpit.Inputs, a *cockpit.Assets) error {
	// Source222a and223f converge at2242: the zero-masked sight bezel follows
	// acquired imagery as well as the empty fill. It is not a rectangular clear.
	if in.HUD && in.CameraMode == 7 && !in.MastEmpty && in.Damage[0]&0x20 == 0 {
		if err := a.DrawMastFrameScaled(dst, 1); err != nil {
			return err
		}
	}
	if err := cockpit.DrawNotificationsScaled(dst, a, in.Notifications, 1); err != nil {
		return err
	}
	if err := cockpit.DrawDamageOverlayScaled(dst, a, in, 1); err != nil {
		return err
	}
	return nil
}

func (c *FrontEnd) renderingFrame() []byte {
	if c.Rendering == nil {
		return make([]byte, 64000)
	}
	if err := c.Rendering.draw(c.Office.Assets.Cockpit); err != nil {
		c.Rendering.Error = err
	}
	out := append([]byte(nil), c.Rendering.Frame...)
	if c.flightTextVisible() {
		p := c.Rendering
		if err := p.overlayScratch.Draw(out, p.skyMask, p.Scene.Scale(), func(logical []byte) error { c.composeFlightTextAtScale(logical, 1); return nil }); err != nil {
			p.Error = err
		}
	}
	return out
}
func (c *FrontEnd) flightTextVisible() bool {
	p := c.Rendering
	if p == nil {
		return false
	}
	if p.r11Mechanics && p.ViewControls != nil && p.ViewControls.Mode == 4 && p.Initialized != nil {
		return true
	}
	return p.Flight != nil && !p.Inspection && (p.Flight.Paused || p.Flight.Failure != "")
}
func (c *FrontEnd) composeFlightText(out []byte) {
	c.composeFlightTextAtScale(out, c.Rendering.Scene.Scale())
}
func (c *FrontEnd) composeFlightTextAtScale(out []byte, scale int) {
	c.drawFlightMemberCaptionAtScale(out, scale)
	if r := c.Rendering.Flight; r != nil && !c.Rendering.Inspection {
		message := ""
		if r.Paused {
			if !c.drawLockedTargetDossierAtScale(out, scale) {
				_ = c.Office.Assets.Cockpit.DrawPause(out, uint16(c.Rendering.pauseUISeconds*70), scale)
			}
		}
		if r.Failure != "" {
			message = "FLIGHT STOPPED - ALT+E TO RETURN"
		}
		if message != "" {
			_ = c.Office.Assets.Cockpit.TextScaled(out, message, 160, 90, 15, 0x802, cockpit.Full, scale)
		}
	}
}

func (c *FrontEnd) handleRenderingPreview(a Actions) error {
	if c.Rendering != nil && c.Rendering.Flight != nil && !c.Rendering.Inspection {
		return c.handleFlight(a)
	}
	if a.Selector2 {
		return c.RefreshOffice()
	}
	if a.Cancel {
		return c.returnRenderingBriefing()
	}
	return c.handleSceneView(a)
}

func (c *FrontEnd) returnRenderingBriefing() error {
	c.AbandonFilmRecording()
	if c.Mission == nil {
		return fmt.Errorf("rendering return has no prepared mission")
	}
	if c.Mission.ResultLaunch != nil {
		if err := c.Store.AbandonSortie(c.Mission.ResultLaunch.Start.ID, "preview returned without an admitted final result"); err != nil {
			return c.failPreparationSave("unscored preview return", err)
		}
		c.Mission.ResultLaunch = nil
	}
	c.Screen = ScreenBriefing
	c.fade = nil
	c.SceneEntry = nil
	c.sceneEndConfirmation = false
	c.Mission.Cockpit.APU = 0
	c.Mission.Cockpit.Ready = false
	c.Mission.Cockpit.readyTicks = 0
	c.Mission.View.ResumeScene()
	c.reportStatus("scene", "Sortie ended; briefing retained without persistent outcomes.")
	return nil
}

// AdvanceRendering merges the visual12Hz and flight clocks chronologically.
// Host draw partitions cannot reorder retained reference-buffer operations.
func (c *FrontEnd) AdvanceRendering(seconds float64) error {
	if c.Screen != ScreenScene || c.Rendering == nil || c.fade != nil {
		return nil
	}
	p := c.Rendering
	if seconds <= 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return nil
	}
	if p.GPS != nil || p.Paused || (p.Flight != nil && p.Flight.Paused) {
		if p.GPS == nil {
			p.pauseUISeconds += math.Min(seconds, .25)
		}
		p.visualTime = 0
		p.hostTimeFraction = 0
		if p.Flight != nil {
			p.Flight.Clock.ResetElapsed()
		}
		return nil
	}
	dt := math.Min(seconds, .25)*float64(time.Second) + p.hostTimeFraction
	remaining := int64(math.Round(dt))
	p.hostTimeFraction = dt - float64(remaining)
	rate := int64(p.Rate)
	if float64(rate) != p.Rate || rate <= 0 { // diagnostic rates retain their existing accumulator.
		if !p.Inspection && p.Flight != nil {
			if err := c.advanceFlight(seconds); err != nil {
				return err
			}
		}
		if p.clock.Rate != p.Rate {
			p.clock = tickAccumulator{Rate: p.Rate}
		}
		if err := c.StepRendering(p.clock.Advance(seconds, false)); err != nil {
			return err
		}
	} else {
		for remaining > 0 && c.Screen == ScreenScene {
			part := min(remaining, (int64(time.Second)-p.visualTime+rate-1)/rate)
			if !p.Inspection && p.Flight != nil {
				if err := c.advanceFlight(float64(part) / float64(time.Second)); err != nil {
					return err
				}
			}
			remaining -= part
			p.visualTime += part * rate
			if c.Screen != ScreenScene {
				break
			}
			if p.visualTime >= int64(time.Second) {
				p.visualTime -= int64(time.Second)
				if err := c.StepRendering(1); err != nil {
					return err
				}
			}
		}
	}
	if p.Flight != nil && !p.Inspection && p.presentation.mode == "smooth" {
		if p.Flight.State.Terminal == nil && p.Flight.Failure == "" {
			p.presentation.alpha = p.Flight.Clock.InterpolationFraction()
		}
		p.Dirty = true
	}
	return nil
}

func (c *FrontEnd) StepRendering(steps int) error {
	if steps > 0 && c.Screen == ScreenScene && c.Rendering != nil {
		defer func() { c.recordFilmBoundary(filmBoundary{Kind: "reference", Count: steps}) }()
	}
	if c.Screen != ScreenScene || c.Rendering == nil || steps == 0 {
		return nil
	}
	p := c.Rendering
	if steps < 0 || steps > 100000 {
		return fmt.Errorf("visual steps outside diagnostic bound")
	}
	for i := 0; i < steps; i++ {
		if !p.Inspection && p.Flight == nil && p.ViewControls != nil {
			p.ViewControls.MastKeyboard(p.mastHeld[0], p.mastHeld[1], p.mastHeld[2], p.mastHeld[3])
			if err := p.ViewControls.Tick(p.cameraInputs(), p.Renderer.Assets); err != nil {
				return err
			}
			p.publishView()
			p.filterSceneInstances()
		}
		p.previousVisual = p.Visual.Clone()
		if err := p.Renderer.Advance(p.Scene, &p.Visual, 1); err != nil {
			return err
		}
		if p.Inspection || p.Flight == nil {
			p.Scene.Environment.Step(p.Scene.GraphicsOptions)
		}
	}
	p.presentation.referenceDirty = true
	if p.PaletteState != nil && (p.Inspection || p.Flight == nil) {
		p.PaletteState.Advance(p.Scene.GraphicsOptions, p.Visual.Phase, p.Scene.Environment)
	}
	p.Dirty = true
	if err := p.advanceAvionicsReference(c.Office.Assets.Cockpit); err != nil {
		return err
	}
	return p.commitReferenceFrame(c.Office.Assets.Cockpit)
}

// Inspection navigation changes only the camera, never aircraft or mission data.
func (c *FrontEnd) MoveInspectionCamera(seconds, forward, side, vertical, yaw, pitch float64) {
	if c.Screen != ScreenScene || c.Rendering == nil {
		return
	}
	p := c.Rendering
	cam := p.Scene.Camera
	seconds = math.Min(math.Max(seconds, 0), .1)
	angle := float64(cam.Angles[1]) * 2 * math.Pi / 65536
	d := seconds * 4096
	x := int64(cam.H0) + int64((-math.Sin(angle)*forward+math.Cos(angle)*side)*d)
	z := int64(cam.H1) + int64((math.Cos(angle)*forward+math.Sin(angle)*side)*d)
	h := int(cam.Height) + int(vertical*d)
	if x < 0 || z < 0 || x >= 64*8192 || z >= 64*8192 || h < 1 || h > 32767 {
		return
	}
	cam.H0, cam.H1, cam.Height = uint32(x), uint32(z), uint16(h)
	cam.Angles[1] += uint16(int(yaw * seconds * 8192))
	cam.Angles[0] += uint16(int(pitch * seconds * 8192))
	if cam != p.Scene.Camera {
		p.Scene.Camera = cam
		p.Dirty = true
	}
}

func (p *RenderingPreview) Report() map[string]any {
	// Legacy diagnostic consumers read this integer only for the two integral modes.
	var legacyScale any
	layout := p.Scene.Layout()
	if !layout.Mapped() {
		legacyScale = p.Scene.Scale()
	}
	return map[string]any{"gps": p.GPS, "simulation_render_scale": legacyScale, "simulation_resolution": fmt.Sprintf("%dx%d", layout.Width, layout.Height), "presentation_mode": p.presentation.mode, "presentation_alpha": p.presentation.alpha, "visual_interpolation_alpha": float64(p.visualTime) / float64(time.Second), "component_smoothing_policy": render3d.Callback3PresentationPolicy, "world": p.Scene.World, "mission_world_source": p.SourceMissionWorld, "mission_objects_sha256": p.MissionObjectHash, "camera": p.Scene.Camera, "camera_mode": p.Cockpit.CameraMode, "hud": p.Cockpit.HUD, "instances": p.Scene.Instances, "palette_bank": p.Scene.PaletteBank, "graphics_options": p.Scene.GraphicsOptions, "viewport": p.Scene.View, "visual_phase": p.Visual.Phase, "preview_rate_policy_hz": p.Rate, "paused": p.Paused, "arrangement": p.Cockpit.Arrangement, "aircraft_id": p.Cockpit.AircraftID, "diagnostic_policy": p.DiagnosticPolicy, "initialized_mission": p.Initialized, "view_controller": p.ViewControls, "inspection_camera": p.Inspection, "environment": p.Scene.Environment, "source_rows_excluded": p.SkippedRows, "stats": p.Stats, "error": fmt.Sprint(p.Error), "flight": p.Flight, "flight_clock_policy": flight.NativeClockPolicy, "scope": "Source player flight/weapons, stationary damage and supplied accepted-player-hit consequences; autonomous incoming attacks and full loss normalization inactive. Career-result and recovery admission belong to the front-end sortie controller."}
}

// Only the explicit development entry supplies these overrides. Ordinary final
// preflight always keeps its selected mission/world and accepted configuration.
type RenderingInspection struct {
	World                   string
	Camera                  *render3d.Camera
	CockpitAircraft         *int
	Arrangement             int
	VisualTicks             int
	Rate                    float64
	Paused, Exterior, NoHUD bool
}

func (c *FrontEnd) ConfigureRenderingInspection(options RenderingInspection) error {
	if c.Screen != ScreenScene || c.Rendering == nil {
		return fmt.Errorf("no rendering preview to configure")
	}
	if options.Arrangement < 0 || options.Arrangement > 3 || options.VisualTicks < 0 || options.VisualTicks > 10000 || math.IsNaN(options.Rate) || math.IsInf(options.Rate, 0) || options.Rate <= 0 || options.Rate > 120 {
		return fmt.Errorf("viewer needs arrangement0..3, visual ticks0..10000 and preview rate in(0,120]")
	}
	p := c.Rendering
	p.Inspection = true
	p.Scene.Instances = append([]render3d.Instance(nil), p.instances...)
	if options.World != "" {
		w := p.Renderer.Assets.Worlds[options.World]
		if w == nil {
			return fmt.Errorf("unprepared development world %q", options.World)
		}
		p.Scene.World = options.World
		p.Scene.Instances = nil
		p.Scene.Environment = nil
		p.Scene.PaletteBank = "DAY"
		p.Scene.Camera = render3d.Camera{Pose: render3d.Pose{H0: 32*8192 + 4096, H1: 32*8192 + 4096, Height: 2048}, Angles: [3]uint16{0, 0xe000, 0}}
		p.Cockpit.Map = &cockpit.MapInputs{Grid: w.Grid, Atlas: w.TileAtlas, H0: p.Scene.Camera.H0, H1: p.Scene.Camera.H1, DrawOwnship: true}
		p.DiagnosticPolicy += " Standalone development world override: mission instances excluded; DAY palette and central diagnostic camera, not the selected mission."
	}
	if options.Camera != nil {
		if options.Camera.H0 >= 64*8192 || options.Camera.H1 >= 64*8192 || options.Camera.Height == 0 || options.Camera.Height > 32767 {
			return fmt.Errorf("inspection camera requires interior native coordinates0..524287 and height1..32767; this diagnostic policy does not repair unknown edge carriers")
		}
		p.Scene.Camera = *options.Camera
	}
	if options.CockpitAircraft != nil {
		if _, err := cockpit.Family(*options.CockpitAircraft); err != nil {
			return err
		}
		p.Cockpit.AircraftID = *options.CockpitAircraft
		p.DiagnosticPolicy += " Development cockpit-artwork override; accepted aircraft/equipment is unchanged."
	}
	p.Cockpit.Arrangement = options.Arrangement
	p.Cockpit.HUD = !options.NoHUD
	if options.Exterior {
		p.Cockpit.CameraMode = 1
	}
	p.Paused, p.Rate = options.Paused, options.Rate
	if err := c.StepRendering(options.VisualTicks); err != nil {
		return err
	}
	paletteState, err := p.Renderer.NewPaletteState(p.Scene, c.Office.Assets.Cockpit.NightPalette)
	p.PaletteState = paletteState
	if err != nil {
		return err
	}
	p.resetPresentation()
	p.Dirty = true
	return p.draw(c.Office.Assets.Cockpit)
}

// Select before initialization, not after a current-profile constructor has
// populated private owners. These bits never enter a film or user preference.
func launchSimulationPolicy(profile string) (formation, modern bool) {
	switch profile {
	case "", replay.RouteAcquisitionCompatibility, replay.RouteAcquisitionAuthoredCompatibility, replay.ManeuverCompatibility, replay.ManeuverAuthoredCompatibility, replay.W31Compatibility, replay.W31AuthoredCompatibility, replay.W3Compatibility, replay.W3AuthoredCompatibility, replay.V45Compatibility, replay.V45AuthoredCompatibility, replay.U26Compatibility, replay.U26AuthoredCompatibility, replay.U25Compatibility, replay.U25AuthoredCompatibility, replay.U24Compatibility, replay.U24AuthoredCompatibility, replay.T5Compatibility, replay.T5AuthoredCompatibility, replay.R86Compatibility, replay.R86AuthoredCompatibility, replay.R83Compatibility, replay.R83AuthoredCompatibility, replay.R8Compatibility, replay.R8AuthoredCompatibility, replay.R6Compatibility, replay.R6AuthoredCompatibility, replay.R5Compatibility, replay.R5AuthoredCompatibility, replay.R4Compatibility, replay.R4AuthoredCompatibility, replay.R3Compatibility, replay.R3AuthoredCompatibility, replay.R231Compatibility, replay.R231AuthoredCompatibility, replay.R23Compatibility, replay.R23AuthoredCompatibility, replay.R221Compatibility, replay.R221AuthoredCompatibility, replay.R22Compatibility, replay.R22AuthoredCompatibility, replay.R21Compatibility, replay.R21AuthoredCompatibility, replay.R12Compatibility, replay.R12AuthoredCompatibility, replay.R11WaypointReleaseCompatibility, replay.R11WaypointReleaseAuthoredCompatibility, replay.R11AvoidanceCompatibility, replay.R11AvoidanceAuthoredCompatibility, replay.R11CameraCompatibility, replay.R11CameraAuthoredCompatibility, replay.R11Compatibility, replay.R11AuthoredCompatibility, replay.R1Compatibility, replay.R1AuthoredCompatibility, replay.P7Compatibility, replay.P7AuthoredCompatibility, replay.P6Compatibility, replay.P6AuthoredCompatibility, replay.P5Compatibility, replay.P5AuthoredCompatibility, replay.P4Compatibility, replay.P4AuthoredCompatibility:
		return true, true
	case replay.CurrentCompatibility, replay.CurrentAuthoredCompatibility:
		return true, false
	default:
		return false, false
	}
}
