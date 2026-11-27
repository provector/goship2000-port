package menus

import (
	"encoding/binary"
	"fmt"
	"gunship2000-assets/game/avionics"
	"gunship2000-assets/game/cockpit"
	"gunship2000-assets/game/diagnostics"
	"gunship2000-assets/game/navigation"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/game/world"
	"slices"
	"strings"
)

func (p *RenderingPreview) initializeNavigation() {
	if p.Flight == nil {
		return
	}
	for _, e := range p.Initialized.Entities {
		if e.FlightSlot >= 0 && e.FlightSlot < 5 {
			p.Flight.Navigation[e.FlightSlot].Raw = e.ControlRaw
		}
	}
	p.Flight.Navigation[0].ToFlight(p.Flight.State)
	p.publishNavigation()
}

// P4 actors and the contact tail now have real CONTROL writers. Navigation is
// their UI carrier, not an independent stale route owner. Snapshot only at a
// reached logical publication/entry; rendering never calls this synchronization.
func (p *RenderingPreview) syncFollowerNavigation() {
	if p == nil || !p.p4Mechanics || p.Flight == nil || p.Initialized == nil {
		return
	}
	for _, e := range p.Initialized.Entities {
		if e.FlightSlot > 0 && e.FlightSlot < 5 {
			p.Flight.Navigation[e.FlightSlot].Raw = e.ControlRaw
		}
	}
}

func (p *RenderingPreview) applyNavigationKey(key string) {
	if p.Flight == nil {
		return
	}
	r := &p.Flight.Navigation[0]
	switch key {
	case "6":
		r.Next()
	case "7":
		r.Previous()
	default:
		return
	}
	r.ToFlight(p.Flight.State)
	p.publishNavigation()
	p.Dirty = true
}
func (p *RenderingPreview) syncNavigationFromFlight() {
	if p.Flight == nil {
		return
	}
	p.Flight.Navigation[0].FromFlight(p.Flight.State)
	p.publishNavigation()
}
func (p *RenderingPreview) publishNavigation() {
	if p.Flight == nil {
		return
	}
	r := &p.Flight.Navigation[0]
	s := p.Flight.State
	in := &cockpit.NavigationInputs{Active: r.Index()}
	from := navigation.Point{H0: s.Pose.H0, H1: s.Pose.H1}
	for n := 1; n <= 3; n++ {
		v := r.Point(n)
		if !v.Present() {
			continue
		}
		a, d := navigation.Bearing(from, v, p.Renderer.Assets)
		in.Points[n-1] = cockpit.WaypointDisplay{Present: true, Bearing: a, Range: d}
	}
	p.Cockpit.Route = in
	p.Cockpit.Navigation = &cockpit.NavigationBearing{HUDMarker: p.fieldCorrectionsMechanics}
	if r.Index() >= 1 && r.Index() <= 3 && r.Point(int(r.Index())).Present() {
		p.Cockpit.Navigation.Active = true
		p.Cockpit.Navigation.Bearing = in.Points[r.Index()-1].Bearing
	}
	for _, e := range p.Initialized.Entities {
		if e.FlightSlot == 0 {
			p.Initialized.Entities[e.RuntimeRow].ControlRaw = r.Raw
			break
		}
	}
}
func (c *FrontEnd) openGPS() error {
	p := c.Rendering
	if p == nil || p.Flight == nil || p.Inspection {
		return nil
	}
	if p.Flight.State.Terminal != nil {
		return nil
	}
	outer := p.filmTransaction
	p.filmTransaction = true
	defer func() {
		p.filmTransaction = outer
		if !outer {
			c.recordFilmBoundary(filmBoundary{Kind: "gps-open"})
		}
	}()
	var poses [5]navigation.Point
	var active [5]bool
	var heads [5]uint16
	for _, e := range p.Initialized.Entities {
		if e.FlightSlot >= 0 && e.FlightSlot < 5 {
			n := e.FlightSlot
			poses[n] = navigation.Point{H0: e.Pose.H0, H1: e.Pose.H1}
			active[n] = e.Participation != 0
			if p.r1Mechanics {
				active[n] = e.ControlRaw[1] != 0
			}
			heads[n] = e.Pose.Angles[1]
		}
	}
	active[0] = true
	p.syncFollowerNavigation()
	g := navigation.New(&p.Flight.Navigation, poses, active)
	if p.filmPlayback == nil || p.filmPlayback.Practice || (p.filmPlayback.Film.Header.InputSchema == replay.P6InputCompatibility || replay.ContinuousInput(p.filmPlayback.Film.Header.InputSchema)) {
		g.UseSourceSupportInterlocks()
	}
	if p.p4Mechanics && p.Initialized.Support != nil {
		s := p.Initialized.Support
		g.EnableSupport(s.Availability, s.Request, p.Initialized.PlayerRow)
	}
	if c.wingmanRouteEditProfile() {
		g.PreserveWingmanRouteEdits()
	}
	if p.resilientFlight {
		g.StageWingmanRoutes(func(slot int) (uint16, bool) {
			if slot < 1 || slot >= len(p.Initialized.Entities) || p.Initialized.Stores == nil || slot >= len(p.Initialized.Stores.Damage) {
				return 0, false
			}
			e := p.Initialized.Entities[slot]
			controllable := e.Kind >= 0x20 && e.Kind <= 0x23 || p.correctiveFlight && e.Participation == 1 && e.Kind >= 0x24 && e.Kind <= 0x26
			return uint16(e.Kind)<<8 | uint16(e.Participation), e.Participation < 2 && p.Initialized.Stores.Damage[slot][9] == 0 && e.FlightSlot == slot && e.ControlRaw == p.Flight.Navigation[slot].Raw && controllable && e.ControlRaw[1] != 0
		})
	}
	g.BlinkPhase = p.Cockpit.BlinkPhase
	g.HeadingPerturbation = p.Cockpit.HeadingPerturbation
	if p.Cockpit.Map != nil {
		g.SweepPhase = p.Cockpit.Map.SweepPhase
	}
	rows := make([][32]byte, len(p.Initialized.Rows))
	for i, row := range p.Initialized.Rows {
		rows[i] = row.Raw
	}
	if home, err := navigation.ResolveHome(rows, p.Flight.Navigation[0].Raw, 0); err == nil {
		g.Home = home
	} else {
		g.HomeError = err.Error()
	}
	g.Headings = heads
	if c.Mission != nil && c.Mission.Data != nil {
		if base, ok := c.Mission.Data.Objectives["base"]; ok {
			g.Base = navigation.Point{H0: uint32(base.Point.X), H1: uint32(base.Point.Y)}
		}
		if farp, ok := c.Mission.Data.Objectives["FARP"]; ok && farp.DisplayKind != 0 {
			g.FARP = navigation.Point{H0: uint32(farp.Point.X), H1: uint32(farp.Point.Y)}
			g.FARPPresent = true
		}
	}
	objectives, err := c.gpsPanelObjectives()
	if err != nil {
		return err
	}
	g.Panel = &navigation.PanelSnapshot{Objectives: objectives}
	if p.Flight.Avionics != nil {
		p.Flight.Avionics.GPSRefresh(p.contactEntities())
		if p.p4Mechanics && p.Initialized.Support != nil {
			p.Flight.Avionics.MapPhase(p.contactEntities(), p.Initialized.Support.Request != 0)
		}
		g.Markers = p.contactMapMarkers(true)
	}
	g.DeliverySites = p.discoveredDeliverySites(objectives)
	p.publishMapRoutes()
	if p.Cockpit.Map != nil && p.Cockpit.Map.Mission != nil {
		mission := *p.Cockpit.Map.Mission
		g.Mission = &mission
	}
	g.SetR11Presentation(p.r11Mechanics)
	g.ClearConfirmedEmpty = p.independentManeuvers
	g.SetFollowerOrders(p.r1Mechanics, func(slot int) {
		var effects [5]navigation.OrderEffects
		kind := byte(0x23)
		if g.Routes[slot].Index() == 0 {
			kind = 0x20
		}
		effects[slot] = navigation.OrderEffects{Kind: kind, ChangeKind: true, ResetPhase: true}
		p.publishFollowerOrders(effects)
	})
	p.GPS = g
	p.publishSupportStrike()
	p.publishGPSPanel()
	p.clearFlightInput()
	p.Dirty = true
	return nil
}

// Role flags follow the actual formation, including a promoted secondary.
// Any sighted member identifies the site, even when it has no troop contact.
func (p *RenderingPreview) discoveredDeliverySites(objectives [2]navigation.PanelObjective) []navigation.Point {
	var sites []navigation.Point
	if p.Flight.Avionics == nil {
		return nil
	}
	for role, o := range objectives {
		if !o.Available || binary.LittleEndian.Uint16(o.Raw[32:]) != 1 {
			continue
		}
		site := navigation.Point{H0: binary.LittleEndian.Uint32(o.Raw[34:]), H1: binary.LittleEndian.Uint32(o.Raw[38:])}
		if slices.Contains(sites, site) {
			continue
		}
		for _, e := range p.Initialized.Entities {
			if e.Object.Raw[0]&(0x10<<role) != 0 && p.Flight.Avionics.PlayerSighted(e.RuntimeRow) {
				sites = append(sites, site)
				break
			}
		}
	}
	return sites
}

// Use the evolving full TARGET records when present. Broader flight modes may
// have no admitted result carrier; prepare their established launch names on a
// private MissionData copy, without changing result admission or saved state.
func (c *FrontEnd) gpsPanelObjectives() ([2]navigation.PanelObjective, error) {
	var out [2]navigation.PanelObjective
	p := c.Rendering
	targets := p.ResultTargets
	if targets == nil {
		if c.Mission == nil || c.Mission.Data == nil {
			return out, fmt.Errorf("GPS needs retained mission TARGET state")
		}
		m := *c.Mission.Data
		if m.Targets == nil && m.SourcePreparation == nil {
			// This display needs only the two established name/status records.
			// An unrelated alternate countdown cannot deny previously supported
			// GPS use in Flight/Campaign, nor admit those result lifecycles.
			m.Objectives = make(map[string]gamestate.MissionObjective, 2)
			for _, role := range []string{"primary", "secondary"} {
				if o, ok := c.Mission.Data.Objectives[role]; ok {
					m.Objectives[role] = o
				}
			}
		}
		if m.Targets == nil {
			var err error
			if m.SourcePreparation != nil {
				err = gamestate.PrepareMissionTargets(nil, &m)
			} else {
				if c.Office == nil || c.Office.Assets.Missions == nil {
					return out, fmt.Errorf("GPS TARGET preparation assets unavailable")
				}
				err = gamestate.PrepareMissionTargets(c.Office.Assets.Missions, &m)
			}
			if err != nil {
				return out, fmt.Errorf("GPS TARGET: %w", err)
			}
		}
		targets = m.Targets
	}
	for i := range out {
		out[i] = navigation.PanelObjective{Available: true, Raw: targets.Records[i+2]}
	}
	if p.ResultTargets == nil && p.Flight != nil && p.Flight.Combat != nil {
		// The existing logical count-objective producer owns these current
		// statuses even when career results for this mission are not admitted.
		binary.LittleEndian.PutUint16(out[0].Raw[32:], uint16(p.Flight.Combat.Primary.DisplayKind))
		binary.LittleEndian.PutUint16(out[1].Raw[32:], uint16(p.Flight.Combat.Secondary.DisplayKind))
	}
	return out, nil
}

func (p *RenderingPreview) publishGPSPanel() {
	if p == nil || p.GPS == nil || p.Flight == nil {
		return
	}
	g, r := p.GPS, p.Flight
	slot := g.Selected
	if slot < 0 || slot >= len(r.Navigation) {
		return
	}
	g.CargoReleaseAvailable = false
	panel := navigation.PanelSnapshot{Selected: slot, Player: byte(p.Initialized.PlayerRow),
		LeaderA: p.gpsLeaderSelector(4), LeaderB: p.gpsLeaderSelector(3), Control: r.Navigation[slot].Raw}
	if g.Panel != nil {
		panel.Objectives = g.Panel.Objectives
	}
	for _, e := range p.Initialized.Entities {
		if e.FlightSlot == slot {
			panel.Callsign = stringTrimZero(e.Object.Raw[15:27])
			panel.ShipFlags = byte(e.SerializedAircraftSelector)
			panel.Height = int16(e.Pose.Height)
			break
		}
	}
	if stores := p.Initialized.Stores; stores != nil && slot < len(stores.Ships) {
		ship := stores.Ships[slot]
		g.CargoReleaseAvailable = (slot == p.Initialized.PlayerRow || p.p4Mechanics) && ship.Flags&7 == 6 && ship.Flags&0xf8 != 0
		panel.ShipFlags = ship.Flags
		panel.Fuel, panel.Chaff, panel.Flares = uint16(ship.Fuel), uint16(ship.Chaff), uint16(ship.Flares)
		for j, st := range ship.Stations {
			panel.Stations[j] = navigation.PanelStation{SystemID: st.CompactID, Name: st.ShortName, Quantity: uint16(st.Ammunition)}
		}
	}
	if slot == p.Initialized.PlayerRow {
		panel.Height = r.State.Pose.Height
		panel.FuelSeverity, panel.FuelSeverityKnown = r.Systems.DamageMasks[0], true
		panel.Damage[0], panel.Damage[1] = r.Systems.DamageMasks[0], r.Systems.DamageMasks[1]
		copy(panel.Damage[2:], r.Systems.Damage[:8])
		panel.DamageKnown = true
	}
	if p.r11Mechanics && slot != p.Initialized.PlayerRow && p.Initialized.Stores != nil && slot < len(p.Initialized.Stores.Damage) {
		if p.wingmanDamageMechanics && p.Initialized.Stores.NPCDamage != nil {
			damage := p.Initialized.Stores.Damage12(slot)
			copy(panel.Damage[:], damage[:10])
			panel.FuelSeverity, panel.FuelSeverityKnown = damage[0], true
			panel.DamageKnown = true
		} else {
			copy(panel.Damage[2:], p.Initialized.Stores.Damage[slot][:8])
			panel.ScalarDamageKnown = true
		}
	}
	// Historical nonplayer interpretations only own physical600 onward. R22's
	// explicit full record supplies5fe/5ff; never shift physical bytes into masks.
	g.Panel = &panel
}

func (p *RenderingPreview) gpsLeaderSelector(row int) int8 {
	// DATA5a5/55f are fixed control-row aliases even in Single, where actors
	// three/four need not be flight members. Navigation owns current flight
	// rows; other rows retain their actual initialized ControlRaw carrier.
	for _, e := range p.Initialized.Entities {
		if e.RuntimeRow == row && e.FlightSlot != row {
			return int8(e.ControlRaw[5])
		}
	}
	return int8(p.Flight.Navigation[row].Raw[5])
}
func (c *FrontEnd) closeGPS() {
	p := c.Rendering
	if p == nil || p.GPS == nil {
		return
	}
	outer := p.filmTransaction
	p.filmTransaction = true
	defer func() {
		p.filmTransaction = outer
		if !outer {
			c.recordFilmBoundary(filmBoundary{Kind: "gps-close"})
		}
	}()
	if p.resilientFlight {
		p.syncFollowerNavigation()
	}
	p.GPS.CancelUnfinishedWingmanRoute()
	if p.Cockpit.Map != nil {
		p.Cockpit.Map.SweepPhase = p.GPS.SweepPhase
	}
	if p.p4Mechanics && p.Initialized.Support != nil {
		p.Initialized.Support.Availability = p.GPS.CloseSupport(p.Initialized.Support.Request)
	}
	if p.r1Mechanics {
		p.publishFollowerOrders([5]navigation.OrderEffects{})
		var contacts *[80]avionics.Contact
		if p.Flight.Avionics != nil {
			contacts = &p.Flight.Avionics.Contacts
		}
		if err := p.Flight.Combat.FinalizeFollowerRoutes(p.nextGameplayRandom, contacts); err != nil {
			if p.Flight.Failure == "" {
				p.Flight.Failure = err.Error()
				c.reportDiagnostic("orders", err.Error(), err)
			}
		}
		p.syncFollowerNavigation()
	}
	p.GPS = nil
	p.Flight.gpsNeedsRelease = false
	p.Flight.Navigation[0].ToFlight(p.Flight.State)
	p.publishNavigation()
	p.clearFlightInput()
	p.Dirty = true
}
func (c *FrontEnd) handleGPS(a Actions) error {
	recordedAction := a
	p := c.Rendering
	var support *navigation.SupportCommand
	outer := p.filmTransaction
	p.filmTransaction = true
	defer func() {
		p.filmTransaction = outer
		if !outer {
			if support != nil {
				c.recordFilmBoundary(filmBoundary{Kind: "gps-support", Action: recordedAction, Support: support})
			} else {
				c.recordFilmBoundary(filmBoundary{Kind: "gps", Action: recordedAction})
			}
		}
	}()
	g := p.GPS
	if g == nil {
		return nil
	}
	dx, dy := int16(0), int16(0)
	if a.PointerRelative && !a.PointerReset {
		dx, dy = int16(a.PointerDX*16), int16(a.PointerDY*16)
	}
	// The real keyboard producer071d:05fd ramps held axes; GPS has its
	// own filter and UI clock so it cannot steer flight or depend on render FPS.
	p.Flight.gpsHeld = a.SceneDirections
	if a.SceneDirections == [4]bool{} { // direct semantic edge callers (native uses held state).
		if a.Left {
			dx -= 16
		}
		if a.Right {
			dx += 16
		}
		if a.Up {
			dy -= 16
		}
		if a.Down {
			dy += 16
		}
	}
	viewKey := a.ViewKey
	if a.ViewShift {
		viewKey = 0
	} // source0398 accepts BIOS3b00..4300, not Shift-F-key codes.
	// C is an overview-only cargo transaction before route editing dispatch.
	// Its SHIP mutation is not part of the route rollback snapshot. Nonplayer
	// orders/operations remain unavailable in this bounded player integration.
	if strings.EqualFold(a.GPSKey, "c") && !g.Low && (g.Selected == p.Initialized.PlayerRow || p.p4Mechanics) && viewKey == 0 && !a.Cancel && !a.Selector1 {
		p.Flight.Combat.DropCargo(g.Selected)
		p.collectCombatEvents()
		p.publishObjectiveDisplays()
		p.filterSceneInstances()
	}
	modern := p.filmPlayback == nil || p.filmPlayback.Practice || replay.HeldInput(p.filmPlayback.Film.Header.InputSchema)
	confirmPartial := c.simulationProfile == "" || c.filmIsolation != nil && c.filmIsolation.Mode == "practice"
	if playback := p.filmPlayback; playback != nil {
		confirmPartial = playback.Film.Header.InputSchema == replay.RouteConfirmationInput
	}
	g.SetPartialRouteConfirmation(confirmPartial)
	g.SetNativeSelectors(modern)
	current := p.filmPlayback == nil || p.filmPlayback.Practice || (p.filmPlayback.Film.Header.InputSchema == replay.Q32InputCompatibility || replay.WingmanInput(p.filmPlayback.Film.Header.InputSchema))
	g.SetEntryOnlySelectors(current)
	if !current {
		a.Selector1Down = false
		a.SelectorHoldValid = false
	}

	// Only the overview ordinary-order branch owns these keys. Modal route,
	// Support and detail gestures keep their existing input priority.
	if p.resilientFlight {
		p.syncFollowerNavigation()
	}
	key := strings.ToLower(a.GPSKey)
	if p.r1Mechanics && !g.Low && !g.Editing && (g.Support == nil || !g.Support.Editing) && viewKey == 0 && !a.Cancel && !a.Selector1 && (g.Selected != 0 && strings.Contains("hsalbdgw", key) && len(key) == 1 || g.Selected == 0 && key == "w") {
		if e := p.wingmanOrder(g.Selected, key, true); e != nil {
			c.reportDiagnostic("orders", e.Error(), e)
		}
		a.GPSKey = ""
	}
	wasEditing := g.Editing
	previous := g.Routes[g.Selected]
	slot := g.Selected
	exit, err := g.Handle(navigation.Input{PrimaryHeld: a.Selector1Down, HoldValid: a.SelectorHoldValid, Key: a.GPSKey, Escape: a.Cancel, Selector1: a.Selector1, Selector2: a.Selector2 || a.Backspace, ViewKey: viewKey, AxisX: dx, AxisY: dy, Pointer: a.Inside && !a.PointerRelative && !a.PointerReset, X: a.X, Y: a.Y})
	if p.filmPlayback == nil && wasEditing && !g.Editing && slot != 0 {
		diagnostics.Info("gps", "Wingman route edit ended", "actor", slot, "changed", previous != g.Routes[slot], "step", p.Flight.Steps)
	}
	if p.p4Mechanics {
		support = g.TakeSupportCommand()
		if support != nil {
			if p.Initialized.Support == nil {
				return fmt.Errorf("GPS support has no admitted runtime owner")
			}
			command := p.Initialized.Support.Command
			if p.p5Mechanics {
				command = p.Initialized.Support.CommandP5
			}
			if e := command(gamestate.InitializedPose{H0: support.Point.H0, H1: support.Point.H1}, support.Cancel); e != nil {
				return e
			}
			if p.filmPlayback != nil {
				v := *support
				p.filmPlayback.reachedSupport = &v
			}
		}
	}
	if p.r1Mechanics {
		p.publishFollowerOrders([5]navigation.OrderEffects{})
	}
	p.publishSupportStrike()
	p.publishGPSPanel()
	p.Flight.Navigation[0].ToFlight(p.Flight.State)
	p.publishNavigation()
	p.Dirty = true
	if err != nil {
		c.reportDiagnostic("gps", err.Error(), err)
		return nil
	}
	if exit != 0 {
		c.closeGPS()
		if exit > 0 {
			a := Actions{ViewKey: exit}
			err := c.handleSceneView(a)
			c.recordFilmBoundary(filmBoundary{Kind: "control", Action: a})
			return err
		}
	}
	return nil
}
func (c *FrontEnd) gpsFrame() []byte {
	out := make([]byte, 64000)
	p := c.Rendering
	if p == nil || p.GPS == nil {
		return out
	}
	w := p.Renderer.Assets.Worlds[p.Scene.World]
	if err := p.GPS.Draw(out, w, c.Office.Assets.Cockpit); err != nil {
		p.Error = fmt.Errorf("GPS: %w", err)
	}
	return out
}

func stringTrimZero(raw []byte) string {
	return strings.TrimRight(strings.SplitN(string(raw), "\x00", 2)[0], " ")
}

// Initialization copies source TARGET display names/categories, not Orders prose.
// Later objective events must update this transient snapshot; no achieved event
// or RawKind-to-DisplayKind remapping is synthesized by drawing.
func (p *RenderingPreview) initializeNavigationObjectives(m *gamestate.MissionData) {
	if m == nil {
		return
	}
	for i, role := range []string{"primary", "secondary"} {
		if o, ok := m.Objectives[role]; ok {
			p.Cockpit.Objectives[i] = cockpit.ObjectiveDisplay{Available: true, Text: o.Name, Status: uint16(o.DisplayKind)}
		}
	}
	if p.Cockpit.Map != nil {
		point := func(role string) cockpit.MapObjective {
			o, ok := m.Objectives[role]
			if !ok {
				return cockpit.MapObjective{}
			}
			v := cockpit.MapObjective{Status: int16(o.DisplayKind), H0: uint32(o.Point.X), H1: uint32(o.Point.Y)}
			if o.Endpoint != nil {
				v.EndpointH0, v.EndpointH1 = uint32(o.Endpoint.X), uint32(o.Endpoint.Y)
				// Source015b:053c..0582 shifts signed32 deltas by6 BEFORE
				// taking their low words;2814 receives(-deltaH0,deltaH1).
				dx := int16((o.Endpoint.X - o.Point.X) >> 6)
				dz := int16((o.Endpoint.Y - o.Point.Y) >> 6)
				v.EndpointDirection = world.SourceAngle(-dx, dz, p.Renderer.Assets)
			}
			return v
		}
		p.Cockpit.Map.Mission = &cockpit.MapMission{Base: point("base"), FARP: point("FARP"), Primary: point("primary"), Secondary: point("secondary"), PassThrough: p.Initialized.Switches&0x800 != 0}
		if o, ok := m.Objectives["FARP"]; ok {
			p.Cockpit.Map.Mission.FARPCount = int16(o.Count)
		}
	}
}

// tickGPS uses the existing70Hz native controller cadence. Original GPS polling
// wall-time is not calibrated. Source axis ramp/SAR-release and word consumers
// are preserved; no flight/animation/RNG counter is advanced by this modal tick.
func (c *FrontEnd) tickGPS() error {
	tick, err := c.consumeGPSTick()
	if err != nil {
		return err
	}
	if tick != nil {
		c.recordFilmBoundary(filmBoundary{Kind: "gps-tick", GPSTick: tick})
	}
	return nil
}
