package assets

import "fmt"

var debriefSceneNames = []string{"COMMISSN.SCN", "JUMP.SCN", "HAPPY.SCN", "SAD.SCN", "RESCUE.SCN", "KIA.SCN", "CREWPRO.SCN", "WIN.SCN", "LOSE.SCN", "REPLACE.SCN"}
var debriefImageNames = []string{"COMMISSN.PIC", "COMM_AN.PIC", "JUMP.PIC", "JUMPCMOH.PIC", "BPATCHES.PIC", "RANKS.PIC", "HAPPY_2.PIC", "HAPPY_AN.PIC", "SAD_2.PIC", "SAD_AN.PIC", "MEDAL.PIC", "RESCUE.PIC", "RESC_AN.PIC", "RESC_AN1.PIC", "KIA.PIC", "KIA_AN.PIC", "PATCHES.PIC", "PILOT.PIC", "WIN.PIC", "LOSE.PIC", "FIELD.PIC", "MEMO.PIC", "HAPPY.PIC", "HAPPY_3.PIC", "SAD.PIC", "SAD_3.PIC"}
var debriefPaletteNames = []string{"COMM.PAL", "JUMP.PAL", "HAPPY.PAL", "SAD_2.PAL", "RESCUE.PAL", "KIA.PAL", "FIELD.PAL", "WIN.PAL", "LOSE.PAL", "SAD.PAL", "SAD_3.PAL"}

// FeedbackSceneMedia follows source A26D. Historical callers retain their
// prepared carrier variant; new frozen context supplies this source-owned kind.
func FeedbackSceneMedia(scene string, kind byte) (picture, palette string, err error) {
	if scene != "SAD.SCN" && scene != "HAPPY.SCN" {
		return "", "", fmt.Errorf("not a feedback scene")
	}
	if kind > 5 {
		return "", "", fmt.Errorf("feedback scene kind%d outside0..5", kind)
	}
	base := scene[:len(scene)-4]
	suffix := ""
	if kind == 2 {
		suffix = "_2"
	} else if kind == 4 || kind == 5 {
		suffix = "_3"
	}
	palette = "HAPPY.PAL"
	if base == "SAD" {
		palette = base + suffix + ".PAL"
	}
	return base + suffix + ".PIC", palette, nil
}

// CompositeGroup executes only prepared source CMPX rectangles. RESCUE groups
// restore their own affected pieces; zero remains transparent, as in1658:0117.
func (a *DebriefAssets) CompositeGroup(dst []byte, scene string, group int) error {
	if a == nil || len(dst) != 64000 {
		return fmt.Errorf("debrief composite target")
	}
	s, ok := a.Scenes[scene]
	if !ok {
		return fmt.Errorf("debrief composite scene")
	}
	found := false
	for _, v := range s.Components {
		if v.Group != group {
			continue
		}
		found = true
		if err := OfficeBlit(a.Images[v.SourceAsset], dst, v.Source, v.Destination[0], v.Destination[1], true); err != nil {
			return err
		}
	}
	if !found {
		return fmt.Errorf("debrief composite group%d missing", group)
	}
	return nil
}

func (a *DebriefAssets) RescueParagraph(name string, variant int) string {
	if name == "" {
		return a.Text[0x606a]
	}
	ds := 0x5f84
	if variant&1 != 0 {
		ds = 0x5feb
	}
	return fmt.Sprintf(a.Text[ds], name)
}

// The victory/defeat artwork follows the Campaign verdict, but the paragraph
// independently follows8612 (the sortie success flag), perA1b591.
func (a *DebriefAssets) CampaignParagraph(sortieSuccess bool) string {
	if sortieSuccess {
		return a.Text[0x36e]
	}
	return a.Text[0x2e8]
}
