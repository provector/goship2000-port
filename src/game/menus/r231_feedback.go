package menus

// threatWarningTone is source1975:3CC5..3CD3. Semantic DE40 is separate
// from DE80 speech and host audio mute. The caller supplies one reached HUD
// visit; several visits may share DA, so this must not deduplicate by phase.
func threatWarningTone(flags, phase uint16, soundGate byte) bool {
	return flags != 0 && soundGate&0x40 != 0 && phase&3 == 0
}
