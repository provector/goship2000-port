package controllers

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"gunship2000-assets/internal/userpack"
	"io"
	"os"
	"path/filepath"
	"sync"
)

var looseWrites sync.Mutex

const maxBytes = 1 << 20

type Document struct {
	Path   string
	Config Config
	raw    []byte
	exists bool
	hash   [32]byte
}
type Checkpoint struct {
	raw    []byte
	exists bool
	config Config
}

func Load(path string) (*Document, error) {
	d := &Document{Path: path, Config: Defaults()}
	if filepath.Base(path) == userpack.ConfigName {
		b, e := userpack.ReadConfig(filepath.Dir(path), "controls.json")
		if e != nil {
			return nil, e
		}
		return decodeControls(d, b)
	}
	i, e := os.Lstat(path)
	if os.IsNotExist(e) {
		return d, nil
	}
	if e != nil {
		return nil, fmt.Errorf("%s: %w", path, e)
	}
	if !i.Mode().IsRegular() || i.Size() > maxBytes {
		return nil, fmt.Errorf("%s: ordinary controls file up to1MiB required", path)
	}
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, fmt.Errorf("%s: %w", path, e)
	}
	return decodeControls(d, b)
}
func decodeControls(d *Document, b []byte) (*Document, error) {
	path := d.Path
	var e error
	if len(b) > maxBytes {
		return nil, fmt.Errorf("oversized controls entry")
	}
	if e := uniqueFields(b); e != nil {
		return nil, fmt.Errorf("%s: %w", path, e)
	}
	var object map[string]json.RawMessage
	if e = json.Unmarshal(b, &object); e != nil || object == nil {
		return nil, fmt.Errorf("%s: controls JSON object required: %v", path, e)
	}
	if _, ok := object["version"]; !ok {
		return nil, fmt.Errorf("%s: version field required", path)
	}
	var version int
	if e = json.Unmarshal(object["version"], &version); e != nil || (version < 1 || version > Version) {
		return nil, fmt.Errorf("%s: unsupported controls version %s (supported1..4); preserved", path, object["version"])
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if e = dec.Decode(&d.Config); e != nil {
		return nil, fmt.Errorf("%s: controls JSON at byte%d: %w", path, dec.InputOffset(), e)
	}
	if e = dec.Decode(new(any)); e != io.EOF {
		return nil, fmt.Errorf("%s: trailing controls JSON", path)
	}
	if e = d.Config.Validate(); e != nil {
		return nil, fmt.Errorf("%s: %w (file preserved)", path, e)
	}
	d.raw = b
	d.exists = true
	d.hash = sha256.Sum256(b)
	return d, nil
}
func (d *Document) matches() error {
	current, e := Load(d.Path)
	if e != nil {
		return e
	}
	if current.exists != d.exists || current.hash != d.hash {
		return fmt.Errorf("%s changed externally; reopen SETUP", d.Path)
	}
	return nil
}
func (d *Document) Checkpoint() Checkpoint {
	c := Defaults()
	if len(d.raw) > 0 {
		json.Unmarshal(d.raw, &c)
	}
	return Checkpoint{append([]byte(nil), d.raw...), d.exists, c}
}
func (d *Document) Save() error {
	if filepath.Base(d.Path) == userpack.ConfigName {
		c, e := d.PackedChange()
		if e != nil {
			return e
		}
		e = userpack.Locked(d.Path, func() error { return userpack.PatchLocked(d.Path, []userpack.Change{c}) })
		if e == nil {
			d.PackedSaved(c.After)
		}
		return e
	}
	if e := d.Config.Validate(); e != nil {
		return fmt.Errorf("%s: %w", d.Path, e)
	}
	if e := d.matches(); e != nil {
		return e
	}
	if i, e := os.Stat(d.Path); e == nil && i.Mode().Perm()&0222 == 0 {
		return fmt.Errorf("%s: read-only", d.Path)
	}
	for id, p := range d.Config.Profiles {
		p.Device.Instance = 0
		p.Device.Path = ""
		d.Config.Profiles[id] = p
	}
	b, e := json.MarshalIndent(d.Config, "", "  ")
	if e != nil {
		return e
	}
	b = append(b, '\n')
	if len(b) > maxBytes {
		return fmt.Errorf("%s: saved controls exceed1MiB", d.Path)
	}
	if d.exists && bytes.Equal(b, d.raw) {
		return nil
	}
	looseWrites.Lock()
	defer looseWrites.Unlock()
	if e = d.replace(b); e != nil {
		return e
	}
	d.raw = b
	d.hash = sha256.Sum256(b)
	d.exists = true
	return nil
}
func (d *Document) replace(b []byte) error {
	f, e := os.CreateTemp(filepath.Dir(d.Path), ".controls.cfg-saving-")
	if e != nil {
		return fmt.Errorf("%s: stage: %w", d.Path, e)
	}
	name := f.Name()
	defer os.Remove(name)
	mode := os.FileMode(0600)
	if i, e := os.Stat(d.Path); e == nil {
		mode = i.Mode().Perm()
	}
	if e = f.Chmod(mode); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return fmt.Errorf("%s: write: %w", d.Path, e)
	}
	if e = d.matches(); e != nil {
		return e
	}
	if e = os.Rename(name, d.Path); e != nil {
		return fmt.Errorf("%s: atomic replacement: %w", d.Path, e)
	}
	if dir, e := os.Open(filepath.Dir(d.Path)); e == nil {
		dir.Sync()
		dir.Close()
	}
	return nil
}

// Restore only this Save's unchanged output if the companion general-config
// commit fails. Never overwrite another writer's later edit.
func (d *Document) Rollback(c Checkpoint) error {
	if filepath.Base(d.Path) == userpack.ConfigName {
		return fmt.Errorf("packed settings and controls are saved together")
	}
	looseWrites.Lock()
	defer looseWrites.Unlock()
	if e := d.matches(); e != nil {
		return e
	}
	if !c.exists {
		if e := os.Remove(d.Path); e != nil && !errors.Is(e, os.ErrNotExist) {
			return e
		}
	} else if e := d.replace(c.raw); e != nil {
		return e
	}
	d.raw = c.raw
	d.exists = c.exists
	d.Config = c.config
	d.hash = [32]byte{}
	if c.exists {
		d.hash = sha256.Sum256(c.raw)
	}
	return nil
}

// JSON object IDs must be unique even in nested binding/profile objects.
func uniqueFields(b []byte) error {
	d := json.NewDecoder(bytes.NewReader(b))
	var value func() error
	value = func() error {
		t, e := d.Token()
		if e != nil {
			return e
		}
		v, ok := t.(json.Delim)
		if !ok {
			if t == nil {
				return fmt.Errorf("null controls value; omit an unbound optional field instead")
			}
			return nil
		}
		switch v {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				t, e := d.Token()
				if e != nil {
					return e
				}
				k, ok := t.(string)
				if !ok {
					return fmt.Errorf("invalid object key")
				}
				if seen[k] {
					return fmt.Errorf("duplicate field %q", k)
				}
				seen[k] = true
				if e = value(); e != nil {
					return e
				}
			}
			_, e = d.Token()
			return e
		case '[':
			for d.More() {
				if e = value(); e != nil {
					return e
				}
			}
			_, e = d.Token()
			return e
		}
		return fmt.Errorf("unexpected delimiter")
	}
	if e := value(); e != nil {
		return e
	}
	if _, e := d.Token(); e != io.EOF {
		return fmt.Errorf("trailing controls JSON")
	}
	return nil
}

func DocumentPath(directory string) string {
	p := filepath.Join(directory, userpack.ConfigName)
	if _, e := os.Lstat(p); !os.IsNotExist(e) {
		return p
	}
	return filepath.Join(directory, Filename)
}
func (d *Document) PackedChange() (userpack.Change, error) {
	if e := d.Config.Validate(); e != nil {
		return userpack.Change{}, e
	}
	for id, p := range d.Config.Profiles {
		p.Device.Instance = 0
		p.Device.Path = ""
		d.Config.Profiles[id] = p
	}
	b, e := json.MarshalIndent(d.Config, "", "  ")
	if e != nil {
		return userpack.Change{}, e
	}
	b = append(b, '\n')
	if len(b) > maxBytes {
		return userpack.Change{}, fmt.Errorf("controls exceed 1 MiB")
	}
	return userpack.Change{Name: "controls.json", Before: d.raw, After: b}, nil
}
func (d *Document) PackedSaved(b []byte) { d.raw = b; d.exists = true; d.hash = sha256.Sum256(b) }
