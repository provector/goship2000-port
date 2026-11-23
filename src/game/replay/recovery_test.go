package replay

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func recoveryTestFilm(t *testing.T) (*Film, []byte, *RecoveryReceipt) {
	t.Helper()
	groups := map[string]string{}
	for _, k := range []string{"flight", "mission", "combat", "navigation", "environment", "reference", "pending-hit", "palette", "visual"} {
		groups[k] = strings.Repeat("a", 64)
	}
	f := &Film{Header: Header{Version: Version, ID: strings.Repeat("1", 32), Title: "Old film", Simulation: "native-non-ai-stepO-v1", InputSchema: "logical-operations-v1", InitializationSchema: "accepted-native-launch-v1", ClockPolicy: "test", Resources: strings.Repeat("b", 64), Complete: true}, Launch: []byte(`{}`), Operations: []Operation{{Kind: "boundary", Payload: []byte(`{"Kind":"gps"}`), Checkpoint: groups}, {Kind: "end", Payload: []byte(`{"Reason":"test"}`), Checkpoint: groups}}, End: End{Checkpoint: groups}}
	b, e := Marshal(f)
	if e != nil {
		t.Fatal(e)
	}
	content, e := RecoveryContentIdentity(f)
	if e != nil {
		t.Fatal(e)
	}
	r := &RecoveryReceipt{Version: 1, Profile: RecoveryProfile, Domain: RecoveryDomain, FilmID: f.Header.ID, SourceSHA256: BytesDigest(b), FilmSHA256: BytesDigest(b), ContentSHA256: content, Simulation: f.Header.Simulation, InputSchema: f.Header.InputSchema, Resources: f.Header.Resources, Operation: 0, CursorX: 31, CursorY: 130, Operations: 2, EndCheckpoint: groups}
	return f, b, r
}
func TestRecoveryReceiptStrictIdentityAndPublication(t *testing.T) {
	f, b, r := recoveryTestFilm(t)
	if e := r.Matches(b, f); e != nil {
		t.Fatal(e)
	}
	raw, e := MarshalRecovery(r)
	if e != nil {
		t.Fatal(e)
	}
	for _, bad := range [][]byte{append(append([]byte(nil), raw...), []byte(`{}`)...), bytes.Replace(raw, []byte(`"Version": 1`), []byte(`"Version": 2`), 1), bytes.Replace(raw, []byte(`"CursorX": 31`), []byte(`"CursorX": 9`), 1), bytes.Replace(raw, []byte(`"CursorY": 130`), []byte(`"CursorY": 155`), 1), bytes.Replace(raw, []byte(`"Version": 1`), []byte(`"Extra":true,"Version":1`), 1)} {
		if _, e := UnmarshalRecovery(bad); e == nil {
			t.Fatal("accepted malformed/unsupported recovery")
		}
	}
	changed := append([]byte(nil), b...)
	changed[20] ^= 1
	if e := r.Matches(changed, f); e == nil {
		t.Fatal("accepted wrong film hash")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "receipt.json")
	if e := WriteRecoveryNew(target, r); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(target)
	if e := WriteRecoveryNew(target, r); e == nil {
		t.Fatal("replaced existing output")
	}
	after, _ := os.ReadFile(target)
	if !bytes.Equal(before, after) {
		t.Fatal("existing receipt overwritten")
	}
	if _, e := ReadRecovery(target); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(target, filepath.Join(dir, "linked.json")); e == nil {
		if _, e := ReadRecovery(filepath.Join(dir, "linked.json")); e == nil {
			t.Fatal("accepted symlink receipt")
		}
	}
}
func TestManagedRecoveredRenameAndDelete(t *testing.T) {
	f, b, r := recoveryTestFilm(t)
	s := Store{Root: t.TempDir()}
	if e := os.WriteFile(filepath.Join(s.Root, f.Header.ID+".g2r"), b, 0600); e != nil {
		t.Fatal(e)
	}
	path, e := s.RecoveryPath(f.Header.ID)
	if e != nil {
		t.Fatal(e)
	}
	if e = WriteRecoveryNew(path, r); e != nil {
		t.Fatal(e)
	}
	if e = s.MigrateCompanions(); e != nil {
		t.Fatal(e)
	}
	if e = s.Rename(f.Header.ID, "Retained film"); e != nil {
		t.Fatal(e)
	}
	got, e := s.LoadRecovery(f.Header.ID)
	if e != nil {
		t.Fatal(e)
	}
	renamed, e := s.Read(f.Header.ID)
	if e != nil {
		t.Fatal(e)
	}
	ff, e := s.Load(f.Header.ID)
	if e != nil {
		t.Fatal(e)
	}
	if e = got.Matches(renamed, ff); e != nil {
		t.Fatal(e)
	}
	if got.SourceSHA256 != r.SourceSHA256 || got.ContentSHA256 != r.ContentSHA256 || got.FilmSHA256 != r.FilmSHA256 || bytes.Equal(b, renamed) || ff.Header.Title != "Retained film" {
		t.Fatal("rename changed immutable content/receipt or lost the new title")
	}
	if e = s.Delete(f.Header.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("owned recovery sidecar not removed")
	}
}
func TestRecoveryRejectsMissingAndCorruptOptionalReceipt(t *testing.T) {
	_, b, r := recoveryTestFilm(t)
	s := Store{Root: t.TempDir()}
	if _, e := s.LoadRecovery(r.FilmID); !os.IsNotExist(e) {
		t.Fatal("missing optional receipt not distinguishable")
	}
	p, _ := s.RecoveryPath(r.FilmID)
	if e := os.WriteFile(p, []byte(`{"Version":1}`), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := s.LoadRecovery(r.FilmID); e == nil {
		t.Fatal("corrupt receipt accepted")
	}
	if e := os.WriteFile(filepath.Join(s.Root, r.FilmID+".g2r"), b, 0600); e != nil {
		t.Fatal(e)
	}
	if e := s.Rename(r.FilmID, "No change"); e == nil {
		t.Fatal("renamed through corrupt receipt")
	}
	got, _ := s.Read(r.FilmID)
	if !bytes.Equal(got, b) {
		t.Fatal("failed rename changed film")
	}
}
