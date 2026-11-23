package avionics

import (
	"encoding/binary"
	"encoding/hex"
	"gunship2000-assets/game/world"
	"gunship2000-assets/internal/testfixture"
	"os"
	"reflect"
	"testing"
)

func TestSourceCandidateHandBoundaries(t *testing.T) {
	for _, c := range []struct {
		a, b int16
		want uint16
	}{{3, 4, 5}, {-3, -4, 5}, {-32768, 2, 49154}, {100, 50, 125}} {
		if got := Metric(c.a, c.b); got != c.want {
			t.Fatalf("metric%+v got%d", c, got)
		}
	}
	for _, c := range []struct {
		height, speed     int16
		grey, night, mast bool
		want              uint16
	}{{100, 0, false, false, false, 1024}, {1024, 0, true, false, false, 4096}, {512, 0, false, true, false, 3072}, {100, 200, false, false, true, 4096}} {
		if got := Envelope(c.height, c.speed, c.grey, c.night, c.mast); got != c.want {
			t.Fatalf("envelope%+v got%d", c, got)
		}
	}
	s := New()
	s.Candidates[0] = Candidate{2, 0xfff, 0x1fff}
	s.Contacts[2] = Contact{Flags: 2, Kind: 0x17}
	s.Candidates[1] = Candidate{3, 0x1000, 0}
	s.Contacts[3].Flags = 2
	if got, _ := s.selectTarget(0); got != 3 {
		t.Fatalf("strict valid target %d", got)
	}
	s.Candidates[0].Angle = 0x2000
	if got, _ := s.selectTarget(0); got != 0 {
		t.Fatal("angle equality admitted")
	}
	s.Candidates[0].Angle = 0x1fff
	s.Contacts[2].Flags = 6
	if got, _ := s.selectTarget(0); got != 0 {
		t.Fatal("state6 treated exact2")
	}
	s.Contacts[2].Flags = 2
	s.Candidates[0].Angle = 0
	s.SelectManual()
	if s.Target != 3 {
		t.Fatal("first select")
	}
	s.SelectManual()
	if s.Target != 0 || s.Suppressed != 3 {
		t.Fatal("lone target notstandby")
	}
	s.SelectManual()
	if s.Target != 3 || s.Suppressed != 0 {
		t.Fatal("second press notreacquire")
	}
}
func contactTables(t *testing.T) *world.Bundle {
	t.Helper()
	b, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional source table fixture")
	}
	if err != nil {
		t.Fatal(err)
	}
	a := &world.Bundle{Sine: make([]int16, 1025), AngleBases: make([]uint16, 8), Atan: make([]uint16, 4096), Worlds: map[string]*world.World{}, Geometries: map[string]*world.Geometry{}}
	for i := range a.Sine {
		a.Sine[i] = int16(binary.LittleEndian.Uint16(b[0x4bc50+2*i:]))
	}
	for i := range a.AngleBases {
		a.AngleBases[i] = binary.LittleEndian.Uint16(b[0x4c452+2*i:])
	}
	for i := range a.Atan {
		a.Atan[i] = binary.LittleEndian.Uint16(b[0x4c462+2*i:])
	}
	return a
}
func contactFixture(t *testing.T, height int16) (Input, *int) {
	a := contactTables(t)
	a.Geometries["cell"] = &world.Geometry{Heights: []world.HeightRegion{{Points: [][2]int16{{-4096, -4096}, {-4096, 4096}, {4096, 4096}, {4096, -4096}, {-4096, -4096}}, Plane: world.Plane{0, 1, 0, -int32(height)}}}}
	cells := make([]world.Cell, 64)
	for i := range cells {
		cells[i].Geometry = "cell"
	}
	a.Worlds["TEST"] = &world.World{Grid: make([]byte, 4096), Cells: cells}
	count := 0
	in := Input{World: "TEST", Assets: a, Player: Pose{H0: 100000, H1: 100000, Height: 128}, Random: func() uint16 { count++; return uint16(count) }}
	in.Head = in.Player
	in.Head.Height += 128
	in.Entities[0] = Entity{Present: true, Row: 0, FlightSlot: 0, Pose: in.Player}
	in.Entities[1] = Entity{Present: true, Row: 1, FlightSlot: -1, Kind: 0x17, Participation: 1, Pose: Pose{H0: 100000, H1: 104096, Height: 128}}
	return in, &count
}
func TestSourceContactsClearBlockedAndSharedDraws(t *testing.T) {
	in, n := contactFixture(t, 0)
	s := New()
	if err := s.Advance(in); err != nil {
		t.Fatal(err)
	}
	if *n != 5 || s.TailDraws != [5]uint16{1, 2, 3, 4, 5} {
		t.Fatalf("shared draws %d%v", *n, s.TailDraws)
	}
	if s.Contacts[1].Flags&7 != 2 || s.Target != 2 {
		t.Fatalf("clear contact %v target%d", s.Contacts[1], s.Target)
	}
	if s.Contacts[1].Yaw != in.Entities[1].Pose.Angles[1] {
		t.Fatal("yaw was range")
	}
	in, n = contactFixture(t, 200)
	s = New()
	if err := s.Advance(in); err != nil {
		t.Fatal(err)
	}
	if s.Contacts[1].Flags&7 == 2 || s.Target != 0 || *n != 5 {
		t.Fatalf("blockedcontacttarget %v %d draws%d", s.Contacts[1], s.Target, *n)
	}
	in, _ = contactFixture(t, 128)
	s = New()
	if err := s.Advance(in); err != nil || s.Contacts[1].Flags&7 != 2 {
		t.Fatalf("plane equality blocks %v%v", s.Contacts[1], err)
	}
}
func TestPreviousCacheAndStableSourceOrdering(t *testing.T) {
	in, _ := contactFixture(t, 0)
	in.Entities[2] = in.Entities[1]
	in.Entities[2].Row = 2
	s := New()
	s.SelectManual()
	if s.Target != 0 {
		t.Fatal("new scan somehow selected beforelogicalupdate")
	}
	if err := s.Advance(in); err != nil {
		t.Fatal(err)
	}
	if s.Candidates[0].Row != 1 || s.Candidates[1].Row != 2 {
		t.Fatalf("tieunstable %v", s.Candidates)
	}
	before := s.Contacts
	in.Entities[1].Participation = 0
	in.Entities[1].Kind = 0x1c
	if err := s.Advance(in); err != nil {
		t.Fatal(err)
	}
	if s.Contacts[1].Kind != 0x1c || s.Contacts[1].Flags&7 != 1 || before[1].Flags&7 != 2 {
		t.Fatal("death snapshot notstale")
	}
}
func TestDescriptorEighthStepRetainsStatus(t *testing.T) {
	in, _ := contactFixture(t, -1)
	s := New()
	e := in.Entities[1]
	e.Pose.H1 = 120000
	d := Descriptor{Remaining: 8, Status: 1, Pose: in.Player}
	s.Contacts[1].Flags = 2
	result, err := s.queryBatch(&d, e, 16, in, 0)
	if err != nil || result != 0xffff || d.Remaining != 0 || d.Status != 1 {
		t.Fatalf("eighth %+v %x %v", d, result, err)
	}
	d = Descriptor{Remaining: 7, Status: 1, Pose: in.Player}
	result, err = s.queryBatch(&d, e, 16, in, 0)
	if err != nil || result != 0 || d.Status != 0 || s.Contacts[1].Flags&7 != 1 {
		t.Fatal("seventh exhaustion notclear")
	}
}
func TestObserverAliasAndGPSAdministrativeRefresh(t *testing.T) {
	in, _ := contactFixture(t, 0)
	in.Entities[13] = Entity{Present: true, Row: 13, FlightSlot: -1}
	in.Entities[13].Object[25] = 0xff
	if alive, known := ownerAlive(31, in); !known || !alive {
		t.Fatal("explicit OBJECT13+25 alias")
	}
	if _, known := ownerAlive(8, in); known {
		t.Fatal("invented SHIP8")
	}
	s := New()
	in.Entities[1].FlightSlot = 1
	s.GPSRefresh(in.Entities)
	if s.Contacts[1].Flags&7 != 2 || s.Contacts[2].Flags != 0 {
		t.Fatal("GPS all-world false detection")
	}
	before := *s
	s.MapPhase(in.Entities, false)
	if !reflect.DeepEqual(before, *s) {
		t.Fatal("inactive map transaction wrote contacts")
	}
}
func TestOriginalContactOperandsAndKeys(t *testing.T) {
	b, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional original byte witnesses")
	}
	if err != nil {
		t.Fatal(err)
	}
	for ip, wantHex := range map[int]string{0x3f15: "837ee608", 0x3fd8: "837ee850", 0x40e0: "817ef40020", 0x43f9: "9a1606db00", 0x4bdc: "9a38207f03", 0x4be7: "9a38207f03", 0x4bf2: "9a38207f03", 0x4bfd: "9a38207f03", 0x4c08: "9a38207f03"} {
		want, _ := hex.DecodeString(wantHex)
		if got := b[0xd8b0+ip:][:len(want)]; !reflect.DeepEqual(got, want) {
			t.Fatalf("source0975:%x", ip)
		}
	}
}
func TestThreatOnlyObservedRowsAndWeaponPredicates(t *testing.T) {
	a := contactTables(t)
	s := New()
	s.Contacts[2] = Contact{Flags: 2, Kind: 0x17, H0: 1000, H1: 1000}
	s.Contacts[3] = Contact{Flags: 0, Kind: 0x17, H0: 1000, H1: 1000}
	s.AdvanceThreat(Pose{H0: 1000, H1: 1000}, a, 0)
	if len(s.Threat.Points) != 1 || s.Threat.Points[0].Row != 2 || s.Threat.Points[0].Colour != 7 {
		t.Fatal("threat displayed unobserved or assigned genericIFF")
	}
	if s.Threat.WarningFlags != 0 {
		t.Fatal("statickind invented attack")
	}
	if err := s.SetAttackDefinition(2, [8]byte{0, 0, 0, 0, 0, 0x20}); err != nil {
		t.Fatal(err)
	}
	s.AdvanceThreat(Pose{H0: 1000, H1: 1000}, a, 0, func(ThreatPlot) bool { return true })
	if s.Threat.WarningFlags != 0x2000 {
		t.Fatal("actualattackflag lost")
	}
	if got := TargetSymbol(true, 0, 0, 10, 11, 1, 7, 0); got != 1 {
		t.Fatal(got)
	}
	if got := TargetSymbol(true, 0, 0, 11, 11, 1, 7, 0); got != 2 {
		t.Fatal("range equality", got)
	}
	if got := TargetSymbol(true, 5, 0, 10, 11, 1, 7, 0); got != 3 {
		t.Fatal("gun angular equality", got)
	}
	if got := TargetSymbol(true, 0, 0, 0, 10, 0x40|0x800, 7, 0); got != 3 {
		t.Fatal("radar guard ignored", got)
	}
}

func TestTargetZoomClippingOwnership(t *testing.T) {
	s := New()
	s.TargetCache.Zoom = 1024
	s.AdvanceTargetZoom(8, false, 2)
	if s.TargetCache.Zoom != 1024 {
		t.Fatal("horizontal sourceFB didnotstop model zoom")
	}
	s.AdvanceTargetZoom(8, false, 0)
	if s.TargetCache.Zoom != 896 {
		t.Fatal("source SAR3 zoom")
	}
	s.TargetCache.Zoom = 64
	s.AdvanceTargetZoom(8, true, 0)
	if s.TargetCache.Zoom != 64 {
		t.Fatal("extent boundary shrankworldcamera")
	}
}

func TestSelectedForwardEqualityAndMastRange(t *testing.T) {
	for _, tc := range []struct {
		yaw  uint16
		want uint16
	}{{0x2000, 2}, {0x2001, 0}} {
		in, _ := contactFixture(t, 0)
		in.Player.Angles[1] = tc.yaw
		s := New()
		s.Target = 2
		if err := s.Advance(in); err != nil {
			t.Fatal(err)
		}
		if s.Target != tc.want {
			t.Fatalf("selected forward gate equality yaw%x got%d", tc.yaw, s.Target)
		}
	}
	in, _ := contactFixture(t, 0)
	in.Mast = true
	in.Entities[1].Pose.H1 = 120000
	s := New()
	s.Target = 2
	if err := s.Advance(in); err != nil {
		t.Fatal(err)
	}
	if s.Target != 2 || s.BodyQuery.Status != 0 || s.HeadQuery.Status != 1 || s.Contacts[1].Flags>>3 != 31 {
		t.Fatalf("mast bodyrange/headray %+v %+v target%d flags%x", s.BodyQuery, s.HeadQuery, s.Target, s.Contacts[1].Flags)
	}
}
