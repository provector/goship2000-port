package main

import (
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/diagnostics"
	"gunship2000-assets/game/logview"
	"gunship2000-assets/game/managedassets"
	menuui "gunship2000-assets/game/menus"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"log/slog"
	"os"
	"path/filepath"
)

func frontEndStartupServices() startupServices {
	services := defaultStartupServices()
	services.load = assetdata.LoadFrontEndAssets
	services.ready = assetdata.ValidateCurrentFrontEndCapabilities
	return services
}

func validateSaveSeparation(paths ApplicationPaths) error {
	saves, err := assetdata.ResolvedNewPath(paths.Saves)
	if err != nil {
		return err
	}
	assets, err := assetdata.ResolvedNewPath(paths.Assets)
	if err != nil {
		return err
	}
	if assetdata.Within(saves, assets) || assetdata.Within(assets, saves) {
		return fmt.Errorf("saves and generated assets directories must not overlap")
	}
	return nil
}

// Ordinary startup never supplies a mission seed. Explicit owned development
// reopen can retain its reproducible scenario without changing normal startup.
func newRuntimeFrontEnd(assets *assetdata.OfficeAssets, store *gamestate.SaveStore, seed *int64) (*menuui.FrontEnd, error) {
	c, err := menuui.NewFrontEnd(assets, store)
	if err != nil {
		return nil, err
	}
	if seed != nil {
		value := *seed
		c.MissionSeed = &value
	}
	return c, nil
}

func startFrontEnd(paths ApplicationPaths, rebuild bool, missionSeed *int64, brandSplash bool) (code int) {
	started := false

	if err := validateSaveSeparation(paths); err != nil {
		return showGameStartup("", err)
	}
	if err := gamestate.PrepareExistingCareer(paths.Saves, paths.Directory); err != nil {
		return showGameStartup(paths.Saves, err)
	}
	logging := paths.Config.Logging
	session, sessionErr := diagnostics.StartSession(paths.Saves, menuui.DiagnosticBuildIdentity(), fmt.Sprintf("resolution %s, presentation %s, recording budget %d MiB, log level %s", paths.Config.Runtime.SimulationResolution, paths.Config.Runtime.PresentationMode, int64(paths.RecordingLimit.Effective())>>20, logging.Level))
	if sessionErr != nil {
		fmt.Fprintln(os.Stderr, "Crash diagnostics unavailable:", sessionErr)
	}
	defer func() {
		if v := recover(); v != nil {
			if started {
				panic(v)
			}
			code = showGameStartup(paths.Saves, fmt.Errorf("startup panic: %v", v))
		}
		if session != nil {
			session.Close()
		}
	}()
	var logs *diagnostics.Runtime
	if logging.ShowLog || logging.WriteLog {
		opts := diagnostics.Destinations(logging, os.Stdout, os.Stderr, logview.Open)
		opts.AsyncFile = true
		logs = diagnostics.Start(paths.Directory, logging, opts)
	}
	defer logs.Close()
	if diagnostics.Enabled(slog.LevelInfo) {
		diagnostics.Info("startup", "Launch configuration", "app", paths.Directory, "assets", paths.Assets, "saves", paths.Saves, "render_scale", paths.Config.Runtime.SimulationRenderScale, "simulation_resolution", paths.Config.Runtime.SimulationResolution, "presentation", paths.Config.Runtime.PresentationMode, "presentation_fps", paths.Config.Runtime.PresentationFPS, "fullscreen_preference", paths.Config.Fullscreen, "recording_mib", int64(paths.RecordingLimit.Effective())/(1024*1024), "show_log", logging.ShowLog, "terminal_target", logging.TerminalTarget, "write_log", logging.WriteLog, "log_file", configurationLogPath(paths))
	}

	fmt.Printf("User data: %s\n", paths.Directory)
	fmt.Printf("Assets: %s\n", paths.Assets)
	var assets *assetdata.OfficeAssets
	err := menuui.PreparationWindow(paths.Saves, func(progress func(assetdata.PreparationProgress)) error {
		var e error
		stage := "Locate original installation"
		services := managedassets.FrontEndServices(func(p assetdata.PreparationProgress) { stage = p.Stage; progress(p) })
		services.SourceUsed = func(path string) { paths.Original = path }
		assets, e = managedassets.Ensure(sharedPaths(paths), rebuild, os.Stdout, services)
		if e != nil {
			return fmt.Errorf("%s (original %s, destination %s): %w", stage, paths.Original, paths.Assets, e)
		}
		return nil
	})
	if err != nil {
		return showGameStartup(paths.Saves, err)
	}
	store, err := gamestate.OpenManagedCareerAt(paths.Saves, paths.Original, paths.Directory, paths.install(), true)
	if err != nil {
		return showGameStartup(paths.Saves, err)
	}
	defer store.Close()
	if err = store.RelocateLibraries(paths.Directory); err != nil {
		return showGameStartup(paths.Saves, err)
	}
	if err := (replay.Store{Root: filepath.Join(paths.Saves, "replays")}).MigrateCompanions(); err != nil {
		diagnostics.Warn("films", "Recording migration left individual files unchanged", "reason", err)
	}
	controller, err := newRuntimeFrontEnd(assets, store, missionSeed)
	if err != nil {
		return showGameStartup(paths.Saves, err)
	}
	controller.RecordingLimit = paths.RecordingLimit.Effective()
	fmt.Printf("New recording limit: %d MiB (compressed file, including index and ending)\n", int64(controller.RecordingLimit)/(1024*1024))
	controller.FilmDirectory = filepath.Join(paths.Saves, "replays")
	controller.MissionDirectory = filepath.Join(paths.Saves, "missions")
	controller.SummaryDirectory = paths.Saves
	controller.FadeMilliseconds = paths.Config.FadeMilliseconds
	if err = controller.BeginStartup(assets.Startup); err != nil {
		return showGameStartup(paths.Saves, err)
	}
	fmt.Printf("Player packs: %s\n", store.PackDirectory())
	fmt.Printf("Recordings: %s\n", controller.FilmDirectory)
	if p := store.State().SelectedPilot(); p != nil {
		fmt.Printf("Selected pilot: %s (%s)\n", p.Name, p.Rank)
	}
	controller.SimplifiedFields = paths.Config.SimplifiedFields
	controller.ConfigurePreflightGPS(paths.Config.LegacyCopyProtection, paths.Directory)
	controller.Runtime = paths.Config.Runtime
	err = menuui.RunOfficeWindow(controller.Office, menuui.WindowOptions{OnStarted: func() { started = true }, BrandSplash: brandSplash, InstallDirectory: paths.install(), AppDirectory: paths.Directory, Runtime: paths.Config.Runtime, DrawFPSCPU: paths.Config.DrawFPSCPU, FrontEnd: controller, AudioPreferences: paths.Config.audioPreferences(), SoundEffectsEnabled: &paths.Config.SoundEffectsEnabled, MusicEnabled: &paths.Config.MusicEnabled, WindowMode: configuration.Config(paths.Config).WindowMode(), SavesDirectory: paths.Saves, Width: paths.Config.WindowWidth, Height: paths.Config.WindowHeight, SquarePixels: paths.Config.SquarePixels, DrawPointer: true, TickRate: menuui.NominalOfficeTickRate})
	if err != nil {
		if !started {
			return showGameStartup(paths.Saves, err)
		}
		diagnostics.Report("window", err.Error(), "The game window could not continue.")
		return startupError(err)
	}
	diagnostics.Info("startup", "Game window closed normally")
	return 0
}

func configurationLogPath(paths ApplicationPaths) string {
	return filepath.Join(paths.Directory, "logs", paths.Config.Logging.FileName)
}
