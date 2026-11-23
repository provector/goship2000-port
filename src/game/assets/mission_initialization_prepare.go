package assets

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// GearGraphBinding compiles a source-mutated near-graph byte into an immutable
// per-instance component switch. Model IDs are not aircraft/cockpit selectors.
type GearGraphBinding struct {
	ModelID, Node, Side, DownChild, UpChild int
}

type MissionInitializationTables struct {
	Version                       int
	Formation                     []byte
	AircraftSelectors, CPGMapping []byte
	DynamicRows                   []byte
	// Cold carrier bytes only: 80 nominal 70-byte offsets overlap later
	// OBJECT/TARGET storage; only five flight controls have setup ownership.
	ControlRows         []byte
	SourceGS, SourceGS2 string
	InitialViewWords    [6]int16
	GearGraph           GearGraphBinding
}

func prepareMissionInitialization(gs, gs2 []byte, p *AssetVersionProfile) (*MissionInitializationTables, error) {
	if p == nil || p.InitializationLayout == nil || !p.SourceMatches(RoleMainExecutable, Hash(gs)) || !p.SourceMatches(RoleSimulationExecutable, Hash(gs2)) {
		return nil, fmt.Errorf("mission initialization requires the recognized GS/GS2 source pair")
	}
	l := p.InitializationLayout
	slice := func(b []byte, at, n int) ([]byte, error) {
		if at < 0 || n < 0 || at > len(b)-n {
			return nil, fmt.Errorf("mission initialization source range outside profile")
		}
		return append([]byte(nil), b[at:at+n]...), nil
	}
	t := &MissionInitializationTables{Version: 3, SourceGS: Hash(gs), SourceGS2: Hash(gs2), GearGraph: l.GearGraph}
	for _, op := range l.GearGraphSource {
		b, err := slice(gs2, op.File, len(op.Bytes))
		if err != nil || len(op.Bytes) == 0 || !bytes.Equal(b, op.Bytes) {
			return nil, fmt.Errorf("gear component source operands do not match supported profile")
		}
	}
	for i, offset := range l.InitialViewWordsDS {
		b, err := slice(gs2, l.SimulationDataFile+offset, 2)
		if err != nil {
			return nil, err
		}
		t.InitialViewWords[i] = int16(binary.LittleEndian.Uint16(b))
	}
	var err error
	if t.Formation, err = slice(gs, p.OfficeLayout.DataFileAnchor+l.FormationDS, 15*11); err != nil {
		return nil, err
	}
	if t.AircraftSelectors, err = slice(gs, p.OfficeLayout.DataFileAnchor+l.AircraftSelectorDS, 8); err != nil {
		return nil, err
	}
	if t.CPGMapping, err = slice(gs, p.OfficeLayout.DataFileAnchor+l.CPGMappingDS, 3); err != nil {
		return nil, err
	}
	if t.DynamicRows, err = slice(gs2, l.SimulationDataFile+l.DynamicRowsOffset, 80*24); err != nil {
		return nil, err
	}
	if t.ControlRows, err = slice(gs2, l.SimulationDataFile+l.ControlRowsOffset, 80*70); err != nil {
		return nil, err
	}
	return t, ValidateMissionInitialization(t)
}

func ValidateMissionInitialization(t *MissionInitializationTables) error {
	if t == nil || t.Version != 3 || len(t.Formation) != 15*11 || len(t.AircraftSelectors) != 8 || len(t.CPGMapping) != 3 || len(t.DynamicRows) != 80*24 || len(t.ControlRows) != 80*70 || t.SourceGS == "" || t.SourceGS2 == "" {
		return fmt.Errorf("mission initialization tables missing/incompatible; rebuild supported assets")
	}
	b := t.GearGraph
	if b.ModelID < 0 || b.ModelID >= 230 || b.Node < 0 || b.Node >= 255 || b.Side < 0 || b.Side > 1 || b.DownChild < 0 || b.DownChild >= 255 || b.UpChild != 255 {
		return fmt.Errorf("gear component binding missing/incompatible; rebuild supported assets")
	}
	for _, v := range t.AircraftSelectors {
		if v > 7 {
			return fmt.Errorf("serialized aircraft selector outside supported domain")
		}
	}
	return nil
}
