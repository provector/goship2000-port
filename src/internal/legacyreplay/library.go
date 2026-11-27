package legacyreplay

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"gunship2000-assets/internal/playerdirs"
	"gunship2000-assets/internal/userpack"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gunship2000-assets/game/replay"
)

type Summary struct {
	Converted, Current, Skipped, Failed int
	Reasons                             []string
}

func (s Summary) String() string {
	text := fmt.Sprintf("Converted %d · Already current %d · Skipped %d · Failed %d", s.Converted, s.Current, s.Skipped, s.Failed)
	if len(s.Reasons) > 0 {
		text += "\n" + strings.Join(s.Reasons, "\n")
	}
	return text
}

// ConvertLibrary runs under SETUP's exclusive store ownership. Keep the public
// filename and last-mission pointer stable. A stopped transaction can be retried.
func ConvertLibrary(root string) Summary {
	var result Summary
	fail := func(name string, err error) {
		result.Failed++
		if len(result.Reasons) < 8 {
			result.Reasons = append(result.Reasons, name+": "+err.Error())
		}
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return result
	}
	if err != nil {
		fail(root, err)
		return result
	}
	owners := map[string][]string{}
	type item struct{ path, id string }
	var pending []item
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".g2r") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		p, err := prefix(path)
		if err != nil {
			fail(entry.Name(), err)
			continue
		}
		if p == current {
			f, err := replay.OpenFile(path)
			if err != nil {
				fail(entry.Name(), err)
				continue
			}
			owners[f.Header.ID] = append(owners[f.Header.ID], path)
			result.Current++
			continue
		}
		if p != oldCore && p != oldPublished {
			if strings.HasPrefix(p, "GS2FILM") {
				fail(entry.Name(), errors.New("unsupported storage version"))
			} else {
				result.Skipped++
			}
			continue
		}
		id, err := replay.ImportID(path, format())
		if err != nil {
			fail(entry.Name(), err)
			continue
		}
		owners[id] = append(owners[id], path)
		pending = append(pending, item{path, id})
	}
	for _, film := range pending {
		if len(owners[film.id]) != 1 {
			fail(filepath.Base(film.path), fmt.Errorf("multiple films claim %s", film.id))
			continue
		}
		if err := replaceFilm(film.path); err != nil {
			fail(filepath.Base(film.path), err)
			continue
		}
		result.Converted++
	}
	return result
}
func digest(path string) ([]byte, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	h := sha256.New()
	_, e = io.Copy(h, f)
	return h.Sum(nil), e
}
func replaceFilm(path string) error {
	dir := filepath.Dir(path)
	backup := filepath.Join(dir, "legacy-originals")
	if err := os.Mkdir(backup, 0700); err != nil && !os.IsExist(err) {
		return err
	}
	st, err := os.Lstat(backup)
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return errors.New("legacy-originals must be a local directory")
	}
	tmp, err := playerdirs.Scratch(filepath.Dir(dir))
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	output := filepath.Join(tmp, "converted")
	if err = Convert(path, output); err != nil {
		return err
	}
	original := filepath.Join(backup, filepath.Base(path))
	if err = os.Link(path, original); err != nil {
		if !os.IsExist(err) {
			return fmt.Errorf("retain original: %w", err)
		}
		st, err = os.Lstat(original)
		if err != nil || !st.Mode().IsRegular() {
			return errors.New("invalid previous backup")
		}
		a, err := digest(path)
		if err != nil {
			return err
		}
		b, err := digest(original)
		if err != nil {
			return err
		}
		if !bytes.Equal(a, b) {
			return errors.New("previous backup contains different data")
		}
	}
	if err = syncDirectory(backup); err != nil {
		return err
	}
	if err = syncDirectory(dir); err != nil {
		return err
	}
	// The work directory may be on another volume. Verify the final
	// destination-local image before replacing the original library entry.
	stage, err := os.CreateTemp(dir, ".converted-")
	if err != nil {
		return err
	}
	defer os.Remove(stage.Name())
	src, err := os.Open(output)
	if err != nil {
		stage.Close()
		return err
	}
	_, err = io.Copy(stage, src)
	err = errors.Join(err, src.Close(), stage.Sync(), stage.Close())
	if err != nil {
		return err
	}
	want, err := digest(output)
	if err != nil {
		return err
	}
	got, err := digest(stage.Name())
	if err != nil {
		return err
	}
	if !bytes.Equal(want, got) {
		return errors.New("converted film changed while staging")
	}
	if err = userpack.Replace(stage.Name(), path); err != nil {
		return err
	}
	return syncDirectory(dir)
}
