package navigation

import (
	"encoding/binary"
	"fmt"
	"gunship2000-assets/game/cockpit"
)

func drawDamageDiagram(dst []byte, a *cockpit.Assets, damage [10]byte) error {
	positions := a.Opaque["gps-damage-positions"]
	if len(positions) != 40 {
		return fmt.Errorf("GPS damage positions unavailable; prepare assets21")
	}
	// Source b202 flags4 preserves zero pixels; the7x7 bursts use flags7.
	if err := a.Sprite(dst, "gps-damage-body", 229, 0, cockpit.Full); err != nil {
		return err
	}
	for i, v := range damage {
		if v&3 == 0 {
			continue
		}
		name := "gps-damage-yellow"
		if v&3 == 2 {
			name = "gps-damage-red"
		}
		x := int(int16(binary.LittleEndian.Uint16(positions[i*4:])))
		y := int(int16(binary.LittleEndian.Uint16(positions[i*4+2:])))
		if err := a.Sprite(dst, name, x-3, y-3, cockpit.Full); err != nil {
			return err
		}
	}
	return nil
}
