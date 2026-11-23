package assets

// AudioSourceLayout belongs to a recognized edition. These are source-file and
// module-local coordinates, never a common address space for the three overlays.
type AudioSourceLayout struct {
	Drivers                                                             map[string]AudioDriverLayout
	DigitalEventTableCS, DigitalListCS, DigitalListBytes, DigitalEvents int
	CompanionEventTableCS, CompanionListCS                              int
	PITFrequency, BaselineReload, FastReload, PlayerFallbackReload      int
	PlayerFallbackDivisor                                               int
}

type AudioDriverLayout struct {
	Name                                                                       string
	Role                                                                       SourceRole
	DSFile, Channels, Stride, Shadow, Pairs, Operators, Pitches, Depth         int
	NoiseBank, Life, Masks, Bases, Deltas, Owners, Alternate, RNG              int
	Ordinal, Pointer, InstrumentPointer, ScratchChannel, ScratchOperator       int
	Backend, Probe, InstrumentCount                                            int
	EventTableCS, EventCount, InitCommonCS, FirstInitCS, ResetCS, StopCS       int
	WorkerFlag                                                                 int
	UpdateEnabled, NoiseKind, NoiseArgument, NoiseSpeed, NoiseStep, NoiseLevel int
	LogoStarts                                                                 []int
	InitializationStream                                                       int   `json:",omitempty"`
	InitializationAlternate                                                    int   `json:",omitempty"`
	InitializationDurations                                                    int   `json:",omitempty"`
	FixedGameplayEvents                                                        []int `json:",omitempty"`
	DigitalGameplayEvents                                                      []int `json:",omitempty"`
}

var islandIceAudioLayout = AudioSourceLayout{
	PITFrequency: 1193182, BaselineReload: 0x4c90, FastReload: 0x0f88,
	PlayerFallbackReload: 19886, PlayerFallbackDivisor: 5,
	DigitalEventTableCS: 0x104f, DigitalListCS: 0xc4f, DigitalListBytes: 22 * 14, DigitalEvents: 75,
	CompanionEventTableCS: 0xa18, CompanionListCS: 0x11e9,
	Drivers: map[string]AudioDriverLayout{
		// GS2 is a separate carrier/address domain. Early15h and bounded0bh(1)
		// initialization are baked. L.2 also reuses this verified data layout for
		// the isolated retained row6 rotor controller; it never executes MZ code.
		"GS2": {Name: "GS2", Role: RoleAudioADigital, DSFile: 0x2690, Channels: 0xd8c, Stride: 30, Shadow: 0xe86, Pairs: 0xd4e, Operators: 0xd60, Pitches: 0xd32, Depth: 0xd4a, NoiseBank: 0xf86, Life: 0xe7c, Masks: 0xe7e, Bases: 0xd82, Deltas: 0xd86, Owners: 0xe84, Alternate: 0xe83, RNG: 0xc88, Ordinal: 0xe82, Pointer: 0xfe2, InstrumentPointer: 0xfe4, ScratchChannel: 0xfde, ScratchOperator: 0xfe0, Backend: 0xc8d, Probe: 0xc8f, InstrumentCount: 48, EventTableCS: 0x104f, EventCount: 75, InitCommonCS: 0x730, FirstInitCS: 0x702, ResetCS: 0x204, StopCS: 0x77c, WorkerFlag: -1, UpdateEnabled: 0xd8a, NoiseKind: 0xc92, NoiseArgument: 0xc90, NoiseSpeed: 0xc94, NoiseStep: 0xc96, NoiseLevel: 0xc98, InitializationStream: 0x904, InitializationAlternate: 0x908, InitializationDurations: 0xc99, FixedGameplayEvents: []int{0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0f, 0x10, 0x11, 0x14, 0x17, 0x18, 0x25, 0x27, 0x28, 0x29, 0x2a, 0x4a}, DigitalGameplayEvents: []int{0x2d, 0x2e, 0x30, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38, 0x3b, 0x3c, 0x3d, 0x3e, 0x3f, 0x45, 0x46, 0x48}},
		"LOG": {Name: "LOG", Role: roleAudioLOG, DSFile: 0x13c0, Channels: 0x43c, Stride: 26, Shadow: 0x516, Pairs: 0x404, Operators: 0x416, Pitches: 0x3e8, Depth: 0x400, NoiseBank: 0x616, Life: 0x50c, Masks: 0x50e, Bases: 0x432, Deltas: 0x436, Owners: 0x514, Alternate: 0x513, RNG: 0x3ce, Ordinal: 0x512, Pointer: 0x672, InstrumentPointer: 0x674, ScratchChannel: 0x66e, ScratchOperator: 0x670, Backend: -1, Probe: -1, InstrumentCount: 4, UpdateEnabled: 0x43a, EventTableCS: 0x5d4, EventCount: 3, LogoStarts: []int{0x10e, 0x120, 0x14c, 0x17a, 0x1a6, 0x1b6}},
		"GS3": {Name: "GS3", Role: roleAudioGS3, DSFile: 0x1aa0, Channels: 0x16ec, Stride: 30, Shadow: 0x17e6, Pairs: 0x16b0, Operators: 0x16c2, Pitches: 0x1694, Depth: 0x16ac, NoiseBank: 0x18e6, Life: 0x17dc, Masks: 0x17de, Bases: 0x16e2, Deltas: 0x16e6, Owners: 0x17e4, Alternate: 0x17e3, RNG: 0x15ec, Ordinal: 0x17e2, Pointer: 0x1942, InstrumentPointer: 0x1944, ScratchChannel: 0x193e, ScratchOperator: 0x1940, Backend: 0x15f0, Probe: 0x15f2, InstrumentCount: 48, EventTableCS: 0xaba, EventCount: 74, InitCommonCS: 0x67e, FirstInitCS: 0x650, ResetCS: 0x232, StopCS: 0x6ca, WorkerFlag: 0x15c8, UpdateEnabled: 0x16ea, NoiseKind: 0x15f5, NoiseArgument: 0x15f3, NoiseSpeed: 0x15f7, NoiseStep: 0x15f9, NoiseLevel: 0x15fb},
		"GS4": {Name: "GS4", Role: RoleAudioGS4, DSFile: 0x1b10, Channels: 0x2f3c, Stride: 30, Shadow: 0x3036, Pairs: 0x2f02, Operators: 0x2f14, Pitches: 0x2ee6, Depth: 0x2efe, NoiseBank: 0x3136, Life: 0x302c, Masks: 0x302e, Bases: 0x2f32, Deltas: 0x2f36, Owners: 0x3034, Alternate: 0x3033, RNG: 0x2e3c, Ordinal: 0x3032, Pointer: 0x3192, InstrumentPointer: 0x3194, ScratchChannel: 0x318e, ScratchOperator: 0x3190, Backend: 0x2e41, Probe: 0x2e43, InstrumentCount: 48, EventTableCS: 0xb1a, EventCount: 75, InitCommonCS: 0x6d7, FirstInitCS: 0x6a9, ResetCS: 0x1bd, StopCS: 0x723, WorkerFlag: 0x2e1a, UpdateEnabled: 0x2f3a, NoiseKind: 0x2e46, NoiseArgument: 0x2e44, NoiseSpeed: 0x2e48, NoiseStep: 0x2e4a, NoiseLevel: 0x2e4c},
	},
}
