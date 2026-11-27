package configuration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gunship2000-assets/internal/playerdirs"
	"gunship2000-assets/internal/userpack"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// All paths are relative to the two exclusively owned roots. Only these
// named legacy authorities and their descendants participate in a transfer.
var legacyItems = []string{saveOriginName, "missions", "films", "SUMMARY.TXT", ".t3-library-backup", ".t3-config-backup", ".t3-config-pending", ".t3-config-migrated.json", "controls.cfg", "gps_codes.json", "config.cfg", Filename}

const transferRecord = ".v45-location-transfer.json"

const saveOriginName = ".v45-external-career.json"

type saveOrigin struct {
	Version            int
	Relative, Absolute string
}
type transfer struct {
	Version  int
	Source   string
	Files    map[string]string
	Dirs     []string
	External *saveOrigin `json:",omitempty"`
}

func localPath(root, name string) (string, error) {
	if !filepath.IsLocal(name) || name == "." || filepath.Clean(name) != name {
		return "", fmt.Errorf("invalid transfer member %q", name)
	}
	path := filepath.Join(root, name)
	real, e := ResolvedNewPath(path)
	if e != nil {
		return "", e
	}
	if rel, err := filepath.Rel(path, real); err != nil || rel != "." {
		return "", fmt.Errorf("transfer path contains a link: %s", path)
	}
	return path, nil
}
func configuredItems(src, install string) ([]string, string, error) {
	items := slices.Clone(legacyItems)
	c := DefaultConfig()
	configured := false
	for _, n := range []string{Filename, "config.cfg"} {
		if _, e := os.Lstat(filepath.Join(src, n)); os.IsNotExist(e) {
			continue
		}
		d, e := LoadAt(filepath.Join(src, n), install)
		if e != nil {
			return nil, "", e
		}
		c = d.Config
		configured = true
		name := filepath.Clean(d.Config.Saves)
		if filepath.IsLocal(name) {
			if name == "." {
				return nil, "", fmt.Errorf("installation root cannot be migrated as a saves tree")
			}
			items = append(items, name)
			for _, side := range []string{"imports", "reports"} {
				if playerdirs.OwnedArchive(filepath.Join(src, name), side) {
					items = append(items, filepath.Join(filepath.Dir(name), side))
				}
			}
		}
		break
	}
	if !configured {
		items = append(items, "saves")
	}
	// Reject a nonstandard configuration that places game material in one
	// of the proposed transfer trees. Never copy or retire that tree.
	for _, name := range items {
		path, e := ResolvedNewPath(filepath.Join(src, name))
		if e != nil {
			return nil, "", e
		}
		for _, asset := range []string{c.Original, c.Assets} {
			keep, e := ResolvedNewPath(Relative(install, asset))
			if e != nil {
				return nil, "", e
			}
			if Within(path, keep) || Within(keep, path) {
				return nil, "", fmt.Errorf("data migration overlaps original/prepared files: %s", path)
			}
		}
	}
	excluded := ""
	if filepath.IsLocal(c.Saves) {
		excluded = filepath.Join(c.Saves, "screenshots")
	}
	return items, excluded, nil
}
func moveUserStore(src, dst string) error {
	return transferUserStore(src, dst, src, false)
}
func transferUserStore(src, dst, install string, keepSource bool) error {
	marker := filepath.Join(dst, transferRecord)
	if keepSource {
		marker = filepath.Join(dst, switchRecord)
	}
	var tr transfer
	raw, e := os.ReadFile(marker)
	if os.IsNotExist(e) {
		items, _, e := configuredItems(dst, install)
		if e != nil {
			return e
		}
		for _, n := range items {
			if st, err := os.Lstat(filepath.Join(dst, n)); err == nil {
				if st.IsDir() {
					entries, err := os.ReadDir(filepath.Join(dst, n))
					if err != nil {
						return err
					}
					if len(entries) == 0 {
						continue
					}
				}
				return nil
			} else if !os.IsNotExist(err) {
				return err
			}
		}
		items, excluded, e := configuredItems(src, install)
		if e != nil {
			return e
		}
		tr = transfer{Version: 1, Source: src, Files: map[string]string{}}
		skipScreens := filepath.Join(src, excluded)
		for _, n := range []string{Filename, "config.cfg"} {
			if _, e := os.Lstat(filepath.Join(src, n)); os.IsNotExist(e) {
				continue
			}
			d, e := LoadAt(filepath.Join(src, n), install)
			if e != nil {
				return e
			}
			if name := d.Config.Saves; !filepath.IsAbs(name) && !filepath.IsLocal(name) {
				tr.External = &saveOrigin{Version: 1, Relative: name, Absolute: Relative(install, name)}
			}
			break
		}
		for _, item := range items {
			from, e := localPath(src, item)
			if e != nil {
				return e
			}
			if _, e = os.Lstat(from); os.IsNotExist(e) {
				continue
			} else if e != nil {
				return e
			}
			if e = filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() && excluded != "" && path == skipScreens {
					return filepath.SkipDir
				}
				rel, e := filepath.Rel(src, path)
				if e != nil {
					return e
				}
				if d.IsDir() {
					tr.Dirs = append(tr.Dirs, rel)
					return nil
				}
				if !d.Type().IsRegular() {
					return fmt.Errorf("cannot migrate special file %s", path)
				}
				h, e := userpack.HashFile(path)
				if e == nil {
					tr.Files[rel] = h
				}
				return e
			}); e != nil {
				return e
			}
		}
		if len(tr.Files) == 0 && len(tr.Dirs) == 0 {
			return nil
		}
		raw, e = json.Marshal(tr)
		if e != nil {
			return e
		}
		if e = safeRecord(marker, raw); e != nil {
			return e
		}
	} else if e != nil {
		return e
	} else if e = json.Unmarshal(raw, &tr); e != nil {
		return e
	}
	if tr.Version != 1 || tr.Source != src {
		return fmt.Errorf("unfinished user-data transfer from %s; reopen that installation to finish", tr.Source)
	}
	// A SETUP copy starts in an empty directory and owns this one scratch
	// path under its durable intent. A killed streaming copy can be retried
	// without accepting unknown files or discarding the authoritative source.
	scratch := ""
	if keepSource {
		scratch = filepath.Join(dst, switchScratch)
		if _, ok := tr.Files[switchScratch]; ok || slices.Contains(tr.Dirs, switchScratch) {
			return fmt.Errorf("transfer scratch conflicts with source data")
		}
		if st, err := os.Lstat(scratch); err == nil {
			if !st.Mode().IsRegular() {
				return fmt.Errorf("unexpected transfer scratch at %s", scratch)
			}
			if err = os.Remove(scratch); err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	// Re-derive authority from configuration, including after retiring its old copy.
	configRoot := src
	for _, n := range []string{Filename, "config.cfg"} {
		if _, ok := tr.Files[n]; ok {
			if _, e := os.Stat(filepath.Join(src, n)); os.IsNotExist(e) {
				configRoot = dst
			}
			break
		}
	}
	items, excluded, e := configuredItems(configRoot, install)
	if e != nil {
		return e
	}
	// A resumed installation transfer can already have retired the source
	// ownership marker. Its verified destination copy proves the same scope.
	dstItems, _, err := configuredItems(dst, install)
	if err != nil {
		return err
	}
	items = append(items, dstItems...)
	// Apply the same transfer scope to an unfinished inventory, including
	// inventories made before the screenshot output directory changed.
	if excluded != "" {
		exclude := func(name string) bool { return Within(name, excluded) }
		tr.Dirs = slices.DeleteFunc(tr.Dirs, exclude)
		for name := range tr.Files {
			if exclude(name) {
				delete(tr.Files, name)
			}
		}
	}
	member := func(n string) bool {
		for _, v := range items {
			if n == v || strings.HasPrefix(n, v+string(filepath.Separator)) {
				return true
			}
		}
		return false
	}
	for _, n := range tr.Dirs {
		if !member(n) {
			return fmt.Errorf("unrecognized transfer directory %s", n)
		}
		target, e := localPath(dst, n)
		if e != nil {
			return e
		}
		if e = os.MkdirAll(target, 0700); e != nil {
			return e
		}
	}
	names := make([]string, 0, len(tr.Files))
	for n := range tr.Files {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		want := tr.Files[n]
		if !member(n) {
			return fmt.Errorf("unrecognized transfer file %s", n)
		}
		old, e := localPath(src, n)
		if e != nil {
			return e
		}
		target, e := localPath(dst, n)
		if e != nil {
			return e
		}
		if h, e := userpack.HashFile(target); e == nil {
			if h != want {
				return fmt.Errorf("different file at migration destination %s", target)
			}
			continue
		} else if !os.IsNotExist(e) {
			return e
		}
		if e = copyVerifiedUsing(old, target, want, scratch); e != nil {
			return e
		}
	}
	if tr.External != nil {
		if tr.External.Absolute != Relative(install, tr.External.Relative) || filepath.IsLocal(tr.External.Relative) || filepath.IsAbs(tr.External.Relative) {
			return fmt.Errorf("invalid external career location")
		}
		b, e := json.Marshal(tr.External)
		if e != nil {
			return e
		}
		if e = safeRecord(filepath.Join(dst, saveOriginName), b); e != nil {
			return e
		}
	}
	// All destination members are now flushed and verified. Repeated opens after
	// interruption recheck every destination before retiring any remaining duplicate.
	if e = userpack.SyncDir(dst); e != nil {
		return e
	}
	if keepSource {
		return verifySwitchCopy(src, dst, install, items, tr)
	}
	for _, n := range names {
		old, e := localPath(src, n)
		if e != nil {
			return e
		}
		h, e := userpack.HashFile(old)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		if h != tr.Files[n] {
			return fmt.Errorf("source changed during transfer: %s", old)
		}
		if e = os.Remove(old); e != nil && !os.IsPermission(e) {
			return e
		}
	}
	for i := len(tr.Dirs) - 1; i >= 0; i-- {
		old, e := localPath(src, tr.Dirs[i])
		if e != nil {
			return e
		}
		os.Remove(old)
	} // Only empty directories can be retired.
	if e = os.Remove(marker); e != nil {
		return e
	}
	return userpack.SyncDir(dst)
}
func copyVerified(src, dst, want string) error { return copyVerifiedUsing(src, dst, want, "") }
func copyVerifiedUsing(src, dst, want, scratch string) error {
	f, e := os.Open(src)
	if e != nil {
		return e
	}
	defer f.Close()
	if e = os.MkdirAll(filepath.Dir(dst), 0700); e != nil {
		return e
	}
	var out *os.File
	if scratch == "" {
		out, e = os.CreateTemp(filepath.Dir(dst), ".v45-copy-")
	} else {
		out, e = os.OpenFile(scratch, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	}
	if e != nil {
		return e
	}
	defer os.Remove(out.Name())
	h := sha256.New()
	_, e = io.Copy(io.MultiWriter(out, h), f)
	if e == nil && hex.EncodeToString(h.Sum(nil)) != want {
		e = fmt.Errorf("source changed during copy: %s", src)
	}
	if e == nil {
		e = out.Sync()
	}
	ce := out.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	if h, e := userpack.HashFile(out.Name()); e != nil || h != want {
		return fmt.Errorf("cannot verify copied %s: %v", src, e)
	}
	return userpack.Replace(out.Name(), dst)
}
