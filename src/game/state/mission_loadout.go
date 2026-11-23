package state

import (
	"encoding/binary"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
)

func RowWord(r []byte, at int) int16 { return int16(binary.LittleEndian.Uint16(r[at:])) }

func RowPut(r []byte, at int, value int) { binary.LittleEndian.PutUint16(r[at:], uint16(value)) }

// Shared default/replacement load calculations retain source integer widths.
// Opaque terms keep their proven operations rather than guessed physics names;
// these rows configure preparation and never launch a simulation.
func PrepareReadinessRows(a *assetdata.MissionAssets, m *MissionData) ([][36]byte, error) {
	if m == nil || m.SourceWorld == nil || m.FlightTemplate < 0 || m.FlightTemplate >= len(a.Generation.Tables.Templates) {
		return nil, fmt.Errorf("mission lacks a valid source flight template")
	}
	template := a.Generation.Tables.Templates[m.FlightTemplate]
	if len(template) != 47 || len(m.SourceWorld.THHD) != 54 {
		return nil, fmt.Errorf("default configuration source layout is incomplete")
	}
	count := 1
	if m.Inputs.MissionType.Raw() > 1 && !m.SoloBuilder {
		count = 5
	}
	rows := make([][36]byte, count)
	class2 := 0
	for i := range rows {
		d := template[2+i*9 : 11+i*9]
		r := rows[i][:]
		sh, ok := a.AircraftDefinitions[int(int8(d[0]))]
		if !ok || len(sh) != 252 {
			return nil, fmt.Errorf("aircraft definition%d missing", d[0])
		}
		initialized, err := InitializeReadinessRow(a, m, int(int8(d[0])))
		if err != nil {
			return nil, err
		}
		rows[i] = initialized
		r = rows[i][:]
		// Descriptor data overwrites initialization defaults in A226c0..22713.
		RowPut(r, 21, int(binary.LittleEndian.Uint16(d[1:3])))
		for j := 0; j < 3; j++ {
			RowPut(r, 12+2*j, int(int8(d[3+2*j])))
			r[3+j] = d[4+2*j]
		}
		if r[1] == 2 {
			class2++
			if class2 == 1 && m.RawE278 != 0 {
				RowPut(r, 16, 1)
				r[5], r[35] = 20, 2
				if m.RawE278 == 2 {
					r[5], r[35] = 21, 1
				}
			}
		}
		if err := RecomputeReadinessRow(a, sh, r); err != nil {
			return nil, err
		}
	}
	// A22512 tries these exact percentages of the unchanged original word27,
	// stopping when the capacity predicate passes; no invented final fallback.
	for _, factor := range []int{90, 80, 70} {
		if readinessCapacityOK(rows) {
			break
		}
		for i := range rows {
			r := rows[i][:]
			RowPut(r, 29, int(int32(RowWord(r, 27))*int32(factor)/100))
			if err := RecomputeReadinessRow(a, a.AircraftDefinitions[int(int8(r[0]))], r); err != nil {
				return nil, err
			}
		}
	}
	return rows, nil
}

func readinessCapacityOK(rows [][36]byte) bool {
	for _, r := range rows {
		if PreflightRatio(RowWord(r[:], 25), RowWord(r[:], 23)) > 100 {
			return false
		}
	}
	return true
}

func RecomputeReadinessRow(a *assetdata.MissionAssets, sh, r []byte) error {
	if len(sh) != 252 || len(r) != 36 {
		return fmt.Errorf("incomplete readiness row or definition")
	}
	total := RowWord(r, 29)
	if def, ok := a.WeaponDefinitions[int(int8(r[2]))]; ok {
		if len(def) != 214 {
			return fmt.Errorf("invalid system definition width")
		}
		total += int16(int(RowWord(def, 13)) * int(RowWord(r, 21)/100))
	}
	for i := 0; i < 3; i++ {
		if def, ok := a.WeaponDefinitions[int(int8(r[3+i]))]; ok {
			if len(def) != 214 {
				return fmt.Errorf("invalid system definition width")
			}
			fixed := int16(int(RowWord(def, 15)) * int(int8(sh[9])))
			RowPut(r, 6+2*i, int(fixed))
			total += int16(int(RowWord(def, 13))*int(RowWord(r, 12+2*i))) + fixed
		}
	}
	RowPut(r, 25, int(total))
	return nil
}

// initializeReadinessRow is A22bd4's replacement/default initializer, before any
// mission-template stores overwrite it. A helicopter change must not keep the
// previous type's weapon package or reapply a default mission template.
func InitializeReadinessRow(a *assetdata.MissionAssets, m *MissionData, id int) ([36]byte, error) {
	var row [36]byte
	if m == nil || m.SourceWorld == nil || len(m.SourceWorld.THHD) != 54 {
		return row, fmt.Errorf("aircraft initialization lacks theatre data")
	}
	sh, ok := a.AircraftDefinitions[id]
	if !ok || len(sh) != 252 {
		return row, fmt.Errorf("aircraft definition %d missing", id)
	}
	r := row[:]
	r[0], r[1], r[2] = byte(id), sh[1], sh[3]
	divisor := int(int8(sh[4]))
	if divisor == 0 {
		return row, fmt.Errorf("source aircraft%d capacity divisor is zero", byte(id))
	}
	// A22c48 reads DS:b62f; the54-byte THHD table starts at DS:b614.
	// Offset0x1b is a signed capacity input, unlike byte13 inside the name.
	q := int(int8(m.SourceWorld.THHD[0x1b])) / divisor
	if q < -128 || q > 127 {
		return row, fmt.Errorf("source capacity byte division would overflow")
	}
	base := int32(100-q) * int32(RowWord(sh, 10)) / 100
	temp, k := int(m.Conditions.Temperature), int(int8(sh[7]))
	adjust := int16(k * missionAbs(temp-45))
	if temp > 70 {
		adjust += int16(k * (temp - 70))
	}
	if temp < 45 {
		adjust += int16(k * (45 - temp))
	}
	if temp < 30 {
		adjust += int16(2 * k * (30 - temp))
	}
	RowPut(r, 23, int(base-int32(adjust)))
	RowPut(r, 27, int(RowWord(sh, 12)))
	RowPut(r, 29, int(RowWord(sh, 12)))
	RowPut(r, 31, 30)
	RowPut(r, 33, 30)

	if def, ok := a.WeaponDefinitions[int(int8(r[2]))]; ok {
		if len(def) != 214 {
			return row, fmt.Errorf("invalid cannon definition")
		}
		RowPut(r, 21, int(RowWord(def, 2)))
	}
	if err := RecomputeReadinessRow(a, sh, r); err != nil {
		return row, err
	}
	return row, nil
}
