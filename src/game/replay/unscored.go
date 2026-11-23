package replay

import (
	"bytes"
	"encoding/json"
	"fmt"

	"io"
	"os"
	"path/filepath"
)

// UnscoredDisposition describes publication only. It never authorizes a career
// result or changes recorded simulation facts. Its identity excludes only the
// ordinary editable film title, using the existing recovery identity owner.
type UnscoredDisposition struct {
	Version                       int
	FilmID, ContentSHA256, Reason string
}

func (s Store) unscoredPath(id string) (string, error) {
	p, e := s.path(id)
	return p + ".unscored.json", e
}
func (s Store) Unscored(f *Film) (*UnscoredDisposition, error) {
	if f == nil {
		return nil, fmt.Errorf("missing film")
	}
	m, err := s.metadata(f.Header.ID)
	if err != nil {
		return nil, err
	}
	if m.portable != nil {
		if m.portable.pub.Unscored == nil {
			return nil, os.ErrNotExist
		}
		h, e := RecoveryContentIdentity(f)
		if e != nil || m.portable.pub.Unscored.ContentSHA256 != h {
			return nil, fmt.Errorf("unscored film content differs")
		}
		return m.portable.pub.Unscored, nil
	}
	h, e := RecoveryContentIdentity(f)
	if e != nil {
		return nil, e
	}
	return s.unscoredIdentity(f.Header.ID, func() (string, error) { return h, nil })
}
func (s Store) unscoredIdentity(id string, identity func() (string, error)) (*UnscoredDisposition, error) {
	p, e := s.unscoredPath(id)
	if e != nil {
		return nil, e
	}
	i, e := os.Lstat(p)
	if e != nil {
		return nil, e
	}
	if !i.Mode().IsRegular() || i.Size() > 8192 {
		return nil, fmt.Errorf("invalid unscored disposition file")
	}
	b, e := os.ReadFile(p)
	if e != nil {
		return nil, e
	}
	var v UnscoredDisposition
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e = d.Decode(&v); e != nil {
		return nil, e
	}
	if e = d.Decode(new(any)); e != io.EOF {
		return nil, fmt.Errorf("trailing unscored disposition")
	}
	h, e := identity()
	if e != nil {
		return nil, e
	}
	if v.Version != 1 || v.FilmID != id || v.ContentSHA256 != h || len(v.Reason) == 0 || len(v.Reason) > 4096 {
		return nil, fmt.Errorf("unscored disposition identity differs")
	}
	return &v, nil
}

// PublishUnscored preserves a complete sealed film byte-for-byte without
// replacing the previous Last Mission/index or assigning an unknown career score.
func (s Store) PublishUnscored(f *Film, reason string) error {
	if f == nil {
		return nil
	}
	if !f.Header.Complete {
		return fmt.Errorf("incomplete recording cannot be published")
	}
	if len(reason) == 0 || len(reason) > 4096 {
		return fmt.Errorf("unscored reason must be1..4096 bytes")
	}
	if e := f.Validate(); e != nil {
		return e
	}
	p, e := s.path(f.Header.ID)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		return e
	}
	publicationWrites.Lock()
	defer publicationWrites.Unlock()
	if e = s.recoverNameEdit(); e != nil {
		return e
	}
	if existing, e := s.LoadEntry(Entry{ID: f.Header.ID}); e == nil {
		if _, e = s.Unscored(existing); e != nil {
			return fmt.Errorf("existing film is not this unscored publication: %w", e)
		}
		a, e := RecoveryContentIdentity(f)
		if e != nil {
			return e
		}
		b, e := RecoveryContentIdentity(existing)
		if e != nil || a != b {
			return fmt.Errorf("film identity already belongs to different content")
		}
		if _, e = s.Unscored(existing); e != nil {
			return fmt.Errorf("existing publication has no matching unscored disposition: %w", e)
		}
		if e = syncFilmDirectory(filepath.Dir(p)); e != nil {
			return e
		}
		return s.namePublished(f)
	} else if !os.IsNotExist(e) {
		return fmt.Errorf("existing film identity unavailable; preserved: %w", e)
	}
	if _, e := os.Lstat(p + ".unscored.json"); !os.IsNotExist(e) {
		return fmt.Errorf("pre-existing disposition without a bound film; preserved")
	}
	h, e := RecoveryContentIdentity(f)
	if e != nil {
		return e
	}
	status := UnscoredDisposition{1, f.Header.ID, h, reason}
	pub := publication{Title: f.Header.Title, Unscored: &status}
	if f.BlockStorage() {
		e = s.publishSealed(f, p, pub)
	} else {
		e = s.write(f, pub)
	}
	if e != nil {
		return e
	}
	_ = os.Remove(filepath.Join(s.Root, ".film-"+f.Header.ID+".tmp"))
	_ = os.Remove(filepath.Join(s.Root, ".film-"+f.Header.ID+".tmp.sealed"))
	return s.namePublished(f)
}
func writeNameIntent(path string, b []byte) error {
	tmp, e := os.CreateTemp(filepath.Dir(path), ".name-intent-*")
	if e != nil {
		return e
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, e = tmp.Write(b); e == nil {
		e = tmp.Sync()
	}
	if ce := tmp.Close(); e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	if e = os.Rename(name, path); e != nil {
		return e
	}
	return syncFilmDirectory(filepath.Dir(path))
}
