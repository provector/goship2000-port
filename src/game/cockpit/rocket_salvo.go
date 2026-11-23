package cockpit

import "fmt"

// Source section2:00cb..0107 appends DS3162's " xx" template, replacing
// its final x with ASCII('0'+AC1). Only the primary green weapon caption uses
// it; the common white HUD caption and stored weapon names remain unchanged.
func primaryWeaponName(s Station, salvo int16) string {
	if s.DefinitionFlags&0x1000 != 0 && (salvo == 1 || salvo == 2 || salvo == 4) {
		return fmt.Sprintf("%s x%d", s.Name, salvo)
	}
	return s.Name
}
