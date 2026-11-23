// Package world holds prepared visual/query data independently of menus and saves.
package world

import "image/color"

const Schema = 2

// Vertex remains in original (H0,height,H1) units after GLB import conversion.
// GLB stores (X,Y,-Z); source planes and integer queries keep native coordinates.
type Vertex [3]int16
type Plane [4]int32

type Primitive struct {
	Opcode        byte   `json:"opcode"`
	Colour        uint16 `json:"colour"`
	Flags         []byte `json:"flags,omitempty"`
	Indices       []int  `json:"indices"`
	Radius        int    `json:"radius,omitempty"`
	TriangleRange [2]int `json:"glb_triangle_range"`
}

type Group struct {
	TwoSided  bool       `json:"two_sided,omitempty"`
	Primitive *Primitive `json:"primitive,omitempty"`
	Children  []Group    `json:"children,omitempty"`
}

type HeightRegion struct {
	Threshold int16      `json:"threshold"`
	Points    [][2]int16 `json:"closed_points"`
	Plane     Plane      `json:"plane"`
	Tail      uint16     `json:"raw_tail"`
}

// Generator compiles the encountered compact command3 into explicit shifts and
// cluster offsets. Its geometry is generated from the current view, never baked
// at an arbitrary camera. Other compact families use ordinary GLB vertices.
type Generator struct {
	Kind          string     `json:"kind"`
	Shifts        [3]int     `json:"shifts"`
	ClusterDeltas [][2]int16 `json:"cluster_deltas"`
}

type Geometry struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	GLB             string         `json:"glb"`
	Raw             string         `json:"raw"`
	SourceHash      string         `json:"source_sha256"`
	SourceOffset    int            `json:"source_offset"`
	SourceLength    int            `json:"source_length"`
	Vertices        []Vertex       `json:"-"`
	Planes          []Plane        `json:"planes,omitempty"`
	Root            int            `json:"root"`
	Children        [][2]int       `json:"children,omitempty"`
	PlaneGroups     []Group        `json:"plane_groups,omitempty"`
	DirectGroups    []Group        `json:"direct_groups,omitempty"`
	Heights         []HeightRegion `json:"height_regions,omitempty"`
	Generator       *Generator     `json:"generator,omitempty"`
	CompactCallback int            `json:"compact_callback,omitempty"`
	// EmbeddedPreparation is derived from the manifest-verified retained GS2
	// carrier on load. It is distinct from world compact callbacks/cache families
	// and adds no prepared-file or historical resource identity.
	EmbeddedPreparation EmbeddedVertexPreparation `json:"-"`
	Extent              int16                     `json:"extent"`
	Status              []string                  `json:"status,omitempty"`
}

type Placement struct {
	ID               string `json:"id"`
	Position         Vertex `json:"position"`
	Geometry         string `json:"geometry"`
	Raw              []byte `json:"raw"`
	DistantReference uint16 `json:"distant_reference_raw,omitempty"`
	DistantGeometry  string `json:"distant_geometry,omitempty"`
}

type Attachment struct {
	Plane      int         `json:"plane"`
	Side       int         `json:"side"`
	Placements []Placement `json:"placements"`
}

type Cell struct {
	ID          int          `json:"id"`
	Geometry    string       `json:"geometry"`
	Attachments []Attachment `json:"attachments,omitempty"`
	Raw         []byte       `json:"raw"`
}

type World struct {
	ID                 string `json:"id"`
	SourceHash         string `json:"source_sha256"`
	Grid               []byte `json:"grid"`
	Cells              []Cell `json:"cells"`
	TileAtlas          []byte `json:"tile_atlas"`
	Overview           []byte `json:"overview_indices,omitempty"`
	OverviewSourceHash string `json:"overview_source_sha256,omitempty"`
	Raw                string `json:"raw"`
}

type Palette struct {
	ID   string `json:"id"`
	RGB6 []byte `json:"rgb6"`
}

func (p Palette) Colours() color.Palette {
	out := make(color.Palette, 256)
	for i := range out {
		if len(p.RGB6) >= 3*i+3 {
			out[i] = color.RGBA{p.RGB6[3*i]<<2 | p.RGB6[3*i]>>4, p.RGB6[3*i+1]<<2 | p.RGB6[3*i+1]>>4, p.RGB6[3*i+2]<<2 | p.RGB6[3*i+2]>>4, 255}
		}
	}
	return out
}

type Modifier struct {
	ID             int    `json:"id"`
	Kind           string `json:"kind"`
	Vertices       int    `json:"vertices"`
	SourceEntry    uint16 `json:"source_entry_original_IP"`
	CoefficientQ15 int16  `json:"coefficient_q15,omitempty"`
	Guard          byte   `json:"guard"`
	Ownership      string `json:"ownership"`
	Transform      string `json:"transform_after"`
}

type Bundle struct {
	TerrainFeedback *TerrainFeedback     `json:"terrain_feedback,omitempty"`
	Combat          *CombatPresentation  `json:"combat,omitempty"`
	Schema          int                  `json:"schema"`
	Profile         string               `json:"profile"`
	GraphicsOptions byte                 `json:"graphics_options"`
	GraphicsMode    byte                 `json:"graphics_mode"`
	Models          []string             `json:"models"`
	Geometries      map[string]*Geometry `json:"geometries"`
	Worlds          map[string]*World    `json:"worlds"`
	Palettes        map[string]Palette   `json:"palettes"`
	Modifiers       []Modifier           `json:"modifiers"`
	Sine            []int16              `json:"sine_q14"`
	AngleBases      []uint16             `json:"angle_bases"`
	Atan            []uint16             `json:"atan_lookup"`
	WeatherRemap    []byte               `json:"weather_destination_remap"`
	Sources         map[string]string    `json:"source_sha256"`
	Notes           []string             `json:"notes"`
}
