package managedassets

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
)

type Installation struct {
	Source, Path string
}

func (i Installation) String() string {
	switch i.Source {
	case configuration.OriginalSteam:
		return "Steam — " + i.Path
	case configuration.OriginalGOG:
		return "GOG — " + i.Path
	default:
		return "Custom — " + i.Path
	}
}

func InstalledGames() []Installation {
	return installedGames(steamRoots(), gogRoots())
}

func installedGames(steam, gog []string) []Installation {
	var out []Installation
	add := func(source, path string) {
		if p, err := filepath.EvalSymlinks(path); err == nil {
			path = p
		}
		for _, i := range out {
			if SameInstallation(i.Path, path) {
				return
			}
		}
		out = append(out, Installation{source, path})
	}
	for _, path := range steamInstallations(steam, directoryExists) {
		add(configuration.OriginalSteam, path)
	}
	for _, root := range gog {
		if path := gogDataDirectory(root); path != "" {
			add(configuration.OriginalGOG, path)
		}
	}
	return out
}

func directoryExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func SameInstallation(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if a == b || runtime.GOOS == "windows" && strings.EqualFold(a, b) {
		return true
	}
	left, e := os.Stat(a)
	right, f := os.Stat(b)
	return e == nil && f == nil && os.SameFile(left, right)
}

// GOG's current Gunship bundle and Steam have an identically named data
// wrapper. Standalone installations can expose the CD directory directly.
func gogDataDirectory(root string) string {
	if !filepath.IsAbs(root) {
		return ""
	}
	nested := filepath.Join(root, "GD", "Gunship2000", "MPS", "GS2000CD")
	if directoryExists(nested) {
		return nested
	}
	for _, p := range []string{filepath.Join(root, "game"), root} {
		if data, e := assetdata.OriginalDirectory(p); e == nil && supportedSteamData(data) {
			return data
		}
	}
	return ""
}

func gogTitle(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "gunship" || s == "gunship 2000" || strings.HasPrefix(s, "gunship 2000:")
}

// An offline GOG installer leaves the same local product metadata as Galaxy.
func hasGOGProduct(root string) bool {
	entries, e := os.ReadDir(root)
	if e != nil {
		return false
	}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.Type().IsRegular() || !strings.HasPrefix(name, "goggame-") || !strings.HasSuffix(name, ".info") {
			continue
		}
		b, e := assetdata.ReadLimitedFile(filepath.Join(root, name), 256<<10)
		var product struct{ Name, GameID string }
		if e == nil && json.Unmarshal(b, &product) == nil && product.GameID != "" && name == "goggame-"+product.GameID+".info" && gogTitle(product.Name) {
			return true
		}
	}
	return false
}

func PreferredInstallation(c configuration.Config, install string, detected []Installation) (Installation, bool) {
	if c.OriginalSource == configuration.OriginalUnselected {
		for _, source := range []string{configuration.OriginalSteam, configuration.OriginalGOG} {
			for _, i := range detected {
				if i.Source == source {
					return i, true
				}
			}
		}
		return Installation{configuration.OriginalCustom, filepath.Join(install, configuration.DefaultOriginal)}, false
	}
	source := c.OriginalSource
	if source == "" {
		source = configuration.OriginalCustom
	}
	return Installation{source, configuration.Relative(install, c.Original)}, true
}

func sourceCandidates(c configuration.Config, install string, detected []Installation) ([]Installation, error) {
	preferred, ok := PreferredInstallation(c, install, detected)
	if !ok {
		return nil, fmt.Errorf("%w: no Steam or GOG installation was detected. Select a source in SETUP > Port Settings > Original Installation (suggested Custom folder: %s)", ErrOriginalUnavailable, preferred.Path)
	}
	out := []Installation{preferred}
	if preferred.Source == configuration.OriginalCustom {
		return out, nil
	}
	for _, i := range detected {
		if i.Source != preferred.Source && (i.Source == configuration.OriginalGOG || i.Source == configuration.OriginalSteam) && !SameInstallation(i.Path, preferred.Path) {
			return append(out, i), nil
		}
	}
	return out, nil
}
