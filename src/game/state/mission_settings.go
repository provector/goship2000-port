package state

import (
	"encoding/binary"
	"fmt"
)

// These saved/UI ordinals come from REALITY's ANIM/LABL records. LaunchSwitches
// adapts them for flight/rendering consumers; deferred combat consumers remain
// separate, and serialized bit values must not replace these preferences.
type PreflightSettings struct {
	CPGControls  int `json:"cpg_controls"`
	EnemyQuality int `json:"enemy_quality"`
	FlightModel  int `json:"flight_model"`
	Landings     int `json:"landings"`
	Avoidance    int `json:"avoidance"`
	Wind         int `json:"wind"`
	Visibility   int `json:"visibility"`
}

var PreflightChoiceCounts = [7]int{3, 4, 2, 2, 2, 2, 2}

// A248a1 clears the new290-byte record; REALITY C01db..0241 reads these
// seven bytes without replacing them. Zero is each control's first choice.
func defaultPreflightSettings() PreflightSettings { return PreflightSettings{} }

// Copy by value so session edits cannot alias another pilot's saved preferences.
// The caller owns candidate-save commit/publication and its failure boundary.
func CopyPilotPreflight(p *Pilot, settings PreflightSettings) error {
	if p == nil {
		return fmt.Errorf("preflight preferences require a selected pilot")
	}
	if err := settings.Validate(); err != nil {
		return err
	}
	p.Preflight = &settings
	return nil
}

func (s PreflightSettings) Values() [7]int {
	return [7]int{s.CPGControls, s.EnemyQuality, s.FlightModel, s.Landings, s.Avoidance, s.Wind, s.Visibility}
}

func (s *PreflightSettings) Set(key, value int) {
	fields := []*int{&s.CPGControls, &s.EnemyQuality, &s.FlightModel, &s.Landings, &s.Avoidance, &s.Wind, &s.Visibility}
	if key >= 0 && key < len(fields) {
		*fields[key] = value
	}
}

func (s PreflightSettings) Validate() error {
	for i, value := range s.Values() {
		count := PreflightChoiceCounts[i]
		if value < 0 || value >= count {
			return fmt.Errorf("preflight control%d=%d is outside0..%d", i, value, count-1)
		}
	}
	return nil
}

func PreflightFromPilot(p *Pilot) (PreflightSettings, error) {
	if p == nil {
		return PreflightSettings{}, fmt.Errorf("preflight preferences require a selected pilot")
	}
	if p.Preflight != nil {
		return *p.Preflight, p.Preflight.Validate()
	}
	var result PreflightSettings
	for key, offset := range [7]int{0x5f, 0x5d, 0x5e, 0x60, 0x61, 0x62, 0x63} {
		value, known := p.LegacyByte(offset)
		if !known {
			return result, fmt.Errorf("preflight preferences unavailable: no typed preferences or legacy field +%02xh", offset)
		}
		result.Set(key, int(value))
	}
	return result, result.Validate()
}

// A23166's arithmetic is also used after default initialization. A zero
// denominator returns zero, matching A212f0, without inventing physical units.
func PreflightRatio(numerator, denominator int16) int16 {
	if denominator == 0 {
		return 0
	}
	return int16(int32(numerator) * 100 / int32(denominator))
}

func PreflightReadiness(rows [][36]byte) error {
	types := map[byte]bool{}
	for _, r := range rows {
		if PreflightRatio(int16(binary.LittleEndian.Uint16(r[25:])), int16(binary.LittleEndian.Uint16(r[23:]))) > 100 {
			return fmt.Errorf("configuration exceeds the source readiness capacity limit; adjust armament")
		}
		types[r[0]] = true
	}
	if len(rows) >= 5 && len(types) > 3 {
		return fmt.Errorf("the source readiness rule permits at most three helicopter types in a five-row flight")
	}
	return nil
}
