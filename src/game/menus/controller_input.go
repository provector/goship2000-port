package menus

import (
	"fmt"
	"gunship2000-assets/game/controllers"
	"gunship2000-assets/game/diagnostics"
	"gunship2000-assets/game/flightcontrol"
	"gunship2000-assets/game/sdlraw"
	"gunship2000-assets/internal/branding"
	"math"
	"os"
	"strings"
	"time"
)

type gameController struct {
	trace                *flightInputTrace
	swapYawRoll          bool
	live                 *controllers.Live
	status               string
	collectivePolicy     string
	generation           uint64
	previous             map[string]bool
	pointerOwned         bool
	xFraction, yFraction float64
	altArrowGate         [2]bool
}

func openGameController(directory string) *gameController {
	g := &gameController{previous: map[string]bool{}}
	if branding.DeveloperTools && os.Getenv("GS2000_INPUT_TRACE") == "1" {
		g.trace = &flightInputTrace{start: time.Now()}
	}
	if directory == "" {
		return g
	} // Explicit controller-free harness, never cwd guessing.
	p := controllers.DocumentPath(directory)
	d, e := controllers.Load(p)
	if e != nil {
		g.notice(fmt.Sprintf("%s; keyboard/mouse remain available", e))
		return g
	}
	if d.Config.Selected == "" {
		return g
	}
	b, e := sdlraw.Open()
	if e != nil {
		g.notice(fmt.Sprintf("%s: %v; keyboard/mouse remain available", p, e))
		return g
	}
	g.live = controllers.NewLive(d.Config, b)
	diagnostics.Info("controller", "Read-only selected controller profile loaded", "path", p, "profile", d.Config.Selected)
	return g
}
func (g *gameController) Close() {
	if g.live != nil {
		g.live.Close()
	}
}
func (g *gameController) notice(s string) {
	if s != "" && s != g.status {
		diagnostics.Warn("controller", s)
	}
	g.status = s
}

func controllerContext(c *FrontEnd) (context, epoch string) {
	context = "pointer"
	epoch = string(c.Screen)
	switch c.Screen {
	case ScreenScene:
		context = "flight"
		if c.Rendering != nil && c.Rendering.GPS != nil {
			context = "gps"
			epoch += "/gps"
		}
		if c.Rendering != nil && c.Rendering.Flight != nil {
			epoch += fmt.Sprintf("/%t/%t", c.Rendering.Flight.Paused, c.Rendering.Flight.State.Terminal != nil)
		}
	case ScreenReplay:
		context = "replay"
		if c.Viewer != nil && c.Viewer.Practice {
			context = "flight"
			epoch += "/practice"
			if c.Viewer.Playback != nil && c.Viewer.Playback.Preview() != nil && c.Viewer.Playback.Preview().GPS != nil {
				context = "gps"
				epoch += "/gps"
			}
		}
	case ScreenMissionEditor:
		context = "editor"
		if c.Editor != nil {
			epoch += "/" + c.Editor.Stage
		}
	}
	return context, epoch
}

// Merge precedes the common edge latch. OR merging held sources preserves a
// keyboard press when the controller releases, and vice versa. No host cursor
// warping or simulated OS key events are involved.
func (g *gameController) Merge(c *FrontEnd, b Buttons, seconds, mouseDX, mouseDY float64, alt bool, keyHeld func(rune) bool) Buttons {
	context, epoch := controllerContext(c)
	focused := !b.FlightFocusLost
	epoch += fmt.Sprint("/focus", focused)
	s := controllers.LiveSample{Down: map[string]bool{}}
	if g.trace != nil {
		c.inputTrace = g.trace
	}
	if g.live != nil {
		g.live.TraceInput = g.trace != nil
		s = g.live.Poll(epoch, focused)
		g.notice(s.Status)
		if policy := s.Mode + "/" + s.Source + "/" + s.SourceReason; s.Connected && policy != g.collectivePolicy {
			g.collectivePolicy = policy
			diagnostics.Info("controller", "Effective collective policy", "mode", s.Mode, "source", s.Source, "reason", s.SourceReason)
		}
	}
	if g.trace != nil && context == "flight" {
		profile := ""
		if g.live != nil {
			profile = g.live.Config.Selected
		}
		g.trace.add(inputTraceRow{Stage: "poll", FrameMS: seconds * 1000, Focus: focused, Profile: profile, Source: s.Source, Mode: s.Mode, Epoch: g.generation, Connected: s.Connected, Reset: s.Reset, Raw: s.RawCollective, AxisAvailable: s.HasRawCollective, IR: s.RawIRButton, KeyIR: keyHeld != nil && keyHeld('.')})
	}
	if !focused {
		g.xFraction, g.yFraction = 0, 0
		g.previous = map[string]bool{}
		// Release input, not the shared pointer's position/ownership. Returning
		// focus with a stationary mouse must not restore its old OS coordinates.
		return Buttons{FlightFocusLost: true, Close: b.Close, InputTime: b.InputTime, X: c.Office.PointerX, Y: c.Office.PointerY, PointerRelative: g.pointerOwned}
	}
	if g.swapYawRoll && context == "flight" {
		s.Axes[1], s.Axes[3] = s.Axes[3], s.Axes[1]
		s.Bound[1], s.Bound[3] = s.Bound[3], s.Bound[1]
		s.Directions[1], s.Directions[3] = s.Directions[3], s.Directions[1]
	}
	for i := range s.Axes {
		if s.RoleInvert[i] {
			s.Axes[i] = -s.Axes[i]
		}
		if s.PairInvert[i] {
			s.Directions[i][0], s.Directions[i][1] = s.Directions[i][1], s.Directions[i][0]
		}
	}
	// Absolute/slider lever follows its own post-inversion full travel.
	if s.RoleInvert[2] {
		s.Lever = 65535 - s.Lever
	}
	if g.previous == nil {
		g.previous = map[string]bool{}
	}
	// New input interpretation selects the source independent cyclic/rudder
	// reader (SETUP bit02) at logical ticks. Old film operations never do so.
	if s.Reset || g.generation == 0 {
		g.generation++
	}
	b.Live = &flightcontrol.LiveInput{Axes: s.Axes, Bound: s.Bound, Digital: s.Directions, Lever: s.Lever, Absolute: s.Absolute, Mode: s.Mode, Slider: s.Slider, Epoch: g.generation}
	if context == "flight" {
		for i := 0; i < 2; i++ {
			if !b.SceneDirections[i] {
				g.altArrowGate[i] = false
			}
			if alt && b.SceneDirections[i] {
				g.altArrowGate[i] = true
				b.Live.Digital[3][i] = true
			}
			if g.altArrowGate[i] {
				b.SceneDirections[i] = false
				if i == 0 {
					b.Left = false
				} else {
					b.Right = false
				}
			}
		}
		for i, v := range b.SceneDirections {
			axis, side := 1, i
			if i >= 2 {
				axis, side = 0, i-2
			}
			b.Live.Digital[axis][side] = b.Live.Digital[axis][side] || v
		}
		// Buttons/key collective retain the existing held/repeat command owner.
		if s.Directions[2][0] && !s.Directions[2][1] {
			b.FlightKeys[1] = true
		}
		if s.Directions[2][1] && !s.Directions[2][0] {
			b.FlightKeys[0] = true
		}
		b.Live.Digital[2][0] = b.Live.Digital[2][0] || b.FlightKeys[1]
		b.Live.Digital[2][1] = b.Live.Digital[2][1] || b.FlightKeys[0]
		if b.Live.Digital[2][0] && b.Live.Digital[2][1] {
			b.FlightKeys[0], b.FlightKeys[1] = false, false
		}
	}
	if context != "flight" {
		g.altArrowGate = [2]bool{}
	}
	for _, cmd := range controllerCommands {
		down := s.Down[cmd.ID]
		allowed := cmd.Context == context || cmd.Context == "global" || cmd.Context == "pointer" && context != "flight"
		if cmd.ID == "menu.selector1" || cmd.ID == "menu.selector2" {
			allowed = true
		}
		if cmd.Context == "global" && cmd.Token != "quit" && context != "flight" && context != "replay" {
			allowed = false
		}
		if !allowed {
			g.previous[cmd.ID] = down
			continue
		}
		edge := down && !g.previous[cmd.ID]
		g.previous[cmd.ID] = down
		if !down {
			continue
		}
		if edge {
			b.AnyKeyPressed = true
		}
		b.AnyKeyDown = true
		// GPS cursor bindings join the Basic digital vector. An endpoint also
		// bound through Basic must not move both the pointer and keyboard pan.
		if context == "gps" && cmd.Context == "gps" {
			axis, side := -1, 0
			switch cmd.Token {
			case "left":
				axis, side = 1, 0
			case "right":
				axis, side = 1, 1
			case "up":
				axis, side = 0, 0
			case "down":
				axis, side = 0, 1
			}
			if axis >= 0 {
				s.Directions[axis][side] = true
				continue
			}
		}
		applyControllerCommand(&b, context, cmd.Token, edge, keyHeld)
	}
	if context != "flight" {
		axis := func(i int) float64 {
			if context == "gps" {
				if i == 1 && (b.SceneDirections[0] || b.SceneDirections[1]) || i == 0 && (b.SceneDirections[2] || b.SceneDirections[3]) {
					return 0
				}
			}
			n, p := s.Directions[i][0], s.Directions[i][1]
			if n && p {
				return 0
			}
			if n {
				return -1
			}
			if p {
				return 1
			}
			return float64(s.PointerAxes[i]) / 127
		}
		xv, yv := axis(1), axis(0)
		if xv != 0 || yv != 0 {
			g.pointerOwned = true
		}
		if g.pointerOwned {
			// Fine quadratic motion, 120 logical pixels/s at full travel. Cursor
			// time is uncompressed wall time, independent of presentation FPS.
			dt := math.Max(0, math.Min(seconds, .25))
			g.xFraction += xv * math.Abs(xv) * 120 * dt
			g.yFraction += yv * math.Abs(yv) * 120 * dt
			dx, dy := int(g.xFraction), int(g.yFraction)
			g.xFraction -= float64(dx)
			g.yFraction -= float64(dy)
			b.X, b.Y = c.Office.PointerX, c.Office.PointerY
			if c.Screen == ScreenMissionEditor && c.Editor != nil {
				b.X, b.Y = c.Editor.CursorX, c.Editor.CursorY
			}
			if c.Screen == ScreenReplay && c.Viewer != nil && !c.Viewer.Practice {
				b.X, b.Y = c.Viewer.PointerX, c.Viewer.PointerY
			}
			var preview *RenderingPreview
			if c.Screen == ScreenScene {
				preview = c.Rendering
			} else if c.Screen == ScreenReplay && c.Viewer != nil && c.Viewer.Playback != nil {
				preview = c.Viewer.Playback.Preview()
			}
			if preview != nil && preview.GPS != nil {
				b.X, b.Y = preview.GPS.CursorX, preview.GPS.CursorY
			}
			g.xFraction += mouseDX
			g.yFraction += mouseDY
			dx += int(g.xFraction)
			dy += int(g.yFraction)
			g.xFraction -= float64(int(g.xFraction))
			g.yFraction -= float64(int(g.yFraction))
			height := 191
			if context == "editor" {
				height = 200
			}
			b.X = max(0, min(319, b.X+dx))
			b.Y = max(0, min(height-1, b.Y+dy))
			b.Inside, b.PointerRelative, b.PointerDX, b.PointerDY = true, true, dx, dy
		}
	}
	if g.trace != nil && context == "flight" && b.Live != nil {
		g.trace.add(inputTraceRow{Stage: "merge", Epoch: g.generation, Lever: b.Live.Lever, Axis: b.Live.Axes[2], IR: s.Down["weapon.ir_jammer"], Focus: focused})
	}
	return b
}

func applyControllerCommand(b *Buttons, context, token string, edge bool, keyHeld func(rune) bool) {
	switch token {
	case "selector1":
		if context == "flight" {
			b.WeaponTrigger = true
		} else {
			b.LeftMouse = true
		}
	case "selector2":
		if context == "flight" {
			b.ControllerTarget = true
		} else {
			b.RightMouse = true
		}
	case "quit":
		b.AltQ = true
	case "faster":
		b.CompressionIncrease = true
	case "normal":
		b.CompressionReset = true
	case "pause":
		b.FlightPause = true
	case "end":
		b.AltE = true
	case "arrange":
		b.AltS = true
	case "speech":
		b.AltC = true
	case "weapon":
		b.OrdnanceNext = true
	case "last-message":
		b.LastMessage = true
	case "jettison":
		b.Jettison = true
	case "gps":
		b.GPSOpen = true
	case "next":
		b.WaypointKeys[0] = true
	case "previous":
		b.WaypointKeys[1] = true
	case "salvo1":
		b.RocketSalvoKeys[0] = true
	case "salvo2":
		b.RocketSalvoKeys[1] = true
	case "salvo4":
		b.RocketSalvoKeys[2] = true
	case "chaff":
		b.DefenseKeys[0] = true
	case "flare":
		b.DefenseKeys[1] = true
	case "radar":
		b.DefenseKeys[2] = true
	case "ir":
		b.DefenseKeys[3] = true
	case "left":
		b.Left = true
		if context == "gps" {
			b.SceneDirections[0] = true
		}
	case "right":
		b.Right = true
		if context == "gps" {
			b.SceneDirections[1] = true
		}
	case "up":
		b.Up = true
		if context == "gps" {
			b.SceneDirections[2] = true
		}
	case "down":
		b.Down = true
		if context == "gps" {
			b.SceneDirections[3] = true
		}
	case "enter":
		b.Enter = true
	case "backspace":
		b.Backspace = true
	case "escape":
		b.Escape = true
	case "yes":
		b.ConfirmQuit = true
	case "tab":
		b.MenuTab = true
	case "backtab":
		b.MenuBackTab = true
	case "home":
		b.MenuHome = true
	case "last":
		b.MenuEnd = true
	case "resume":
		b.ReplayResume = true
	case "demo":
		b.ReplayDemo = true
	default:
		if strings.HasPrefix(token, "order:") && context == "flight" {
			if edge {
				b.WingmanOrder = strings.TrimPrefix(token, "order:")
			}
			return
		}
		if strings.HasPrefix(token, "shiftview") {
			b.ViewCommandShift[int(token[len(token)-1]-'1')] = true
			token = strings.TrimPrefix(token, "shift")
		}
		if strings.HasPrefix(token, "view") {
			b.FunctionKeys[int(token[len(token)-1]-'1')] = true
			return
		}
		if context == "flight" {
			for i, key := range []string{"=", "-", "9", "0", "c", "v", "z", "x", "3", "8", "5"} {
				if token == key {
					b.FlightKeys[i] = true
					return
				}
			}
		}
		if context == "gps" {
			if edge {
				b.GPSKey = token
			}
			return
		}
		if edge && len(token) == 1 {
			r := rune(token[0])
			if keyHeld != nil && keyHeld(r) {
				return
			}
			for _, old := range b.Text {
				if old == r || old == r-32 {
					return
				}
			}
			b.Text = append(b.Text, r)
		}
	}
}
