package state

import "fmt"

// LaunchSwitches is the semantic GS8ca adapter, not a DOS cache structure. The
// input conditions are committed mission data; UI preferences replace only the
// source-owned bits. In particular CLEAR removes GREY, not DARK/night.
func LaunchSwitches(m *MissionData, settings PreflightSettings, cpgMap []byte) (uint16, error) {
	if m == nil {
		return 0, fmt.Errorf("launch switches need a prepared mission")
	}
	if err := settings.Validate(); err != nil {
		return 0, err
	}
	if len(cpgMap) != 3 {
		return 0, fmt.Errorf("launch CP/G mapping requires three source entries")
	}
	kind := m.Inputs.MissionType.Raw()
	if m.SoloBuilder && m.Inputs.MissionType == MissionSimulated {
		kind = MissionSingle.Raw()
	}
	if kind < 0 || kind > 4 {
		return 0, fmt.Errorf("unsupported launch mission type%d", kind)
	}
	flags := m.Conditions.Flags
	low := uint16(settings.EnemyQuality) | uint16(settings.FlightModel)<<2 | uint16(cpgMap[settings.CPGControls]&3)<<3
	// GS1c89:091c/0934 use CMP1; SBB; NEG: ordinal zero sets
	// protection/avoidance. These are independent from Training's0400.
	if settings.Landings == 0 {
		low |= 0x20
	}
	if settings.Avoidance == 0 {
		low |= 0x40
	}
	if settings.Wind == 1 {
		low |= 0x80
	}
	flags = flags&0xff00 | low
	if settings.Visibility == 0 {
		flags &^= 0x100
	}
	if kind == 0 {
		flags |= 0x400
	} else {
		flags &^= 0x400
	}
	if (m.Scene == 2 || m.Scene == 5) && flags&0x800 == 0 {
		flags |= 0x1000
	} else {
		flags &^= 0x1000
	}
	flags = flags&0x1fff | uint16(kind)<<13
	return flags, nil
}
