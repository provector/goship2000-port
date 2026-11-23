package state

import "testing"

// Source outcome flags retain their own domains after raw2 admission. These are
// supplied final-fact boundary tests, not evidence of natural crash reachability.
func TestAuthoredLossAndWoundUseOrdinarySourceOutcome(t *testing.T) {
	for _, tc := range []struct {
		flags  byte
		status PilotStatus
		ph     int8
	}{
		{0x10, StatusKIA, 1}, {8, StatusMIA, 0}, {0x18, StatusMIA, 1}, {0x84, StatusActive, 1},
	} {
		p, f := flightFixture(t)
		p.MissionType = MissionSimulated
		f.MissionType = MissionSimulated
		f.LaunchSwitches = f.LaunchSwitches&0x1fff | 0x4000
		f.SourceFlags = tc.flags
		f.Reason = "terminal"
		f.PlayerTerminal = true
		out := nProposal(t, p, f)
		q, e := ResolveSortieProposal(out, true)
		if e != nil {
			t.Fatal(e)
		}
		if out.Version != SortieProposalVersion || q.Status != tc.status || q.Statistics.AwardCounts[6] != tc.ph {
			t.Fatalf("raw2 flags%x outcome%+v", tc.flags, out)
		}
	}
}
