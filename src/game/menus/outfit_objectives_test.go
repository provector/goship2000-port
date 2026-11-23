package menus

import (
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"strings"
	"testing"
)

// These literals come from DS1b82/0222/022a in GS.GS2, not the briefing
// DisplayKind labels or an expected string produced by objectiveSummary.
func outfitTextFixture(t *testing.T) *OutfitView {
	t.Helper()
	v := outfitTestView(t)
	v.Assets.Preflight.ObjectiveSummary = &assetdata.OutfitObjectiveText{
		Headers:       [2]string{"%cPRIMARY:%c  %s", "%cSECONDARY:%c  %s"},
		RecordFormat:  "%02d/%02d %s %Fs",
		Verbs:         []string{"Training", "S&D", "Destroy", "Destroy", "Pick up", "Drop off", "S&R", "Recon"},
		Abbreviations: [][2]string{{"Assault", "Assl"}, {"Attack", "Atk"}, {"Group", "Grp"}, {"Helicopter", "Heli"}},
	}
	return v
}

func TestOutfitObjectiveOriginalProducer(t *testing.T) {
	v := outfitTextFixture(t)
	v.Mission.SourceWorld = &assetdata.MissionWorld{FMHD: make([]byte, 39*2)}
	copy(v.Mission.SourceWorld.FMHD, "Truck Park")
	copy(v.Mission.SourceWorld.FMHD[39:], "Troops")
	primary := gamestate.MissionObjective{Formation: 0, RawKind: 3, DisplayKind: 4, Count: 99, Name: "paper OBJECT override"}
	secondary := gamestate.MissionObjective{Formation: 1, RawKind: 4, DisplayKind: 1, Count: 2, Name: "paper override"}
	if got := v.objectiveSummary(0, primary); got != "Destroy Truck Park" {
		t.Fatalf("primary: %q", got)
	}
	if got := v.objectiveSummary(1, secondary); got != "Pick up Troops" {
		t.Fatalf("secondary: %q", got)
	}
	// Neither transformed Count nor DisplayKind is an argument at A19196.
	primary.Count, primary.DisplayKind = 0, 7
	if got := v.objectiveSummary(0, primary); got != "Destroy Truck Park" {
		t.Fatalf("mapped-domain leakage: %q", got)
	}
	if assetdata.OfficeCString(v.Mission.SourceWorld.FMHD[:25]) != "Truck Park" {
		t.Fatal("render mutated formation")
	}
}

func TestOutfitObjectiveSourceShorteningAndImportedRecord(t *testing.T) {
	v := outfitTextFixture(t)
	o := gamestate.MissionObjective{RawKind: 2, Name: "Assault Helicopter Group"}
	// Six coordinate bytes + eight action bytes +23 name bytes =37. Source
	// substitutions reduce37->34->32->26, leaving20 visible bytes after+6.
	if got := v.objectiveSummary(0, o); got != "Destroy Assl Heli Grp" {
		t.Fatalf("ordered shortening: %q", got)
	}
	o.Name = "abcdefghijklmnopqrstuvwxyz0123456789"
	if got := v.objectiveSummary(0, o); got != "Destroy abcdefghijklmnopq" {
		t.Fatalf("source31-byte cap: %q", got)
	}
	v.Mission.SourcePreparation = &gamestate.CampaignSourceRaw{TargetBlock: make([]byte, 310)}
	copy(v.Mission.SourcePreparation.TargetBlock[2*62:], "03/09 Destroy Source Target")
	if got := v.objectiveSummary(0, o); got != "Destroy Source Target" {
		t.Fatalf("retained TARGET name double-formatted: %q", got)
	}
	v.Mission.Objectives = map[string]gamestate.MissionObjective{"primary": o}
	lines := strings.Join(v.objectiveLines(), "|")
	if !strings.Contains(lines, "PRIMARY:") || !strings.Contains(lines, "SECONDARY:") || strings.Contains(lines, "03/09") {
		t.Fatalf("source headers/prefix: %q", lines)
	}
}
