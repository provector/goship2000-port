package diagnostics

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"gunship2000-assets/internal/branding"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gunship2000-assets/game/replay"
)

const historyBudget = 5_000_000

var currentSession atomic.Pointer[Session]

type MissionContext struct {
	Build, Settings, Mission, Film, Phase, Facts string
	Version, Flavor                              string
	Step                                         uint64
}
type reportRequest struct {
	Context             MissionContext
	Reason, Disposition string
}
type history struct {
	lines []string
	bytes int
}

func (h *history) add(line string) {
	h.lines = append(h.lines, line)
	h.bytes += len(line)
	for h.bytes > historyBudget {
		h.bytes -= len(h.lines[0])
		h.lines[0] = ""
		h.lines = h.lines[1:]
	}
}

type Session struct {
	closed       atomic.Bool
	mu           sync.Mutex
	dir, saves   string
	lastProgress MissionContext // journal worker only
	context      MissionContext
	ring, delta  history
	notices      []string
	warned       map[string]bool
	seen         map[string]bool
	wg           sync.WaitGroup
	stop, done   chan struct{}
}

func saveRecord(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".write-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	e = errors.Join(e, f.Close())
	if e == nil {
		e = publishRename(f.Name(), path)
	}
	return e
}
func publishRename(from, to string) error {
	if err := os.Rename(from, to); err != nil {
		return err
	}
	return syncReportDirectory(filepath.Dir(to))
}

func loadRecord(path string, dst any) error {
	before, e := os.Lstat(path)
	if e != nil {
		return e
	}
	if !before.Mode().IsRegular() {
		return fmt.Errorf("invalid diagnostic companion")
	}

	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return e
	}
	if !os.SameFile(before, st) || st.Size() > 64<<10 {
		return fmt.Errorf("invalid diagnostic record")
	}
	return json.NewDecoder(io.LimitReader(f, 64<<10)).Decode(dst)
}
func StartSession(saves, build, settings string) (*Session, error) {
	session, err := startDiskSession(saves, build, settings)
	if err != nil {
		session = &Session{saves: saves, context: MissionContext{Version: branding.Version, Flavor: branding.Flavor, Build: build, Settings: settings, Phase: "startup"}, seen: map[string]bool{}, warned: map[string]bool{}}
		currentSession.Store(session)
		session.failure("recovery storage", err)
	}
	return session, err
}
func startDiskSession(saves, build, settings string) (*Session, error) {
	root := filepath.Join(saves, "reports")
	if e := os.MkdirAll(root, 0700); e != nil {
		return nil, e
	}
	st, e := os.Lstat(root)
	if e != nil || !st.IsDir() {
		return nil, fmt.Errorf("invalid reports directory")
	}
	dir, e := os.MkdirTemp(root, "session-")
	if e != nil {
		return nil, e
	}
	s := &Session{dir: dir, saves: saves, context: MissionContext{Version: branding.Version, Flavor: branding.Flavor, Build: build, Settings: settings, Phase: "startup"}, seen: map[string]bool{}, warned: map[string]bool{}, stop: make(chan struct{}), done: make(chan struct{})}
	if e = saveRecord(filepath.Join(dir, "active.json"), s.context); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(filepath.Join(dir, "fatal.txt"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e == nil {
		e = debug.SetCrashOutput(f, debug.CrashOptions{})
		f.Close()
	}
	if e != nil {
		s.failure("fatal output", e)
	}
	currentSession.Store(s)
	go s.journalLoop()
	s.wg.Add(1)
	go func() { defer s.wg.Done(); s.recover(root) }()
	return s, nil
}

// ResultAwaitingAcknowledgment is emitted after the recoverable proposal has
// been durably staged. Final commitment and archive publication report separately.
func ResultAwaitingAcknowledgment() {
	if s := currentSession.Load(); s != nil {
		s.notice("Mission completed with a recoverable error. Acknowledge the debrief to save your result.")
	}
}

func (s *Session) notice(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.notices) < 16 {
		s.notices = append(s.notices, msg)
	}
}
func (s *Session) failure(sink string, e error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.warned[sink] {
		return
	}
	s.warned[sink] = true
	line := fmt.Sprintf("Diagnostics %s failed: %v", sink, e)
	s.notices = append(s.notices, line)
	s.ring.add(line + "\n")
	fmt.Fprintln(os.Stderr, line)
}
func recordHistory(line string) {
	if s := currentSession.Load(); s != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.ring.add(line)
		if s.dir != "" {
			s.delta.add(line)
		}
	}
}

// Flush each batch off the UI thread. Two half-budget generations evict old
// complete lines without rewriting the retained diagnostic history.
func (s *Session) flush() {
	s.mu.Lock()
	ctx := s.context
	delta := s.delta
	s.delta = history{}
	s.mu.Unlock()
	if ctx.Step != 0 && ctx != s.lastProgress {
		if e := saveRecord(filepath.Join(s.dir, "progress.json"), ctx); e == nil {
			s.lastProgress = ctx
		} else {
			s.failure("logical progress", e)
		}
	}
	if len(delta.lines) == 0 {
		return
	}
	p := filepath.Join(s.dir, "journal.log")
	f, e := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		s.failure("journal", e)
		return
	}
	st, e := f.Stat()
	var size int64
	if e == nil {
		size = st.Size()
	}
	for i := 0; e == nil && i < len(delta.lines); {
		if size+int64(len(delta.lines[i])) > historyBudget/2 {
			e = f.Close()
			if e == nil {
				e = os.Rename(p, p+".previous")
			}
			if e != nil {
				break
			}
			f, e = os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if e != nil {
				break
			}
			size = 0
		}
		var batch strings.Builder
		for i < len(delta.lines) && batch.Len() < 64<<10 && size+int64(batch.Len()+len(delta.lines[i])) <= historyBudget/2 {
			batch.WriteString(delta.lines[i])
			i++
		}
		_, e = io.WriteString(f, batch.String())
		size += int64(batch.Len())
	}
	if f != nil && e == nil {
		e = f.Sync()
	}
	if f != nil {
		f.Close()
	}
	if e != nil {
		s.failure("journal", e)
	}
}
func (s *Session) journalLoop() {
	defer close(s.done)
	t := time.NewTicker(250 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			s.flush()
			return
		case <-t.C:
			s.flush()
		}
	}
}
func SetMission(mission, film, phase string, step uint64) {
	s := currentSession.Load()
	if s == nil {
		return
	}
	s.mu.Lock()
	s.context.Facts = ""
	s.context.Mission = mission
	s.context.Film = film
	s.context.Phase = phase
	s.context.Step = step
	ctx := s.context
	s.mu.Unlock()
	if s.dir == "" {
		return
	}
	if e := saveRecord(filepath.Join(s.dir, "active.json"), ctx); e != nil {
		s.failure("session metadata", e)
	}
}

// Report is called at a significant boundary, never per frame. Retain the
// small immutable incident request before handing expensive work to a worker.
func Report(key, reason, disposition string) bool {
	s := currentSession.Load()
	if s == nil {
		return false
	}
	s.mu.Lock()
	key = s.context.Film + "/" + key
	if s.closed.Load() || s.seen[key] {
		s.mu.Unlock()
		return false
	}
	s.seen[key] = true
	s.wg.Add(1)
	req := reportRequest{s.context, reason, disposition}
	lines := append([]string(nil), s.ring.lines...)
	s.mu.Unlock()
	if s.dir == "" {
		defer s.wg.Done()
		s.failure("report request", fmt.Errorf("recovery storage unavailable"))
		return false
	}
	dir, e := os.MkdirTemp(s.dir, "incident-")
	if e == nil {
		e = saveRecord(filepath.Join(dir, "request.json"), req)
	}
	if e != nil {
		defer s.wg.Done()
		s.failure("report request", e)
		return false
	}
	replay.PinReportFilm(filepath.Join(s.saves, "replays"), dir, req.Context.Film)
	s.notice(disposition + " Preparing diagnostics.")
	go func() { defer s.wg.Done(); s.makeReport(dir, s.dir, "error_report.zip", req, lines) }()
	return true
}
func (s *Session) makeReport(dir, session, file string, req reportRequest, lines []string) bool {
	final := filepath.Join(dir, file)
	if st, err := os.Lstat(final); err == nil {
		if st.Mode().IsRegular() {
			a, err := zip.OpenReader(final)
			if err == nil {
				names := map[string]bool{}
				for _, f := range a.File {
					names[f.Name] = true
				}
				a.Close()
				if names["report.txt"] && names["log.txt"] && names["recovery.json"] {
					return true
				}
			}
		}
		s.failure("archive", fmt.Errorf("existing output is not a diagnostic archive: %s", file))
		return false
	}
	f, e := os.CreateTemp(dir, ".archive-")
	if e != nil {
		_ = saveRecord(filepath.Join(session, "retry.json"), true)
		s.failure("archive", e)
		return false
	}
	defer os.Remove(f.Name())
	z := zip.NewWriter(f)
	write := func(name, data string) {
		if e != nil {
			return
		}
		var w io.Writer
		w, e = z.Create(name)
		if e == nil {
			_, e = io.WriteString(w, data)
		}
	}
	// An older unfinished session may predate flavor receipts.
	version, flavor := req.Context.Version, req.Context.Flavor
	if version == "" {
		version = "unknown"
	}
	if flavor == "" {
		flavor = "unknown"
	}
	write("report.txt", fmt.Sprintf("GoShip 2000 %s\nFlavor %s\nBuild %s\nPlatform %s/%s\nSettings %s\nMission %s\nLogical update at last recorded phase %d\nPhase %s\n%s\n%s\n", version, flavor, req.Context.Build, runtime.GOOS, runtime.GOARCH, req.Context.Settings, req.Context.Mission, req.Context.Step, req.Context.Phase+" "+req.Context.Facts, req.Disposition, req.Reason))
	if lines == nil {
		for _, n := range []string{"journal.log.previous", "journal.log"} {
			if b, er := readBounded(filepath.Join(session, n), historyBudget); er == nil {
				lines = append(lines, string(b))
			}
		}
	}
	write("log.txt", strings.Join(lines, ""))
	if b, er := readBounded(filepath.Join(session, "fatal.txt"), 256<<10); er == nil && len(b) > 0 {
		write("crash.txt", string(b))
	}
	if e == nil {
		source := filepath.Join(s.saves, "replays")
		if replay.HasPinnedReport(dir, req.Context.Film) {
			source = dir
		}
		status, n, err := replay.CopyReportFilm(z, source, req.Context.Film)
		if err != nil {
			if errors.Is(err, replay.ErrReportWrite) {
				e = err
			} else {
				status = "Recording evidence unavailable: " + err.Error()
			}
		}
		rec, _ := json.MarshalIndent(map[string]any{"version": 1, "recording": status, "verified_operations": n, "career_authority": false, "termination_or_error": req.Reason}, "", "  ")
		write("recovery.json", string(rec))
	}
	e = errors.Join(e, z.Close())
	if e == nil {
		e = f.Sync()
	}
	e = errors.Join(e, f.Close())
	if e == nil {
		e = publishRename(f.Name(), final)
	}
	if e != nil {
		_ = saveRecord(filepath.Join(session, "retry.json"), true)
		s.failure("archive", e)
		return false
	}
	replay.ReleaseReportPin(dir, req.Context.Film)
	rel, _ := filepath.Rel(s.saves, final)
	s.notice(req.Disposition + " Diagnostics saved to " + rel + ". Please include this report when reporting the issue.")
	return true
}
func readBounded(path string, limit int64) ([]byte, error) {
	st, e := os.Lstat(path)
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("invalid diagnostic companion")
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(st, opened) {
		return nil, fmt.Errorf("diagnostic companion changed during open")
	}
	return io.ReadAll(io.LimitReader(f, limit))
}
func (s *Session) recover(root string) {
	dirs, e := os.ReadDir(root)
	if e != nil {
		s.failure("session scan", e)
		return
	}
	for _, entry := range dirs {
		dir := filepath.Join(root, entry.Name())
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "session-") || dir == s.dir {
			continue
		}
		var ctx MissionContext
		if loadRecord(filepath.Join(dir, "active.json"), &ctx) != nil {
			if _, err := os.Lstat(filepath.Join(dir, "retry.json")); err != nil {
				continue
			}
			if loadRecord(filepath.Join(dir, "clean.json"), &ctx) != nil {
				continue
			}
		}
		if loadRecord(filepath.Join(dir, "active.json"), &ctx) == nil {
			var last MissionContext
			if loadRecord(filepath.Join(dir, "progress.json"), &last) == nil && last.Film == ctx.Film && last.Mission == ctx.Mission && last.Step > ctx.Step {
				ctx.Step = last.Step
			}
			all := s.retryRequests(dir)
			req := reportRequest{ctx, "Unexpected termination; cause unknown.", "The previous session ended unexpectedly."}
			if b, _ := readBounded(filepath.Join(dir, "fatal.txt"), 1024); len(b) > 0 {
				req.Reason = "Runtime fatal output captured; see crash.txt."
			}
			if s.makeReport(dir, dir, "crash_report.zip", req, nil) && all {
				publishRename(filepath.Join(dir, "active.json"), filepath.Join(dir, "recovered.json"))
			}
		}
		if loadRecord(filepath.Join(dir, "clean.json"), &ctx) == nil && s.retryRequests(dir) {
			os.Remove(filepath.Join(dir, "retry.json"))
		}
	}
}
func (s *Session) retryRequests(dir string) bool {
	entries, e := os.ReadDir(dir)
	if e != nil {
		return false
	}
	ok := true
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "incident-") {
			continue
		}
		sub := filepath.Join(dir, entry.Name())
		var req reportRequest
		if loadRecord(filepath.Join(sub, "request.json"), &req) == nil {
			ok = s.makeReport(sub, dir, "error_report.zip", req, nil) && ok
		}
	}
	return ok
}
func (s *Session) Close() {
	if s == nil || !s.closed.CompareAndSwap(false, true) {
		return
	}
	currentSession.CompareAndSwap(s, nil)
	// Synchronize with any producer registering an incident at shutdown.
	s.mu.Lock()
	s.mu.Unlock()
	if s.dir == "" {
		return
	}
	close(s.stop)
	<-s.done
	s.wg.Wait()
	debug.SetCrashOutput(nil, debug.CrashOptions{})
	if err := publishRename(filepath.Join(s.dir, "active.json"), filepath.Join(s.dir, "clean.json")); err != nil {
		s.failure("clean shutdown marker", err)
		return
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	disposable := map[string]bool{"progress.json": true, "owner.lease": true, "journal.log": true, "journal.log.previous": true, "fatal.txt": true, "clean.json": true}
	for _, e := range entries {
		if !disposable[e.Name()] || !e.Type().IsRegular() {
			return
		}
	}
	// Detach the name while owning the lease. A concurrent recovery scanner
	// that saw active.json can no longer reach this directory by its old name.
	retired := filepath.Join(filepath.Dir(s.dir), ".clean-"+filepath.Base(s.dir))
	if err := os.Rename(s.dir, retired); err != nil {
		return
	}
	for _, entry := range entries {
		os.Remove(filepath.Join(retired, entry.Name()))
	}
	os.Remove(retired)
}

func Phase(phase string) {
	s := currentSession.Load()
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.context.Facts == "" {
		s.context.Facts = s.context.Phase
	}
	s.context.Phase = phase
	ctx := s.context
	s.mu.Unlock()
	if s.dir != "" {
		if e := saveRecord(filepath.Join(s.dir, "active.json"), ctx); e != nil {
			s.failure("session metadata", e)
		}
	}
}

func FlightProgress(step uint64) {
	if s := currentSession.Load(); s != nil {
		s.mu.Lock()
		s.context.Step = step
		s.mu.Unlock()
		Info("flight", "Reached logical update", "step", step)
	}
}
