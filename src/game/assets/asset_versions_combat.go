package assets

type WorldCombatSourceLayout struct {
	HeaderFile, HeaderBytes                           int
	NearPoolFile, NearPoolBytes                       int
	FarPoolFile, FarPoolBytes                         int
	ExplosionHeaderOffset, ExplosionBaseOffset        int
	DriverFile, SpriteLookupOffset, SpriteLookupCount int
}

var islandIceCombatLayout = WorldCombatSourceLayout{
	HeaderFile: 0x4e470, HeaderBytes: 4 * 38,
	NearPoolFile: 0x61bc0, NearPoolBytes: 0x3553,
	FarPoolFile: 0x4e510, FarPoolBytes: 0x19a0,
	ExplosionHeaderOffset: 0x2ae8, ExplosionBaseOffset: 0x668,
	DriverFile: 0x3e550, SpriteLookupOffset: 0x6100, SpriteLookupCount: 28,
}
