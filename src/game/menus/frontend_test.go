package menus

import (
	"bytes"
	"testing"
)

func TestOfficeBriefingArrowNavigation(t *testing.T) {
	c := missionController(t)
	var latch windowInput
	v := officeViewport(960, 720, false)
	poll := func(b Buttons) { act(t, c, latch.ReadViewport(b, v)) }
	// A parked host pointer outside the logical viewport cannot erase key focus.
	poll(Buttons{Down: true})
	if c.Office.HoverKey != 1 {
		t.Fatalf("office roster hotspot: %d", c.Office.HoverKey)
	}
	poll(Buttons{Down: true})
	if c.Office.HoverKey != 1 {
		t.Fatal("held edge repeated")
	}
	poll(Buttons{Enter: true})
	if c.Screen != ScreenRoster {
		t.Fatal("office Enter", c.Screen)
	}
	poll(Buttons{Escape: true})
	if c.Screen != ScreenOffice {
		t.Fatal("roster Escape", c.Screen)
	}
	readyMission(t, c)
	poll(Buttons{Left: true})
	hit := func() int { return c.Mission.View.Hit(c.Office.PointerX, c.Office.PointerY) }
	if hit() != 3 {
		t.Fatal("briefing reverse", hit())
	}
	poll(Buttons{Right: true})
	if hit() != 1 {
		t.Fatal("briefing forward", hit())
	}
	poll(Buttons{Up: true})
	poll(Buttons{Enter: true})
	if c.Screen != ScreenSettings {
		t.Fatal("briefing Enter", c.Screen)
	}
	poll(Buttons{Escape: true})
	if c.Screen != ScreenBriefing {
		t.Fatal("settings Escape", c.Screen)
	}
	poll(Buttons{Down: true})
	x, y, ok := missionActionPoint(c, 2)
	if !ok {
		t.Fatal("missing papers")
	}
	poll(Buttons{Inside: true, X: x, Y: y})
	if hit() != 2 || c.keyboardHotspot != "" {
		t.Fatal("pointer did not regain ownership")
	}
}

func TestPreflightRadioAndContextualSwitchHelp(t *testing.T) {
	for _, required := range []bool{false, true} {
		c := missionController(t)
		c.ConfigurePreflightGPS(required, t.TempDir())
		readyMission(t, c)
		missionClick(t, c, 3)
		g := &c.Mission.gps
		radio := func(text string) { act(t, c, Actions{Inside: true, X: 150, Y: 100, Text: []rune(text)}) }
		if required {
			wrong := "00000"
			if wrong == g.choice.Authenticator {
				wrong = "11111"
			}
			radio(wrong)
			missionClick(t, c, 9)
			if c.gpsAllowsStart() || c.Mission.Cockpit.APU != 0 || c.Mission.PreflightWarning == "" {
				t.Fatal("wrong code admitted start or missing rejection")
			}
			radio(g.choice.Authenticator)
		}
		radio("")
		locked := c.Frame(false)
		choice, entry := g.choice, g.entry
		if !g.locked || choice.Challenge == "" || entry != choice.Authenticator {
			t.Fatal("invalid lock")
		}
		check := func() {
			t.Helper()
			frame := c.Frame(false)
			for _, r := range [][4]int{{120, 97, 26, 12}, {150, 90, 31, 18}} {
				for y := r[1]; y < r[1]+r[3]; y++ {
					i := 320*y + r[0]
					if !bytes.Equal(frame[i:i+r[2]], locked[i:i+r[2]]) {
						t.Fatalf("retained display differs at y=%d, required=%t", y, required)
					}
				}
			}
			if !g.locked || g.choice != choice || g.entry != entry {
				t.Fatal("switch edited locked radio")
			}
		}
		for _, key := range []int{1, 3, 7, 10, 11, 9} {
			missionClick(t, c, key)
			// The switch retains its own contextual monitor on activation.
			m := c.Mission
			help := renderCockpitWithHelp(c.Office.Assets.Missions, m.Settings, m.Cockpit, key, m.SettingsHelp)
			got := c.Frame(false)
			for y := 8; y < 64; y++ {
				i := 320*y + 200
				if !bytes.Equal(got[i:i+100], help[i:i+100]) {
					t.Fatalf("switch %d lost its MFD", key)
				}
			}
			check()
			act(t, c, Actions{Inside: true, X: c.pointerX, Y: c.pointerY})
			check()
		}
		x, y, _ := missionActionPoint(c, 2)
		act(t, c, Actions{Inside: true, X: x, Y: y})
		alternate := c.Frame(false)
		same := true
		for y := 8; y < 64; y++ {
			i := 320*y + 200
			same = same && bytes.Equal(alternate[i:i+100], locked[i:i+100])
		}
		if same {
			t.Fatal("alternate switch help unavailable")
		}
		act(t, c, Actions{})
		check()
	}
}
