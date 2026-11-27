package state

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/internal/playerfile"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

var missionWrites sync.Mutex

const AuthoredFormat = "gunship2000-native-mission"
const AuthoredSchema = 1
const AuthoredCompiler = "native-authored-compilation-seed-v1"
const AuthoredCompilerP5 = "native-authored-compilation-seed-v2"
const AuthoredMaxBytes = 4 * 1024 * 1024

type AuthoredSearchPoint struct {
	Profile string `json:"profile"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
}
type AuthoredPlacement struct {
	GeneratedGroup   bool                     `json:"generated_group,omitempty"`
	GoalCopyProfile  string                   `json:"goal_copy_profile,omitempty"`
	TrainProfile     string                   `json:"train_profile,omitempty"`
	Entry            *AuthoredDeepStrikeEntry `json:"entry,omitempty"`
	Route            *AuthoredRoute           `json:"route,omitempty"`
	Search           *AuthoredSearchPoint     `json:"search,omitempty"`
	SelectionProfile string                   `json:"selection_profile,omitempty"`
	Filled           bool                     `json:"filled,omitempty"`
	ID               string                   `json:"id"`
	Role             string                   `json:"role"`
	Formation        int                      `json:"formation"`
	Component        int                      `json:"component"`
	Template         int                      `json:"template"`
	Kind             int                      `json:"kind"`
	X                int                      `json:"x"`
	Y                int                      `json:"y"`
	Terrain          byte                     `json:"terrain"`
	Builtin          bool                     `json:"builtin,omitempty"`
	Reference        string                   `json:"reference,omitempty"`
}
type AuthoredDocument struct {
	Generated         *GeneratedBlueprint    `json:"generated,omitempty"`
	ResourcePolicy    string                 `json:"resource_policy,omitempty"`
	SupportEdits      *AuthoredSupportEdits  `json:"support_edits,omitempty"`
	TargetNameProfile string                 `json:"target_name_profile,omitempty"`
	SupportMemory     *AuthoredSupportMemory `json:"support_memory,omitempty"`
	Support           *AuthoredSupport       `json:"support,omitempty"`
	Editor            *AuthoredEditorState   `json:"editor,omitempty"`
	// Optional native accepted loadout; original SIM did not serialize full SHIP/WEAPON.
	Aircraft   [][36]byte              `json:"aircraft,omitempty"`
	Initial    *AuthoredInitialCarrier `json:"initial,omitempty"`
	Format     string                  `json:"format"`
	Schema     int                     `json:"schema"`
	ID         string                  `json:"id"`
	Title      string                  `json:"title"`
	World      string                  `json:"world"`
	Seed       uint32                  `json:"seed"`
	Compiler   string                  `json:"compiler"`
	Dependency string                  `json:"dependency"`
	Placements []AuthoredPlacement     `json:"placements"`
	Conditions MissionConditions       `json:"conditions"`
	Compiled   *AuthoredRealization    `json:"compiled,omitempty"`
}

// A verified native film can retain an exact source-known initial configuration.
// It contains no controls/dynamic actors/results or mid-flight snapshot.
type AuthoredInitialCarrier struct {
	Profile    string              `json:"profile"`
	SourceType MissionType         `json:"source_type"`
	Digest     string              `json:"digest"`
	Layout     AuthoredRealization `json:"layout"`
}
type AuthoredRealization struct {
	TargetNameProfile string                      `json:"target_name_profile,omitempty"`
	Entry             *AuthoredPlacement          `json:"entry,omitempty"`
	Support           *SupportConfiguration       `json:"support,omitempty"`
	TargetCountdown   *uint16                     `json:"target_countdown,omitempty"`
	Revision          string                      `json:"revision"`
	Objects           []MissionObject             `json:"objects"`
	Objectives        map[string]MissionObjective `json:"objectives"`
	Targets           MissionTargets              `json:"targets"`
	Paths             []missionPath               `json:"paths"`
	Conditions        MissionConditions           `json:"conditions"`
	FlightTemplate    int                         `json:"flight_template"`
	Rank              int                         `json:"rank"`
	Scene             int                         `json:"scene"`
	RawE278           int                         `json:"raw_e278"`
	ObjectHash        string                      `json:"object_hash"`
	Exported          bool                        `json:"exported,omitempty"`
}
type AuthoredOption struct {
	Unavailable                                    string
	SelectionProfile                               string
	Formation, Component, Template, Cost, Category int
	Name                                           string
	Fixed                                          bool
	X, Y                                           int
}
type AuthoredEntry struct{ ID, Title, World, Revision, Error string }
type AuthoredStore struct{ Directory string }

func authoredID() (string, error) {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return hex.EncodeToString(b), nil
}
func authoredDigest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func authoredBytesDigest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func AuthoredRevision(d *AuthoredDocument) string {
	if d == nil {
		return ""
	}
	q := *d
	q.Compiled = nil
	return authoredDigest(q)
}
func CloneAuthored(d *AuthoredDocument) (*AuthoredDocument, error) {
	b, e := json.Marshal(d)
	if e != nil {
		return nil, e
	}
	var q AuthoredDocument
	e = json.Unmarshal(b, &q)
	return &q, e
}
func AuthoredWorld(data *assetdata.MissionGenerationData, name string) (*assetdata.MissionWorld, error) {
	if data == nil || data.Version != 1 {
		return nil, fmt.Errorf("mission source tables unavailable")
	}
	for i := range data.Worlds {
		if data.Worlds[i].WorldName == name {
			return &data.Worlds[i], nil
		}
	}
	return nil, fmt.Errorf("mission world %q unavailable", name)
}
func authoredDependency(w *assetdata.MissionWorld) string { return authoredDigest(w) }
func ValidateAuthoredTitle(s string) error {
	if len(s) < 1 || len(s) > 20 || s[0] == ' ' {
		return fmt.Errorf("mission title needs1..20 characters without a leading space")
	}
	for _, r := range s {
		if r < 32 || r > 126 {
			return fmt.Errorf("mission title uses unsupported characters")
		}
	}
	return nil
}
func validateAuthoredShape(d *AuthoredDocument) error {
	if d == nil || d.Format != AuthoredFormat || d.Schema != AuthoredSchema || (d.Compiler != AuthoredCompiler && d.Compiler != AuthoredCompilerP5 && d.Compiler != AuthoredCompilerSupport && d.Compiler != AuthoredCompilerSupportP5) {
		return fmt.Errorf("unsupported native mission format/compiler")
	}
	if d.SupportEdits != nil && (d.SupportEdits.Profile != OwnedSupportChoiceV1 || d.SupportMemory != nil || (d.SupportEdits.PreviousRevision == "") != (d.SupportEdits.RetiredReservations == "")) {
		return fmt.Errorf("invalid owned Support-choice policy")
	}
	if d.ResourcePolicy != "" && d.ResourcePolicy != NativeModelResourcesV1 {
		return fmt.Errorf("unsupported authored resource policy")
	}
	if !authoredValidID(d.ID) {
		return fmt.Errorf("invalid mission document identity")
	}
	if d.TargetNameProfile != "" && d.TargetNameProfile != AuthoredTargetNameProfile {
		return fmt.Errorf("unsupported authored target-name interpretation")
	}
	if e := ValidateAuthoredTitle(d.Title); e != nil {
		return e
	}
	if len(d.World) < 1 || len(d.World) > 16 || len(d.Dependency) != 64 {
		return fmt.Errorf("missing mission world/dependency")
	}
	if len(d.Aircraft) != 0 && len(d.Aircraft) != 5 && !(d.Generated != nil && len(d.Generated.Reserved) == 1 && len(d.Aircraft) == 1) {
		return fmt.Errorf("native mission loadout requires exactly five accepted rows")
	}
	for _, row := range d.Aircraft {
		if row[0] > 7 {
			return fmt.Errorf("mission aircraft selector is unsupported")
		}
	}
	if len(d.Placements) > 80 {
		return fmt.Errorf("mission placement count exceeds source capacity")
	}
	seen := map[string]bool{}
	roles := map[string]bool{}
	for _, p := range d.Placements {
		if p.GoalCopyProfile != "" && (!authoredDetachedGoal(p)) {
			return fmt.Errorf("invalid detached source goal interpretation")
		}
		if p.TrainProfile != "" && p.TrainProfile != AuthoredTrainProfile {
			return fmt.Errorf("unsupported Train constructor interpretation")
		}
		if !authoredValidID(p.ID) || seen[p.ID] {
			return fmt.Errorf("invalid/duplicate placement identity")
		}
		seen[p.ID] = true
		if p.Role != "defender" && p.Role != "base" && p.Role != "FARP" && p.Role != "primary" && p.Role != "secondary" {
			return fmt.Errorf("unsupported mission placement role %q", p.Role)
		}
		if p.Role != "defender" {
			if roles[p.Role] {
				return fmt.Errorf("duplicate %s", p.Role)
			}
			roles[p.Role] = true
		}
		if p.SelectionProfile != "" && p.SelectionProfile != AuthoredSelectionProfile && p.SelectionProfile != AuthoredSelectionProfileP2 && !authoredP5Selection(p.SelectionProfile) {
			return fmt.Errorf("unsupported editor selection profile")
		}
		if p.Entry != nil && (p.Entry.Profile != AuthoredEntryProfile || p.Role != "base" || d.Initial != nil) {
			return fmt.Errorf("invalid Deep Strike placement carrier")
		}
		if p.Route != nil {
			if p.Kind != 2 || (p.Role != "primary" && p.Role != "secondary") {
				return fmt.Errorf("invalid source Intercept route role")
			}
			switch p.Route.Profile {
			case AuthoredRouteProfile:
				if p.Route.X < 0 || p.Route.X >= 1536 || p.Route.Y < 0 || p.Route.Y >= 1152 {
					return fmt.Errorf("planned Intercept destination outside source map")
				}
			case AuthoredTrainRouteProfile:
				// Fixed source rails may end beyond the visible map. The exact
				// derived endpoint is checked against immutable PATH before use.
				if p.TrainProfile != AuthoredTrainProfile || p.Route.X < -32768 || p.Route.X > 32767 || p.Route.Y < -32768 || p.Route.Y > 32767 {
					return fmt.Errorf("invalid Train preset endpoint")
				}
			default:
				return fmt.Errorf("unsupported source Intercept route interpretation")
			}
		}
		if e := validateAuthoredSearch(d, p); e != nil {
			return e
		}
		if p.Filled && p.Role != "defender" {
			return fmt.Errorf("only defenders can own Fill provenance")
		}
		if p.Formation < 0 || p.Formation > 255 || p.Component < 0 || p.Component > 8191 || (p.Template < 0 && p.Entry == nil) || p.Template > 65535 || p.X < 0 || p.X > 1535 || p.Y < 0 || p.Y > 1151 || p.Kind < 0 || p.Kind > 7 {
			return fmt.Errorf("mission placement outside source domain")
		}
	}
	if d.Editor != nil && (d.Editor.Profile != AuthoredControlProfile || d.Editor.FillBoundary < 0 || d.Editor.FillBoundary > 80) {
		return fmt.Errorf("unsupported editor control profile")
	}
	if d.Conditions.WindSpeed < 0 || d.Conditions.WindSpeed > 30 {
		return fmt.Errorf("wind speed outside0..30")
	}
	if c := d.Compiled; c != nil {
		if c.TargetNameProfile != "" && c.TargetNameProfile != AuthoredTargetNameProfile || d.Initial == nil && c.TargetNameProfile != d.TargetNameProfile {
			return fmt.Errorf("compiled target-name interpretation is inconsistent")
		}
		if c.Revision != AuthoredRevision(d) || (len(c.Objects) != 80 && !(d.Generated != nil && len(c.Objects) == 81)) || len(c.Paths) > 7281 || len(c.Objectives) > 5 || c.FlightTemplate < 0 || c.Rank < 5 || c.Rank > 11 || c.Scene < 0 || c.Scene > 5 {
			return fmt.Errorf("compiled mission realization is inconsistent")
		}
		if c.ObjectHash != authoredObjectHash(c.Objects) {
			return fmt.Errorf("compiled OBJECT digest differs")
		}
	}
	return nil
}
func authoredValidID(s string) bool {
	if len(s) != 32 {
		return false
	}
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 16
}
func (s AuthoredStore) path(id string) (string, error) {
	if !authoredValidID(id) {
		return "", fmt.Errorf("invalid mission ID")
	}
	if s.Directory == "" {
		return "", fmt.Errorf("mission directory unavailable")
	}
	p := filepath.Join(s.Directory, id+".g2m")
	return p, nil
}
func (s AuthoredStore) resolveDocument(id string, forWrite bool) (string, error) {
	canonical, e := s.path(id)
	if e != nil {
		return "", e
	}
	files, e := os.ReadDir(s.Directory)
	if os.IsNotExist(e) {
		return canonical, nil
	}
	if e != nil {
		return "", e
	}
	var found, revision string
	for _, v := range files {
		if !strings.EqualFold(filepath.Ext(v.Name()), ".g2m") {
			continue
		}
		p := filepath.Join(s.Directory, v.Name())
		d, rev, e := readMissionDocument(p)
		if e != nil || d.ID != id {
			if p == canonical {
				return "", fmt.Errorf("occupied mission filename; content or identity differs")
			}
			continue
		}
		if found != "" {
			if forWrite || revision != rev {
				return "", fmt.Errorf("multiple missions claim identity %s; retain one before editing", id)
			}
			continue
		}
		found, revision = p, rev
	}
	if found != "" {
		return found, nil
	}
	return canonical, nil
}
func (s AuthoredStore) Load(id string) (*AuthoredDocument, string, error) {
	p, e := s.resolveDocument(id, false)
	if e != nil {
		return nil, "", e
	}
	return readMissionDocument(p)
}
func readMissionDocument(p string) (*AuthoredDocument, string, error) {
	var e error
	if st, e := os.Lstat(p); e != nil {
		return nil, "", e
	} else if !st.Mode().IsRegular() || st.Size() > AuthoredMaxBytes {
		return nil, "", fmt.Errorf("expected a bounded regular mission file")
	}
	f, e := os.Open(p)
	if e != nil {
		return nil, "", e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return nil, "", e
	}
	if !st.Mode().IsRegular() || st.Size() > AuthoredMaxBytes {
		return nil, "", fmt.Errorf("mission file exceeds bounded format")
	}
	b, e := io.ReadAll(io.LimitReader(f, AuthoredMaxBytes+1))
	if e != nil {
		return nil, "", e
	}
	if len(b) > AuthoredMaxBytes {
		return nil, "", fmt.Errorf("mission file too large")
	}
	if e = authoredJSONBounds(b); e != nil {
		return nil, "", e
	}
	var d AuthoredDocument
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if e = dec.Decode(&d); e != nil {
		return nil, "", fmt.Errorf("read mission: %w", e)
	}
	if dec.Decode(new(any)) != io.EOF {
		return nil, "", fmt.Errorf("mission contains trailing data")
	}
	if e = validateAuthoredShape(&d); e != nil {
		return nil, "", e
	}

	return &d, authoredBytesDigest(b), nil
}
func (s AuthoredStore) List() ([]AuthoredEntry, error) {
	entries, e := os.ReadDir(s.Directory)
	if os.IsNotExist(e) {
		return []AuthoredEntry{}, nil
	}
	if e != nil {
		return nil, e
	}
	out := []AuthoredEntry{}
	seen := map[string]int{}
	for _, v := range entries {
		if !strings.EqualFold(filepath.Ext(v.Name()), ".g2m") {
			continue
		}
		d, rev, err := readMissionDocument(filepath.Join(s.Directory, v.Name()))
		if err != nil {
			out = append(out, AuthoredEntry{Title: "Unreadable mission", Error: err.Error()})
			continue
		}
		if i, ok := seen[d.ID]; ok {
			if out[i].Revision != rev {
				out[i].Error = "conflicting missions claim the same ID"
			}
			continue
		}
		seen[d.ID] = len(out)
		out = append(out, AuthoredEntry{ID: d.ID, Title: d.Title, World: d.World, Revision: rev})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Title == out[j].Title {
			return out[i].ID < out[j].ID
		}
		return out[i].Title < out[j].Title
	})
	return out, nil
}
func (s AuthoredStore) Save(d *AuthoredDocument, expected string) (string, error) {
	unlock, e := s.lock()
	if e != nil {
		return "", e
	}
	defer unlock()
	return s.saveLocked(d, expected)
}

// Title selection and publication share the same reservation, including
// simultaneous exports of two differently identified films with the same name.
func (s AuthoredStore) SaveExport(d *AuthoredDocument) (string, error) {
	if d == nil || d.Compiled == nil {
		return "", fmt.Errorf("export lacks compiled mission")
	}
	unlock, err := s.lock()
	if err != nil {
		return "", err
	}
	defer unlock()
	entries, err := s.List()
	if err != nil {
		return "", err
	}
	names := map[string]bool{}
	for _, entry := range entries {
		names[strings.ToLower(entry.Title)] = true
	}
	title := d.Title
	for i := 2; names[strings.ToLower(d.Title)]; i++ {
		suffix := fmt.Sprintf(" (%d)", i)
		d.Title = strings.TrimRight(title[:min(len(title), 20-len(suffix))], " ") + suffix
	}
	d.Compiled.Revision = AuthoredRevision(d)
	return s.saveLocked(d, "")
}
func (s AuthoredStore) saveLocked(d *AuthoredDocument, expected string) (string, error) {
	if e := validateAuthoredShape(d); e != nil {
		return "", e
	}
	if d.Compiled == nil {
		return "", fmt.Errorf("SAVE requires a successfully compiled valid mission")
	}
	p, e := s.resolveDocument(d.ID, true)
	if e != nil {
		return "", e
	}

	if e = os.MkdirAll(s.Directory, 0700); e != nil {
		return "", e
	}
	if st, e := os.Lstat(p); e == nil {
		if st.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("mission path is a symbolic link")
		}
		_, rev, e := s.Load(d.ID)
		if e != nil {
			return "", e
		}
		if expected == "" || rev != expected {
			return "", fmt.Errorf("mission changed outside the editor")
		}
	} else if !os.IsNotExist(e) {
		return "", e
	} else if expected != "" {
		return "", fmt.Errorf("mission was removed outside the editor")
	}
	b, e := json.MarshalIndent(d, "", "  ")
	if e != nil {
		return "", e
	}
	b = append(b, '\n')
	if len(b) > AuthoredMaxBytes {
		return "", fmt.Errorf("mission exceeds file limit")
	}
	f, e := os.CreateTemp(s.Directory, ".mission-*.tmp")
	if e != nil {
		return "", e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, e = f.Write(b); e != nil {
		f.Close()
		return "", e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return "", e
	}
	if e = f.Close(); e != nil {
		return "", e
	}
	entries, e := os.ReadDir(s.Directory)
	if e != nil {
		return "", e
	}
	source := ""
	if expected != "" {
		source = filepath.Base(p)
	}
	name := playerfile.NamesIn(entries).Choose(d.Title, source, ".g2m") + ".g2m"
	dest := filepath.Join(s.Directory, name)
	if expected == "" {
		if e = playerfile.MoveNew(tmp, dest); e != nil {
			return "", e
		}
	} else {
		// Move the still-valid generation first. Its internal ID remains discoverable
		// even if the process stops before the new document is published.
		if dest != p {
			if e = playerfile.MoveNew(p, dest); e != nil {
				return "", e
			}
		}
		if e = os.Rename(tmp, dest); e != nil {
			if dest != p {
				e = errors.Join(e, playerfile.MoveNew(dest, p))
			}
			return "", e
		}
	}
	if dir, e := os.Open(s.Directory); e == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return authoredBytesDigest(b), nil
}
func (s AuthoredStore) Delete(id, expected string) error {
	unlock, e := s.lock()
	if e != nil {
		return e
	}
	defer unlock()
	p, e := s.resolveDocument(id, true)
	if e != nil {
		return e
	}
	_, rev, e := s.Load(id)
	if e != nil {
		return e
	}
	if expected == "" || rev != expected {
		return fmt.Errorf("mission changed outside the editor")
	}
	return os.Remove(p)
}

// Bound collection expansion and reject duplicate keys before typed decoding.
// encoding/json otherwise silently ignores excess fixed-array elements.
func authoredJSONBounds(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	tokens := 0
	var walk func(string, int) error
	walk = func(context string, depth int) error {
		if depth > 32 {
			return fmt.Errorf("mission JSON nesting exceeds limit")
		}
		tok, e := dec.Token()
		if e != nil {
			return e
		}
		tokens++
		if tokens > 100000 {
			return fmt.Errorf("mission JSON element budget exceeded")
		}
		switch v := tok.(type) {
		case json.Delim:
			switch v {
			case '{':
				seen := map[string]bool{}
				for dec.More() {
					key, e := dec.Token()
					if e != nil {
						return e
					}
					s, ok := key.(string)
					if !ok || seen[s] {
						return fmt.Errorf("duplicate/invalid mission field")
					}
					seen[s] = true
					if e = walk(strings.ToLower(s), depth+1); e != nil {
						return e
					}
				}
				end, e := dec.Token()
				if e != nil || end != json.Delim('}') {
					return fmt.Errorf("invalid mission object")
				}
			case '[':
				limit := 128
				child := context
				switch context {
				case "aircraft":
					limit = 5
					child = "aircraftrow"
				case "aircraftrow":
					limit = 36
				case "objects":
					limit = 81
				case "placements":
					limit = 80
				case "paths", "route":
					limit = 7281
				case "records":
					limit = 5
					child = "targetrow"
				case "targetrow":
					limit = 62
				case "raw":
					limit = 32
				}
				n := 0
				for dec.More() {
					n++
					if n > limit {
						return fmt.Errorf("mission %s array exceeds%d", context, limit)
					}
					if e = walk(child, depth+1); e != nil {
						return e
					}
				}
				end, e := dec.Token()
				if e != nil || end != json.Delim(']') {
					return fmt.Errorf("invalid mission array")
				}
			default:
				return fmt.Errorf("invalid mission JSON delimiter")
			}
		}
		return nil
	}
	if e := walk("", 0); e != nil {
		return e
	}
	if _, e := dec.Token(); e != io.EOF {
		return fmt.Errorf("mission has trailing JSON")
	}
	return nil
}
func (s AuthoredStore) lock() (func(), error) {
	if s.Directory == "" {
		return nil, fmt.Errorf("mission directory unavailable")
	}
	abs, e := filepath.Abs(s.Directory)
	if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(abs, 0700); e != nil {
		return nil, e
	}
	real, e := filepath.EvalSymlinks(abs)
	if e != nil {
		return nil, e
	}
	if filepath.Clean(real) != filepath.Clean(abs) {
		return nil, fmt.Errorf("mission directory uses a symbolic link")
	}
	missionWrites.Lock()
	return missionWrites.Unlock, nil
}
