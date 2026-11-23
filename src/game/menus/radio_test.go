package menus

import (
	"bytes"
	"testing"

	"gunship2000-assets/game/cockpit"
)

// Test the transition through the two real APU actions and the paced system
// checks. The established radio view supplies the expected physical LCD ink.
func TestRadioDuringAPUInitialization(t *testing.T) {
	for _, mode := range []string{"automatic", "authenticated"} {
		t.Run(mode, func(t *testing.T) {
			c := renderingController(t)
			c.ConfigurePreflightGPS(mode == "authenticated", t.TempDir())
			// Limit only the test catalogue so before/after captures use one
			// identical insignia, challenge and leading-zero answer.
			c.gps.choices = c.gps.choices[1:2]
			c.gps.last = 1
			missionClick(t, c, 3)
			g := &c.Mission.gps
			radio := func(text string) { act(t, c, Actions{Inside: true, X: 150, Y: 100, Text: []rune(text)}) }
			if mode == "authenticated" {
				radio("00000")
				missionClick(t, c, 9)
				if g.locked || c.Mission.Cockpit.APU != 0 || c.Mission.PreflightWarning == "" {
					t.Fatal("wrong answer admitted APU")
				}
				radio(g.choice.Authenticator)
			}
			radio("")
			if !g.locked || g.choice.Challenge == "" || g.entry != g.choice.Authenticator {
				t.Fatal("missing lock before start")
			}
			initial := c.Frame(false)
			gps := *g
			keypadCapture(t, c, mode+"-radio")
			rect := func(frame []byte, x, y, w, h int) []byte {
				var region []byte
				for row := y; row < y+h; row++ {
					i := 320*row + x
					region = append(region, frame[i:i+w]...)
				}
				return region
			}
			checkRadio := func(frame []byte) {
				t.Helper()
				for _, r := range [][4]int{{120, 97, 26, 12}, {150, 90, 31, 18}} {
					if !bytes.Equal(rect(frame, r[0], r[1], r[2], r[3]), rect(initial, r[0], r[1], r[2], r[3])) {
						t.Errorf("LCD differs at %v, screen %v", r, c.Screen)
						break
					}
				}
				if gps != *g {
					t.Fatal("radio state changed")
				}
			}
			missionClick(t, c, 9)
			missionClick(t, c, 9)
			if c.Mission.Cockpit.APU != 2 {
				t.Fatal("APU start action missed")
			}
			checkRadio(c.Frame(false))
			for i := 0; i < 200 && c.Screen == ScreenSettings; i++ {
				if err := c.Tick(); err != nil {
					t.Fatal(err)
				}
			}
			for stage := 0; stage < 7; stage++ {
				if c.Screen != ScreenSceneInitialization || c.SceneEntry.Stage != stage {
					t.Fatal("system check", stage, c.Screen)
				}
				frame := c.Frame(false)
				checkRadio(frame)
				expected := make([]byte, 64000)
				if err := cockpit.DrawInitialization(expected, c.Office.Assets.Cockpit, stage+1); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(rect(frame, 200, 8, 100, 56), rect(expected, 200, 8, 100, 56)) {
					t.Fatal("MFD status text changed", stage)
				}
				if stage == 4 {
					keypadCapture(t, c, mode+"-checks")
				}
				c.MarkScenePresented()
				if err := c.AdvanceSceneInitialization(SceneInitializationLineSeconds); err != nil {
					t.Fatal(err)
				}
			}
			finishInitialization(t, c)
		})
	}
}
