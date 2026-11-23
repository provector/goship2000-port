package configuration

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
)

const Filename = "config.g2pak"

func Relative(directory, name string) string {
	if filepath.IsAbs(name) {
		return filepath.Clean(name)
	}
	return filepath.Join(directory, name)
}

// The same exact source-marker rule serves game and SETUP. Installed builds
// always choose their executable's directory, regardless of the caller's cwd.
func ApplicationDirectory(override, cwd, executable, module, mode string) (string, error) {
	directory := override
	if directory == "" {
		directory = filepath.Dir(executable)
		if mode != "installed" && module != "" {
			source := cwd
			if filepath.Base(cwd) == "runtime" {
				source = filepath.Dir(cwd)
			}
			b, e := os.ReadFile(filepath.Join(source, "go.mod"))
			m, me := os.Stat(filepath.Join(source, "game/main.go"))
			if e == nil && me == nil && m.Mode().IsRegular() {
				for _, l := range strings.Split(string(b), "\n") {
					f := strings.Fields(l)
					if len(f) == 2 && f[0] == "module" && strings.Trim(f[1], "\"") == module {
						directory = filepath.Join(source, "runtime")
						break
					}
				}
			}
		}
	}
	directory = Relative(cwd, directory)
	p, e := filepath.EvalSymlinks(directory)
	if e != nil {
		return "", fmt.Errorf("application directory %s: %w", directory, e)
	}
	i, e := os.Stat(p)
	if e != nil || !i.IsDir() {
		return "", fmt.Errorf("application directory must exist: %s", p)
	}
	return p, nil
}
func ResolveDirectory(override, mode string) (string, error) {
	cwd, e := os.Getwd()
	if e != nil {
		return "", e
	}
	exe, e := os.Executable()
	if e != nil {
		return "", e
	}
	module := ""
	if i, ok := debug.ReadBuildInfo(); ok {
		module = i.Main.Path
	}
	return ApplicationDirectory(override, cwd, exe, module, mode)
}

type Paths struct {
	RecordingLimit                     RecordingLimit
	Directory, Original, Assets, Saves string
	AssetsOrigin                       string
	Config                             Config
}

func LoadApplication(dir, assetsOverride string) (Paths, error) {
	return LoadApplicationAt(dir, dir, assetsOverride)
}
func LoadApplicationAt(dir, data, assetsOverride string) (Paths, error) {
	d, _, e := InitializeAt(filepath.Join(data, Filename), dir)
	if e != nil {
		return Paths{}, e
	}
	c := d.Config
	origin := "default-managed"
	if _, ok := d.raw["assets"]; ok {
		origin = "configured"
	}
	if origin == "configured" && !filepath.IsAbs(c.Assets) && filepath.Clean(c.Assets) == "assets" {
		origin = "configured-default-managed"
	}
	if assetsOverride != "" {
		c.Assets = assetsOverride
		origin = "command-line"
	}
	limit, e := ParseRecordingLimit(fmt.Sprint(c.ReplayMaxMiB))
	if e != nil {
		return Paths{}, e
	}
	return Paths{limit, dir, Relative(dir, c.Original), Relative(dir, c.Assets), SavePath(dir, data, c.Saves), origin, c}, nil
}
