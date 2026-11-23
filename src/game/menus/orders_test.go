package menus

import (
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/navigation"
	"strings"
	"testing"
	"time"
)

func TestCtrlOrderSamplerAndTimestampedRecipient(t *testing.T) {
	if sampleWingmanOrderKey(func(k int32) bool { return k == 'H' }) != "h" {
		t.Fatal("CtrlH make not captured")
	}
	var latch inputLatch
	a := latch.Read(Buttons{WingmanOrder: "h"})
	if a.WingmanOrder != "h" {
		t.Fatal(a)
	}
	if b := latch.Read(Buttons{WingmanOrder: "h"}); b.WingmanOrder != "" {
		t.Fatal("held repeats", b)
	}
	var q flightCommandInput
	a.WingmanRecipient = 3
	q.SampleAt(50*time.Millisecond, a)
	q.NextAt(49 * time.Millisecond)
	if len(q.TakeAux()) != 0 {
		t.Fatal("early order")
	}
	q.NextAt(50 * time.Millisecond)
	aux := q.TakeAux()
	if len(aux) != 1 {
		t.Fatal(aux)
	}
	slot, key, ok := decodeWingmanOrder(aux[0])
	if !ok || slot != 3 || key != "h" {
		t.Fatal(slot, key, ok)
	}
	q.NextAt(100 * time.Millisecond)
	if len(q.TakeAux()) != 0 {
		t.Fatal("order repeated")
	}
}
func TestControllersUseExistingSharedOrders(t *testing.T) {
	for _, v := range []struct{ context, id, token string }{{"gps", "orders.halt", "h"}, {"gps", "orders.land", "l"}, {"flight", "orders.direct_hold", "order:h"}, {"flight", "orders.direct_base", "order:b"}} {
		found := false
		for _, c := range controllerCommands {
			if c.ID == v.id && c.Context == v.context && c.Token == v.token {
				found = true
			}
		}
		if !found {
			t.Fatal(v)
		}
		var b Buttons
		applyControllerCommand(&b, v.context, v.token, true, nil)
		if v.context == "gps" && b.GPSKey != v.token || v.context == "flight" && b.WingmanOrder != v.token[len("order:"):] {
			t.Fatal(v, b)
		}
	}
}
func TestGPSRecipientAndDirectMemberRemainIndependent(t *testing.T) {
	c := mastFixture(t, 0, "pass")
	jFinishFade(t, c)
	p := c.Rendering
	p.r1Mechanics = true
	p.Flight.Combat.SetOrdersEnabled(true)
	// Isolated assigned-member carriers. This is dispatcher evidence, not a flown
	// Flight mission or natural objective completion.
	for slot := 1; slot <= 2; slot++ {
		e := p.Initialized.Entities[0]
		e.ID = []string{"", "order-one", "order-two"}[slot]
		e.RuntimeRow = slot
		e.SourceRow = slot
		e.FlightSlot = slot
		e.Kind = 0x20
		e.Participation = 1
		e.DynamicRaw[0] = 0x20
		e.DynamicRaw[1] = 1
		e.ControlRaw = [70]byte{}
		e.ControlRaw[1] = 2
		e.ControlRaw[2] = 1
		e.ControlRaw[5] = 0
		p.Initialized.Entities[slot] = e
	}
	p.Initialized.FlightRows = []int{0, 1, 2}
	p.Initialized.Rows[1].Raw[10] |= 0x20
	p.Flight.OrderRecipient = 1
	p.syncFollowerNavigation()
	c.Handle(Actions{})
	if err := c.openGPS(); err != nil {
		t.Fatal(err)
	}
	p.GPS.Selected = 2
	if err := c.handleGPS(Actions{GPSKey: "s"}); err != nil {
		t.Fatal(err)
	}
	if p.Initialized.Entities[1].ControlRaw[8] != 0 || p.Initialized.Entities[2].ControlRaw[8] != 1 || p.Flight.OrderRecipient != 1 {
		t.Fatal("GPS used cockpit recipient")
	}
	if err := c.handleGPS(Actions{GPSKey: "h"}); err != nil {
		t.Fatal(err)
	}
	if p.Initialized.Entities[2].ControlRaw[1] != 4 || p.Initialized.Entities[1].ControlRaw[1] != 2 {
		t.Fatal("GPS hold recipient")
	}
	// GPS stays modal and consumes order input without flying anyone.
	steps := p.Flight.Steps
	c.AdvanceRendering(.5)
	if p.Flight.Steps != steps {
		t.Fatal("GPS changed simulation advancement")
	}
	c.closeGPS()
	code, _ := encodeWingmanOrder(1, "s")
	if err := p.applyWingmanAux(code); err != nil {
		t.Fatal(err)
	}
	if p.Initialized.Entities[1].ControlRaw[8] != 1 || p.Initialized.Entities[2].ControlRaw[8] != 3 {
		t.Fatal("direct used GPS recipient")
	}
	// Follower F is a real route transaction with full cancellation.
	c.openGPS()
	p.GPS.Selected = 1
	before := p.Flight.Navigation[1]
	c.handleGPS(Actions{GPSKey: "f"})
	if !p.GPS.Editing {
		t.Fatal("follower F ignored")
	}
	c.handleGPS(Actions{Cancel: true})
	if p.Flight.Navigation[1] != before {
		t.Fatal("cancel lost row")
	}
	c.handleGPS(Actions{GPSKey: "f"})
	p.GPS.CursorX, p.GPS.CursorY = 112, 80
	c.handleGPS(Actions{Selector1: true})
	c.handleGPS(Actions{GPSKey: "f"})
	if p.Initialized.Entities[1].Kind != 0x23 || p.Initialized.Entities[1].Participation != 0 || p.Flight.Navigation[1].Point(1) == (navigation.Point{}) {
		t.Fatal("route did not reach actual actor")
	}
}

// This is a newly generated Flight launch and recorded input sequence. It
// checks deterministic dispatch/playback; it does not claim pad-flight success.
func TestOrderFilmUsesRecordedRecipientAndRules(t *testing.T) {
	c := mastFixture(t, 0, "pass")
	jFinishFade(t, c)
	p := c.Rendering
	act := func(a Actions) {
		t.Helper()
		if err := c.Handle(a); err != nil {
			t.Fatal(err)
		}
	}
	act(Actions{})
	act(Actions{ViewKey: 5, ViewShift: true})
	act(Actions{ViewKey: 1})
	if p.Flight.OrderRecipient != 1 {
		t.Fatal("ShiftF5 recipient", p.Flight.OrderRecipient)
	}
	act(Actions{WingmanOrder: "h", InputTime: time.Millisecond})
	if err := c.advanceFlight(.12); err != nil {
		t.Fatal(err)
	}
	act(Actions{})
	act(Actions{GPSOpen: true})
	act(Actions{})
	act(Actions{GPSKey: "n"})
	act(Actions{GPSKey: "n"})
	if p.GPS == nil || p.GPS.Selected != 2 {
		t.Fatal("GPS next recipient")
	}
	act(Actions{GPSKey: "h"})
	act(Actions{GPSKey: "n"})
	act(Actions{GPSKey: "f"})
	act(Actions{Inside: true, X: 130, Y: 80, Selector1: true})
	act(Actions{GPSKey: "f"})
	act(Actions{GPSKey: "b"})
	act(Actions{ViewKey: 1})
	act(Actions{})
	act(Actions{WingmanOrder: "s", InputTime: time.Second})
	if err := c.advanceFlight(.12); err != nil {
		t.Fatal(err)
	}
	act(Actions{EndMission: true})
	act(Actions{ConfirmQuit: true})
	if c.Screen == ScreenQuit {
		act(Actions{ConfirmQuit: true})
	}
	f, err := c.SealRecordedFilm()
	if err != nil {
		t.Fatal(err)
	}

	opt := configuration.Defaults()
	opt.DisableWingmenAI = true
	q, err := NewFilmPlayback(c.Office.Assets, f, opt)
	if err != nil {
		t.Fatal(err)
	}
	for !q.Done {
		if err = q.Step(); err != nil {
			t.Fatalf("%v diag=%+v", err, q.FailureDiagnostic)
		}
	}
	found := false
	for _, op := range f.Operations {
		if strings.Contains(string(op.Payload), `"Aux":[`) {
			found = true
		}
	}
	if !found {
		t.Fatal("orders not recorded")
	}
}
