package navigation

import "strings"

// SetR11Presentation preserves historical GPS image interpretation.
func (g *GPS) SetR11Presentation(enabled bool) { g.r11Presentation = enabled }

// CommandAvailable shares the reached modal and CONTROL eligibility with the
// displayed command ink. It is observational; it never changes routes or RNG.
func (g *GPS) CommandAvailable(key string) bool {
	if g == nil || g.Routes == nil || g.Selected < 0 || g.Selected >= 5 {
		return false
	}
	support := g.Support != nil && g.Support.Editing
	key = strings.ToLower(key)
	if key == "n" {
		if g.Editing || support {
			return false
		}
		for i, on := range g.Active {
			if on && i != g.Selected {
				return true
			}
		}
		return false
	}
	if key == "f" {
		return !support && (g.Selected == 0 || g.followerOrders)
	}
	if g.Selected == 0 {
		switch key {
		case "c":
			return !g.Low && g.CargoReleaseAvailable
		case "b":
			return !g.Low && !g.Editing && !support && g.HomeError == ""
		case "m":
			return g.SupportAvailable()
		case "w":
			return g.followerOrders && !g.Low && !g.Editing && !support
		}
		return false
	}
	if !g.followerOrders || !g.Active[g.Selected] || g.Routes[g.Selected].Raw[1] == 0 || g.Low || g.Editing || support {
		return false
	}
	switch key {
	case "c":
		return g.CargoReleaseAvailable
	case "h", "s", "a", "l", "b", "w":
		return true
	case "d":
		return g.Routes[g.Selected].Raw[1] != 1
	case "g":
		p := int(int8(g.Routes[g.Selected].Raw[5]))
		return g.Routes[g.Selected].Raw[1] != 1 && p >= 0 && p < 5 && g.Routes[p].Raw[1] != 0
	}
	return false
}
