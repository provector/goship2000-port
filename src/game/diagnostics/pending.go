package diagnostics

import (
	"gunship2000-assets/game/replay"
	"gunship2000-assets/internal/playerdirs"
	"os"
	"path/filepath"
)

// A debrief can span two clean process lifetimes. This optional diagnostic
// companion has no career authority and is keyed by the immutable sortie ID.
func RememberWarning(sortie, reason string) {
	s := currentSession.Load()
	if s == nil || s.dir == "" || reason == "" || !replay.ValidID(sortie) {
		return
	}
	s.mu.Lock()
	req := reportRequest{Context: s.context, Reason: reason}
	s.mu.Unlock()
	if e := saveRecord(filepath.Join(filepath.Dir(s.dir), "pending-"+sortie+".json"), req); e != nil {
		s.failure("pending diagnostic", e)
	}
}

func RecallWarning(sortie string) string {
	s := currentSession.Load()
	if s == nil || s.dir == "" || !replay.ValidID(sortie) {
		return ""
	}
	var req reportRequest
	for _, dir := range playerdirs.ArchiveReaders(s.saves, "reports") {
		if loadRecord(filepath.Join(dir, "pending-"+sortie+".json"), &req) == nil {
			SetMission(req.Context.Mission, req.Context.Film, req.Context.Phase, req.Context.Step)
			return req.Reason
		}
	}
	return ""
}

func ForgetWarning(sortie string) {
	s := currentSession.Load()
	if s != nil && s.dir != "" && replay.ValidID(sortie) {
		for _, dir := range playerdirs.ArchiveReaders(s.saves, "reports") {
			path := filepath.Join(dir, "pending-"+sortie+".json")
			var req reportRequest
			if loadRecord(path, &req) == nil {
				if err := os.Remove(path); err != nil {
					s.failure("pending diagnostic retirement", err)
				}
			}
		}
	}
}
