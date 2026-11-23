package menus

import "testing"

func TestSceneSmokeAcknowledgesAdmittedDebriefBeforeReentry(t *testing.T) {
	c := nResultController(t)
	c.Rendering.Initialized.EndSortieForQoL()
	if err := c.finishSortieResult(); err != nil {
		t.Fatal(err)
	}
	if c.Screen != ScreenDebrief {
		t.Fatal("fixture did not reach admitted result")
	}
	sequence := renderingSmokeSequence{Step: 19}
	for tick := 0; tick < 600 && c.Screen != ScreenBriefing; tick++ {
		if err := c.Handle(sequence.Next(c)); err != nil {
			t.Fatal(err)
		}
		if err := c.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen != ScreenBriefing || sequence.Step != 19 {
		t.Fatal("scene smoke bypassed result or failed office reentry", c.Screen, sequence.Step)
	}
	if c.Store.State().SelectedPilot().Statistics.MissionCampaignCounter != 1 {
		t.Fatal("smoke result not consumed once")
	}
}
