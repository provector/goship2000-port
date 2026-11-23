//go:build !headless

package menus

import (
	"encoding/json"
	"fmt"
	rl "github.com/gen2brain/raylib-go/raylib"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/diagnostics"
	"gunship2000-assets/game/nativeaudio"
	sound "gunship2000-assets/game/sounds"
	"gunship2000-assets/internal/branding"
	"gunship2000-assets/internal/presentationstats"
	"gunship2000-assets/internal/rasterlayout"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

func RunOfficeWindow(o *Office, options WindowOptions) error {
	if options.FrontEnd != nil {
		defer options.FrontEnd.AbandonFilmRecording()
		defer options.FrontEnd.finishPendingRejectionWrite()
	}
	options.Runtime = options.Runtime.Effective()
	if err := options.Runtime.Validate(); err != nil {
		return fmt.Errorf("runtime configuration: %w", err)
	}
	if options.FrontEnd != nil {
		options.FrontEnd.Runtime = options.Runtime
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if branding.DeveloperTools && options.CaptureDirectory != "" {
		directory, err := assetdata.GeneratedPath(options.CaptureDirectory, o.Assets.Root)
		if err != nil {
			return err
		}
		if err = os.MkdirAll(filepath.Dir(directory), 0755); err != nil {
			return err
		}
		if err = os.Mkdir(directory, 0755); err != nil {
			return err
		}
		options.CaptureDirectory = directory
	}
	rl.SetTraceLogLevel(rl.LogWarning)
	if branding.DeveloperTools && options.Development && options.SceneSmoke {
		rl.SetTraceLogLevel(rl.LogInfo)
	}
	rl.SetConfigFlags(rl.FlagWindowResizable)
	rl.InitWindow(int32(options.Width), int32(options.Height), "goship2000")
	if !rl.IsWindowReady() {
		return fmt.Errorf("raylib could not create a window / graphics context")
	}
	defer rl.CloseWindow()
	if options.FrontEnd != nil && options.FrontEnd.Startup != nil {
		defer options.FrontEnd.releaseStartup()
	}
	rl.SetWindowTitle(MainWindowTitle)
	branding.SetGameIcon()
	if options.Fullscreen {
		rl.SetWindowSize(options.Width, options.Height)
		rl.ToggleFullscreen()
	}
	defer rl.EnableCursor()
	if options.FrontEnd != nil {
		rl.SetExitKey(rl.KeyNull)
	}
	rl.SetWindowMinSize(320, 200)
	rl.SetTargetFPS(int32(options.Runtime.PresentationFPS))
	meter := presentationstats.Start(options.DrawFPSCPU)
	gamepad := openGameController(options.AppDirectory)
	gamepad.swapYawRoll = options.Runtime.SwapYawRollAxis
	defer gamepad.Close()
	capture := mouseCapture{}
	latch := windowInput{}
	if options.BrandSplash && options.FrontEnd != nil && options.FrontEnd.Screen == ScreenStartup {
		closed, err := presentBrand(options, gamepad, &latch, &capture, meter)
		if closed || err != nil {
			return err
		}
	}
	var audio *sound.AudioPlayer
	if options.FrontEnd != nil && o.Assets.Audio != nil {
		prefs := sound.DefaultAudioPreferences()
		if options.AudioPreferences != nil {
			prefs = *options.AudioPreferences
		}
		var audioErr error
		audio, audioErr = sound.NewAudioPlayer(o.Assets.Audio, prefs)
		if audioErr != nil {
			return audioErr
		}
		if options.SoundEffectsEnabled != nil {
			audio.SetSoundEffectsEnabled(*options.SoundEffectsEnabled)
		}
		if options.MusicEnabled != nil {
			audio.SetMusicEnabled(*options.MusicEnabled)
		}
		syncFrontEndAudio(audio, options.FrontEnd)
		if options.FrontEnd.Startup != nil {
			options.FrontEnd.Startup.AudioBusy = func(stage int) bool { return stage == 1 && audio.MusicBusy() }
		}
		closeAudio := sound.StartAudioOutput(audio, nativeaudio.OpenAudioOutput, os.Stderr)
		defer closeAudio()
		if diagnostics.Enabled(slog.LevelDebug) {
			diagnostics.Debug("audio", "Audio output opened", "state", audio.Status())
		}
		if branding.DeveloperTools && options.Development && !options.SceneSmoke && options.FrontEnd.Screen == ScreenScene && options.FrontEnd.Rendering != nil && options.FrontEnd.Rendering.Flight != nil {
			// Direct development entry bypasses the MFD loader. Supply its explicit
			// late0bh(1) boundary once; normal preflight uses the real readiness latch.
			audio.PlayInitializationStart()
		}
	}
	rgba, err := assetdata.FrameRGBA(o.Frame(options.DrawPointer), o.Assets.Palette)
	if options.FrontEnd != nil {
		rgba, err = options.FrontEnd.RenderRGBA(options.DrawPointer)
	}
	if err != nil {
		return err
	}
	// NewImage references Go-owned bytes. Loading copies them to the GPU; calling
	// UnloadImage here would incorrectly free Go memory through the C allocator.
	texture := rl.LoadTextureFromImage(rl.NewImage(rgba.Pix, int32(rgba.Rect.Dx()), int32(rgba.Rect.Dy()), 1, rl.UncompressedR8g8b8a8))
	if texture.ID == 0 {
		return fmt.Errorf("raylib could not upload office texture")
	}
	defer func() { rl.UnloadTexture(texture) }()
	rl.SetTextureFilter(texture, rl.FilterPoint)
	clock := tickAccumulator{Rate: options.TickRate}
	paused := false
	hidden := false
	type smokeCheckpoint struct {
		Frame          int      `json:"frame"`
		Window         [2]int   `json:"window"`
		Viewport       Viewport `json:"viewport"`
		LogicalPointer [2]int   `json:"logical_pointer"`
		HoverKey       int      `json:"hover_key"`
		Ticks          uint64   `json:"ticks"`
		CursorHidden   bool     `json:"host_cursor_hidden"`
		Focused        bool     `json:"focused"`
		SmokePointer   bool     `json:"programmatic_smoke_pointer"`
		Screen         Screen   `json:"screen"`
		StartupStage   int      `json:"startup_stage"`
		StartupFrame   int      `json:"startup_frame"`
	}
	var checkpoints []smokeCheckpoint
	var flightTriggerWitnesses []map[string]any
	lastFlightTrigger := false
	var pointerWitnesses []map[string]any

	var utilityKeys utilityKeyChords
	var inputClock flightWindowClock
	heldKeys := map[int32]bool{}
	responseKeys := map[int32]bool{}
	menuTest := menuSmokeSequence{}
	briefingTest := briefingSmokeSequence{}
	preflightTest := preflightSmokeSequence{}
	sceneTest := renderingSmokeSequence{}
	var sceneTimings []sceneFrameTiming
	var audioBoundaries []map[string]any
	lastAudioBoundary := ""
	capturedScreens := map[string]bool{}
	probeKey := ""
	probeSequence := 0
	lastScreenKey, stableScreenFrames := "", 0
	shortcuts := windowShortcuts{}
	graphics := graphicsWindow{windowed: displayWindow{options.Width, options.Height, false}}
	keyMakes := make([]int32, 0, 16)
	logNotice := ""
	noticeInk := rl.White
	logNoticeUntil := time.Time{}
	for frame := 0; ; frame++ {
		elapsed := float64(rl.GetFrameTime())
		for key := range graphics.consumed {
			if !rl.IsKeyDown(key) {
				delete(graphics.consumed, key)
			}
		}
		keyMakes = keyMakes[:0]
		enterMake, altMake := false, false
		for key := rl.GetKeyPressed(); key != 0; key = rl.GetKeyPressed() {
			if graphics.consumed[key] {
				continue
			}
			keyMakes = append(keyMakes, key)
			enterMake = enterMake || key == rl.KeyEnter || key == rl.KeyKpEnter
			altMake = altMake || key == rl.KeyLeftAlt || key == rl.KeyRightAlt
		}
		altHeld := rl.IsKeyDown(rl.KeyLeftAlt) || rl.IsKeyDown(rl.KeyRightAlt)
		toggleFullscreen, suppressEnter := shortcuts.Read(rl.IsWindowFocused(), altHeld || enterMake && altMake, graphics.keyDown(rl.KeyEnter) || graphics.keyDown(rl.KeyKpEnter), enterMake)
		for toggle := 0; graphics.menu == nil && !graphics.consumed[rl.KeyEnter] && !graphics.consumed[rl.KeyKpEnter] && toggle < toggleFullscreen; toggle++ {
			rl.ToggleFullscreen()
			if diagnostics.Enabled(slog.LevelInfo) {
				diagnostics.Info("window", "Alt+Enter toggled fullscreen", "fullscreen", rl.IsWindowFullscreen(), "width", rl.GetScreenWidth(), "height", rl.GetScreenHeight(), "preference_unchanged", true, "activation", "chord released")
			}
		}
		closeRequested := rl.WindowShouldClose()
		blocked, notice := graphics.input(&options, keyMakes, altHeld || altMake, closeRequested, &meter)
		inputTime, elapsed := inputClock.frame(elapsed, blocked)
		if blocked {
			for rl.GetCharPressed() != 0 {
			}
		}
		if notice != "" {
			noticeInk = rl.White
			logNotice = notice
			logNoticeUntil = time.Now().Add(6 * time.Second)
		}
		if closeRequested {
			blocked = false
		}
		if options.FrontEnd == nil && closeRequested {
			break
		}
		if options.FrontEnd != nil && options.FrontEnd.ExitRequested {
			break
		}
		if branding.DeveloperTools && options.Development && options.SmokeFrames > 0 {
			if frame == 30 {
				rl.SetWindowSize(1280, 720)
			}
			if frame == 60 {
				rl.SetWindowSize(800, 800)
			}
			if frame >= options.SmokeFrames {
				break
			}
		}
		viewport := officeViewport(int(rl.GetScreenWidth()), int(rl.GetScreenHeight()), options.SquarePixels)
		mouse := rl.GetMousePosition()
		motion := rl.GetMouseDelta() // Exactly one raw backend sample per input update.
		x, y, inside := viewport.LogicalPoint(float64(mouse.X), float64(mouse.Y))
		logicalHeight := 191
		if options.FrontEnd != nil && options.FrontEnd.Screen == ScreenMissionEditor {
			logicalHeight = 200
		}
		active := inside && y < logicalHeight && rl.IsWindowFocused()
		client := mouse.X >= 0 && mouse.Y >= 0 && mouse.X < float32(rl.GetScreenWidth()) && mouse.Y < float32(rl.GetScreenHeight())
		// Disabled-cursor coordinates are virtual and may be outside the client;
		// client membership is needed only for a released acquisition click.
		captureEpoch := "office"
		if options.FrontEnd != nil {
			captureEpoch = string(options.FrontEnd.Screen)
		}
		ctrlHeld := rl.IsKeyDown(rl.KeyLeftControl) || rl.IsKeyDown(rl.KeyRightControl)
		pointer := capture.Read(mouseCaptureSample{
			LogicalHeight: logicalHeight, Enabled: options.Runtime.Effective().MouseCapture == "click", Focused: rl.IsWindowFocused(), Minimized: rl.IsWindowMinimized(), Client: client,
			Left: rl.IsMouseButtonDown(rl.MouseButtonLeft), Right: rl.IsMouseButtonDown(rl.MouseButtonRight), CtrlF10: ctrlHeld && rl.IsKeyDown(rl.KeyF10), F10: rl.IsKeyDown(rl.KeyF10),
			AbsoluteX: x, AbsoluteY: y, AbsoluteInside: active, DeltaX: float64(motion.X), DeltaY: float64(motion.Y), Viewport: viewport, Epoch: captureEpoch,
		})
		if pointer.Acquire {
			rl.DisableCursor()
		}
		if pointer.Release {
			rl.EnableCursor()
			hidden = false
		}
		x, y, active = pointer.X, pointer.Y, pointer.Inside
		actualPointerActive := active && (pointer.Captured || options.Runtime.Effective().MouseCapture == "disabled")
		if branding.DeveloperTools && options.Development && options.SmokeFrames > 0 && (options.MenuSmoke || options.StartupSmoke || options.BriefingSmoke || options.PreflightSmoke || options.SceneSmoke) {
			// Semantic smoke scenarios own a supplied pointer. A bounded development
			// window without a scenario keeps real OS mouse input for dispatch tests.
			// The smoke path exercises our own logical pointer/viewport, not OS input and
			// never original-game execution. Normal runs always use the real mouse.
			targets := [][2]int{{184, 70}, {140, 88}, {270, 45}}
			p := targets[(frame/30)%len(targets)]
			mx := viewport.X + (float64(p[0])+.5)*viewport.Width/320
			my := viewport.Y + (float64(p[1])+.5)*viewport.Height/200
			x, y, inside = viewport.LogicalPoint(mx, my)
			active = inside && y < logicalHeight
			actualPointerActive = active // Explicit synthetic development pointer only.
		}
		if options.FrontEnd != nil && !blocked {
			c := options.FrontEnd
			anyKey := false
			debriefKey := false
			triggerPressed := false
			for _, key := range keyMakes {
				if !windowResponseMake(key, suppressEnter) {
					continue
				}
				debriefKey = debriefKey || debriefResponseKey(key)
				heldKeys[key] = true
				if key == rl.KeyEnter || key == rl.KeyKpEnter {
					triggerPressed = true // GLFW make queue survives a release in the same host poll.
				}
				if !pointer.SuppressF10 || (key != rl.KeyF10 && key != rl.KeyLeftControl && key != rl.KeyRightControl) {
					anyKey = true
				}
			}
			for key := range heldKeys {
				if !rl.IsKeyDown(key) || !windowResponseMake(key, suppressEnter) {
					delete(heldKeys, key)
				}
			}
			for key := range responseKeys {
				if !rl.IsKeyDown(key) {
					delete(responseKeys, key)
				}
			}
			text := []rune{}
			for key := rl.GetCharPressed(); key != 0; key = rl.GetCharPressed() {
				if graphics.consumed[rl.KeyO] && (key == 'o' || key == 'O') {
					continue
				}
				if !suppressEnter || key != '\r' && key != '\n' {
					text = append(text, key)
				}
			}
			var functionKeys [9]bool
			alt := rl.IsKeyDown(rl.KeyLeftAlt) || rl.IsKeyDown(rl.KeyRightAlt)
			ctrl := ctrlHeld
			var flightKeys [11]bool
			flightCodes := [11]int32{rl.KeyEqual, rl.KeyMinus, rl.KeyNine, rl.KeyZero, rl.KeyC, rl.KeyV, rl.KeyZ, rl.KeyX, rl.KeyThree, rl.KeyEight, rl.KeyFive}
			if !alt && !ctrl && rl.IsWindowFocused() {
				for i, key := range flightCodes {
					flightKeys[i] = rl.IsKeyDown(key)
				}
			}
			if !alt && !ctrl {
				for i := range functionKeys {
					functionKeys[i] = rl.IsKeyDown(rl.KeyF1 + int32(i))
				}
			}
			gpsKey := ""
			wingmanOrder := ""
			if ctrl && !alt && rl.IsWindowFocused() {
				wingmanOrder = sampleWingmanOrderKey(rl.IsKeyPressed)
			}
			shiftHeld := rl.IsKeyDown(rl.KeyLeftShift) || rl.IsKeyDown(rl.KeyRightShift)
			if !alt && !ctrl && !shiftHeld {
				gpsKey = sampleGPSCommandKey(rl.IsKeyPressed)
			}
			flightModifiers := !alt && !ctrl && rl.IsWindowFocused()
			inputContext, _ := controllerContext(c)
			lastMessage, jettison := utilityKeys.sample(rl.IsWindowFocused(), inputContext == "flight", alt, ctrl, shiftHeld, rl.IsKeyDown(rl.KeyM), rl.IsKeyDown(rl.KeySpace))
			if options.FrontEnd.Rendering != nil && options.FrontEnd.Rendering.Flight != nil {
				options.FrontEnd.Rendering.Flight.RequireTerminalAck = true
			}
			if c.Screen == ScreenReplay && c.Viewer != nil && c.Viewer.Playback != nil {
				if p := c.Viewer.Playback.Preview(); p != nil && p.Flight != nil {
					p.Flight.RequireTerminalAck = true
				}
			}
			b := Buttons{LastMessage: lastMessage, Jettison: jettison, ReplayMap: alt && rl.IsKeyDown(rl.KeyM), ReplayResume: alt && rl.IsKeyDown(rl.KeyX), ReplayDemo: alt && rl.IsKeyDown(rl.KeyY), CompressionIncrease: flightModifiers && !shiftHeld && rl.IsKeyDown(rl.KeyRightBracket), CompressionReset: flightModifiers && !shiftHeld && rl.IsKeyDown(rl.KeyLeftBracket), WeaponTriggerPressed: !suppressEnter && flightModifiers && !shiftHeld && triggerPressed, WeaponTrigger: flightModifiers && !shiftHeld && ((!pointer.SuppressButtons && rl.IsMouseButtonDown(rl.MouseButtonLeft)) || !suppressEnter && (graphics.keyDown(rl.KeyEnter) || graphics.keyDown(rl.KeyKpEnter))), RocketSalvoKeys: [3]bool{flightModifiers && !shiftHeld && rl.IsKeyDown(rl.KeyOne), flightModifiers && !shiftHeld && rl.IsKeyDown(rl.KeyTwo), flightModifiers && !shiftHeld && rl.IsKeyDown(rl.KeyFour)}, OrdnanceNext: flightModifiers && !shiftHeld && !utilityKeys.spaceConsumed && rl.IsKeyDown(rl.KeySpace), GPSOpen: !pointer.SuppressF10 && flightModifiers && !shiftHeld && rl.IsKeyDown(rl.KeyF10), GPSKey: gpsKey, WingmanOrder: wingmanOrder, DefenseKeys: [4]bool{flightModifiers && !shiftHeld && !utilityKeys.mConsumed && rl.IsKeyDown(rl.KeyM), flightModifiers && !shiftHeld && rl.IsKeyDown(rl.KeySlash), flightModifiers && !shiftHeld && rl.IsKeyDown(rl.KeyN), flightModifiers && !shiftHeld && rl.IsKeyDown(rl.KeyPeriod)}, WaypointKeys: [2]bool{flightModifiers && !shiftHeld && rl.IsKeyDown(rl.KeySix), flightModifiers && !shiftHeld && rl.IsKeyDown(rl.KeySeven)}, InputTime: inputTime, PointerRelative: pointer.Relative, PointerDX: pointer.DX, PointerDY: pointer.DY, PointerEpoch: pointer.Reset, AltC: alt && rl.IsKeyDown(rl.KeyC), AltT: alt && rl.IsKeyDown(rl.KeyT), FlightKeys: flightKeys, FlightPause: pauseChord(rl.IsKeyDown(rl.KeyP), alt, ctrl, shiftHeld), FlightFocusLost: !rl.IsWindowFocused() || rl.IsWindowMinimized(), MenuHome: !alt && !ctrl && rl.IsKeyDown(rl.KeyHome), MenuEnd: !alt && !ctrl && rl.IsKeyDown(rl.KeyEnd), MenuTab: !alt && !ctrl && rl.IsKeyDown(rl.KeyTab) && !(rl.IsKeyDown(rl.KeyLeftShift) || rl.IsKeyDown(rl.KeyRightShift)), MenuBackTab: !alt && !ctrl && rl.IsKeyDown(rl.KeyTab) && (rl.IsKeyDown(rl.KeyLeftShift) || rl.IsKeyDown(rl.KeyRightShift)), PageUp: !alt && !ctrl && rl.IsKeyDown(rl.KeyPageUp), PageDown: !alt && !ctrl && rl.IsKeyDown(rl.KeyPageDown), SceneDirections: [4]bool{graphics.keyDown(rl.KeyLeft) || rl.IsKeyDown(rl.KeyKp4), graphics.keyDown(rl.KeyRight) || rl.IsKeyDown(rl.KeyKp6), graphics.keyDown(rl.KeyUp) || rl.IsKeyDown(rl.KeyKp8), graphics.keyDown(rl.KeyDown) || rl.IsKeyDown(rl.KeyKp2)}, FunctionKeys: functionKeys, ViewShift: rl.IsKeyDown(rl.KeyLeftShift) || rl.IsKeyDown(rl.KeyRightShift), AltS: alt && rl.IsKeyDown(rl.KeyS), X: x, Y: y, Inside: active, LeftMouse: !pointer.SuppressButtons && rl.IsMouseButtonDown(rl.MouseButtonLeft), RightMouse: !pointer.SuppressButtons && rl.IsMouseButtonDown(rl.MouseButtonRight), Space: !utilityKeys.spaceConsumed && rl.IsKeyDown(rl.KeySpace), Enter: !suppressEnter && (graphics.keyDown(rl.KeyEnter) || graphics.keyDown(rl.KeyKpEnter)), Escape: graphics.keyDown(rl.KeyEscape), AltQ: (rl.IsKeyDown(rl.KeyLeftAlt) || rl.IsKeyDown(rl.KeyRightAlt)) && rl.IsKeyDown(rl.KeyQ), AltE: (rl.IsKeyDown(rl.KeyLeftAlt) || rl.IsKeyDown(rl.KeyRightAlt)) && rl.IsKeyDown(rl.KeyE), Left: graphics.keyDown(rl.KeyLeft), Right: graphics.keyDown(rl.KeyRight), Up: graphics.keyDown(rl.KeyUp), Down: graphics.keyDown(rl.KeyDown), Backspace: rl.IsKeyDown(rl.KeyBackspace), Delete: rl.IsKeyDown(rl.KeyDelete), Close: closeRequested, ConfirmQuit: rl.IsKeyDown(rl.KeyY), Text: text, AnyKeyPressed: anyKey, AnyKeyDown: len(responseKeys) > 0}
			mdx, mdy := float64(pointer.DX), float64(pointer.DY)
			if !pointer.Relative {
				mdx = float64(motion.X) * 320 / viewport.Width
				mdy = float64(motion.Y) * 200 / viewport.Height
			}
			b = gamepad.Merge(c, b, elapsed, mdx, mdy, alt, func(r rune) bool {
				if r >= 'a' && r <= 'z' {
					r -= 32
				}
				return rl.IsKeyDown(int32(r))
			})
			previousButtons := latch.previous
			a := latch.ReadViewport(b, viewport)
			if c.Screen == ScreenSettings && c.gps.required && rl.IsWindowFocused() {
				// Physical makes include the numeric keypad regardless of Num Lock.
				// Do not also consume the character queue for these same digits.
				a.Text = nil
				for _, r := range text {
					if r == '#' && len(keyMakes) > 0 {
						a.Text = append(a.Text, r)
						break
					}
				}
				for _, key := range keyMakes {
					switch {
					case alt && !ctrl && key >= rl.KeyF1 && key <= rl.KeyF5 && rl.IsKeyPressed(key):
						a.gpsSteps = append(a.gpsSteps, int(key-rl.KeyF1)+1)
					case !alt && !ctrl && key == rl.KeyE:
						a.Text = append(a.Text, 'E')
					case !alt && !ctrl && !shiftHeld && key >= rl.KeyZero && key <= rl.KeyNine:
						a.Text = append(a.Text, rune('0'+key-rl.KeyZero))
					case !alt && !ctrl && key >= rl.KeyKp0 && key <= rl.KeyKp9:
						a.Text = append(a.Text, rune('0'+key-rl.KeyKp0))
					}
				}
			}
			if gamepad.pointerOwned && !b.FlightFocusLost {
				actualPointerActive = true
			}

			a = c.editorArrowMakes(a, a.SceneDirections,
				[4]bool{rl.IsKeyPressed(rl.KeyLeft) || rl.IsKeyPressed(rl.KeyKp4), rl.IsKeyPressed(rl.KeyRight) || rl.IsKeyPressed(rl.KeyKp6), rl.IsKeyPressed(rl.KeyUp) || rl.IsKeyPressed(rl.KeyKp8), rl.IsKeyPressed(rl.KeyDown) || rl.IsKeyPressed(rl.KeyKp2)},
				[4]bool{rl.IsKeyPressedRepeat(rl.KeyLeft) || rl.IsKeyPressedRepeat(rl.KeyKp4), rl.IsKeyPressedRepeat(rl.KeyRight) || rl.IsKeyPressedRepeat(rl.KeyKp6), rl.IsKeyPressedRepeat(rl.KeyUp) || rl.IsKeyPressedRepeat(rl.KeyKp8), rl.IsKeyPressedRepeat(rl.KeyDown) || rl.IsKeyPressedRepeat(rl.KeyKp2)})

			if c.Screen == ScreenDebrief {
				// A bare Alt/Shift must not acknowledge a result while the user
				// is preparing a shortcut such as Alt+Q. Other screens retain
				// their accepted key handling; this is new debrief ownership.
				a.AnyKey = debriefKey
			}
			if c.Screen == ScreenScene && c.Rendering != nil || c.Screen == ScreenReplay && c.Viewer != nil && c.Viewer.Practice {
				// Expansion keyboard profile: Space selects ordnance; Enter is Selector1.
				// Menus retain their separately established Space/Enter mapping.
				a = normalizeSceneSelectors(a, b.LeftMouse && !previousButtons.LeftMouse, b.RightMouse && !previousButtons.RightMouse, flightModifiers)
			}
			pointerReset := a.PointerReset
			// Collective repeats belong to logical flight input time, never host typematic.
			if c.Screen == ScreenOutfit {
				// Assignment commands are key edges, not text-editor auto-repeat.
				a.Text = nil
				for _, key := range []int32{rl.KeyP, rl.KeyH, rl.KeyD, rl.KeyL} {
					if rl.IsKeyPressed(key) {
						a.Text = append(a.Text, rune(key))
					}
				}
			}
			if branding.DeveloperTools && options.Development && options.MenuSmoke {
				a = menuTest.Next(c)
				if menuTest.Err != nil {
					return menuTest.Err
				}
			}
			if branding.DeveloperTools && options.Development && options.BriefingSmoke {
				a = briefingTest.Next(c)
				if briefingTest.Err != nil {
					return briefingTest.Err
				}
			}
			if branding.DeveloperTools && options.Development && options.PreflightSmoke {
				a = preflightTest.Next(c)
				if preflightTest.Err != nil {
					return preflightTest.Err
				}
			}
			if branding.DeveloperTools && options.Development && options.SceneSmoke {
				a = sceneTest.Next(c)
				if sceneTest.Err != nil {
					return sceneTest.Err
				}
			}
			wasQuit := c.Screen == ScreenQuit
			wasCrew := c.crewPointerPresented()
			if branding.DeveloperTools && options.Development && options.CaptureDirectory != "" && c.Screen == ScreenScene && (a.AnyKey || a.Cancel || a.Quit || a.Close || a.FlightPause) {
				// Input receipt witness only; no second dispatch or simulation update.
				pointerWitnesses = append(pointerWitnesses, map[string]any{"frame": frame, "event": "flight_controller_input", "any_key": a.AnyKey, "cancel": a.Cancel, "quit": a.Quit, "close": a.Close, "focus_lost": a.FlightFocusLost, "focused": rl.IsWindowFocused(), "captured": pointer.Captured})
			}
			a.PointerReset = a.PointerReset || pointerReset
			if err := c.Handle(a); err != nil {
				if c.Message == "" {
					c.reportDiagnostic("menu", "Action incomplete", err)
				}
			}
			if branding.DeveloperTools && options.Development && options.CaptureDirectory != "" && (wasCrew || c.crewPointerPresented()) && (a.PointerPressed || a.Selector1 || a.Enter || a.MenuHome || a.MenuEnd || a.MenuTab || a.MenuBackTab || a.Up || a.Down || a.PointerDX != 0 || a.PointerDY != 0 || a.PointerReset) {
				// Bounded diagnostics of real input; never a second dispatch/producer.
				w := map[string]any{"frame": frame, "event": "crew_controller_input", "selector1": a.Selector1, "selector2": a.Selector2, "enter": a.Enter, "pointer_pressed": a.PointerPressed, "inside": a.Inside, "raw_logical": [2]int{a.X, a.Y}, "relative": a.PointerRelative, "delta": [2]int{a.PointerDX, a.PointerDY}, "reset": a.PointerReset, "captured": pointer.Captured, "buttons_suppressed": pointer.SuppressButtons, "draw_pointer": options.DrawPointer, "controller_pointer": [2]int{c.Office.PointerX, c.Office.PointerY}, "controller_inside": c.Office.PointerInside, "screen": c.Screen}
				if c.Debrief != nil {
					w["page"], w["crew_selection"] = c.Debrief.Page, c.Debrief.CrewSelection
					if c.Debrief.Pending.CrewChoices != nil {
						w["recipients"] = append([]int(nil), c.Debrief.Pending.CrewChoices.Recipients...)
					}
				}
				pointerWitnesses = append(pointerWitnesses, w)
			}
			if branding.DeveloperTools && options.Development && options.CaptureDirectory != "" && (a.WeaponTriggerHeld != lastFlightTrigger || a.WeaponTriggerPressed) && c.Rendering != nil && c.Rendering.Flight != nil {
				r := c.Rendering.Flight
				flightTriggerWitnesses = append(flightTriggerWitnesses, map[string]any{"frame": frame, "time_ns": a.InputTime, "normalized_trigger": a.WeaponTriggerHeld, "normalized_press": a.WeaponTriggerPressed, "logical_trigger": r.commandInput.Trigger, "release_gate": r.requireRelease, "steps": r.Steps, "input_logical_ns": r.commandInput.elapsed, "input_sample_ns": r.commandInput.sampleElapsed, "station": c.Rendering.Initialized.SelectedStation, "screen": c.Screen})
				lastFlightTrigger = a.WeaponTriggerHeld
			}
			if branding.DeveloperTools && options.Development && c.Screen == ScreenScene && c.Rendering.Inspection && !a.Quit && !wasQuit {
				// A modal response (notably N) must not also single-step/navigation.
				if rl.IsKeyPressed(rl.KeyP) {
					c.Rendering.Paused = !c.Rendering.Paused
				}
				if c.Rendering.Paused && (rl.IsKeyPressed(rl.KeyPeriod) || rl.IsKeyPressed(rl.KeyN)) {
					if err := c.StepRendering(1); err != nil {
						return err
					}
				}
				if rl.IsKeyPressed(rl.KeyV) {
					c.Rendering.Cockpit.Arrangement = (c.Rendering.Cockpit.Arrangement + 1) % 4
					c.Rendering.Dirty = true
				}
				if rl.IsKeyPressed(rl.KeyTab) {
					c.Rendering.Cockpit.CameraMode ^= 1
					c.Rendering.Dirty = true
				}
				axis := func(positive, negative int32) float64 {
					var n float64
					if rl.IsKeyDown(positive) {
						n++
					}
					if rl.IsKeyDown(negative) {
						n--
					}
					return n
				}
				c.MoveInspectionCamera(elapsed, axis(rl.KeyW, rl.KeyS), axis(rl.KeyD, rl.KeyA), axis(rl.KeyR, rl.KeyF), axis(rl.KeyRight, rl.KeyLeft), axis(rl.KeyUp, rl.KeyDown))
			}
			if wasQuit && c.Screen != ScreenQuit {
				// Suppress only the response keys still held, not the user's next
				// distinct character if typing resumes immediately after dismissal.
				for key := range heldKeys {
					responseKeys[key] = true
				}
				latch.SuppressTextUntilRelease()
			}
			o = c.Office
		} else if options.FrontEnd == nil {
			o.SetPointer(x, y, active)
		}
		if branding.DeveloperTools && options.Development && options.CaptureDirectory != "" && (pointer.Acquire || pointer.Release || pointer.Reset || pointer.DX != 0 || pointer.DY != 0) {
			witness := map[string]any{"frame": frame, "captured": pointer.Captured, "acquire": pointer.Acquire, "release": pointer.Release, "reset": pointer.Reset, "buttons_suppressed": pointer.SuppressButtons, "f10_suppressed": pointer.SuppressF10, "logical": [2]int{pointer.X, pointer.Y}, "delta": [2]int{pointer.DX, pointer.DY}, "backend_delta": [2]float32{motion.X, motion.Y}, "focused": rl.IsWindowFocused(), "minimized": rl.IsWindowMinimized()}
			if options.FrontEnd != nil {
				witness["screen"], witness["revision"] = options.FrontEnd.Screen, options.FrontEnd.Store.State().Revision
			}
			pointerWitnesses = append(pointerWitnesses, witness)
		}
		shouldHide := actualPointerActive && rl.IsWindowFocused() && options.DrawPointer
		if !pointer.Captured && shouldHide != hidden {
			if shouldHide {
				rl.HideCursor()
			} else {
				rl.ShowCursor()
			}
			hidden = shouldHide
		}
		if pointer.Captured {
			hidden = true
		}
		if branding.DeveloperTools && options.Development && options.FrontEnd == nil && (rl.IsKeyPressed(rl.KeyP) || rl.IsKeyPressed(rl.KeySpace)) {
			paused = !paused
		}
		if (options.Development && options.FrontEnd == nil && rl.IsKeyPressed(rl.KeyC)) || (options.FrontEnd == nil && active && o.HoverKey == 6 && rl.IsMouseButtonPressed(rl.MouseButtonLeft)) {
			o.CycleComputerMode()
		}
		if branding.DeveloperTools && options.Development && options.FrontEnd == nil && paused && (rl.IsKeyPressed(rl.KeyN) || rl.IsKeyPressed(rl.KeyPeriod)) {
			o.Tick()
		}
		for ticks := clock.Advance(elapsed, paused || blocked); ticks > 0; ticks-- {
			if options.FrontEnd != nil {
				if err := options.FrontEnd.Tick(); err != nil {
					return err
				}
			} else {
				o.Tick()
			}
		}
		if options.FrontEnd != nil && !blocked {
			if err := options.FrontEnd.AdvanceStartup(elapsed); err != nil {
				return err
			}
			// Preparation failures are logged once by their reached outcome owner.
			_ = options.FrontEnd.AdvanceSceneInitialization(elapsed)
			if err := options.FrontEnd.AdvanceReplay(elapsed); err != nil {
				options.FrontEnd.recordDiagnostic(slog.LevelWarn, "replay", "Replay update rejected", err)
			}
			if err := options.FrontEnd.AdvanceRendering(elapsed); err != nil {
				return err
			}
		}
		if audio != nil {
			options.FrontEnd.audioFocusPaused = blocked || !rl.IsWindowFocused() || rl.IsWindowMinimized()
			syncFrontEndAudio(audio, options.FrontEnd)
			if (options.SceneSmoke || options.Development) && options.CaptureDirectory != "" {
				c := options.FrontEnd
				key := fmt.Sprintf("%s-%t", c.Screen, c.sceneEndConfirmation)
				if c.SceneEntry != nil {
					key += fmt.Sprint(c.SceneEntry.Stage)
				}
				if options.Development {
					status := audio.Status()
					key += fmt.Sprintf("/%d/%d/%t", status.InitializationCommands, status.EffectCommands, status.EffectsEnabled)
					if status.Rotor != nil {
						key += fmt.Sprintf("/%d", status.Rotor.Commands)
					}
				}
				if key != lastAudioBoundary {
					audioBoundaries = append(audioBoundaries, map[string]any{"frame": frame, "screen": c.Screen, "previous_screen": c.previousScreen, "audio": audio.Status()})
					lastAudioBoundary = key
				}
			}
		}
		var compositionStart time.Time
		var sceneRedrawn bool
		if branding.DeveloperTools && options.Development && options.CaptureDirectory != "" {
			sceneRedrawn = options.FrontEnd != nil && options.FrontEnd.Screen == ScreenScene && options.FrontEnd.Rendering.Dirty
			compositionStart = time.Now()
		}
		if graphics.menu != nil {
			rgba, err = graphics.menu.compose(options.FrontEnd)
		} else if options.FrontEnd != nil {
			showPointer := actualPointerActive || options.FrontEnd.keyboardHotspot == options.FrontEnd.Screen
			rgba, err = options.FrontEnd.renderRGBA(showPointer && options.DrawPointer, rgba)
		} else {
			rgba, err = assetdata.FrameRGBA(o.Frame(actualPointerActive && options.DrawPointer), o.Assets.Palette)
		}
		if err != nil {
			return err
		}
		if options.FrontEnd != nil && options.FrontEnd.Screen == ScreenScene {
			rl.SetWindowTitle(MainWindowTitle)
			if branding.DeveloperTools && options.Development && options.CaptureDirectory != "" {
				sceneTimings = append(sceneTimings, sceneFrameTiming{options.FrontEnd.Rendering.GPS != nil, [2]int{rgba.Rect.Dx(), rgba.Rect.Dy()}, frame, [2]int{int(rl.GetScreenWidth()), int(rl.GetScreenHeight())}, float64(time.Since(compositionStart).Nanoseconds()) / 1e6, float64(rl.GetFrameTime()) * 1000, sceneRedrawn, options.FrontEnd.Rendering.Stats.Primitives, options.FrontEnd.Rendering.Cockpit.CameraMode})
			}
		} else {
			rl.SetWindowTitle(MainWindowTitle)
		}
		if int(texture.Width) != rgba.Rect.Dx() || int(texture.Height) != rgba.Rect.Dy() {
			replacement := rl.LoadTextureFromImage(rl.NewImage(rgba.Pix, int32(rgba.Rect.Dx()), int32(rgba.Rect.Dy()), 1, rl.UncompressedR8g8b8a8))
			if replacement.ID == 0 {
				if g := graphics.menu; g != nil && g.draft.runtime != g.original.runtime {
					g.draft.runtime = g.original.runtime
					applyDisplay(options.FrontEnd, g.original.runtime)
					g.dirty = true
					noticeInk = rl.White
					logNotice = "Could not allocate the new picture. Previous graphics restored."
					logNoticeUntil = time.Now().Add(6 * time.Second)
					continue
				}
				return fmt.Errorf("could not upload changed simulation target")
			}
			rl.UnloadTexture(texture)
			texture = replacement
			rl.SetTextureFilter(texture, rl.FilterPoint)
		} else {
			rl.UpdateTexture(texture, rgba)
		}
		rl.BeginDrawing()
		rl.ClearBackground(rl.Black)
		srcRect := rl.Rectangle{Width: float32(texture.Width), Height: float32(texture.Height)}
		if layout := rasterlayout.FromPixels(int(texture.Width) * int(texture.Height)); layout != nil {
			srcRect.Y = float32(layout.Top)
			srcRect.Height = float32(layout.PictureHeight)
		}
		rl.DrawTexturePro(texture, srcRect, rl.Rectangle{X: float32(viewport.X), Y: float32(viewport.Y), Width: float32(viewport.Width), Height: float32(viewport.Height)}, rl.Vector2{}, 0, rl.White)
		if notice := diagnostics.TakeNotice(); notice != "" {
			noticeInk = rl.NewColor(255, 92, 92, 255)
			logNotice = notice
			logNoticeUntil = time.Now().Add(15 * time.Second)
		}
		if logNotice != "" && time.Now().Before(logNoticeUntil) {
			y := int(rl.GetScreenHeight()) - 76
			rl.DrawRectangle(4, int32(y), int32(rl.GetScreenWidth()-8), 72, rl.NewColor(0, 0, 0, 235))
			drawStartupText(logNotice, 10, y+4, rl.GetScreenWidth()-20, 14, noticeInk)
		}
		drawPerformanceOverlay(meter)
		rl.EndDrawing()
		meter.Presented()
		if branding.DeveloperTools && options.FrontEnd != nil && options.Development && options.CaptureDirectory != "" {
			probe := options.FrontEnd.windowProbe()
			// Cursor progression is payload, not a new UI boundary every physics step.
			key := string(options.FrontEnd.Screen)
			if v := options.FrontEnd.Viewer; v != nil {
				key += fmt.Sprintf("/%t/%t/%t/%t/%t/%t/%d/%s/%s", v.Stopped, v.Practice, v.Fast, v.Next, v.Map, v.Demo, v.Actor, v.Playback.Divergence, v.Playback.Engine.Screen)
			}
			if e := options.FrontEnd.Editor; e != nil {
				key += fmt.Sprintf("/%s/%s/%t/%d/%s", e.Stage, e.Status, e.Held, e.Option, e.ShadowError)
			}
			if f := options.FrontEnd.Files; f != nil {
				key += fmt.Sprintf("/%d/%s/%t", len(f.Entries), f.Status, f.Editing)
			}
			if v := options.FrontEnd.Viewer; v != nil && v.Practice {
				key += fmt.Sprintf("/%d/%d", v.Playback.Preview().Flight.State.Rotor, v.Playback.Preview().Flight.State.Collective)
			}
			if l := options.FrontEnd.Library; l != nil {
				key += fmt.Sprintf("/%d/%d/%t/%s", len(l.Entries), l.Selected, l.Editing, l.Status)
			}
			if key != probeKey {
				probeKey = key
				probeSequence++
				probe["frame"] = frame
				data, _ := json.MarshalIndent(probe, "", "  ")
				if _, err = assetdata.WriteNew(options.CaptureDirectory, fmt.Sprintf("ui-%06d.json", probeSequence), data); err != nil {
					return err
				}
			}
		}
		if options.FrontEnd != nil {
			options.FrontEnd.MarkScenePresented()
		}
		// Everything after presentation in this block is for requested capture checkpoints.
		if !branding.DeveloperTools || !options.Development || options.SmokeFrames <= 0 {
			continue
		}
		screenKey := ""
		if options.MenuSmoke || options.StartupSmoke || options.BriefingSmoke || options.PreflightSmoke || options.SceneSmoke {
			screenKey = fmt.Sprintf("%s-r%d", options.FrontEnd.Screen, options.FrontEnd.Store.State().Revision)
			if options.FrontEnd.Screen == ScreenStartup {
				p := options.FrontEnd.Startup
				divisor := 100
				if p.Stage == 0 {
					divisor = 10
				}
				screenKey = fmt.Sprintf("startup-%d-%d", p.Stage, p.Frame/divisor)
			}
			if options.FrontEnd.Screen == ScreenScene && options.FrontEnd.Rendering.GPS != nil {
				g := options.FrontEnd.Rendering.GPS
				screenKey += fmt.Sprintf("-gps-low%t-edit%t", g.Low, g.Editing)
			}
			if options.FrontEnd.Draft != nil {
				screenKey += fmt.Sprintf("_editor%d", options.FrontEnd.Draft.Stage)
			}
			if options.FrontEnd.Mission != nil {
				screenKey += fmt.Sprintf("-page%d-settings%v-message%s", options.FrontEnd.Mission.Page, options.FrontEnd.Mission.Settings.Values(), options.FrontEnd.Message)
			}
			if options.FrontEnd.Mission != nil && options.FrontEnd.Mission.Ordnance != nil && options.FrontEnd.Screen == ScreenOrdnance {
				screenKey += fmt.Sprint(options.FrontEnd.Mission.Ordnance.stack)
			}
			if options.FrontEnd.Screen == ScreenScene {
				screenKey += fmt.Sprintf("-camera%d-arrangement%d", options.FrontEnd.Rendering.Cockpit.CameraMode, options.FrontEnd.Rendering.Cockpit.Arrangement)
				if branding.DeveloperTools && options.SceneSmoke {
					screenKey += fmt.Sprintf("-flight-step%d", sceneTest.Step)
				}
			}
			if options.FrontEnd.Screen == ScreenSceneInitialization {
				screenKey += fmt.Sprintf("-stage%d", options.FrontEnd.SceneEntry.Stage)
			}
			if options.FrontEnd.Screen == ScreenSettings {
				screenKey += fmt.Sprintf("-help%d-%t", options.FrontEnd.Mission.settingsHover, options.FrontEnd.Mission.SettingsHelp.Detailed)
			}
			if options.FrontEnd.sceneConfirmationPresented() {
				screenKey += fmt.Sprintf("-from%s-end%t", options.FrontEnd.previousScreen, options.FrontEnd.sceneEndConfirmation)
			}
		}
		if screenKey != lastScreenKey {
			lastScreenKey, stableScreenFrames = screenKey, 0
		} else {
			stableScreenFrames++
		}
		// A screen-change checkpoint can read the previous front buffer on some
		// compositors. Wait for repeated presentations before taking a witness.
		captureScenario := options.MenuSmoke || options.StartupSmoke || options.BriefingSmoke || options.PreflightSmoke || options.SceneSmoke
		fadeComplete := options.FrontEnd == nil || options.FrontEnd.fade == nil
		captureMenu := captureScenario && fadeComplete && stableScreenFrames >= 3 && !capturedScreens[screenKey]
		stableCapture := fadeComplete && (!captureScenario || stableScreenFrames >= 3)
		if branding.DeveloperTools && options.Development && options.SmokeFrames > 0 && stableCapture && (captureMenu || frame == 20 || frame == 50 || frame == 80 || frame == options.SmokeFrames-1) {
			if captureMenu {
				capturedScreens[screenKey] = true
			}
			checkpoint := smokeCheckpoint{Frame: frame, Window: [2]int{int(rl.GetScreenWidth()), int(rl.GetScreenHeight())}, Viewport: viewport, LogicalPointer: [2]int{x, y}, HoverKey: o.HoverKey, Ticks: o.TickCount, CursorHidden: hidden, Focused: rl.IsWindowFocused(), SmokePointer: captureScenario, StartupStage: -1, StartupFrame: -1}
			if options.FrontEnd != nil {
				checkpoint.Screen = options.FrontEnd.Screen
				if options.FrontEnd.startupPresented() {
					checkpoint.StartupStage = options.FrontEnd.Startup.Stage
					checkpoint.StartupFrame = options.FrontEnd.Startup.Frame
				}
			}
			checkpoints = append(checkpoints, checkpoint)
			if branding.DeveloperTools && options.CaptureDirectory != "" {
				// The binding's TakeScreenshot prepends raylib's storage directory even
				// to absolute names. Read the framebuffer and use our checked writer.
				capture := rl.LoadImageFromScreen()
				data := rl.ExportImageToMemory(*capture, ".png")
				rl.UnloadImage(capture)
				if len(data) == 0 {
					return fmt.Errorf("framebuffer PNG export failed")
				}
				if _, err = assetdata.WriteNew(options.CaptureDirectory, fmt.Sprintf("window-%03d.png", frame), data); err != nil {
					return err
				}
				if options.FrontEnd != nil {
					if err = writeDevelopmentCheckpoint(options.CaptureDirectory, frame, options.FrontEnd); err != nil {
						return err
					}
				}
			}
		}
	}
	if branding.DeveloperTools && options.Development && options.CaptureDirectory != "" {
		data, err := assetdata.JsonBytes(pointerWitnesses)
		if err != nil {
			return err
		}
		if _, err = assetdata.WriteNew(options.CaptureDirectory, "pointer-capture.json", data); err != nil {
			return err
		}
	}
	if (options.SceneSmoke || options.Development) && options.CaptureDirectory != "" {
		data, err := assetdata.JsonBytes(audioBoundaries)
		if err != nil {
			return err
		}
		if _, err = assetdata.WriteNew(options.CaptureDirectory, "audio-boundaries.json", data); err != nil {
			return err
		}
	}
	if options.SceneSmoke && sceneTest.Step != renderingSmokeSteps {
		return fmt.Errorf("scene smoke incomplete:%d/%d", sceneTest.Step, renderingSmokeSteps)
	}
	if branding.DeveloperTools && options.CaptureDirectory != "" && options.SceneSmoke {
		data, err := assetdata.JsonBytes(sceneTest.Trace)
		if err != nil {
			return err
		}
		if _, err = assetdata.WriteNew(options.CaptureDirectory, "scene-actions.json", data); err != nil {
			return err
		}
	}
	if branding.DeveloperTools && options.CaptureDirectory != "" && len(sceneTimings) > 0 {
		data, err := assetdata.JsonBytes(scenePerformanceReport(sceneTimings))
		if err != nil {
			return err
		}
		if _, err = assetdata.WriteNew(options.CaptureDirectory, "scene-performance.json", data); err != nil {
			return err
		}
	}
	if audio != nil && options.CaptureDirectory != "" {
		data, audioErr := assetdata.JsonBytes(audio.Status())
		if audioErr != nil {
			return audioErr
		}
		if _, audioErr = assetdata.WriteNew(options.CaptureDirectory, "audio-playback.json", data); audioErr != nil {
			return audioErr
		}
	}
	if options.PreflightSmoke && preflightTest.Step != 22 {
		return fmt.Errorf("preflight smoke incomplete:%d/22", preflightTest.Step)
	}
	if branding.DeveloperTools && options.CaptureDirectory != "" && options.PreflightSmoke {
		data, err := assetdata.JsonBytes(struct {
			Trace         []menuSmokeRecord `json:"actions"`
			SaveDirectory string            `json:"scratch_save_directory"`
		}{preflightTest.Trace, options.FrontEnd.Store.Directory})
		if err != nil {
			return err
		}
		if _, err = assetdata.WriteNew(options.CaptureDirectory, "preflight-actions.json", data); err != nil {
			return err
		}
	}
	if options.MenuSmoke && menuTest.Step != 20 {
		return fmt.Errorf("menu smoke incomplete: %d/20 actions", menuTest.Step)
	}
	if options.BriefingSmoke && briefingTest.Step != 16 {
		return fmt.Errorf("briefing smoke incomplete:%d/16", briefingTest.Step)
	}
	if branding.DeveloperTools && options.CaptureDirectory != "" && options.BriefingSmoke {
		data, err := assetdata.JsonBytes(struct {
			Trace         []menuSmokeRecord `json:"actions"`
			SaveDirectory string            `json:"scratch_save_directory"`
		}{briefingTest.Trace, options.FrontEnd.Store.Directory})
		if err != nil {
			return err
		}
		if _, err = assetdata.WriteNew(options.CaptureDirectory, "briefing-actions.json", data); err != nil {
			return err
		}
	}
	if branding.DeveloperTools && options.CaptureDirectory != "" && options.MenuSmoke {
		data, err := assetdata.JsonBytes(struct {
			Trace         []menuSmokeRecord `json:"actions"`
			SaveDirectory string            `json:"scratch_save_directory"`
		}{menuTest.Trace, options.FrontEnd.Store.Directory})
		if err != nil {
			return err
		}
		if _, err = assetdata.WriteNew(options.CaptureDirectory, "menu-actions.json", data); err != nil {
			return err
		}
	}
	if branding.DeveloperTools && options.CaptureDirectory != "" {
		data, err := assetdata.JsonBytes(struct {
			Checkpoints []smokeCheckpoint `json:"checkpoints"`
			Scope       string            `json:"scope"`
		}{checkpoints, "Own native window; programmatic resize and logical pointer path. Not an original-game capture or manual OS-input verification."})
		if err != nil {
			return err
		}
		if _, err = assetdata.WriteNew(options.CaptureDirectory, "window-checkpoints.json", data); err != nil {
			return err
		}
		triggerData, e := json.MarshalIndent(flightTriggerWitnesses, "", "  ")
		if e != nil {
			return e
		}
		if _, e = assetdata.WriteNew(options.CaptureDirectory, "flight-trigger.json", append(triggerData, '\n')); e != nil {
			return e
		}
	}
	return nil
}

func debriefResponseKey(key int32) bool {
	switch key {
	case rl.KeyLeftAlt, rl.KeyRightAlt, rl.KeyLeftControl, rl.KeyRightControl, rl.KeyLeftShift, rl.KeyRightShift, rl.KeyLeftSuper, rl.KeyRightSuper, rl.KeyCapsLock, rl.KeyNumLock, rl.KeyScrollLock:
		return false
	default:
		return key != 0
	}
}

// A physical Alt make may arrive a poll before Enter. It is a modifier, never
// an "any key" acknowledgment; otherwise the modal would close before the
// complete window chord can be recognized. Actual Alt commands keep their
// separately sampled owner, and ordinary Enter is unchanged.
func windowResponseMake(key int32, suppressEnter bool) bool {
	if key == rl.KeyLeftAlt || key == rl.KeyRightAlt {
		return false
	}
	return !suppressEnter || key != rl.KeyEnter && key != rl.KeyKpEnter
}

func drawPerformanceOverlay(meter *presentationstats.Meter) {
	if meter == nil {
		return
	}
	text := meter.Text
	w, h := int32(rl.GetScreenWidth()), int32(rl.GetScreenHeight())
	size := int32(10)
	if h >= 720 {
		size = 20
	}
	width := rl.MeasureText(text, size) + 8
	x := max(0, w-width-4)
	rl.DrawRectangle(x, 4, width, size+6, rl.NewColor(0, 0, 0, 220))
	rl.DrawText(text, x+4, 7, size, rl.RayWhite)
}
