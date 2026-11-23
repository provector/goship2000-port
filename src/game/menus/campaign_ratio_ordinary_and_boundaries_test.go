package menus

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"math"
	"os"
	"testing"
)

func TestCampaignRatioOrdinaryAndBoundaries(t *testing.T) {
	count := 0
	for total := 5; total <= 8; total++ {
		for success := 0; success <= total; success++ {
			for failure := 0; failure <= total; failure++ {
				c := gamestate.CampaignGenerationInputs{Raw50: total, Raw51: success, Raw52: failure}
				p, err := briefingCampaignProgress(c)
				want := 128 + (success-failure)*100/total
				if err != nil || int(p) != want {
					t.Fatal(c, p, want, err)
				}
				for _, w := range []int{121, 139} {
					if got := briefingRevealWidth(w, byte(p)); got != w*want/255 {
						t.Fatal(c, w, got)
					}
					count++
				}
			}
		}
	}
	if count != 460 {
		t.Fatal(count)
	}
	for _, c := range []struct{ total, success, failure, progress int }{{5, 0, 0, 128}, {5, 4, 0, 208}, {5, 0, 4, 48}, {100, 127, 0, 255}, {1, 127, 0, 255}, {1, 0, 127, 0}, {5, 128, 127, 0}} {
		p, err := briefingCampaignProgress(gamestate.CampaignGenerationInputs{Raw50: c.total, Raw51: c.success, Raw52: c.failure})
		if err != nil || int(p) != c.progress {
			t.Fatal(c, p, err)
		}
	}
	for _, total := range []int{0, 128, 255, 256, -1} {
		if _, err := briefingCampaignProgress(gamestate.CampaignGenerationInputs{Raw50: total}); err == nil {
			t.Fatal(total)
		}
	}
	for _, p := range []byte{0, 1, 127, 128, 254, 255} {
		for _, w := range []int{0, 1, 139, 32767} {
			got := briefingRevealWidth(w, p)
			if got < 0 || got > w {
				t.Fatal(w, p, got)
			}
		}
	}
	if briefingRevealWidth(math.MaxInt, 255) != -1 {
		t.Fatal("unchecked product overflow")
	}
}

func TestCampaignClipTranslationAndAtomicInvalidRecord(t *testing.T) {
	for _, offset := range [][4]int{{-8, 0, 0, 31}, {0, -3, 0, 31}, {0, 0, -6, 31}, {0, 0, 310, 197}, {175, 0, 0, 31}} {
		v, sheet := campaignIllustrationFixture()
		for i := range sheet {
			sheet[i] = byte((i/320*3+i%320)%251 + 1)
		}
		raw := v.Assets.Metadata.Screens["FIELD.SCN"].Chunks[0].Payload
		for key, value := range map[int]int{1: offset[0], 3: offset[1], 11: offset[2], 13: offset[3]} {
			binary.LittleEndian.PutUint16(raw[key:], uint16(int16(value)))
		}
		before := bytes.Clone(v.clean)
		if err := v.drawCampaignIllustration(); err != nil {
			t.Fatal(offset, err)
		}
		// Independent destination walk: only valid translated source/destination
		// pairs contribute. Tile0 suffix and tile1 prefix remain disjoint.
		for y := 0; y < 78; y++ {
			for x := 0; x < 139; x++ {
				sx := offset[0] + x
				if x < 69 {
					sx += 139
				}
				sy := offset[1] + y
				dx := offset[2] + x
				dy := offset[3] + y
				if sx >= 0 && sx < 320 && sy >= 0 && sy < 200 && dx >= 0 && dx < 320 && dy >= 0 && dy < 200 {
					before[dy*320+dx] = sheet[sy*320+sx]
				}
			}
		}
		if !bytes.Equal(before, v.clean) {
			t.Fatal("translation mismatch", offset)
		}
	}
	for _, bad := range []string{"atlas", "record", "divisor", "width", "vertical", "second-record", "state"} {
		v, _ := campaignIllustrationFixture()
		before := bytes.Clone(v.clean)
		screen := v.Assets.Metadata.Screens["FIELD.SCN"]
		switch bad {
		case "atlas":
			v.Assets.Images["FIELD_AN.PIC"] = nil
		case "record":
			screen.Chunks[0].Payload = screen.Chunks[0].Payload[:2]
		case "divisor":
			binary.LittleEndian.PutUint16(screen.Chunks[0].Payload[9:], 0)
		case "width":
			binary.LittleEndian.PutUint16(screen.Chunks[0].Payload[5:], 0xffff)
		case "vertical":
			screen.Chunks[0].Payload[15] = 1
		case "second-record":
			screen.Chunks = append(screen.Chunks, assetdata.OfficeChunk{Tag: "GRAF", Payload: []byte{0}})
		case "state":
			v.Mission.Inputs.Campaign = nil
		}
		v.Assets.Metadata.Screens["FIELD.SCN"] = screen
		if err := v.drawCampaignIllustration(); err == nil || !bytes.Equal(before, v.clean) {
			t.Fatal(bad, err)
		}
	}
}

func TestCampaignOptionalFailureKeepsActualBriefingAndState(t *testing.T) {
	root := os.Getenv("GS2000_TEST_MISSIONS")
	if root == "" {
		t.Fatal("mandatory prepared fixture missing")
	}
	a, err := assetdata.LoadMissionAssets(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, scene := range []string{"FIELD", "FIELD2", "FIELD3"} {
		m := &gamestate.MissionData{StableScene: scene, Scene: 2, Inputs: gamestate.MissionInputs{MissionType: gamestate.MissionCampaign, Campaign: &gamestate.CampaignGenerationInputs{Raw50: 5}}}
		p := &gamestate.Pilot{Name: "R8 TEST"}
		state, _ := json.Marshal(m)
		good, err := NewBriefingView(a, m, p, 1)
		if err != nil {
			t.Fatal(scene, err)
		}
		screen := a.Metadata.Screens[scene+".SCN"]
		name := screen.Basenames[1] + ".PIC"
		atlas := a.Images[name]
		a.Images[name] = nil
		fallback, err := NewBriefingView(a, m, p, 1)
		a.Images[name] = atlas
		if err != nil || fallback == nil || bytes.Equal(good.clean, fallback.clean) {
			t.Fatal("optional atlas blocked valid briefing", scene, err)
		}
		after, _ := json.Marshal(m)
		if !bytes.Equal(state, after) {
			t.Fatal("render mutated Campaign")
		}
		// Missing required background is still a core briefing error.
		background := a.Images[scene+".PIC"]
		a.Images[scene+".PIC"] = nil
		if _, err = NewBriefingView(a, m, p, 1); err == nil {
			t.Fatal("required background accepted")
		}
		a.Images[scene+".PIC"] = background
	}
}
