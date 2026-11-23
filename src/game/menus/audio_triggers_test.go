package menus

import (
	gamestate "gunship2000-assets/game/state"
	"reflect"
	"testing"
)

// These literals are independently checked against the51 original caller
// windows by tools/step_f_trigger_source_checks.py, not derived from the queue.
func TestAudioTriggerCommandIDsMatchOriginalImmediates(t *testing.T) {
	got := []uint16{gs4Computer, gs4Switch, gs4Knob, gs4Denied, gs4Accepted, gs4Entry, gs4Page, gs4Door}
	want := []uint16{0x0d, 0x0e, 0x13, 0x21, 0x22, 0x23, 0x24, 0x42}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("original GS4 command numbers differ: %#v", got)
	}
}

func requireAudioCues(t *testing.T, c *FrontEnd, expected ...uint16) {
	t.Helper()
	if actual := c.takeAudioCues(); !reflect.DeepEqual(actual, expected) {
		t.Fatalf("source cue sequence got%#v want%#v", actual, expected)
	}
	if len(c.takeAudioCues()) != 0 {
		t.Fatal("draining repeated source commands")
	}
}

func TestAudioTriggerOfficeWindowEdgesAndNoHover(t *testing.T) {
	c := sourceFrontEnd(t, true)
	input := &windowInput{}
	b := Buttons{X: 140, Y: 87, Inside: true}
	stateWindowInput(t, c, input, b)
	_ = c.Frame(true)
	requireAudioCues(t, c)
	b.LeftMouse = true
	stateWindowInput(t, c, input, b)
	requireAudioCues(t, c, gs4Computer)
	stateWindowInput(t, c, input, b)
	_ = c.Frame(true)
	if err := c.Tick(); err != nil {
		t.Fatal(err)
	}
	requireAudioCues(t, c)
	b.LeftMouse = false
	stateWindowInput(t, c, input, b)
	b.X, b.Y, b.LeftMouse = 184, 70, true
	stateWindowInput(t, c, input, b)
	if c.Screen != ScreenRoster {
		t.Fatal(c.Screen)
	}
	requireAudioCues(t, c, gs4Accepted)
	stateWindowInput(t, c, input, b)
	requireAudioCues(t, c) // Opening press cannot edit/post the destination.
	b.LeftMouse, b.Escape = false, true
	stateWindowInput(t, c, input, b)
	requireAudioCues(t, c, gs4Accepted)
}

func TestAudioTriggerOfficeGuardsChoosersRecoveryAndQuit(t *testing.T) {
	c := sourceFrontEnd(t, false)
	officeClick(t, c, 100, 170)
	requireAudioCues(t, c, gs4Denied)
	officeClick(t, c, 230, 50)
	requireAudioCues(t, c, gs4Denied)

	c = sourceFrontEnd(t, true)
	officeClick(t, c, 270, 45)
	requireAudioCues(t, c, gs4Accepted)
	act(t, c, Actions{X: 10, Y: 10, Inside: true, Selector1: true, PointerPressed: true})
	requireAudioCues(t, c, gs4Denied)
	act(t, c, Actions{Cancel: true})
	requireAudioCues(t, c, gs4Accepted)
	officeClick(t, c, 100, 170)
	requireAudioCues(t, c, gs4Accepted)
	finishFolder(t, c)
	requireAudioCues(t, c) // No invented sound per animation group.
	act(t, c, Actions{Cancel: true})
	requireAudioCues(t, c, gs4Accepted)
	act(t, c, Actions{Quit: true})
	act(t, c, Actions{Text: []rune{'n'}})
	requireAudioCues(t, c)
	if err := c.Store.PrepareMissionSnapshot(); err != nil {
		t.Fatal(err)
	}
	if err := c.RefreshOffice(); err != nil {
		t.Fatal(err)
	}
	officeClick(t, c, 290, 150)
	if c.Screen != ScreenRecovery {
		t.Fatal("reference recovery hotspot", c.Office.HoverKey, c.Screen)
	}
	requireAudioCues(t, c, gs4Entry)
	act(t, c, Actions{Cancel: true})
	requireAudioCues(t, c, gs4Accepted)
}

func TestAudioTriggerRosterDraftDoesNotInventTypingOrEmblemCues(t *testing.T) {
	c := sourceFrontEnd(t, true)
	officeClick(t, c, 184, 70)
	requireAudioCues(t, c, gs4Accepted)
	act(t, c, Actions{X: 30, Y: 90, Inside: true, Selector2: true, PointerPressed: true})
	requireAudioCues(t, c, gs4Accepted)
	act(t, c, Actions{Text: []rune("XYZ")})
	act(t, c, Actions{Enter: true})
	act(t, c, Actions{Right: true})
	act(t, c, Actions{Cancel: true})
	requireAudioCues(t, c)
	act(t, c, Actions{X: 30, Y: 90, Inside: true, Selector1: true, PointerPressed: true})
	requireAudioCues(t, c, gs4Accepted)
}

func TestAudioTriggerBriefingPapersSettingsAndKnobs(t *testing.T) {
	c := missionController(t)
	readyMission(t, c)
	requireAudioCues(t, c, gs4Door)
	s := c.Mission
	missionClick(t, c, 2)
	requireAudioCues(t, c, gs4Accepted)
	missionClick(t, c, 3)
	requireAudioCues(t, c, gs4Page)
	for i := 0; i < 10; i++ {
		_ = c.Frame(true)
	}
	requireAudioCues(t, c)
	act(t, c, Actions{Cancel: true})
	finishPaperMotion(t, c)
	requireAudioCues(t, c, gs4Accepted)
	missionClick(t, c, 3)
	requireAudioCues(t, c, gs4Accepted)
	for key := 1; key <= 7; key++ {
		missionClick(t, c, key)
		want := gs4Computer
		if key <= 2 {
			want = gs4Switch
		}
		requireAudioCues(t, c, want)
	}
	for _, key := range []int{10, 11} {
		missionClick(t, c, key)
		requireAudioCues(t, c, gs4Knob)
	}
	missionClick(t, c, 9)
	requireAudioCues(t, c, gs4Computer)
	// The exceptional APU leave arm follows the new switch cue. A held pointer
	// then produces neither another leave nor a click.
	missionClick(t, c, 1)
	requireAudioCues(t, c, gs4Switch, gs4Computer)
	act(t, c, Actions{X: c.pointerX, Y: c.pointerY, Inside: true})
	requireAudioCues(t, c)
	act(t, c, Actions{Quit: true})
	act(t, c, Actions{Text: []rune{'n'}})
	requireAudioCues(t, c)
	if c.Mission != s || c.Screen != ScreenSettings {
		t.Fatal("quit cancellation changed mission/settings")
	}
	act(t, c, Actions{Cancel: true})
	requireAudioCues(t, c) // C0566 Escape return has no direct source cue.
}

func TestAudioTriggerOutfitSourceActions(t *testing.T) {
	v := outfitTestView(t)
	cases := []struct {
		a    Actions
		want uint16
	}{
		{outfitPanelAction(0, "P", false, false), gs4Denied},
		{outfitPanelAction(1, "P", false, false), gs4Accepted},
		{outfitPanelAction(3, "", true, false), gs4Accepted},
		{outfitPanelAction(0, "H", false, false), gs4Accepted},
		{Actions{Cancel: true}, gs4Accepted},
		{outfitPanelAction(1, "L", false, false), gs4Denied},
		{Actions{X: 300, Y: 130, Inside: true, Selector1: true}, gs4Denied},
	}
	for _, tc := range cases {
		got, err := v.Handle(tc.a)
		if err != nil || got.AudioCue != tc.want {
			t.Fatalf("action%+v cue%x want%x err%v", tc.a, got.AudioCue, tc.want, err)
		}
	}
	v.Mission.CampaignUnavailableAircraft = []int{int(v.Rows[0][0])}
	got, err := v.Handle(outfitPanelAction(0, "D", false, false))
	if err != nil || got.AudioCue != gs4Denied {
		t.Fatal("Campaign duplication denial", got, err)
	}
	got, err = v.Handle(outfitPanelAction(0, "H", false, false))
	if err != nil || got.AudioCue != gs4Accepted {
		t.Fatal("retained aircraft swap has its separate source predicate", got, err)
	}
}

func TestAudioTriggerOrdnanceNestedAndRejectedAcceptOrder(t *testing.T) {
	v := ordnanceTestView(t, gamestate.RankCaptain)
	index := ordnanceChoice(t, v, "fits", 7)
	if _, err := v.Activate(index); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(v.audioCues, []uint16{gs4Accepted}) {
		t.Fatal(v.audioCues)
	}
	v.audioCues = nil
	if _, err := v.Handle(Actions{Cancel: true}); err != nil || len(v.audioCues) != 0 {
		t.Fatal("ORDNANCE Escape does not select Back or make a sound")
	}
	// The source row selector ignores horizontal position. Move it into the
	// nonselectable heading, rather than assuming a blank left-side click misses.
	_, _ = v.Handle(Actions{X: 5, Y: 100, Inside: true})
	if _, err := v.Handle(Actions{X: 5, Y: 0, Inside: true, Selector1: true}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(v.audioCues, []uint16{gs4Denied}) {
		t.Fatal(v.audioCues)
	}
	v.audioCues = nil
	_, _ = v.Activate(ordnanceChoice(t, v, "back", -1))
	v.audioCues = nil
	gamestate.RowPut(v.Working[:], 29, 0)
	_, _ = v.Activate(ordnanceChoice(t, v, "back", -1))
	if !reflect.DeepEqual(v.audioCues, []uint16{gs4Accepted, gs4Denied}) || v.Done {
		t.Fatal("positive row22 then acceptance rejection21", v.audioCues, v.Done)
	}
	v.audioCues = nil
	before := v.Working
	v.Choices = []OrdnanceChoice{{Action: "replace", Value: 1, Enabled: false}}
	_, _ = v.Activate(0)
	if !reflect.DeepEqual(v.audioCues, []uint16{0x22, 0x21}) || v.Working != before {
		t.Fatal("positive row then unavailable callback, without equipment mutation", v.audioCues)
	}
}

func TestAudioTriggerLoadoutControllerForwardsOnceAndReturnGuard(t *testing.T) {
	c := missionController(t)
	readyMission(t, c)
	requireAudioCues(t, c, 0x42)
	s := c.Mission
	for len(s.Flight) < 5 {
		s.Flight = append(s.Flight, s.Flight[0])
	}
	s.ReadinessRows = [][36]byte{validFlightRow(true), validFlightRow(false), validFlightRow(false), validFlightRow(false), validFlightRow(false)}
	if err := c.openOutfit(); err != nil {
		t.Fatal(err)
	}
	gamestate.RowPut(s.ReadinessRows[3][:], 25, 1010)
	missionClick(t, c, 6)
	requireAudioCues(t, c, 0x21) // Port whole-flight guard replaces RETURN22.
	if c.Screen != ScreenOutfit {
		t.Fatal("rejected return changed screen")
	}
	gamestate.RowPut(s.ReadinessRows[3][:], 25, 500)
	missionClick(t, c, 6)
	requireAudioCues(t, c, 0x22)
	if err := c.openOrdnance(0); err != nil {
		t.Fatal(err)
	}
	index := ordnanceChoice(t, s.Ordnance, "fits", 7)
	// The original row selector consumes relative vertical movement; a raw
	// full-screen row coordinate is no longer an activation shortcut.
	x, y, _ := missionActionPoint(c, index)
	act(t, c, Actions{X: x, Y: y, Inside: true, Selector1: true, PointerPressed: true})
	requireAudioCues(t, c, 0x22)
	act(t, c, Actions{X: x, Y: y, Inside: true})
	requireAudioCues(t, c) // The view queue is drained once by the controller.
}
