package menus

import (
	"fmt"
	"gunship2000-assets/game/diagnostics"
	gamestate "gunship2000-assets/game/state"
)

// continueAfterSortie is reached only after the result's final-ack transaction
// is durable. Original COM0159..017b chains GS-e -> library -> GS-c.
// The library now owns this once-only successor; an ongoing Campaign reaches
// this next preparation only on Library EXIT.
func (c *FrontEnd) continueAfterSortie() error {
	// Discard the old debrief/flight before publishing another actionable page.
	// Restart still opens the office: this is not unfinished-session resumption.
	if err := c.RefreshOffice(); err != nil {
		return err
	}
	p := c.Store.State().SelectedPilot()
	if p == nil || p.Status != gamestate.StatusActive || p.MissionType != gamestate.MissionCampaign || p.Campaign == nil || p.Campaign.Phase != gamestate.CampaignActive || gamestate.CampaignTerminal(p.Campaign, p.MissionType) != 0 {
		return nil
	}
	// Preparation clones the committed Campaign. A failed next mission cannot
	// undo the completed result or publish part of the next draft/cache history.
	next, err := prepareMissionSession(c.Office.Assets.Missions, p, c.nextMissionSeed(), c.MissionDrawStream, c.retainedMissionObject, c.safeGameplayProfile(), c.r6Profile())
	if err != nil {
		c.Message = "Previous result saved. Next mission could not be prepared. See terminal."
		diagnostics.Warn("career", "Next Campaign preparation failed; prior result remains committed", "status", c.Message, "error", err)
		return fmt.Errorf("completed result saved; next Campaign preparation: %w", err)
	}
	diagnostics.Info("career", "Continued Campaign preparation ready", "world", next.Data.World)
	c.publishMission(next)
	return nil
}
