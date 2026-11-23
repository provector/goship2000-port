package menus

import "testing"

func TestPreflightAndOrdersKeyboard(t *testing.T) {
	c := missionController(t)
	readyMission(t, c)
	var latch windowInput
	v := officeViewport(960, 720, false)
	poll := func(b Buttons) { act(t, c, latch.ReadViewport(b, v)) }
	key := func(b Buttons) { poll(Buttons{}); poll(b) }
	missionClick(t, c, 3)
	before := c.Mission.Settings
	for i := 1; i <= 11; i++ {
		key(Buttons{Down: true})
		if c.Mission.settingsHover != i || before != c.Mission.Settings {
			t.Fatal("focus activated or skipped switch", i, c.Mission.settingsHover)
		}
	}
	key(Buttons{Down: true})
	key(Buttons{Enter: true})
	if c.Mission.Settings == before {
		t.Fatal("Enter did not change selected switch")
	}
	poll(Buttons{Enter: true})
	if c.Mission.Settings == before {
		t.Fatal("held Enter repeated")
	}
	key(Buttons{Escape: true})
	missionClick(t, c, 2)
	finishPaperMotion(t, c)
	key(Buttons{Down: true})
	for page := 0; page < 4; page++ {
		if c.missionHit(c.Office.PointerX, c.Office.PointerY) != 3 || c.Mission.Page != page {
			t.Fatal("orders page focus", page)
		}
		key(Buttons{Enter: true})
		poll(Buttons{})
		finishPaperMotion(t, c)
	}
	key(Buttons{Right: true})
	if c.missionHit(c.Office.PointerX, c.Office.PointerY) != 4 {
		t.Fatal("return focus")
	}
	key(Buttons{Enter: true})
	finishPaperMotion(t, c)
	if c.Screen != ScreenBriefing {
		t.Fatal(c.Screen)
	}
}
