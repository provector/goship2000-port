package assets

// These offsets belong only to the recognized selected-VGA GS2 profile. They
// are preparation locators, never runtime addresses or gameplay identifiers.
type cockpitSourceSlice struct {
	Name                                   string
	Section, CS, IP, Width, Height, Length int
}
type cockpitSceneLineLayout struct {
	Name   string
	X, Y   int
	Colour byte
	Style  uint16
}
type cockpitScenePromptLayout struct {
	Name                                                   string
	Foreground, Background                                 byte
	CenterX, TextY, LeftPadding, RightPadding, Top, Bottom int
	Style, MeasureStyle                                    uint16
}
type CockpitSourceLayout struct {
	SectionBodies                                                     [9]int
	SectionCS                                                         [9]int
	FontFile, FontBytes, SineFile, SineWords                          int
	NightPaletteSection, NightPaletteCS, NightPaletteIP               int
	AttitudeSection, AttitudeCS, AttitudeIP, AttitudeBytes            int
	MapVectorCS, MapVectorStart, MapVectorEnd, MapVectorTable         int
	MapVectorRoots                                                    []int
	Atlases, Sprites, Texts, Opaque, Vectors                          []cockpitSourceSlice
	HeadingTableDS, BodyTableDS, ChoiceTableDS, DetailedChoiceTableDS int
	SceneInitialization                                               []cockpitSceneLineLayout
	SceneQuit, SceneEnd                                               cockpitScenePromptLayout
}

var islandIceCockpitLayout = CockpitSourceLayout{
	SectionBodies: [9]int{0xa120, 0xd8b0, 0x1dc60, 0x21b90, 0x24290, 0x2f7d0, 0x34650, 0x3e550, 0x487f0},
	SectionCS:     [9]int{0x975, 0x975, 0x1975, 0x1975, 0x975, 0x975, 0x1d47, 0x1d47, 0x272d},
	FontFile:      0x4ec10, FontBytes: 670, SineFile: 0x4bc50, SineWords: 1025,
	NightPaletteSection: 7, NightPaletteCS: 0x1d47, NightPaletteIP: 0x13e1,
	AttitudeSection: 2, AttitudeCS: 0x1975, AttitudeIP: 0x34f0, AttitudeBytes: 25,
	MapVectorCS: 0x15b, MapVectorStart: 0x6d4, MapVectorEnd: 0x8a8, MapVectorTable: 0x84b,
	MapVectorRoots: []int{0x6d4, 0x6ed, 0x70d, 0x75a, 0x79d, 0x7aa, 0x7b7, 0x7c4, 0x7df, 0x7fa, 0x805, 0x81e, 0x870, 0x73d, 0x773, 0x78c, 0x722},
	Vectors:        []cockpitSourceSlice{{Name: "gps-fly-cursor", Section: 4, CS: 0x975, IP: 0x2144, Length: 13}, {Name: "gps-support-cursor", Section: 4, CS: 0x975, IP: 0x212b, Length: 25}},
	Atlases: []cockpitSourceSlice{
		{"heading-strip", 7, 0x1d47, 0x16e1, 256, 16, 4096},
		{"vertical-strip", 7, 0x1d47, 0x26e1, 6, 80, 480},
		{"fixed-strip", 7, 0x1d47, 0x28c1, 5, 32, 160},
		{"fixed-scale", 7, 0x1d47, 0x2961, 16, 65, 1040},
		{"digits-opaque", 7, 0x1d47, 0x2d71, 6, 60, 360},
		{"digits-zero-mask", 7, 0x1d47, 0x2ed9, 4, 60, 240},
	},
	Sprites: []cockpitSourceSlice{
		{"gps-damage-body", 4, 0x975, 0x2257, 91, 67, 0},
		{"gps-damage-yellow", 4, 0x975, 0x2151, 7, 7, 0},
		{"gps-damage-red", 4, 0x975, 0x2186, 7, 7, 0},
		{"gps-overview-box", 4, 0x975, 0x21bb, 11, 9, 0},
		{"aim-fixed", 2, 0x1975, 0x2a43, 21, 17, 0}, {"station-plain", 2, 0x1975, 0x2bac, 3, 3, 0},
		{"station-highlight", 2, 0x1975, 0x2bb9, 3, 3, 0}, {"station-baseline", 2, 0x1975, 0x2bc6, 35, 3, 0},
		{"attitude-overlay", 2, 0x1975, 0x2c33, 31, 31, 0}, {"hud-arrow", 2, 0x1975, 0x2ff8, 15, 7, 0},
		{"target-marker", 2, 0x1975, 0x3439, 5, 3, 0}, {"warning", 2, 0x1975, 0x344c, 4, 4, 0},
		{"waypoint-marker", 2, 0x1975, 0x3476, 7, 4, 0}, {"lateral-marker", 2, 0x1975, 0x3496, 4, 5, 0},
		{"bearing-marker", 2, 0x1975, 0x34ae, 7, 3, 0}, {"hud-arrow-alternate", 2, 0x1975, 0x3509, 15, 7, 0},
		{"driver-marker", 7, 0x1d47, 0x10cd, 4, 4, 0}, {"driver-conditional", 7, 0x1d47, 0x6138, 11, 14, 0},
		{"mast-view", 7, 0x1d47, 0x61d6, 211, 73, 0}, {"target-symbol-1", 2, 0x1975, 0x32fa, 21, 15, 0},
		{"target-symbol-2", 2, 0x1975, 0x31bb, 21, 15, 0}, {"target-symbol-3", 2, 0x1975, 0x3074, 19, 17, 0},
	},
	Texts: []cockpitSourceSlice{
		{Name: "notification-hq", Section: 8, CS: 0x2a29, IP: 0x471, Length: 3},
		{Name: "notification-01", Section: 8, CS: 0x2a29, IP: 0x2be, Length: 27},
		{Name: "notification-02", Section: 8, CS: 0x2a29, IP: 0x2d9, Length: 22},
		{Name: "notification-03", Section: 8, CS: 0x2a29, IP: 0x2ef, Length: 18},
		{Name: "notification-04", Section: 8, CS: 0x2a29, IP: 0x301, Length: 20},
		{Name: "notification-05", Section: 8, CS: 0x2a29, IP: 0x315, Length: 17},
		{Name: "notification-06", Section: 8, CS: 0x2a29, IP: 0x326, Length: 22},
		{Name: "notification-07", Section: 8, CS: 0x2a29, IP: 0x33c, Length: 16},
		{Name: "notification-08", Section: 8, CS: 0x2a29, IP: 0x34c, Length: 14},
		{Name: "notification-09", Section: 8, CS: 0x2a29, IP: 0x35a, Length: 25},
		{Name: "notification-10", Section: 8, CS: 0x2a29, IP: 0x373, Length: 22},
		{Name: "notification-11", Section: 8, CS: 0x2a29, IP: 0x389, Length: 21},
		{Name: "notification-12", Section: 8, CS: 0x2a29, IP: 0x39e, Length: 22},
		{Name: "notification-13", Section: 8, CS: 0x2a29, IP: 0x3b4, Length: 23},
		{Name: "notification-14", Section: 8, CS: 0x2a29, IP: 0x3cb, Length: 19},
		{Name: "notification-15", Section: 8, CS: 0x2a29, IP: 0x3de, Length: 25},
		{Name: "notification-16", Section: 8, CS: 0x2a29, IP: 0x3f7, Length: 20},
		{Name: "notification-17", Section: 8, CS: 0x2a29, IP: 0x40b, Length: 22},
		{Name: "notification-18", Section: 8, CS: 0x2a29, IP: 0x421, Length: 24},
		{Name: "notification-19", Section: 8, CS: 0x2a29, IP: 0x439, Length: 24},
		{Name: "notification-20", Section: 8, CS: 0x2a29, IP: 0x451, Length: 13},
		{Name: "notification-21", Section: 8, CS: 0x2a29, IP: 0x45e, Length: 10},
		{Name: "notification-22", Section: 8, CS: 0x2a29, IP: 0x468, Length: 9},
		{Name: "gps-controls", Section: 4, CS: 0x975, IP: 0x3a2c, Length: 114},
		{Name: "gps-controls-overview", Section: 4, CS: 0x975, IP: 0x5275, Length: 91},
		{Name: "gps-controls-follower-detail", Section: 4, CS: 0x975, IP: 0x52dc, Length: 94},
		{Name: "gps-controls-follower-overview", Section: 4, CS: 0x975, IP: 0x535b, Length: 143},
		{Name: "gps-commands", Section: 4, CS: 0x975, IP: 0x540b, Length: 10},
		{Name: "gps-support-aircraft", Section: 4, CS: 0x975, IP: 0x2222, Length: 11},
		{Name: "gps-support-artillery", Section: 4, CS: 0x975, IP: 0x222d, Length: 16},
		{Name: "gps-orders", Section: 4, CS: 0x975, IP: 0x53f9, Length: 18},
		{Name: "gps-landed", Section: 4, CS: 0x975, IP: 0x5231, Length: 7},
		{Name: "gps-achieved", Section: 4, CS: 0x975, IP: 0x51a3, Length: 10},
		{Name: "gps-hold-label", Section: 4, CS: 0x975, IP: 0x6c7a, Length: 7},
		{Name: "gps-hold-0", Section: 4, CS: 0x975, IP: 0x6c68, Length: 6},
		{Name: "gps-hold-1", Section: 4, CS: 0x975, IP: 0x6c54, Length: 6},
		{Name: "gps-hold-2", Section: 4, CS: 0x975, IP: 0x6c4d, Length: 7},
		{Name: "gps-hold-3", Section: 4, CS: 0x975, IP: 0x6c44, Length: 9},
		{Name: "gps-weapons-free", Section: 4, CS: 0x975, IP: 0x5208, Length: 13},
		{Name: "gps-weapons-hold", Section: 4, CS: 0x975, IP: 0x521e, Length: 13},
		{Name: "gps-altitude-speed", Section: 4, CS: 0x975, IP: 0x5341, Length: 18},
		{Name: "gps-altitude-1", Section: 4, CS: 0x975, IP: 0x6c89, Length: 4},
		{Name: "gps-altitude-2", Section: 4, CS: 0x975, IP: 0x6c81, Length: 8},
		{Name: "gps-altitude-3", Section: 4, CS: 0x975, IP: 0x6c76, Length: 4},
		{Name: "gps-speed-1", Section: 4, CS: 0x975, IP: 0x5270, Length: 5},
		{Name: "gps-speed-2", Section: 4, CS: 0x975, IP: 0x525c, Length: 7},
		{Name: "gps-speed-3", Section: 4, CS: 0x975, IP: 0x5248, Length: 5},
		{Name: "gps-speed-4", Section: 4, CS: 0x975, IP: 0x5248, Length: 5},
		{Name: "gps-fuel", Section: 4, CS: 0x975, IP: 0x53f4, Length: 5},
		{Name: "gps-chaff", Section: 4, CS: 0x975, IP: 0x51f9, Length: 6},
		{Name: "gps-flares", Section: 4, CS: 0x975, IP: 0x51ec, Length: 7},
		{Name: "gps-mode-0", Section: 4, CS: 0x975, IP: 0x53ea, Length: 10},
		{Name: "gps-mode-1", Section: 4, CS: 0x975, IP: 0x53ea, Length: 10},
		{Name: "gps-mode-2", Section: 4, CS: 0x975, IP: 0x5353, Length: 8},
		{Name: "gps-mode-3", Section: 4, CS: 0x975, IP: 0x533a, Length: 7},
		{Name: "gps-mode-4", Section: 4, CS: 0x975, IP: 0x52d0, Length: 12},
		{Name: "gps-mode-5", Section: 4, CS: 0x975, IP: 0x5263, Length: 13},
		{Name: "gps-mode-6", Section: 4, CS: 0x975, IP: 0x5254, Length: 8},
		{Name: "gps-mode-7", Section: 4, CS: 0x975, IP: 0x5254, Length: 8},
		{Name: "gps-mode-8", Section: 4, CS: 0x975, IP: 0x5240, Length: 8},
		{Name: "gps-aircraft-0", Section: 4, CS: 0x975, IP: 0x524d, Length: 7},
		{Name: "gps-aircraft-1", Section: 4, CS: 0x975, IP: 0x5238, Length: 8},
		{Name: "gps-aircraft-2", Section: 4, CS: 0x975, IP: 0x522b, Length: 6},
		{Name: "gps-aircraft-3", Section: 4, CS: 0x975, IP: 0x5215, Length: 9},
		{Name: "gps-aircraft-4", Section: 4, CS: 0x975, IP: 0x51ff, Length: 9},
		{Name: "gps-aircraft-5", Section: 4, CS: 0x975, IP: 0x51f3, Length: 6},
		{Name: "gps-aircraft-6", Section: 4, CS: 0x975, IP: 0x51e2, Length: 10},
		{Name: "gps-aircraft-7", Section: 4, CS: 0x975, IP: 0x6d45, Length: 1},
		{Name: "autopilot", Section: 2, CS: 0x1975, IP: 0x3460, Length: 11},
		{Name: "autohover", Section: 2, CS: 0x1975, IP: 0x346b, Length: 11},
		{Name: "waypoints", Section: 2, CS: 0x1975, IP: 0x34c7, Length: 8},
		{Name: "secondary", Section: 2, CS: 0x1975, IP: 0x34cf, Length: 10},
		{Name: "primary", Section: 2, CS: 0x1975, IP: 0x34d9, Length: 8},
		{Name: "fuel", Section: 2, CS: 0x1975, IP: 0x34e1, Length: 5},
		{Name: "achieved", Section: 2, CS: 0x1975, IP: 0x34e6, Length: 10},
		{Name: "scene-init-initializing", Section: 0, CS: 0xafe, IP: 0xd14, Length: 24},
		{Name: "scene-init-verify", Section: 0, CS: 0xafe, IP: 0xd02, Length: 18},
		{Name: "scene-init-avionics", Section: 0, CS: 0xafe, IP: 0xcee, Length: 20},
		{Name: "scene-init-optics", Section: 0, CS: 0xafe, IP: 0xcdc, Length: 18},
		{Name: "scene-init-gps", Section: 0, CS: 0xafe, IP: 0xccd, Length: 15},
		{Name: "scene-init-weapons", Section: 0, CS: 0xafe, IP: 0xcba, Length: 19},
		{Name: "scene-init-ok", Section: 0, CS: 0xafe, IP: 0xca8, Length: 18},
		{Name: "scene-quit", Section: 8, CS: 0x2a29, IP: 0x4, Length: 18},
		{Name: "scene-end", Section: 8, CS: 0x2a29, IP: 0x288, Length: 20},
		{Name: "scene-pause", Section: 8, CS: 0x2a29, IP: 0x29c, Length: 15},
	},
	Opaque: []cockpitSourceSlice{
		{Name: "gps-damage-positions", Section: 4, CS: 0x975, IP: 0x517b, Length: 40},
		{Name: "scene-confirmation-remap", Section: 7, CS: 0x1d47, IP: 0xd34, Length: 256},
		{Name: "driver-6100-untyped", Section: 7, CS: 0x1d47, IP: 0x6100, Length: 56},
		{Name: "secondary-MFD-jump-table", Section: 2, CS: 0x1975, IP: 0x1cbe, Length: 14},
		{Name: "target-symbol-far-table", Section: 8, CS: 0x2e99, IP: 0, Length: 12},
	},
	HeadingTableDS: 0x4bce, BodyTableDS: 0x4be0, ChoiceTableDS: 0x4bf2, DetailedChoiceTableDS: 0x4c3a,
	// Section0 alias0afe:0004..0ad4 draws cumulative source lines; these
	// constants are the literal push operands, not native readiness claims.
	SceneInitialization: []cockpitSceneLineLayout{
		{"scene-init-initializing", 200, 8, 10, 2},
		{"scene-init-verify", 200, 14, 10, 2},
		{"scene-init-avionics", 200, 20, 10, 2},
		{"scene-init-optics", 200, 26, 10, 2},
		{"scene-init-gps", 200, 32, 10, 2},
		{"scene-init-weapons", 200, 38, 10, 2},
		{"scene-init-ok", 200, 48, 10, 2},
	},
	SceneQuit: cockpitScenePromptLayout{"scene-quit", 0x1e, 0xe8, 160, 64, 4, 3, 62, 70, 0x32, 2},
	SceneEnd:  cockpitScenePromptLayout{"scene-end", 0x1e, 0xe8, 160, 64, 4, 3, 62, 70, 0x32, 2},
}
