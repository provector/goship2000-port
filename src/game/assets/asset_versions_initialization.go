package assets

// InitializationSourceLayout is installation-specific preparation information;
// no original executable offsets are consulted by mission runtime code.
type InitializationSourceLayout struct {
	FormationDS, AircraftSelectorDS, CPGMappingDS            int
	SimulationDataFile, DynamicRowsOffset, ControlRowsOffset int
	InitialViewWordsDS                                       [6]int
	GearGraph                                                GearGraphBinding
	GearGraphSource                                          [3]initializationOperand
}

type initializationOperand struct {
	File  int
	Bytes []byte
}

var islandIceInitializationLayout = InitializationSourceLayout{
	FormationDS: 0x124, AircraftSelectorDS: 0x3d0c, CPGMappingDS: 0x3d44,
	SimulationDataFile: 0x487f0, DynamicRowsOffset: 0x2582, ControlRowsOffset: 0x488,
	InitialViewWordsDS: [6]int{0xacf, 0xad1, 0xad7, 0xad9, 0xadf, 0xae7},
	GearGraph:          GearGraphBinding{ModelID: 139, Node: 46, Side: 0, DownChild: 41, UpChild: 255},
	// Loader header+16 is the packed near-graph root. Source gear writers
	// replace its+5d byte: 1+2*46 selects node46's first child.
	GearGraphSource: [3]initializationOperand{
		{0xb9b0 + 0x13df, []byte{0xa1, 0x66, 0x35, 0xc4, 0x5e, 0xea, 0x26, 0x89, 0x47, 0x16}},
		{0xd8b0 + 0xcf5, []byte{0xc6, 0x47, 0x5d, 0xff}},
		{0xd8b0 + 0xd0d, []byte{0xc6, 0x47, 0x5d, 0x29}},
	},
}
