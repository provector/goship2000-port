package diagnostics

import (
	"gunship2000-assets/game/replay"
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
	if e := saveRecord(filepath.Join(s.saves, "reports", "pending-"+sortie+".json"), req); e != nil {
		s.failure("pending diagnostic", e)
	}
}

func RecallWarning(sortie string) string {
	s := currentSession.Load()
	if s == nil || s.dir == "" || !replay.ValidID(sortie) {
		return ""
	}
	var req reportRequest
	if loadRecord(filepath.Join(s.saves, "reports", "pending-"+sortie+".json"), &req) != nil {
		return ""
	}
	SetMission(req.Context.Mission, req.Context.Film, req.Context.Phase, req.Context.Step)
	return req.Reason
}

func ForgetWarning(sortie string) {
	s := currentSession.Load()
	if s != nil && s.dir != "" && replay.ValidID(sortie) {
		os.Remove(filepath.Join(s.saves, "reports", "pending-"+sortie+".json"))
	}
}
