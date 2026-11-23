package menus

import (
	"encoding/binary"
	"fmt"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/render3d"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/game/world"
	"math/rand"
)

// DevelopmentScenario is an explicitly supplied native test setup, never a save
// format, ordinary menu action, original-runtime witness or follower simulation.
type DevelopmentScenario struct {
	Name             string                `json:"name"`
	ScratchSave      string                `json:"scratch_save"`
	Stage            string                `json:"stage"`
	Seed             int64                 `json:"seed"`
	MissionType      gamestate.MissionType `json:"mission_type"`
	Theatre          gamestate.Theatre     `json:"theatre"`
	World            string                `json:"world"`
	Scenario         string                `json:"scenario"`
	ObjectSHA256     string                `json:"object_sha256"`
	AcceptedAircraft []int                 `json:"accepted_aircraft"` // Frontend IDs; numeric JSON, not SHIP or base64.
	AcceptedRows     [][36]byte            `json:"accepted_loadout_rows"`
	LaunchSwitches   uint16                `json:"launch_switches"`
	InitialRNG       uint16                `json:"initial_gameplay_rng"`
	Pose             flight.Pose           `json:"supplied_pose"`
	SupportHeight    int16                 `json:"source_support_height"`
	Rotor            int16                 `json:"supplied_rotor"`
	Collective       int16                 `json:"supplied_collective"`
	FlightStage      int16                 `json:"supplied_flight_stage"`
	Paused           bool                  `json:"starts_paused"`
	Instructions     string                `json:"instructions"`
	Profile          string                `json:"supplied_profile,omitempty"`
	Expected         string                `json:"expected_outcome,omitempty"`
	GoalRole         int                   `json:"current_goal_role,omitempty"`
	TargetRows       []int                 `json:"objective_target_rows,omitempty"`
	Adjustments      []string              `json:"supplied_adjustments"`
	ResultBoundary   string                `json:"result_boundary"`
	DamagePlan       string                `json:"accepted_hit_plan,omitempty"`
	DamageInputs     []DevelopmentHitInput `json:"supplied_accepted_hits,omitempty"`
	PrimaryTARGET    [62]byte              `json:"primary_TARGET62"`
	BaseTARGET       [62]byte              `json:"base_TARGET62"`
	FARPTARGET       [62]byte              `json:"farp_TARGET62"`
}

type DevelopmentHitInput struct {
	VictimRow       int    `json:"victim_row"`
	ProjectileFlags uint16 `json:"projectile_flags"`
	RNGState        uint16 `json:"supplied_shared_rng_state"`
	Source          string `json:"accepted_hit_boundary"`
}

type developmentScenarioProfile struct {
	seed     int64
	theatre  gamestate.Theatre
	mission  gamestate.MissionType
	rank     gamestate.Rank
	aircraft int
	cargo    bool
	kind     int
}

// DevelopmentScenarioNames is the deliberately small named fixture domain. The
// normal application dispatcher never accepts these names or calls this setup.
var DevelopmentScenarioNames = append([]string{"o-clean-flight", "o-clean-campaign", "o-short-flight", "o-short-campaign", "o-combat", "o-cargo", "o-terminal", "pickup", "rescue", "delivery", "recon", "pickup-return", "farp", "return", "damage", "crash", "power-loss", "damaged-return", "unsafe-contact", "flight-return", "crew-choice", "outcome-rescue", "outcome-mia", "campaign-return", "campaign-win"}, append(N5DevelopmentScenarioNames, N6DevelopmentScenarioNames...)...)

func developmentDamageScenario(name string) bool {
	name = developmentScenarioBase(name)
	return name == "damage" || name == "crash" || name == "power-loss" || name == "damaged-return"
}

func developmentProfile(name string) (developmentScenarioProfile, bool) {
	if base, ok := oFilmScenarioBase[name]; ok {
		name = base
	}
	if base, ok := n6ScenarioBase[name]; ok {
		name = base
	}
	plan, n5 := n5DevelopmentPlan(name)
	firstCommission := p3FirstCommissionScenario(name)
	name = developmentScenarioBase(name)
	p := developmentScenarioProfile{1, gamestate.TheatrePersianGulf, gamestate.MissionFlight, gamestate.RankCaptain, 5, false, 2}
	switch name {
	case "pickup", "pickup-return", "crew-choice":
		p.seed = 38
	case "rescue":
		p.seed = 5
	case "delivery":
		p.cargo, p.kind = true, 1
	case "recon":
		p.seed, p.aircraft, p.kind = 10, 4, 4
	case "farp":
		// Source-generated Europe/Brigadier General/Flight seed10 retains the
		// real three-row FARP and selects None Support. Seed1 selects active Air
		// Support, whose full flight controller remains an explicit launch guard.
		// This fixture changes only its declared seed; it never erases Support.
		p.seed = 10
		p.theatre, p.rank, p.aircraft, p.kind = gamestate.TheatreCentralEurope, gamestate.RankBrigadierGeneral, 0, -1
	case "return", "damage", "crash", "power-loss", "damaged-return", "unsafe-contact", "outcome-rescue", "outcome-mia":
		p.mission, p.aircraft, p.kind = gamestate.MissionSingle, 0, -1
	case "flight-return":
		p.kind = -1
	case "campaign-return", "campaign-win":
		p.kind, p.mission = -1, gamestate.MissionCampaign
	default:
		return developmentScenarioProfile{}, false
	}
	if n5 {
		p.seed, p.aircraft, p.kind = plan.seed, plan.aircraft, plan.kind
	}
	if firstCommission {
		p.rank = gamestate.RankWO1
	}
	return p, true
}

// ConfigureDevelopmentScenario reuses actual generation, ORDNANCE acceptance,
// settings/APU and initialization controllers. Only labelled approach/history
// adjustments are supplied; completion and service run after ordinary resume.
func (c *FrontEnd) ConfigureDevelopmentScenario(name string) (*DevelopmentScenario, error) {
	requestedName := name
	_, cleanFilm := oFilmScenarioBase[name]
	if base, ok := oFilmScenarioBase[name]; ok {
		name = base
	}
	if base, ok := n6ScenarioBase[name]; ok {
		name = base
	}
	plan, n5 := n5DevelopmentPlan(name)
	baseName := developmentScenarioBase(name)
	profile, ok := developmentProfile(name)
	if !ok {
		return nil, fmt.Errorf("unknown development scenario %q; choose %v", name, DevelopmentScenarioNames)
	}
	if c == nil || c.Office == nil || c.Office.Assets == nil || c.Store == nil || c.Mission != nil || c.Rendering != nil {
		return nil, fmt.Errorf("development scenario needs a fresh scratch front end")
	}
	next := c.Store.State()
	pilot := next.SelectedPilot()
	if pilot == nil {
		return nil, fmt.Errorf("development scenario has no disposable pilot")
	}
	pilot.Name, pilot.Rank, pilot.MissionType, pilot.Theatre = "Test Pilot Name", profile.rank, profile.mission, profile.theatre
	if profile.mission == gamestate.MissionFlight || profile.mission == gamestate.MissionCampaign || n5 && !p3FirstCommissionScenario(name) {
		var err error
		pilot.SquadronPilots, err = gamestate.InitializeSquadron(pilot, next.Slots[:], c.Office.Assets.Missions.Preflight.SquadronNames, rand.New(rand.NewSource(42)).Intn)
		if err != nil {
			return nil, err
		}
	}
	if n5 {
		history := n5EstablishedHistory
		if p3FirstCommissionScenario(name) {
			history = p3FirstCommissionHistory
		}
		if err := history(pilot); err != nil {
			return nil, err
		}
		if name == "inactive-phoenix" {
			other, err := gamestate.NewPilot()
			if err != nil {
				return nil, err
			}
			other.Theatre, other.MissionType = profile.theatre, gamestate.MissionSingle
			if err = n5EstablishedHistory(other); err != nil {
				return nil, err
			}
			other.Name = "Reed"
			next.Slots[1] = other
		}
	}
	if err := c.Store.Commit(next); err != nil {
		return nil, err
	}
	if err := c.RefreshOffice(); err != nil {
		return nil, err
	}
	seed := profile.seed
	c.MissionSeed = &seed
	oldFade := c.FadeMilliseconds
	c.FadeMilliseconds = 0 // Setup acknowledges source pages without host-time waits.
	defer func() { c.FadeMilliseconds = oldFade }()
	if err := c.enterMission(); err != nil {
		return nil, err
	}
	if baseName == "campaign-win" && c.Mission.CampaignDraft != nil {
		// Controlled history consumer fixture; no result/objective/award is supplied.
		c.Mission.CampaignDraft.TargetCounter = 1
		c.Mission.CampaignDraft.ResultCounters = [2]int{2, 0}
	}
	if n5 && c.Mission.CampaignDraft != nil && (name == "campaign-victory" || name == "campaign-defeat") {
		c.Mission.CampaignDraft.TargetCounter = 1
		c.Mission.CampaignDraft.ResultCounters = [2]int{0, 0}
	}
	for i := 0; c.Screen == ScreenInterstitial && i < 100; i++ {
		if err := c.Tick(); err != nil {
			return nil, err
		}
	}
	if c.Screen != ScreenBriefing || c.Mission == nil {
		return nil, fmt.Errorf("scenario preparation stayed on %s: %s", c.Screen, c.Message)
	}
	if name == "mia-objective-recovery" {
		other, err := gamestate.NewPilot()
		if err != nil {
			return nil, err
		}
		other.Theatre, other.MissionType = profile.theatre, gamestate.MissionSingle
		if err = n5EstablishedHistory(other); err != nil {
			return nil, err
		}
		other.Name, other.Status, other.LegacyStatus = "Jordan", gamestate.StatusMIA, 3
		other.LegacyRaw[0x56] = 3
		other.LegacyRaw[0x5b] = byte(c.Mission.Data.SourceWorld.Suffix)
		next := c.Store.State()
		next.Slots[1] = other
		if err = c.Store.Commit(next); err != nil {
			return nil, err
		}
	}
	choose := func(action string, value int) error {
		for row, option := range c.Mission.Ordnance.Choices {
			if option.Enabled && option.Action == action && (value < 0 || option.Value == value) {
				return c.developmentScenarioClick(row)
			}
		}
		return fmt.Errorf("scenario ORDNANCE lacks admitted %s/%d", action, value)
	}
	for slot := range c.Mission.ReadinessRows {
		if err := c.openOrdnance(slot); err != nil {
			return nil, err
		}
		for _, action := range []struct {
			name string
			id   int
		}{{"aircraft", -1}, {"replace", profile.aircraft}, {"back", -1}} {
			if err := choose(action.name, action.id); err != nil {
				return nil, err
			}
		}
		if slot == 0 && ordnanceAcceptance(c.Mission.Ordnance.Working[:], 0) == "HELICOPTER IS UNARMED.  Add weapons." {
			for _, action := range []string{"fits", "mount", "back"} {
				if err := choose(action, -1); err != nil {
					return nil, err
				}
			}
		}
		if slot == 0 && n5 && plan.goals > 0 {
			// RawWSHD4 is the verified normal Hellfire definition, not a
			// compact projectile ID. Fit it through the genuine ORDNANCE
			// station/quantity/capacity transaction; replacement alone can
			// legitimately leave the Apache with only its integral cannon.
			found := false
			for row, option := range c.Mission.Ordnance.Choices {
				if option.Enabled && option.Action == "fits" && option.System == 4 {
					if err := c.developmentScenarioClick(row); err != nil {
						return nil, err
					}
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("N.5 combat scenario requires an admitted rawWSHD4 Hellfire fit")
			}
			mount, quantity := -1, -1
			for row, option := range c.Mission.Ordnance.Choices {
				if option.Enabled && option.Action == "mount" && option.System == 4 {
					fit := c.Mission.Ordnance.ship().Fits[option.Value]
					if fit.Quantity > quantity {
						mount, quantity = row, fit.Quantity
					}
				}
			}
			if mount < 0 {
				return nil, fmt.Errorf("N.5 Hellfire has no mountable source fit")
			}
			if err := c.developmentScenarioClick(mount); err != nil {
				return nil, err
			}
			if err := choose("back", -1); err != nil {
				return nil, err
			}
		}
		if slot == 0 && profile.cargo {
			found := false
			for row, option := range c.Mission.Ordnance.Choices {
				w := c.Office.Assets.Missions.WeaponDefinitions[option.System]
				if option.Enabled && option.Action == "mount" && len(w) == 214 && gamestate.RowWord(w, 4)&0x4100 == 0x4100 {
					if err := c.developmentScenarioClick(row); err != nil {
						return nil, err
					}
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("scenario requires actual CARGO mount")
			}
		}
		if err := choose("back", -1); err != nil {
			return nil, err
		}
	}
	if c.Screen == ScreenOutfit {
		if err := c.developmentScenarioClick(6); err != nil {
			return nil, err
		}
	}
	if c.Screen != ScreenBriefing {
		return nil, fmt.Errorf("scenario loadout not accepted: %s %s", c.Screen, c.Message)
	}
	if n5 && !p3FirstCommissionScenario(name) || developmentDamageScenario(name) {
		// Explicit supplied preference: q2 avoids the source's initial q0/q1
		// suppression draw. Real hit selection still owns its shared RNG stream.
		c.Mission.Settings.EnemyQuality = 2
		c.Mission.SettingsChanged = true
	}
	if name == "fuel-empty" {
		if requestedName == "n6-safe-unpowered" || requestedName == "n6-unsafe-contact" {
			c.Mission.Settings.Landings = 1
			c.Mission.SettingsChanged = true
		}
		// REALITY ordinal1 is REALISTIC. Only this dev approach opts in;
		// the actual source model and all normal saved settings are unchanged.
		c.Mission.Settings.FlightModel = 1
	}
	for _, key := range []int{3, 9, 9} {
		if err := c.developmentScenarioClick(key); err != nil {
			return nil, err
		}
	}
	for i := 0; c.Screen != ScreenScene && i < 300; i++ {
		if err := c.Tick(); err != nil {
			return nil, err
		}
		c.MarkScenePresented()
		if err := c.AdvanceSceneInitialization(1.0 / 60); err != nil {
			return nil, err
		}
	}
	if c.Screen != ScreenScene || c.Rendering == nil || c.Rendering.ResultTargets == nil {
		return nil, fmt.Errorf("scenario launch lacks initialized TARGET state: %s %s", c.Screen, c.Message)
	}
	p := c.Rendering
	if profile.kind >= 0 && int(binary.LittleEndian.Uint16(p.ResultTargets.Records[2][32:])) != profile.kind {
		return nil, fmt.Errorf("scenario seed%d no longer has mapped objective%d", profile.seed, profile.kind)
	}
	meta := &DevelopmentScenario{Name: name, ScratchSave: c.Store.Directory, Stage: "approach", Seed: profile.seed, MissionType: profile.mission, Theatre: profile.theatre,
		World: c.Mission.Data.World, Scenario: c.Mission.Data.Scenario, ObjectSHA256: c.Mission.Data.ObjectHash,
		AcceptedRows: append([][36]byte(nil), c.Mission.Final.Aircraft...), LaunchSwitches: p.Initialized.Switches, InitialRNG: p.Initialized.RandomState,
		Paused: true, ResultBoundary: c.Mission.ResultUnavailable,
		Adjustments:   []string{"Disposable supplied pilot rank/history; genuine source mission generation and accepted ORDNANCE", "Supplied approach pose/motion; no objective, service, damage or result was pre-completed", "Paused before the first mission logical update; ordinary Escape resumes"},
		PrimaryTARGET: p.ResultTargets.Records[2], BaseTARGET: p.ResultTargets.Records[0], FARPTARGET: p.ResultTargets.Records[1]}
	for _, row := range c.Mission.Final.Aircraft {
		meta.AcceptedAircraft = append(meta.AcceptedAircraft, int(row[0]))
	}
	if c.Mission.ResultLaunch != nil {
		meta.ResultBoundary = "Source finalizer/normalizer and bounded player/Flight/Campaign results; final acknowledgement commits once"
	}
	c.DevelopmentScenario = meta
	if cleanFilm {
		// This variant supplies an established profile/seed through preparation only.
		// Keep the real initialized flight unchanged: a clean-launch film must not
		// hide later fixture teleports or accept a diagnostic snapshot as its launch.
		meta.Name, meta.Stage = requestedName, "film-clean"
		meta.Adjustments = []string{"Supplied established Captain career and deterministic generation seed; genuine preparation/ORDNANCE; unchanged initialized flight"}
		meta.Instructions = "DEVELOPMENT " + requestedName + ": clean launch, paused. Escape resumes; ordinary controls/firing. Alt+E then Y ends; acknowledge debrief to Mission Films. PLAY/Resume cannot commit."
		if e := c.configureOFilmScenario(requestedName, meta); e != nil {
			return nil, e
		}
		meta.Pose = p.Flight.State.Pose
		meta.Rotor = p.Flight.State.Rotor
		p.Paused = true
		p.Flight.Paused = true
		c.Message = meta.Instructions
		return meta, nil
	}
	point := meta.PrimaryTARGET
	heightAboveSupport := int16(2)
	rotor, stage := int16(0), int16(128)
	switch baseName {
	case "pickup", "rescue", "pickup-return", "delivery", "crew-choice":
		meta.Instructions = "DEVELOPMENT " + name + ": paused at genuine query approach; Escape resumes production objective logic; F10 inspects cargo/status"
	case "recon":
		heightAboveSupport, rotor = 300, 1
		meta.Instructions = "DEVELOPMENT recon: paused inside genuine recon query radius; Escape resumes; F10 inspects P status"
	case "farp":
		point, heightAboveSupport, rotor, stage = meta.FARPTARGET, 2, 1, 0
		meta.Instructions = "DEVELOPMENT FARP: Escape resumes contact; 9 stops rotor; expect refill, not CALL IT A DAY"
	case "return", "damaged-return", "flight-return", "campaign-return", "campaign-win":
		point, heightAboveSupport, rotor, stage = meta.BaseTARGET, 2, 1, 0
		meta.Instructions = "DEVELOPMENT return: Escape resumes contact; 9 stops rotor; answer natural CALL IT A DAY normally"
	case "damage", "crash", "power-loss":
		point, heightAboveSupport, rotor = meta.BaseTARGET, 400, 1
		meta.DamagePlan = baseName
		meta.Adjustments = append(meta.Adjustments, "EnemyQuality2 supplied through accepted preflight; named accepted-hit plan queued for real logical damage entry")
		meta.Instructions = "DEVELOPMENT " + name + ": Escape resumes queued accepted hit(s); no autonomous attack or career loss classification"
	}
	if baseName == "damaged-return" {
		meta.DamagePlan = "damage"
	}
	if baseName == "campaign-win" {
		meta.Adjustments = append(meta.Adjustments, "Controlled launch Campaign history: TargetCounter1, successes2/failures0; source conclusion is tested, not a naturally earned win")
	}
	if baseName == "outcome-rescue" || baseName == "outcome-mia" {
		// Pick an existing far world row, not a manufactured enemy or result flag.
		baseH0, baseH1 := binary.LittleEndian.Uint32(meta.BaseTARGET[42:]), binary.LittleEndian.Uint32(meta.BaseTARGET[46:])
		farthest := uint64(0)
		for _, e := range p.Initialized.Entities[:80] {
			if e.Kind&0x30 != 0 || e.Kind&0x40 != 0 {
				continue
			}
			dx, dy := int64(e.Pose.H0)-int64(baseH0), int64(e.Pose.H1)-int64(baseH1)
			if dx < 0 {
				dx = -dx
			}
			if dy < 0 {
				dy = -dy
			}
			d := uint64(dx + dy)
			if d > farthest {
				farthest = d
				binary.LittleEndian.PutUint32(point[34:], e.Pose.H0)
				binary.LittleEndian.PutUint32(point[38:], e.Pose.H1)
			}
		}
		heightAboveSupport, rotor, stage = 400, 1, 128
		seed := uint32(1)
		if baseName == "outcome-mia" {
			seed = 9923
		}
		if c.Mission.ResultLaunch != nil {
			c.Mission.ResultLaunch.DevelopmentSeed = &seed
		}
		meta.Adjustments = append(meta.Adjustments, fmt.Sprintf("Supplied front-end result CRT seed%d; far existing-object approach; Alt+E uses real proximity/normalizer", seed))
		meta.Instructions = "DEVELOPMENT " + name + ": paused far approach; Escape then Alt+E,Y ends through real normalization; final acknowledgement alone commits"
	}
	if baseName == "unsafe-contact" {
		point = meta.BaseTARGET
		heightAboveSupport, rotor, stage = 2, 1, 0
		meta.Instructions = "DEVELOPMENT unsafe contact: Escape resumes supplied unsafe forward speed; actual contact/terminal/normalizer decides outcome"
	}
	reference := name == "farp" || baseName == "return" || baseName == "damaged-return" || baseName == "flight-return" || baseName == "campaign-return" || baseName == "campaign-win" || baseName == "unsafe-contact" || developmentDamageScenario(name)
	if err := c.developmentScenarioPose(point, reference, heightAboveSupport, rotor, stage); err != nil {
		return nil, err
	}
	if (profile.kind == 1 || profile.kind == 2) && meta.Pose.Height >= 16 {
		return nil, fmt.Errorf("scenario query support%d cannot satisfy original signed world-height<16 without placing the aircraft through terrain", meta.SupportHeight)
	}
	if baseName == "farp" {
		r := p.Flight
		r.Systems.Fuel[0] = 9
		r.Defenses.Chaff, r.Defenses.Flares = 0, 0
		for i := range p.Initialized.Stores.Ships[0].Stations {
			p.Initialized.Stores.Ships[0].Stations[i].Ammunition = 0
		}
		meta.Adjustments = append(meta.Adjustments, "Current fuel9/empty ammunition/zero countermeasure counts supplied; accepted refill blueprint unchanged")
		p.publishFlight()
		p.resetPresentation()
	}
	if baseName == "unsafe-contact" {
		p.Flight.State.Forward = 400
		p.publishFlight()
		p.resetPresentation()
		meta.Adjustments = append(meta.Adjustments, "Supplied forward400 at low approach; no terminal/status bit prefilled")
	}
	if developmentDamageScenario(name) {
		damagePlan := baseName
		if baseName == "damaged-return" {
			damagePlan = "damage"
		}
		if err := p.queueDevelopmentDamageScenario(damagePlan); err != nil {
			return nil, err
		}
		for _, hit := range p.Flight.acceptedPlayerHits {
			if hit.developmentSeed != nil {
				meta.DamageInputs = append(meta.DamageInputs, DevelopmentHitInput{hit.VictimRow, hit.ProjectileFlags, *hit.developmentSeed, hit.Source})
			}
		}
	}
	if n5 {
		if err := c.configureN5Scenario(name, plan); err != nil {
			return nil, err
		}
	}
	if _, ok := n6ScenarioBase[requestedName]; ok {
		if err := c.configureN6Scenario(requestedName); err != nil {
			return nil, err
		}
	}
	if stagedCampaignDevelopment(meta.Name) && p.filmRecorder != nil {
		p.filmRecorder.Error = "Supplied multi-stage development approaches are outside clean-launch film recording; prior films and career behavior are preserved."
		meta.Instructions += " Film recording is unavailable for these supplied stages; Film Library EXIT remains available."
	}
	c.Message = meta.Instructions
	fmt.Println(meta.Instructions)
	return meta, nil
}

// These old multi-stage controls supply teleports, not a representable clean
// film stream. They remain useful result/presentation fixtures with explicit
// recording failure; ordinary missions and named clean films do not use them.
func stagedCampaignDevelopment(name string) bool {
	switch name {
	case "campaign-victory", "campaign-ongoing-success", "campaign-ongoing-failure", "campaign-defeat":
		return true
	}
	return false
}

func (c *FrontEnd) developmentScenarioClick(key int) error {
	x, y, ok := missionActionPoint(c, key)
	if !ok {
		return fmt.Errorf("development source action%d unavailable on%s", key, c.Screen)
	}
	if err := c.Handle(Actions{X: x, Y: y, Inside: true, Selector1: true, PointerPressed: true}); err != nil {
		return err
	}
	for i := 0; c.Screen == ScreenPaperTransition && i < 100; i++ {
		if err := c.Tick(); err != nil {
			return err
		}
	}
	return nil
}

func (c *FrontEnd) developmentScenarioPose(record [62]byte, reference bool, above, rotor, stage int16) error {
	p, meta := c.Rendering, c.DevelopmentScenario
	r, s := p.Flight, p.Flight.State
	offset := 34
	if reference {
		offset = 42
	}
	pose := flight.Pose{H0: binary.LittleEndian.Uint32(record[offset:]), H1: binary.LittleEndian.Uint32(record[offset+4:])}
	highest := int16(-32768)
	// Source contact samples four48-unit foot points at the supplied yaw0.
	for _, d := range [][2]int32{{0, 48}, {48, 0}, {0, -48}, {-48, 0}} {
		h, err := p.Renderer.Assets.ContactHeight(p.Initialized.World, world.QueryPose{H0: pose.H0 + uint32(d[0]), H1: pose.H1 + uint32(d[1]), Height: 32767}, p.Initialized.ContactObjects())
		if err != nil {
			return err
		}
		if h > highest {
			highest = h
		}
	}
	pose.Height = highest + above
	s.Pose = pose
	s.Forward, s.Vertical, s.Lateral, s.IndicatedSpeed = 0, 0, 0, 0
	s.History0, s.History1 = [4]int16{}, [4]int16{}
	s.Hover, s.HeightControl, s.CruiseSpeed, s.Collective = 0, 0, 0, 0
	s.Power, s.Thrust, s.HoverDelay, s.Shake = 0, 0, 0, 0
	s.Rotor, s.FlightStage, s.HeightTarget = rotor, stage, pose.Height
	if above > 32 {
		s.Collective = 16
	}
	s.ContactHeight, s.RadarAltitude = highest, above-2
	s.ValidContact = false // Only production's first contact query may accept it.
	s.BoundaryCrossed, s.BoundaryH0BeforeClamp = 0, pose.H0
	cell, err := p.Renderer.Assets.SourceCellID(p.Initialized.World, world.QueryPose{H0: pose.H0, H1: pose.H1, Height: pose.Height})
	if err != nil {
		return err
	}
	s.CellCache = int16(cell)
	p.publishFlight()
	if meta != nil && stagedCampaignDevelopment(meta.Name) {
		// These named, multi-stage development fixtures supply approaches rather
		// than continuous flight. Keep healthy following poses coherent through
		// the source P0 constructor; do not reset failures, fuel, RNG or results.
		count, err := r.Combat.StageDevelopmentFormation()
		if err != nil {
			return err
		}
		meta.Adjustments = append(meta.Adjustments, fmt.Sprintf("Supplied player approach also source-P0 initializes %d eligible healthy following actors; no RNG, fuel, damage, projectile, objective or result adjustment", count))
	}
	p.ViewControls = render3d.NewCameraController(p.cameraInputs())
	if err := p.ViewControls.Tick(p.cameraInputs(), p.Renderer.Assets); err != nil {
		return err
	}
	p.publishView()
	p.Paused, r.Paused = true, true
	p.clearFlightInput()
	p.resetPresentation()
	meta.Pose, meta.SupportHeight = pose, highest
	meta.Rotor, meta.Collective, meta.FlightStage = rotor, s.Collective, stage
	if rotor != 0 {
		// Fast fixtures acknowledge loader pages offline; this existing logical
		// sound command supplies running feedback, not a replay of startup cues.
		p.requestCollectiveRotor()
	}
	return nil
}

// Only the explicit pickup-return development scenario installs this observer.
// The first stage must be completed by real objective logic after ordinary
// resume. Its cargo/assignment/event state then survives a labelled return pose.
func (c *FrontEnd) advanceDevelopmentScenario() error {
	if err := c.advanceN5Scenario(); err != nil {
		return err
	}
	meta := c.DevelopmentScenario
	if meta == nil || (developmentScenarioBase(meta.Name) != "pickup-return" && developmentScenarioBase(meta.Name) != "crew-choice") || meta.Stage != "approach" || c.Rendering == nil || c.Screen != ScreenScene || c.Rendering.GPS != nil || c.Rendering.Paused {
		return nil
	}
	p := c.Rendering
	if p.Initialized.ObjectiveAssignments[0] != uint16(p.Initialized.PlayerRow+1) || p.Initialized.Stores.Ships[p.Initialized.PlayerRow].Flags&0xf8 != 0x18 || binary.LittleEndian.Uint16(p.ResultTargets.Records[2][32:]) != 0 {
		return nil
	}
	meta.Stage = "loaded-return"
	meta.Adjustments = append(meta.Adjustments, "After actual pickup completion, development observer supplied base return pose and paused; cargo and assignment retained")
	meta.Instructions = "DEVELOPMENT pickup-return: real pickup succeeded; now paused at base with cargo retained. Escape resumes contact, 9 stops rotor; answer natural ending"
	if err := c.developmentScenarioPose(p.ResultTargets.Records[0], true, 2, 1, 0); err != nil {
		return err
	}
	c.Message = meta.Instructions
	fmt.Println(meta.Instructions)
	return nil
}
