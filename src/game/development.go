//go:build devtools

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"gunship2000-assets/game/configuration"
	menuui "gunship2000-assets/game/menus"
	"gunship2000-assets/game/render3d"
	gamestate "gunship2000-assets/game/state"
	"math"
	"math/rand"
	"os"
	"path/filepath"
)

func runDevelopment(args []string) int {
	flags := flag.NewFlagSet("gs2000 dev", flag.ContinueOnError)
	replayMiB := flags.String("replay-max-mib", "", "new recording MiB 1..1024; operational policy only")
	editorFixture := flags.Bool("editor-fixture", false, "fresh disposable established Captain; author a mission through ordinary UI")
	commanderFixture := flags.Bool("commander-fixture", false, "fresh disposable R1 Flight Captain; ordinary pad-command mission seed75")
	menus := flags.Bool("menus", false, "front-end preview using a fresh scratch save, never normal saves")
	viewer := flags.Bool("viewer", false, "prepared world/cockpit inspection with a scratch mission/save")
	scenario := flags.String("scenario", "", "paused disposable mission fixture; see Step N.5 manual checklist for names")
	reopenScenario := flags.Bool("reopen-scenario", false, "open the retained scratch pilot at the office; no fixture setup or unfinished-session resume")
	sceneSmoke := flags.Bool("scene-smoke", false, "connected preflight/render/return/quit smoke with scratch saves")
	sceneWorld := flags.String("scene-world", "", "standalone diagnostic world override; excludes mission instances")
	sceneCamera := flags.String("scene-camera", "", "JSON file with H0,H1,Height,Angles inspection camera")
	arrangement := flags.Int("arrangement", 0, "viewer cockpit arrangement0..3")
	cockpitAircraft := flags.Int("cockpit-aircraft", -1, "optional viewer artwork aircraft0..7; accepted equipment stays unchanged")
	visualTicks := flags.Int("visual-phase", 0, "explicit visual steps before scene presentation")
	visualPaused := flags.Bool("visual-paused", false, "freeze the viewer's logical visual state")
	previewRate := flags.Float64("preview-rate", menuui.DefaultPreviewVisualRate, "native preview policy Hz, not simulation frequency")
	exterior := flags.Bool("exterior", false, "viewer exterior presentation without cockpit artwork")
	noHUD := flags.Bool("no-hud", false, "viewer suppresses supplied-state diagnostic HUD")
	startup := flags.Bool("startup", false, "play prepared original startup, using a fresh scratch save")
	recoveryFixture := flags.Bool("recovery-fixture", false, "seed an explicitly synthetic pre-mission backup in the scratch menu save")
	screen := flags.String("screen", "office", "preview: office/roster/theatre/mission_type/recovery/quit_confirmation/briefing/papers/preflight_settings/outfit/ordnance")
	paper := flags.Int("paper", 0, "briefing paper0 Orders,1 Intelligence,2 Map,3 Decline (development only)")
	missionSeed := flags.Int64("mission-seed", 1, "independent mission generation seed (development only)")
	missionDraws := flags.String("mission-draws", "", "optional JSON array of bounded mission draws (development only)")
	preflightSmoke := flags.Bool("preflight-smoke", false, "connected five-aircraft equipment/settings scratch scenario")
	squadronFixture := flags.Bool("squadron-fixture", false, "source-initialize synthetic wingmen in the scratch save only")
	briefingSmoke := flags.Bool("briefing-smoke", false, "connected briefing/papers/settings scenario against a scratch save")
	menuSmoke := flags.Bool("menu-smoke", false, "run connected menu scenario against scratch save (native development only)")
	directory := flags.String("assets", "", "prepared directory; relative to application directory")
	app := flags.String("app-dir", "", "application directory")
	preset := flags.String("state", "", "optional JSON display-state overrides")
	seed := flags.Uint64("seed", 1, "reproducible local computer-effects seed")
	rate := flags.Float64("tick-rate", menuui.NominalOfficeTickRate, "nominal logical ticks/second (approximation)")
	headless := flags.Bool("headless", false, "write a deterministic indexed frame; no window")
	ticks := flags.Int("ticks", 0, "logical ticks before initial presentation/headless capture")
	frame := flags.String("frame", "", "new PNG path (default dev-output/frame.png under application directory)")
	pointer := flags.Bool("pointer", true, "include original pointer")
	px := flags.Int("pointer-x", 184, "initial logical pointer X")
	py := flags.Int("pointer-y", 70, "initial logical pointer Y")
	square := flags.Bool("square-pixels", false, "320:200 presentation instead of aspect-corrected4:3")
	width := flags.Int("width", configuration.DefaultWindowWidth, "initial window width")
	height := flags.Int("height", configuration.DefaultWindowHeight, "initial window height")
	smoke := flags.Int("smoke-frames", 0, "automated native-window smoke frames then close (0 disables)")
	capture := flags.String("capture-dir", "", "new directory for smoke screenshots/checkpoints")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 1
	}
	if flags.NArg() != 0 || *ticks < 0 || *ticks > 10000000 || math.IsNaN(*rate) || math.IsInf(*rate, 0) || *rate <= 0 || *rate > 1000 || *width < 320 || *height < 200 || *width > 7680 || *height > 4320 || *smoke < 0 || (!*headless && *frame != "") || (*capture != "" && *smoke == 0 && !*menuSmoke && !*briefingSmoke && !*preflightSmoke && !*sceneSmoke) {
		fmt.Fprintln(os.Stderr, "dev: use positive tick-rate<=1000, ticks0..10000000, window>=320x200, and -headless -frame NEW.png for capture")
		return 1
	}
	recordingLimit := configuration.RecordingLimit(0)
	var recordingErr error
	if *replayMiB != "" {
		recordingLimit, recordingErr = configuration.ParseRecordingLimit(*replayMiB)
	}
	if recordingErr != nil {
		return startupError(recordingErr)
	}
	if *paper < 0 || *paper > 3 {
		return startupError(fmt.Errorf("paper must be0..3"))
	}
	if *editorFixture {
		if *app == "" || *scenario != "" || *reopenScenario || *viewer || *startup {
			return startupError(fmt.Errorf("-editor-fixture needs a fresh explicit -app-dir and ordinary menus only"))
		}
		if err := validateDevelopmentScenarioArgs(flags, "o-clean-flight", *app); err != nil {
			return startupError(err)
		}
		*menus = true
	}
	if *commanderFixture {
		if *editorFixture || *app == "" || *scenario != "" || *reopenScenario || *viewer || *startup {
			return startupError(fmt.Errorf("-commander-fixture needs a fresh explicit -app-dir and ordinary menus only"))
		}
		if err := validateDevelopmentScenarioArgs(flags, "o-clean-flight", *app); err != nil {
			return startupError(err)
		}
		*missionSeed = 75
		*menus = true
	}
	if *scenario != "" {
		if err := validateDevelopmentScenarioArgs(flags, *scenario, *app); err != nil {
			return startupError(err)
		}
		*menus = true
	}
	if *viewer || *sceneSmoke {
		*menus = true
	}
	if *viewer {
		*screen = string(menuui.ScreenScene)
	}
	if *sceneSmoke && (*viewer || *startup || *preflightSmoke || *briefingSmoke || *menuSmoke) {
		return startupError(fmt.Errorf("choose scene-smoke without other smoke/startup/viewer scenarios"))
	}
	if *sceneSmoke && *headless {
		return startupError(fmt.Errorf("scene-smoke requires a native window; use viewer -headless for deterministic captures"))
	}
	if *briefingSmoke || *preflightSmoke {
		*menus = true
	}
	if *preflightSmoke && (*startup || *menuSmoke || *briefingSmoke) {
		return startupError(fmt.Errorf("choose one development smoke scenario"))
	}
	if *startup && (*menuSmoke || *briefingSmoke) || *menuSmoke && *briefingSmoke || *recoveryFixture && !*menus {
		return startupError(fmt.Errorf("startup, menu-smoke and briefing-smoke are separate scenarios; recovery-fixture requires menus"))
	}
	dir, err := configuration.ResolveDirectory(*app, applicationMode)
	if err != nil {
		return startupError(err)
	}
	var outputs []string
	for _, p := range []string{*frame, *capture} {
		if p != "" {
			outputs = append(outputs, appRelative(dir, p))
		}
	}
	owner, ownerErr := configuration.OpenLocation(*app, applicationMode, *directory, false, outputs...)
	if ownerErr != nil {
		return startupError(ownerErr)
	}
	defer owner.Close()
	paths, err := loadApplicationAt(owner.Install, owner.Data, *directory)
	if err != nil {
		return startupError(err)
	}
	if *replayMiB == "" {
		recordingLimit = paths.RecordingLimit
	}
	visited := map[string]bool{}
	flags.Visit(func(f *flag.Flag) { visited[f.Name] = true })
	dev := paths.Config.Development
	if !visited["seed"] {
		*seed = dev.Seed
	}
	if !visited["tick-rate"] {
		*rate = dev.TickRate
	}
	if !visited["preview-rate"] {
		*previewRate = dev.PreviewRate
	}
	if !visited["arrangement"] {
		*arrangement = dev.Arrangement
	}
	if !visited["cockpit-aircraft"] {
		*cockpitAircraft = dev.CockpitAircraft
	}
	if !visited["visual-paused"] {
		*visualPaused = dev.VisualPaused
	}
	if !visited["pointer"] {
		*pointer = dev.Pointer
	}
	if !visited["pointer-x"] {
		*px = dev.PointerX
	}
	if !visited["pointer-y"] {
		*py = dev.PointerY
	}
	if !visited["width"] {
		*width = paths.Config.WindowWidth
	}
	if !visited["height"] {
		*height = paths.Config.WindowHeight
	}
	if !visited["square-pixels"] {
		*square = paths.Config.SquarePixels
	}
	if *reopenScenario {
		if *app == "" || *scenario != "" || *headless || *viewer || *startup || *menuSmoke || *sceneSmoke || *briefingSmoke || *preflightSmoke {
			return startupError(fmt.Errorf("-reopen-scenario needs only explicit -app-dir and optional -assets"))
		}
		saves, err := reopenScenarioSave(paths.Directory)
		if err != nil {
			return startupError(err)
		}
		paths.Saves, paths.Original = saves, ""
		paths.RecordingLimit = recordingLimit
		var reopenedSeed *int64
		if visited["mission-seed"] {
			reopenedSeed = missionSeed
		}
		return startFrontEnd(paths, false, reopenedSeed, false)
	}
	if *frame == "" && *headless {
		*frame = filepath.Join(paths.Directory, "dev-output", "frame.png")
	}
	if *frame != "" {
		*frame = appRelative(paths.Directory, *frame)
	}
	if *capture != "" {
		*capture = appRelative(paths.Directory, *capture)
	}
	if *preset != "" {
		*preset = appRelative(paths.Directory, *preset)
	}
	services := defaultStartupServices()
	if *menus || *menuSmoke || *startup {
		services = frontEndStartupServices()
	}
	assets, err := ensureOfficeAssets(paths, false, os.Stdout, services)
	if err != nil {
		fmt.Fprintln(os.Stderr, "office assets:", err)
		return 1
	}
	state := menuui.DefaultOfficeState()
	if *preset != "" {
		state, err = menuui.ReadOfficeState(*preset)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	if (*menuSmoke || *briefingSmoke || *preflightSmoke) && *headless {
		return startupError(fmt.Errorf("menu-smoke needs a native window; use menus/screen for headless previews"))
	}
	if *menus || *menuSmoke || *startup {
		if *menuSmoke && *preset == "" {
			state.Rank = 7
		}
		controller, err := newDevelopmentFrontEnd(paths, assets, state, *seed)
		if err != nil {
			return startupError(err)
		}
		// Diagnostic screen entry may construct a scene before opening the window.
		// Apply the same runtime configuration before that first construction.
		controller.SimplifiedFields = paths.Config.SimplifiedFields
		controller.ConfigurePreflightGPS(paths.Config.LegacyCopyProtection, paths.Directory)
		controller.Runtime = paths.Config.Runtime
		controller.RecordingLimit = recordingLimit
		if *preflightSmoke && *preset == "" {
			next := controller.Store.State()
			p := next.SelectedPilot()
			p.Rank = gamestate.RankCaptain
			p.MissionType = gamestate.MissionFlight
			if err = controller.Store.Commit(next); err != nil {
				return startupError(err)
			}
			if err = controller.RefreshOffice(); err != nil {
				return startupError(err)
			}
		}
		if *squadronFixture || *preflightSmoke {
			next := controller.Store.State()
			p := next.SelectedPilot()
			rng := rand.New(rand.NewSource(*missionSeed))
			p.SquadronPilots, err = gamestate.InitializeSquadron(p, next.Slots[:], assets.Missions.Preflight.SquadronNames, rng.Intn)
			if err != nil {
				return startupError(err)
			}
			if err = controller.Store.Commit(next); err != nil {
				return startupError(err)
			}
		}
		if *editorFixture {
			if err = configureEditorAcceptance(controller, paths.Directory); err != nil {
				return startupError(err)
			}
		}
		if *commanderFixture {
			if err = configureCommanderAcceptance(controller, paths.Directory); err != nil {
				return startupError(err)
			}
		}
		controller.MissionSeed = missionSeed
		if *missionDraws != "" {
			name := appRelative(paths.Directory, *missionDraws)
			raw, err := os.ReadFile(name)
			if err != nil {
				return startupError(err)
			}
			if len(raw) > 16<<20 {
				return startupError(fmt.Errorf("mission draw stream exceeds16MiB"))
			}
			if err = json.Unmarshal(raw, &controller.MissionDrawStream); err != nil {
				return startupError(fmt.Errorf("mission draw stream: %w", err))
			}
			if controller.MissionDrawStream == nil {
				return startupError(fmt.Errorf("mission draw stream must be an array, not null"))
			}
		}
		if *recoveryFixture {
			if err = prepareDevelopmentRecovery(controller); err != nil {
				return startupError(err)
			}
		}
		if *scenario != "" {
			meta, setupErr := controller.ConfigureDevelopmentScenario(*scenario)
			if setupErr != nil {
				return startupError(setupErr)
			}
			raw, marshalErr := json.MarshalIndent(meta, "", "  ")
			if marshalErr != nil {
				return startupError(marshalErr)
			}
			metaPath := filepath.Join(paths.Directory, "dev-output", "scenario.json")
			if err = os.WriteFile(metaPath, append(raw, '\n'), 0600); err != nil {
				return startupError(err)
			}
			fmt.Printf("Development supplied-state scenario metadata: %s\n", metaPath)
		} else if !*startup {
			err = menuui.PreviewScreen(controller, menuui.Screen(*screen))
		}
		if err != nil {
			return startupError(err)
		}
		if controller.Mission != nil {
			controller.Mission.Page = *paper
			controller.Mission.View.SetPaper(*paper)
		}
		if *viewer {
			opts := menuui.RenderingInspection{World: *sceneWorld, Arrangement: *arrangement, VisualTicks: *visualTicks, Paused: *visualPaused, Rate: *previewRate, Exterior: *exterior, NoHUD: *noHUD}
			if *cockpitAircraft >= 0 {
				opts.CockpitAircraft = cockpitAircraft
			}
			if *sceneCamera != "" {
				raw, err := os.ReadFile(appRelative(paths.Directory, *sceneCamera))
				if err != nil {
					return startupError(err)
				}
				var camera render3d.Camera
				if err = json.Unmarshal(raw, &camera); err != nil {
					return startupError(fmt.Errorf("viewer camera: %w", err))
				}
				opts.Camera = &camera
			}
			if err = controller.ConfigureRenderingInspection(opts); err != nil {
				return startupError(err)
			}
		}
		if *startup {
			if *screen != "office" && *screen != "quit_confirmation" {
				return startupError(fmt.Errorf("startup supports office handoff or quit_confirmation preview"))
			}
			if err = controller.BeginStartup(assets.Startup); err != nil {
				return startupError(err)
			}
		}
		for i := 0; i < *ticks; i++ {
			if err = controller.AdvanceStartup(1 / *rate); err != nil {
				return startupError(err)
			}
			if err = controller.Tick(); err != nil {
				return startupError(err)
			}
		}
		if *startup && *screen == "quit_confirmation" {
			if err = controller.Handle(menuui.Actions{Quit: true}); err != nil {
				return startupError(err)
			}
		}
		controller.Office.SetPointer(*px, *py, true)
		// Headless previews share the controller's real hover path as well as the
		// pointer sprite coordinates; no activation event is sent.
		if err = controller.Handle(menuui.Actions{X: *px, Y: *py, Inside: true}); err != nil {
			return startupError(err)
		}
		if *headless {
			err = menuui.WriteFrontEndFrame(*frame, controller, *pointer)
		} else {
			controller.FadeMilliseconds = menuui.DefaultSceneFadeMilliseconds
			if controller.Screen == menuui.ScreenOffice {
				controller.BeginSceneFade(nil)
			}
			if (*menuSmoke || *briefingSmoke || *preflightSmoke || *sceneSmoke) && *smoke == 0 {
				*smoke = 1200
			}
			err = menuui.RunOfficeWindow(controller.Office, menuui.WindowOptions{WindowMode: configuration.Config(paths.Config).WindowMode(), SavesDirectory: paths.Saves, InstallDirectory: paths.install(), AppDirectory: paths.Directory, Runtime: paths.Config.Runtime, DrawFPSCPU: paths.Config.DrawFPSCPU, FrontEnd: controller, AudioPreferences: paths.Config.audioPreferences(), SoundEffectsEnabled: &paths.Config.SoundEffectsEnabled, MusicEnabled: &paths.Config.MusicEnabled, Width: *width, Height: *height, SquarePixels: *square, DrawPointer: *pointer, TickRate: *rate, Development: true, MenuSmoke: *menuSmoke, BriefingSmoke: *briefingSmoke, PreflightSmoke: *preflightSmoke, SceneSmoke: *sceneSmoke, StartupSmoke: *startup && *smoke > 0, SmokeFrames: *smoke, CaptureDirectory: *capture})
		}
		if err != nil {
			return startupError(err)
		}
		return 0
	}
	if *screen != "office" {
		return startupError(fmt.Errorf("screen requires -menus"))
	}
	office, err := menuui.NewOffice(assets, state, *seed)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	office.SetPointer(*px, *py, true)
	for i := 0; i < *ticks; i++ {
		office.Tick()
	}
	if *headless {
		err = menuui.WriteOfficeFrame(*frame, office, *pointer, *seed, *rate)
	} else {
		fmt.Println("Office presentation: C computer mode; P/Space pause; N/. single tick while paused; Escape closes. No destination navigation.")
		err = menuui.RunOfficeWindow(office, menuui.WindowOptions{WindowMode: configuration.Config(paths.Config).WindowMode(), SavesDirectory: paths.Saves, Runtime: paths.Config.Runtime, DrawFPSCPU: paths.Config.DrawFPSCPU, Width: *width, Height: *height, SquarePixels: *square, DrawPointer: *pointer, TickRate: *rate, SmokeFrames: *smoke, CaptureDirectory: *capture, Development: true})
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "office:", err)
		return 1
	}
	return 0
}
