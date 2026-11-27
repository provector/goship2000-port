package replay

import (
	"encoding/json"
	"errors"
	"fmt"
	"gunship2000-assets/internal/playerfile"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const editJournal = ".film-name-edit.json"

type nameEdit struct {
	ID, Binding, From, Title, Previous string
	Generation                         uint64
	ReleaseLast                        bool
}

func (s Store) forgetEdit() error {
	if err := os.Remove(filepath.Join(s.Root, editJournal)); err != nil {
		return err
	}
	return syncFilmDirectory(s.Root)
}

// The checksummed title is authoritative. Before moving, persist the intended
// slot change and any local retention change. Only the matching publication
// and title generation may finish this intent after a process interruption.
func (s Store) finishNameEdit(x nameEdit, path string, m filmMetadata) error {
	if m.portable == nil || x.ID != m.Header.ID || fmt.Sprintf("%x", m.portable.binding) != x.Binding ||
		filepath.Base(x.From) != x.From || !strings.EqualFold(filepath.Ext(x.From), ".g2r") || !ValidTitle(x.Title) || playerfile.Basename(x.Title) != x.Title || !ValidTitle(x.Previous) {
		return fmt.Errorf("recording name transaction does not match publication")
	}
	before := m.portable.generation == x.Generation && m.Header.Title == x.Previous
	after := m.portable.generation == x.Generation+1 && m.Header.Title == x.Title
	if !before && !after {
		return fmt.Errorf("recording title changed during name transaction")
	}
	name := x.Title + ".g2r"
	if filepath.Base(path) != name {
		if filepath.Base(path) != x.From || !before {
			return fmt.Errorf("recording moved during name transaction")
		}
		dest := filepath.Join(filepath.Dir(path), name)
		if err := playerfile.MoveNew(path, dest); err != nil {
			return &os.LinkError{Op: "rename recording", Old: path, New: dest, Err: err}
		}
		path = dest
		if err := syncFilmDirectory(s.Root); err != nil {
			return err
		}
	}
	if before {
		if err := renameSlot(path, m.portable, x.Title); err != nil {
			// A read-only file can be movable in a writable directory. Undo
			// the move only if the original generation still validates. If
			// the slot write actually landed, keep the intent for recovery.
			now, check := metadataAt(path)
			if check != nil || now.portable == nil || now.portable.binding != m.portable.binding ||
				now.portable.generation != x.Generation || now.Header.Title != x.Previous {
				return err
			}
			if filepath.Base(path) != x.From {
				if undo := playerfile.MoveNew(path, filepath.Join(filepath.Dir(path), x.From)); undo != nil {
					return errors.Join(err, fmt.Errorf("restoring original film name: %w", undo))
				}
			}
			return errors.Join(err, s.forgetEdit())
		}
	}
	if after {
		// Recovery may observe the new slot after a process died between
		// WriteAt and Sync. Flush it before retiring the rolling pointer.
		f, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			return err
		}
		if err = errors.Join(f.Sync(), f.Close()); err != nil {
			return err
		}
	}
	if x.ReleaseLast {
		last, err := s.lastID()
		if err != nil {
			return err
		}
		if last == x.ID {
			if err = os.Remove(filepath.Join(s.Root, "last.json")); err != nil {
				return err
			}
		}
	}
	return s.forgetEdit()
}
func (s Store) recoverNameEdit() error {
	b, err := smallFile(filepath.Join(s.Root, editJournal), 2048)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var x nameEdit
	if strictJSON(b, &x) != nil || !ValidID(x.ID) {
		return fmt.Errorf("invalid recording rename journal")
	}
	path, err := s.resolved(x.ID, true)
	if err != nil {
		return err
	}
	m, err := metadataAt(path)
	if err != nil {
		return err
	}
	err = s.finishNameEdit(x, path, m)
	var moveErr *os.LinkError
	if errors.As(err, &moveErr) && os.IsExist(moveErr.Err) {
		// Another file occupied the destination before the interrupted move.
		// The checked old generation still owns its original filename.
		return s.forgetEdit()
	}
	return err
}

func (s Store) applyName(path string, m filmMetadata, title string, changeTitle, releaseLast bool) error {
	if !changeTitle {
		dest := filepath.Join(filepath.Dir(path), title+".g2r")
		if dest == path {
			return nil
		}
		if err := playerfile.MoveNew(path, dest); err != nil {
			return err
		}
		return syncFilmDirectory(s.Root)
	}
	if m.portable == nil {
		return fmt.Errorf("recording needs portable publication metadata before changing title")
	}
	x := nameEdit{ID: m.Header.ID, Binding: fmt.Sprintf("%x", m.portable.binding), From: filepath.Base(path), Title: title, Previous: m.Header.Title, Generation: m.portable.generation, ReleaseLast: releaseLast}
	data, err := json.Marshal(x)
	if err != nil {
		return err
	}
	if err = writeNameIntent(filepath.Join(s.Root, editJournal), data); err != nil {
		return err
	}
	if err = s.finishNameEdit(x, path, m); err != nil {
		// A refused move (including an open Windows handle) has made no
		// change. Do not retry a failed user action at the next launch.
		var moveErr *os.LinkError
		if errors.As(err, &moveErr) {
			return errors.Join(err, s.forgetEdit())
		}
	}
	return err
}

type namedFilm struct {
	path string
	m    filmMetadata
}

func (s Store) ReconcileNames() error {
	if s.Root == "" {
		return nil
	}
	if _, err := os.Lstat(s.Root); os.IsNotExist(err) {
		return nil
	}
	if _, err := s.path(strings.Repeat("0", 32)); err != nil {
		return err
	}
	publicationWrites.Lock()
	defer publicationWrites.Unlock()
	if err := s.recoverNameEdit(); err != nil {
		return err
	}
	entries, err := os.ReadDir(s.Root)
	if err != nil {
		return err
	}
	names := playerfile.NamesIn(entries)
	ids := map[string]int{}
	var films []namedFilm
	var issues []error
	for _, d := range entries {
		if !strings.EqualFold(filepath.Ext(d.Name()), ".g2r") {
			continue
		}
		p := filepath.Join(s.Root, d.Name())
		m, err := metadataAt(p)
		if err != nil {
			if errors.Is(err, errLegacyRecording) || os.IsNotExist(err) {
				continue
			}
			issues = append(issues, fmt.Errorf("preserved %q: %w", d.Name(), err))
			continue
		}
		ids[m.Header.ID]++
		films = append(films, namedFilm{p, m})
	}
	last, err := s.lastID()
	if err != nil {
		return err
	}
	for _, f := range films {
		name := filepath.Base(f.path)
		m := f.m
		if ids[m.Header.ID] != 1 {
			issues = append(issues, fmt.Errorf("preserved %q: multiple files claim film %s", name, m.Header.ID))
			continue
		}
		if m.portable == nil {
			issues = append(issues, fmt.Errorf("preserved %q: no portable publication record", name))
			continue
		}
		// Interrupted legacy companion retirement belongs to MigrateCompanions.
		if names[strings.ToLower(name+".title.json")]+names[strings.ToLower(name+".retained")]+names[strings.ToLower(name+".unscored.json")]+names[strings.ToLower(m.Header.ID+".recovery.json")] > 0 {
			issues = append(issues, fmt.Errorf("preserved %q: pending recording companions", name))
			continue
		}
		display := m.displayTitle(last)
		title := names.Choose(display, name, ".g2r")
		if title+".g2r" == name && title == display {
			continue
		}
		if err = s.applyName(f.path, m, title, title != display, false); err != nil {
			issues = append(issues, fmt.Errorf("preserved recording %q: %w", name, err))
			if _, e := os.Lstat(filepath.Join(s.Root, editJournal)); !os.IsNotExist(e) {
				break
			}
			continue
		}
		names[strings.ToLower(name)]--
		names[strings.ToLower(title+".g2r")]++
	}
	return errors.Join(issues...)
}
func (m filmMetadata) displayTitle(last string) string {
	if m.Header.Title == "Last Mission" && m.Header.ID != last && m.portable != nil && m.portable.pub.Unscored != nil {
		return "Unscored Mission"
	}
	return m.Header.Title
}

// last.json owns rolling membership. Also recognize an allocated collision
// suffix, while preserving older named films behind a stale pointer.
func (m filmMetadata) rollingLabel() bool {
	if m.Header.Title == "Last Mission" {
		return true
	}
	n, ok := strings.CutPrefix(m.Header.Title, "Last Mission (")
	if !ok {
		return false
	}
	n, ok = strings.CutSuffix(n, ")")
	if !ok {
		return false
	}
	i, err := strconv.Atoi(n)
	return err == nil && i >= 2
}
func (s Store) namePublished(f *Film) error {
	path, err := s.resolved(f.Header.ID, true)
	if err != nil {
		return err
	}
	m, err := metadataAt(path)
	if err != nil {
		return err
	}
	last, err := s.lastID()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(s.Root)
	if err != nil {
		return err
	}
	title := m.displayTitle(last)
	final := playerfile.NamesIn(entries).Choose(title, filepath.Base(path), ".g2r")
	if err = s.applyName(path, m, final, final != title, false); err != nil {
		return err
	}
	if f.blocks != nil {
		f.blocks.path = filepath.Join(filepath.Dir(path), final+".g2r")
	}
	return nil
}
