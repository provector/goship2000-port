package state

import (
	"bytes"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const MissionSummaryFilename = "SUMMARY.TXT"

// MissionSummaryInput contains only the acknowledged sortie's retained facts
// and decisions. Date is the host date of that acknowledgement, matching the
// source's host-date formatting rather than inventing a simulation calendar.
// The formatter never consults or changes the current office, save or RNG.
type MissionSummaryInput struct {
	Proposal   *SortieProposal
	Pilot      *Pilot
	Result     *SortieResultDetails
	Choices    *SortieCrewChoices
	ShortRanks [12]string
	Date       time.Time
}

// FormatMissionSummary follows GS section3:3442..3A4B. The earlier Step36/55
// index abbreviated the range at391C; the same function continues with the
// credited-object destruction list, then closes at3A3E..3A4B.
func FormatMissionSummary(in MissionSummaryInput) ([]byte, error) {
	p, pilot, result := in.Proposal, in.Pilot, in.Result
	if p == nil || pilot == nil || p.PilotBefore == nil || pilot.ID != p.PilotBefore.ID || in.Date.IsZero() {
		return nil, fmt.Errorf("mission summary needs its actual frozen result, pilot and acknowledgement date")
	}
	if p.Facts.ActorCount < 1 || p.Facts.ActorCount > 5 || p.Facts.ResultContext == nil || len(p.Facts.ResultContext.Actors) != p.Facts.ActorCount || result == nil {
		return nil, fmt.Errorf("mission summary needs the complete actual flight and result context")
	}
	if pilot.Rank.Raw() < 0 || pilot.Rank.Raw() >= len(p.Tables.RankNames) || pilot.Statistics == nil || len(pilot.Statistics.AwardCounts) != 8 || p.PilotBefore.Statistics == nil || len(p.PilotBefore.Statistics.AwardCounts) != 8 {
		return nil, fmt.Errorf("mission summary rank or award provenance is incomplete")
	}
	validText := func(label, value string) error {
		if value == "" || strings.ContainsAny(value, "\x00\r\n") {
			return fmt.Errorf("mission summary %s is missing or contains a line delimiter", label)
		}
		return nil
	}
	if err := validText("pilot name", pilot.Name); err != nil {
		return nil, err
	}
	if err := validText("pilot rank", p.Tables.RankNames[pilot.Rank.Raw()]); err != nil {
		return nil, err
	}
	for _, rank := range in.ShortRanks {
		if err := validText("short rank", rank); err != nil {
			return nil, err
		}
	}
	var b strings.Builder
	//3442..34DA initializes forty spaces, then passes interior pointers.
	fmt.Fprintf(&b, "\n%sMISSION SUMMARY\n\n", strings.Repeat(" ", 31))
	fmt.Fprintf(&b, "DATE:%s%s\n", strings.Repeat(" ", 12), in.Date.Format("January 2, 2006"))
	fmt.Fprintf(&b, "PILOT:%s%s %s\n\n", strings.Repeat(" ", 11), p.Tables.RankNames[pilot.Rank.Raw()], pilot.Name)
	flags := p.Score.NormalizedFlags
	b.WriteString("MISSION STATUS:  ")
	switch {
	case flags&8 != 0:
		b.WriteString("MIA")
	case flags&0x10 != 0:
		b.WriteString("KIA")
	case flags&0x20 != 0:
		b.WriteString("Rescued from behind enemy lines")
	case flags&0x80 != 0:
		b.WriteString("Landed at base")
	default:
		b.WriteString("Landed in friendly territory")
	}
	if flags&4 != 0 && flags&0x10 == 0 {
		b.WriteString(", wounded")
	}
	if flags&3 != 0 {
		b.WriteString(", achieved ")
		if flags&1 != 0 {
			b.WriteString("primary")
		}
		if flags&2 != 0 {
			if flags&1 != 0 {
				b.WriteString(" and ")
			}
			b.WriteString("secondary")
		}
	}
	b.WriteString(".\n\nTIME OF MISSION:")
	minutes, seconds := int(p.Facts.ElapsedDA)/360, int(p.Facts.ElapsedDA)/6%60
	if minutes != 0 || seconds == 0 {
		fmt.Fprintf(&b, " %d minute%s", minutes, summaryPlural(minutes))
	}
	if seconds != 0 {
		fmt.Fprintf(&b, " %d second%s", seconds, summaryPlural(seconds))
	}
	b.WriteString("\n\nAWARDS:" + strings.Repeat(" ", 10))
	var awards []string
	if flags&0x10 == 0 {
		if pilot.Rank.Raw() != p.PilotBefore.Rank.Raw() {
			if p.PilotBefore.Rank.Raw() > 0 && p.PilotBefore.Rank.Raw() < 5 && pilot.Rank.Raw() >= 5 {
				awards = append(awards, "Awarded commission")
			} else {
				awards = append(awards, "Promotion to "+p.Tables.RankNames[pilot.Rank.Raw()])
			}
		}
		//36A8 queries the selected merit medal, not the Purple Heart/initial
		//service badge. Report it only when the acknowledged result actually
		//awarded it, preserving the source's deferred-repeat distinction.
		if medal := p.MedalIndex; medal >= 0 && medal < 6 && p.PilotBefore.Statistics.AwardCounts[medal] != pilot.Statistics.AwardCounts[medal] {
			if err := validText("medal", p.Tables.MedalNames[medal]); err != nil {
				return nil, err
			}
			awards = append(awards, p.Tables.MedalNames[medal])
		}
	}
	if len(awards) == 0 {
		b.WriteString("NONE")
	} else {
		b.WriteString(strings.Join(awards, ", "))
	}
	b.WriteString("\n\nMISSION SCORE:   ")
	//The source prints the retained signed16 exported score. Preserve its
	//word NEG and thousands formatting, including the -32768 wrap boundary.
	score := p.Score.ExportedScoreWord
	if score < 0 {
		b.WriteByte('-')
		score = -score
	}
	if score >= 1000 {
		fmt.Fprintf(&b, "%d,%03d", score/1000, score%1000)
	} else {
		fmt.Fprint(&b, score)
	}
	b.WriteString("\n\n")
	if p.Facts.ActorCount > 1 {
		if len(result.Crew) != p.Facts.ActorCount-1 {
			return nil, fmt.Errorf("mission summary lacks actual follower metrics")
		}
		fmt.Fprintf(&b, "FLIGHT STATUS:%sFLIGHT ATTACK DEFENSE STATUS     RECOMMENDATION\n", strings.Repeat(" ", 15))
		for i, metric := range result.Crew {
			if metric.Actor != i+1 || metric.Actor >= len(p.Facts.ResultContext.Actors) || metric.Rank < 0 || metric.Rank >= len(in.ShortRanks) {
				return nil, fmt.Errorf("mission summary follower identity or rank is incomplete")
			}
			actor := p.Facts.ResultContext.Actors[metric.Actor]
			if actor.Actor != metric.Actor || actor.SourceIndex != metric.SourceIndex || actor.Name != metric.Name {
				return nil, fmt.Errorf("mission summary follower metrics differ from their frozen actor")
			}
			if err := validText("follower name", actor.Name); err != nil {
				return nil, err
			}
			status := "Base"
			switch {
			case metric.Flags&0x10 != 0:
				status = "KIA"
			case metric.Flags&8 != 0:
				status = "MIA"
			case metric.Flags&0x20 != 0:
				status = "Rescued"
			}
			if metric.Flags&0x18 == 0 && metric.Flags&4 != 0 {
				status += "/Wounded"
			}
			recommendation := ""
			if flags&0x10 == 0 {
				var err error
				recommendation, err = missionSummaryRecommendation(in, metric.Actor)
				if err != nil {
					return nil, err
				}
			}
			fmt.Fprintf(&b, "%-29s%4d%%%6d%%%6d%%    %-16s%s\n", in.ShortRanks[metric.Rank]+" "+metric.Name, metric.Flight, metric.Attack, metric.Defense, status, recommendation)
		}
		b.WriteString("\n\n")
	}
	b.WriteString("DESTRUCTION LIST:\n")
	for row, object := range p.Facts.FinalObjects {
		credit := p.Facts.ObjectCredits[row/2] >> uint((row&1)*4) & 15
		if (credit < 1 || credit > 5) && credit != 7 {
			continue
		}
		name := string(bytes.SplitN(object[15:27], []byte{0}, 2)[0])
		if strings.ContainsAny(name, "\r\n") {
			return nil, fmt.Errorf("mission summary object name contains a line delimiter")
		}
		owner := "Mission Assets"
		if credit == 1 {
			owner = in.ShortRanks[pilot.Rank.Raw()] + " " + pilot.Name
		} else if credit <= 5 {
			actor := int(credit) - 1
			if actor >= len(p.Facts.ResultContext.Actors) {
				return nil, fmt.Errorf("mission summary credited follower%d lacks actual identity", actor)
			}
			a := p.Facts.ResultContext.Actors[actor]
			if a.RankRaw < 0 || a.RankRaw >= len(in.ShortRanks) {
				return nil, fmt.Errorf("mission summary credited follower rank is invalid")
			}
			owner = in.ShortRanks[a.RankRaw] + " " + a.Name
		}
		fmt.Fprintf(&b, "%13s -- destroyed by %s\n", name, owner)
	}
	return []byte(b.String()), nil
}

func summaryPlural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func missionSummaryRecommendation(in MissionSummaryInput, actor int) (string, error) {
	var out []string
	for i, rec := range in.Result.Recommendations {
		recipient := rec.Actor
		if in.Choices != nil {
			if len(in.Choices.Recipients) != len(in.Result.Recommendations) {
				return "", fmt.Errorf("mission summary recommendation choices are incomplete")
			}
			recipient = in.Choices.Recipients[i]
		}
		if recipient != actor {
			continue
		}
		if rec.Type == 0 {
			out = append(out, "Promo") //1AB5:081E/section3:0BDE, third argument1.
		} else if rec.Type > 0 && rec.Type < len(in.Proposal.Tables.MedalNames) && in.Proposal.Tables.MedalNames[rec.Type] != "" {
			out = append(out, in.Proposal.Tables.MedalNames[rec.Type])
		} else {
			return "", fmt.Errorf("mission summary recommendation type%d is invalid", rec.Type)
		}
	}
	return strings.Join(out, ", "), nil
}

// WriteMissionSummary is an optional output, deliberately outside the career
// journal and film store. Never remove an existing file to make replacement
// succeed; generation/staging/sync/rename failure retains the previous report.
func WriteMissionSummary(directory string, raw []byte) error {
	return writeMissionSummary(directory, raw, os.Rename)
}

func writeMissionSummary(directory string, raw []byte, rename func(string, string) error) error {
	if strings.TrimSpace(directory) == "" || len(raw) == 0 {
		return fmt.Errorf("mission summary needs a writable runtime directory and a complete report")
	}
	if _, err := assetdata.GeneratedPath(filepath.Join(directory, ".summary-path-check"), filepath.Join(directory, ".unused-input")); err != nil {
		return err
	}
	target := filepath.Join(directory, MissionSummaryFilename)
	if info, err := os.Lstat(target); err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("refusing non-regular mission summary target: %s", target)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	f, err := os.CreateTemp(directory, ".summary-pending-*.tmp")
	if err != nil {
		return fmt.Errorf("stage mission summary: %w", err)
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err = f.Write(raw); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write mission summary (previous file retained): %w", err)
	}
	if err = rename(tmp, target); err != nil {
		return fmt.Errorf("replace %s (previous file retained): %w", target, err)
	}
	return nil
}
