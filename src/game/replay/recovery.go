package replay

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const RecoveryVersion = 1
const RecoveryProfile = "stepO-overview-cursor-recovery-v1"
const RecoveryDomain = "gps-overview-cursor-x10..218-y10..154"
const MaxRecoveryBytes = 32 << 10

// RecoveryReceipt is a bounded, explicit interpretation of omitted Step O UI
// input, never a replacement checkpoint or a career repair. No persisted bool
// is authority to Resume: menus verifies the whole original stream on load.
type RecoveryReceipt struct {
	Version                                         int
	Profile, Domain                                 string
	FilmID, SourceSHA256, FilmSHA256, ContentSHA256 string
	Simulation, InputSchema, Resources              string
	Operation, CursorX, CursorY                     int
	Operations                                      int
	Steps, Events                                   uint64
	EndCheckpoint                                   map[string]string
}

func ValidDigest(s string) bool {
	if len(s) != 64 {
		return false
	}
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == sha256.Size && s == hex.EncodeToString(b)
}
func BytesDigest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// RecoveryContentIdentity excludes only the user-editable display title. Every
// initialization, operation, event, checkpoint and ending byte value remains in
// the canonical container identity; a rename cannot authorize different input.
func RecoveryContentIdentity(f *Film) (string, error) {
	if f == nil {
		return "", fmt.Errorf("missing recovery film")
	}
	if f.BlockStorage() {
		m := f.blocks.manifest
		m.Header = f.Header
		m.End = f.End
		return frameIdentity(m), nil
	}
	clone := *f
	clone.Header.Title = "Recovery identity"
	b, e := Marshal(&clone)
	if e != nil {
		return "", e
	}
	m, _, _, e := readManifest(bytes.NewReader(b), int64(len(b)))
	if e != nil {
		return "", e
	}
	logical := frameIdentity(m)
	if logical == f.snapshotLogical {
		return f.snapshotStored, nil
	}
	return logical, nil
}
func (r *RecoveryReceipt) Validate() error {
	if r == nil || r.Version != RecoveryVersion || r.Profile != RecoveryProfile || r.Domain != RecoveryDomain {
		return fmt.Errorf("unsupported recovery receipt")
	}
	if !ValidID(r.FilmID) || !ValidDigest(r.SourceSHA256) || !ValidDigest(r.FilmSHA256) || !ValidDigest(r.ContentSHA256) || !ValidDigest(r.Resources) {
		return fmt.Errorf("invalid recovery identity")
	}
	if r.Simulation != "native-non-ai-stepO-v1" || r.InputSchema != "logical-operations-v1" {
		return fmt.Errorf("recovery only supports the historical Step O input profile")
	}
	if r.Operation < 0 || r.Operations < 2 || r.Operations > MaxOperations || r.Operation >= r.Operations-1 || r.CursorX < 10 || r.CursorX > 218 || r.CursorY < 10 || r.CursorY > 154 || r.Steps > MaxOperations || r.Events > uint64(MaxOperations)*64 {
		return fmt.Errorf("recovery receipt exceeds supported bounds")
	}
	if len(r.EndCheckpoint) != 9 {
		return fmt.Errorf("recovery ending groups incomplete")
	}
	for _, group := range []string{"flight", "mission", "combat", "navigation", "environment", "reference", "pending-hit", "palette", "visual"} {
		if !ValidDigest(r.EndCheckpoint[group]) {
			return fmt.Errorf("recovery ending group %s invalid", group)
		}
	}
	return nil
}
func (r *RecoveryReceipt) Matches(data []byte, f *Film) error {
	digest := BytesDigest(data)
	if bytes.HasPrefix(data, publicPrefix) {
		_, _, _, p, e := readContainer(bytes.NewReader(data), int64(len(data)))
		if e != nil {
			return e
		}
		h := sha256.New()
		h.Write(blockPrefix)
		h.Write(data[12:p.core])
		digest = fmt.Sprintf("%x", h.Sum(nil))
	}
	return r.MatchesDigest(digest, f)
}
func (r *RecoveryReceipt) MatchesDigest(digest string, f *Film) error {
	if e := r.Validate(); e != nil {
		return e
	}
	if digest != r.FilmSHA256 {
		return fmt.Errorf("recovery receipt belongs to different film bytes")
	}
	content, e := RecoveryContentIdentity(f)
	if e != nil {
		return e
	}
	if f.Header.ID != r.FilmID || f.Header.Simulation != r.Simulation || f.Header.InputSchema != r.InputSchema || f.Header.Resources != r.Resources || content != r.ContentSHA256 || f.OperationCount() != r.Operations || f.End.Steps != r.Steps {
		return fmt.Errorf("recovery film content/profile differs")
	}
	for k, v := range r.EndCheckpoint {
		if f.End.Checkpoint[k] != v {
			return fmt.Errorf("recovery ending identity differs")
		}
	}
	events, e := f.EventCount()
	if e != nil {
		return e
	}
	if events != r.Events {
		return fmt.Errorf("recovery event identity differs")
	}
	return nil
}
func MarshalRecovery(r *RecoveryReceipt) ([]byte, error) {
	if e := r.Validate(); e != nil {
		return nil, e
	}
	b, e := json.MarshalIndent(r, "", "  ")
	if len(b) > MaxRecoveryBytes {
		return nil, fmt.Errorf("recovery receipt too large")
	}
	return append(b, '\n'), e
}
func UnmarshalRecovery(b []byte) (*RecoveryReceipt, error) {
	if len(b) == 0 || len(b) > MaxRecoveryBytes {
		return nil, fmt.Errorf("recovery receipt size invalid")
	}
	var r RecoveryReceipt
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e := d.Decode(&r); e != nil {
		return nil, e
	}
	if e := d.Decode(new(any)); e != io.EOF {
		return nil, fmt.Errorf("trailing recovery receipt data")
	}
	if e := r.Validate(); e != nil {
		return nil, e
	}
	return &r, nil
}
func ReadRecovery(path string) (*RecoveryReceipt, error) {
	i, e := os.Lstat(path)
	if e != nil {
		return nil, e
	}
	if !i.Mode().IsRegular() || i.Size() > MaxRecoveryBytes {
		return nil, fmt.Errorf("recovery receipt must be a bounded regular file")
	}
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	return UnmarshalRecovery(b)
}

// WriteRecoveryNew publishes an explicitly named sidecar without replacing an
// existing receipt, film, save or symlink. The film remains read-only throughout.
func WriteRecoveryNew(path string, r *RecoveryReceipt) error {
	b, e := MarshalRecovery(r)
	if e != nil {
		return e
	}
	if _, e = os.Lstat(path); e == nil {
		return fmt.Errorf("recovery output already exists")
	} else if !os.IsNotExist(e) {
		return e
	}
	parent := filepath.Dir(path)
	i, e := os.Lstat(parent)
	if e != nil {
		return e
	}
	if !i.IsDir() || i.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("recovery output directory is not a regular directory")
	}
	f, e := os.CreateTemp(parent, ".recovery-*.tmp")
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
	// Hard-link publication is create-once, unlike Rename replacing an existing
	// target. The temporary is removed only after this distinct output exists.
	if e = os.Link(name, path); e != nil {
		return e
	}
	return nil
}
func (s Store) Read(id string) ([]byte, error) {
	p, e := s.resolved(id, false)
	if e != nil {
		return nil, e
	}
	i, e := os.Lstat(p)
	if e != nil {
		return nil, e
	}
	if !i.Mode().IsRegular() || i.Size() > MaxSnapshotBytes {
		return nil, fmt.Errorf("invalid managed film file")
	}
	return os.ReadFile(p)
}
func (s Store) RecoveryPath(id string) (string, error) {
	p, e := s.path(id)
	if e != nil {
		return "", e
	}
	return filepath.Join(filepath.Dir(p), id+".recovery.json"), nil
}
func (s Store) LoadRecovery(id string) (*RecoveryReceipt, error) {
	p, e := s.RecoveryPath(id)
	if e != nil {
		return nil, e
	}
	m, err := s.metadata(id)
	if err != nil {
		return nil, err
	}
	if m.portable != nil {
		if m.portable.pub.Recovery == nil {
			return nil, os.ErrNotExist
		}
		return m.portable.pub.Recovery, nil
	}
	r, e := ReadRecovery(p)
	if e == nil && r.FilmID != id {
		return nil, fmt.Errorf("recovery filename identity differs")
	}
	return r, e
}

func (s Store) replaceRecovery(id string, r *RecoveryReceipt) error {
	p, e := s.RecoveryPath(id)
	if e != nil {
		return e
	}
	b, e := MarshalRecovery(r)
	if e != nil {
		return e
	}
	i, e := os.Lstat(p)
	if e != nil {
		return e
	}
	if !i.Mode().IsRegular() {
		return fmt.Errorf("recovery sidecar is not a regular file")
	}
	f, e := os.CreateTemp(filepath.Dir(p), ".recovery-*.tmp")
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
	return os.Rename(name, p)
}

func (f *Film) EventCount() (uint64, error) {
	var n uint64
	for i := 0; i < f.OperationCount(); i++ {
		op, e := f.OperationAt(i)
		if e != nil {
			return 0, e
		}
		n += uint64(len(op.Events))
	}
	return n, nil
}
func FileDigest(path string) (string, error) {
	file, info, e := regularFilm(path)
	if e != nil {
		return "", e
	}
	defer file.Close()
	h := sha256.New()
	n, e := io.Copy(h, io.LimitReader(file, MaxBytes+1))
	if e != nil {
		return "", e
	}
	if n != info.Size() {
		return "", fmt.Errorf("film changed while hashing")
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
func (s Store) Digest(id string) (string, error) {
	p, e := s.resolved(id, false)
	if e != nil {
		return "", e
	}
	return RecoveryDigest(p)
}

// RecoveryDigest binds a receipt to the immutable compressed film and ending.
// For version 3 the original version-2 prefix is used in this digest domain.
func RecoveryDigest(p string) (string, error) {
	m, e := metadataAt(p)
	if e != nil {
		return "", e
	}
	if m.portable != nil {
		return portableCoreDigest(p, m.portable.core)
	}
	return FileDigest(p)
}
