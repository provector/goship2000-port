// Package cockpit composes the recovered indexed artwork and an explicit subset
// of supplied-state instruments. It owns no simulation clock or pilot state.
package cockpit

import "fmt"

const Schema = 3

type Image struct {
	Width, Height int
	Pixels        []byte `json:"-"`
	FullMask      []byte `json:"-"`
	Split, Bottom int
}

type Definition struct {
	Parameters      [74]int16
	SpanReplacement uint16
	Colours         [6]byte
	Raw             []byte
	StateParameters []byte
}

type Font struct {
	First, Last, Width, Height int
	Advances                   []byte
	Rows                       []byte
}

type Vector struct {
	Colour byte
	Points [][2]int16
}

type Assets struct {
	Schema                int
	Images                map[string]Image
	Definitions           map[string]Definition
	Atlases               map[string]Image
	Sprites               map[string]Image
	Font                  Font
	Needles               [128][2]int16
	Sine                  []int16
	Vectors               []Vector
	TargetDossiers        map[int][4]string `json:",omitempty"` // Derived read-only from hash-verified existing rawGS2, not new media.
	MemberCaptionLiterals map[string]string `json:",omitempty"`
	Literals              map[string]string
	MapVectors            map[string][]Vector
	Opaque                map[string][]byte
	ScenePresentation     ScenePresentation
	NightPalette          []byte `json:"-"` // Selected-VGA13e1 RGB6; GPS temporarily installs world staging instead.
}

// Family accepts the preflight SHHD aircraft ID. The original launch writer
// remaps that ID before GS2 uses SHIP's low three bits to select cockpit art.
func Family(aircraft int) (string, error) {
	if aircraft < 0 || aircraft > 7 {
		return "", fmt.Errorf("cockpit aircraft ID %d outside0..7", aircraft)
	}
	shipSelector := [8]int{0, 1, 2, 3, 5, 6, 4, 4}[aircraft]
	return [8]string{"APA", "APA", "COB", "DEF", "COM", "KIA", "BLK", "BLK"}[shipSelector], nil
}

func (a *Assets) Validate() error {
	if a == nil || a.Schema != Schema {
		return fmt.Errorf("unsupported cockpit schema")
	}
	if err := a.ScenePresentation.Validate(); err != nil {
		return err
	}
	if len(a.NightPalette) != 768 {
		return fmt.Errorf("missing selected-VGA night palette bank")
	}
	for _, family := range []string{"APA", "COB", "DEF", "COM", "KIA", "BLK"} {
		i, ok := a.Images[family+"_CPIT.PIC"]
		if !ok || i.Width != 320 || i.Height != 200 || len(i.Pixels) != 64000 || len(i.FullMask) != 64000 || i.Split < 0 || i.Split >= 64000 || i.Bottom < 0 || i.Bottom >= 200 {
			return fmt.Errorf("invalid prepared %s cockpit artwork/mask", family)
		}
		if len(a.Definitions[family].Raw) != 156 {
			return fmt.Errorf("missing %s cockpit definition", family)
		}
	}
	if len(a.Font.Rows) != 95*6 || len(a.Font.Advances) != 95 || len(a.Sine) != 1025 {
		return fmt.Errorf("invalid HUD font/sine table")
	}
	for _, name := range []string{"heading-strip", "vertical-strip", "fixed-strip", "fixed-scale", "digits-opaque", "digits-zero-mask"} {
		i, ok := a.Atlases[name]
		if !ok || i.Width <= 0 || i.Height <= 0 || len(i.Pixels) != i.Width*i.Height {
			return fmt.Errorf("missing HUD atlas %s", name)
		}
	}
	return nil
}

func (a *Assets) Sin(angle uint16) int16 {
	x := int(angle >> 4)
	negative := x&0x800 != 0
	x &= 0x7ff
	if x&0x400 != 0 {
		x = 0x800 - x
	}
	v := a.Sine[x]
	if negative {
		return -v
	}
	return v
}
