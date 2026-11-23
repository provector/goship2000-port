package menus

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// These are the centers of the painted PC keys at source resolution.
var keypadKeys = map[rune][2]int{
	'1': {197, 91}, '2': {208, 91}, '3': {219, 91},
	'4': {197, 99}, '5': {208, 99}, '6': {219, 99},
	'7': {197, 107}, '8': {208, 107}, '9': {219, 107},
	'#': {197, 115}, '0': {208, 115}, 'E': {219, 115},
}

func keypadCapture(t *testing.T, c *FrontEnd, name string) {
	t.Helper()
	if dir := os.Getenv("GS2000_S11_CAPTURES"); dir != "" {
		if err := WriteFrontEndFrame(filepath.Join(dir, name+".png"), c, false); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRadioKeypad(t *testing.T) {
	for _, scale := range []int{1, 2} {
		c := missionController(t)
		c.Runtime.SimulationRenderScale = scale
		c.ConfigurePreflightGPS(true, t.TempDir())
		c.gps.choices = c.gps.choices[1:2] // Crossed sabres, answer 03279.
		readyMission(t, c)
		missionClick(t, c, 3)
		var latch windowInput
		viewport := officeViewport(480*scale, 240*scale, false)
		click := func(key rune) {
			p := keypadKeys[key]
			x, y, inside := viewport.LogicalPoint(viewport.X+(float64(p[0])+.5)*viewport.Width/320, viewport.Y+(float64(p[1])+.5)*viewport.Height/200)
			if !inside || (c.missionHit(x, y) != 8) {
				t.Fatal("key outside console", key, x, y)
			}
			buttons := Buttons{X: x, Y: y, Inside: inside, LeftMouse: true}
			act(t, c, latch.ReadViewport(buttons, viewport))
			act(t, c, latch.ReadViewport(buttons, viewport)) // held
			buttons.LeftMouse = false
			act(t, c, latch.ReadViewport(buttons, viewport))
		}
		send := func(a Actions) { a.X, a.Y, a.Inside = 150, 100, true; act(t, c, a) }
		g := &c.Mission.gps
		c.takeAudioCues()
		send(Actions{})
		unlocked := c.Frame(false)
		if scale == 1 {
			keypadCapture(t, c, "unlocked")
		}
		for _, key := range "1234567890" {
			click(key)
			if g.entry != string(key) {
				t.Fatalf("%c painted key entered %q", key, g.entry)
			}
			click('E')
			requireAudioCues(t, c, gs4Knob, gs4Knob)
		}
		send(Actions{gpsSteps: []int{1, 2}})
		click('#')
		if g.entry != "03" || g.shortcut != 2 {
			t.Fatal("hash changed input", g)
		}
		requireAudioCues(t, c, gs4Knob, gs4Knob, gs4Knob)
		click('E')
		send(Actions{gpsSteps: []int{3}})
		if g.entry != "" || g.shortcut != 0 {
			t.Fatal("erase left a shortcut active")
		}
		requireAudioCues(t, c, gs4Knob)
		send(Actions{Text: []rune("4E")})
		requireAudioCues(t, c, gs4Knob, gs4Knob)
		for _, key := range "99999" {
			click(key)
		}
		requireAudioCues(t, c, gs4Knob, gs4Knob, gs4Knob, gs4Knob, gs4Knob, gs4Denied)
		for _, key := range "03279" {
			click(key)
		}
		requireAudioCues(t, c, gs4Knob, gs4Knob, gs4Knob, gs4Knob, gs4Knob, gs4Accepted)
		if !g.locked || g.entry != "03279" {
			t.Fatal("mouse authentication failed")
		}
		click('E')
		click('5')
		click('#')
		send(Actions{Text: []rune("1E"), Backspace: true})
		requireAudioCues(t, c, gs4Knob, gs4Knob, gs4Knob)
		if g.entry != "03279" || !g.locked {
			t.Fatal("locked entry changed")
		}
		locked := c.Frame(false)
		for y := 8; y < 57; y++ {
			if !bytes.Equal(unlocked[y*320+200:y*320+300], locked[y*320+200:y*320+300]) {
				t.Fatal("challenge or patch lost")
			}
		}
		if scale == 1 {
			keypadCapture(t, c, "authenticated")
		}
	}
}

func TestAutoLockLifetime(t *testing.T) {
	c := missionController(t)
	c.ConfigurePreflightGPS(false, t.TempDir())
	if c.gps.last < 1 {
		t.Fatal("no startup selection")
	}
	want := c.gps.choices[c.gps.last-1]
	for mission := 0; mission < 2; mission++ {
		readyMission(t, c)
		for visit := 0; visit < 2; visit++ {
			missionClick(t, c, 3)
			act(t, c, Actions{X: 150, Y: 100, Inside: true})
			if g := c.Mission.gps; !g.locked || g.choice != want || g.entry != want.Authenticator {
				t.Fatal("automatic choice changed")
			}
			frame := c.Frame(false)
			src := c.Office.Assets.Missions.Images["REAL_PCH.PIC"]
			for y := 0; y < 39; y++ {
				i := (8+y)*320 + 226
				j := (want.sprite/4*39+y)*320 + want.sprite%4*47
				if !bytes.Equal(frame[i:i+47], src[j:j+47]) {
					t.Fatal("Off insignia mismatch")
				}
			}
			if mission == 0 && visit == 0 {
				keypadCapture(t, c, "default-off")
			}
			act(t, c, Actions{Cancel: true})
		}
	}
}
