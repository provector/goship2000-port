package replay

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"gunship2000-assets/internal/playerfile"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

var publicationWrites sync.Mutex

type Store struct{ Root string }
type Entry struct {
	unreadableFile      os.FileInfo
	ID, Title, Revision string
	Score, ResumedScore int16
	Complete            bool
	Error               string
	Unscored            bool
	DispositionError    string
}

func (s Store) path(id string) (string, error) {
	if !ValidID(id) {
		return "", fmt.Errorf("invalid managed film identity")
	}
	if s.Root == "" {
		return "", fmt.Errorf("film directory unavailable")
	}
	root, e := filepath.Abs(s.Root)
	if e != nil {
		return "", e
	}
	if info, e := os.Lstat(root); e == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("film directory must not be a symlink")
	}
	return filepath.Join(root, id+".g2r"), nil
}
func (s Store) Load(id string) (*Film, error) {
	// Explicit full-document API retained for offline callers. The library,
	// playback and Export use LoadEntry and its block cursor.
	f, e := s.LoadEntry(Entry{ID: id})
	if e != nil || !f.BlockStorage() {
		return f, e
	}
	return f.Snapshot()
}
func (s Store) LoadEntry(entry Entry) (*Film, error) { return s.loadSelected(entry) }
func (s Store) List() ([]Entry, error) {
	if s.Root == "" {
		return nil, nil
	}
	last, _ := s.lastID()
	names, e := os.ReadDir(s.Root)
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	out := []Entry{}
	seen := map[string]int{}
	for _, n := range names {
		if !strings.EqualFold(filepath.Ext(n.Name()), ".g2r") {
			continue
		}
		p := filepath.Join(s.Root, n.Name())
		m, e := metadataAt(p)
		if e != nil {
			if errors.Is(e, errLegacyRecording) || os.IsNotExist(e) {
				continue
			}
			r := Entry{Title: "Unreadable film", Error: e.Error()}
			if id := strings.TrimSuffix(n.Name(), ".g2r"); ValidID(id) {
				r.ID = id
				r.unreadableFile, _ = n.Info()
			}
			out = append(out, r)
			continue
		}
		id := m.Header.ID
		r := Entry{ID: id, Title: m.Header.Title, Revision: m.selection(), Complete: m.Header.Complete, Score: m.End.Score, ResumedScore: m.End.ResumedScore}
		if i, ok := seen[id]; ok {
			if out[i].Revision != r.Revision {
				out[i].Error = "conflicting recordings claim this identity"
			}
			continue
		}
		if m.portable != nil {
			r.Unscored = m.portable.pub.Unscored != nil
		} else {
			disp, err := s.unscoredIdentity(id, func() (string, error) { return m.identity, nil })
			r.Unscored = disp != nil
			if err != nil && !os.IsNotExist(err) {
				r.DispositionError = err.Error()
			}
		}
		if r.Title == "Last Mission" && id != last {
			if r.Unscored {
				r.Title = "Unscored Mission"
			} else if m.portable != nil {
				r.Title = "Last Mission"
			} else {
				continue
			}
		}
		seen[id] = len(out)
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID == last || out[j].ID == last {
			return out[i].ID == last && out[j].ID != last
		}
		if out[i].Title == "Last Mission" {
			return out[j].Title != "Last Mission"
		}
		if out[j].Title == "Last Mission" {
			return false
		}
		if out[i].Title == out[j].Title {
			return out[i].ID < out[j].ID
		}
		return out[i].Title < out[j].Title
	})
	return out, nil
}
func (s Store) write(f *Film, pub publication) error {
	b, e := Marshal(f)
	if e != nil {
		return e
	}
	p, e := s.path(f.Header.ID)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		return e
	}
	tmp, e := os.CreateTemp(filepath.Dir(p), ".film-*.tmp")
	if e != nil {
		return e
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, e = tmp.Write(b); e == nil {
		e = tmp.Sync()
	}
	if e == nil {
		m, _, _, err := readManifest(bytes.NewReader(b), int64(len(b)))
		e = err
		if e == nil {
			e = appendPublication(tmp, m, pub, MaxBytes)
		}
	}
	if ce := tmp.Close(); e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	if e = playerfile.MoveNew(name, p); e != nil {
		return e
	}
	return syncFilmDirectory(filepath.Dir(p))
}
func (s Store) PublishLast(f *Film) error {
	if f == nil {
		return nil
	}
	p, lockErr := s.path(f.Header.ID)
	if lockErr != nil {
		return lockErr
	}
	if lockErr = os.MkdirAll(filepath.Dir(p), 0700); lockErr != nil {
		return lockErr
	}
	publicationWrites.Lock()
	defer publicationWrites.Unlock()
	if err := s.recoverNameEdit(); err != nil {
		return err
	}
	old, indexErr := s.lastID()
	if indexErr != nil {
		return fmt.Errorf("previous film index retained: %w", indexErr)
	}
	// Repeated finalization of the same immutable film is idempotent. A film
	// already retained under a user title must never be renamed back to Last.
	existing, e := s.metadata(f.Header.ID)
	if e == nil {
		h, err := RecoveryContentIdentity(f)
		if err != nil || h != existing.identity {
			return fmt.Errorf("film identity already belongs to different content")
		}
		if existing.portable != nil && existing.portable.pub.Unscored != nil {
			return fmt.Errorf("existing publication has an unscored disposition")
		}
		if existing.Header.Title != "Last Mission" && old != f.Header.ID {
			return nil
		}
	} else if !os.IsNotExist(e) {
		return fmt.Errorf("existing film identity unavailable; preserved: %w", e)
	} else {
		clone := *f
		clone.Header.Title = "Last Mission"
		if f.BlockStorage() {
			e = s.publishSealed(&clone, p, publication{Title: clone.Header.Title})
		} else {
			e = s.write(&clone, publication{Title: clone.Header.Title})
		}
		if e != nil {
			return e
		}
	}
	// The small pointer is the publication boundary. An interrupted new file
	// write cannot remove the prior Last Mission or any named film.
	if e := s.setLastID(f.Header.ID); e != nil {
		return e
	}
	var retireErr error
	if old != "" && old != f.Header.ID {
		if prior, e := s.metadata(old); e == nil && prior.rollingLabel() {
			if path, e := s.resolved(old, true); e == nil {
				if err := os.Remove(path); err != nil {
					retireErr = fmt.Errorf("remove previous rolling film: %w", err)
				}
			}
		}
	}
	_ = os.Remove(filepath.Join(s.Root, ".film-"+f.Header.ID+".tmp"))
	_ = os.Remove(filepath.Join(s.Root, ".film-"+f.Header.ID+".tmp.sealed"))
	return errors.Join(retireErr, s.namePublished(f))
}
func (s Store) lastID() (string, error) {
	if s.Root == "" {
		return "", nil
	}
	p := filepath.Join(s.Root, "last.json")
	i, e := os.Lstat(p)
	if os.IsNotExist(e) {
		return "", nil
	}
	if e != nil {
		return "", e
	}
	if !i.Mode().IsRegular() || i.Size() > 256 {
		return "", fmt.Errorf("invalid film index")
	}
	b, e := os.ReadFile(p)
	if e != nil {
		return "", e
	}
	var v struct {
		Version int
		ID      string
	}
	if e = json.Unmarshal(b, &v); e != nil || v.Version != 1 || !ValidID(v.ID) {
		return "", fmt.Errorf("invalid film index")
	}
	return v.ID, nil
}
func (s Store) setLastID(id string) error {
	p, e := s.path(id)
	if e != nil {
		return e
	}
	index := filepath.Join(filepath.Dir(p), "last.json")
	prior, readErr := os.ReadFile(index)
	existed := readErr == nil
	if readErr != nil && !os.IsNotExist(readErr) {
		return readErr
	}
	b, _ := json.Marshal(struct {
		Version int
		ID      string
	}{1, id})
	f, e := os.CreateTemp(filepath.Dir(p), ".last-*.tmp")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	if ce := f.Close(); e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	if e = os.Rename(name, filepath.Join(filepath.Dir(p), "last.json")); e != nil {
		return e
	}
	if e = syncFilmDirectory(filepath.Dir(p)); e != nil {
		restoreErr := restoreFilmIndex(index, prior, existed)
		if restoreErr != nil {
			return fmt.Errorf("film index directory sync: %v; predecessor restore: %w", e, restoreErr)
		}
		return fmt.Errorf("film index directory sync failed; predecessor restored: %w", e)
	}
	return nil
}

func (s Store) Rename(id, title string) error { return s.RenameEntry(Entry{ID: id}, title) }
func (s Store) RenameEntry(entry Entry, title string) error {
	id := entry.ID
	p, err := s.path(id)
	if err != nil {
		return err
	}
	publicationWrites.Lock()
	defer publicationWrites.Unlock()
	if err := s.recoverNameEdit(); err != nil {
		return err
	}
	if !ValidTitle(title) {
		return fmt.Errorf("title must be1..20 printable characters, without a leading space")
	}
	m, e := s.selected(entry)
	if e != nil {
		return e
	}
	last, e := s.lastID()
	if e != nil {
		return e
	}
	p, e = s.resolved(id, true)
	if e != nil {
		return e
	}
	display := m.displayTitle(last)
	if entry.Unscored && m.portable == nil && entry.Title != "" {
		display = entry.Title
	}
	entries, e := os.ReadDir(s.Root)
	if e != nil {
		return e
	}
	final := playerfile.NamesIn(entries).Choose(title, filepath.Base(p), ".g2r")
	if final == "Last Mission" && id != last && display != final {
		return fmt.Errorf("Last Mission is reserved for the current rolling film")
	}
	change := display != final
	if change && m.portable == nil {
		return fmt.Errorf("this older film needs portable-films maintenance before changing its title")
	}
	return s.applyName(p, m, final, change, title != display && last == id)
}
func (s Store) Delete(id string) error {
	rows, e := s.List()
	if e != nil {
		return e
	}
	for _, row := range rows {
		if row.ID == id {
			return s.DeleteEntry(row)
		}
	}
	return s.DeleteEntry(Entry{ID: id})
}
func (s Store) DeleteEntry(entry Entry) error {
	id := entry.ID
	p, err := s.path(id)
	if err != nil {
		return err
	}
	publicationWrites.Lock()
	defer publicationWrites.Unlock()
	if err := s.recoverNameEdit(); err != nil {
		return err
	}
	if entry.Revision != "" {
		if _, err := s.selected(entry); err != nil {
			return err
		}
	}
	if entry.unreadableFile == nil {
		p, err = s.resolved(id, true)
		if err != nil {
			return err
		}
	}
	info, e := os.Lstat(p)
	if e != nil {
		return e
	}
	if entry.Error != "" && entry.Revision == "" {
		old := entry.unreadableFile
		if old == nil || !os.SameFile(old, info) || old.Size() != info.Size() || !old.ModTime().Equal(info.ModTime()) {
			return fmt.Errorf("unreadable film changed since selection")
		}
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("not an owned regular film")
	}
	recoveryPath, e := s.RecoveryPath(id)
	if e != nil {
		return e
	}
	if i, err := os.Lstat(recoveryPath); err == nil && !i.Mode().IsRegular() {
		return fmt.Errorf("recovery sidecar is not an owned regular file")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if e = os.Remove(p); e != nil {
		return e
	}
	if entry.unreadableFile != nil {
		return syncFilmDirectory(filepath.Dir(p))
	}
	// Removing an explicitly selected owned film also removes only its bounded
	// sidecar name; it never touches the preserved source copy or other films.
	_ = os.Remove(recoveryPath)
	_ = os.Remove(p + ".title.json")
	_ = os.Remove(p + ".retained")
	if status, err := s.unscoredPath(id); err == nil {
		_ = os.Remove(status)
	}
	if last, _ := s.lastID(); last == id {
		_ = os.Remove(filepath.Join(s.Root, "last.json"))
	}
	return nil
}

// restoreFilmIndex is used only after a post-rename durability failure. A
// secondary error remains explicit; prior complete films are never deleted.
func restoreFilmIndex(path string, prior []byte, existed bool) error {
	if !existed {
		return os.Remove(path)
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".last-restore-*")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if _, e = f.Write(prior); e != nil {
		f.Close()
		return e
	}
	syncErr := f.Sync()
	closeErr := f.Close()
	if e = os.Rename(name, path); e != nil {
		return e
	}
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}
