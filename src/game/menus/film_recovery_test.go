package menus

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/replay"
)

func TestRecoveryRequiresUniqueEntireOverviewDomain(t *testing.T) {
	count := 0
	x, y, e := uniqueRecoveryCursor(func(x, y int) (bool, error) { count++; return x == 31 && y == 130, nil })
	if e != nil || x != 31 || y != 130 || count != 30305 {
		t.Fatalf("unique domain %d/%d visits%d: %v", x, y, count, e)
	}
	for _, mode := range []string{"none", "ambiguous"} {
		visits := 0
		_, _, e := uniqueRecoveryCursor(func(x, y int) (bool, error) {
			visits++
			return mode == "ambiguous" && y == 10 && (x == 10 || x == 11), nil
		})
		if e == nil || visits != 30305 {
			t.Fatalf("%s accepted or not exhausted", mode)
		}
	}
	marker := errors.New("failed consumer")
	if _, _, e := uniqueRecoveryCursor(func(x, y int) (bool, error) { return false, marker }); !errors.Is(e, marker) {
		t.Fatal("consumer failure hidden")
	}
}
func TestRecoverPreservedFullWitness(t *testing.T) {
	path := os.Getenv("GS2000_TEST_RECOVERY_FILM")
	if path == "" {
		t.Skip("explicit preserved-film copy opt-in")
	}
	data, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	before := replay.BytesDigest(data)
	a, e := assets.LoadFrontEndAssets(os.Getenv("GS2000_TEST_MISSIONS"))
	if e != nil {
		t.Fatal(e)
	}
	opt := configuration.Defaults()
	f, e := replay.Unmarshal(data)
	if e != nil {
		t.Fatal(e)
	}
	var r *replay.RecoveryReceipt
	if existing := os.Getenv("GS2000_TEST_RECOVERY_RECEIPT"); existing != "" {
		r, e = replay.ReadRecovery(existing)
	} else {
		r, e = RecoverLegacyFilm(a, data, opt)
	}
	if e != nil {
		t.Fatal(e)
	}
	// These are independent Step43 observations, never values supplied to search.
	if r.Operation != 995 || r.CursorX != 31 || r.CursorY != 130 || r.Steps != 9974 || r.Events != 23 || r.Operations != 13505 {
		t.Fatalf("wrong recovered witness %+v", r)
	}
	q, e := NewRecoveredFilmPlayback(a, data, r, opt)
	if e != nil {
		t.Fatal(e)
	}
	if q.Cursor != 0 || q.Engine.Store != nil || !q.recoveryVerified {
		t.Fatal("fresh verified isolated runtime missing")
	}
	if _, ok := q.Summary(); !ok {
		t.Fatal("whole verified historical summary not retained")
	}
	if _, e := RewindFilmPlayback(a, q, opt); e != nil {
		t.Fatal(e)
	}
	for q.Cursor < 995 {
		if e = q.Step(); e != nil {
			t.Fatal(e)
		}
	}
	q.Preview().GPS.Low = true
	if _, e = recoveryBoundary(q); e == nil {
		t.Fatal("detail-view omission admitted")
	}
	q.Preview().GPS.Low = false
	if e = q.Step(); e != nil {
		t.Fatal("verified bridge failed", e)
	}
	q.recoveryVerified = false
	if e = q.Resume(); e == nil {
		t.Fatal("unverified recovery resumed")
	}
	q.recoveryVerified = true
	if e = q.Resume(); e != nil {
		t.Fatal(e)
	}
	if q.Engine.Store != nil || q.Engine.Mission.ResultLaunch != nil {
		t.Fatal("practice acquired result/save capability")
	}
	wrong := *r
	wrong.FilmSHA256 = strings.Repeat("0", 64)
	if _, e = NewRecoveredFilmPlayback(a, data, &wrong, opt); e == nil {
		t.Fatal("wrong-hash receipt accepted")
	}
	raw, _ := json.Marshal(f)
	var broken replay.Film
	if e = json.Unmarshal(raw, &broken); e != nil {
		t.Fatal(e)
	}
	broken.Operations[1000].Checkpoint["flight"] = strings.Repeat("0", 64)
	b, e := replay.Marshal(&broken)
	if e != nil {
		t.Fatal(e)
	}
	later := *r
	later.FilmSHA256 = replay.BytesDigest(b)
	later.ContentSHA256, _ = replay.RecoveryContentIdentity(&broken)
	if _, e = NewRecoveredFilmPlayback(a, b, &later, opt); e == nil || !strings.Contains(e.Error(), "operation1000") {
		t.Fatalf("receipt trusted beyond recovered point: %v", e)
	}
	s := replay.Store{Root: t.TempDir()}
	if e = os.WriteFile(filepath.Join(s.Root, f.Header.ID+".g2r"), data, 0600); e != nil {
		t.Fatal(e)
	}
	rp, _ := s.RecoveryPath(f.Header.ID)
	if e = replay.WriteRecoveryNew(rp, r); e != nil {
		t.Fatal(e)
	}
	if _, e = LoadManagedFilmPlayback(a, s, f.Header.ID, opt); e != nil {
		t.Fatal(e)
	}
	// The retained witness predates single-file publication. Normal maintenance
	// binds its independently verified receipt before a mutable title is allowed.
	if e = s.MigrateCompanions(); e != nil {
		t.Fatal(e)
	}
	if e = s.ReconcileNames(); e != nil {
		t.Fatal(e)
	}
	if e = s.Rename(f.Header.ID, "Recovered mission"); e != nil {
		t.Fatal(e)
	}
	if _, e = LoadManagedFilmPlayback(a, s, f.Header.ID, opt); e != nil {
		t.Fatal("renamed film recovery failed", e)
	}
	after, e := os.ReadFile(path)
	if e != nil || !bytes.Equal(after, data) || replay.BytesDigest(after) != before {
		t.Fatal("preserved input modified")
	}
	if out := os.Getenv("GS2000_TEST_RECOVERY_OUTPUT"); out != "" {
		b, _ := replay.MarshalRecovery(r)
		if e = os.WriteFile(out, b, 0600); e != nil {
			t.Fatal(e)
		}
	}
}
func TestOldSimpleFilmNeedsNoRecoveryReceipt(t *testing.T) {
	path := os.Getenv("GS2000_TEST_RECOVERY_SHORT_FILM")
	if path == "" {
		t.Skip("explicit historical simple-film opt-in")
	}
	data, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	f, e := replay.Unmarshal(data)
	if e != nil {
		t.Fatal(e)
	}
	a, e := assets.LoadFrontEndAssets(os.Getenv("GS2000_TEST_MISSIONS"))
	if e != nil {
		t.Fatal(e)
	}
	s := replay.Store{Root: t.TempDir()}
	if e = os.WriteFile(filepath.Join(s.Root, f.Header.ID+".g2r"), data, 0600); e != nil {
		t.Fatal(e)
	}
	q, e := LoadManagedFilmPlayback(a, s, f.Header.ID, configuration.Defaults())
	if e != nil {
		t.Fatal(e)
	}
	if q.Cursor != 0 {
		t.Fatal("strict prepass did not return a fresh playback")
	}
	if summary, ok := q.Summary(); !ok || summary.Score != f.End.Score {
		t.Fatal("old summary unavailable on first frame")
	}
	if q.verifiedSummaryContent == "" {
		t.Fatal("historical summary is not bound to immutable content")
	}
	rewound, err := RewindFilmPlayback(a, q, configuration.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if summary, ok := rewound.Summary(); !ok || summary.Score != f.End.Score || rewound.Cursor != 0 {
		t.Fatal("rewind lost verified historical summary")
	}
	q.Film.End.Score++
	if _, err := RewindFilmPlayback(a, q, configuration.Defaults()); err == nil {
		t.Fatal("mutable content reused a stale historical summary")
	}
	q.Film.End.Score--
	for !q.Done {
		if e = q.Step(); e != nil {
			t.Fatal(e)
		}
	}
	if q.Recovery != nil {
		t.Fatal("strict playback fabricated recovery")
	}
	if _, e = RecoverLegacyFilm(a, data, configuration.Defaults()); e == nil || !strings.Contains(e.Error(), "already verifies") {
		t.Fatalf("unnecessary recovery accepted: %v", e)
	}
}

func TestUnrecoveredManagedFullFilmRejectsWithoutSearch(t *testing.T) {
	path := os.Getenv("GS2000_TEST_RECOVERY_FILM")
	if path == "" {
		t.Skip("explicit preserved-film copy opt-in")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	f, err := replay.Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	a, err := assets.LoadFrontEndAssets(os.Getenv("GS2000_TEST_MISSIONS"))
	if err != nil {
		t.Fatal(err)
	}
	s := replay.Store{Root: t.TempDir()}
	copyPath := filepath.Join(s.Root, f.Header.ID+".g2r")
	if err = os.WriteFile(copyPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadManagedFilmPlayback(a, s, f.Header.ID, configuration.Defaults()); err == nil || !strings.Contains(err.Error(), "operation995") || !strings.Contains(err.Error(), "explicit films recover") {
		t.Fatalf("unrecovered failure was hidden or searched: %v", err)
	}
	entries, err := os.ReadDir(s.Root)
	if err != nil || len(entries) != 1 || entries[0].Name() != f.Header.ID+".g2r" {
		t.Fatal("ordinary playback wrote recovery or other metadata")
	}
	after, err := os.ReadFile(copyPath)
	if err != nil || !bytes.Equal(after, data) {
		t.Fatal("ordinary historical verification changed film")
	}
}
