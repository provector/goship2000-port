package menus

import gamestate "gunship2000-assets/game/state"

// GS paper P038b..0416 sign-extends each retained byte before its word
// arithmetic. This categorical paper is independent of the Campaign wall GRAF.
func campaignPaperChoice(c gamestate.CampaignGenerationInputs) int {
	target, successes, failures := int(int8(c.Raw50)), int(int8(c.Raw51)), int(int8(c.Raw52))
	if successes+failures == 0 {
		return -2 // introductory paragraph wins even at the desperate boundary
	}
	if target-failures-1 == 0 {
		return -1
	}
	i := successes - failures + 3
	if i < 0 {
		i = 0
	} else if i > 6 {
		i = 6
	}
	return i
}

func (v *BriefingView) campaignStatusText() string {
	c := v.Mission.Inputs.Campaign
	if c == nil || len(v.Assets.Metadata.CampaignStatus) != 7 {
		return "" // construction rejects missing new readiness, never guessed prose
	}
	result := v.template(0x33b8)
	switch i := campaignPaperChoice(*c); i {
	case -2:
		return result + v.template(0x33cb)
	case -1:
		return result + v.template(0x344f)
	default:
		return result + v.template(0x34a2, v.Assets.Metadata.CampaignStatus[i])
	}
}
