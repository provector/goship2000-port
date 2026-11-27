//go:build devtools

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	menuui "gunship2000-assets/game/menus"
	"os"
	"path/filepath"
	"strings"
)

func validateDevelopmentScenarioArgs(flags *flag.FlagSet, name, application string) error {
	found := false
	for _, candidate := range menuui.DevelopmentScenarioNames {
		found = found || candidate == name
	}
	if !found {
		return fmt.Errorf("unknown scenario %q; choose %v", name, menuui.DevelopmentScenarioNames)
	}
	if application == "" {
		return fmt.Errorf("-scenario requires an explicit fresh disposable -app-dir; normal application data is never used")
	}
	// A small named fixture owns its seed/history/setup. Mixing another setup
	// route would make the documented starting state misleading.
	conflict := ""
	flags.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "viewer", "scene-smoke", "startup", "preflight-smoke", "briefing-smoke", "menu-smoke", "recovery-fixture", "squadron-fixture", "state", "mission-seed", "mission-draws", "screen", "paper", "scene-world", "scene-camera", "arrangement", "cockpit-aircraft", "visual-phase", "visual-paused", "preview-rate", "exterior", "no-hud", "ticks":
			conflict = f.Name
		}
	})
	if conflict != "" {
		return fmt.Errorf("-scenario owns its setup; do not combine it with -%s", conflict)
	}
	directory, err := filepath.Abs(application)
	if err != nil {
		return err
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return fmt.Errorf("create a new disposable application directory first: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("scenario application must be a real directory")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		// An explicit existing config is allowed for320/640 or smooth/stepped
		// comparisons. No scenario mutates that configuration file.
		if !allowedScenarioFile(entry) {
			return fmt.Errorf("scenario -app-dir must be fresh (empty or config.cfg only); found %s", entry.Name())
		}
	}
	return nil
}

// Reopening reads the pointer emitted by the named disposable setup. It does
// not create a fresh pilot or resume an unfinished flight/debrief transaction.
func reopenScenarioSave(application string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(application, "dev-output", "scenario.json"))
	if os.IsNotExist(err) {
		raw, err = os.ReadFile(filepath.Join(application, "dev-output", "editor-profile.json"))
	}
	if err != nil {
		return "", err
	}
	if len(raw) > 1<<20 {
		return "", fmt.Errorf("scenario metadata too large")
	}
	var meta struct {
		ScratchSave string `json:"scratch_save"`
	}
	if err = json.Unmarshal(raw, &meta); err != nil {
		return "", err
	}
	path, err := assetdata.ResolvedNewPath(meta.ScratchSave)
	if err != nil {
		return "", err
	}
	parent, err := filepath.Abs(filepath.Join(application, "dev-output"))
	if err != nil {
		return "", err
	}
	if filepath.Dir(path) != parent || !strings.HasPrefix(filepath.Base(path), "menu-save-") {
		return "", fmt.Errorf("scratch pointer is outside this development application's owned save directory")
	}
	st, err := os.Lstat(filepath.Join(path, "player0.g2pak"))
	if os.IsNotExist(err) {
		st, err = os.Lstat(filepath.Join(path, "pilots", "player0.g2pak"))
	}
	if os.IsNotExist(err) {
		st, err = os.Lstat(filepath.Join(path, "roster.json"))
	}
	if err != nil {
		return "", err
	}
	if !st.Mode().IsRegular() {
		return "", fmt.Errorf("scratch roster must be a regular file")
	}
	return path, nil
}

func allowedScenarioFile(e os.DirEntry) bool {
	switch e.Name() {
	case "config.cfg", "config.g2pak", ".t3-config-migrated.json", "config.g2pak.save.lock.lease", "config.cfg.save.lock.lease", "controls.cfg.save.lock.lease":
		return e.Type().IsRegular()
	case ".runtime-locks", ".t3-config-backup":
		return e.IsDir()
	}
	return false
}
