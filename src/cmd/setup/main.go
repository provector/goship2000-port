package main

import (
	"bufio"
	"flag"
	"fmt"
	"golang.org/x/term"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/setupui"
	"os"
	"path/filepath"
)

var applicationMode = "auto"

var terminalChild bool

func main() {
	code := run()
	if code != 0 && terminalChild && term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintln(os.Stderr, "SETUP failed. Press Enter to close this terminal.")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	}
	os.Exit(code)
}
func run() int {
	f := flag.NewFlagSet("setup", flag.ContinueOnError)
	debug := f.Bool("debug", false, "show the DEBUG menu for this SETUP session")
	child := f.Bool("terminal-child", false, "internal terminal relaunch guard")
	ready := f.String("terminal-ready", "", "internal terminal readiness file")
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
		fmt.Fprintln(os.Stderr, e)
		return 1
	}
	if *watch < 0 || *watch > 300 {
		fmt.Fprintln(os.Stderr, "watch-controllers:1..300seconds or0 (off)")
		return 1
	}
	if *list || *watch > 0 {
		return controllerDiagnostics(*watch)
	}
	terminalChild = *child
	if !*initOnly && !*check {
		if handled, code := ensureTerminal(dir, os.Args[1:], *child, *ready); handled {
			return code
		}
	}
	if !*initOnly && !*check {
		sizeOwnedTerminal(*child)
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
			terminalChild = false // a refused opener exits without waiting for another key
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer owner.Close()
		path = filepath.Join(owner.Data, configuration.Filename)
		d, _, e = configuration.InitializeAt(path, dir)
	}
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		return 1
	}
	fmt.Println("Configuration:", path)
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
	if e = ui.Run(); e != nil {
		fmt.Fprintln(os.Stderr, "SETUP terminal:", e, "\nRun setup from a terminal (80x25 or larger).")
		return 1
	}
	return 0
}
