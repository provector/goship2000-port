package replay

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RecordingNotices inventories only bounded diagnostic sidecars/names. These are
// never Film Library playable rows, and no ending/recovery is inferred here.
func (s Store) RecordingNotices() ([]string, error) {
	names, e := os.ReadDir(s.Root)
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	out := []string{}
	for _, n := range names {
		name := n.Name()
		suffix := ".tmp.failure.json"
		if !strings.HasPrefix(name, ".film-") || !strings.HasSuffix(name, suffix) {
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(name, ".film-"), suffix)
		if !ValidID(id) {
			continue
		}
		p := filepath.Join(s.Root, name)
		info, e := os.Lstat(p)
		if e != nil || !info.Mode().IsRegular() || info.Size() > 8192 {
			continue
		}
		b, e := os.ReadFile(p)
		if e != nil {
			continue
		}
		var f RecordingFailure
		if json.Unmarshal(b, &f) != nil || f.Version != 1 || f.FilmID != id || len(f.Cause) > 4096 {
			continue
		}
		if _, e = s.metadata(id); e == nil {
			continue
		}
		state := "Incomplete recording preserved (not playable)"
		prefix, prefixErr := os.Lstat(filepath.Join(s.Root, ".film-"+id+".tmp"))
		if prefixErr != nil || !prefix.Mode().IsRegular() {
			state = "Recording failure retained; prefix unavailable (not playable)"
		}
		if f.Phase == "verify-unscored" {
			state = "Complete capture failed strict verification (not admitted to playback)"
		} else if f.Phase == "publish-unscored" {
			state = "Complete verified recording retained; Library publication failed"
		}
		out = append(out, fmt.Sprintf("%s: %s. First cause: %s; operation%d, step%d, bytes%d/limit%d. Use explicit film inspection/recovery tools; no ending is inferred.", state, id, f.Cause, f.Operation, f.Step, f.Bytes, f.Limit))
		if len(out) >= 16 {
			break
		}
	}
	for _, n := range names {
		name := n.Name()
		suffix := ".tmp.sealed"
		if !strings.HasPrefix(name, ".film-") || !strings.HasSuffix(name, suffix) {
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(name, ".film-"), suffix)
		if !ValidID(id) {
			continue
		}
		if _, e = s.metadata(id); e == nil {
			continue
		}
		i, e := os.Lstat(filepath.Join(s.Root, name))
		if e != nil || !i.Mode().IsRegular() {
			continue
		}
		out = append(out, "Sealed recording candidate preserved (not yet admitted to Film Library): "+id+". Explicit strict verification/recovery is required; no career changes are inferred.")
		if len(out) >= 16 {
			break
		}
	}
	return out, nil
}
