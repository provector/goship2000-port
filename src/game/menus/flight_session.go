package menus

import (
	"encoding/binary"
	"fmt"
	"gunship2000-assets/game/diagnostics"
	"gunship2000-assets/game/replay"
	"math"
	"time"

	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/avionics"
	"gunship2000-assets/game/cockpit"
	"gunship2000-assets/game/combat"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/flightcontrol"
	"gunship2000-assets/game/navigation"
	"gunship2000-assets/game/render3d"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/game/world"
)

// FlightRuntime owns inputs/systems around InitializedMission.PlayerFlight.
// It has no pilot-save or mission-generator access and never updates wingman AI.
type FlightRuntime struct {
	OrderRecipient                          int `json:",omitempty"`
	filmTiming                              *flight.Timing
	Combat                                  *combat.State
	Notifications                           combat.NotificationState
	State                                   *flight.State
	Navigation                              [5]navigation.Route
	Avionics                                *avionics.State
	AcquisitionSpeech                       avionics.AcquisitionSpeechState
	SpeechEnabled                           bool // Source DE80; independent of global output effects mute.
	DamageOverlay                           [2][2]int16
	DamageHealth                            uint16 // First-ten low2 source export, not a flight-unit percentage.
	DamageEvents                            []PlayerHitWitness
	TerminalPresentation                    terminalPresentation
	terminalRGB6                            []byte
	acceptedPlayerHits                      []AcceptedPlayerHit
	Defenses                                flight.Defenses
	RocketSalvo                             int16 // sourceAC1, transient1/2/4; never consumes stores without firing
	gpsNeedsRelease                         bool
	gpsHeld                                 [4]bool
	gpsAxes                                 flightcontrol.Axes
	Parameters                              flight.Parameters
	GearGraph                               assetdata.GearGraphBinding
	Systems                                 flight.Systems
	Axes                                    flightcontrol.Axes
	collectiveOwner                         flightcontrol.KeyboardPickup
	liveControls                            flightcontrol.LiveState
	Clock                                   flight.Clock
	Compression                             flight.Compression
	RequireTerminalAck, terminalAwaitingAck bool
	LowLimit, PrimaryMFD, SecondaryMFD      int16
	SystemFlags                             byte
	MassTerm                                int16
	WindHeading                             uint16
	WindStrength                            int16
	Paused                                  bool
	Steps                                   uint64
	ElapsedDA                               uint16 // Last reached logical DA, never queued-but-unprocessed clock ticks.
	Failure                                 string
	commandInput                            flightCommandTimeline
	held                                    flightcontrol.Held
	requireRelease                          bool
	triggerNeedsRelease                     bool                    // Native handoff: a held APU/menu button is not a fresh fire press.
	rotorCommands                           []uint16                // Ordered0bh events; sound sample clock owns playback state.
	sceneAudio                              []sceneAudioCommand     // One order across rotor and fixed GS2 driver calls.
	CombatEvents                            []combat.Event          // Bounded diagnostic witness, not a persistent result log.
	CopilotDefense                          []CopilotDefenseWitness // Bounded receipts; actual ownership is checkpointed in Defenses.
	TriggerTrace                            []WeaponTriggerWitness
	triggerReceiptSeen, triggerReceiptHeld  bool
	triggerReceiptOwner                     string
	projectileGeneration                    [combat.SlotCount]uint64
	projectileAngles                        [combat.SlotCount][3]uint16
	deathAngles                             map[int][3]uint16
	explosionVertices                       []world.Vertex
	deathHistoryCount                       uint16
	deathHistory                            [8]render3d.Pose
	deathHistoryWorld                       []render3d.Instance
}

// FlightKeys is the source flight/system command domain. Navigation, defenses,
// held firing and weapon selection have separate semantic input ownership.
var FlightKeys = [...]string{"=", "-", "9", "0", "c", "v", "z", "x", "3", "8", "5"}

func (p *RenderingPreview) initializeFlight(a *assetdata.OfficeAssets, m *gamestate.MissionData) error {
	if p.Initialized.PlayerRow != 0 {
		return fmt.Errorf("flight requires the supported primary launch row0")
	}
	id := p.Cockpit.AircraftID
	family, err := cockpit.Family(id)
	if err != nil {
		return err
	}
	def := a.Cockpit.Definitions[family]
	if len(def.StateParameters) != 80 {
		return fmt.Errorf("%s flight STAT parameters missing", family)
	}
	r := &FlightRuntime{SpeechEnabled: true, DamageHealth: 255, triggerNeedsRelease: true, RocketSalvo: 1, LowLimit: 128, PrimaryMFD: 0, SecondaryMFD: 5, WindHeading: m.Conditions.WindDirection, WindStrength: int16(m.Conditions.WindSpeed) / 2}
	r.GearGraph = a.Missions.Generation.Tables.Initialization.GearGraph
	copy(r.Parameters.Raw[:], def.StateParameters)
	init := p.Initialized
	s := &flight.State{Pose: flight.Pose{H0: init.Player.H0, H1: init.Player.H1, Height: int16(init.Player.Height), Angles: init.Player.Angles}, Forward: init.InitialForwardMotion, LowLimit: 128, FlightStage: 128}
	if init.Entities[init.PlayerRow].SerializedAircraftSelector == 4 {
		r.SystemFlags = 1
	}
	if init.Switches&0x800 != 0 {
		s.Collective, s.Rotor, s.HeightControl = 16, 1, 1
		s.HeightTarget, s.CruiseSpeed = 300, 120
		if r.SystemFlags&1 != 0 {
			r.SystemFlags |= 4
			putSTATWord(&r.Parameters, 0x46, statWord(r.Parameters, 0x46)-2)
		}
	}
	row := init.Aircraft[0]
	var stations [4]flight.LoadStation
	for i := range stations {
		system := int(int8(row[2+i]))
		quantity := gamestate.RowWord(row[:], 21)
		if i > 0 {
			quantity = gamestate.RowWord(row[:], 10+2*i)
		}
		w, ok := a.Missions.WeaponDefinitions[system]
		if !ok {
			continue
		}
		if len(w) != 214 {
			return fmt.Errorf("flight station%d WSHD width invalid", i)
		}
		stations[i] = flight.LoadStation{Present: true, Flags: binary.LittleEndian.Uint16(w[4:]), Quantity: quantity, UnitTerm: gamestate.RowWord(w, 13), FixedTerm: gamestate.RowWord(w, 15)}
		p.Cockpit.Stations[i].Flags = byte(stations[i].Flags)
	}
	cargo := byte(0)
	if row[1] == 2 && row[35] > 0 {
		cargo = 24
		if row[35] == 1 && m.RawE278 == 2 {
			cargo = 8
		} else if row[35] == 2 && m.RawE278 == 1 {
			cargo = 16
		}
	}
	r.MassTerm = flight.LaunchLoad(r.Parameters, stations, byte(init.Entities[0].SerializedAircraftSelector), cargo)
	active := make([]bool, len(init.Aircraft))
	for _, e := range init.Entities {
		if e.FlightSlot >= 0 && e.FlightSlot < len(active) {
			active[e.FlightSlot] = e.ControlRaw[1] != 0
		}
	}
	r.Systems = flight.NewSystems(init.InitialFuelPercentBySlot, active)
	r.State = s
	init.PlayerFlight = s
	p.Flight = r
	if init.Stores != nil && len(init.Stores.Ships) > 0 {
		ship := init.Stores.Ships[0]
		r.Defenses = flight.NewDefenses(ship.Chaff, ship.Flares)
	}
	if p.copilotMechanics {
		r.Defenses.EnableR3()
	}
	p.initializeNavigation()
	p.initializeNavigationObjectives(m)
	p.initializeAvionics()
	r.Combat, err = combat.New(init, p.Renderer.Assets, m)
	if err != nil {
		return err
	}
	r.Combat.SetFormationEnabled(p.formationMechanics)
	r.Combat.SetLogisticsEnabled(p.p4Mechanics)
	r.Combat.SetMissionActorsEnabled(p.p5Mechanics)
	r.Combat.SetTrainEnabled(p.p6Mechanics)
	r.Combat.SetFeedbackEnabled(p.p7Mechanics)
	r.Combat.SetOrdersEnabled(p.r1Mechanics)
	r.Combat.SetCorridorNavigation(p.corridorNavigation)
	if p.admitExitWaypoint {
		r.Combat.EnableExitWaypoint()
	}
	r.Combat.SetObstacleClearance(p.obstacleClearance)
	if p.resilientFlight {
		r.Combat.EnableResilientRoutes()
	}
	if p.lowCityClearance {
		r.Combat.SetLowCityClearance(m)
	}
	r.Combat.SetSafeFlightOrdersEnabled(p.r5Mechanics)
	if p.r5Mechanics {
		r.Combat.InitializeFreshSeeker()
	}
	r.Combat.SetDispatcherScratchEnabled(p.r11Mechanics)
	r.Combat.SetRouteAvoidanceEnabled(p.routeAvoidanceMechanics)
	r.Combat.SetWaypointReleaseEnabled(p.waypointReleaseMechanics)
	r.Combat.SetNavigationEnabled(p.navigationMechanics)
	r.Combat.SetWingmenAutonomyEnabled(p.wingmenAutonomy)
	r.Combat.SetEnemyBehaviorEnabled(p.enemyBehaviorMechanics)
	r.Combat.SetPersistentPlayerThreat(p.copilotMechanics)
	if p.wingmanDamageMechanics {
		if err = init.Stores.EnableNPCDamage(r.Parameters.Raw[:]); err != nil {
			return fmt.Errorf("wingman damage launch: %w", err)
		}
		r.Combat.SetWingmanDamageEnabled(true)
	}
	if p.stationaryEnemyMechanics {
		if init.EnemyRules == nil {
			return fmt.Errorf("stationary enemy launch definitions missing")
		}
		if err = r.Combat.ConfigureStationaryEnemyAI(init.EnemyRules.Definitions, init.EnemyRules.Disabled); err != nil {
			return err
		}
	}
	p.DiagnosticPolicy = "Source player/weapon updates with AdLib baseline six-clock native timing; autonomous decisions/motion remain unavailable. Stationary hit/destruction mechanisms are active; career-result admission belongs to the front-end sortie controller."
	if p.stationaryEnemyMechanics {
		p.DiagnosticPolicy = "Source stationary C056 hostile decisions use frozen world ENEM and Enemy AI policy; player projectile/damage/results use their existing owners. Mobile/airborne hostile callers and accepted nonplayer damage remain guarded. Wingmen autonomy is an independent frozen policy."
	}
	if p.wingmanDamageMechanics {
		p.DiagnosticPolicy = "Source stationary hostile decisions and victim-owned wingman defense/damage/results use frozen R22 rules. Mobile/airborne hostile callers, active fixed/FARP incoming victims and unknown source leaves retain separate guards. Player copilot automation remains outside this interpretation."
	}
	if p.enemyBehaviorMechanics {
		p.DiagnosticPolicy = "Source six-caller hostile decisions, cold aircraft activation, private submarine models and fixed/FARP victims use frozen R23 rules. Enemy and wingman autonomy policies are independent; ordinary movement and already-live projectiles continue with Enemy AI disabled. Player copilot automation and Step51 guards remain separate."
	}
	if p.copilotMechanics {
		p.DiagnosticPolicy = "R3 frozen player copilot weapon and late defensive decisions; physical threats, manual/automatic jammer ownership and persistent charge/token state are recorded independently of current SETUP AI preferences. Step51 guards remain separate."
	}
	p.publishFlight()
	return nil
}

func statWord(p flight.Parameters, at int) int16 {
	return int16(binary.LittleEndian.Uint16(p.Raw[at:]))
}
func putSTATWord(p *flight.Parameters, at int, n int16) {
	binary.LittleEndian.PutUint16(p.Raw[at:], uint16(n))
}

func (p *RenderingPreview) clearFlightInput() { p.resetFlightInput(false) }

func (p *RenderingPreview) resetFlightInput(pausedFocus bool) {
	if p.Flight == nil {
		return
	}
	r := p.Flight
	if p.filmPlayback == nil || p.filmPlayback.Practice || replay.HeldInput(p.filmPlayback.Film.Header.InputSchema) {
		if p.copilotMechanics {
			if err := r.Defenses.SetR3ManualHeld([2]bool{}, r.Systems.DamageMasks[0], nil); err != nil {
				r.Failure = err.Error()
			}
		} else {
			r.Defenses.SetManualHeld([2]bool{}, r.Systems.DamageMasks[0], nil)
		}
		if p.copilotMechanics {
			p.publishDefenseState()
		} else if p.Cockpit.Defense != nil {
			p.Cockpit.Defense.IRActive = 0
			p.Cockpit.Defense.RadarSignature = 0
		}
	}
	r.Axes.Clear()
	r.collectiveOwner.Sampled = false
	r.liveControls = flightcontrol.LiveState{}
	r.gpsAxes.Clear()
	r.gpsHeld = [4]bool{}
	r.held = flightcontrol.Held{}
	r.commandInput.ResetTimeline()
	r.Clock.ResetElapsed()
	r.Compression.Reset()
	r.requireRelease = true
	p.mastHeld = [4]bool{}
	p.Cockpit.CompressionRequest = 0
	p.recordFilmReset(pausedFocus)
	p.visualTime = 0
	p.hostTimeFraction = 0
	referenceDirty := p.presentation.referenceDirty
	p.resetPresentation()
	p.presentation.referenceDirty = referenceDirty
}

func (c *FrontEnd) handleFlight(a Actions) error {
	p := c.Rendering
	r := p.Flight
	r.traceWeaponReceipt(a, p.GPS != nil)
	if a.FlightFocusLost {
		if p.GPS != nil {
			r.gpsNeedsRelease = true
		}
		p.resetFlightInput(r.Paused)
		return nil
	}
	if p.filmRecorder != nil {
		p.filmRecorder.focusResetValid = false
	}
	if r.State.Terminal != nil {
		return nil
	} // source terminal presentation does not acquire flight commands.
	if p.GPS != nil {
		if r.gpsNeedsRelease {
			if !a.AnyKey && a.GPSKey == "" && !a.Selector1 && !a.Selector2 && !a.Cancel && !a.Backspace && a.SceneDirections == [4]bool{} && a.ViewKey == 0 {
				r.gpsNeedsRelease = false
				c.recordFilmBoundary(filmBoundary{Kind: "gps-release"})
			}
			return nil
		}
		return c.handleGPS(a)
	}
	if r.Paused {
		if a.AnyKey || a.Cancel || a.Selector1 || a.Selector2 || a.FlightPause || len(a.FlightCommands) > 0 || a.ViewKey != 0 {
			r.Paused = false
			p.Paused = false
			p.clearFlightInput()
			p.Dirty = true
		}
		return nil
	}
	if a.FlightPause {
		r.Paused = true
		p.Paused = true
		p.clearFlightInput()
		p.Dirty = true
		return nil
	}

	if a.GPSOpen {
		if r.requireRelease {
			return nil
		}
		return c.openGPS()
	}
	if r.requireRelease {
		liveReleased := true
		if a.Live != nil {
			for _, d := range a.Live.Digital {
				liveReleased = liveReleased && !d[0] && !d[1]
			}
		}
		if liveReleased && a.SceneDirections == [4]bool{} && len(a.FlightCommands) == 0 && (!a.FlightHeldValid || a.FlightHeldKeys == [11]bool{}) && (!a.DefenseHeldValid || a.DefenseHeld == [2]bool{}) && (!a.WeaponTriggerValid || !a.WeaponTriggerHeld) {
			r.requireRelease = false
			r.triggerNeedsRelease = false
		}
		return nil
	}
	if a.CompressionIncrease {
		prior := r.Compression.Request
		r.Compression.Increase()
		p.traceCompressionTransition("bracket-increase", prior, r.Compression.Request)
		r.commandInput.LogicalSampling = true
		p.Dirty = true
	}
	if a.CompressionReset {
		prior := r.Compression.Request
		r.Compression.Reset()
		p.traceCompressionTransition("bracket-reset", prior, 0)
		r.Clock.ResetElapsed()
		p.Dirty = true
	}
	p.Cockpit.CompressionRequest = r.Compression.Request
	beforeDropped := r.commandInput.Dropped
	if a.WeaponTriggerValid && !a.WeaponTriggerHeld {
		r.triggerNeedsRelease = false
	}
	if r.triggerNeedsRelease {
		a.WeaponTriggerHeld = false
		a.WeaponTriggerPressed = false
	}
	if p.r1Mechanics {
		a.WingmanRecipient = r.OrderRecipient
	} else {
		a.WingmanOrder = ""
	}
	if !p.r4Mechanics {
		a.LastMessage, a.Jettison = false, false
	}
	r.commandInput.Sample(a)
	if a.SpeechToggle {
		//0000:05f0 toggles future DE80 calls, without stopping active/pending PCM.
		r.SpeechEnabled = !r.SpeechEnabled
	}
	if r.commandInput.Dropped != beforeDropped {
		c.reportDiagnostic("flight", "Flight command queue full; extra key edges ignored.", nil)
	}
	if a.NightVisionToggle && p.PaletteState != nil {
		p.PaletteState.Toggle(p.Initialized.Switches)
		p.Dirty = true
	}

	err := c.handleSceneView(a)
	control := Actions{CompressionIncrease: a.CompressionIncrease, CompressionReset: a.CompressionReset, SpeechToggle: a.SpeechToggle, NightVisionToggle: a.NightVisionToggle, ViewKey: a.ViewKey, ViewShift: a.ViewShift, ViewCycle: a.ViewCycle, CycleArrangement: a.CycleArrangement}
	if control.CompressionIncrease || control.CompressionReset || control.SpeechToggle || control.NightVisionToggle || control.ViewKey != 0 || control.ViewCycle != 0 || control.CycleArrangement {
		c.recordFilmBoundary(filmBoundary{Kind: "control", Action: control})
	}
	return err
}

func (p *RenderingPreview) applyFlightCommand(encoded int, a *cockpit.Assets) {
	r := p.Flight
	s := r.State
	key := encoded & 0xff
	shift := encoded&0x100 != 0
	if key < 0 || key >= len(FlightKeys) {
		return
	}
	if shift && key >= 2 {
		return
	} // Only collective has recovered Shift variants.
	if key == 8 || key == 9 {
		if r.SystemFlags&1 == 0 {
			return
		}
		delta := int16(0)
		if key == 8 {
			r.SystemFlags ^= 4
			delta = 2
			if r.SystemFlags&4 != 0 {
				delta = -2
			}
		} else {
			r.SystemFlags ^= 2
			delta = -1
			if r.SystemFlags&2 != 0 {
				delta = 1
			}
		}
		putSTATWord(&r.Parameters, 0x46, statWord(r.Parameters, 0x46)+delta)
		return
	}
	if key == 10 {
		if r.Systems.DamageMasks[0]&3 == 2 {
			return
		}
		s.HeightControl ^= 1
		if s.HeightControl != 0 {
			s.HeightTarget = s.Pose.Height
			s.CruiseSpeed = s.IndicatedSpeed
			if s.CruiseSpeed > 120 {
				s.CruiseSpeed = 120
			}
			if s.CruiseSpeed < 0 || s.HeightTarget < r.LowLimit {
				s.HeightControl = 0
			}
		}
		return
	}
	family, _ := cockpit.Family(p.Cockpit.AircraftID)
	d := a.Definitions[family]
	ctrl := flightcontrol.State{Collective: s.Collective, Rotor: s.Rotor, Hover: s.Hover, LowLimit: r.LowLimit, PrimaryMFD: r.PrimaryMFD, SecondaryMFD: r.SecondaryMFD, SpeedE2: s.Forward, AvionicsDamage: r.Systems.DamageMasks[0], SecondaryMFDInstalled: d.Parameters[0x52/2] == d.Parameters[0x5e/2], SpeedHold2D06: s.CruiseSpeed, FlightStage2D08: s.FlightStage}
	ctrl.Apply(flightcontrol.Resolve(FlightKeys[key], shift, false))
	s.Collective, s.Rotor, s.Hover = ctrl.Collective, ctrl.Rotor, ctrl.Hover
	s.CruiseSpeed, s.FlightStage = ctrl.SpeedHold2D06, ctrl.FlightStage2D08
	r.LowLimit, r.PrimaryMFD, r.SecondaryMFD = ctrl.LowLimit, ctrl.PrimaryMFD, ctrl.SecondaryMFD
	if r.SystemFlags&1 == 0 && r.SecondaryMFD < 0 {
		r.SecondaryMFD = 0
	}
	s.LowLimit = r.LowLimit
	p.collectiveRotorCommand(key)
}

func (c *FrontEnd) advanceFlight(seconds float64) (err error) {
	defer func() {
		if err != nil && c.Rendering != nil && c.Rendering.Flight != nil && c.Rendering.Flight.Failure == "" {
			c.Rendering.Flight.Failure = err.Error()
			if c.filmIsolation == nil && c.Rendering.resilientFlight {
				c.reportDiagnostic("flight", "Flight update stopped. End the sortie to review the reached result.", err)
				diagnostics.Report("flight-update", err.Error(), "Flight processing stopped.")
				c.Rendering.Flight.Clock.ResetElapsed()
				err = nil
			}
		}
	}()
	defer c.filmFinishStep()
	p := c.Rendering
	r := p.Flight
	if r == nil || p.Inspection {
		return nil
	}
	if r.Paused || p.Paused || r.Failure != "" {
		r.Clock.ResetElapsed()
		return nil
	}
	// A historical film can hand practice a clock already past the live limit.
	if c.filmIsolation != nil && c.filmIsolation.Mode == "practice" && r.Clock.ReplayState().Phase >= missionClockLimit {
		return c.confirmSortieEnd()
	}
	if r.State.Terminal != nil {
		r.Compression.Reset()
		if r.terminalAwaitingAck {
			r.Clock.ResetElapsed()
			return nil
		}
		if r.filmTiming == nil {
			r.Clock.Schedule(time.Duration(math.Round(seconds*float64(time.Second))), 1)
		}
		if t, ok := r.nextFilmOrClock(); ok {
			if _, _, _, err := c.filmBeginStep(t, true, 0, false, nil); err != nil {
				return err
			}
			r.ElapsedDA = t.Phase
			if err := c.advancePlayerTerminal(); err != nil {
				return err
			}
			r.Clock.ResetElapsed() // Deliver one terminal transaction; discard stall debt.
			if r.TerminalPresentation.Completed {
				return c.finishPlayerTerminal()
			}
			if c.liveMissionClockLimit(t.Phase) {
				return c.confirmSortieEnd()
			}
		}
		return nil
	}
	if r.filmTiming == nil {
		r.Clock.Schedule(time.Duration(math.Round(seconds*float64(time.Second))), r.Compression.Rate())
	}
	for t, ok := r.nextFilmOrClock(); ok; t, ok = r.nextFilmOrClock() {
		// Queue resolution is side-effect-free with respect to source flight/RNG.
		// Resolve once for filming before stage mutation; apply at its old phase.
		command, hasCommand := 0, false
		var aux []int
		if p.filmPlayback == nil {
			command, hasCommand = r.commandInput.NextLogical()
			aux = r.commandInput.TakeAux()
		}
		var err error
		command, hasCommand, aux, err = c.filmBeginStep(t, false, command, hasCommand, aux)
		if err != nil {
			return err
		}
		// Only a reached/accepted source clock owns elapsed result time. Old O
		// films retain their historical stale-zero interpretation, never rescore.
		if !p.filmLegacy {
			r.ElapsedDA = t.Phase
		}
		p.beginPresentationStep()
		r.Defenses.AdvanceCounters(t.E0)
		flight.AdvanceStage(r.State, t.E0) // source02ba precedes current-frame key overwrite.
		if hasCommand {
			p.applyFlightCommand(command, c.Office.Assets.Cockpit)
		}
		directions := r.commandInput.Directions
		r.held = flightcontrol.Held{Left: directions[0], Right: directions[1], Up: directions[2], Down: directions[3]}

		beforeCollective := r.State.Collective
		beforeReady := r.liveControls.HeightSliderReady
		var engineCount byte
		if p.enginePower {
			selector := p.Initialized.Entities[p.Initialized.PlayerRow].SerializedAircraftSelector & 7
			engineCount = 2
			if selector == 3 || selector == 5 {
				engineCount = 1
			}
		}
		power := flight.Input{PoweredEngineCount: engineCount, Damage: r.Systems.Damage, FuelSupplyFailed: p.correctiveFlight && r.Systems.Damage[3]&3 == 2}
		live := r.commandInput.Live
		keyboardCollective := hasCommand && command&0xff <= 1
		if p.resilientFlight {
			was := r.collectiveOwner.Keyboard
			resolved := r.collectiveOwner.Filter(live, keyboardCollective, r.State.Collective, &r.liveControls)
			if live != nil {
				live = &resolved
			}
			if p.correctiveFlight && was && !r.collectiveOwner.Keyboard {
				r.liveControls.HeightAxisSampled = true
				r.liveControls.HeightAxisAnchor = 0
			}
			if was != r.collectiveOwner.Keyboard && p.filmPlayback == nil {
				diagnostics.Info("input", "Collective ownership changed", "keyboard", r.collectiveOwner.Keyboard, "step", r.Steps)
			}
		}
		if live != nil {
			current := p.filmPlayback == nil || p.filmPlayback.Practice || p.filmPlayback.Film.Header.InputSchema == replay.Q32InputCompatibility || replay.WingmanInput(p.filmPlayback.Film.Header.InputSchema)
			if current {
				if p.threatFeedbackMechanics {
					r.State.Collective = r.liveControls.ApplyCollectiveWithSupply(*live, &r.Axes, r.State, keyboardCollective, p.Initialized.Switches&4 == 0, r.Systems.Damage, r.Systems.DamageMasks[0], power.FuelSupplyFailed)
					if p.correctiveFlight {
						r.liveControls.ApplyHeightWithPower(*live, r.State, r.Parameters, p.Initialized.Switches&4 == 0, keyboardCollective, power, r.Systems.DamageMasks[0])
					} else {
						r.liveControls.ApplyHeightQ32Damage(*live, r.State, r.Parameters, p.Initialized.Switches&4 == 0, keyboardCollective, r.Systems.Damage, r.Systems.DamageMasks[0])
					}
				} else {
					r.State.Collective = r.liveControls.ApplyQ32(*live, &r.Axes, r.State.Collective, keyboardCollective, p.Initialized.Switches&4 == 0)
					r.liveControls.ApplyHeightQ32(*live, r.State, r.Parameters, p.Initialized.Switches&4 == 0, keyboardCollective)
				}
			} else {
				r.State.Collective = r.liveControls.Apply(*live, &r.Axes, r.State.Collective, keyboardCollective)
				if p.filmPlayback.Film.Header.InputSchema == replay.Q31PreviousInputCompatibility {
					r.liveControls.ApplyHeightV7(*live, r.State, p.Initialized.Switches&4 == 0, keyboardCollective)
				} else {
					r.liveControls.ApplyHeight(*live, r.State, r.Parameters, p.Initialized.Switches&4 == 0, keyboardCollective)
				}
			}
			if r.State.Rotor != 0 && len(r.Systems.Fuel) > 0 && r.Systems.Fuel[0] > 0 &&
				runningRotorPitch(beforeCollective) != runningRotorPitch(r.State.Collective) {
				p.requestCollectiveRotor()
			}
		} else {
			r.liveControls.HeightAxisSampled = false
			r.Axes.Advance(r.held)
		}
		if p.ViewControls.Mode == 7 {
			p.consumeMastCyclic()
		}
		// Orders are resolved at this logical input boundary, before fuel and
		// actor visitation. They are not renderer-time mutations; recorded Aux
		// carries the selected recipient even if a later view changes it.
		if p.r1Mechanics {
			for _, key := range aux {
				if _, _, ok := decodeWingmanOrder(key); ok {
					if err := p.applyWingmanAux(key); err != nil {
						c.reportDiagnostic("orders", err.Error(), err)
					}
				}
			}
		}
		burn := r.Systems.AdvanceWithFollowers(t.E0, r.State.Collective, r.Combat.FollowerFuelContext(p.nextGameplayRandom))
		p.fuelAudio(burn)
		if burn.PlayerActive && burn.PreviousFuel <= 0 {
			p.limitFlightCompressionCause("fuel-old-empty")
		}
		if p.Scene.Environment != nil {
			p.Scene.Environment.StepFlight(p.Scene.GraphicsOptions, t.E0, t.Phase)
		}
		random := p.nextGameplayRandom
		manualTargetEdges := 0
		for _, key := range aux {
			switch key {
			case 0, 1, 2, 3:
				accepted := r.Defenses.Activate(flight.DefenseAction(key), r.Systems.DamageMasks[0], r.State.Pose, t.DF, p.Renderer.Assets, random)
				p.defenseAudio(flight.DefenseAction(key), accepted)
			case 4:
				manualTargetEdges++
			case 5:
				p.applyNavigationKey("6")
			case 6:
				p.applyNavigationKey("7")
			case 7:
				if err := p.selectNextStation(); err != nil {
					c.reportDiagnostic("flight", err.Error(), err)
				}
			case 8, 9, 10:
				p.selectRocketSalvo([3]int16{1, 2, 4}[key-8])
			case 11:
				if p.r4Mechanics {
					r.Notifications.Recall()
				}
			case 12:
				if p.r4Mechanics {
					if err := p.jettisonSelectedStation(); err != nil {
						return err
					}
					// Consume the whole jettison gesture, including an overlapping
					// held Selector1; it is not a firing/selection operation.
					r.commandInput.Trigger, r.commandInput.TriggerPressed = false, false
				}
			}
		}
		if r.commandInput.Momentary {
			if p.copilotMechanics {
				if err := r.Defenses.SetR3ManualHeld(r.commandInput.jammerHeld, r.Systems.DamageMasks[0], random); err != nil {
					return fmt.Errorf("manual/automatic defense ownership: %w", err)
				}
			} else {
				r.Defenses.SetManualHeld(r.commandInput.jammerHeld, r.Systems.DamageMasks[0], random)
			}
		}
		in := flight.Input{PoweredEngineCount: engineCount, FuelSupplyFailed: power.FuelSupplyFailed, DF: t.DF, E0: t.E0, Phase: t.Phase, X: r.Axes.X, Y: r.Axes.Y, Switches: p.Initialized.Switches, MassTerm: r.MassTerm, GearFlags: r.SystemFlags, Damage: r.Systems.Damage, WindHeading: r.WindHeading, WindStrength: r.WindStrength, Random: random}
		in.ShakeDisplay = func(remaining int16) { p.playerShake.advance(remaining, t.Phase, int(p.ViewControls.Mode)) }
		if p.p4Mechanics {
			in.SourceSoundGate = 0x40 // source semantic DE40; host mute is transport policy.
			in.Warning = func(event byte) { r.sceneAudio = append(r.sceneAudio, sceneAudioCommand{event: uint16(event)}) }
		}
		// Native keyboard/default SETUP byte3 has bit02 clear. It is not
		// mission enemy quality's similarly placed bit in a different word.
		in.ControlFlags = 0
		if r.commandInput.Live != nil {
			in.ControlFlags = 2
			in.Yaw = r.liveControls.Yaw
		}
		if e := p.Scene.Environment; e != nil {
			in.GustActive = e.PulseRemaining != 0
			in.GustHeading = e.PulseDraw
			in.AutoPilotDisturbance = int16(e.CompassDraw)
		}
		q := flight.Queries{Angles: p.Renderer.Assets, Height: func(v flight.Pose) (int16, error) {
			return p.Renderer.Assets.ContactHeight(p.Initialized.World, world.QueryPose{H0: v.H0, H1: v.H1, Height: v.Height}, p.Initialized.ContactObjects())
		}, Cell: func(v flight.Pose) (byte, error) {
			return p.Renderer.Assets.SourceCellID(p.Initialized.World, world.QueryPose{H0: v.H0, H1: v.H1, Height: v.Height})
		}}
		wasContact := r.State.ValidContact
		trace, stepErr := flight.StepWithTrace(r.State, r.Parameters, in, q)
		if stepErr != nil {
			p.Error = fmt.Errorf("flight stopped: %w", stepErr)
			r.Failure = p.Error.Error()
			p.clearFlightInput()
			c.reportDiagnostic("flight", p.Error.Error(), p.Error)
			return nil // Freeze this transient sortie; keep Alt+E/Alt+Q available.
		}
		if r.State.CompressionLimit {
			p.limitFlightCompressionCause("hold-axis")
		}
		if r.State.CompressionReset {
			prior := r.Compression.Request
			r.Compression.Reset()
			p.traceCompressionTransition("route-final-or-empty-next", prior, 0)
			r.Clock.ResetElapsed()
		}
		p.syncNavigationFromFlight()
		if err := c.advanceSortieService(); err != nil {
			return err
		}
		if p.Initialized.Sortie != nil && p.Initialized.Sortie.Result != nil {
			c.rememberSortieFuel()
			return c.finishSortieResult()
		}
		r.Steps++
		if r.Steps%64 == 0 && c.filmIsolation == nil {
			diagnostics.FlightProgress(r.Steps)
		}

		family, _ := cockpit.Family(p.Cockpit.AircraftID)
		d := c.Office.Assets.Cockpit.Definitions[family]
		r.Systems.EaseEngines(t.E0, p.ViewControls.Mode == 0 && p.ViewControls.Arrangement != 2 && r.Systems.DamageMasks[0]&0x10 == 0, d.Parameters[0x1c/2] != 0)
		p.Cockpit.BlinkPhase = t.Phase
		p.Cockpit.MissionCounter = t.Phase
		// Within1543,3df9 fires before3eca's late pose publisher.3e08 selects
		// from the preceding candidate cache only when selector1 is not held.
		combatInput := p.combatInput(t.DF, t.Phase, random)
		combatInput.E0 = t.E0
		if tr := c.inputTrace; tr != nil {
			combatInput.TraceIR = func(slot int, target byte, flags uint16, active uint16) {
				tr.add(inputTraceRow{Stage: "incoming", Step: int(r.Steps), Slot: slot, Target: int(target), Flags: flags, EffectiveIR: active, Phase: t.Phase})
				if active != 0 {
					tr.pending = "active IR visit"
				}
			}
		}
		if p.r4Mechanics && trace.DesiredVerticalRateWritten {
			combatInput.PlayerDesiredVerticalRate = &trace.DesiredVerticalRate
		}
		if r.commandInput.Trigger {
			before := len(r.Combat.Events)
			p.traceWeaponAttempt("trigger boundary")
			if err := r.Combat.Fire(combatInput); err != nil {
				p.traceWeaponAttempt("error: " + err.Error())
				return fmt.Errorf("player fire: %w", err)
			}
			for _, event := range r.Combat.Events[before:] {
				if event.Kind == "launch" || event.Kind == "rejected" || event.Kind == "busy" || event.Kind == "unsupported" {
					p.traceWeaponAttempt(event.Kind + ": " + event.Source)
				}
			}
		} else {
			for i := 0; i < manualTargetEdges; i++ {
				if r.Avionics != nil {
					r.Avionics.SelectManual()
				}
			}
		}
		p.publishFlight()
		//157f is the NPC dispatcher, not target selection. Only its mechanical
		// destruction successors run here;1584 projectiles precede158a's scan.
		if err := p.advanceCombat(combatInput); err != nil {
			return err
		}
		if err := p.advanceAcceptedPlayerHits(t.Phase); err != nil {
			return err
		}
		oldRetryBounds := r.Avionics.QueryRetryBoundHits
		if err := p.advanceAvionics(random, c.Office.Assets.Cockpit); err != nil {
			return err
		}
		if r.Avionics.CompressionLimited {
			p.limitFlightCompressionCause("contact-kind15-object30")
		}
		if r.Avionics.QueryRetryBoundHits != oldRetryBounds {
			c.reportDiagnostic("flight", "Target LOS retry reached an unsupported retained-status cycle; lock cleared.", nil)
		}
		priorCameraMode := p.ViewControls.Mode
		if err := p.ViewControls.Tick(p.cameraInputs(), p.Renderer.Assets); err != nil {
			return err
		}
		projectileReturn := priorCameraMode == 6 && p.ViewControls.Mode != 6
		if projectileReturn {
			p.requestCollectiveRotor()
		}
		p.publishView()
		p.filterSceneInstances()
		// Source1fac runs after manual input. With CP/G defense enabled, its
		// reached physical AEB requests share the one effective jammer state on
		// eligible even phases. Historical films retain the former subset.
		if p.copilotMechanics {
			p.publishTargeting() // Current body/mast view after its late source update.
			late := p.combatInput(t.DF, t.Phase, random)
			late.E0 = t.E0
			if err := p.advanceCopilotWeapons(late); err != nil {
				return fmt.Errorf("late copilot weapon: %w", err)
			}
			if err := p.advanceCopilotDefense(t.E0, byte(t.Phase), t.DF, random); err != nil {
				return err
			}
		} else {
			requests := r.Combat.IncomingThreat
			if r.commandInput.Momentary {
				if r.commandInput.jammerHeld[0] {
					requests |= 0x40
				}
				if r.commandInput.jammerHeld[1] {
					requests |= 0x80
				}
			}
			r.Defenses.ReconcileJammerRequests(p.Initialized.Switches, r.Systems.DamageMasks[0], t.E0, byte(t.Phase), requests)
			if p.Cockpit.Defense != nil {
				p.Cockpit.Defense.IRActive = int16(r.Defenses.IRActive)
				p.Cockpit.Defense.RadarSignature = int16(r.Defenses.RadarSignature)
			}
		}
		r.Defenses.AdvanceDecoy()              // source1faf precedes reached HUD2084.
		p.refreshPlayerDecoyScene()            // Pure scene publication uses final late selector/pose.
		p.advanceCombatObjectives(combatInput) // Source202f reaches the bounded count-completion phase.
		if err := c.advanceDevelopmentScenario(); err != nil {
			return err
		}
		if p.ResultTargets != nil && p.ResultTargets.ProgressFailure != "" {
			c.reportDiagnostic("flight", "Results unavailable: "+p.ResultTargets.ProgressFailure, nil)
		}
		p.Scene.Environment.FinishFlightFrame()
		if err := p.advanceAvionicsReference(c.Office.Assets.Cockpit); err != nil {
			return err
		}
		if err := p.advanceAvionicsDisplays(c.Office.Assets.Cockpit); err != nil {
			return err
		}
		p.advanceCombatNotifications(t.E0)
		if r.State.Terminal != nil {
			if err := c.advancePlayerTerminal(); err != nil {
				return err
			}
		}
		c.rememberSortieFuel()
		p.endPresentationStep()
		// Expiry restores a saved camera position discontinuously. As for manual
		// view changes, snap presentation history without clearing flight input.
		if p.presentationDiscontinuity(projectileReturn, wasContact != r.State.ValidContact, r.State.BoundaryCrossed != 0) {
			p.resetPresentation()
		}
		if tr := c.inputTrace; tr != nil {
			def := r.Defenses.EffectiveR3(r.Systems.DamageMasks[0])
			row := inputTraceRow{Stage: "logical", Step: int(r.Steps), Phase: t.Phase, E0: t.E0, Compression: r.Compression.Rate(), Before: beforeCollective, After: r.State.Collective, Vertical: r.State.Vertical, Ready: r.liveControls.HeightSliderReady, Fallback: r.liveControls.HeightManualFallback, Keyboard: hasCommand && command&255 <= 1, Hold: r.State.HeightControl, Hover: r.State.Hover, Rotor: r.State.Rotor, Critical: r.Systems.Damage[9], Equipment: r.Systems.DamageMasks[0], IR: r.commandInput.jammerHeld[1], Charge: def.IRCharge, EffectiveIR: def.IRActive}
			if def.R3 != nil {
				row.Automatic = def.R3.Automatic[1]
			}
			if l := r.commandInput.Live; l != nil {
				row.Lever = l.Lever
				row.Epoch = l.Epoch
				row.Mode = l.Mode
			}
			tr.add(row)
			if beforeReady != row.Ready && row.Keyboard {
				tr.pending = "keyboard takeover"
			}
			tr.flush()
		}
		if p.PaletteState != nil {
			p.PaletteState.Advance(p.Scene.GraphicsOptions, uint64(r.Steps), p.Scene.Environment)
		}
		if err := p.commitReferenceFrame(c.Office.Assets.Cockpit); err != nil {
			return err
		}
		if r.TerminalPresentation.Completed {
			return c.finishPlayerTerminal()
		}
		if c.liveMissionClockLimit(t.Phase) {
			return c.confirmSortieEnd()
		}
		if r.Paused || p.Paused {
			// A dev-only staged approach can pause after a real objective event.
			// Discard this host batch; it is not permission to run hidden quanta.
			p.clearFlightInput()
			break
		}
		c.filmFinishStep()
		if c.presentNaturalSortieEnd() {
			break
		}
		if r.State.Terminal != nil {
			// Publish the contact frame once, then discard this host update's
			// remaining clock quanta along with pending controls.
			p.clearFlightInput()
			break
		}
	}
	return nil
}

func (p *RenderingPreview) publishFlight() {
	r := p.Flight
	p.Cockpit.CompressionRequest = r.Compression.Request
	s := r.State
	m := p.Initialized
	e := &m.Entities[m.PlayerRow]
	e.Pose = gamestate.InitializedPose{H0: s.Pose.H0, H1: s.Pose.H1, Height: uint16(s.Pose.Height), Angles: s.Pose.Angles}
	m.Player = e.Pose
	for i, v := range s.Pose.Angles {
		binary.LittleEndian.PutUint16(e.DynamicRaw[8+2*i:], v)
	}
	// The late source publisher writes pose/angles only. The old raw motion
	// carrier is retained; live player velocity belongs to PlayerFlight.
	binary.LittleEndian.PutUint32(e.DynamicRaw[14:], s.Pose.H0)
	binary.LittleEndian.PutUint32(e.DynamicRaw[18:], s.Pose.H1)
	binary.LittleEndian.PutUint16(e.DynamicRaw[22:], uint16(s.Pose.Height))
	for i := range p.instances {
		if p.instances[i].ID == e.ID {
			p.instances[i].Pose.H0, p.instances[i].Pose.H1, p.instances[i].Pose.Height = e.Pose.H0, e.Pose.H1, e.Pose.Height
			p.instances[i].Angles = e.Pose.Angles
			b := r.GearGraph
			if r.SystemFlags&1 != 0 && p.instances[i].ModelID == b.ModelID {
				child := b.DownChild
				if r.SystemFlags&4 != 0 {
					child = b.UpChild
				}
				p.instances[i].GraphChild = &render3d.GraphChildOverride{Node: b.Node, Side: b.Side, Child: child, Expected: b.DownChild}
			}
		}
	}
	in := &p.Cockpit
	in.Height, in.Pitch, in.Heading, in.Bank = uint16(s.Pose.Height), s.Pose.Angles[0], s.Pose.Angles[1], s.Pose.Angles[2]
	in.Vertical, in.Lateral = s.IndicatedSpeed, s.Vertical
	in.HUDScale, in.HUDVertical = s.RadarAltitude, r.LowLimit+128
	in.AutoPilot, in.AutoHover = s.HeightControl, s.Hover
	in.Rotor = s.Rotor
	in.SystemFlags, in.SecondaryMFD = r.SystemFlags, r.SecondaryMFD

	in.PrimaryMFD = r.PrimaryMFD
	in.PrimaryMap, in.SecondaryMap = r.PrimaryMFD == 0, r.SecondaryMFD == 0
	in.Fuel = r.Systems.Fuel[0]
	in.RocketSalvo = r.RocketSalvo
	in.FuelMaximum = 100
	if m.Stores != nil && len(m.Stores.Ships) > 0 {
		ship := &m.Stores.Ships[0]
		ship.Fuel = in.Fuel
		for i, v := range ship.Stations {
			in.Stations[i] = cockpit.Station{Present: v.Present, SystemID: int(v.CompactID), Name: v.ShortName, Quantity: v.Ammunition, Flags: byte(v.Flags), DefinitionFlags: v.Flags, Mode: v.Mode}
		}
	}
	p.publishDefenseState()

	in.Damage[0], in.Damage[1] = r.Systems.DamageMasks[0], r.Systems.DamageMasks[1]
	copy(in.Damage[2:], r.Systems.Damage[:8]) // HUD5fe..607, not physical600..609.
	in.DamageOverlay, in.DamageOverlayPoints = r.Systems.Damage[8], r.DamageOverlay
	in.Bars = &cockpit.BarInputs{LoadOrdinal: s.Collective, FuelNumerator: in.Fuel, FuelDivisor: 100, Current: r.Systems.Engines}
	// Use the existing quadrant-aware source lookup, not table indexing aliases.
	in.AttitudeMatrix = [3]int16{render3d.Sine(s.Pose.Angles[0], p.Renderer.Assets.Sine), render3d.Sine(s.Pose.Angles[2]+0x4000, p.Renderer.Assets.Sine), render3d.Sine(s.Pose.Angles[2], p.Renderer.Assets.Sine)}
	if in.Map != nil {
		in.Map.H0, in.Map.H1 = s.Pose.H0, s.Pose.H1
		in.Map.Heading = s.Pose.Angles[1]
	}
	p.publishNavigation()
	p.publishAvionics()
	p.Dirty = true
}

// selectNextStation is source3920h Space selection, not firing or jettison.
// Compact0 is NO BAY, compact1 EMPTY remains selectable even with no ammo.
func (p *RenderingPreview) selectNextStation() error {
	if p.Initialized.Stores == nil || len(p.Initialized.Stores.Ships) == 0 {
		return fmt.Errorf("runtime stores unavailable for selection")
	}
	var ids [4]byte
	for i, s := range p.Initialized.Stores.Ships[0].Stations {
		ids[i] = s.CompactID
	}
	next, err := flight.NextSelectedStation(p.Initialized.SelectedStation, ids)
	if err != nil {
		return err
	}
	p.Initialized.SelectedStation = next
	p.Cockpit.SelectedStation = next
	return nil
}
