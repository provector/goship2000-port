package navigation

import (
	"bytes"
	"encoding/binary"
	"gunship2000-assets/game/cockpit"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/world"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func routeFixture() *GPS {
	var rs [5]Route
	for i := range rs[0].Raw {
		rs[0].Raw[i] = byte(i + 20)
	}
	rs[0].SetIndex(1)
	rs[0].Raw[1] = 3
	_ = rs[0].SetPoint(1, Point{200000, 220000, 0x1234})
	_ = rs[0].SetPoint(2, Point{210000, 230000, 0x4567})
	rs[0].SetPoint(3, Point{0, 0, 0x89ab})
	return New(&rs, [5]Point{{H0: 257510, H1: 268878}}, [5]bool{true})
}
func TestSourceThreePointTransaction(t *testing.T) {
	g := routeFixture()
	before := g.Routes[0]
	if err := g.StartEdit(); err != nil {
		t.Fatal(err)
	}
	for n := 1; n <= 3; n++ {
		if p := g.Routes[0].Point(n); p.H0 != 0 || p.H1 != 0 || p.Tail != before.Point(n).Tail {
			t.Fatalf("clear point%d=%+v", n, p)
		}
	}
	for _, span := range [][2]int{{1, 10}, {40, 70}} {
		if !bytes.Equal(g.Routes[0].Raw[span[0]:span[1]], before.Raw[span[0]:span[1]]) {
			t.Fatal("opaque data erased")
		}
	}
	exit, e := g.Handle(Input{Escape: true})
	if e != nil || exit != 0 || g.Editing || g.Routes[0] != before {
		t.Fatal("Escape mustrestore70andremainGPS")
	}
	g.StartEdit()
	g.FinishEdit()
	if g.Routes[0].Index() != 0 || g.Routes[0] == before {
		t.Fatal("emptyroutecommitislegal")
	}
	g.StartEdit()
	for n := 1; n <= 3; n++ {
		g.CursorX, g.CursorY = 20+n, 60+n
		g.Append()
	}
	if g.Editing || g.Routes[0].Index() != 1 || g.Routes[0].Raw[1] != 3 {
		t.Fatal("thirdpoint mustfinish")
	}
	for n := 1; n <= 3; n++ {
		p := g.Routes[0].Point(n)
		want := HighPoint(20+n, 60+n)
		if p.H0 != want.H0 || p.H1 != want.H1 || p.Tail != before.Point(n).Tail {
			t.Fatalf("point%d %+v", n, p)
		}
	}
}
func TestSourceFKeyBypassesEditRollback(t *testing.T) {
	g := routeFixture()
	g.StartEdit()
	g.CursorX, g.CursorY = 100, 100
	g.Append()
	partial := g.Routes[0]
	exit, e := g.Handle(Input{ViewKey: 2, Escape: true})
	if e != nil || exit != 2 || !g.Editing || g.Routes[0] != partial {
		t.Fatal("view exit must preserveprovisionalroute beforeEscape")
	}
}
func TestSourceH0OnlyAndAsymmetricCycle(t *testing.T) {
	r := Route{}
	r.SetIndex(1)
	r.SetPoint(2, Point{H0: 0, H1: 100})
	r.Next()
	if r.Index() != 1 {
		t.Fatal("H1 doesnotmakepointpresent")
	}
	r.SetPoint(2, Point{H0: 1, H1: 0})
	r.Next()
	if r.Index() != 2 {
		t.Fatal("next2")
	}
	r.Previous()
	if r.Index() != 1 {
		t.Fatal("prev1")
	}
	r.Previous()
	if r.Index() != 3 {
		t.Fatal("sourceprevwrap3 notlastnonempty")
	}
	r.SetIndex(0)
	r.Raw[1] = 1
	r.Previous()
	if r.Index() != 0 {
		t.Fatal("rawfirstdword gate")
	}
	r.Raw[1] = 0
	r.Previous()
	if r.Index() != 3 {
		t.Fatal("allzeroheader sourceprevious3")
	}
}
func TestSourceMapConversionsAndWrap(t *testing.T) {
	if p := HighPoint(112, 80); p.H0 != 257510 || p.H1 != 268878 {
		t.Fatal(p)
	}
	g := routeFixture()
	g.CursorX, g.CursorY = 112, 80
	g.StartEdit()
	g.Handle(Input{Key: "space"})
	if !g.Low || g.Center != HighPoint(g.CursorX, g.CursorY) {
		t.Fatal("edit highcenter")
	}
	g.Low = true
	g.Center = Point{H0: 0x30000, H1: 0x30000}
	g.Handle(Input{AxisX: 1024, AxisY: 1024})
	if g.Center.H0 != 0x28000 || g.Center.H1 != 0x38000 {
		t.Fatalf("shift mustwrapbeforeCWD %+v", g.Center)
	}
	g.Handle(Input{AxisX: -32768, AxisY: -32768})
	if g.Center.H0 != 0x28000 || g.Center.H1 != 0x38000 {
		t.Fatal("wordshiftzero")
	}
}
func TestSourceNumericMoveAndReadOnlyFollower(t *testing.T) {
	g := routeFixture()
	g.Low = true
	g.Handle(Input{Key: "1"})
	if g.Moving != 1 || g.Center.H0 != 200000 {
		t.Fatal("selectexisting")
	}
	g.Handle(Input{AxisX: 16})
	if g.Routes[0].Point(1).H0 != 200512 {
		t.Fatal("writesroutepoint")
	}
	g.Handle(Input{Key: "1"})
	if g.Moving != 0 {
		t.Fatal("repeatdeselect")
	}
	g.Active[1] = true
	g.Handle(Input{Key: "n"})
	before := *g.Routes
	if e := g.StartEdit(); e == nil || *g.Routes != before {
		t.Fatal("AIorderdomain mustreadonly")
	}
}
func TestSourceHomeFARPTailsAndAutopilotAdapter(t *testing.T) {
	g := routeFixture()
	r := &g.Routes[0]
	tail := r.Point(1).Tail
	home := Point{300000, 350000, 0}
	farp := Point{400000, 300000, 0}
	r.Home(home, farp, true)
	if r.Point(1).H0 != home.H0 {
		t.Fatal("firsthome")
	}
	r.Home(home, farp, true)
	if r.Point(1).H0 != farp.H0 || r.Point(1).Tail != tail {
		t.Fatal("FARPtoggle")
	}
	for n := 2; n <= 3; n++ {
		if r.Point(n).Present() {
			t.Fatal("tailrouteclear")
		}
	}
	s := flight.State{}
	r.ToFlight(&s)
	if s.WaypointIndex != 1 || s.Waypoints[1].H0 != farp.H0 {
		t.Fatal("actualflightadapter")
	}
	s.WaypointIndex = 0
	r.FromFlight(&s)
	if r.Index() != 0 || r.Point(1).H0 != farp.H0 {
		t.Fatal("flightadvancepreservepoints")
	}
}
func TestGPSVisibleMapEdges(t *testing.T) {
	g := routeFixture()
	g.Low = true
	a := &cockpit.Assets{Sine: make([]int16, 1025), Images: map[string]cockpit.Image{"TESTMAP.GS2": {Pixels: make([]byte, 64000)}}, Sprites: map[string]cockpit.Image{"gps-overview-box": {Width: 11, Height: 9, Pixels: make([]byte, 99)}}}
	w := &world.World{ID: "TEST", Grid: make([]byte, 4096), TileAtlas: make([]byte, 192*144), Overview: make([]byte, 89*65)}
	for _, h0 := range []uint32{0xa000, 0x76000} {
		for _, h1 := range []uint32{0xa000, 0x76000} {
			g.Center = Point{H0: h0, H1: h1}
			if e := g.Draw(make([]byte, 64000), w, a); e != nil {
				t.Fatalf("corner%x,%x:%v", h0, h1, e)
			}
		}
	}
}
func TestNavigationBearingOriginalAxes(t *testing.T) {
	raw, e := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if e != nil {
		t.Skip("originalsourceoperandfixtureabsent")
	}
	a := &world.Bundle{AngleBases: make([]uint16, 8), Atan: make([]uint16, 4096)}
	for i := range a.AngleBases {
		a.AngleBases[i] = binary.LittleEndian.Uint16(raw[0x4c452+2*i:])
	}
	for i := range a.Atan {
		a.Atan[i] = binary.LittleEndian.Uint16(raw[0x4c462+2*i:])
	}
	origin := Point{H0: 200000, H1: 200000}
	for _, q := range []struct {
		to       Point
		bearing  uint16
		distance uint16
	}{{Point{200000, 201600, 0}, 0, 200}, {Point{201600, 200000, 0}, 0x4000, 200}, {Point{200000, 198400, 0}, 0x8000, 200}, {Point{198400, 200000, 0}, 0xc000, 200}} {
		b, d := Bearing(origin, q.to, a)
		if b != q.bearing || d != q.distance {
			t.Fatalf("%+v bearing%x metric%d", q, b, d)
		}
	}
}

func TestSourceRelativeHomeAndDistinctBase(t *testing.T) {
	rows := make([][32]byte, 2)
	binary.LittleEndian.PutUint32(rows[0][2:], 1)
	binary.LittleEndian.PutUint32(rows[0][6:], uint32(0xffffff9c))
	binary.LittleEndian.PutUint32(rows[1][2:], 30000)
	binary.LittleEndian.PutUint32(rows[1][6:], 40000)
	var control [70]byte
	control[5] = 1
	p, e := ResolveHome(rows, control, 0)
	if e != nil || p.H0 != 30000 || p.H1 != 39900 {
		t.Fatal(p, e)
	}
	control[5] = 255
	if _, e = ResolveHome(rows, control, 0); e == nil {
		t.Fatal("negativeleaderdomain")
	}
	r := Route{}
	r.SetPoint(1, Point{10000, 99999, 7})
	r.HomeFrom(Point{10000, 20000, 0}, Point{11000, 21000, 0}, Point{30000, 40000, 0}, true)
	if r.Point(1).H0 != 30000 {
		t.Fatal("FARPtogglecomparesbaseH0notownhome/H1")
	}
}
