// Package controllers owns shared read-only profiles and raw input interpretation.
// SETUP alone writes profiles; the game adapts them at its input boundary.
package controllers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

const Filename = "controls.cfg"
const Version = 4
const PreviewDeadZone = 0.05
const CaptureThreshold = 0.35

var Basic = []string{"flight.pitch", "flight.roll", "flight.collective", "flight.yaw", "menu.selector1", "menu.selector2", "weapon.weapon"}

type Device struct {
	Instance                    int32 `json:"instance,omitempty"`
	Backend, GUID, Serial, Name string
	Path                        string `json:"path,omitempty"`
	Vendor, Product, Revision   uint16
	Axes, Buttons, Hats         int
}

func (d Device) Key() string {
	b, _ := json.Marshal(struct {
		Backend, GUID, Serial, Name string
		Vendor, Product, Revision   uint16
		Axes, Buttons, Hats         int
	}{d.Backend, d.GUID, d.Serial, d.Name, d.Vendor, d.Product, d.Revision, d.Axes, d.Buttons, d.Hats})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

type State struct {
	Axes    []float64
	Buttons []bool
	Hats    []uint8
}
type Snapshot struct {
	Devices    []Device
	States     map[int32]State
	Error      string
	Subsystems uint32
}
type Backend interface {
	Poll() (Snapshot, error)
	Close()
}
type Control struct {
	Kind      string `json:"kind"`
	Index     int    `json:"index"`
	Direction uint8  `json:"direction,omitempty"`
	Sign      int8   `json:"sign,omitempty"`
}

func (c Control) String() string {
	if c.Kind == "axis_endpoint" {
		sign := "+"
		if c.Sign < 0 {
			sign = "-"
		}
		return fmt.Sprintf("Axis %d %s endpoint", c.Index+1, sign)
	}
	if c.Kind == "button" {
		return fmt.Sprintf("Button %02d", c.Index+1)
	}
	names := map[uint8]string{1: "Up", 2: "Right", 4: "Down", 8: "Left", 3: "Up-Right", 6: "Down-Right", 9: "Up-Left", 12: "Down-Left"}
	return fmt.Sprintf("Hat %d %s", c.Index+1, names[c.Direction])
}
func (c Control) Held(s State) bool {
	if c.Kind == "axis_endpoint" {
		return c.Index >= 0 && c.Index < len(s.Axes) && float64(c.Sign)*s.Axes[c.Index] >= 0.95
	}
	if c.Kind == "button" {
		return c.Index >= 0 && c.Index < len(s.Buttons) && s.Buttons[c.Index]
	}
	return c.Kind == "hat" && c.Index >= 0 && c.Index < len(s.Hats) && s.Hats[c.Index] == c.Direction
}
func (c Control) Validate() error {
	if c.Index < 0 || c.Index >= 512 {
		return fmt.Errorf("invalid control index")
	}
	if c.Kind == "axis_endpoint" && c.Index < 128 && c.Direction == 0 && (c.Sign == -1 || c.Sign == 1) {
		return nil
	}
	if c.Sign != 0 {
		return fmt.Errorf("sign only applies to axis endpoints")
	}
	if c.Kind == "button" && c.Direction == 0 {
		return nil
	}
	if c.Kind == "hat" && c.Index < 16 && (c.Direction == 1 || c.Direction == 2 || c.Direction == 4 || c.Direction == 8 || c.Direction == 3 || c.Direction == 6 || c.Direction == 9 || c.Direction == 12) {
		return nil
	}
	return fmt.Errorf("invalid discrete control")
}

type Binding struct {
	Kind     string   `json:"kind"`
	Axis     int      `json:"axis,omitempty"`
	Negative *Control `json:"negative,omitempty"`
	Positive *Control `json:"positive,omitempty"`
	Control  *Control `json:"control,omitempty"`
	Invert   bool     `json:"invert,omitempty"`
}

func (b Binding) String() string {
	switch b.Kind {
	case "axis":
		return fmt.Sprintf("Axis %d", b.Axis+1)
	case "pair":
		n, p := "<Unassigned>", "<Unassigned>"
		if b.Negative != nil {
			n = b.Negative.String()
		}
		if b.Positive != nil {
			p = b.Positive.String()
		}
		return n + " / " + p
	case "control":
		return b.Control.String()
	}
	return "—"
}
func (b Binding) Uses() []Control {
	if b.Kind == "control" && b.Control != nil {
		return []Control{*b.Control}
	}
	if b.Kind == "pair" {
		var out []Control
		if b.Negative != nil {
			out = append(out, *b.Negative)
		}
		if b.Positive != nil {
			out = append(out, *b.Positive)
		}
		return out
	}
	return nil
}
func (b Binding) Validate(axis bool) error {
	switch b.Kind {
	case "":
		if b.Axis == 0 && b.Control == nil && b.Negative == nil && b.Positive == nil && (!b.Invert || axis) {
			return nil
		}
	case "axis":
		if axis && b.Axis >= 0 && b.Axis < 128 && b.Control == nil && b.Negative == nil && b.Positive == nil {
			return nil
		}
	case "pair":
		if axis && b.Control == nil && b.Axis == 0 && b.Negative != nil && b.Positive != nil && *b.Negative != *b.Positive {
			if e := b.Negative.Validate(); e != nil {
				return e
			}
			return b.Positive.Validate()
		}
	case "control":
		if !axis && !b.Invert && b.Axis == 0 && b.Control != nil && b.Negative == nil && b.Positive == nil {
			return b.Control.Validate()
		}
	}
	return fmt.Errorf("invalid binding for action")
}

type Profile struct {
	AxisConvention   string             `json:"axis_convention,omitempty"`
	CollectiveSource string             `json:"collective_source,omitempty"`
	ID               string             `json:"id"`
	Device           Device             `json:"device"`
	CollectiveMode   string             `json:"collective_mode"`
	Bindings         map[string]Binding `json:"bindings"`
	AxisButtons      map[string]Binding `json:"axis_buttons,omitempty"`
}
type Config struct {
	Version  int                `json:"version"`
	Selected string             `json:"selected_profile,omitempty"`
	Profiles map[string]Profile `json:"profiles"`
}

func Defaults() Config { return Config{Version: Version, Profiles: map[string]Profile{}} }
func Clone(c Config) Config {
	b, _ := json.Marshal(c)
	var out Config
	json.Unmarshal(b, &out)
	return out
}
func (c Config) Validate() error {
	if c.Version < 1 || c.Version > Version {
		return fmt.Errorf("version: supported version is %d (found %d)", Version, c.Version)
	}
	if c.Profiles == nil || len(c.Profiles) > 128 {
		return fmt.Errorf("profiles: object with at most128profiles required")
	}
	if c.Selected != "" {
		if _, ok := c.Profiles[c.Selected]; !ok {
			return fmt.Errorf("selected_profile: unknown profile")
		}
	}
	for id, p := range c.Profiles {
		if id == "" || len(id) > 128 || id != p.ID || len(p.Device.Name) > 512 || p.Device.Backend == "" || p.Device.Axes < 0 || p.Device.Axes > 128 || p.Device.Buttons < 0 || p.Device.Buttons > 512 || p.Device.Hats < 0 || p.Device.Hats > 16 {
			return fmt.Errorf("profile %q: invalid identity/capabilities", id)
		}
		if p.CollectiveMode != "" && p.CollectiveMode != "absolute" && p.CollectiveMode != "relative" && p.CollectiveMode != "relative_height" {
			return fmt.Errorf("profile %s collective_mode: absolute, relative or relative_height required", id)
		}
		if c.Version < 4 && p.CollectiveMode == "" {
			return fmt.Errorf("unspecified collective mode requires controls version4")
		}
		if p.AxisConvention != "" && p.AxisConvention != "native-v1" {
			return fmt.Errorf("unknown axis convention")
		}
		if p.CollectiveSource != "" && p.CollectiveSource != "centered" && p.CollectiveSource != "slider" {
			return fmt.Errorf("collective_source must be centered or slider")
		}
		if c.Version < 3 && (p.AxisConvention != "" || p.CollectiveSource != "" || p.CollectiveMode == "relative_height") {
			return fmt.Errorf("profile requires controls version3")
		}
		if p.Bindings == nil || len(p.Bindings) > 512 {
			return fmt.Errorf("too many bindings")
		}
		if c.Version == 1 && (len(p.AxisButtons) > 0 || RequiresVersion(Config{Version: 1, Profiles: map[string]Profile{id: p}}) > 1) {
			return fmt.Errorf("profile %s: new direction data requires controls version2", id)
		}
		if len(p.AxisButtons) > 4 {
			return fmt.Errorf("too many button axes")
		}
		for axis, b := range p.AxisButtons {
			a, ok := ActionByID(axis)
			if !ok || !a.Axis || b.Invert {
				return fmt.Errorf("%s: invalid button-axis owner", axis)
			}
			if e := partialPairValid(b); e != nil {
				return e
			}
			for _, v := range b.Uses() {
				if v.Kind == "button" && v.Index >= p.Device.Buttons || v.Kind == "hat" && v.Index >= p.Device.Hats || v.Kind == "axis_endpoint" && v.Index >= p.Device.Axes {
					return fmt.Errorf("%s: direction control absent from device", axis)
				}
			}
		}
		for key, b := range p.Bindings {
			if _, direction := DirectionByID(key); direction {
				return fmt.Errorf("%s: direction belongs in its axis pair, not bindings", key)
			}
			a, ok := ActionByID(key)
			if !ok {
				return fmt.Errorf("profile %s: unknown action %s", id, key)
			}
			e := b.Validate(a.Axis)
			if c.Version >= 2 && a.Axis && b.Kind == "pair" {
				e = partialPairValid(b)
			}
			if e != nil {
				return fmt.Errorf("%s: %w", key, e)
			}
			if b.Kind == "axis" && b.Axis >= p.Device.Axes {
				return fmt.Errorf("%s: axis absent from saved device layout", key)
			}
			for _, v := range b.Uses() {
				if v.Kind == "button" && v.Index >= p.Device.Buttons || v.Kind == "hat" && v.Index >= p.Device.Hats || v.Kind == "axis_endpoint" && v.Index >= p.Device.Axes {
					return fmt.Errorf("%s: control absent from saved device layout", key)
				}
			}
		}
	}
	return nil
}
func (b Binding) Value(s State, absolute bool) float64 {
	v := 0.0
	if b.Kind == "axis" && b.Axis < len(s.Axes) && b.Axis >= 0 {
		v = s.Axes[b.Axis]
	} else if b.Kind == "pair" {
		if b.Negative != nil && b.Negative.Held(s) {
			v--
		}
		if b.Positive != nil && b.Positive.Held(s) {
			v++
		}
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	v = math.Max(-1, math.Min(1, v))
	if !absolute && math.Abs(v) <= PreviewDeadZone {
		v = 0
	}
	if b.Invert {
		v = -v
	}
	return v
}
func Conflicts(p Profile, id string, b Binding) []string {
	a, _ := MappingAction(id)
	var out []string
	checks := map[string]Binding{}
	for key, b := range p.Bindings {
		checks[key] = b
	}
	for _, d := range AxisDirections {
		b, _ := EffectiveBinding(p, d.ID)
		checks[d.ID] = b
	}
	for key, old := range checks {
		if a, ok := ActionByID(key); ok && a.Axis && old.Kind == "pair" {
			continue
		}
		if d, ok := DirectionByID(key); ok && id == d.Axis {
			continue
		}
		if LogicalID(key) == LogicalID(id) || old.Kind == "" {
			continue
		}
		other, _ := MappingAction(key)
		same := false
		for _, x := range a.Contexts {
			for _, y := range other.Contexts {
				same = same || x == y || x == "*" || y == "*"
			}
		}
		if !same {
			continue
		}
		hit := old.Kind == "axis" && b.Kind == "axis" && old.Axis == b.Axis
		for _, x := range old.Uses() {
			for _, y := range b.Uses() {
				hit = hit || x == y
			}
		}
		if hit {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}

// Only a unique candidate AND unique saved profile can auto-reattach. A selected
// indistinguishable instance can be attached explicitly for this process only.
func Resolve(c Config, s Snapshot, explicit int32) (Device, bool, string) {
	p, ok := c.Profiles[c.Selected]
	if !ok {
		return Device{}, false, "<None>"
	}
	var matches []Device
	profiles := 0
	for _, q := range c.Profiles {
		if q.Device.Key() == p.Device.Key() {
			profiles++
		}
	}
	for _, d := range s.Devices {
		if d.Key() == p.Device.Key() {
			matches = append(matches, d)
			if explicit >= 0 && d.Instance == explicit {
				return d, true, ""
			}
		}
	}
	if len(matches) == 1 && profiles == 1 {
		return matches[0], true, ""
	}
	if len(matches) > 0 {
		return Device{}, false, "Identical devices/profiles: choose explicitly"
	}
	if s.Error != "" {
		return Device{}, false, s.Error
	}
	return Device{}, false, "Disconnected / unavailable"
}
func PairPrompt(id string, positive bool) string {
	if id == "flight.roll" || id == "flight.yaw" {
		if positive {
			return "Right"
		}
		return "Left"
	}
	if id == "flight.pitch" {
		if positive {
			return "Down"
		}
		return "Up"
	}
	if positive {
		return "Increase"
	}
	return "Decrease"
}
func Label(id string) string {
	a, ok := ActionByID(id)
	if ok {
		return a.Label
	}
	return strings.TrimPrefix(id, "flight.")
}

// AxisValue maps raw SDL signs to the source flight input convention exactly
// once, then applies the user's saved inversion. Legacy profiles keep their
// effective flight mapping; new/reset profiles use native-v1 with Invert Off.
func AxisValue(p Profile, id string, s State, absolute bool) float64 {
	b := p.Bindings[id]
	invert := b.Invert
	b.Invert = false
	v := b.Value(s, absolute)
	if p.AxisConvention == "native-v1" && b.Kind == "axis" && id == "flight.collective" {
		v = -v
	}
	if invert {
		v = -v
	}
	return v
}
