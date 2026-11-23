package menus

import (
	"fmt"
	gamestate "gunship2000-assets/game/state"
	"strings"
)

// A244ff reads this bounded table through DS1a94 -> DS0c26 in the supported
// release. These are display names only, not posting or progression predicates.
var rosterStatusLabels = [...]string{"Replacement", "Active", "Retired", "MIA", "KIA"}

func rosterStatusLabel(p *gamestate.Pilot) string {
	if p == nil || p.Status == gamestate.StatusInactive {
		return rosterStatusLabels[0]
	}
	if p.Status == gamestate.StatusActive {
		return rosterStatusLabels[1]
	}
	if p.Status == gamestate.StatusRetired {
		return rosterStatusLabels[2]
	}
	if p.Status == gamestate.StatusMIA {
		return rosterStatusLabels[3]
	}
	if p.Status == gamestate.StatusKIA {
		return rosterStatusLabels[4]
	}
	if p.Status == gamestate.StatusLegacyUnknown && int(p.LegacyStatus) < len(rosterStatusLabels) {
		return rosterStatusLabels[p.LegacyStatus]
	}
	return "Unknown"
}

func (c *FrontEnd) rosterCampaignLine(p *gamestate.Pilot) string {
	stage, err := gamestate.CampaignStage(p)
	// Typed Campaign state supersedes the import witness, as it already does for
	// mission eligibility and theatre locking. Unknown history is not made active.
	if err == nil && stage != 0 && p.Status == gamestate.StatusActive {
		return strings.TrimPrefix(c.Menus.Strings["campaign_underway"], "\n")
	}
	if p.Statistics != nil && int16(p.Statistics.Campaigns()) != 0 {
		n := p.Statistics.Campaigns()
		suffix := "s"
		if int16(n) == 1 {
			suffix = ""
		}
		format := strings.TrimPrefix(c.Menus.Strings["campaigns_format"], "\n")
		return fmt.Sprintf(format, int16(n), suffix)
	}
	return ""
}
