package navigation

import "encoding/binary"

// OrderEffects are source DYN writes accompanying CONTROL70. A zero Kind does
// not mean kind0: ChangeKind distinguishes orders which change the controller.
type OrderEffects struct {
	Kind                                  byte
	ChangeKind, ResetPhase, ClearCooldown bool
}

// ApplyOrder implements GS2 resident 0E88 and GPS4:14DA..18DA. Only W
// propagates a write to later mode1 children; other section behavior follows
// their unchanged parent and controller in the simulation, rather than copying
// the leader's independent CONTROL row into each child.
func ApplyOrder(routes *[5]Route, active [5]bool, slot int, key string, base, home, farp Point, farpPresent bool) [5]OrderEffects {
	return applyOrder(routes, active, slot, key, base, home, farp, farpPresent, false)
}

// ApplyGPSOrder keeps GPS1756's zero-prior hold restoration distinct from
// resident10DA, which explicitly guards it. Other order writers share operands.
func ApplyGPSOrder(routes *[5]Route, active [5]bool, slot int, key string, base, home, farp Point, farpPresent bool) [5]OrderEffects {
	return applyOrder(routes, active, slot, key, base, home, farp, farpPresent, true)
}

func applyOrder(routes *[5]Route, active [5]bool, slot int, key string, base, home, farp Point, farpPresent, gps bool) [5]OrderEffects {
	var out [5]OrderEffects
	if routes == nil || slot < 0 || slot >= 5 || !active[slot] || routes[slot].Raw[1] == 0 {
		return out
	}
	r := &routes[slot]
	word := func(at int) {
		n := int16(binary.LittleEndian.Uint16(r.Raw[at:]))
		n++
		if n > 3 {
			n = 1
		}
		binary.LittleEndian.PutUint16(r.Raw[at:], uint16(n))
	}
	defaults := func() { binary.LittleEndian.PutUint16(r.Raw[6:], 1); binary.LittleEndian.PutUint16(r.Raw[8:], 3) }
	kind := func(k byte, reset bool) {
		out[slot].Kind = k
		out[slot].ChangeKind = true
		out[slot].ResetPhase = reset
	}
	switch key {
	case "s":
		word(8)
	case "a":
		word(6)
	case "w":
		r.Raw[4] ^= 1
		out[slot].ClearCooldown = true
		for i := slot + 1; i < 5; i++ {
			if routes[i].Raw[1] == 1 && int(int8(routes[i].Raw[5])) == slot {
				routes[i].Raw[4] ^= 1
				out[i].ClearCooldown = true
			}
		}
	case "h":
		if r.Raw[1] != 2 {
			r.Raw[2] = r.Raw[1]
			r.Raw[1] = 2
			kind(0x20, true)
		} else {
			prior := r.Raw[2]
			if prior != 0 || gps {
				r.Raw[1] = prior
				if prior >= 1 && prior <= 8 {
					kind([8]byte{0x24, 0x20, 0x23, 0x24, 0x25, 0x26, 0x26, 0x22}[prior-1], true)
					if prior == 1 {
						r.Raw[1] = 4
						binary.LittleEndian.PutUint16(r.Raw[8:], 3)
					}
				}
			}
			out[slot].ResetPhase = true
		}
	case "l":
		r.Raw[1] = 6
		r.SetIndex(0)
		r.ClearPoints()
		kind(0x26, false)
	case "d":
		if r.Raw[1] != 1 {
			defaults()
			r.Raw[1] = 5
			r.SetIndex(0)
			r.ClearPoints()
			kind(0x25, true)
		}
	case "g":
		parent := int(int8(r.Raw[5]))
		if r.Raw[1] != 1 && parent >= 0 && parent < 5 && routes[parent].Raw[1] != 0 {
			defaults()
			r.Raw[1] = 4
			r.SetIndex(0)
			r.ClearPoints()
			kind(0x24, false)
		}
	case "b":
		r.HomeFrom(base, home, farp, farpPresent)
		if slot != 0 {
			defaults()
			r.Raw[1] = 8
			kind(0x22, true)
		}
	}
	return out
}

// SetFollowerOrders is a launch-profile policy. Historical GPS retains its
// read-only follower behavior; it is never inferred from current SETUP state.
func (g *GPS) SetFollowerOrders(on bool, finish func(int)) {
	g.followerOrders = on
	g.routeFinished = finish
}
