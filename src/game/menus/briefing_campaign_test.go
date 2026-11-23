package menus

import (
	"encoding/binary"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"os"
	"testing"
)

func TestBriefingCampaignArithmeticUsesSignedWordsAndALOnly(t *testing.T) {
	// Retained original-arithmetic oracle, not the selected R8 display policy.
	legacyProgress := func(c gamestate.CampaignGenerationInputs) (uint16, error) {
		for _, raw := range []int{c.Raw50, c.Raw51, c.Raw52} {
			if raw < 0 || raw > 255 {
				return 0, fmt.Errorf("not a byte")
			}
		}
		div := int(int8(c.Raw50))
		if div == 0 {
			return 0, fmt.Errorf("zero divisor")
		}
		q := uint16(int16(int(int16((int(int8(c.Raw51))-int(int8(c.Raw52)))*100)) / div))
		return q&0xff00 | uint16(byte(byte(q)+128)), nil
	}
	legacyWidth := func(w int, p byte) int { return int(int16(w*int(p))) / 255 }
	cases := []struct {
		total, gained, lost int
		word                uint16
		reveal              int
	}{{5, 0, 0, 0x0080, 69}, {5, 1, 0, 0x0094, 80}, {5, 0, 1, 0xff6c, 58}, {255, 128, 127, 0x631c, 15}}
	for _, c := range cases {
		got, err := legacyProgress(gamestate.CampaignGenerationInputs{Raw50: c.total, Raw51: c.gained, Raw52: c.lost})
		if err != nil || got != c.word {
			t.Fatal(c, got, err)
		}
		if width := legacyWidth(139, byte(got)); width != c.reveal {
			t.Fatal(c, width)
		}
	}
	// Independently:139*255=35445 -> signed lowword-30091; IDIV255=-118.
	if got := legacyWidth(139, 255); got != -118 {
		t.Fatal(got)
	}
	if _, err := legacyProgress(gamestate.CampaignGenerationInputs{}); err == nil {
		t.Fatal("zero native divisor silently accepted")
	}
	if _, err := legacyProgress(gamestate.CampaignGenerationInputs{Raw50: 5, Raw51: 256}); err == nil {
		t.Fatal("non-byte source state accepted")
	}
}

func campaignIllustrationFixture() (*BriefingView, []byte) {
	sheet := make([]byte, 64000)
	for y := 0; y < 78; y++ {
		for x := 0; x < 139; x++ {
			sheet[y*320+x] = 3
			sheet[y*320+x+139] = 7
		}
	}
	raw := make([]byte, 18)
	for offset, value := range map[int]int{5: 139, 7: 78, 9: 1, 11: 0, 13: 31} {
		binary.LittleEndian.PutUint16(raw[offset:], uint16(value))
	}
	screen := assetdata.MissionScreen{Basenames: [3]string{"FIELD", "FIELD_AN", "COMMANDR"}, Chunks: []assetdata.OfficeChunk{{Tag: "GRAF", Payload: raw}}}
	a := &assetdata.MissionAssets{Images: map[string][]byte{"FIELD_AN.PIC": sheet}, Metadata: assetdata.MissionPresentationData{Screens: map[string]assetdata.MissionScreen{"FIELD.SCN": screen}}}
	m := &gamestate.MissionData{StableScene: "FIELD", Inputs: gamestate.MissionInputs{MissionType: gamestate.MissionCampaign, Campaign: &gamestate.CampaignGenerationInputs{Raw50: 5}}}
	return &BriefingView{Assets: a, Mission: m, clean: make([]byte, 64000)}, sheet
}

func TestBriefingCampaignRevealPartitionsOriginalTiles(t *testing.T) {
	v, sheet := campaignIllustrationFixture()
	if err := v.drawCampaignIllustration(); err != nil {
		t.Fatal(err)
	}
	// Initial source progress128 gives69 columns from tile1, then70 from tile0.
	for y := 31; y < 109; y++ {
		for x := 0; x < 139; x++ {
			want := byte(3)
			if x < 69 {
				want = 7
			}
			if got := v.clean[y*320+x]; got != want {
				t.Fatal(x, y, got, want)
			}
		}
	}
	if v.clean[30*320] != 0 || v.clean[109*320] != 0 || v.clean[31*320+139] != 0 || sheet[0] != 3 || sheet[139] != 7 {
		t.Fatal("crop wrote outside destination or changed immutable source")
	}
}

func TestCampaignExtremeCropAndInvalidVertical(t *testing.T) {
	v, _ := campaignIllustrationFixture()
	v.Mission.Inputs.Campaign = &gamestate.CampaignGenerationInputs{Raw50: 100, Raw51: 127}
	// Supplied extreme: original signed16 width was-118; R8 reveals139.
	if err := v.drawCampaignIllustration(); err != nil {
		t.Fatal(err)
	}
	for y := 31; y < 109; y++ {
		for x := 0; x < 139; x++ {
			if v.clean[y*320+x] != 7 {
				t.Fatal("full revealed prefix missing")
			}
		}
	}
	v, _ = campaignIllustrationFixture()
	v.Assets.Metadata.Screens["FIELD.SCN"].Chunks[0].Payload[15] = 1
	if err := v.drawCampaignIllustration(); err == nil {
		t.Fatal("unsupported vertical branch was silently horizontal")
	}
}

func TestBriefingCampaignPreparedSourceCropsWhenRequested(t *testing.T) {
	root := os.Getenv("GS2000_TEST_MISSIONS")
	if root == "" {
		t.Skip("set GS2000_TEST_MISSIONS for source GRAF integration")
	}
	a, err := assetdata.LoadMissionAssets(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, scene := range []struct {
		name, atlas      string
		width, height, y int
	}{{"FIELD", "FIELD_AN", 139, 78, 31}, {"FIELD2", "FIELD_A2", 121, 61, 63}, {"FIELD3", "FIELD_A2", 121, 61, 63}} {
		v := &BriefingView{Assets: a, Mission: &gamestate.MissionData{StableScene: scene.name, Inputs: gamestate.MissionInputs{MissionType: gamestate.MissionCampaign, Campaign: &gamestate.CampaignGenerationInputs{Raw50: 5}}}, clean: append([]byte(nil), a.Images[scene.name+".PIC"]...)}
		if err = v.drawCampaignIllustration(); err != nil {
			t.Fatal(scene.name, err)
		}
		reveal := scene.width * 128 / 255 // products17792/15488 fit signed16 here.
		source := a.Images[scene.atlas+".PIC"]
		for y := 0; y < scene.height; y++ {
			for x := 0; x < scene.width; x++ {
				sx := x
				if x < reveal {
					sx += scene.width
				}
				if v.clean[(y+scene.y)*320+x] != source[y*320+sx] {
					t.Fatal("source keyed reveal mismatch", scene.name, x, y)
				}
			}
		}
	}
}
