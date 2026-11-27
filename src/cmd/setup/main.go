package main

import (
	"errors"
	"flag"
	"fmt"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/diagnostics"
	"gunship2000-assets/game/setupui"
	"gunship2000-assets/game/state"
	"gunship2000-assets/internal/instancealert"
	"gunship2000-assets/internal/playerdirs"
	"gunship2000-assets/internal/storeowner"
	"os"
	"path/filepath"
)

var applicationMode = "auto"

func main() { os.Exit(run()) }

func setupStartupError(saves string, err error) int {
	fmt.Fprintln(os.Stderr, err)
	if errors.Is(err, storeowner.ErrInUse) {
		instancealert.Show(err)
		return 1
	}
	reported := diagnostics.StartupError("SETUP", saves, err)
	instancealert.Startup("SETUP", reported.Error())
	return 1
}

func run() (code int) {
	var saves string
	defer func() {
		if v := recover(); v != nil {
			code = setupStartupError(saves, fmt.Errorf("unexpected SETUP failure: %v", v))
		}
	}()
	// Resolve console output before flag parsing (including -help).
	if len(os.Args) > 1 {
		attachCLI()
	}
	f := flag.NewFlagSet("setup", flag.ContinueOnError)
	debug := f.Bool("debug", false, "show the DEBUG menu for this SETUP session")
	child := f.Bool("terminal-child", false, "internal terminal relaunch guard")
	f.String("terminal-ready", "", "internal terminal readiness file")
	list := f.Bool("list-controllers", false, "list raw joystick capabilities without a window, configuration write or game assets")
	watch := f.Int("watch-controllers", 0, "print raw joystick samples for1..300seconds (explicit diagnostic)")
	installRoot := f.String("terminal-install", "", "internal terminal launch directory")
	app := f.String("app-dir", "", "shared application directory (normally automatic)")
	initOnly := f.Bool("init", false, "create missing default config.g2pak and exit (explicit scriptable action)")
	check := f.Bool("check", false, "validate existing config.g2pak without writing")
	if e := f.Parse(os.Args[1:]); e != nil {
		return 1
	}
	if f.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "unexpected arguments")
		return 1
	}
	resolvedApp := *app
	if *child && *installRoot != "" {
		resolvedApp = *installRoot
	}
	dir, e := configuration.ResolveDirectory(resolvedApp, applicationMode)
	if e != nil {
		return setupStartupError("", e)
	}
	if *watch < 0 || *watch > 300 {
		fmt.Fprintln(os.Stderr, "watch-controllers:1..300seconds or0 (off)")
		return 1
	}
	if *list || *watch > 0 {
		return controllerDiagnostics(*watch)
	}
	path := filepath.Join(dir, configuration.Filename)
	var d *configuration.Document
	var owner *configuration.Location
	if *check {
		path, e = configuration.ExistingConfigPath(dir, *app)
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			return 1
		}
		d, e = configuration.LoadAt(path, dir)
	} else {
		var err error
		owner, err = configuration.OpenLocationAt(dir, *app, "", false)
		if err != nil {
			return setupStartupError("", err)
		}
		defer owner.Close()
		saves = filepath.Join(owner.Data, "saves")
		path = filepath.Join(owner.Data, configuration.Filename)
		d, _, e = configuration.InitializeAt(path, dir)
	}
	if e != nil {
		if *check {
			fmt.Fprintln(os.Stderr, e)
			return 1
		}
		return setupStartupError(saves, e)
	}
	fmt.Println("Configuration:", path)
	if !*check {
		saves = configuration.SavePath(dir, owner.Data, d.Config.Saves)
		playerdirs.Archive(saves, "reports", false)
		if err := state.PrepareExistingCareer(saves, owner.Data); err != nil {
			return setupStartupError(saves, err)
		}
	}
	if *initOnly || *check {
		return 0
	}
	ui := setupui.New(d)
	ui.Location = owner
	if os.Getenv("GOSHIP2000_DATA_DIR") != "" {
		ui.LocationOverride = "GOSHIP2000_DATA_DIR"
	} else if *app != "" {
		ui.LocationOverride = "-app-dir"
	}
	if *debug {
		ui.RevealDebug()
	}
	if e = ui.RunWindow(); e != nil {
		return setupStartupError(saves, fmt.Errorf("SETUP window: %w", e))
	}
	return 0
}
