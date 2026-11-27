package menus

import (
	"gunship2000-assets/game/replay"
	"path/filepath"
	"reflect"
	"testing"
)

// The input is published by the W3.5 companion migrator and then converted.
// Check the original evidence is still bound, even though its container digest changed.
func portableRecoveryFixture(t *testing.T, path string, historical *replay.RecoveryReceipt) (*replay.RecoveryReceipt, string) {
	t.Helper()
	store := replay.Store{Root: filepath.Dir(path)}
	bound, err := store.LoadRecovery(historical.FilmID)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := store.Digest(historical.FilmID)
	if err != nil {
		t.Fatal(err)
	}
	f, err := store.Load(historical.FilmID)
	if err != nil {
		t.Fatal(err)
	}
	if historical.MatchesDigest(digest, f) == nil {
		t.Fatal("old receipt accepted different container bytes")
	}
	if err = bound.MatchesDigest(digest, f); err != nil {
		t.Fatal(err)
	}
	reference := *historical
	reference.FilmSHA256, reference.ContentSHA256 = bound.FilmSHA256, bound.ContentSHA256
	if !reflect.DeepEqual(reference, *bound) {
		t.Fatal("conversion changed recovery witness or original provenance")
	}
	return bound, digest
}
