package menus

import (
	"bytes"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"io"
	"os"
	"strings"
)

const officeWidth, officeHeight = 320, 200

func DefaultOfficeState() gamestate.OfficeState {
	return gamestate.OfficeState{PilotName: "Test Pilot Name", Squadron: "Test Squadron Name", Emblem: 1, Theatre: 1, Active: true}
}

// A partial JSON preset overrides the named fixture defaults. Unknown fields are
// errors so a misspelled selector does not quietly display a different state.
func ReadOfficeState(path string) (gamestate.OfficeState, error) {
	state := DefaultOfficeState()
	f, err := os.Open(path)
	if err != nil {
		return state, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 65537))
	if err != nil {
		return state, err
	}
	if len(raw) > 65536 {
		return state, fmt.Errorf("office state exceeds 65536 bytes")
	}
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '{' {
		return state, fmt.Errorf("office state must be a JSON object")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err = d.Decode(&state); err != nil {
		return state, fmt.Errorf("office state: %w", err)
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return state, fmt.Errorf("office state must contain one JSON object")
	}
	return state, nil
}

func validateOfficeState(s gamestate.OfficeState, m assetdata.OfficeMetadata) error {
	for _, v := range []struct {
		name     string
		n, limit int
	}{{"rank", s.Rank, len(m.Ranks)}, {"mission_type", s.MissionType, len(m.Missions)}, {"theatre", s.Theatre, len(m.Theatres)}, {"emblem", s.Emblem, 18}, {"computer_mode", s.ComputerMode, 4}} {
		if v.n < 0 || v.n >= v.limit {
			return fmt.Errorf("office state %s=%d is outside supported range 0..%d", v.name, v.n, v.limit-1)
		}
	}
	for _, v := range []struct {
		name, value string
		limit       int
	}{{"pilot_name", s.PilotName, 35}, {"squadron", s.Squadron, 25}} {
		if len(v.value) > v.limit {
			return fmt.Errorf("office %s exceeds %d original text bytes", v.name, v.limit)
		}
		for _, c := range []byte(v.value) {
			if c < 32 || c > 127 {
				return fmt.Errorf("office %s requires original single-byte characters 32..127", v.name)
			}
		}
	}
	return nil
}

type OfficeEvent struct {
	Operation string `json:"operation"`
	Key       int    `json:"key"`
	Frame     int    `json:"frame,omitempty"`
}

type officeAnimationState struct {
	Frame, Hold int
	Paused      bool
}

type officeBlock struct{ X, Y, W, H int }

type officeComputer struct {
	Reset                           bool
	Cursor, Threshold               int
	Blocks                          [4]officeBlock
	Count, CursorX, CursorY, Target int
	SavedPixel                      byte
	RedrawParity                    byte
}

// Atlases remain immutable. Scene is the persistent off-screen bank; pointer
// composition happens on a separate copy in Frame, avoiding save-under trails.
type Office struct {
	Assets                                  *assetdata.OfficeAssets
	State                                   gamestate.OfficeState
	Scene                                   []byte
	TickCount                               uint64
	LastEvents                              []OfficeEvent
	Animations                              []officeAnimationState
	OfficerGroup, OfficerCounter            int
	Computer                                officeComputer
	PointerX, PointerY                      int
	PointerInside                           bool
	HoverKey                                int
	pointerStyle, pointerFrame, pointerHold int
	rng                                     uint64
}

func NewOffice(a *assetdata.OfficeAssets, s gamestate.OfficeState, seed uint64) (*Office, error) {
	if a == nil {
		return nil, fmt.Errorf("nil office assets")
	}
	if err := validateOfficeState(s, a.Metadata); err != nil {
		return nil, err
	}
	for _, name := range []string{"BASE.PIC", "BASE_AN.PIC", "BASE_AN1.PIC", "BASE_AN2.PIC", "POINTER.PIC"} {
		if len(a.Images[name]) != officeWidth*officeHeight {
			return nil, fmt.Errorf("office %s requires 320x200 indices", name)
		}
	}
	if len(a.Metadata.Animations) != 5 {
		return nil, fmt.Errorf("office requires five animation controls")
	}
	if a.Fonts[3] == nil || a.Fonts[5] == nil {
		return nil, fmt.Errorf("office requires original bitmap fonts 3 and 5")
	}
	if seed == 0 {
		seed = 1
	}
	o := &Office{Assets: a, State: s, Scene: append([]byte(nil), a.Images["BASE.PIC"]...), OfficerGroup: -1, rng: seed}
	for _, control := range a.Metadata.Animations {
		o.Animations = append(o.Animations, officeAnimationState{Frame: control.Frame, Hold: control.Hold, Paused: control.Key != s.ComputerMode})
	}
	if err := o.prepareStatic(); err != nil {
		return nil, err
	}
	o.Computer.Reset = true
	if s.ComputerMode == 3 {
		o.generateBlock(0)
	}
	o.ForceRedraw()
	o.SetPointer(184, 70, true)
	// Initialization calls the shared callback once, independently of normal ticks.
	o.sharedUpdate()
	return o, nil
}

func (o *Office) prepareStatic() error {
	m := o.Assets.Metadata
	for _, segment := range m.Theatres[o.State.Theatre].Segments {
		if err := assetdata.OfficeLine(o.Scene, segment[0], segment[1], segment[2], segment[3], 4); err != nil {
			return err
		}
	}
	if err := o.drawLori(0, o.State.Emblem, 158, 52); err != nil {
		return err
	}
	label := assetdata.OfficePilotLabel(o.State.PilotName, m.Ranks[o.State.Rank].Text)
	for _, call := range assetdata.AtlasTextCommands(label, 157, 46, 60, 1) {
		if err := assetdata.OfficeBlit(o.Assets.Images["BASE_AN.PIC"], o.Scene, call.Source, call.Destination[0], call.Destination[1], true); err != nil {
			return err
		}
	}
	for _, call := range assetdata.AtlasTextCommands(o.State.Squadron, 169, 52, 48, 2) {
		if err := assetdata.OfficeBlit(o.Assets.Images["BASE_AN.PIC"], o.Scene, call.Source, call.Destination[0], call.Destination[1], true); err != nil {
			return err
		}
	}
	// The mission stripe copies zeros too; the other two LORI uses are masked.
	return o.drawLori(1, o.State.MissionType, 208, 163)
}

func (o *Office) drawLori(key, selector, x, y int) error {
	for _, l := range o.Assets.Metadata.Lori {
		if l.Key != key {
			continue
		}
		if l.Divisor <= 0 {
			return fmt.Errorf("invalid LORI divisor for key %d", key)
		}
		r := [4]int{l.Base[0] + selector/l.Divisor*l.Cell[0], l.Base[1] + selector%l.Divisor*l.Cell[1], l.Cell[0], l.Cell[1]}
		return assetdata.OfficeBlit(o.Assets.Images[l.SourceAsset], o.Scene, r, x, y, l.Mode != 0)
	}
	return fmt.Errorf("missing LORI key %d", key)
}

// ComposeGroup is also useful for matching the historical pre/post-group
// checkpoints. It never resets Scene to the background between sparse groups.
func (o *Office) ComposeGroup(key, callerX, callerY int) error {
	for _, g := range o.Assets.Metadata.Groups {
		if g.Key != key {
			continue
		}
		for _, index := range g.Records {
			if index < 0 || index >= len(o.Assets.Metadata.Components) {
				return fmt.Errorf("group %d has invalid record %d", key, index)
			}
			c := o.Assets.Metadata.Components[index]
			x := int(int16(uint16(c.Additive[0] + callerX)))
			y := int(int16(uint16(c.Additive[1] + callerY)))
			if err := assetdata.OfficeBlit(o.Assets.Images[c.SourceAsset], o.Scene, c.Source, x, y, true); err != nil {
				return fmt.Errorf("CMPX %d: %w", index, err)
			}
		}
		o.LastEvents = append(o.LastEvents, OfficeEvent{Operation: "officer_group", Key: key})
		return nil
	}
	return fmt.Errorf("unknown office group %d", key)
}

func (o *Office) Tick() {
	o.TickCount++
	o.LastEvents = o.LastEvents[:0]
	o.sharedUpdate()
	o.advanceAnimations()
	o.advancePointer()
}

func (o *Office) advancePointer() {
	o.pointerHold++
	if o.pointerHold >= 5 {
		o.pointerHold = 0
		o.pointerFrame = (o.pointerFrame + 1) % 6
	}
}

func (o *Office) sharedUpdate() {
	old := o.OfficerCounter
	o.OfficerCounter++
	if old >= 12 {
		o.OfficerCounter = 0
		o.OfficerGroup = (o.OfficerGroup + 1) % 11
		_ = o.ComposeGroup(o.OfficerGroup, 0, 0) // Metadata bounds were checked when loaded.
	}
}

func (o *Office) advanceAnimations() {
	for i, c := range o.Assets.Metadata.Animations {
		a := &o.Animations[i]
		if a.Paused {
			continue
		}
		a.Hold++
		// The native helper returns from the entire traversal on an unexpired hold.
		if a.Hold < c.HoldCount {
			return
		}
		a.Hold = 0
		a.Frame++
		if a.Frame >= c.FrameCount {
			a.Frame = 0
		}
		o.drawAnimation(i)
	}
}

func (o *Office) drawAnimation(i int) {
	c := o.Assets.Metadata.Animations[i]
	a := o.Animations[i]
	switch c.Key {
	case 0, 3:
		o.drawComputer(a.Frame)
	case 4:
		sy := 191
		if o.Computer.RedrawParity&1 != 0 {
			sy = 182
		}
		_ = assetdata.OfficeBlit(o.Assets.Images["BASE_AN2.PIC"], o.Scene, [4]int{0, sy, 15, 9}, 136, 85, false)
		o.Computer.RedrawParity++
	default:
		frame := c.Frames[a.Frame]
		_ = assetdata.OfficeBlit(o.Assets.Images[c.Asset], o.Scene, frame.Source, frame.Destination[0], frame.Destination[1], false)
	}
	o.LastEvents = append(o.LastEvents, OfficeEvent{Operation: "animation", Key: c.Key, Frame: a.Frame})
}

// Callback records are invoked even when paused during a forced redraw. Paused
// ordinary atlas records, however, do not draw with the office's zero flags.
func (o *Office) ForceRedraw() {
	for i := len(o.Animations) - 1; i >= 0; i-- {
		key := o.Assets.Metadata.Animations[i].Key
		if key == 0 || key == 3 || key == 4 || !o.Animations[i].Paused {
			o.drawAnimation(i)
		}
	}
}

func (o *Office) CycleComputerMode() {
	o.State.ComputerMode = (o.State.ComputerMode + 1) % 4
	for i, c := range o.Assets.Metadata.Animations {
		o.Animations[i].Paused = c.Key != o.State.ComputerMode
	}
	o.Computer.Reset = true
	if o.State.ComputerMode == 3 {
		o.generateBlock(0)
	}
	// A mode change activates its existing control state, not a fabricated frame0.
}

func (o *Office) randN(n int) int {
	if n <= 1 {
		return 0
	}
	// Reproducible local randomness is intentional; native shared RNG history is
	// neither needed nor claimed by this presentation-only implementation.
	x := o.rng
	x ^= x << 13
	x ^= x >> 7
	x ^= x << 17
	o.rng = x
	return int(x % uint64(n))
}

func (o *Office) randRange(lo, hi int) int { return lo + o.randN(hi-lo+1) }

func (o *Office) generateBlock(index int) {
	b := officeBlock{}
	b.X = o.randRange(1, 9)
	b.W = o.randRange(4, 13-b.X)
	if index != 0 {
		b.Y = o.randRange(0, 4)
	}
	b.H = o.randRange(3, 8-b.Y)
	o.Computer.Blocks[index] = b
}

func (o *Office) drawComputer(frame int) {
	switch o.State.ComputerMode {
	case 0:
		o.drawTerminal(frame)
	case 3:
		o.drawBlocks()
		// Both procedural callbacks still run on forced redraw in modes1/2, but the
		// original switch does no procedural work for those modes.
	}
}

func (o *Office) drawTerminal(frame int) {
	c := &o.Computer
	if c.Reset {
		assetdata.OfficeFill(o.Scene, 136, 85, 15, 9, 0)
		c.Cursor = 0
		c.Threshold = 0
		c.Reset = false
		return
	}
	if frame == 0 {
		c.Cursor = int(uint16(c.Cursor + 1))
	}
	if int16(c.Threshold) <= int16(c.Cursor) {
		_ = assetdata.OfficeBlit(o.Scene, o.Scene, [4]int{136, 86, 15, 8}, 136, 85, false)
		_ = assetdata.OfficeLine(o.Scene, 136, 93, 150, 93, 0)
		c.Threshold = o.randN(16)
		c.Cursor = 0
	}
	n := 4
	if c.Cursor == 0 {
		n = 3
	}
	colours := [4]byte{10, 2, 2, 0}
	o.Scene[93*320+136+c.Cursor] = colours[o.randN(n)]
	c.Reset = false
}

func (o *Office) drawBlocks() {
	c := &o.Computer
	redraw := c.Reset
	o.Scene[(85+c.CursorY)*320+136+c.CursorX] = c.SavedPixel
	target := c.Blocks[c.Target]
	c.CursorX = officeApproach(c.CursorX, target.X)
	c.CursorY = officeApproach(c.CursorY, target.Y)
	if c.CursorX == target.X && c.CursorY == target.Y {
		redraw = true
		if c.Count > c.Target || (c.Count == c.Target && c.Count < 3) {
			if c.Count == c.Target {
				c.Count++
			} else if c.Count-c.Target-1 == 0 {
				c.Count--
			}
			o.generateBlock(c.Count)
		}
		c.Target = o.randN(4)
	}
	if redraw {
		assetdata.OfficeFill(o.Scene, 136, 86, 15, 8, 9)
		_ = assetdata.OfficeLine(o.Scene, 136, 85, 150, 85, 11)
		for i := 0; i < c.Count; i++ {
			b := c.Blocks[i]
			x, y := 136+b.X, 85+b.Y
			inside, border := byte(3), byte(11)
			if i == 2 {
				if b.W < b.H {
					inside, border = 2, 10
				} else {
					inside, border = 4, 12
				}
			}
			assetdata.OfficeFill(o.Scene, x+1, y+1, b.W-2, b.H-2, inside)
			_ = assetdata.OfficeLine(o.Scene, x, y, x+b.W-1, y, border)
			_ = assetdata.OfficeLine(o.Scene, x, y+b.H-1, x+b.W-1, y+b.H-1, border)
			_ = assetdata.OfficeLine(o.Scene, x, y, x, y+b.H-1, border)
			_ = assetdata.OfficeLine(o.Scene, x+b.W-1, y, x+b.W-1, y+b.H-1, border)
			_ = assetdata.OfficeLine(o.Scene, x+b.W, y+1, x+b.W, y+b.H, 0)
			_ = assetdata.OfficeLine(o.Scene, x+1, y+b.H, x+b.W, y+b.H, 0)
		}
	}
	pos := (85+c.CursorY)*320 + 136 + c.CursorX
	c.SavedPixel = o.Scene[pos]
	o.Scene[pos] = 15
	c.Reset = false
}

func officeApproach(v, target int) int {
	if v < target {
		return v + 1
	}
	if v > target {
		return v - 1
	}
	return v
}

func (o *Office) SetPointer(x, y int, inside bool) {
	o.PointerX, o.PointerY, o.PointerInside = x, y, inside
	o.HoverKey = -1
	style := 0
	if inside {
		for i := len(o.Assets.Metadata.Areas) - 1; i >= 0; i-- {
			a := o.Assets.Metadata.Areas[i]
			r := a.Rectangle
			if x >= r[0] && x < r[0]+r[2] && y >= r[1] && y < r[1]+r[3] {
				o.HoverKey = a.Key
				if a.VisualFlag != 0 {
					style = 1
				}
				break
			}
		}
	}
	if style != o.pointerStyle {
		o.pointerStyle = style
		o.pointerFrame = 0
		o.pointerHold = 0
	}
}

func (o *Office) Caption() (string, int, byte) {
	m := o.Assets.Metadata
	k := o.HoverKey
	if (k == 2 || k == 3 || k == 4) && !o.State.Active {
		return m.Denials[0].Text, 3, 12
	}
	if k == 4 && o.State.MissionType == 0 && o.State.Theatre > 1 {
		return m.Denials[1].Text, 3, 12
	}
	if k == 9 && !o.State.SnapshotAvailable {
		return m.Denials[2].Text, 3, 12
	}
	if k == 2 && o.State.MissionType == 4 {
		k = 10
	} else if k < 0 || (k >= 5 && k < 8) {
		k = 5
	}
	text := ""
	for _, caption := range m.Captions {
		if caption.Selector == k {
			text = caption.Text
			break
		}
	}
	if k == 2 || k == 10 {
		text += m.Theatres[o.State.Theatre].Label
	}
	if k == 3 {
		text = strings.ToUpper(text + m.Missions[o.State.MissionType].Text)
	}
	font := 5
	if k < 1 {
		font = 3
	}
	return text, font, 7
}

func (o *Office) Frame(includePointer bool) []byte {
	frame := append([]byte(nil), o.Scene...)
	assetdata.OfficeFill(frame, 0, 191, 320, 9, 0)
	caption, font, colour := o.Caption()
	f := o.Assets.Fonts[font]
	assetdata.DrawBottomTooltip(frame, f, caption, int(colour))
	if includePointer {
		o.drawPointer(frame)
	}
	return frame
}

func (o *Office) drawPointer(frame []byte) {
	if o.PointerInside {
		sequence := [6]int{0, 1, 2, 3, 2, 1}
		hot := 0
		base := 0
		if o.pointerStyle != 0 {
			hot = 7
			base = 4
		}
		source := o.Assets.Images["POINTER.PIC"]
		sx := (base + sequence[o.pointerFrame]) * 16
		for y := 0; y < 16; y++ {
			dy := o.PointerY - hot + y
			if dy < 0 || dy >= 191 {
				continue
			}
			for x := 0; x < 16; x++ {
				dx := o.PointerX - hot + x
				if dx < 0 || dx >= 320 {
					continue
				}
				v := source[y*320+sx+x]
				if v != 0 {
					frame[dy*320+dx] = v
				}
			}
		}
	}
}
