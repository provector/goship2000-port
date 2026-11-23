package replay

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var ErrReportWrite = errors.New("writing recording into report")

func reportNames(id string) []string {
	return []string{id + ".g2r", ".film-" + id + ".tmp.sealed", ".film-" + id + ".tmp"}
}

// Keep incident members ID-based even when the live library uses a display name.
func reportPaths(root, id string) []string {
	names := reportNames(id)
	// ID-named pinned evidence can be an interrupted prefix without a manifest.
	// Its initialization is authenticated by CopyReportFilm.
	if paths, err := (Store{root}).matching(id); err == nil && len(paths) > 0 {
		names[0] = filepath.Base(paths[0])
	}
	return names
}

// Pin one inode at a mission boundary so rolling publication or removal cannot
// remove the source before the worker opens it. No payload I/O on the UI path.
func PinReportFilm(root, incident, id string) {
	if !ValidID(id) {
		return
	}
	for i, name := range reportPaths(root, id) {
		src, dst := filepath.Join(root, name), filepath.Join(incident, reportNames(id)[i])
		f, st, err := regularFilm(src)
		if err != nil {
			continue
		}
		f.Close()
		if err = os.Link(src, dst); err == nil {
			linked, err := os.Lstat(dst)
			if err == nil && os.SameFile(st, linked) {
				return
			}
			os.Remove(dst)
		}
	}
}
func HasPinnedReport(dir, id string) bool {
	if !ValidID(id) {
		return false
	}
	for _, name := range reportNames(id) {
		if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}
func ReleaseReportPin(dir, id string) {
	if !ValidID(id) {
		return
	}
	for _, name := range reportNames(id) {
		os.Remove(filepath.Join(dir, name))
	}
}

// Publication can rewrite the magic and the small title slots. Compressed
// frames are append-only. Freeze just those two mutable regions and the length
// before validating and streaming any member, even if publication proceeds.
type reportView struct {
	reader *io.SectionReader
	prefix [12]byte
	tail   []byte
	tailAt int64
}

func (r *reportView) ReadAt(dst []byte, off int64) (int, error) {
	n, err := r.reader.ReadAt(dst, off)
	if off >= 0 && off < 12 {
		copy(dst[:n], r.prefix[off:])
	}
	start := max(off, r.tailAt)
	end := min(off+int64(n), r.tailAt+int64(len(r.tail)))
	if end > start {
		copy(dst[start-off:end-off], r.tail[start-r.tailAt:end-r.tailAt])
	}
	return n, err
}
func indexMatchesPayload(r io.ReaderAt, m manifest) bool {
	at := int64(12)
	for i := 0; i < m.Blocks; i++ {
		actual, e := readDescriptor(r, at)
		indexed, err := readDescriptor(r, m.IndexOffset+int64(i)*descriptorSize)
		if e != nil || err != nil || actual != indexed {
			return false
		}
		at += descriptorSize + int64(actual.Stored)
	}
	return at == m.IndexOffset
}

// A truncated publication has no directory footer. Only the report worker
// needs this fallback. Identify it by its first authenticated compressed block.
func orphanReportPrefix(root, id string) (string, error) {
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var match string
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.EqualFold(filepath.Ext(entry.Name()), ".g2r") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		if _, err := metadataAt(path); err == nil {
			continue
		}
		f, _, err := regularFilm(path)
		if err != nil {
			continue
		}
		ok := false
		var prefix [12]byte
		if _, err := f.ReadAt(prefix[:], 0); err == nil && (bytes.Equal(prefix[:], publicPrefix) || bytes.Equal(prefix[:], blockPrefix)) {
			first, err := readDescriptor(f, 12)
			if err == nil && first.valid(true) {
				raw, err := decodeFrame(f, frame{first, 12, 0})
				var start recordedStart
				ok = err == nil && json.Unmarshal(raw, &start) == nil && start.Header.ID == id && len(start.Launch) != 0
			}
		}
		f.Close()
		if ok {
			if match != "" {
				return "", fmt.Errorf("multiple interrupted publications claim film %s", id)
			}
			match = entry.Name()
		}
	}
	return match, nil
}

// CopyReportFilm validates a bounded, fixed-length view of the recording. Never
// chase an arbitrary path from a diagnostic journal or publish a private prefix.
func CopyReportFilm(z *zip.Writer, root, id string) (status string, operations int, err error) {
	if !ValidID(id) {
		return "No recording was identified for this session.", 0, nil
	}
	names := reportPaths(root, id)
	available := false
	for _, name := range names {
		if _, err := os.Lstat(filepath.Join(root, name)); !os.IsNotExist(err) {
			available = true
			break
		}
	}
	if !available {
		name, err := orphanReportPrefix(root, id)
		if err != nil {
			return "", 0, err
		}
		if name != "" {
			names = append(names, name)
		}
	}
	for _, name := range names {
		f, info, e := regularFilm(filepath.Join(root, name))
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return "", 0, e
		}
		defer f.Close()
		r := &reportView{reader: io.NewSectionReader(f, 0, info.Size())}
		if _, e = r.reader.ReadAt(r.prefix[:], 0); e != nil {
			return "", 0, e
		}
		if bytes.Equal(r.prefix[:], publicPrefix) && info.Size() >= pubTailSize {
			r.tailAt = info.Size() - pubTailSize
			r.tail = make([]byte, pubTailSize)
			if _, e = r.reader.ReadAt(r.tail, r.tailAt); e != nil {
				return "", 0, e
			}
		}
		private := bytes.Equal(r.prefix[:], privatePrefix)
		if !private && !bytes.Equal(r.prefix[:], publicPrefix) {
			if e = checkPrefix(r); e != nil {
				return "", 0, e
			}
		}
		first, e := readDescriptor(r, 12)
		if e != nil || !first.valid(true) {
			return "", 0, invalidBlock
		}
		raw, e := decodeFrame(r, frame{first, 12, 0})
		if e != nil {
			return "", 0, e
		}
		var start recordedStart
		if json.Unmarshal(raw, &start) != nil || start.Header.ID != id || len(start.Launch) == 0 {
			return "", 0, invalidBlock
		}
		blocks, n, valid, stop := inspectPrefix(r)
		if blocks == 0 {
			return "", 0, fmt.Errorf("no valid initialization: %w", stop)
		}
		var reader io.ReaderAt = r
		if private {
			reader = originalPrefixReader{r}
		}
		film, full := openBlocks(reader, info.Size(), "")
		member := "mission-prefix.g2r"
		status = fmt.Sprintf("Verified contiguous initialization and %d operation blocks; no verified ending: %v", blocks-1, stop)
		if full == nil && blocks == film.blocks.manifest.Blocks && valid == film.blocks.manifest.IndexOffset && n == film.OperationCount() && indexMatchesPayload(reader, film.blocks.manifest) {
			valid = info.Size()
			member = "mission.g2r"
			status = "Complete published recording; index and compressed blocks verified."
			if private {
				status = "Complete private sealed recording; index and compressed blocks verified. No publication or career receipt."
			}
		}
		entry, e := z.CreateHeader(&zip.FileHeader{Name: member, Method: zip.Store})
		if e != nil {
			return "", 0, errors.Join(ErrReportWrite, e)
		}
		_, e = io.CopyN(entry, io.NewSectionReader(r, 0, valid), valid)
		if e != nil {
			return "", 0, errors.Join(ErrReportWrite, e)
		}
		return status, n, nil
	}
	return "Recording disabled, unavailable, or no longer present.", 0, nil
}
