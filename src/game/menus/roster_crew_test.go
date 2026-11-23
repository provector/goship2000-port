package menus

import (
	"bytes"
	"encoding/json"
	gamestate "gunship2000-assets/game/state"
	"reflect"
	"testing"
)

func TestRosterStoredCrewUsesPersistedRowsAndSignedMedals(t *testing.T) {
	p := &gamestate.Pilot{LegacyRaw: make([]byte, 290), SquadronPilots: make([]gamestate.SquadronPilot, 4)}
	for i := range p.SquadronPilots {
		raw := make([]byte, 41)
		raw[26], raw[33] = 255, byte(i+1)
		p.SquadronPilots[i] = gamestate.SquadronPilot{Name: []string{"Arthur", "Adams", "Anderson", "Bennett"}[i], Rank: gamestate.RankWO1, LegacyRaw: raw}
	}
	before, _ := json.Marshal(p)
	rows := rosterStoredCrew(p)
	if rows[2].Name != "Anderson" || rows[2].Rank != 1 || rosterMedalsText(rows[2].Medals) != "NDS3 " {
		t.Fatal(rows)
	}
	if len(p.SquadronPilots) != 4 {
		t.Fatal("display generated a squadron")
	}
	after, _ := json.Marshal(p)
	if !bytes.Equal(before, after) {
		t.Fatal("display mutated stored medals")
	}
	c := debriefDisplay(t)
	frame := bytes.Repeat([]byte{15}, 64000)
	c.drawRosterCrew(frame, p)
	expected := bytes.Repeat([]byte{15}, 64000)
	// Source GS A246df/A24cd0: bank x78, row49, advance6, font3.
	font := c.Menus.Fonts[3]
	nameX := 78 + font.Measure([]byte("WO1")) + 4
	medalX := 78 + font.Measure([]byte("WO1")) + font.Measure([]byte("Anderson")) + 12
	for i, name := range []string{"Arthur", "Adams", "Anderson", "Bennett"} {
		_ = font.Draw(expected, []byte("WO1"), 78, 49+6*i, 0, 0, 0)
		_ = font.Draw(expected, []byte(name), nameX, 49+6*i, 0, 0, 0)
		_ = font.Draw(expected, []byte([]string{"NDS ", "NDS2 ", "NDS3 ", "NDS4 "}[i]), medalX, 49+6*i, 0, 0, 0)
	}
	if !bytes.Equal(frame, expected) {
		t.Fatal("source roster columns/ink/row spacing diverged")
	}
	p.SquadronPilots[1].Rank = gamestate.RankWOC
	frame = bytes.Repeat([]byte{15}, 64000)
	c.drawRosterCrew(frame, p)
	if !bytes.Equal(frame[55*320:61*320], bytes.Repeat([]byte{15}, 6*320)) {
		t.Fatal("rank0 row must be blank without shifting later rows")
	}
	// A24ced..24d4a measures even an omitted row before the later draw gate.
	p.SquadronPilots[1].Name = "VeryLongHiddenName"
	frame = bytes.Repeat([]byte{15}, 64000)
	c.drawRosterCrew(frame, p)
	expected = bytes.Repeat([]byte{15}, 64000)
	rankWidth := max(font.Measure([]byte("WO1")), font.Measure([]byte("WOC")))
	_ = font.Draw(expected, []byte("WO1"), 78, 49, 0, 0, 0)
	_ = font.Draw(expected, []byte("Arthur"), 78+rankWidth+4, 49, 0, 0, 0)
	_ = font.Draw(expected, []byte("NDS "), 78+rankWidth+font.Measure([]byte("VeryLongHiddenName"))+12, 49, 0, 0, 0)
	if !bytes.Equal(frame[49*320:55*320], expected[49*320:55*320]) {
		t.Fatal("omitted-row source measurement lost")
	}
}

func TestInactiveInspectionAndPhoenixDoesNotPostOrChangeOtherPilot(t *testing.T) {
	c := sourceFrontEnd(t, true)
	if err := c.Store.PrepareMissionSnapshot(); err != nil {
		t.Fatal(err)
	}
	st := c.Store.State()
	id := st.SelectedPilotID
	st.Slots[0].Status, st.Slots[0].LegacyStatus, st.Slots[0].LegacyRaw[0x56] = gamestate.StatusKIA, 4, 4
	other, err := gamestate.NewPilot()
	if err != nil {
		t.Fatal(err)
	}
	other.Name = "Other Pilot"
	other.Squadron = "Other Squadron"
	st.Slots[1] = other
	if err = c.Store.Commit(st); err != nil {
		t.Fatal(err)
	}
	c.Screen = ScreenRoster
	c.Highlight = 1
	act(t, c, Actions{Selector1: true})
	if c.Store.State().SelectedPilotID != other.ID {
		t.Fatal("active post failed")
	}
	otherBefore := c.Store.State().Slots[1]
	c.Screen = ScreenRoster
	c.Highlight = 0
	act(t, c, Actions{Selector1: true})
	if c.Screen != ScreenOffice || c.ViewedPilotID != id || c.Store.State().SelectedPilotID != other.ID || !c.Office.State.SnapshotAvailable {
		t.Fatal("inspection did not preserve posted identity")
	}
	act(t, c, Actions{X: 240, Y: 70, Inside: true, Selector1: true, PointerPressed: true})
	if c.Mission != nil {
		t.Fatal("viewing unavailable record launched posted other pilot")
	}
	officeClick(t, c, 300, 140)
	if c.Screen != ScreenRecovery || c.Recovery.State.SelectedPilotID != id {
		t.Fatalf("wrong Phoenix owner screen=%s recovery=%+v viewed=%s expected=%s message=%s hover=%d ptr=%d,%d cue=%v fade=%v draft=%v saveErr=%v", c.Screen, c.Recovery, c.ViewedPilotID, id, c.Message, c.Office.HoverKey, c.Office.PointerX, c.Office.PointerY, c.pointerCue, c.fade, c.Draft, c.missionSaveError)
	}
	act(t, c, Actions{Selector1: true})
	after := c.Store.State()
	if after.Slots[0].Status != gamestate.StatusActive || after.Slots[0].Statistics.PhoenixCounter != -1 || after.SelectedPilotID != other.ID || !reflect.DeepEqual(otherBefore, after.Slots[1]) {
		t.Fatal("recovery changed incorrect owner")
	}
	reopened, err := gamestate.OpenSaveStore(c.Store.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if reopened.State().Slots[0].Status != gamestate.StatusActive {
		t.Fatal("recovered status lost")
	}
	if rec, err := reopened.RecoveryCandidateForPilot(id); err != nil || rec != nil {
		t.Fatal("consumed Phoenix reappeared", err)
	}
}

func TestCrewSourceAreasInkAndPointerDomain(t *testing.T) {
	// Hand boundaries of source width1 AREA descriptors. Negative means sourcekey0.
	for _, q := range []struct{ n, x, y, want int }{{1, 3, 145, -1}, {1, 3, 146, 0}, {1, 3, 152, 0}, {1, 3, 153, -1}, {3, 3, 166, 2}, {3, 3, 167, -1}, {3, 3, 172, -1}, {3, 3, 173, 3}, {3, 3, 182, 3}, {3, 3, 183, -1}, {1, 4, 146, -1}, {0, 3, 146, -1}, {0, 3, 173, 0}} {
		if got := crewAreaAt(q.n, q.x, q.y); got != q.want {
			t.Fatal(q, got)
		}
	}
	if got := crewRecommendationText("PROMOTE %s to %c%s%c", []string{"Arthur", "CW2"}, 10, 7); got != "PROMOTE Arthur to \x87CW2\x8a" {
		t.Fatalf("%q", got)
	}
	if got := crewRecommendationText("AWARD %c%s%c TO %s", []string{"Air Medal", "Adams"}, 15, 15); got != "AWARD \x8fAir Medal\x8f TO Adams" {
		t.Fatalf("%q", got)
	}
	c := debriefDisplay(t)
	c.enterDebriefPage("crew")
	c.Debrief.Pending.Proposal.Result.Recommendations = []gamestate.SortieCrewRecommendation{{Actor: 1, Type: 0}}
	c.Office.PointerInside = true
	x, y := c.updateGamePointerAction(Actions{X: 280, Y: 100, Inside: true})
	if x != 3 || y != 146 {
		t.Fatal(x, y)
	}
	x, y = c.updateGamePointerAction(Actions{X: 280, Y: 199, Inside: true})
	if x != 3 || y != 182 {
		t.Fatal(x, y)
	}
	x, y = c.updateGamePointerAction(Actions{PointerRelative: true, PointerDY: -10, Inside: true})
	if x != 3 || y != 172 {
		t.Fatal("relative reverse had accumulated boundary debt", x, y)
	}
	before, _ := json.Marshal(c.Debrief.Pending.Proposal)
	if err := c.handleCrewDebrief(Actions{X: 3, Y: 172, Inside: true, Selector1: true}); err != nil || c.Debrief.Page != "crew" || c.Debrief.CrewSelection != -1 {
		t.Fatal("gap activated stale recipient/Accept", err)
	}
	if err := c.handleCrewDebrief(Actions{MenuEnd: true}); err != nil || c.Office.PointerX != 3 || c.Office.PointerY != 173 {
		t.Fatal("keyboard/pointer selection split", err)
	}
	c.Office.SetPointer(3, 173, true)
	plain, with := c.Frame(false), c.Frame(true)
	if bytes.Equal(plain, with) {
		t.Fatal("crew pointer suppressed")
	}
	art := c.Office.Assets.Images["POINTER.PIC"]
	for dy := 0; dy < 16; dy++ {
		for dx := 0; dx < 16; dx++ {
			v := art[dy*320+8*16+dx]
			if v != 0 && with[(173+dy)*320+3+dx] != v {
				t.Fatal("source pointer8/hotspot0 differs")
			}
		}
	}
	after, _ := json.Marshal(c.Debrief.Pending.Proposal)
	if !bytes.Equal(before, after) {
		t.Fatal("pointer/draw mutated proposal")
	}
	// Supplied local atlas sentinel exposes the y191 footer boundary even when
	// that source sprite's ordinary bottom texels happen to be transparent.
	copyArt := append([]byte(nil), art...)
	copyArt[15*320+8*16] = 14
	c.Office.Assets.Images["POINTER.PIC"] = copyArt
	c.Office.SetPointer(3, 182, true)
	plain, with = c.Frame(false), c.Frame(true)
	if with[197*320+3] != plain[197*320+3] {
		t.Fatal("pointer wrote beneath source191 clip")
	}
	c.enterDebriefPage("feedback")
	if x, y := c.updateGamePointerAction(Actions{X: 300, Y: 180, Inside: true}); x != 300 || y != 180 {
		t.Fatal("pointer confinement leaked")
	}
}

func TestCrewCaptionAcceptedAircraftDomain(t *testing.T) {
	c := debriefDisplay(t)
	for _, q := range []struct {
		id   byte
		name string
	}{{4, "KIOWA"}, {5, "BLACKHAWK"}, {6, "COMANCHE GUNSHIP"}, {7, "COMANCHE SCOUT"}} {
		if got := c.crewAircraftName(q.id); got != q.name {
			t.Fatalf("SHHD%d: %q want%q", q.id, got, q.name)
		}
	}
	b := originalGS(t)
	// CREWPRO reads a36-byte accepted header ID, then searches252-byte records
	// by byte0. The name comes from+26h; no compact/cockpit conversion occurs.
	sourceOperand(t, b, 0x369c0+0x90b, "6b5e06248a870cbb988946cc509a9c038115")
	sourceOperand(t, b, 0x369c0+0x92c, "69c0fc0003063ab88b163cb80526005250")
	sourceOperand(t, b, 0x17bac+0x1f, "695efefc00c4363ab8268a00983b46067507")
}
