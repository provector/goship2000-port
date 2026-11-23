// Package render3d draws prepared indexed scenes. It has no window, save or
// simulation dependency; the application supplies poses and logical phases.
package render3d

import (
	"fmt"
	"gunship2000-assets/game/world"
	"gunship2000-assets/internal/rasterlayout"
	"image/color"
)

const Width, Height = 320, 200

type Pose struct {
	H0, H1 uint32
	Height uint16
}

type Camera struct {
	Pose
	Angles [3]uint16
}

// Rect has inclusive logical bounds. Sprite/font clipping is a separate caller
// context and must not be substituted for the world viewport.
type Rect struct{ Left, Top, Right, Bottom int }
type Point struct{ X, Y int }
type Matrix [3][3]int16
type Vector [3]int32

type View struct {
	Bounds     Rect
	Center     Point
	DepthScale uint8
}

func FullView() View {
	return View{Bounds: Rect{0, 0, 319, 199}, Center: Point{160, 70}}
}

type Instance struct {
	MutableModel *world.MutableModel `json:",omitempty"` // R23 source-owned per-instance model; prepared geometry remains immutable.
	GeometryID   string              // Embedded source combat geometry, never an OBJFILE model ID.
	Impact       bool                // Resident30e2 depth/phase-selected sprite callback.
	Explosion    bool                // Shared2cff:668 ->6a0 source vertex operation.
	ID           string
	ModelID      int
	Pose
	Angles     [3]uint16
	Callback   uint8
	Oriented   bool
	GraphChild *GraphChildOverride
}

// oriented is the shared source instance-transform admission. Several source
// callbacks carry orientation without the explicit native Oriented flag.
func (in Instance) oriented() bool {
	return in.Oriented || in.Callback == 1 || in.Callback == 2 || in.Callback == 4 || in.Callback == 6
}

// GraphChildOverride is a frame-consumed semantic component input. It can only
// retain or disable the declared original edge; shared geometry stays immutable.
type GraphChildOverride struct {
	Node, Side, Child, Expected int
}

// Carrier supplies validated live source memory for generic high-byte-zero
// materials (scenery as well as wrecks). No universal carrier is invented.
type Carrier struct {
	CS, DS []byte
	// Readable bounds BOTH source reads. Nil means the caller validated both
	// complete carriers; a partial capture must declare its admitted ranges.
	// These are logical320-byte row addresses even for a640-pixel destination.
	Readable []CarrierRange
}

type CarrierRange struct{ First, End int } // Half-open source byte coordinates.

// PoseFraction retains only the sub-unit remainder of an interpolated display
// pose. Axes are world H0, height, H1; angles use the original 65536-unit turn.
type PoseFraction struct {
	Position, Angles [3]float64
}

type Scene struct {
	FloatingGeometry  bool                    `json:"-"`
	CameraFraction    PoseFraction            `json:"-"`
	InstanceFractions map[string]PoseFraction `json:"-"`
	// Preserve canonical indexed fills when drawing avionics reference images.
	SourceFillReference bool `json:",omitempty"`
	// SkyMask is an optional presentation-only coverage/pair output. Canonical
	// indexed pixels remain identical; geometry erases its covered destinations.
	// TokenB4 additionally identifies the EURO6 brown terrain memory-fallback leaf
	// for the named postcanonical material presentation policy.
	GroundOwners      []byte         `json:"-"` // Optional exact source-writer/layer plane, distinct from recipe admission.
	SkyMask           []byte         `json:"-"`
	CombatPhase       uint16         // Source DA supplied by the logical combat owner.
	ExplosionVertices []world.Vertex // Final shared logical668/6a0 stream, immutable during Draw.
	// RenderScale is a native presentation policy. It
	// never changes source coordinates, query geometry, visibility or logical clocks.
	Resolution      string `json:",omitempty"`
	RenderScale     int
	World           string
	Camera          Camera
	View            View
	Instances       []Instance
	GraphicsOptions byte
	PaletteBank     string
	Environment     *EnvironmentState
	MaterialCarrier *Carrier
	// These are source setup thresholds, not physical distances or a new LOD.
	NearReference, OutlineThreshold, ExtentLimit int16
}

func (s Scene) view() View {
	if s.View.Bounds.Right == 0 && s.View.Bounds.Bottom == 0 {
		return FullView()
	}
	return s.View
}

type Stats struct {
	Cells, Placements, Instances, Primitives int
	Rejected, ArithmeticRejected             int
	SourceMemoryFallbackPrimitives           int
	NativeTerrainFeedbackPrimitives          int
	Status                                   []string
}

func (s *Stats) status(message string) {
	for _, old := range s.Status {
		if old == message {
			return
		}
	}
	s.Status = append(s.Status, message)
}

type Renderer struct {
	Assets    *world.Bundle
	materials *materialRegistry
	// Draw is serialized by the scene owner. Scratch has no simulation lifetime;
	// it is fully reset for every draw, including resize and diagnostic requests.
	ownerScratch, styleScratch []byte
	floatScratch               floatWorkspace
}

func New(assets *world.Bundle) (*Renderer, error) {
	if assets == nil || len(assets.Sine) != 1025 {
		return nil, fmt.Errorf("rendering requires a prepared world bundle and 1025-word sine table")
	}
	return &Renderer{Assets: assets, materials: compileMaterialRegistry(assets)}, nil
}

// VisualState owns source-shared geometry copies and preview phase. Draw never
// advances this state. The chosen preview cadence is deliberately not sim Hz.
type VisualState struct {
	Phase     uint64
	Wave      uint8
	streams   map[string][]world.Vertex
	rotations map[string]bool // Last logical step's exclusively selector3-mutated streams.
	// Native presentation only: instance-local rotor poses cannot change the
	// canonical source-shared model streams or another aircraft's stopped rotor.
	instanceVertices map[string][]world.Vertex
}

func (r *Renderer) Palette(scene Scene, state *VisualState) (color.Palette, error) {
	bank := scene.PaletteBank
	if bank == "" {
		bank = "DAY"
	}
	if len(scene.World) < 4 {
		return nil, fmt.Errorf("invalid world palette prefix %q", scene.World)
	}
	id := scene.World[:4] + bank + ".PAL"
	p, ok := r.Assets.Palettes[id]
	if !ok {
		p, ok = r.Assets.Palettes[scene.World[:4]+bank]
	}
	if !ok || len(p.RGB6) != 768 {
		return nil, fmt.Errorf("prepared palette %s is unavailable", id)
	}
	rgb := append([]byte(nil), p.RGB6...)
	copy(rgb[39:42], rgb[45:48])
	if state != nil && scene.GraphicsOptions&0x10 != 0 {
		phase := int(state.Phase % 6)
		for i := 0; i < 6; i++ {
			copy(rgb[3*(154+i):3*(155+i)], p.RGB6[3*(154+(i+phase)%6):3*(155+(i+phase)%6)])
		}
	}
	scene.Environment.applyPalette(rgb)
	return (world.Palette{RGB6: rgb}).Colours(), nil
}

// Scale defaults to the reference target for existing programmatic callers.
func (s Scene) Scale() int {
	if s.Layout().Height == 400 {
		return 2
	}
	return 1
}
func (s Scene) Layout() *rasterlayout.Layout {
	return rasterlayout.Resolve(s.Resolution, s.RenderScale)
}
func (s Scene) Dimensions() (int, int) { l := s.Layout(); return l.Width, l.Height }
func (s Scene) TargetPixels() int      { w, h := s.Dimensions(); return w * h }
func (s Scene) rasterView() View {
	v := s.view()
	l := s.Layout()
	v.Bounds = Rect{l.X[v.Bounds.Left], l.Y[v.Bounds.Top], l.X[v.Bounds.Right+1] - 1, l.Y[v.Bounds.Bottom+1] - 1}
	v.Center = Point{v.Center.X * l.Width / Width, l.Top + v.Center.Y*l.PictureHeight/Height}
	return v
}
