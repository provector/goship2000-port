package configuration

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Location and its ownership handle are retained until all writing has finished.
type Location struct {
	*os.File
	Install, Data string
}

func SavePath(install, data, name string) string {
	if filepath.IsLocal(name) {
		return Relative(data, name)
	}
	// Preserve existing external relative paths, including ../careers.
	return Relative(install, name)
}
func DataChoice(install, appOverride string) (string, error) {
	if path := os.Getenv("GOSHIP2000_DATA_DIR"); path != "" {
		if !filepath.IsAbs(path) {
			return "", fmt.Errorf("GOSHIP2000_DATA_DIR must be absolute")
		}
		return filepath.Clean(path), nil
	}
	if appOverride != "" {
		return install, nil
	}
	path, _, err := locatedData()
	return path, err
}
func OpenLocation(override, mode, assets string, rebuild bool, outputs ...string) (*Location, error) {
	dir, e := ResolveDirectory(override, mode)
	if e != nil {
		return nil, e
	}
	return OpenLocationAt(dir, override, assets, rebuild, outputs...)
}
func OpenLocationAt(dir, override, assets string, rebuild bool, outputs ...string) (*Location, error) {
	var data string
	var e error
	explicit := os.Getenv("GOSHIP2000_DATA_DIR") != "" || override != ""
	pinned := false
	if explicit {
		data, e = DataChoice(dir, override)
	} else {
		data, pinned, e = locatedData()
		if locator, err := locatorPath(); err == nil {
			outputs = append(outputs, locator)
		}
	}
	if e != nil && (explicit || pinned) {
		return nil, e
	}
	if e == nil {
		data, e = ResolvedNewPath(data)
		if e == nil {
			var loc *Location
			loc, e = openData(dir, data, assets, rebuild, pinned, !explicit, outputs...)
			if loc != nil {
				return loc, nil
			}
			var unavailable *unusableRoot
			if explicit || pinned || data == dir || !errors.As(e, &unavailable) {
				return nil, e
			}
		}
	}
	if explicit || pinned {
		return nil, e
	}
	loc, fallback := openData(dir, dir, assets, rebuild, false, !explicit, outputs...)
	if fallback != nil {
		return nil, fmt.Errorf("preferred user data unavailable (%v); installation fallback: %w", e, fallback)
	}
	fmt.Fprintf(os.Stderr, "User data fallback: %s (%v)\n", dir, e)
	return loc, nil
}

type unusableRoot struct{ error }

func openData(dir, data, assets string, rebuild, pinned, checkLocator bool, outputs ...string) (*Location, error) {
	// Inspect usability of the directory separately from the integrity of its contents.
	if st, e := os.Stat(data); e == nil {
		if !st.IsDir() {
			return nil, &unusableRoot{fmt.Errorf("%s is not a directory", data)}
		}
		f, e := os.Open(data)
		if e != nil {
			return nil, &unusableRoot{e}
		}
		f.Close()
	} else if !os.IsNotExist(e) {
		return nil, &unusableRoot{e}
	}
	owner, e := beginOwnershipAt(dir, data, assets, rebuild, outputs...)
	if e != nil {
		return nil, e
	}
	fail := func(e error) (*Location, error) { owner.Close(); return nil, e }
	if checkLocator {
		current, selected, err := locatedData()
		if selected != pinned || selected && (err != nil || current != data) {
			return fail(fmt.Errorf("saves location changed during startup; please reopen"))
		}
	}
	if _, err := os.Lstat(filepath.Join(data, switchRecord)); err == nil {
		if !pinned {
			return fail(fmt.Errorf("unfinished SETUP transfer; reopen the source and save the same location to finish"))
		}
		// The locator is published only after every destination file is verified.
		if e = os.Remove(filepath.Join(data, switchRecord)); e != nil {
			return fail(e)
		}
	} else if !os.IsNotExist(err) {
		return fail(err)
	}
	if pinned {
		if _, e = os.Stat(filepath.Join(data, Filename)); e != nil {
			return fail(fmt.Errorf("selected Saves Location unavailable: %w", e))
		}
	}
	if e = os.MkdirAll(data, 0700); e != nil {
		return fail(&unusableRoot{e})
	}
	if e = probeWritable(data); e != nil {
		return fail(&unusableRoot{e})
	}
	if data != dir && !pinned {
		if e = moveUserStore(dir, data); e != nil {
			return fail(e)
		}
	}
	return &Location{owner, dir, data}, nil
}

// ExistingConfigPath resolves SETUP -check without creating a store.
func ExistingConfigPath(dir, override string) (string, error) {
	if override != "" || os.Getenv("GOSHIP2000_DATA_DIR") != "" {
		data, e := DataChoice(dir, override)
		return filepath.Join(data, Filename), e
	}
	data, pinned, e := locatedData()
	if pinned {
		return filepath.Join(data, Filename), e
	}
	if e == nil {
		path := filepath.Join(data, Filename)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			return path, err
		}
	}
	return filepath.Join(dir, Filename), nil
}

func probeWritable(dir string) error {
	f, e := os.CreateTemp(dir, ".write-check-")
	if e != nil {
		return e
	}
	closeErr := f.Close()
	return errors.Join(closeErr, os.Remove(f.Name()))
}
