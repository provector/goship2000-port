package main

import (
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/managedassets"
	menuui "gunship2000-assets/game/menus"
	"io"
	"os"
)

// These three seams let filesystem transition tests use small synthetic trees;
// production always uses the real parser, verified loader and ordinary renames.
type startupServices struct {
	prepare func(string, string) (*assetdata.Manifest, error)
	load    func(string) (*assetdata.OfficeAssets, error)
	rename  func(string, string) error
	ready   func(*assetdata.OfficeAssets) error
}

func defaultStartupServices() startupServices {
	return startupServices{prepare: prepareAssets, load: assetdata.LoadOfficeAssets, rename: os.Rename}
}

func startOffice(paths ApplicationPaths, rebuild bool, log io.Writer, services startupServices, display func(*menuui.Office) error) int {
	assets, err := ensureOfficeAssets(paths, rebuild, log, services)
	if err != nil {
		return startupError(err)
	}
	office, err := menuui.NewOffice(assets, menuui.DefaultOfficeState(), 1)
	if err != nil {
		return startupError(err)
	}
	fmt.Fprintln(log, "Opening office.")
	if err = display(office); err != nil {
		return startupError(err)
	}
	return 0
}

func sharedPaths(p ApplicationPaths) configuration.Paths {
	return configuration.Paths{RecordingLimit: p.RecordingLimit, Directory: p.install(), Original: p.Original, Assets: p.Assets, Saves: p.Saves, AssetsOrigin: p.AssetsOrigin, Config: configuration.Config(p.Config)}
}
func ensureOfficeAssets(p ApplicationPaths, rebuild bool, log io.Writer, s startupServices) (*assetdata.OfficeAssets, error) {
	return managedassets.Ensure(sharedPaths(p), rebuild, log, managedassets.Services{Prepare: s.prepare, Load: s.load, Rename: s.rename, Ready: s.ready})
}
func managedPaths(in, out string) (string, string, error) { return managedassets.ManagedPaths(in, out) }
func replaceableAssets(p string) error                    { return managedassets.ReplaceableAssets(p) }
func promotePreparation(target, generated string, w io.Writer, rename func(string, string) error) error {
	return managedassets.PromotePreparation(target, generated, w, rename)
}
func recoverPreparation(target string, w io.Writer, rename func(string, string) error) error {
	return managedassets.RecoverPreparation(target, w, rename)
}

type preparationRecovery struct {
	Version int    `json:"version"`
	Target  string `json:"target"`
	Backup  string `json:"backup_container"`
}
