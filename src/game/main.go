package main

import (
	"errors"
	"flag"
	"fmt"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/diagnostics"
	"gunship2000-assets/game/logview"
	"gunship2000-assets/game/replay"
	"gunship2000-assets/game/state"
	"gunship2000-assets/internal/branding"
	"gunship2000-assets/internal/instancealert"
	"gunship2000-assets/internal/storeowner"
	"log/slog"
	"os"
	"path/filepath"
)

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "__log-console":
			if err := logview.Run(); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
			return 0
		case "portable-films":
			f := flag.NewFlagSet("portable-films", flag.ContinueOnError)
			app := f.String("app-dir", "", "application root")
			if f.Parse(args[1:]) != nil || f.NArg() != 0 {
				return 1
			}
			owner, ownerErr := configuration.OpenLocation(*app, applicationMode, "", false)
			if ownerErr != nil {
				return startupError(ownerErr)
			}
			defer owner.Close()
			paths, e := loadApplicationAt(owner.Install, owner.Data, "")
			if e == nil {
				store := replay.Store{Root: filepath.Join(paths.Saves, "replays")}
				e = errors.Join(store.MigrateCompanions(), store.ReconcileNames())
			}
			if e != nil {
				return startupError(e)
			}
			return 0
		case "migrate-storage":
			f := flag.NewFlagSet("migrate-storage", flag.ContinueOnError)
			app := f.String("app-dir", "", "application directory")
			if e := f.Parse(args[1:]); e != nil || f.NArg() != 0 {
				return 1
			}
			owner, ownerErr := configuration.OpenLocation(*app, applicationMode, "", false)
			if ownerErr != nil {
				return startupError(ownerErr)
			}
			defer owner.Close()
			paths, e := loadApplicationAt(owner.Install, owner.Data, "")
			if e == nil {
				e = state.MigrateCareerAt(paths.Saves, paths.Original, paths.Directory, paths.install())
			}
			if e != nil {
				return startupError(e)
			}
			return 0
		case "films":
			return runFilms(args[1:])
		case "import-campaign":
			return runCampaignImport(args[1:])
		case "prepare":
			return runPreparation(args[1:])
		case "dev":
			return runDevelopment(args[1:])
		case "office":
			return runNormal(args[1:]) // Compatibility alias, with player options only.
		case "-input", "-output":
			return runPreparation(args)
		}
	}
	return runGame(args, len(args) == 0)
}

func runNormal(args []string) int { return runGame(args, false) }

func runGame(args []string, brandSplash bool) (code int) {
	var saves string
	handedOff := false
	defer func() {
		if handedOff {
			return
		}
		if v := recover(); v != nil {
			code = showGameStartup(saves, fmt.Errorf("startup panic: %v", v))
		}
	}()

	flags := flag.NewFlagSet("gs2000", flag.ContinueOnError)
	app := flags.String("app-dir", "", "application directory (config.g2pak, original/, assets/)")
	assets := flags.String("assets", "", "optional prepared directory; relative to application directory")
	replayMiB := flags.String("replay-max-mib", "", "maximum stored recording size in MiB (1..1024)")
	rebuild := flags.Bool("rebuild", false, "regenerate managed assets from configured originals")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: gs2000 [options]\nRun without arguments to prepare assets if needed and display the office.\nMaintenance: gs2000 prepare -input DIRECTORY -output NEW_DIRECTORY\nFilms: gs2000 films -help")
		if branding.DeveloperTools {
			fmt.Fprintln(flags.Output(), "Development: gs2000 dev -help")
		}
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 1
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "unexpected arguments; use -help")
		return 1
	}
	if *replayMiB != "" {
		if _, e := configuration.ParseRecordingLimit(*replayMiB); e != nil {
			return startupError(e)
		}
	}
	owner, ownerErr := configuration.OpenLocation(*app, applicationMode, *assets, *rebuild)
	if ownerErr != nil {
		return showGameStartup("", ownerErr)
	}
	defer owner.Close()
	saves = filepath.Join(owner.Data, "saves")
	paths, err := loadApplicationAt(owner.Install, owner.Data, *assets)
	if err != nil {
		showGameStartup(saves, err)
		var missing *configuration.MissingError
		if errors.As(err, &missing) {
			return 2
		}
		return 1
	}
	if *replayMiB != "" {
		limit, e := configuration.ParseRecordingLimit(*replayMiB)
		if e != nil {
			return startupError(e)
		}
		paths.RecordingLimit = limit
	}
	handedOff = true
	return startFrontEnd(paths, *rebuild, nil, brandSplash)

}

func showGameStartup(saves string, err error) int {
	if errors.Is(err, storeowner.ErrInUse) {
		instancealert.Show(err)
		return startupError(err)
	}
	reported := diagnostics.StartupError("Game", saves, err)
	instancealert.Startup("Game", reported.Error()+"\n\nCheck Original Installation and the paths in SETUP, then try again.")
	return startupError(reported)
}

func startupError(err error) int {
	diagnostics.Existing(slog.LevelError, "startup", "startup", err, os.Stderr)
	return 1
}

func runPreparation(args []string) int {
	flags := flag.NewFlagSet("gs2000 prepare", flag.ContinueOnError)
	input := flags.String("input", "", "original installation directory (read only)")
	output := flags.String("output", "", "new output directory; must not exist")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 1
	}
	if *input == "" || *output == "" || flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: gs2000 prepare -input DIRECTORY -output NEW_DIRECTORY\n       gs2000 [-app-dir DIRECTORY] [-assets DIRECTORY] [-rebuild]")
		return 1
	}
	owner, ownerErr := configuration.OwnOutput(*output)
	if ownerErr != nil {
		return startupError(ownerErr)
	}
	defer owner.Close()
	manifest, err := prepareAssets(*input, *output)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fatal extraction error:", err)
		return 1
	}
	s := manifest.Summary
	fmt.Printf("%s: %d containers; %d loose files inventoried, %d copied, %d executables inventoried\n", manifest.Result, s.Containers, s.LooseFiles, s.LooseCopied, s.Executables)
	fmt.Printf("%d archive entries extracted and byte-verified; %d pictures (%d supported-colour, %d diagnostic); %d palettes exported\n", s.Entries, s.Pictures, s.ColourPictures, s.DiagnosticPictures, s.Palettes)
	fmt.Printf("%d unsupported resources; %d conversion failures; manifest: %s/manifest.json\n", s.Unsupported, s.ConversionFailures, *output)
	fmt.Printf("%d font selectors exported; office metadata: %s\n", s.FontSelectors, manifest.OfficeMetadata)
	if s.AudioFMTracks+s.AudioDigitalTracks > 0 {
		fmt.Printf("%d FM and %d digital audio tracks; metadata: %s\n", s.AudioFMTracks, s.AudioDigitalTracks, manifest.AudioMetadata)
	}
	if s.StartupAnimations > 0 {
		fmt.Printf("%d startup animations; %d presentation frames including palette updates\n", s.StartupAnimations, s.StartupFrames)
	}
	for _, operation := range manifest.UnsupportedVersionOperations {
		fmt.Printf("profile limitation: %s: %s (supported: %s)\n", operation.Operation, operation.Reason, operation.Profile)
	}
	if s.ConversionFailures > 0 || s.Unsupported > 0 {
		return 2
	}
	return 0
}
