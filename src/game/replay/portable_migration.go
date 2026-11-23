package replay

import (
	"bytes"
	"errors"
	"fmt"

	"io"
	"os"
	"path/filepath"
	"strings"
)

// MigrateCompanions runs as explicit maintenance. Listing never performs a
// migration or reads operations. Each replacement is validated before rename.
func (s Store) MigrateCompanions() error {
	entries, e := os.ReadDir(s.Root)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	publicationWrites.Lock()
	defer publicationWrites.Unlock()
	if e = s.recoverNameEdit(); e != nil {
		return e
	}
	var issues []error
	last, e := s.lastID()
	if e != nil {
		return e
	}
	for _, v := range entries {
		if !v.Type().IsRegular() || !strings.EqualFold(filepath.Ext(v.Name()), ".g2r") {
			continue
		}
		p := filepath.Join(s.Root, v.Name())
		m, e := metadataAt(p)
		if e != nil {
			issues = append(issues, fmt.Errorf("preserved %s: %w", v.Name(), e))
			continue
		}
		// A pre-T4 file without publication evidence may be an orphaned private seal.
		if m.portable != nil {
			recPath, _ := s.RecoveryPath(m.Header.ID)
			pending := false
			for _, name := range []string{p + ".title.json", p + ".retained", p + ".unscored.json", recPath} {
				if _, err := os.Lstat(name); !os.IsNotExist(err) {
					pending = true
					break
				}
			}
			if !pending {
				continue
			}
		}
		if m.portable == nil && m.Header.Title == "Last Mission" && last != m.Header.ID && !m.retained {
			if _, e = os.Lstat(p + ".unscored.json"); os.IsNotExist(e) {
				continue
			}
		}
		if _, e = s.resolved(m.Header.ID, true); e != nil {
			issues = append(issues, e)
			continue
		}
		if e = s.migrateOne(p, m); e != nil {
			issues = append(issues, fmt.Errorf("preserved %s: %w", v.Name(), e))
		}
	}
	return errors.Join(issues...)
}
func (s Store) migrateOne(p string, m filmMetadata) error {
	var owned []string
	pub := publication{Title: m.Header.Title}
	recPath, e := s.RecoveryPath(m.Header.ID)
	if e != nil {
		return e
	}
	for _, item := range []struct {
		name  string
		limit int64
	}{{p + ".title.json", 512}, {p + ".retained", 128}, {p + ".unscored.json", 8192}, {recPath, MaxRecoveryBytes}} {
		b, e := smallFile(item.name, item.limit)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		switch item.name {
		case p + ".title.json":
			var t filmTitle
			if strictJSON(b, &t) != nil || t.Revision != m.revision || !ValidTitle(t.Title) || t.Title == "Last Mission" {
				return fmt.Errorf("unbound title metadata preserved")
			}
			pub.Title = t.Title
		case p + ".retained":
			if string(b) != m.revision {
				return fmt.Errorf("unbound retention marker preserved")
			}
		case p + ".unscored.json":
			var u UnscoredDisposition
			if strictJSON(b, &u) != nil || u.Version != 1 || u.FilmID != m.Header.ID || u.ContentSHA256 != m.identity || u.Reason == "" || len(u.Reason) > 4096 {
				return fmt.Errorf("unbound unscored metadata preserved")
			}
			pub.Unscored = &u
		case recPath:
			r, e := UnmarshalRecovery(b)
			if e != nil {
				return e
			}
			pub.Recovery = r
		}
		owned = append(owned, item.name)
	}
	if m.portable != nil {
		// Resume retirement only if each surviving companion matches the activated data.
		for _, n := range owned {
			switch n {
			case p + ".title.json":
				if pub.Title != m.portable.pub.Title {
					return invalidBlock
				}
			case p + ".unscored.json":
				if m.portable.pub.Unscored == nil || *pub.Unscored != *m.portable.pub.Unscored {
					return invalidBlock
				}
			case recPath:
				a, _ := MarshalRecovery(pub.Recovery)
				b, e := MarshalRecovery(m.portable.pub.Recovery)
				if e != nil || !bytes.Equal(a, b) {
					return invalidBlock
				}
			}
		}
	} else {
		file, st, e := regularFilm(p)
		if e != nil {
			return e
		}
		defer file.Close()
		f, e := openBlocks(file, st.Size(), p)
		if e != nil {
			return e
		}
		for i := 0; i < f.OperationCount(); i++ {
			if _, e = f.OperationAt(i); e != nil {
				return e
			}
		}
		digest, e := FileDigest(p)
		if e != nil {
			return e
		}
		if pub.Recovery != nil {
			if e = pub.Recovery.MatchesDigest(digest, f); e != nil {
				return e
			}
		}
		tmp, e := os.CreateTemp(s.Root, ".portable-stage-")
		if e != nil {
			return e
		}
		defer os.Remove(tmp.Name())
		defer tmp.Close()
		if _, e = io.Copy(tmp, file); e != nil {
			return e
		}
		if e = appendPublication(tmp, f.blocks.manifest, pub, MaxBytes); e != nil {
			return e
		}
		size, e := tmp.Stat()
		if e != nil {
			return e
		}
		_, _, _, portable, e := readContainer(tmp, size.Size())
		if e != nil {
			return e
		}
		if portable == nil {
			return invalidBlock
		}
		h, e := portableCoreDigest(tmp.Name(), portable.core)
		if e != nil || h != digest {
			return fmt.Errorf("migration payload changed")
		}
		if e = tmp.Close(); e != nil {
			return e
		}
		if e = file.Close(); e != nil {
			return e
		}
		now, e := FileDigest(p)
		if e != nil || now != digest {
			return fmt.Errorf("source changed during migration")
		}
		if e = os.Rename(tmp.Name(), p); e != nil {
			return e
		}
		if e = syncFilmDirectory(s.Root); e != nil {
			return e
		}
	}
	for _, n := range owned {
		if e = os.Remove(n); e != nil {
			return e
		}
	}
	if len(owned) > 0 {
		return syncFilmDirectory(s.Root)
	}
	return nil
}
