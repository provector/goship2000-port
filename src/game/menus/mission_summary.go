package menus

import (
	"fmt"
	"gunship2000-assets/game/diagnostics"
	gamestate "gunship2000-assets/game/state"
	"path/filepath"
	"time"
)

func (c *FrontEnd) acknowledgedMissionSummary(pending *gamestate.PendingSortie) ([]byte, error) {
	if pending == nil || pending.Proposal == nil || c.Office == nil || c.Office.Assets == nil || c.Office.Assets.Missions == nil || c.Office.Assets.Missions.Preflight == nil {
		return nil, fmt.Errorf("completed mission summary lacks frozen result or source rank labels")
	}
	accepted := pending.CommissionChoice != nil && *pending.CommissionChoice
	pilot, err := gamestate.ResolveSortieProposalWithChoices(pending.Proposal, accepted, pending.CrewChoices)
	if err != nil {
		return nil, err
	}
	result := gamestate.SortieResultForChoice(pending.Proposal, accepted, pending.CrewChoices)
	if len(c.Office.Assets.Missions.Preflight.OutfitRanks) != 12 {
		return nil, fmt.Errorf("completed mission summary needs all twelve source rank labels")
	}
	var ranks [12]string
	copy(ranks[:], c.Office.Assets.Missions.Preflight.OutfitRanks)
	return gamestate.FormatMissionSummary(gamestate.MissionSummaryInput{Proposal: pending.Proposal, Pilot: pilot,
		Result: result, Choices: pending.CrewChoices, ShortRanks: ranks, Date: time.Now()})
}

// This is an intentional native transaction boundary: SUMMARY.TXT is published
// only after final career acknowledgement succeeds, outside the career journal
// and optional film. A report failure must not undo either valid outcome.
func (c *FrontEnd) publishAcknowledgedMissionSummary(raw []byte, generationErr error) {
	err := generationErr
	if err == nil {
		err = gamestate.WriteMissionSummary(c.SummaryDirectory, raw)
	}
	path := filepath.Join(c.SummaryDirectory, gamestate.MissionSummaryFilename)
	if err != nil {
		diagnostics.Warn("summary", "Completed mission summary not saved; previous report retained", "path", path, "error", err)
		return
	}
	diagnostics.Info("summary", "Completed mission summary saved", "path", path)
}
