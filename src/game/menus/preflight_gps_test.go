package menus

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestGPSCatalogue(t *testing.T) {
	choices, err := parseGPSCodes(stockGPSCodes)
	if err != nil || len(choices) != 90 {
		t.Fatalf("shipped sheet: %d pairs, %v", len(choices), err)
	}
	// Independent landmarks from the supplied JPEG, including a reused challenge.
	for _, q := range []struct {
		sprite            int
		challenge, answer string
	}{{0, "60267", "03279"}, {4, "34620", "10929"}, {9, "97662", "91186"}, {15, "48958", "85567"}, {16, "56824", "01287"}, {17, "66391", "06246"}} {
		found := false
		for _, row := range choices {
			if row.sprite == q.sprite && row.Challenge == q.challenge && row.Authenticator == q.answer {
				found = true
			}
		}
		if !found {
			t.Fatal(q)
		}
	}
	example := `{"schema_version":1,"atlas":"REAL_PCH.PIC","logos":[{"id":"sabres","name":"Sabres","sprite":0,"entries":[{"challenge":"01234","authenticator":"00001"}]}]}`
	for _, bad := range [][2]string{{"01234", "1234"}, {"00001", "0000x"}, {"0,", "18,"}, {`"sprite":0,`, ""}, {"REAL_PCH", "WRONG"}, {`{"challenge":"01234","authenticator":"00001"}`, ""}, {`"00001"}`, `"00001"},{"challenge":"01234","authenticator":"99999"}`}} {
		s := strings.Replace(example, bad[0], bad[1], 1)
		if _, err := parseGPSCodes([]byte(s)); err == nil {
			t.Fatal("accepted invalid catalogue", s)
		}
	}
}

func TestGPSPreflightInteraction(t *testing.T) {
	c := missionController(t)
	c.ConfigurePreflightGPS(true, t.TempDir())
	// Keep the ordinary chooser and session code, limiting the supplied sheet to one known pair.
	c.gps.choices = c.gps.choices[1:2]
	readyMission(t, c)
	s := c.Mission
	missionBefore, err := json.Marshal(s.Data)
	if err != nil {
		t.Fatal(err)
	}
	storeBefore := c.Store.State()
	missionClick(t, c, 3)
	g := &s.gps
	if !g.initialized || g.locked || g.choice.Authenticator != "03279" {
		t.Fatal(g)
	}
	c.takeAudioCues()
	// Mouse, Space and the mapped controller selector all converge on Selector1.
	var mapped Buttons
	applyControllerCommand(&mapped, "menu", "selector1", true, nil)
	for _, b := range []Buttons{{LeftMouse: true}, {Space: true}, mapped} {
		x, y, _ := missionActionPoint(c, 9)
		b.X, b.Y, b.Inside = x, y, true
		var l inputLatch
		act(t, c, l.Read(b))
		if s.Cockpit.APU != 0 || s.PreflightWarning == "" {
			t.Fatal("unlocked APU")
		}
		requireAudioCues(t, c, gs4Denied)
	}
	x, y, _ := missionActionPoint(c, 8)
	send := func(a Actions) { a.X, a.Y, a.Inside = x, y, true; act(t, c, a) }
	send(Actions{Text: []rune("99999")})
	if !g.rejected || g.locked {
		t.Fatal(g)
	}
	requireAudioCues(t, c, gs4Knob, gs4Knob, gs4Knob, gs4Knob, gs4Knob, gs4Denied)
	send(Actions{})
	f := c.Frame(false)
	if !bytes.Contains(f[191*320:], []byte{12}) {
		t.Fatal("hover erased red warning")
	}
	send(Actions{Text: []rune("01")})
	send(Actions{Backspace: true})
	if g.entry != "0" {
		t.Fatal(g)
	}
	act(t, c, Actions{Cancel: true})
	missionClick(t, c, 3)
	if g.entry != "0" || g.choice.Authenticator != "03279" {
		t.Fatal("revisit discarded entry")
	}
	send(Actions{Text: []rune("3279")})
	if !g.locked || g.entry != "03279" || s.PreflightWarning != "" {
		t.Fatal(g)
	}
	send(Actions{Text: []rune("9"), Backspace: true, gpsSteps: []int{1}})
	if g.entry != "03279" {
		t.Fatal("edited locked code")
	}
	act(t, c, Actions{Cancel: true})
	missionClick(t, c, 3)
	if !g.locked || g.entry != "03279" {
		t.Fatal("authenticated session lost on return")
	}
	missionClick(t, c, 9)
	if s.Cockpit.APU != 1 {
		t.Fatal("cover")
	}
	missionClick(t, c, 9)
	if s.Cockpit.APU != 2 {
		t.Fatal("start")
	}
	after, err := json.Marshal(s.Data)
	if err != nil || !bytes.Equal(missionBefore, after) || !reflect.DeepEqual(storeBefore, c.Store.State()) {
		t.Fatal("authentication changed mission draws, data or career", err)
	}
}

func TestGPSShortcutAndFreshSession(t *testing.T) {
	c := missionController(t)
	c.ConfigurePreflightGPS(true, t.TempDir())
	readyMission(t, c)
	missionClick(t, c, 3)
	first := c.Mission.gps.choice
	x, y, _ := missionActionPoint(c, 8)
	apply := func(a Actions) { a.X, a.Y, a.Inside = x, y, true; act(t, c, a) }
	apply(Actions{Text: []rune("111")})
	apply(Actions{gpsSteps: []int{1, 1, 3, 2, 2, 4, 5}})
	if g := c.Mission.gps; g.locked || g.entry != first.Authenticator[:2] {
		t.Fatal(g)
	}
	apply(Actions{gpsSteps: []int{3, 4, 5}})
	if g := c.Mission.gps; !g.locked || g.entry != first.Authenticator {
		t.Fatal(g)
	}
	readyMission(t, c)
	missionClick(t, c, 3)
	if g := c.Mission.gps; g.locked || g.entry != "" || g.choice == first {
		t.Fatal(g)
	}
}

func TestGPSInvalidCatalogueAndOffLaunch(t *testing.T) {
	c := renderingController(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gps_codes.json"), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	c.ConfigurePreflightGPS(true, dir)
	missionClick(t, c, 3)
	missionClick(t, c, 9)
	if c.Mission.Cockpit.APU != 0 || !strings.Contains(c.Mission.PreflightWarning, "gps_codes.json") {
		t.Fatal(c.Mission.PreflightWarning)
	}
	act(t, c, Actions{Cancel: true})
	c.ConfigurePreflightGPS(false, dir)
	c.simulationProfile = "" // Exercise the current ordinary launch rules.
	readyMission(t, c)
	beginInitialization(t, c)
	finishInitialization(t, c)
}
