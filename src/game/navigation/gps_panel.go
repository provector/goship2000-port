package navigation

import (
	"encoding/binary"
	"fmt"
	"gunship2000-assets/game/cockpit"
	"strings"
)

// PanelSnapshot owns copies of the selected actor's source carriers. The modal
// publishes a replacement after input, so Draw never consults mutable mission,
// SHIP or route state. Selected actor and player identity are separate domains.
type PanelSnapshot struct {
	Selected            int
	Player              byte
	LeaderA, LeaderB    int8 // DATA5a5/55f: Control0488 + 4/3*70 + 5.
	Callsign            string
	Control             [70]byte
	Height              int16
	ShipFlags           byte // Serialized SHIP byte0, including the retained upper hold bits.
	Fuel, Chaff, Flares uint16
	Stations            [4]PanelStation
	FuelSeverity        byte // DATA5fe low two bits; separate from physical600..609.
	FuelSeverityKnown   bool
	Damage              [10]byte // Source5fe..607;608 overlay/609 terminal are separate.
	DamageKnown         bool
	ScalarDamageKnown   bool // physical600..607 known, masks5fe/5ff remain separate.
	Objectives          [2]PanelObjective
}

type PanelStation struct {
	SystemID byte // 0 no bay;1 EMPTY; other compact IDs carry unsigned quantity.
	Name     string
	Quantity uint16
}

// PanelObjective preserves all62 bytes, including uninterpreted TARGET tails.
// The GPS text pointer is +6 (after the grid prefix), while +32 is the current
// source TARGET kind/status, not the frontend MissionObjective.RawKind enum.
type PanelObjective struct {
	Available bool
	Raw       [62]byte
}

func (p PanelSnapshot) HeaderInk() byte {
	if int16(p.LeaderA) == int16(p.Player) {
		return 9
	}
	if int16(p.LeaderB) == int16(p.Player) {
		if p.Selected < 3 {
			return 9
		}
		return 7
	}
	if p.Selected >= 2 {
		return 9
	}
	return 7
}

func (p PanelSnapshot) ordersLiteral() string {
	if p.Control[1] == 2 && p.Height < 2 { // signed JL boundary at4:ae57.
		return "gps-landed"
	}
	return fmt.Sprintf("gps-mode-%d", int8(p.Control[1]))
}

func drawPanel(dst []byte, a *cockpit.Assets, p *PanelSnapshot) error {
	if p == nil {
		return nil
	}
	panel := cockpit.Rect{Left: 232, Top: 72, Right: 316, Bottom: 163}
	for y := panel.Top; y <= panel.Bottom; y++ {
		for x := panel.Left; x <= panel.Right; x++ {
			ink := byte(8)
			if y <= 78 {
				ink = p.HeaderInk()
			}
			dst[y*320+x] = ink
		}
	}
	text := func(s string, x, y int, ink byte) error { return a.Text(dst, s, x, y, ink, 2, panel) }
	literal := func(name string, x, y int, ink byte) error {
		s, ok := a.Literals[name]
		if !ok {
			return fmt.Errorf("GPS prepared literal %s missing; rebuild supported assets", name)
		}
		return text(s, x, y, ink)
	}
	if err := text(p.Callsign, 233, 73, 15); err != nil {
		return err
	}
	for _, row := range []struct {
		name string
		x, y int
		ink  byte
	}{
		{"gps-orders", 233, 80, 2}, {p.ordersLiteral(), 267, 80, 10},
		{fmt.Sprintf("gps-aircraft-%d", p.ShipFlags&7), 276, 86, 2},
		{"gps-fuel", 238, 94, 2},
	} {
		if err := literal(row.name, row.x, row.y, row.ink); err != nil {
			return err
		}
	}
	// AEDC's label precedes the severity guard; only the numeric value is
	// suppressed. Unknown nonplayer damage is not inferred from player masks.
	if !p.FuelSeverityKnown || p.FuelSeverity&3 != 2 {
		if err := text(fmt.Sprintf("%d%%", p.Fuel), 280, 94, 10); err != nil {
			return err
		}
	}
	y := 100
	for _, st := range p.Stations {
		if st.SystemID == 0 {
			continue
		}
		if err := text(st.Name, 238, y, 2); err != nil {
			return err
		}
		if st.SystemID != 1 {
			if err := text(fmt.Sprintf("%d", st.Quantity), 280, y, 10); err != nil {
				return err
			}
		}
		y += 6
	}
	for _, row := range []struct {
		name  string
		value uint16
	}{{"gps-chaff", p.Chaff}, {"gps-flares", p.Flares}} {
		if err := literal(row.name, 238, y, 2); err != nil {
			return err
		}
		if err := text(fmt.Sprintf("%d", row.value), 280, y, 10); err != nil {
			return err
		}
		y += 6
	}
	if p.ShipFlags&7 == 6 {
		hold := p.ShipFlags >> 3
		if hold > 3 {
			return fmt.Errorf("GPS Blackhawk hold selector%d outside established0..3", hold)
		}
		if err := literal("gps-hold-label", 234, 130, 2); err != nil {
			return err
		}
		if err := literal(fmt.Sprintf("gps-hold-%d", hold), 260, 130, 10); err != nil {
			return err
		}
	}
	if p.Selected == int(p.Player) {
		for i, o := range p.Objectives {
			if !o.Available {
				continue
			}
			vectors, ok := a.MapVectors[[]string{"map-075a", "map-0722"}[i]]
			if !ok {
				return fmt.Errorf("GPS prepared objective vector missing; rebuild supported assets")
			}
			y := 139 + i*14
			// Source style0308: shift8 and (+1,+1) black pass, followed by
			// the source literal colours. Keep vector and text anchors distinct.
			for _, v := range vectors {
				v.Colour = 0
				a.DrawVectors(dst, []cockpit.Vector{v}, 238, y+1, 0, 8, 0, cockpit.Full)
			}
			a.DrawVectors(dst, vectors, 237, y, 0, 8, 0, cockpit.Full)
			if binary.LittleEndian.Uint16(o.Raw[32:]) == 0 {
				if err := literal("gps-achieved", 244, y-1, 7); err != nil {
					return err
				}
			} else {
				name := strings.SplitN(string(o.Raw[6:32]), "\x00", 2)[0]
				// TARGET stores the compact name and route endpoint separately.
				// Use the same nonzero, distinct endpoint as the paper orders.
				ex, ez := binary.LittleEndian.Uint32(o.Raw[54:]), binary.LittleEndian.Uint32(o.Raw[58:])
				if ex != 0 && (ex != binary.LittleEndian.Uint32(o.Raw[42:]) || ez != binary.LittleEndian.Uint32(o.Raw[46:])) && !strings.HasSuffix(strings.ToLower(strings.TrimSpace(name)), " moving") {
					name = strings.TrimSpace(name) + " moving"
				}
				// Original text window starts234; wrapped continuation lines
				// return there after the initial x244 anchor.
				wrap := cockpit.Rect{Left: 234, Top: 72, Right: 316, Bottom: 163}
				if err := a.Text(dst, name, 244, y-1, 7, 0x102, wrap); err != nil {
					return err
				}
			}
		}
		return nil
	}
	weapons := "gps-weapons-hold"
	if p.Control[4] != 0 {
		weapons = "gps-weapons-free"
	}
	if err := literal(weapons, 234, 140, 10); err != nil {
		return err
	}
	altitude, speed := binary.LittleEndian.Uint16(p.Control[6:]), binary.LittleEndian.Uint16(p.Control[8:])
	if altitude == 0 || speed == 0 {
		return nil
	}
	// Source table indices are bounded; adjacent literal LOW is not a speed
	// pointer. Do not manufacture labels for unsupported supplied control rows.
	if altitude > 3 || speed > 4 {
		return fmt.Errorf("GPS retained altitude/speed selectors%d/%d outside established1..3/1..4", altitude, speed)
	}
	labels, ok := a.Literals["gps-altitude-speed"]
	if !ok {
		return fmt.Errorf("GPS prepared altitude/speed literal missing; rebuild supported assets")
	}
	if err := a.Text(dst, labels, 234, 148, 2, 2, cockpit.Rect{Left: 234, Top: 72, Right: 316, Bottom: 163}); err != nil {
		return err
	}
	if err := literal(fmt.Sprintf("gps-altitude-%d", altitude), 269, 148, 10); err != nil {
		return err
	}
	return literal(fmt.Sprintf("gps-speed-%d", speed), 269, 155, 10)
}
