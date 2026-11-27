package configuration

import (
	"errors"
	"fmt"
	"gunship2000-assets/internal/filelease"
	"gunship2000-assets/internal/playerdirs"
	"gunship2000-assets/internal/storeowner"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// BeginOwnership reads only enough configuration to identify every writable
// tree. Recheck after acquisition before allowing migration or initialization.
func BeginOwnership(override, mode, assets string, rebuild bool, outputs ...string) (*os.File, error) {
	dir, e := ResolveDirectory(override, mode)
	if e != nil {
		return nil, e
	}
	return beginOwnershipAt(dir, dir, assets, rebuild, outputs...)
}
func beginOwnershipAt(dir, data, assets string, rebuild bool, outputs ...string) (*os.File, error) {
	inspect := func() ([]storeowner.Tree, Config, error) {
		c := DefaultConfig()
	loadConfig:
		for _, root := range []string{data, dir} {
			for _, n := range []string{Filename, "config.cfg"} {
				path := filepath.Join(root, n)
				if _, e := os.Lstat(path); os.IsNotExist(e) {
					continue
				} else if e != nil {
					return nil, c, e
				}
				doc, e := LoadAt(path, dir)
				if e != nil {
					return nil, c, e
				}
				c = doc.Config
				break loadConfig
			}
		}
		choice := c.Assets
		if assets != "" {
			choice = assets
		}
		roots := []storeowner.Tree{{Path: dir, Write: true}, {Path: SavePath(dir, data, c.Saves), Write: true}, {Path: Relative(dir, choice), Write: rebuild || assets == "" && filepath.Clean(c.Assets) == "assets"}}
		roots = append(roots, storeowner.Tree{Path: data, Write: true})
		for _, p := range outputs {
			if p != "" {
				roots = append(roots, storeowner.Tree{Path: p, Write: true})
			}
		}
		for i, r := range roots {
			real, e := ResolvedNewPath(r.Path)
			if e != nil {
				return nil, c, e
			}
			roots[i].Path = real
		}
		original, e := ResolvedNewPath(Relative(dir, c.Original))
		if e != nil {
			return nil, c, e
		}
		if data != dir {
			for _, protected := range []string{original, roots[2].Path} {
				if Within(data, protected) || Within(protected, data) && !Within(protected, dir) {
					return nil, c, fmt.Errorf("user data overlaps original or prepared assets")
				}
			}
			// The application may live under Documents or directly in it. Keep
			// both roots out of any subtree the transfer could copy or retire.
			if Within(data, dir) || Within(dir, data) {
				for _, pair := range [][2]string{{dir, data}, {data, dir}} {
					for _, name := range append(slices.Clone(legacyItems), c.Saves, "saves") {
						if !filepath.IsLocal(name) {
							continue
						}
						tree, err := ResolvedNewPath(filepath.Join(pair[0], name))
						if err != nil {
							return nil, c, err
						}
						if Within(pair[1], tree) {
							return nil, c, fmt.Errorf("installation and personal data overlap managed content in %s", tree)
						}
					}
				}
			}
		}
		if Within(roots[3].Path, original) || Within(roots[1].Path, original) || Within(original, roots[1].Path) || roots[2].Write && (Within(roots[2].Path, original) || Within(original, roots[2].Path)) {
			return nil, c, fmt.Errorf("writable store overlaps original installation")
		}
		if err := validateSiblings(roots[1].Path, original, roots[2].Path); err != nil {
			return nil, c, err
		}
		for _, name := range []string{"reports", "imports", "temp", "screenshots"} {
			path := playerdirs.Sibling(roots[1].Path, name)
			if !Within(path, data) && !Within(path, dir) {
				roots = append(roots, storeowner.Tree{Path: path, Write: true})
			}
		}
		return roots, c, nil
	}
	roots, _, e := inspect()
	if e != nil {
		return nil, e
	}
	f, e := storeowner.Acquire(roots...)
	if e != nil {
		return nil, e
	}
	current, c, e := inspect()
	if e == nil && !slices.Equal(current, roots) {
		e = fmt.Errorf("storage paths changed during startup; reopen the application")
	}
	if e == nil {
		saves := roots[1].Path
		writers := []string{
			filepath.Join(dir, Filename+".save.lock"), filepath.Join(dir, "config.cfg.save.lock"),
			filepath.Join(dir, "controls.cfg.save.lock"), filepath.Join(dir, "assets.prepare.lock"),
			filepath.Join(saves, ".save.lock"), filepath.Join(dir, "films", ".film-publish.lock"),
			filepath.Join(dir, "missions", ".authored-store.lock"), filepath.Join(saves, "replays", ".film-publish.lock"),
			filepath.Join(saves, "missions", ".authored-store.lock"), LogPath(dir, c.Logging) + ".lock",
		}
		if roots[2].Write && roots[2].Path != filepath.Join(dir, "assets") {
			writers = append(writers, roots[2].Path+".prepare.lock")
		}
		anchors := []string{filepath.Join(saves, ".career-owner.lease")}
		reportRoots := []string{filepath.Join(saves, "reports")}
		if playerdirs.OwnedArchive(saves, "reports") {
			reportRoots = append(reportRoots, playerdirs.Sibling(saves, "reports"))
		}
		for _, reports := range reportRoots {
			if dirs, err := os.ReadDir(reports); err == nil {
				for _, d := range dirs {
					if d.IsDir() && strings.HasPrefix(d.Name(), "session-") {
						anchor := filepath.Join(reports, d.Name(), "owner.lease")
						if err := filelease.Retire(nil, []string{anchor}); err != nil {
							if errors.Is(err, os.ErrPermission) && !strings.Contains(err.Error(), "already in use") {
								fmt.Fprintf(os.Stderr, "Leaving inaccessible report session at %s: %v\n", anchor, err)
							} else {
								return nil, errors.Join(err, f.Close())
							}
						}
					}
				}
			}
		}
		e = filelease.Retire(writers, anchors)
	}
	if e != nil {
		f.Close()
		return nil, e
	}
	return f, nil
}

func OwnOutput(path string) (*os.File, error) {
	real, e := ResolvedNewPath(path)
	if e != nil {
		return nil, e
	}
	return storeowner.Acquire(storeowner.Tree{Path: real, Write: true})
}
