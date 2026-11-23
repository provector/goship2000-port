package menus

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	. "gunship2000-assets/game/state"
	"gunship2000-assets/internal/testfixture"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCampaignSuspendSaveRestartRestore(t *testing.T) {
	a := missionReadinessAssets(t)
	p := campaignTestPilot(t)
	m, draft, err := CampaignStart(a, p, 13, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	flight, err := InitFlightPilots(p, m)
	if err != nil {
		t.Fatal(err)
	}
	if err = RefreshFlightPresentation(flight, a, m); err != nil {
		t.Fatal(err)
	}
	rows, err := PrepareReadinessRows(a, m)
	if err != nil {
		t.Fatal(err)
	}
	s := &MissionSession{Data: m, PilotID: p.ID, CampaignDraft: draft, ReadinessRows: rows, Flight: flight, Settings: PreflightSettings{CPGControls: 2, EnemyQuality: 3, Visibility: 1}, SecondaryControl: 1}
	for _, row := range rows {
		s.InitialAircraftIDs = append(s.InitialAircraftIDs, row[0])
	}
	if err = campaignSuspend(p, s); err != nil {
		t.Fatal(err)
	}
	store := saveTestStore(t)
	state := store.State()
	state.Slots[0] = p
	state.SelectedPilotID = p.ID
	if err = store.Commit(state); err != nil {
		t.Fatal(err)
	}
	loaded, err := OpenSaveStore(store.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	p = loaded.State().SelectedPilot()
	before := loaded.State()
	resumed, draft, err := CampaignStart(a, p, 99, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.ObjectHash != m.ObjectHash || len(resumed.Draws) != 5 {
		t.Fatal("resume regenerated objects or consumed more than operation/hour draws")
	}
	if resumed.Conditions.Callsign != m.Conditions.Callsign || resumed.Conditions.WindDirection != m.Conditions.WindDirection || resumed.Conditions.Temperature != m.Conditions.Temperature {
		t.Fatal("resume changed retained weather/callsigns")
	}
	restoredFlight, err := InitFlightPilots(p, resumed)
	if err != nil {
		t.Fatal(err)
	}
	restored := &MissionSession{Data: resumed, PilotID: p.ID, Flight: restoredFlight}
	if err = restoreCampaignPreparation(restored, draft); err != nil {
		t.Fatal(err)
	}
	if err = RefreshFlightPresentation(restored.Flight, a, resumed); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.ReadinessRows, s.ReadinessRows) || !reflect.DeepEqual(restored.Flight, s.Flight) || restored.Settings != s.Settings || restored.SecondaryControl != 1 {
		t.Fatal("suspended flight/settings did not survive")
	}
	if !reflect.DeepEqual(before, loaded.State()) {
		t.Fatal("opening suspended session wrote the authoritative save")
	}
	if _, err = os.Stat(filepath.Join(store.Directory, filepath.ToSlash(filepath.Join("pilots", p.ID+".backup.json")))); !os.IsNotExist(err) {
		t.Fatal("preparation/suspend created a gameplay backup")
	}
}

func TestCampaignCompactAssignmentsUseCurrentAuthoritativeIdentity(t *testing.T) {
	a := missionReadinessAssets(t)
	p := campaignTestPilot(t)
	m, draft, err := CampaignStart(a, p, 13, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	flight, err := InitFlightPilots(p, m)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := PrepareReadinessRows(a, m)
	if err != nil {
		t.Fatal(err)
	}
	s := &MissionSession{Data: m, PilotID: p.ID, CampaignDraft: draft, Flight: flight, ReadinessRows: rows}
	for _, row := range rows {
		s.InitialAircraftIDs = append(s.InitialAircraftIDs, row[0])
	}
	if err = campaignSuspend(p, s); err != nil {
		t.Fatal(err)
	}
	x := p.Campaign.Suspended
	x.Flight[1], x.Flight[2] = x.Flight[2], x.Flight[1]
	x.Flight[1].Group, x.Flight[1].Leader = 1, true
	raw, err := json.Marshal(x.Flight)
	if err != nil {
		t.Fatal(err)
	}
	var entries []map[string]json.RawMessage
	if err = json.Unmarshal(raw, &entries); err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if len(entry) != 3 || entry["source_index"] == nil || entry["group"] == nil || entry["leader"] == nil {
			t.Fatal("suspension duplicates authoritative identity or presentation fields:", string(raw))
		}
	}
	p.Name, p.Rank = "Updated Authority", RankMajor
	p.Statistics.AwardCounts[0] = 3
	p.SquadronPilots = make([]SquadronPilot, 4)
	for i := range p.SquadronPilots {
		p.SquadronPilots[i] = SquadronPilot{Name: fmt.Sprintf("Current Wingman%d", i+1), Rank: RankCaptain, Portrait: i, LegacyRaw: make([]byte, 41)}
		p.SquadronPilots[i].LegacyRaw[26] = byte(i + 1)
	}
	resumed, resumedDraft, err := CampaignStart(a, p, 99, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = restoreCampaignPreparation(&MissionSession{Data: resumed}, resumedDraft); err == nil {
		t.Fatal("restore accepted absent authoritative identity inputs")
	}
	fresh, err := InitFlightPilots(p, resumed)
	if err != nil {
		t.Fatal(err)
	}
	restored := &MissionSession{Data: resumed, Flight: fresh}
	if err = restoreCampaignPreparation(restored, resumedDraft); err != nil {
		t.Fatal(err)
	}
	if err = RefreshFlightPresentation(restored.Flight, a, resumed); err != nil {
		t.Fatal(err)
	}
	if restored.Flight[0].Name != p.Name || restored.Flight[0].Rank != RankMajor || restored.Flight[0].AwardCounts[0] != 3 || restored.Flight[1].Name != "Current Wingman2" || restored.Flight[1].AwardCounts[0] != 2 || restored.Flight[1].Portrait != 1 || restored.Flight[1].Group != 1 || !restored.Flight[1].Leader {
		t.Fatal("resume did not combine authoritative identity with compact assignment")
	}
	bad, err := CloneCampaign(p.Campaign)
	if err != nil {
		t.Fatal(err)
	}
	bad.Suspended.Flight[0], bad.Suspended.Flight[1] = bad.Suspended.Flight[1], bad.Suspended.Flight[0]
	if err = bad.Validate(); err == nil {
		t.Fatal("primary pilot reassignment accepted")
	}
	bad.Suspended.Flight[0], bad.Suspended.Flight[1] = bad.Suspended.Flight[1], bad.Suspended.Flight[0]
	bad.Suspended.Flight[1].SourceIndex = bad.Suspended.Flight[0].SourceIndex
	if err = bad.Validate(); err == nil {
		t.Fatal("duplicate source assignment accepted")
	}
}

func TestCampaignDeclineAbortAndBadState(t *testing.T) {
	a := &assetdata.MissionAssets{Generation: missionTestData(t)}
	p := campaignTestPilot(t)
	m, draft, err := CampaignStart(a, p, 7, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := &MissionSession{Data: m, PilotID: p.ID, CampaignDraft: draft}
	if err = campaignDecline(p, s); err != nil {
		t.Fatal(err)
	}
	if p.Campaign.TargetCounter != int(byte(draft.TargetCounter+1)) || p.Campaign.ResultCounters[1] != 1 || binary.LittleEndian.Uint16(p.LegacyRaw[0x59:]) != 300 {
		t.Fatal("decline did not apply source counters")
	}
	if _, _, err = CampaignStart(a, p, 8, nil, nil); err != nil {
		t.Fatal("known retained Campaign cannot continue:", err)
	}
	if err = campaignAbort(p, s); err != nil {
		t.Fatal(err)
	}
	if p.Campaign != nil || p.MissionType != MissionFlight || p.LegacyRaw[0x4f] != 0 || binary.LittleEndian.Uint16(p.LegacyRaw[0x59:]) != 800 {
		t.Fatal("abort source result differs")
	}
	p = campaignTestPilot(t)
	p.LegacyRaw[0x4f] = 2
	if _, _, err = CampaignStart(a, p, 1, nil, nil); err == nil || !strings.Contains(err.Error(), "import") {
		t.Fatal("unknown legacy suspended state silently generated")
	}
}

func TestCampaignSuppliedSuspendedImport(t *testing.T) {
	a := missionReadinessAssets(t)
	dir := testfixture.Path("captain-observation")
	raw, err := os.ReadFile(filepath.Join(dir, "ROSTER.DAT"))
	if os.IsNotExist(err) {
		t.Skip("user comparison files absent")
	}
	if err != nil {
		t.Fatal(err)
	}
	state, err := ParseLegacyRoster("user comparison", raw)
	if err != nil {
		t.Fatal(err)
	}
	p := state.SelectedPilot()
	full, err := os.ReadFile(filepath.Join(dir, "CAMPGN09.DAT"))
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := os.ReadFile(filepath.Join(dir, "CAMPGN03.DAT"))
	if err != nil {
		t.Fatal(err)
	}
	imported, err := ImportCampaignCache(a, p, full, map[string][]byte{"CAMPGN03.DAT": fragment})
	if err != nil {
		t.Fatal(err)
	}
	p.Campaign = imported
	badPhase, err := CloneCampaign(imported)
	if err != nil {
		t.Fatal(err)
	}
	badPhase.Phase = CampaignActive
	if err = badPhase.Validate(); err == nil {
		t.Fatal("active state could carry a stale suspended snapshot")
	}
	m, draft, err := CampaignStart(a, p, 1, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if m.ObjectHash != "947bc8d134f3b735ca49b7dd76e645033df1f775f4365f6e31a24352a8c2382f" || m.World != "GULF2.WRL" || m.Scene != 1 || len(m.Paths) != 2 || len(m.Objects) != 80 {
		t.Fatal("supplied snapshot does not match independently framed Step24 witness")
	}
	if m.SourcePreparation == nil || !reflect.DeepEqual(m.SourcePreparation.RuntimeHeader, full[:18]) || len(m.SourcePreparation.TargetBlock) != 310 {
		t.Fatal("imported uninterpreted source state was lost")
	}
	flight, err := InitFlightPilots(p, m)
	if err != nil {
		t.Fatal(err)
	}
	s := &MissionSession{Data: m, Flight: flight}
	if err = restoreCampaignPreparation(s, draft); err != nil {
		t.Fatal(err)
	}
	if s.ReadinessRows[0][0] != 2 || s.ReadinessRows[2][0] != 4 || !reflect.DeepEqual(m.CampaignUnavailableAircraft, []int{5, 0, -1, -1}) {
		t.Fatal("cached flight/restrictions were replaced by defaults")
	}
	if _, err = ImportCampaignCache(a, p, full[:100], nil); err == nil {
		t.Fatal("truncated snapshot accepted")
	}
	bad := append([]byte(nil), full...)
	bad[2620] = 0
	if _, err = ImportCampaignCache(a, p, bad, nil); err == nil {
		t.Fatal("mismatched theatre accepted")
	}
}

func campaignTestPilot(t *testing.T) *Pilot {
	t.Helper()
	p := saveTestPilot(t, "Campaign Pilot")
	p.Rank = RankCaptain
	p.MissionType = MissionCampaign
	return p
}
