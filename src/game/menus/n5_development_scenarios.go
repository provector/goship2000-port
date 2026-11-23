package menus

import (
	"encoding/binary"
	"fmt"

	gamestate "gunship2000-assets/game/state"
)

// These names are accepted only by the existing dev dispatcher. They supply
// initial history/approaches, never objective completion, final facts or awards.
var N5DevelopmentScenarioNames = []string{
	"established-failure", "established-primary", "crew-choice-250",
	"fuel-empty", "critical-fuel-hit", "inactive-phoenix",
	"campaign-ongoing-success", "campaign-ongoing-failure",
	"campaign-victory", "campaign-defeat", "campaign-mixed-win-failure",
	"mia-objective-recovery",
	"p3-first-commission",
	"p3-first-commission-safe-approach",
}

type n5ScenarioPlan struct {
	base     string
	seed     int64
	aircraft int
	kind     int
	goals    int // 0: no staged goals; 1: primary; 2: both, each by real mechanics.
	expected string
}

func p3FirstCommissionScenario(name string) bool {
	return name == "p3-first-commission" || name == "p3-first-commission-safe-approach"
}

func n5DevelopmentPlan(name string) (n5ScenarioPlan, bool) {
	p := n5ScenarioPlan{base: "return", seed: 1, aircraft: 0, kind: -1}
	switch name {
	case "established-failure":
		p.expected = "Neither objective; safe return alone is failure; independently retained no-match gives score0, separate demerit+200 and GS1c"
	case "established-primary":
		p.seed, p.kind, p.goals = 2, 3, 1
		p.expected = "Actual HQ destruction completes primary; final kinds/credit, coefficient and elapsed determine score; ordinary success music unless source progression selects another cue"
	case "p3-first-commission", "p3-first-commission-safe-approach":
		p.seed, p.kind, p.goals = 2, 3, 1
		p.expected = "Supplied qualified WO1 career990; actual primary Hellfire destruction then Alt+E/Y earns commission. Accept reaches REPLACE/optional10-byte rename; refusal creates no squadron. No return/objective/result is preset."
	case "crew-choice-250":
		p.base, p.seed, p.aircraft, p.kind = "crew-choice", 38, 5, 2
		p.expected = "Actual pickup and qualified return: primary100+player50+four followers25=250 at EQ2/coefficient10/no-match; source recipient choice and stored crew copyback"
	case "mia-objective-recovery":
		p.base, p.seed, p.aircraft, p.kind = "pickup-return", 38, 5, 2
		p.expected = "Supplied unique matching MIA history; actual pickup plus assigned safe return produces special congratulations; final acknowledgement atomically recovers that stable pilot only"
	case "fuel-empty":
		p.expected = "Supplied fuel1/countdown0 burns to0 with609 unchanged; next oldempty burn produces power loss, source warning/rotor request; no unconditional crash/KIA"
	case "critical-fuel-hit", "inactive-phoenix":
		p.base = "crash"
		p.expected = "Two labelled accepted603 hits reach genuine critical-damage terminal/normalizer; final acknowledgement commits the actual outcome once; Phoenix remains independently owned"
	case "campaign-ongoing-success", "campaign-victory":
		p.base, p.seed, p.kind, p.goals = "campaign-return", 58, 3, 2
		p.expected = "Real primary+secondary destruction and return give source quality200; successful ongoing debrief reaches next genuine briefing"
		if name == "campaign-victory" {
			p.expected = "Controlled threshold1/history0: real both-objective success reaches WIN + success paragraph + GS1e, concludes once"
		}
	case "campaign-ongoing-failure", "campaign-defeat":
		p.base = "campaign-return"
		p.expected = "Neither objective: unsuccessful safe return, GS1c and next genuine briefing while Campaign remains active"
		if name == "campaign-defeat" {
			p.expected = "Controlled threshold1/history0: real unsuccessful return reaches LOSE + failure paragraph + GS1f, concludes once"
		}
	case "campaign-mixed-win-failure":
		p.base = "campaign-win"
		p.expected = "Labelled winning history plus failed current sortie: authentic WIN art, failure paragraph and GS1e"
	default:
		return n5ScenarioPlan{}, false
	}
	return p, true
}

func developmentScenarioBase(name string) string {
	if p, ok := n5DevelopmentPlan(name); ok {
		return p.base
	}
	return name
}

// The source offsets are supplied established history, synchronized with typed
// native owners. They are not generated achievements or a fake finished result.
func n5EstablishedHistory(p *gamestate.Pilot) error {
	if p == nil || len(p.LegacyRaw) != gamestate.LegacyRecordSize || p.Statistics == nil {
		return fmt.Errorf("established scenario requires a complete disposable pilot")
	}
	p.Name, p.Rank, p.Status, p.LegacyStatus = "Morgan", gamestate.RankCaptain, gamestate.StatusActive, 1
	p.LegacyRaw[0x54], p.LegacyRaw[0x55], p.LegacyRaw[0x56] = 7, 5, 1
	p.LegacyRaw[0x64] = byte(p.Theatre.Raw())
	p.Statistics.AwardCounts = []int8{0, 0, 0, 1, 1, 2, 1, 1}
	for i, count := range p.Statistics.AwardCounts {
		p.LegacyRaw[0x24+i] = byte(count)
	}
	clear(p.LegacyRaw[0x2c:0x32])
	binary.LittleEndian.PutUint16(p.LegacyRaw[0x59:], 300)
	p.Statistics.LastMissionScore, p.Statistics.BestMissionScore = 650, 2000
	p.Statistics.CareerScore, p.Statistics.MissionCampaignCounter, p.Statistics.PhoenixCounter = 7900, 12, 0
	for _, field := range []struct {
		offset int
		value  uint32
	}{{0x66, 2000}, {0x6a, 650}, {0x6e, 12}, {0x72, 7900}, {0x76, 5000}, {0x7a, 6500}, {0xcc, 0}} {
		binary.LittleEndian.PutUint32(p.LegacyRaw[field.offset:], field.value)
	}
	for i := range p.SquadronPilots {
		row := &p.SquadronPilots[i]
		if len(row.LegacyRaw) != 41 {
			return fmt.Errorf("established scenario crew%d lacks original41-byte storage", i+1)
		}
		ranks := [4]gamestate.Rank{gamestate.RankCW2, gamestate.RankWO1, gamestate.RankCW3, gamestate.RankWO1}
		row.Rank = ranks[i]
		row.LegacyRaw[34] = byte(row.Rank.Raw())
		clear(row.LegacyRaw[26:34])
		row.LegacyRaw[31], row.LegacyRaw[33] = 1, 1 // Existing ACV and NDS.
		copy(p.LegacyRaw[0x55+41*(i+1):0x55+41*(i+2)], row.LegacyRaw)
	}
	return nil
}

func p3FirstCommissionHistory(p *gamestate.Pilot) error {
	if p == nil || len(p.LegacyRaw) != gamestate.LegacyRecordSize || p.Statistics == nil {
		return fmt.Errorf("first commission fixture needs complete disposable history")
	}
	p.Name, p.Rank, p.Status, p.LegacyStatus = "P3 Commission", gamestate.RankWO1, gamestate.StatusActive, 1
	p.Preflight = &gamestate.PreflightSettings{CPGControls: 2, EnemyQuality: 3, Wind: 1, Visibility: 1}
	p.LegacyRaw[0x54], p.LegacyRaw[0x55], p.LegacyRaw[0x56] = 1, 5, 1
	p.Statistics.AwardCounts = []int8{0, 0, 0, 0, 0, 0, 0, 1}
	for i, n := range p.Statistics.AwardCounts {
		p.LegacyRaw[0x24+i] = byte(n)
	}
	clear(p.LegacyRaw[0x2c:0x32])
	binary.LittleEndian.PutUint16(p.LegacyRaw[0x59:], 0)
	p.Statistics.LastMissionScore, p.Statistics.BestMissionScore = 990, 990
	p.Statistics.CareerScore, p.Statistics.MissionCampaignCounter, p.Statistics.PhoenixCounter = 990, 1, 0
	for _, f := range []struct {
		at int
		n  uint32
	}{{0x66, 990}, {0x6a, 990}, {0x6e, 1}, {0x72, 990}, {0x76, 0}, {0x7a, 990}, {0xcc, 0}} {
		binary.LittleEndian.PutUint32(p.LegacyRaw[f.at:], f.n)
	}
	p.SquadronPilots = nil
	clear(p.LegacyRaw[0x55+41:])
	return nil
}

func (c *FrontEnd) configureN5Scenario(name string, plan n5ScenarioPlan) error {
	p, meta := c.Rendering, c.DevelopmentScenario
	r := p.Flight
	meta.Profile, meta.Expected = "Supplied established Captain:12 missions,career7900,qualification05,existing medals/crew", plan.expected
	meta.Adjustments = append(meta.Adjustments, "Established Captain history and four meaningful persisted crew records supplied before genuine preparation; no qualification award preset")
	if name == "fuel-empty" {
		if err := c.developmentScenarioPose(meta.BaseTARGET, true, 400, 1, 128); err != nil {
			return err
		}
		r.Systems.Fuel[0], r.Systems.Countdown = 1, 0
		r.State.Hover, r.State.HeightControl, r.State.HeightTarget = 1, 1, r.State.Pose.Height
		r.State.Power = r.State.Collective << 6
		r.State.Thrust = statWord(r.Parameters, int(r.State.Collective)*2)
		meta.Adjustments = append(meta.Adjustments, "REALISTIC FlightModel1 accepted through genuine preflight (other defaults, including No Crashes, retained); supplied ordinary fuel1/countdown0, coherent running power/thrust and source hover/altitude hold; no accepted damage event or terminal/outcome preset")
		meta.Instructions = "DEVELOPMENT fuel-empty: click/release then Escape. Watch0% without explosion, then next burn's power warning. Press9 for source unpowered descent; actual contact/normalizer decides outcome."
		p.publishFlight()
		p.resetPresentation()
	} else if plan.goals > 0 {
		if err := c.n5ObjectiveApproach(0); err != nil {
			return err
		}
		meta.Stage = "primary-combat"
	} else {
		meta.Instructions = "DEVELOPMENT " + name + ": " + meta.Instructions + ". " + plan.expected
	}
	if name == "crew-choice-250" {
		meta.Instructions = "DEVELOPMENT crew-choice-250: Escape performs real pickup, then pauses at loaded base approach. Escape,9,Y returns; use confined source mouse/keyboard to change CREWPRO recipient, then Accept. Expect250."
	}
	if name == "mia-objective-recovery" {
		meta.Adjustments = append(meta.Adjustments, "Other uniquely identified pilot Jordan has explicitly supplied prior MIA status and actual launch theatre/scenario; no crash coordinate or completed current result supplied")
		meta.Instructions = "DEVELOPMENT mia-objective-recovery: Escape performs real pickup; Escape at staged loaded base,9,Y returns. Inspect special congratulations, then acknowledge; Jordan becomes Active only at atomic commit. Native compatibility repair, not original natural revival proof."
	}
	if name == "inactive-phoenix" {
		meta.Instructions += " After committed loss, inspect other Active pilot Reed, then reselect Morgan's inactive record and Recover. Reopen the same scratch save."
	}
	if p3FirstCommissionScenario(name) {
		e := p.Initialized.Entities[meta.TargetRows[0]]
		distance, above := uint32(1500), int16(128)
		label := "P3 supplied qualified WO1 career990, primary hover approach; actual firing/objective/ending/commission"
		if name == "p3-first-commission-safe-approach" {
			distance, above = 8000, 1024
			label = "P3 qualified WO1 career990, primary hover8000 sourceunits south/1024 above support; actual firing/objective/ending/commission"
		}
		if err := c.ApplyFilmInitialAdjustment(FilmInitialAdjustment{Label: label, Approach: &FilmApproach{H0: e.Pose.H0, H1: e.Pose.H1 - distance, AboveSupport: above, Rotor: 1, Stage: 128, Hover: 1, HeightControl: 1}}); err != nil {
			return err
		}
		seed := uint32(1)
		c.Mission.ResultLaunch.DevelopmentSeed = &seed
		meta.Profile = "Supplied qualified WO1: prior career990, one prior sortie, NDS1, no squadron; source seed2 and real ORDNANCE"
		meta.Adjustments = []string{"Disposable qualified WO1 history: career990, one prior sortie, NDS1, no squadron; CPG2/EQ3/wind1/visibility1 supplied before genuine seed2 preparation", "Accepted Apache/Hellfire through real ORDNANCE; one actual actor", "Recorded initial hover1500 sourceunits south of the actual generated primary; no hits, objective credits, result or return flags preset", "Separate supplied result CRT seed1; shared simulation LFSR unchanged; no automatic return observer"}
		meta.Instructions = "DEVELOPMENT p3-first-commission: Escape resumes recorded hover near primary. Space selects Hellfire, Backspace designates, Enter fires. After real primary completes, Alt+E then Y; acknowledge commander (Enter accepts, Escape refuses), conditional rescue, JUMP and REPLACE. Backspace/right-click renames; Enter accepts. Final feedback commits once. No return teleport is installed."
		meta.Pose, meta.Rotor, meta.Collective, meta.FlightStage = p.Flight.State.Pose, p.Flight.State.Rotor, p.Flight.State.Collective, p.Flight.State.FlightStage
		meta.SupportHeight = p.Flight.State.ContactHeight
		if name == "p3-first-commission-safe-approach" {
			meta.Adjustments[2] = "Recorded initial hover8000 sourceunits south of actual generated primary,1024 above sampled support; no hits, goals, ending or stores preset"
			meta.Instructions = "DEVELOPMENT p3-first-commission-safe-approach: paused about1 source NAV km south and400ft above support. Escape resumes; select Hellfire with Space, designate the primary with Backspace and fire with Enter. Keep designation while the missile travels. Complete the real primary, then Alt+E/Y; accept or refuse commission. REPLACE rename is optional. No goal or return teleport is installed."
		}
	}
	return nil
}

func (c *FrontEnd) n5ObjectiveApproach(role int) error {
	p, meta := c.Rendering, c.DevelopmentScenario
	if role < 0 || role > 1 {
		return fmt.Errorf("development objective role%d", role)
	}
	flag := byte(0x10 << role)
	row := -1
	for i, e := range p.Initialized.Entities {
		if i < 80 && e.Object.Raw[0]&flag != 0 && e.Kind&0x38 == 0 && e.Participation == 1 && e.Object.Raw[10] < 16 {
			row = i
			break
		}
	}
	if row < 0 {
		return fmt.Errorf("development role%d has no live supported ground target", role)
	}
	e := p.Initialized.Entities[row]
	point := p.ResultTargets.Records[role+2] // Copy only; never mutate authoritative TARGET.
	binary.LittleEndian.PutUint32(point[34:], e.Pose.H0)
	binary.LittleEndian.PutUint32(point[38:], e.Pose.H1-1500)
	if err := c.developmentScenarioPose(point, false, 128, 1, 128); err != nil {
		return err
	}
	r := p.Flight
	r.State.Hover, r.State.HeightControl, r.State.HeightTarget = 1, 1, r.State.Pose.Height
	r.State.Power = r.State.Collective << 6
	r.State.Thrust = statWord(r.Parameters, int(r.State.Collective)*2)
	p.publishFlight()
	p.resetPresentation()
	meta.GoalRole = role
	meta.TargetRows = nil
	for i, q := range p.Initialized.Entities {
		if i < 80 && q.Object.Raw[0]&flag != 0 && q.Participation == 1 && q.Kind&0x38 == 0 {
			meta.TargetRows = append(meta.TargetRows, i)
		}
	}
	meta.Adjustments = append(meta.Adjustments, fmt.Sprintf("Supplied coherent hover1500 source coordinate units south of real objective%d row%d; real stores, candidate scan, firing, impacts and counts remain authoritative", role+1, row))
	meta.Instructions = fmt.Sprintf("DEVELOPMENT %s: paused near objective%d (%s), source rows%v. Escape; Backspace selects/cycles real contacts; Space cycles weapon, Enter fires. Destroy the required objective targets; only actual completion stages the next paused approach.", meta.Name, role+1, c.Mission.Data.Objectives[[2]string{"primary", "secondary"}[role]].Name, meta.TargetRows)
	return nil
}

func (c *FrontEnd) advanceN5Scenario() error {
	meta := c.DevelopmentScenario
	if meta == nil {
		return nil
	}
	plan, ok := n5DevelopmentPlan(meta.Name)
	if !ok || plan.goals == 0 || p3FirstCommissionScenario(meta.Name) || c.Screen != ScreenScene || c.Rendering == nil || c.Rendering.GPS != nil || c.Rendering.Paused {
		return nil
	}
	p := c.Rendering
	if meta.Stage != "primary-combat" && meta.Stage != "secondary-combat" {
		return nil
	}
	role := meta.GoalRole
	if binary.LittleEndian.Uint16(p.ResultTargets.Records[role+2][32:]) != 0 || ![2]bool{p.Flight.Combat.Primary.Achieved, p.Flight.Combat.Secondary.Achieved}[role] {
		return nil
	}
	if role == 0 && plan.goals == 2 {
		if err := c.n5ObjectiveApproach(1); err != nil {
			return err
		}
		meta.Stage = "secondary-combat"
	} else {
		if err := c.developmentScenarioPose(meta.BaseTARGET, true, 2, 1, 0); err != nil {
			return err
		}
		meta.Stage = "combat-return"
		meta.Adjustments = append(meta.Adjustments, "Only after real objective completion, observer supplied paused base approach; credits, ammo, projectiles and objective state retained")
		meta.Instructions = "DEVELOPMENT " + meta.Name + ": real objective action completed. Paused at supplied base approach; Escape,9,Y uses production contact/rotor/return. Final acknowledgement alone commits."
	}
	c.Message = meta.Instructions
	fmt.Println(meta.Instructions)
	return nil
}
