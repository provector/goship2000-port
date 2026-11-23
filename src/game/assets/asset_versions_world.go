package assets

// WorldSourceLayout belongs to the recognized executable profile. Parsers do
// not borrow offsets from whichever executable happens to share a filename.
type WorldSourceLayout struct {
	Combat                                          *WorldCombatSourceLayout
	SineFile                                        int
	AngleBasesFile, AtanFile, WeatherRemapFile      int
	SetupModeOffset, SetupOptionsOffset, SetupBytes int
	CallbackTableFile                               int
	WorldNames                                      []string
}

var islandIceWorldLayout = WorldSourceLayout{
	Combat:            &islandIceCombatLayout,
	SineFile:          0x4bc50,
	AngleBasesFile:    0x4bc50 + 0x802,
	AtanFile:          0x4bc50 + 0x812,
	WeatherRemapFile:  0x487f0 + 0x2d62,
	CallbackTableFile: 0x64cce,
	SetupModeOffset:   0, SetupOptionsOffset: 2, SetupBytes: 38,
	WorldNames: []string{"ANTC1", "ANTC2", "ANTC3", "EURO6", "EURO7", "EURO8", "EURO9", "GULF2", "GULF3", "GULF4", "GULF5", "PHIL1", "PHIL2", "PHIL3"},
}
