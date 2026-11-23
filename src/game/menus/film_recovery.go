package menus

import (
	"bytes"
	"fmt"
	"os"

	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/replay"
)

func recoveryBoundary(q *FilmPlayback) (filmBoundary, error) {
	var b filmBoundary
	if q == nil || q.Preview() == nil || q.Cursor < 0 || q.Cursor >= q.Film.OperationCount() || q.Film.Header.Simulation != "native-non-ai-stepO-v1" || q.Film.Header.InputSchema != "logical-operations-v1" {
		return b, fmt.Errorf("recovery requires the historical Step O profile")
	}
	op, e := q.Film.OperationAt(q.Cursor)
	if e != nil {
		return b, e
	}
	if op.Kind != "boundary" || len(op.Events) != 0 {
		return b, fmt.Errorf("recovery only supports a no-event GPS append boundary")
	}
	if e := decodeFilmPayload(op.Payload, &b); e != nil {
		return b, e
	}
	a := b.Action
	p := q.Preview()
	g := p.GPS
	if b.Kind != "gps" || b.Hit != nil || b.Count != 0 || g == nil || g.Low || !g.Editing || g.Selected != 0 || g.Moving != 0 || p.Flight.Navigation[0].Index() < 0 || p.Flight.Navigation[0].Index() > 2 {
		return b, fmt.Errorf("missing GPS state is outside the known overview append domain")
	}
	// Only Selector1's append is admitted. No pan/detail/other command may be
	// hidden inside the operation whose missing cursor is being reconstructed.
	if !a.Selector1 || a.Selector2 || a.Cancel || a.Backspace || a.ViewKey != 0 || a.GPSKey != "" || a.SceneDirections != [4]bool{} || a.Left || a.Right || a.Up || a.Down || a.PointerReset || a.PointerDX != 0 || a.PointerDY != 0 || !a.PointerRelative {
		return b, fmt.Errorf("GPS append has unsupported simultaneous input")
	}
	return b, nil
}
func (q *FilmPlayback) applyVerifiedRecovery() error {
	if q.Recovery == nil || q.Cursor != q.Recovery.Operation {
		return nil
	}
	if e := q.Recovery.Validate(); e != nil {
		return e
	}
	if _, e := recoveryBoundary(q); e != nil {
		return e
	}
	// The only reconstructed value is the source-bounded, previously unrecorded
	// modal cursor. No route, pose, RNG, timing or expected checkpoint is assigned.
	q.Preview().GPS.CursorX = q.Recovery.CursorX
	q.Preview().GPS.CursorY = q.Recovery.CursorY
	return nil
}
func verifyRecoveredEnding(q *FilmPlayback, r *replay.RecoveryReceipt) error {
	if !q.Done || q.Divergence != "" || q.Cursor != q.Film.OperationCount() || q.reachedSteps != r.Steps || q.EventCount != r.Events {
		return fmt.Errorf("recovered film did not verify the complete stream")
	}
	if group := compareFilmCheckpoint(r.EndCheckpoint, q.Preview().FilmCheckpoint()); group != "" {
		return fmt.Errorf("recovered ending %s differs", group)
	}
	return nil
}
func cloneRecovery(r *replay.RecoveryReceipt) *replay.RecoveryReceipt {
	b, _ := replay.MarshalRecovery(r)
	v, _ := replay.UnmarshalRecovery(b)
	return v
}

// RecoverLegacyFilm is deliberately explicit and CPU-bounded. Ordinary PLAY
// never invokes this search. One supported missing cursor must match every
// untouched old checkpoint, then every remaining operation/end must verify.
// uniqueRecoveryCursor exhausts the complete documented source domain even
// after finding a match. Choosing the first hit would hide ambiguous receipts.
func uniqueRecoveryCursor(match func(int, int) (bool, error)) (int, int, error) {
	matches, mx, my := 0, 0, 0
	for y := 10; y <= 154; y++ {
		for x := 10; x <= 218; x++ {
			ok, err := match(x, y)
			if err != nil {
				return 0, 0, err
			}
			if ok {
				matches++
				mx, my = x, y
			}
		}
	}
	if matches != 1 {
		return 0, 0, fmt.Errorf("overview recovery requires exactly one matching cursor; found%d", matches)
	}
	return mx, my, nil
}

func RecoverLegacyFilm(a *assetdata.OfficeAssets, data []byte, options configuration.RuntimeOptions) (*replay.RecoveryReceipt, error) {
	f, e := replay.Unmarshal(data)
	if e != nil {
		return nil, e
	}
	return RecoverLegacySource(a, f, replay.BytesDigest(data), options)
}
func RecoverLegacySource(a *assetdata.OfficeAssets, f *replay.Film, digest string, options configuration.RuntimeOptions) (*replay.RecoveryReceipt, error) {
	if f.Header.Simulation != "native-non-ai-stepO-v1" || f.Header.InputSchema != "logical-operations-v1" {
		return nil, fmt.Errorf("only historical Step O films need this recovery")
	}
	q, e := NewFilmPlayback(a, f, options)
	if e != nil {
		return nil, e
	}
	for !q.Done {
		if e = q.Step(); e != nil {
			break
		}
	}
	if q.Divergence == "" {
		return nil, fmt.Errorf("film already verifies strictly; no recovery is needed")
	}
	target := q.Cursor
	q, e = NewFilmPlayback(a, f, options)
	if e != nil {
		return nil, e
	}
	for q.Cursor < target {
		if e = q.Step(); e != nil {
			return nil, fmt.Errorf("recovery prefix changed: %w", e)
		}
	}
	b, e := recoveryBoundary(q)
	if e != nil {
		return nil, e
	}
	p := q.Preview()
	g := *p.GPS
	state := *p.Flight.State
	routes := p.Flight.Navigation
	held := p.Flight.gpsHeld
	op, e := f.OperationAt(target)
	if e != nil {
		return nil, e
	}
	mx, my, e := uniqueRecoveryCursor(func(x, y int) (bool, error) {
		p.Flight.Navigation = routes
		*p.Flight.State = state
		z := g
		z.Routes = &p.Flight.Navigation
		p.GPS = &z
		p.GPS.CursorX = x
		p.GPS.CursorY = y
		p.Flight.gpsHeld = held
		q.events = nil
		if err := q.Engine.applyFilmBoundary(b); err != nil {
			return false, err
		}
		return compareFilmCheckpoint(op.Checkpoint, p.FilmCheckpoint()) == "" && bytes.Equal(filmJSON(op.Events), filmJSON(q.events)), nil
	})
	if e != nil {
		return nil, e
	}
	content, e := replay.RecoveryContentIdentity(f)
	if e != nil {
		return nil, e
	}
	events, e := f.EventCount()
	if e != nil {
		return nil, e
	}
	r := &replay.RecoveryReceipt{Version: replay.RecoveryVersion, Profile: replay.RecoveryProfile, Domain: replay.RecoveryDomain, FilmID: f.Header.ID, SourceSHA256: digest, FilmSHA256: digest, ContentSHA256: content, Simulation: f.Header.Simulation, InputSchema: f.Header.InputSchema, Resources: f.Header.Resources, Operation: target, CursorX: mx, CursorY: my, Operations: f.OperationCount(), Steps: f.End.Steps, Events: events, EndCheckpoint: f.End.Checkpoint}
	if _, e = NewRecoveredSource(a, f, digest, r, options); e != nil {
		return nil, fmt.Errorf("candidate failed complete verification: %w", e)
	}
	return r, nil
}

// NewRecoveredFilmPlayback treats even a well-formed persisted receipt as
// untrusted. It first verifies all operations/end with that one bridge and only
// then returns a fresh isolated session eligible for normal controls/Resume.
func NewRecoveredFilmPlayback(a *assetdata.OfficeAssets, data []byte, r *replay.RecoveryReceipt, options configuration.RuntimeOptions) (*FilmPlayback, error) {
	f, e := replay.Unmarshal(data)
	if e != nil {
		return nil, e
	}
	return NewRecoveredSource(a, f, replay.BytesDigest(data), r, options)
}
func NewRecoveredSource(a *assetdata.OfficeAssets, f *replay.Film, digest string, r *replay.RecoveryReceipt, options configuration.RuntimeOptions) (*FilmPlayback, error) {
	if e := r.MatchesDigest(digest, f); e != nil {
		return nil, e
	}
	q, e := NewFilmPlayback(a, f, options)
	if e != nil {
		return nil, e
	}
	q.Recovery = cloneRecovery(r)
	for !q.Done {
		if e = q.Step(); e != nil {
			return nil, e
		}
	}
	if e = verifyRecoveredEnding(q, r); e != nil {
		return nil, e
	}
	summary, _ := q.Summary()
	out, e := NewFilmPlayback(a, f, options)
	if e != nil {
		return nil, e
	}
	out.Recovery = cloneRecovery(r)
	out.recoveryVerified = true
	out.verifiedSummaryContent = r.ContentSHA256
	if summary != nil {
		v := *summary
		out.summary = &v
	}
	return out, nil
}

// Historical O containers lack the frozen sidebar fields. A strict bounded
// prepass is therefore necessary before showing that summary on the first
// frame. This never searches missing inputs or writes a film/sidecar; a mismatch
// remains a precise failure directing the user to explicit recovery on a copy.
func newVerifiedLegacyPlayback(a *assetdata.OfficeAssets, f *replay.Film, options configuration.RuntimeOptions) (*FilmPlayback, error) {
	q, err := NewFilmPlayback(a, f, options)
	if err != nil {
		return nil, err
	}
	for !q.Done {
		if err = q.Step(); err != nil {
			return nil, fmt.Errorf("historical film failed strict verification: %w; use explicit films recover on a copy for supported omitted overview input", err)
		}
	}
	events, e := f.EventCount()
	if e != nil {
		return nil, e
	}
	if q.Cursor != f.OperationCount() || q.reachedSteps != f.End.Steps || q.EventCount != events {
		return nil, fmt.Errorf("historical film ending counts differ")
	}
	summary, ok := q.Summary()
	if !ok {
		return nil, fmt.Errorf("historical film has no verified frozen summary")
	}
	content, err := replay.RecoveryContentIdentity(f)
	if err != nil {
		return nil, err
	}
	out, err := NewFilmPlayback(a, f, options)
	if err != nil {
		return nil, err
	}
	out.summary = summary
	out.verifiedSummaryContent = content
	return out, nil
}

func LoadManagedFilmPlayback(a *assetdata.OfficeAssets, s replay.Store, id string, options configuration.RuntimeOptions) (*FilmPlayback, error) {
	return loadSelectedFilmPlayback(a, s, replay.Entry{ID: id}, options)
}
func loadSelectedFilmPlayback(a *assetdata.OfficeAssets, s replay.Store, entry replay.Entry, options configuration.RuntimeOptions) (*FilmPlayback, error) {
	id := entry.ID
	f, e := s.LoadEntry(entry)
	if e != nil {
		return nil, e
	}
	r, e := s.LoadRecovery(id)
	if os.IsNotExist(e) {
		if f.Legacy() {
			return newVerifiedLegacyPlayback(a, f, options)
		}
		return NewFilmPlayback(a, f, options)
	}
	if e != nil {
		return nil, fmt.Errorf("recovery receipt is unusable: %w", e)
	}
	digest, e := s.Digest(id)
	if e != nil {
		return nil, e
	}
	return NewRecoveredSource(a, f, digest, r, options)
}
func RewindFilmPlayback(a *assetdata.OfficeAssets, q *FilmPlayback, options configuration.RuntimeOptions) (*FilmPlayback, error) {
	if q == nil {
		return nil, fmt.Errorf("no film to rewind")
	}
	if q.Recovery == nil {
		if q.Film.Legacy() {
			if q.verifiedSummaryContent == "" {
				return newVerifiedLegacyPlayback(a, q.Film, options)
			}
			content, err := replay.RecoveryContentIdentity(q.Film)
			if err != nil {
				return nil, err
			}
			if content != q.verifiedSummaryContent || q.summary == nil {
				return nil, fmt.Errorf("verified historical film content changed")
			}
			out, err := NewFilmPlayback(a, q.Film, options)
			if err != nil {
				return nil, err
			}
			v := *q.summary
			out.summary = &v
			out.verifiedSummaryContent = content
			return out, nil
		}
		return NewFilmPlayback(a, q.Film, options)
	}
	if !q.recoveryVerified {
		return nil, fmt.Errorf("recovery has not verified the complete film")
	}
	// In-memory immutable content is compared canonically; source raw bytes may
	// have been formatted differently. No search or altered checkpoint is used.
	content, e := replay.RecoveryContentIdentity(q.Film)
	if e != nil {
		return nil, e
	}
	if e = q.Recovery.Validate(); e != nil {
		return nil, e
	}
	if content != q.Recovery.ContentSHA256 {
		return nil, fmt.Errorf("recovered film content changed")
	}
	out, e := NewFilmPlayback(a, q.Film, options)
	if e != nil {
		return nil, e
	}
	out.Recovery = cloneRecovery(q.Recovery)
	out.recoveryVerified = true
	out.verifiedSummaryContent = content
	if q.summary != nil {
		v := *q.summary
		out.summary = &v
	}
	return out, nil
}

// RecoveryDescription is player-facing metadata, not a successful-result flag.
func RecoveryDescription(r *replay.RecoveryReceipt) string {
	if r == nil {
		return ""
	}
	return fmt.Sprintf("Verified overview recovery at operation%d", r.Operation)
}
