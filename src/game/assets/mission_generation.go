package assets

import "gunship2000-assets/game/world"

// MissionGenerationData is prepared from source-bound tables, not executable reads at runtime.
type MissionGenerationData struct {
	NativeModels map[string]*world.Geometry `json:"-"` // Bound from the verified prepared bundle, never regenerated or serialized.
	Version      int
	Worlds       []MissionWorld
	Tables       MissionTables
	// Raw immutable WRLD.ENEM banks. Missing only in historical preparations;
	// launch policy transforms a selected bank after REALITY and freezes it.
	EnemyDefinitions map[string][]EnemyDefinition `json:",omitempty"`
}

type MissionWorld struct {
	Theatre, Suffix                                int
	Scenario, WorldName, TerrainName               string
	THHD, SNHD, FMHD, FRMD, OBJL, LAND, Grid, PATH []byte
}

type MissionTables struct {
	EditorObjectTypes            []byte      `json:",omitempty"`
	EditorObjectiveVerbs         []string    `json:",omitempty"`
	EditorObjectiveAbbreviations [][2]string `json:",omitempty"`
	EditorModelCosts             []uint16    `json:",omitempty"`
	EditorTheatreOffsets         []int8      `json:",omitempty"`
	EditorWorldBaseCosts         []uint16    `json:",omitempty"`
	Initialization               *MissionInitializationTables
	Leader                       []byte
	Wind, Temperature            [][]int
	Consonants                   string
	Callsigns, Compass, Ranks    []string
	VisibilityPairs              [2][2]string
	Templates                    [][]byte
	Thresholds                   map[int]int
	Primary, Secondary           [][]int
	Masks                        []uint32
	DisplayKinds                 []int
	WindMultiplier               float64
}
