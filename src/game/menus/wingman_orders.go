package menus

import (
	"encoding/binary"
	"fmt"
	"gunship2000-assets/game/diagnostics"
	"gunship2000-assets/game/navigation"
	"strings"
)

const wingmanOrderKeys = "hlbdgwsa"
const wingmanOrderAuxBase = 0x100

func encodeWingmanOrder(slot int, key string) (int, bool) {
	i := strings.Index(wingmanOrderKeys, strings.ToLower(key))
	if slot < 1 || slot > 4 || len(key) != 1 || i < 0 {
		return 0, false
	}
	return wingmanOrderAuxBase + slot*8 + i, true
}
func decodeWingmanOrder(code int) (int, string, bool) {
	n := code - wingmanOrderAuxBase
	slot := n / 8
	if n < 8 || slot > 4 {
		return 0, "", false
	}
	return slot, string(wingmanOrderKeys[n%8]), true
}

// Publish only actual assigned CONTROL rows. This owner is called after source
// input mutations, never while drawing a map or cockpit snapshot.
func (p *RenderingPreview) publishFollowerOrders(effects [5]navigation.OrderEffects) {
	for i := range p.Initialized.Entities {
		e := &p.Initialized.Entities[i]
		slot := e.FlightSlot
		if slot <= 0 || slot >= 5 {
			continue
		}
		e.ControlRaw = p.Flight.Navigation[slot].Raw
		v := effects[slot]
		if v.ChangeKind || v.ResetPhase {
			p.Flight.Combat.ResetRouteAvoidance(e.RuntimeRow)
			p.Flight.Combat.ResetNavigation(e.RuntimeRow)
		}
		if v.ChangeKind {
			e.Kind = v.Kind
			e.DynamicRaw[0] = v.Kind
		}
		if v.ResetPhase {
			e.Participation = 0
			e.DynamicRaw[1] = 0
		}
		if v.ClearCooldown {
			e.DynamicRaw[3] = 0
		}
	}
}
func (p *RenderingPreview) wingmanOrder(slot int, key string, gps bool) error {
	if !p.r1Mechanics || p.Initialized == nil || p.Flight == nil {
		return nil
	}
	rows := make([][32]byte, len(p.Initialized.Rows))
	for i := range rows {
		rows[i] = p.Initialized.Rows[i].Raw
	}
	var active [5]bool
	for _, e := range p.Initialized.Entities {
		if e.FlightSlot >= 0 && e.FlightSlot < 5 {
			active[e.FlightSlot] = e.ControlRaw[1] != 0
		}
	}
	home := navigation.Point{}
	if key == "b" {
		var err error
		home, err = navigation.ResolveHome(rows, p.Flight.Navigation[slot].Raw, slot)
		if berth := p.carrierBerths[slot]; berth.Present() {
			home = berth
		}
		if err != nil {
			return err
		}
	}
	base, farp := navigation.Point{}, navigation.Point{}
	farpPresent := false
	if t := p.Initialized.Targets; t != nil {
		point := func(row [62]byte) navigation.Point {
			return navigation.Point{H0: binary.LittleEndian.Uint32(row[42:]), H1: binary.LittleEndian.Uint32(row[46:])}
		}
		base, farp = point(t.Records[0]), point(t.Records[1])
		farpPresent = binary.LittleEndian.Uint16(t.Records[1][32:]) != 0
	} else if key == "b" {
		return fmt.Errorf("return order lacks retained base/FARP TARGET records")
	}
	apply := navigation.ApplyOrder
	if gps {
		apply = navigation.ApplyGPSOrder
	}
	if p.filmPlayback == nil {
		diagnostics.Info("orders", "Wingman command", "actor", slot, "order", key, "step", p.Flight.Steps)
	}
	effects := apply(&p.Flight.Navigation, active, slot, key, base, home, farp, farpPresent)
	p.publishFollowerOrders(effects)
	p.Flight.Navigation[0].ToFlight(p.Flight.State)
	p.publishNavigation()
	p.Dirty = true
	return nil
}
func (p *RenderingPreview) applyWingmanAux(code int) error {
	slot, key, ok := decodeWingmanOrder(code)
	if !ok {
		return fmt.Errorf("invalid wingman order%d", code)
	}
	p.syncFollowerNavigation()
	if err := p.wingmanOrder(slot, key, false); err != nil {
		return err
	}
	if key == "b" {
		if err := p.Flight.Combat.FinalizeDirectReturn(slot, &p.Flight.Avionics.Contacts); err != nil {
			return err
		}
		p.syncFollowerNavigation()
	}
	return nil
}
