package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func summaryFixture() MissionSummaryInput {
	before := &Pilot{ID: "summary-fixture", Name: "Pilot Example", Rank: RankCaptain, LegacyRaw: make([]byte, LegacyRecordSize), Statistics: &PilotStatistics{AwardCounts: make([]int8, 8)}}
	after := cloneSortiePilot(before)
	after.Rank = RankMajor
	after.Statistics.AwardCounts[2] = 1
	p := &SortieProposal{PilotBefore: before, MedalIndex: 2, Score: SortieScore{NormalizedFlags: 0x83, ExportedScoreWord: 1234}}
	p.Facts.ActorCount, p.Facts.ElapsedDA = 5, 13500 //37 minutes30 seconds, DA in sixths.
	p.Tables.RankNames = [12]string{"Warrant Officer Candidate", "Warrant Officer W-1", "Chief Warrant Officer W-2", "Chief Warrant Officer W-3", "Chief Warrant Officer W-4", "2nd Lieutenant", "1st Lieutenant", "Captain", "Major", "Lieutenant Colonel", "Colonel", "Brigadier General"}
	p.Tables.MedalNames = [8]string{"CONGRESSIONAL MEDAL OF HONOR", "DISTINGUISHED SERVICE CROSS", "SILVER STAR", "BRONZE STAR FOR VALOR", "AIR MEDAL FOR VALOR", "ARMY COMMENDATION FOR VALOR", "PURPLE HEART", "NATIONAL DEFENSE SERVICE MEDAL"}
	p.Facts.ResultContext = &SortieResultContext{Actors: make([]SortieActorFacts, 5)}
	r := &SortieResultDetails{Recommendations: []SortieCrewRecommendation{{Type: 0, Actor: 1}, {Type: 2, Actor: 2}}}
	for i := range p.Facts.ResultContext.Actors {
		a := &p.Facts.ResultContext.Actors[i]
		a.Actor, a.SourceIndex, a.RankRaw, a.Name = i, i, 1, "Crew "+string(rune('A'+i-1))
		if i != 0 {
			r.Crew = append(r.Crew, SortieCrewMetric{Actor: i, SourceIndex: i, Name: a.Name, Rank: 1, Flight: 100 - i, Attack: 50 + i, Defense: 94})
		}
	}
	r.Crew[0].Flags = 0x84 //Base/Wounded.
	r.Crew[1].Flags = 0x18 //KIA before MIA on follower rows.
	r.Crew[2].Flags = 0x0c //MIA cannot get/Wounded.
	r.Crew[3].Flags = 0x24 //Rescued/Wounded.
	copy(p.Facts.FinalObjects[5][15:27], "Fuel Tank")
	copy(p.Facts.FinalObjects[6][15:27], "MT-LB")
	copy(p.Facts.FinalObjects[7][15:27], "Warehouse")
	copy(p.Facts.FinalObjects[8][15:27], "Not credited")
	p.Facts.ObjectCredits[2] = 1 << 4
	p.Facts.ObjectCredits[3] = 2 | 7<<4
	p.Facts.ObjectCredits[4] = 6 //Source excludes6, even though nonzero.
	return MissionSummaryInput{Proposal: p, Pilot: after, Result: r,
		Choices:    &SortieCrewChoices{Recipients: []int{4, 1}},
		ShortRanks: [12]string{"WOC", "WO1", "CW2", "CW3", "CW4", "2LT", "1LT", "CPT", "MAJ", "LTC", "COL", "BG"},
		Date:       time.Date(1992, 11, 10, 23, 59, 0, 0, time.FixedZone("fixture", 3600))}
}

func TestSummaryFullFormatterUsesFrozenActualFlight(t *testing.T) {
	in := summaryFixture()
	before, _ := json.Marshal(in)
	raw, err := FormatMissionSummary(in)
	if err != nil {
		t.Fatal(err)
	}
	want := "\n                               MISSION SUMMARY\n\n" +
		"DATE:            November 10, 1992\n" +
		"PILOT:           Major Pilot Example\n\n" +
		"MISSION STATUS:  Landed at base, achieved primary and secondary.\n\n" +
		"TIME OF MISSION: 37 minutes 30 seconds\n\n" +
		"AWARDS:          Promotion to Major, SILVER STAR\n\n" +
		"MISSION SCORE:   1,234\n\n" +
		"FLIGHT STATUS:               FLIGHT ATTACK DEFENSE STATUS     RECOMMENDATION\n" +
		"WO1 Crew A                     99%    51%    94%    Base/Wounded    SILVER STAR\n" +
		"WO1 Crew B                     98%    52%    94%    KIA             \n" +
		"WO1 Crew C                     97%    53%    94%    MIA             \n" +
		"WO1 Crew D                     96%    54%    94%    Rescued/Wounded Promo\n\n\n" +
		"DESTRUCTION LIST:\n" +
		"    Fuel Tank -- destroyed by MAJ Pilot Example\n" +
		"        MT-LB -- destroyed by WO1 Crew A\n" +
		"    Warehouse -- destroyed by Mission Assets\n"
	if string(raw) != want {
		t.Fatalf("full source-style text differs\nGOT:\n%s\nWANT:\n%s", raw, want)
	}
	after, _ := json.Marshal(in)
	if !bytes.Equal(before, after) {
		t.Fatal("formatting changed frozen result, choices or RNG")
	}
}

func TestSummaryStatusPriorityTimeAndScoreBoundaries(t *testing.T) {
	for _, tc := range []struct {
		flags byte
		want  string
	}{{0x1c, "MIA"}, {0x10, "KIA"}, {0x24, "Rescued from behind enemy lines, wounded"}, {0x80, "Landed at base"}, {4, "Landed in friendly territory, wounded"}} {
		in := summaryFixture()
		in.Proposal.Score.NormalizedFlags = tc.flags
		b, e := FormatMissionSummary(in)
		if e != nil || !strings.Contains(string(b), "MISSION STATUS:  "+tc.want+".\n") {
			t.Fatal(tc.flags, string(b), e)
		}
		if tc.flags&0x10 != 0 && (!strings.Contains(string(b), "AWARDS:          NONE") || strings.Contains(string(b), "SILVER STAR") || strings.Contains(string(b), "Promo")) {
			t.Fatal("player KIA must suppress awards/recommendations", string(b))
		}
	}
	for _, tc := range []struct {
		da    uint16
		want  string
		score int16
		value string
	}{{0, "0 minutes", 0, "0"}, {6, "1 second", -1, "-1"}, {360, "1 minute", -1234, "-1,234"}, {366, "1 minute 1 second", -32768, "--32768"}} {
		in := summaryFixture()
		in.Proposal.Facts.ElapsedDA, in.Proposal.Score.ExportedScoreWord = tc.da, tc.score
		b, e := FormatMissionSummary(in)
		if e != nil || !strings.Contains(string(b), "TIME OF MISSION: "+tc.want+"\n") || !strings.Contains(string(b), "MISSION SCORE:   "+tc.value+"\n") {
			t.Fatal(tc, string(b), e)
		}
	}
}

func TestSummarySingleAndInvalidGeneration(t *testing.T) {
	in := summaryFixture()
	in.Proposal.Facts.ActorCount = 1
	in.Proposal.Facts.ResultContext.Actors = in.Proposal.Facts.ResultContext.Actors[:1]
	in.Result.Crew, in.Result.Recommendations, in.Choices = nil, nil, nil
	in.Proposal.Facts.ObjectCredits = [40]byte{}
	b, e := FormatMissionSummary(in)
	if e != nil || strings.Contains(string(b), "FLIGHT STATUS:") || !strings.HasSuffix(string(b), "DESTRUCTION LIST:\n") {
		t.Fatal("Single must not invent follower rows", string(b), e)
	}
	in.Proposal.Facts.ResultContext = nil
	if b, e = FormatMissionSummary(in); e == nil || b != nil {
		t.Fatal("incomplete actual result must not generate a replacement report", e)
	}
}

func TestSummaryAtomicReplacementAndFailurePreservesPrevious(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, MissionSummaryFilename)
	old := []byte("previous complete report\n")
	if e := WriteMissionSummary(dir, old); e != nil {
		t.Fatal(e)
	}
	e := writeMissionSummary(dir, []byte("new complete report\n"), func(string, string) error { return errors.New("controlled rename failure") })
	if e == nil || !strings.Contains(e.Error(), "previous file retained") {
		t.Fatal(e)
	}
	got, e := os.ReadFile(target)
	if e != nil || !bytes.Equal(got, old) {
		t.Fatal("failed replace lost prior file", e)
	}
	if e = WriteMissionSummary(dir, nil); e == nil {
		t.Fatal("empty failed generation must not replace a report")
	}
	new := []byte("second completed mission\n")
	if e = WriteMissionSummary(dir, new); e != nil {
		t.Fatal(e)
	}
	got, e = os.ReadFile(target)
	if e != nil || !bytes.Equal(got, new) {
		t.Fatal("second successful mission did not atomically replace", e)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != MissionSummaryFilename {
		t.Fatal("temporary report leak", entries)
	}
	link := filepath.Join(t.TempDir(), MissionSummaryFilename)
	if e = os.Symlink(target, link); e == nil {
		if e = WriteMissionSummary(filepath.Dir(link), []byte("must not follow link")); e == nil {
			t.Fatal("summary followed an unrelated target")
		}
		got, _ = os.ReadFile(target)
		if !bytes.Equal(got, new) {
			t.Fatal("symlink rejection changed original file")
		}
	}
}
