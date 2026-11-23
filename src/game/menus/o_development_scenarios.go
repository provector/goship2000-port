package menus

import (
	"encoding/binary"
	"fmt"
)

// Only the dev command reaches these names. Every film setup is a typed,
// first-step initial condition, never a completed objective or result snapshot.
var oFilmScenarioBase = map[string]string{
	"o-clean-flight": "established-failure", "o-clean-campaign": "campaign-ongoing-failure",
	"o-short-flight": "established-failure", "o-short-campaign": "campaign-ongoing-failure",
	"o-combat": "established-primary", "o-cargo": "crew-choice-250", "o-terminal": "established-failure",
}

func (c *FrontEnd) configureOFilmScenario(name string, meta *DevelopmentScenario) error {
	if name == "o-clean-flight" || name == "o-clean-campaign" {
		return nil
	}
	p := c.Rendering
	point := p.ResultTargets.Records[0]
	x, z := binary.LittleEndian.Uint32(point[42:]), binary.LittleEndian.Uint32(point[46:])
	a := FilmInitialAdjustment{Label: "Step O " + name + ": supplied coherent initial approach only"}
	approach := &FilmApproach{H0: x, H1: z, AboveSupport: 2, Rotor: 1, Stage: 0}
	switch name {
	case "o-combat":
		row := -1
		for i, e := range p.Initialized.Entities {
			if i < 80 && e.Object.Raw[0]&0x10 != 0 && e.Kind&0x38 == 0 && e.Participation == 1 && e.Object.Raw[10] < 16 {
				row = i
				break
			}
		}
		if row < 0 {
			return fmt.Errorf("Step O genuine generated objective lacks supported target")
		}
		e := p.Initialized.Entities[row]
		approach = &FilmApproach{H0: e.Pose.H0, H1: e.Pose.H1 - 1500, AboveSupport: 128, Rotor: 1, Stage: 128, Hover: 1, HeightControl: 1}
		meta.TargetRows = []int{row}
		meta.Instructions = "DEVELOPMENT o-combat: paused south of a real objective. Escape; Backspace selects/cycles contacts; Space selects Hellfire, Enter fires. Actual hits/destruction/credit are recorded. Alt+E/Y ends; debrief then PLAY. No automatic teleport/return."
	case "o-cargo":
		point = p.ResultTargets.Records[2]
		approach.H0, approach.H1 = binary.LittleEndian.Uint32(point[34:]), binary.LittleEndian.Uint32(point[38:])
		approach.Stage = 128
		meta.Instructions = "DEVELOPMENT o-cargo: paused at genuine pickup approach. Escape allows actual pickup; F10 inspects cargo, C drops in the GPS player detail panel. Alt+E/Y ends without promising safe recovery. Replay Next uses genuine pickup/objective events."
	case "o-terminal":
		approach.AboveSupport, approach.Stage = 400, 128
		meta.Instructions = "DEVELOPMENT o-terminal: paused; two explicitly supplied accepted fuel-system hits will run production damage after Escape. Inspect real terminal delivery and ending, then debrief/PLAY. This is not ordinary fuel exhaustion."
	default:
		meta.Instructions = "DEVELOPMENT " + name + ": supplied base approach, paused. Escape resumes actual contact; 9 stops rotor; Y answers natural end. Debrief -> Mission Films. PLAY/Resume return here; EXIT alone advances the Campaign successor once."
	}
	a.Approach = approach
	if e := c.ApplyFilmInitialAdjustment(a); e != nil {
		return e
	}
	if name == "o-terminal" {
		if e := p.queueDevelopmentDamageScenario("crash"); e != nil {
			return e
		}
		meta.DamagePlan = "crash"
	}
	meta.Adjustments = append(meta.Adjustments, a.Label)
	return nil
}
