package navigation

import (
	"gunship2000-assets/game/cockpit"
	"strings"
)

// The source b2de..b50d uses different command strings in overview/detail and
// player/follower contexts. Disabled commands replace only ink0a with20h;
// selected command tags replace0a with0f. These are indexed operations.
type legendRegion struct {
	x, y, width, height int
	old, next           byte
}

func (g *GPS) legend() (string, []legendRegion) {
	name := "gps-controls-overview"
	if g.Low {
		name = "gps-controls"
	}
	if g.Selected != 0 {
		name = "gps-controls-follower-overview"
		if g.Low {
			name = "gps-controls-follower-detail"
		}
	}
	regions := []legendRegion{}
	mark := func(x, y, w, h int, ink byte) { regions = append(regions, legendRegion{x, y, w, h, 10, ink}) }
	next := false
	for i, active := range g.Active {
		if active && i != g.Selected {
			next = true
		}
	}
	if !next {
		mark(50, 172, 32, 6, 0x20)
	}
	if g.Selected != 0 && (!g.followerOrders || !g.r11Presentation) {
		// Historical profiles retain read-only followers. Keep N
		// inspection available, without advertising fake route/order commands.
		mark(87, 172, 230, 6, 0x20)
		mark(4, 178, 313, 6, 0x20)
		return name, regions
	}
	if g.Selected != 0 {
		if g.Editing {
			mark(87, 172, 10, 5, 15)
		}
		if !g.Low && !g.CargoReleaseAvailable {
			mark(171, 172, 55, 6, 0x20)
		}
		if g.Low && g.Routes[g.Selected].Raw[1] == 8 {
			mark(4, 178, 264, 6, 0x20)
		}
		return name, regions
	}
	if g.Editing {
		mark(87, 172, 10, 5, 15)
	}
	if g.Low {
		if !g.SupportAvailable() {
			mark(135, 172, 75, 5, 0x20)
		} else if g.Support.Editing {
			mark(135, 172, 12, 5, 15)
		}
	} else {
		if g.Editing || g.HomeError != "" {
			mark(135, 172, 32, 6, 0x20)
		}
		// Source b4d9..b50b changes only the C region171..225.
		if !g.CargoReleaseAvailable {
			mark(171, 172, 55, 6, 0x20)
		}
		// Weapons-free decisions remain outside the player mechanism.
		if !g.followerOrders || (g.r11Presentation && !g.CommandAvailable("w")) {
			mark(226, 172, 91, 6, 0x20)
		}
		if !g.SupportAvailable() {
			mark(4, 178, 75, 5, 0x20)
		} else if g.Support.Editing {
			mark(4, 178, 12, 5, 15)
		}
	}
	return name, regions
}

func (g *GPS) drawLegend(dst []byte, a *cockpit.Assets, clip cockpit.Rect) error {
	name, regions := g.legend()
	// The installed text window starts at4. Its first line is explicitly
	// positioned at50, but LF returns to the window's left edge, not50.
	for n, line := range strings.Split(a.Literals[name], "\n") {
		x := 50
		if n > 0 {
			x = 4
		}
		if err := a.Text(dst, line, x, 172+n*6, 10, 2, clip); err != nil {
			return err
		}
	}
	// Source B377/B392 and B44D/B466 add a source-selected type/count
	// after the Mission Assets caption. The artillery count is the entry AA4
	// high nibble, rendered by the source's literal ASCII addition, not a
	// separate battery count or regenerated mission descriptor.
	if g.SupportAvailable() {
		key := "gps-support-aircraft"
		if g.Support.Availability&15 == 1 {
			key = "gps-support-artillery"
		}
		if caption := a.Literals[key]; caption != "" {
			if key == "gps-support-artillery" && len(caption) == 15 {
				b := []byte(caption)
				b[14] = '0' + g.Support.Availability>>4
				caption = string(b)
			}
			x, y := 76, 178
			if g.Low {
				x, y = 207, 172
			}
			if err := a.Text(dst, caption, x, y, 2, 2, clip); err != nil {
				return err
			}
		}
	}
	if g.Low && g.Selected == 0 {
		// Original numbered-point commands are visible even for absent points.
		// The native unavailable treatment follows the real H0/edit/mode gates,
		// using source font advances rather than guessed equal-width columns.
		lines := strings.Split(a.Literals[name], "\n")
		if len(lines) > 1 {
			line := lines[1]
			for n := 1; n <= 3; n++ {
				tag := "[" + string(byte('0'+n)) + "]"
				at := strings.Index(line, tag)
				if at < 0 {
					continue
				}
				end := len(line)
				if next := strings.Index(line[at+len(tag):], "  ["); next >= 0 {
					end = at + len(tag) + next
				}
				x := 4 + gpsTextWidth(a, line[:at])
				if g.Editing || g.Routes[0].Raw[1] == 8 || !g.Routes[0].Point(n).Present() {
					regions = append(regions, legendRegion{x, 178, gpsTextWidth(a, line[at:end]), 6, 10, 0x20})
				} else if g.Moving == n {
					regions = append(regions, legendRegion{x, 178, gpsTextWidth(a, tag), 5, 10, 15})
				}
			}
		}
	}
	// R1 implemented the handlers but left the earlier blanket grey follower
	// legend. Resolve each visible tag using actual modal/CONTROL eligibility.
	if g.followerOrders && g.r11Presentation {
		for lineNo, line := range strings.Split(a.Literals[name], "\n") {
			x := 50
			if lineNo > 0 {
				x = 4
			}
			for at := 0; at < len(line); at++ {
				if line[at] != '[' || at+2 >= len(line) || line[at+2] != ']' {
					continue
				}
				key := strings.ToLower(line[at+1 : at+2])
				if !strings.Contains("nfbcwhgsaldm", key) || g.CommandAvailable(key) {
					continue
				}
				end := len(line)
				if n := strings.Index(line[at+3:], "["); n >= 0 {
					end = at + 3 + n
				}
				regions = append(regions, legendRegion{x + gpsTextWidth(a, line[:at]), 172 + lineNo*6, gpsTextWidth(a, line[at:end]), 6, 10, 0x20})
			}
		}
	}
	for _, r := range regions {
		for y := r.y; y < r.y+r.height && y < 200; y++ {
			for x := r.x; x < r.x+r.width && x < 320; x++ {
				if dst[y*320+x] == r.old {
					dst[y*320+x] = r.next
				}
			}
		}
	}
	return nil
}

func gpsTextWidth(a *cockpit.Assets, text string) int {
	w := 0
	for _, ch := range []byte(text) {
		i := int(ch) - a.Font.First
		if i >= 0 && i < len(a.Font.Advances) {
			w += int(a.Font.Advances[i])
		}
	}
	return w
}
