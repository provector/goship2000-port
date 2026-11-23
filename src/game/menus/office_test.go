package menus

import (
	"bytes"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"os"
	"strings"
	"testing"
)

func TestOfficeBlitPreservesMaskedZerosAndOrder(t *testing.T) {
	src, dst := make([]byte, 64000), bytes.Repeat([]byte{9}, 64000)
	src[0], src[1], src[2] = 1, 0, 2
	if err := assetdata.OfficeBlit(src, dst, [4]int{0, 0, 3, 1}, 5, 4, true); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(dst[4*320+5:4*320+8], []byte{1, 9, 2}) {
		t.Fatal("transparent zero changed destination")
	}
	if err := assetdata.OfficeBlit(src, dst, [4]int{1, 0, 2, 1}, 6, 4, false); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(dst[4*320+5:4*320+8], []byte{1, 0, 2}) {
		t.Fatal("opaque overwrite order differs")
	}
}

func TestOfficeBlitExtractsBeforeAliasedPlacement(t *testing.T) {
	image := make([]byte, 64000)
	copy(image, []byte{1, 2, 3, 4, 5})
	if err := assetdata.OfficeBlit(image, image, [4]int{0, 0, 4, 1}, 1, 0, false); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(image[:5], []byte{1, 1, 2, 3, 4}) {
		t.Fatal("copy read overwritten source")
	}
	if err := assetdata.OfficeBlit(image, image, [4]int{319, 0, 2, 1}, 0, 0, true); err == nil {
		t.Fatal("silently clamped source")
	}
}

func TestOfficeLineInclusiveTieRule(t *testing.T) {
	image := make([]byte, 64000)
	if err := assetdata.OfficeLine(image, 291, 45, 300, 44, 4); err != nil {
		t.Fatal(err)
	}
	want := map[[2]int]bool{{291, 45}: true, {292, 45}: true, {293, 45}: true, {294, 45}: true, {295, 45}: true, {296, 44}: true, {297, 44}: true, {298, 44}: true, {299, 44}: true, {300, 44}: true}
	for y := 44; y <= 45; y++ {
		for x := 291; x <= 300; x++ {
			if (image[y*320+x] == 4) != want[[2]int{x, y}] {
				t.Fatalf("unexpected line pixel %d,%d", x, y)
			}
		}
	}
	if err := assetdata.OfficeLine(image, 0, 0, 0, 0, 7); err != nil || image[0] != 7 {
		t.Fatal("single inclusive endpoint missing")
	}
}

func syntheticOffice(mode int) *Office {
	m := assetdata.OfficeMetadata{}
	holds := []int{2, 1, 4, 5, 8}
	counts := []int{2, 40, 80, 2, 2}
	for i := 0; i < 5; i++ {
		c := assetdata.OfficeAnimation{Key: i, HoldCount: holds[i], FrameCount: counts[i], Asset: "BASE_AN.PIC"}
		for j := 0; j < counts[i]; j++ {
			c.Frames = append(c.Frames, assetdata.OfficeFrame{Source: [4]int{j % 20, 0, 1, 1}, Destination: [2]int{136, 85}})
		}
		m.Animations = append(m.Animations, c)
	}
	for i := 0; i <= 10; i++ {
		m.Groups = append(m.Groups, assetdata.OfficeGroup{Key: i})
	}
	a := &assetdata.OfficeAssets{Metadata: m, Images: map[string][]byte{}}
	for _, name := range []string{"BASE.PIC", "BASE_AN.PIC", "BASE_AN1.PIC", "BASE_AN2.PIC", "POINTER.PIC"} {
		a.Images[name] = make([]byte, 64000)
	}
	for i := 0; i < 20; i++ {
		a.Images["BASE_AN.PIC"][i] = byte(i + 1)
	}
	o := &Office{Assets: a, State: gamestate.OfficeState{ComputerMode: mode}, Scene: make([]byte, 64000), rng: 1, OfficerGroup: -1}
	for i := 0; i < 5; i++ {
		o.Animations = append(o.Animations, officeAnimationState{Paused: i != mode})
	}
	return o
}

func TestOfficeAnimationAdvanceBeforeDrawAndHolds(t *testing.T) {
	o := syntheticOffice(1)
	o.advanceAnimations()
	if o.Animations[1].Frame != 1 || o.Scene[85*320+136] != 2 {
		t.Fatal("first tick must draw frame1")
	}
	o = syntheticOffice(2)
	for i := 0; i < 3; i++ {
		o.advanceAnimations()
	}
	if o.Animations[2].Frame != 0 || o.Scene[85*320+136] != 0 {
		t.Fatal("hold expired early")
	}
	o.advanceAnimations()
	if o.Animations[2].Frame != 1 || o.Scene[85*320+136] != 2 {
		t.Fatal("hold4 did not advance before drawing")
	}
}

func TestOfficeUnexpiredHoldReturnsFromTraversal(t *testing.T) {
	o := syntheticOffice(0)
	o.Animations[1].Paused = false
	o.advanceAnimations()
	if o.Animations[0].Hold != 1 || o.Animations[1].Hold != 0 {
		t.Fatal("later animation progressed after early return")
	}
}

func TestOfficeOfficerSharedCallbackPeriod(t *testing.T) {
	o := syntheticOffice(1)
	for i := 0; i < 12; i++ {
		o.sharedUpdate()
	}
	if o.OfficerGroup != -1 {
		t.Fatal("ambient group drawn too early")
	}
	o.sharedUpdate()
	if o.OfficerGroup != 0 || o.OfficerCounter != 0 {
		t.Fatal("thirteenth callback must draw group0")
	}
	for i := 0; i < 13*11; i++ {
		o.sharedUpdate()
	}
	if o.OfficerGroup != 0 {
		t.Fatal("ambient group did not wrap after10")
	}
}

func TestOfficeForcedCallbacksDespitePauseReverseOrder(t *testing.T) {
	o := syntheticOffice(0)
	o.Computer.Reset = true
	o.ForceRedraw()
	var keys []int
	for _, e := range o.LastEvents {
		if e.Operation == "animation" {
			keys = append(keys, e.Key)
		}
	}
	if len(keys) != 3 || keys[0] != 4 || keys[1] != 3 || keys[2] != 0 {
		t.Fatalf("force order %v", keys)
	}
	if o.Computer.RedrawParity != 1 {
		t.Fatal("forced callback4 not executed")
	}
}

func TestOfficeAllComputerModesRemainInBoundsAndDeterministic(t *testing.T) {
	for mode := 0; mode < 4; mode++ {
		a, b := syntheticOffice(mode), syntheticOffice(mode)
		a.Computer.Reset = true
		b.Computer.Reset = true
		if mode == 3 {
			a.generateBlock(0)
			b.generateBlock(0)
		}
		for i := 0; i < 2000; i++ {
			a.Tick()
			b.Tick()
		}
		if !bytes.Equal(a.Scene, b.Scene) {
			t.Fatalf("mode%d nondeterministic", mode)
		}
		if a.Computer.Cursor < 0 || a.Computer.Cursor > 14 {
			t.Fatalf("terminal cursor %d outside rectangle", a.Computer.Cursor)
		}
		for y := 0; y < 200; y++ {
			for x := 0; x < 320; x++ {
				if (x < 136 || x >= 151 || y < 85 || y >= 94) && a.Scene[y*320+x] != 0 {
					t.Fatalf("mode%d wrote outside computer at%d,%d", mode, x, y)
				}
			}
		}
	}
}

func TestOfficeAREAReversePriorityAndZeroFlagHit(t *testing.T) {
	o := syntheticOffice(0)
	o.Assets.Metadata.Areas = []assetdata.OfficeArea{{Key: 0, Rectangle: [4]int{0, 0, 320, 191}}, {Key: 6, Rectangle: [4]int{136, 85, 15, 9}}, {Key: 3, Rectangle: [4]int{0, 160, 244, 32}, VisualFlag: 1}}
	o.SetPointer(150, 93, true)
	if o.HoverKey != 6 || o.pointerStyle != 0 {
		t.Fatal("zero visualflag incorrectly disabled hit")
	}
	o.SetPointer(151, 93, true)
	if o.HoverKey != 0 {
		t.Fatal("exclusive end accepted")
	}
	o.SetPointer(0, 160, true)
	if o.HoverKey != 3 || o.pointerStyle != 1 {
		t.Fatal("priority or hoverstyle differs")
	}
}

func TestOfficeStateRejectsUnsupportedSelectorsAndByteText(t *testing.T) {
	m := assetdata.OfficeMetadata{Ranks: make([]assetdata.OfficeTextSource, 12), Missions: make([]assetdata.OfficeTextSource, 5), Theatres: make([]assetdata.OfficeTheatre, 4)}
	s := DefaultOfficeState()
	if err := validateOfficeState(s, m); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []gamestate.OfficeState{{PilotName: "é", Emblem: 1, Theatre: 1}, {ComputerMode: 4}, {MissionType: -1}, {Emblem: 18}, {Theatre: 4}} {
		if err := validateOfficeState(bad, m); err == nil {
			t.Fatalf("accepted unsupportedstate %+v", bad)
		}
	}
}

func TestOfficePreparedAssetsIntegration(t *testing.T) {
	path := os.Getenv("GS2000_OFFICE_ASSETS")
	if path == "" {
		t.Skip("set GS2000_OFFICE_ASSETS for source-backed office integration")
	}
	assets, err := assetdata.LoadOfficeAssets(path)
	if err != nil {
		t.Fatal(err)
	}
	original := map[string][]byte{}
	for name, pixels := range assets.Images {
		original[name] = append([]byte(nil), pixels...)
	}
	for mode := 0; mode < 4; mode++ {
		state := DefaultOfficeState()
		state.ComputerMode = mode
		a, err := NewOffice(assets, state, 123)
		if err != nil {
			t.Fatal(err)
		}
		b, err := NewOffice(assets, state, 123)
		if err != nil {
			t.Fatal(err)
		}
		for tick := 0; tick < 750; tick++ {
			a.Tick()
			b.Tick()
		}
		if !bytes.Equal(a.Frame(true), b.Frame(true)) {
			t.Fatalf("prepared mode%d nondeterministic", mode)
		}
		// Pointer layers are copies, never destructive updates of the persistent bank.
		before := append([]byte(nil), a.Scene...)
		a.SetPointer(319, 190, true)
		a.Frame(true)
		a.SetPointer(0, 0, true)
		a.Frame(true)
		if !bytes.Equal(before, a.Scene) {
			t.Fatal("pointer left pixels in persistent scene")
		}
	}
	for name, pixels := range assets.Images {
		if !bytes.Equal(original[name], pixels) {
			t.Fatalf("immutable atlas %s modified", name)
		}
	}
}

func TestReadOfficeStateStrictOverlay(t *testing.T) {
	path := t.TempDir() + "/state.json"
	if err := os.WriteFile(path, []byte(`{"computer_mode":2}`), 0600); err != nil {
		t.Fatal(err)
	}
	state, err := ReadOfficeState(path)
	if err != nil || state.PilotName != "Test Pilot Name" || state.ComputerMode != 2 {
		t.Fatalf("overlay %+v: %v", state, err)
	}
	for _, raw := range []string{`null`, `[]`, `{"computermode":2}`, `{} {}`, string(bytes.Repeat([]byte{' '}, 65537))} {
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadOfficeState(path); err == nil {
			t.Fatalf("accepted invalid JSON state %q", raw[:min(len(raw), 40)])
		}
	}
}

func TestOfficeComputerCycleRetainsSafeIndependentState(t *testing.T) {
	o := syntheticOffice(0)
	o.Computer.Reset = true
	for cycle := 0; cycle < 12; cycle++ {
		want := (cycle + 1) % 4
		o.CycleComputerMode()
		if o.State.ComputerMode != want {
			t.Fatal("computer cycle", o.State.ComputerMode, want)
		}
		for tick := 0; tick < 200; tick++ {
			o.Tick()
		}
		if o.Computer.Count < 0 || o.Computer.Count > 3 || o.Computer.CursorX < 0 || o.Computer.CursorX > 14 || o.Computer.CursorY < 0 || o.Computer.CursorY > 8 {
			t.Fatal("mode transition escaped computer bounds")
		}
	}
}

func TestOfficeNameEditorAndDisplayLimitsAreDistinct(t *testing.T) {
	m := assetdata.OfficeMetadata{Ranks: make([]assetdata.OfficeTextSource, 12), Missions: make([]assetdata.OfficeTextSource, 5), Theatres: make([]assetdata.OfficeTheatre, 4)}
	s := DefaultOfficeState()
	s.PilotName = strings.Repeat("A", 35)
	if err := validateOfficeState(s, m); err != nil {
		t.Fatal(err)
	}
	if got := assetdata.OfficePilotLabel(s.PilotName, " WOC "); got != "WOC "+strings.Repeat("A", 30) {
		t.Fatal("final-token copy limit", got)
	}
	if got := assetdata.OfficePilotLabel(" Test Pilot Name ", "WOC"); got != "WOC Name" {
		t.Fatal(got)
	}
	s.PilotName += "A"
	if validateOfficeState(s, m) == nil {
		t.Fatal("accepted beyond supported35-byte editor input")
	}
	s = DefaultOfficeState()
	s.Squadron = strings.Repeat("S", 26)
	if validateOfficeState(s, m) == nil {
		t.Fatal("accepted beyond25-byte unit input")
	}
}
