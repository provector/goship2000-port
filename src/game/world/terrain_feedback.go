package world

import "fmt"

// TerrainFeedbackPolicy deliberately omits mutable DS. This native material
// repeats a small static CS low-two-bit sequence; it is not a DOS memory image.
const TerrainFeedbackPolicy = "native-periodic-source-CS-low2-DS-omitted-v1"
const TerrainFeedbackSamples = 8192

type TerrainFeedback struct {
	Policy           string `json:"policy"`
	SourceSHA256     string `json:"source_sha256"`
	SourceFileOffset int    `json:"source_file_offset"`
	SourceCSOffset   int    `json:"source_cs_offset"`
	SourceBytes      int    `json:"source_bytes"`
	SourceSliceHash  string `json:"source_slice_sha256"`
	PackedSHA256     string `json:"packed_sha256"`
	PackedLow2       []byte `json:"packed_low2"`
	lookup           string // Immutable after preparation/load; one byte per pair lookup.
}

// Initialize checks the native mapping and expands the 2KiB prepared sequence
// once. Drawing reads the immutable string; it never reads the mutable metadata.
func (m *TerrainFeedback) Initialize() error {
	if m != nil {
		m.lookup = ""
	}
	if m == nil || m.Policy != TerrainFeedbackPolicy || m.SourceFileOffset != 0x42550 || m.SourceCSOffset != 0x4000 || m.SourceBytes != TerrainFeedbackSamples || len(m.SourceSHA256) != 64 || len(m.SourceSliceHash) != 64 || len(m.PackedLow2) != TerrainFeedbackSamples/4 || hash(m.PackedLow2) != m.PackedSHA256 {
		return fmt.Errorf("terrain feedback metadata incompatible; rebuild prepared assets")
	}
	lookup := make([]byte, TerrainFeedbackSamples)
	for i := range lookup {
		lookup[i] = (m.PackedLow2[i>>2] >> (2 * (i & 3))) & 3
	}
	m.lookup = string(lookup)
	return nil
}

func (m *TerrainFeedback) Ready() bool { return m != nil && len(m.lookup) == TerrainFeedbackSamples }

// Low2 uses a named native periodic index. Source CS4000..5fff supplies the
// sequence; logical320 destination address modulo8192 selects its sample.
// There is no assertion that this is original CS at that screen address.
func (m *TerrainFeedback) Low2(logicalAddress int) byte {
	return m.lookup[logicalAddress&(TerrainFeedbackSamples-1)]
}
