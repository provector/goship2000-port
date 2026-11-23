package navigation

import (
	"bytes"
	"encoding/binary"
	"gunship2000-assets/game/cockpit"
	"testing"
)

// Independent source operands: section4 ADAB..AE8B compares signed leader
// bytes against unsigned player593c, then selected actor against3 or2.
func TestGPSHeaderPlayerAndLeaderOperands(t *testing.T) {
	for _, tc := range []struct {
		player   byte
		a, b     int8
		selected int
		ink      byte
	}{
		{0, 0, -1, 0, 9}, {1, 1, -1, 0, 9}, {1, 1, -1, 3, 9}, {1, -1, 1, 0, 9},
		{1, -1, 1, 2, 9}, {1, -1, 1, 3, 7}, {1, -1, -1, 0, 7},
		{1, -1, -1, 1, 7}, {1, -1, -1, 2, 9}, {255, -1, -1, 0, 7},
	} {
		p := PanelSnapshot{Player: tc.player, LeaderA: tc.a, LeaderB: tc.b, Selected: tc.selected}
		if got := p.HeaderInk(); got != tc.ink {
			t.Fatalf("player%d leaders%d/%d selected%d: ink%d want%d", tc.player, tc.a, tc.b, tc.selected, got, tc.ink)
		}
	}
}

// This diagnostic font encodes each ASCII byte in four pairs of pixels. The
// assertions below read those pixels directly, independently of Text/DrawPanel.
func panelAssets() *cockpit.Assets {
	a := &cockpit.Assets{Font: cockpit.Font{First: 32, Last: 126, Width: 2, Height: 6, Advances: make([]byte, 95), Rows: make([]byte, 95*6)},
		Literals: map[string]string{
			"gps-orders": "ORDERS:", "gps-mode-0": "formed up", "gps-mode-1": "formed up", "gps-mode-2": "holding", "gps-mode-3": "fly to", "gps-landed": "landed",
			"gps-aircraft-0": "Apache", "gps-aircraft-6": "Blackhawk", "gps-fuel": "FUEL:", "gps-chaff": "CHAFF:", "gps-flares": "FLARES:",
			"gps-hold-label": "HOLD: ", "gps-hold-0": "Empty", "gps-hold-1": "Cargo", "gps-hold-2": "Troops", "gps-hold-3": "Occupied", "gps-achieved": "achieved!",
			"gps-weapons-free": "WEAPONS FREE", "gps-weapons-hold": "WEAPONS HOLD", "gps-altitude-speed": "altitude:\nspeed:\n",
			"gps-altitude-1": "NOE", "gps-altitude-2": "CONTOUR", "gps-altitude-3": "LOW", "gps-speed-1": "SLOW", "gps-speed-2": "MEDIUM", "gps-speed-3": "HIGH", "gps-speed-4": "HIGH",
		}, Sine: make([]int16, 1025), MapVectors: map[string][]cockpit.Vector{
			// Original resident015b:075a/0722 artwork; source4: B15C/B1AC
			// pushes y139/153,x237,style0308, not the text anchors244,138/152.
			"map-075a": {{Colour: 15, Points: [][2]int16{{-1, -2}, {-1, 2}, {1, 2}, {1, 0}, {-1, 0}}}},
			"map-0722": {{Colour: 15, Points: [][2]int16{{-1, -2}, {1, -2}, {1, 0}, {-1, 0}, {-1, 2}, {1, 2}}}},
		}}
	a.Sine[1024] = 16384
	for c := 32; c <= 126; c++ {
		i := c - 32
		a.Font.Advances[i] = 2
		a.Font.Rows[i*6] = 0x80
		for y := 1; y <= 4; y++ {
			for x := 0; x < 2; x++ {
				if c&(1<<(2*(y-1)+x)) != 0 {
					a.Font.Rows[i*6+y] |= 0x80 >> x
				}
			}
		}
	}
	return a
}

func assertPanelText(t *testing.T, pixels []byte, text string, x, y int, ink byte) {
	t.Helper()
	for i, c := range []byte(text) {
		if pixels[y*320+x+i*2] != ink {
			t.Fatalf("%q missing anchor x%d,y%d", text, x+i*2, y)
		}
		for row := 1; row <= 4; row++ {
			for col := 0; col < 2; col++ {
				got := pixels[(y+row)*320+x+i*2+col]
				want := c&(1<<(2*(row-1)+col)) != 0
				if (got == ink) != want {
					t.Fatalf("%q byte%d pixel%d,%d got%d expected ink%t", text, i, x+i*2+col, y+row, got, want)
				}
			}
		}
	}
}

func TestGPSPlayerTargetsAnchorsAndUnsignedCounts(t *testing.T) {
	a := panelAssets()
	p := PanelSnapshot{Callsign: "ZERO", LeaderA: 0, Fuel: 65535, Chaff: 65535, Flares: 2,
		Stations: [4]PanelStation{{SystemID: 0, Name: "NO BAY", Quantity: 12}, {SystemID: 1, Name: "EMPTY", Quantity: 77}, {SystemID: 2, Name: "GUN", Quantity: 65535}}}
	copy(p.Objectives[0].Raw[:], "04/06 Destroy Tanks")
	copy(p.Objectives[1].Raw[:], "99/99 retained ignored")
	p.Objectives[0].Available, p.Objectives[1].Available = true, true
	binary.LittleEndian.PutUint16(p.Objectives[0].Raw[32:], 3)
	p.Objectives[0].Raw[61] = 0x9b
	before := p
	dst := bytes.Repeat([]byte{44}, 64000)
	if err := drawPanel(dst, a, &p); err != nil {
		t.Fatal(err)
	}
	if p != before {
		t.Fatal("draw mutated TARGET/control/SHIP snapshot")
	}
	if dst[72*320+232] != 9 || dst[78*320+316] != 9 || dst[79*320+316] != 8 || dst[71*320+232] != 44 || dst[164*320+232] != 44 {
		t.Fatal("source panel/header bounds")
	}
	assertPanelText(t, dst, "ZERO", 233, 73, 15)
	assertPanelText(t, dst, "65535%", 280, 94, 10)
	assertPanelText(t, dst, "EMPTY", 238, 100, 2)
	if dst[100*320+280] != 8 {
		t.Fatal("EMPTY count or no-bay row fabricated")
	}
	assertPanelText(t, dst, "GUN", 238, 106, 2)
	assertPanelText(t, dst, "65535", 280, 106, 10)
	assertPanelText(t, dst, "65535", 280, 112, 10)
	assertPanelText(t, dst, "2", 280, 118, 10)
	assertPanelText(t, dst, "Destroy Tanks", 244, 138, 7)
	assertPanelText(t, dst, "achieved!", 244, 152, 7)
	// Independent P/S raster witnesses, including black shadow at(+1,+1).
	for _, q := range []struct {
		x, y int
		ink  byte
	}{{236, 137, 15}, {238, 137, 15}, {236, 141, 15}, {239, 138, 0}, {236, 151, 15}, {238, 155, 15}, {239, 156, 0}} {
		if dst[q.y*320+q.x] != q.ink {
			t.Fatalf("source objective glyph %d,%d got%d want%d", q.x, q.y, dst[q.y*320+q.x], q.ink)
		}
	}
	// Both raw kind branches belong to the same immutable TARGET carrier.
	binary.LittleEndian.PutUint16(p.Objectives[0].Raw[32:], 0)
	binary.LittleEndian.PutUint16(p.Objectives[1].Raw[32:], 7)
	if err := drawPanel(dst, a, &p); err != nil {
		t.Fatal(err)
	}
	assertPanelText(t, dst, "achieved!", 244, 138, 7)
	assertPanelText(t, dst, "retained ignored", 244, 152, 7)
}

func TestGPSNonplayerRetainedControlAndGuards(t *testing.T) {
	a := panelAssets()
	for _, tc := range []struct {
		altitude, speed uint16
		free            byte
		alt, velocity   string
	}{
		{1, 1, 0, "NOE", "SLOW"}, {2, 2, 1, "CONTOUR", "MEDIUM"}, {3, 3, 1, "LOW", "HIGH"}, {3, 4, 255, "LOW", "HIGH"},
		{0, 2, 0, "", ""}, {2, 0, 1, "", ""},
	} {
		p := PanelSnapshot{Player: 1, Selected: 0, LeaderA: -1, LeaderB: -1}
		p.Objectives[0].Available = true
		p.Control[4] = tc.free
		binary.LittleEndian.PutUint16(p.Control[6:], tc.altitude)
		binary.LittleEndian.PutUint16(p.Control[8:], tc.speed)
		dst := make([]byte, 64000)
		if err := drawPanel(dst, a, &p); err != nil {
			t.Fatal(err)
		}
		weapons := "WEAPONS HOLD"
		if tc.free != 0 {
			weapons = "WEAPONS FREE"
		}
		assertPanelText(t, dst, weapons, 234, 140, 10)
		if tc.alt == "" {
			if dst[148*320+234] != 8 || dst[148*320+269] != 8 || dst[155*320+269] != 8 {
				t.Fatal("zero selector must suppress both orders lines")
			}
		} else {
			assertPanelText(t, dst, "altitude:", 234, 148, 2)
			assertPanelText(t, dst, tc.alt, 269, 148, 10)
			assertPanelText(t, dst, tc.velocity, 269, 155, 10)
		}
		if dst[137*320+236] != 8 {
			t.Fatal("selected nonplayer inherited player P glyph")
		}
	}
	for _, bad := range [][2]uint16{{4, 1}, {1, 5}} {
		p := PanelSnapshot{Player: 1}
		binary.LittleEndian.PutUint16(p.Control[6:], bad[0])
		binary.LittleEndian.PutUint16(p.Control[8:], bad[1])
		if drawPanel(make([]byte, 64000), a, &p) == nil {
			t.Fatal("unproven table extension admitted", bad)
		}
	}
}

func TestGPSLandedHoldAndSuppliedSeverity(t *testing.T) {
	a := panelAssets()
	for _, tc := range []struct {
		mode   byte
		height int16
		word   string
	}{{2, 1, "landed"}, {2, 2, "holding"}, {2, -1, "landed"}, {1, 1, "formed up"}} {
		p := PanelSnapshot{Height: tc.height}
		p.Control[1] = tc.mode
		dst := make([]byte, 64000)
		if err := drawPanel(dst, a, &p); err != nil {
			t.Fatal(err)
		}
		assertPanelText(t, dst, tc.word, 267, 80, 10)
	}
	for hold, word := range []string{"Empty", "Cargo", "Troops", "Occupied"} {
		p := PanelSnapshot{ShipFlags: byte(6 | hold<<3)}
		dst := make([]byte, 64000)
		if err := drawPanel(dst, a, &p); err != nil {
			t.Fatal(err)
		}
		assertPanelText(t, dst, "HOLD: ", 234, 130, 2)
		assertPanelText(t, dst, word, 260, 130, 10)
	}
	if drawPanel(make([]byte, 64000), a, &PanelSnapshot{ShipFlags: 6 | 4<<3}) == nil {
		t.Fatal("unsupported upper flags became cargo labels")
	}
	// Labelled supplied DATA5fe severities: this is a consumer check, not an
	// incoming-hit simulation or proof of broader damage schematic production.
	for _, known := range []bool{false, true} {
		for severity := byte(0); severity < 8; severity++ {
			p := PanelSnapshot{Fuel: 37, FuelSeverity: severity, FuelSeverityKnown: known}
			dst := make([]byte, 64000)
			if err := drawPanel(dst, a, &p); err != nil {
				t.Fatal(err)
			}
			assertPanelText(t, dst, "FUEL:", 238, 94, 2)
			if known && severity&3 == 2 {
				if dst[94*320+280] != 8 {
					t.Fatal("severity2 fuel shown")
				}
			} else {
				assertPanelText(t, dst, "37%", 280, 94, 10)
			}
		}
	}
}

func TestMovingObjectivePanelPixels(t *testing.T) {
	a := panelAssets()
	// Three pixels per character forces the moving suffix onto line two.
	for i := range a.Font.Advances {
		a.Font.Advances[i] = 3
	}
	p := PanelSnapshot{}
	for i := range p.Objectives {
		o := &p.Objectives[i]
		o.Available = true
		copy(o.Raw[6:32], "Destroy Heavy Tank Platoon")
		binary.LittleEndian.PutUint16(o.Raw[32:], 3)
		binary.LittleEndian.PutUint32(o.Raw[42:], 1000)
		binary.LittleEndian.PutUint32(o.Raw[54:], 1000+uint32(i))
	}
	dst := make([]byte, 64000)
	if err := drawPanel(dst, a, &p); err != nil {
		t.Fatal(err)
	}
	// First objective is stationary, second has a distinct endpoint.
	for i, want := range []string{"Platoon", "Platoon moving"} {
		y := 144 + 14*i
		for j, c := range want {
			assertPanelText(t, dst, string(c), 234+3*j, y, 7)
		}
		if dst[y*320+234+3*len(want)] == 7 {
			t.Fatal("unexpected extra text", i)
		}
	}
}
