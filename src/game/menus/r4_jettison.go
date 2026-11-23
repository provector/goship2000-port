package menus

import (
	"encoding/binary"
	"fmt"
	gamestate "gunship2000-assets/game/state"
)

// jettisonStation is0975:0EB8..0FA0. Compact definitions are24 bytes at E6;
// source F1/F3 are therefore offsets11/13. Only AX's low product word survives.
// Nonzero (not positive) ammunition enters this branch, including signed values.
func jettisonStation(st *gamestate.RuntimeStation, station int, raw [24]byte, mass int16) int16 {
	if st.Ammunition == 0 {
		return mass
	}
	unit := int16(binary.LittleEndian.Uint16(raw[11:]))
	if station == 0 || raw[0]&1 != 0 && raw[1]&0x10 == 0 {
		mass += int16(int32(st.Ammunition/-20) * int32(unit))
	} else {
		mass -= int16(int32(st.Ammunition) * int32(unit))
	}
	mass -= int16(binary.LittleEndian.Uint16(raw[13:]))
	st.Ammunition = 0
	return mass
}

func (p *RenderingPreview) jettisonSelectedStation() error {
	if p.Initialized == nil || p.Initialized.Stores == nil || len(p.Initialized.Stores.Ships) == 0 || p.Flight == nil {
		return fmt.Errorf("jettison requires actual player stores")
	}
	station := p.Initialized.SelectedStation
	if station < 0 || station >= len(p.Initialized.Stores.Ships[0].Stations) {
		return fmt.Errorf("jettison station%d outside source0..3", station)
	}
	st := &p.Initialized.Stores.Ships[0].Stations[station]
	if st.Ammunition == 0 {
		return nil //0EC9 exits before reading the definition or altering anything.
	}
	def, err := p.Initialized.Stores.LookupDefinition(st.CompactID)
	if err != nil {
		return err
	}
	p.Flight.MassTerm = jettisonStation(st, station, def.Raw, p.Flight.MassTerm)
	return nil
}
