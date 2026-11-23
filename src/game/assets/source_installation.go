package assets

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"path"
	"strings"
)

// SourceInstallation binds logical source names to one complete supplied game
// directory. Physical provenance/export paths remain untouched. Original DOS
// lookup was case-insensitive; this supported import profile deliberately requires
// its original uppercase filesystem/member names and diagnoses case variants.
// It never combines individually matching files from different installations.
type SourceInstallation struct {
	Root  string
	files map[string][]*Resource
}

func sourceMemberName(r *Resource) (string, error) {
	if r.Source.Kind != "archive_entry" {
		return path.Base(r.Source.Path), nil
	}
	if r.Source.Record == nil {
		return "", fmt.Errorf("archive %s has no raw member identity", r.Source.Path)
	}
	b, e := hex.DecodeString(r.Source.Record.NameHex)
	if e != nil || len(b) != 12 {
		return "", fmt.Errorf("archive %s has invalid raw member name", r.Source.Path)
	}
	name := bytes.SplitN(b, []byte{0}, 2)[0]
	if len(name) == 0 || bytes.IndexAny(name, "/\\") >= 0 {
		return "", fmt.Errorf("archive %s has unsafe member name", r.Source.Path)
	}
	return string(name), nil
}

func validateSourceResource(r *Resource) error {
	if r == nil {
		return fmt.Errorf("nil source resource")
	}
	s := r.Source
	if s.Path == "" || path.IsAbs(s.Path) || path.Clean(s.Path) != s.Path || s.Path == ".." || strings.HasPrefix(s.Path, "../") || strings.Contains(s.Path, "\\") {
		return fmt.Errorf("invalid physical source path %q", s.Path)
	}
	if len(s.SHA256) != 64 || len(s.PayloadSHA256) != 64 {
		return fmt.Errorf("source %s missing content identity", s.Path)
	}
	if r.Payload != nil && (uint64(len(r.Payload)) != s.Length || Hash(r.Payload) != s.PayloadSHA256) {
		return fmt.Errorf("source %s payload length/hash differs", s.Path)
	}
	switch s.Kind {
	case "loose_file":
		if s.SHA256 != s.PayloadSHA256 || s.Offset != 0 {
			return fmt.Errorf("loose source %s has inconsistent identity", s.Path)
		}
	case "archive_entry":
		if s.Container != s.Path || s.Record == nil {
			return fmt.Errorf("archive source %s has inconsistent container", s.Path)
		}
		rec := s.Record
		raw, e := hex.DecodeString(rec.RawHex)
		if e != nil || len(raw) != 24 {
			return fmt.Errorf("archive %s raw directory record unavailable", s.Path)
		}
		if rec.Index < 0 || rec.Index >= maxRecords || hex.EncodeToString(raw[:12]) != rec.NameHex || uint64(binary.LittleEndian.Uint32(raw[16:])) != s.Length || uint64(binary.LittleEndian.Uint32(raw[20:])) != s.Offset || rec.Offset != s.Offset || rec.Length != s.Length || rec.End != s.Offset+s.Length || rec.End > maxSourceBytes || rec.DirectoryOffset != 2+24*rec.Index {
			return fmt.Errorf("archive %s member extent/identity differs", s.Path)
		}
		if r.backing != nil && (s.Offset+s.Length > uint64(len(r.backing)) || !bytes.Equal(r.backing[s.Offset:s.Offset+s.Length], r.Payload)) {
			return fmt.Errorf("archive %s member outside backing bytes", s.Path)
		}
		if _, e = sourceMemberName(r); e != nil {
			return e
		}
	default:
		return fmt.Errorf("source %s has unknown kind %q", s.Path, s.Kind)
	}
	return nil
}

// ResolveSourceInstallation also accepts persisted manifests (Payload/backing nil)
// for profile validation. The prepared reader verifies actual output bytes; source
// identities/record extents remain strict here without requiring original files.
func ResolveSourceInstallation(m *Manifest) (*SourceInstallation, error) {
	if m == nil {
		return nil, fmt.Errorf("missing source inventory")
	}
	c := &SourceInstallation{files: map[string][]*Resource{}}
	physical := map[string]string{}
	folded := map[string]string{}
	for _, r := range m.Resources {
		if e := validateSourceResource(r); e != nil {
			return nil, e
		}
		name := path.Base(r.Source.Path)
		if prior, ok := folded[strings.ToUpper(name)]; ok && prior != r.Source.Path {
			return nil, fmt.Errorf("ambiguous source installation: %s and %s", prior, r.Source.Path)
		}
		folded[strings.ToUpper(name)] = r.Source.Path
		if old, ok := physical[r.Source.Path]; ok && old != r.Source.SHA256 {
			return nil, fmt.Errorf("conflicting source identities at %s", r.Source.Path)
		}
		physical[r.Source.Path] = r.Source.SHA256
		c.files[name] = append(c.files[name], r)
	}
	for _, role := range []SourceRole{RoleMainExecutable, rolePictureCatalog, roleScreenCatalog, roleBuilderCatalog, roleBitmapFonts} {
		want := IslandIce469085.Source(role)
		rows := c.files[want.Name]
		if len(rows) == 0 {
			for name := range c.files {
				if strings.EqualFold(name, want.Name) {
					return nil, fmt.Errorf("unsupported filesystem case %s; supported profile requires %s", name, want.Name)
				}
			}
			return nil, fmt.Errorf("supported installation source %s missing", want.Name)
		}
		dir := path.Dir(rows[0].Source.Path)
		if c.Root == "" {
			c.Root = dir
		} else if c.Root != dir {
			return nil, fmt.Errorf("mixed source installation: %s is outside %s", rows[0].Source.Path, c.Root)
		}
		for _, r := range rows {
			if r.Source.SHA256 != want.SHA256 {
				return nil, fmt.Errorf("unsupported source identity for %s at %s", want.Name, r.Source.Path)
			}
		}
	}
	for file := range physical {
		if path.Dir(file) != c.Root {
			return nil, fmt.Errorf("source %s is outside coherent installation %s", file, c.Root)
		}
	}
	for _, rows := range c.files {
		if rows[0].Source.Kind == "loose_file" && len(rows) != 1 {
			return nil, fmt.Errorf("ambiguous loose source %s", rows[0].Source.Path)
		}
	}
	return c, nil
}

func (c *SourceInstallation) Loose(name string) (*Resource, error) {
	rows := c.files[name]
	if len(rows) == 0 {
		for actual := range c.files {
			if strings.EqualFold(actual, name) {
				return nil, fmt.Errorf("unsupported filesystem case %s; expected %s", actual, name)
			}
		}
	}
	if len(rows) != 1 || rows[0].Source.Kind != "loose_file" {
		return nil, fmt.Errorf("logical loose source %s unavailable in %s", name, c.Root)
	}
	return rows[0], nil
}
func (c *SourceInstallation) Archive(container, member string) (*Resource, error) {
	rows := c.files[container]
	known, ok := catalogHashes[container]
	if !ok || len(rows) == 0 {
		return nil, fmt.Errorf("logical archive %s unavailable in %s", container, c.Root)
	}
	var result *Resource
	for _, r := range rows {
		if r.Source.Kind != "archive_entry" || r.Source.SHA256 != known {
			return nil, fmt.Errorf("unsupported archive identity %s", container)
		}
		n, e := sourceMemberName(r)
		if e != nil {
			return nil, e
		}
		if strings.EqualFold(n, member) && n != member {
			return nil, fmt.Errorf("unsupported archive member case %s/%s; expected %s", container, n, member)
		}
		if n == member {
			if result != nil {
				return nil, fmt.Errorf("ambiguous logical member %s/%s", container, member)
			}
			result = r
		}
	}
	if result == nil {
		return nil, fmt.Errorf("logical archive member %s/%s missing", container, member)
	}
	return result, nil
}
