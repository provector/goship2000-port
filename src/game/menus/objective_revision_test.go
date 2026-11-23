package menus

import (
	"bytes"
	"testing"

	gamestate "gunship2000-assets/game/state"
)

func TestObjectiveTargetsLiveCopyPreservesPreparation(t *testing.T) {
	c, _ := retainedTraining(t, false, true, false)
	p := c.Rendering
	prepared := c.Mission.Final.Mission.Targets
	before := filmJSON(prepared)
	if prepared == nil || prepared.ObjectiveRevisions != nil || p.ResultTargets == prepared || p.Initialized.Targets != p.ResultTargets || p.ResultTargets.ObjectiveRevisions == nil {
		t.Fatal("fresh live revision owner aliased or mutated preparation")
	}
	for role, revision := range p.ResultTargets.ObjectiveRevisions {
		if revision != (gamestate.ObjectiveRevision{Seq: 1, Source: prepared.Records[role+2]}) {
			t.Fatal("fresh goal provenance differs from accepted preparation", role, revision)
		}
	}
	liveBefore := p.FilmCheckpoint()["mission"]
	cloned := gamestate.CloneMissionTargets(p.ResultTargets)
	cloned.CompleteObjectiveRevision(0)
	cloned.ObjectiveRevisions[1].Source[61] ^= 0xff
	cloned.Records[2][32] = 0
	if !bytes.Equal(before, filmJSON(prepared)) || p.FilmCheckpoint()["mission"] != liveBefore || p.ResultTargets.ObjectiveRevisions[0].Completed {
		t.Fatal("detached clone changed preparation or live checkpoint")
	}
}
