package configuration

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"gunship2000-assets/internal/userpack"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
)

var looseWrites sync.Mutex

const MaxConfigBytes = 1 << 20

var ErrChanged = errors.New("configuration changed externally; reopen SETUP before saving")

type Document struct {
	Path         string
	installation string
	Config       Config
	raw          map[string]json.RawMessage
	fingerprint  [32]byte
}
type MissingError struct{ Path string }

func (e *MissingError) Error() string {
	return "Configuration file not found. Please run the setup utility.\nExpected path: " + e.Path
}
func (e *MissingError) Unwrap() error { return os.ErrNotExist }
func field(name, msg string) error    { return fmt.Errorf("%s: %s", name, msg) }
func (c Config) Validate() error {
	if e := c.Logging.Validate(); e != nil {
		return e
	}
	d := c.Development
	if math.IsNaN(d.TickRate) || math.IsInf(d.TickRate, 0) || d.TickRate <= 0 || d.TickRate > 1000 {
		return field("development.tick_rate", "must be >0 and <=1000")
	}
	if math.IsNaN(d.PreviewRate) || math.IsInf(d.PreviewRate, 0) || d.PreviewRate <= 0 || d.PreviewRate > 1000 {
		return field("development.preview_rate", "must be >0 and <=1000")
	}
	if d.Arrangement < 0 || d.Arrangement > 3 {
		return field("development.arrangement", "must be0..3")
	}
	if d.CockpitAircraft < -1 || d.CockpitAircraft > 7 {
		return field("development.cockpit_aircraft", "must be -1..7")
	}
	if d.PointerX < 0 || d.PointerX > 319 || d.PointerY < 0 || d.PointerY > 199 {
		return field("development.pointer", "coordinates must be0..319 by0..199")
	}
	if c.FormatVersion != 1 {
		return field("format_version", "supported value is 1")
	}
	for k, v := range map[string]string{"original": c.Original, "assets": c.Assets, "saves": c.Saves} {
		if strings.TrimSpace(v) == "" || strings.ContainsRune(v, 0) {
			return field(k, "must be a nonempty path")
		}
	}
	if c.WindowWidth < 320 || c.WindowWidth > 7680 {
		return field("window_width", "must be 320..7680")
	}
	if c.WindowHeight < 200 || c.WindowHeight > 4320 {
		return field("window_height", "must be 200..4320")
	}
	if c.FadeMilliseconds < 0 || c.FadeMilliseconds > 2000 {
		return field("fade_milliseconds", "must be 0..2000")
	}
	for k, v := range map[string]int{"music_volume": c.MusicVolume, "sound_effects_volume": c.SoundEffectsVolume, "last_music_volume": c.LastMusicVolume, "last_effects_volume": c.LastEffectsVolume} {
		if v < 0 || v > 100 {
			return field(k, "must be an integer in 0..100")
		}
	}
	if c.Audio.MusicMode != MusicAll && c.Audio.MusicMode != MusicBackgroundOff && c.Audio.MusicMode != MusicOff {
		return field("audio.music_mode", "must be all, background_off or off")
	}
	if _, e := ParseRecordingLimit(fmt.Sprint(c.ReplayMaxMiB)); e != nil {
		return field("replay_max_mib", e.Error())
	}
	if e := c.Runtime.Validate(); e != nil {
		return field("runtime", e.Error())
	}
	for id, t := range c.UI {
		if len(t.Label) > 256 || len(t.Tooltip) > 4096 {
			return field("ui."+id, "label/tooltip exceeds 256/4096 bytes")
		}
		for key, q := range t.Choices {
			if len(q.Label) > 256 || len(q.Tooltip) > 4096 {
				return field("ui."+id+".choices."+key, "label/tooltip too long")
			}
		}
	}
	return nil
}
func object(data []byte) (map[string]json.RawMessage, error) {
	var v map[string]json.RawMessage
	d := json.NewDecoder(bytes.NewReader(data))
	if e := d.Decode(&v); e != nil {
		return nil, e
	}
	if v == nil {
		return nil, errors.New("expected JSON object")
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		return nil, errors.New("unexpected trailing JSON")
	}
	return v, nil
}
func parseError(path string, b []byte, e error) error {
	var s *json.SyntaxError
	var t *json.UnmarshalTypeError
	offset := int64(0)
	if errors.As(e, &s) {
		offset = s.Offset
	}
	if errors.As(e, &t) {
		offset = t.Offset
	}
	if offset > 0 && offset <= int64(len(b)) {
		line := bytes.Count(b[:offset-1], []byte{'\n'}) + 1
		last := bytes.LastIndexByte(b[:offset-1], '\n')
		return fmt.Errorf("%s: line %d column %d: %w", path, line, int(offset)-last-1, e)
	}
	return fmt.Errorf("%s: %w", path, e)
}
func (d *Document) InstallDirectory() string {
	if d.installation != "" {
		return d.installation
	}
	return filepath.Dir(d.Path)
}
func Load(path string) (*Document, error) { return LoadAt(path, filepath.Dir(path)) }
func LoadAt(path, install string) (*Document, error) {
	if filepath.Base(path) == userpack.ConfigName {
		b, e := userpack.ReadConfig(filepath.Dir(path), "settings.json")
		if e != nil {
			return nil, e
		}
		return decodeSettingsAt(path, install, b)
	}
	info, e := os.Lstat(path)
	if os.IsNotExist(e) {
		return nil, &MissingError{path}
	}
	if e != nil {
		return nil, fmt.Errorf("%s: %w", path, e)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s: expected ordinary configuration file, not symlink/device", path)
	}
	if info.Size() > MaxConfigBytes {
		return nil, fmt.Errorf("%s: exceeds 1 MiB", path)
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	b, e := io.ReadAll(io.LimitReader(f, MaxConfigBytes+1))
	f.Close()
	if e != nil {
		return nil, e
	}
	if len(b) > MaxConfigBytes {
		return nil, fmt.Errorf("%s: exceeds 1 MiB", path)
	}
	return decodeSettingsAt(path, install, b)
}
func decodeSettings(path string, b []byte) (*Document, error) {
	return decodeSettingsAt(path, filepath.Dir(path), b)
}
func decodeSettingsAt(path, install string, b []byte) (*Document, error) {
	if len(b) > MaxConfigBytes {
		return nil, fmt.Errorf("oversized settings entry")
	}
	raw, e := object(b)
	if e != nil {
		return nil, parseError(path, b, e)
	}
	if e := uniqueKeys(b); e != nil {
		return nil, fmt.Errorf("%s: %w", path, e)
	}
	if e := rejectKnownNull(raw, reflect.TypeOf(Config{}), "config"); e != nil {
		return nil, fmt.Errorf("%s: %w", path, e)
	}
	c := DefaultConfig()
	// U2 encoded a saved Off by omitting the runtime key. Fresh packs have
	// an explicit Modern value; absence in a pre-existing document means Off.
	c.Runtime.FloatingGeometry = false
	filtered := make(map[string]json.RawMessage)
	for k, v := range raw {
		filtered[k] = v
	}
	// Preserve unrelated future runtime fields without treating them as consumers.
	// Historical RuntimeOptions.UnmarshalJSON remains strict for film readers.
	if r, ok := raw["runtime"]; ok {
		m, e := object(r)
		if e != nil {
			return nil, fmt.Errorf("%s: runtime: %w", path, e)
		}
		// A saved pre-U1 scale takes precedence over the fresh-install default.
		if _, explicit := m["simulation_resolution"]; !explicit && m["simulation_render_scale"] != nil {
			c.Runtime.SimulationResolution = ""
		}
		allowed := map[string]json.RawMessage{}
		ty := reflect.TypeOf(RuntimeOptions{})
		for i := 0; i < ty.NumField(); i++ {
			k, _, _ := strings.Cut(ty.Field(i).Tag.Get("json"), ",")
			if v, ok := m[k]; ok {
				allowed[k] = v
			}
		}
		filtered["runtime"], _ = json.Marshal(allowed)
	}
	// Merge known text defaults before typed decoding so hand-edited partial text
	// keeps its supplied fields; unknown fields survive in raw through Save.
	if u, ok := raw["ui"]; ok {
		var old map[string]json.RawMessage
		if e = json.Unmarshal(u, &old); e != nil {
			return nil, fmt.Errorf("%s: ui: %w", path, e)
		}
		for id, t := range c.UI {
			if v, ok := old[id]; ok {
				tb, _ := json.Marshal(t)
				var base map[string]json.RawMessage
				json.Unmarshal(tb, &base)
				var custom map[string]json.RawMessage
				if e = json.Unmarshal(v, &custom); e != nil {
					return nil, fmt.Errorf("%s: ui.%s: %w", path, id, e)
				}
				for k, v := range custom {
					base[k] = v
				}
				old[id], _ = json.Marshal(base)
			}
		}
		filtered["ui"], _ = json.Marshal(old)
	}
	clean, _ := json.Marshal(filtered)
	if e = json.Unmarshal(clean, &c); e != nil {
		return nil, fmt.Errorf("%s: field/type: %w", path, e)
	}
	if !filepath.IsAbs(c.Saves) && !filepath.IsLocal(c.Saves) {
		var anchor saveOrigin
		b, err := os.ReadFile(filepath.Join(filepath.Dir(path), saveOriginName))
		if err == nil {
			if json.Unmarshal(b, &anchor) != nil || anchor.Version != 1 || !filepath.IsAbs(anchor.Absolute) {
				return nil, fmt.Errorf("invalid external career location beside %s", path)
			}
			if c.Saves == anchor.Relative {
				c.Saves = anchor.Absolute
			}
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	// Older SETUP versions serialized the stock size even though they had no
	// window-size picker. Preserve every other previously valid custom size.
	if c.WindowWidth == 960 && c.WindowHeight == 720 {
		c.WindowWidth, c.WindowHeight = DefaultWindowWidth, DefaultWindowHeight
	}
	// Old source launches defaulted to the parent directory. A missing path
	// can adopt the local fallback without changing any working selection.
	if filepath.Clean(c.Original) == filepath.Join("..", DefaultOriginal) {
		if _, err := os.Stat(Relative(install, c.Original)); os.IsNotExist(err) {
			c.Original = DefaultOriginal
		}
	}
	switch c.Runtime.SimulationResolution {
	case "":
		if c.Runtime.SimulationRenderScale == 1 {
			c.Runtime.SimulationResolution = "320x200"
		} else {
			c.Runtime.SimulationResolution = "640x480"
		}
	case "640x400":
		c.Runtime.SimulationResolution = "640x480"
	}
	// The withdrawn graphics preset wrote 10 here independently of the
	// selected motion mode. Preserve that mode and other custom frame rates.
	if c.Runtime.PresentationFPS == 10 {
		c.Runtime.PresentationFPS = Defaults().PresentationFPS
	}
	refreshStockText(c.UI)
	if e = c.Validate(); e != nil {
		return nil, fmt.Errorf("%s: %w", path, e)
	}
	if e = ValidateStorePaths(install, filepath.Dir(path), c); e != nil {
		return nil, fmt.Errorf("%s: %w", path, e)
	}
	return &Document{Path: path, installation: install, Config: c, raw: raw, fingerprint: sha256.Sum256(b)}, nil
}

// Initialize is SETUP-only. Exclusive creation cannot replace a file that a
// second SETUP just created. The game never calls this method.
func Initialize(path string) (*Document, bool, error) { return InitializeAt(path, filepath.Dir(path)) }
func InitializeAt(path, install string) (*Document, bool, error) {
	if filepath.Base(path) == userpack.ConfigName {
		return ensureConfig(path, install)
	}
	if d, e := Load(path); e == nil {
		return d, false, nil
	} else {
		var missing *MissingError
		if !errors.As(e, &missing) {
			return nil, false, e
		}
	}
	c := DefaultConfig()
	b, _ := json.MarshalIndent(c, "", "  ")
	b = append(b, '\n')
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(e) {
		d, e := Load(path)
		return d, false, e
	}
	if e != nil {
		return nil, false, fmt.Errorf("%s: create: %w", path, e)
	}
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return nil, false, fmt.Errorf("%s: create: %w", path, e)
	}
	d, e := Load(path)
	return d, true, e
}

// Settings must write Off explicitly when merging into a saved On. Keep the
// embedded runtime's omitempty representation for historical film launch data.
func (c Config) MarshalJSON() ([]byte, error) {
	type settings Config
	type savedRuntime struct {
		RuntimeOptions
		Floating bool `json:"floating_point_rendering"`
	}
	return json.Marshal(struct {
		settings
		Runtime savedRuntime `json:"runtime"`
	}{settings(c), savedRuntime{c.Runtime, c.Runtime.FloatingGeometry}})
}

func merge(old, new map[string]json.RawMessage) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	for k, v := range old {
		out[k] = v
	}
	for k, v := range new {
		var a, b map[string]json.RawMessage
		if json.Unmarshal(old[k], &a) == nil && a != nil && json.Unmarshal(v, &b) == nil && b != nil {
			v, _ = json.Marshal(merge(a, b))
		}
		out[k] = v
	}
	return out
}
func (d *Document) Save() error {
	if filepath.Base(d.Path) == userpack.ConfigName {
		return d.SaveTogether(nil)
	}
	if e := ValidateStorePaths(d.InstallDirectory(), filepath.Dir(d.Path), d.Config); e != nil {
		return fmt.Errorf("%s: %w", d.Path, e)
	}
	if i, e := os.Stat(d.Path); e == nil && i.Mode().Perm()&0222 == 0 {
		return fmt.Errorf("%s: configuration is read-only", d.Path)
	}
	if e := d.Config.Validate(); e != nil {
		return fmt.Errorf("%s: %w", d.Path, e)
	}
	looseWrites.Lock()
	defer looseWrites.Unlock()
	current, e := Load(d.Path)
	if e != nil {
		return e
	}
	if current.fingerprint != d.fingerprint {
		return fmt.Errorf("%s: %w", d.Path, ErrChanged)
	}
	b, e := json.Marshal(d.Config)
	if e != nil {
		return e
	}
	var known map[string]json.RawMessage
	json.Unmarshal(b, &known)
	all := merge(d.raw, known)
	b, e = json.MarshalIndent(all, "", "  ")
	if e != nil {
		return e
	}
	b = append(b, '\n')
	if len(b) > MaxConfigBytes {
		return fmt.Errorf("%s: saved configuration exceeds1MiB", d.Path)
	}
	f, e := os.CreateTemp(filepath.Dir(d.Path), ".config.cfg-saving-")
	if e != nil {
		return fmt.Errorf("%s: %w", d.Path, e)
	}
	name := f.Name()
	defer os.Remove(name)
	if info, e := os.Stat(d.Path); e == nil {
		if e = f.Chmod(info.Mode().Perm()); e != nil {
			f.Close()
			return e
		}
	}
	_, e = f.Write(b)
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
	current, e = Load(d.Path)
	if e != nil {
		return e
	}
	if current.fingerprint != d.fingerprint {
		return fmt.Errorf("%s: %w", d.Path, ErrChanged)
	}
	if e = os.Rename(name, d.Path); e != nil {
		return fmt.Errorf("%s: atomic replacement: %w", d.Path, e)
	}
	if dir, e := os.Open(filepath.Dir(d.Path)); e == nil {
		_ = dir.Sync()
		dir.Close()
	}
	d.raw = all
	d.fingerprint = sha256.Sum256(b)
	return nil
}

// Duplicate IDs/fields are diagnosed instead of silently collapsing one copy at
// Save. JSON numbers in unrelated fields remain exact raw tokens.
func uniqueKeys(b []byte) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var value func(string, int) error
	value = func(path string, depth int) error {
		if depth > 64 {
			return fmt.Errorf("JSON nesting exceeds64 at %s", path)
		}
		t, e := d.Token()
		if e != nil {
			return e
		}
		switch t {
		case json.Delim('{'):
			seen := map[string]bool{}
			for d.More() {
				k, e := d.Token()
				if e != nil {
					return e
				}
				name := k.(string)
				if seen[name] {
					return fmt.Errorf("duplicate JSON field %s.%s at byte%d", path, name, d.InputOffset())
				}
				seen[name] = true
				if e = value(path+"."+name, depth+1); e != nil {
					return e
				}
			}
			_, e = d.Token()
			return e
		case json.Delim('['):
			i := 0
			for d.More() {
				if e = value(fmt.Sprintf("%s[%d]", path, i), depth+1); e != nil {
					return e
				}
				i++
			}
			_, e = d.Token()
			return e
		}
		return nil
	}
	return value("config", 0)
}

// JSON null has no typed setting meaning. Unknown extension keys remain opaque.
func rejectKnownNull(raw map[string]json.RawMessage, ty reflect.Type, path string) error {
	for i := 0; i < ty.NumField(); i++ {
		f := ty.Field(i)
		key := strings.Split(f.Tag.Get("json"), ",")[0]
		b, ok := raw[key]
		if !ok {
			continue
		}
		name := path + "." + key
		if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
			return field(name, "null is not a typed value")
		}
		if f.Type.Kind() == reflect.Struct {
			m, e := object(b)
			if e != nil {
				return field(name, "expected JSON object")
			}
			if e = rejectKnownNull(m, f.Type, name); e != nil {
				return e
			}
		}
	}
	return nil
}
