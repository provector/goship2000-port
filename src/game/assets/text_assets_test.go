package assets

import (
	"bytes"
	"encoding/binary"
	"gunship2000-assets/internal/testfixture"
	"os"
	"path/filepath"
	"testing"
)

func TestReplayTextSourceBanksAndExactTerminators(t *testing.T) {
	d := replayOriginalMetadata(t)
	for name, want := range map[string]string{"mission_score": "MISSION SCORE: ", "time_taken": "TIME TAKEN: ", "total_kills": "TOTAL KILLS: ", "demo_title": "GUNSHIP 2000: DEMO MODE", "demo_copyright": "Copyright 1991 MicroProse Software Inc."} {
		if d.Text[name] != want {
			t.Fatalf("%s=%q", name, d.Text[name])
		}
	}
	raw, e := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(raw[0x4bc0e:0x4bc18], []byte(": Thanks!\x00")) {
		t.Fatal("notice21 source bytes")
	}
}

func TestPreparedCampaignParagraphsAndRescueReadiness(t *testing.T) {
	root := os.Getenv("GS2000_TEST_O1_ASSETS")
	if root == "" {
		t.Skip("set GS2000_TEST_O1_ASSETS to fresh Preparation25")
	}
	a, e := LoadFrontEndAssets(root)
	if e != nil {
		t.Fatal(e)
	}
	if a.Cockpit.Literals["notification-21"] != ": Thanks!" || len(a.Missions.Metadata.CampaignStatus) != 7 {
		t.Fatal("new typed readiness missing")
	}
	source, e := os.ReadFile(testfixture.Path("installation/GS.GS2"))
	if e != nil {
		t.Fatal(e)
	}
	for i, want := range []uint16{0x3119, 0x317d, 0x31d4, 0x321f, 0x3278, 0x32b8, 0x330b} {
		ds := binary.LittleEndian.Uint16(source[0x3d2f0+0x35ea+2*i:])
		if ds != want {
			t.Fatal("source pointertable", i, ds)
		}
		raw := source[0x3d2f0+int(ds):]
		end := bytes.IndexByte(raw, 0)
		if end < 0 || a.Missions.Metadata.CampaignStatus[i] != string(raw[:end]) {
			t.Fatal("source paragraph differs", i)
		}
	}
	broken := append([]string(nil), a.Missions.Metadata.CampaignStatus...)
	broken[0] += "!"
	if validateCampaignStatusParagraphs(broken) == nil {
		t.Fatal("wrong source paragraph accepted")
	}
	manifest, e := os.ReadFile(filepath.Join(root, "manifest.json"))
	if e != nil || !bytes.Contains(manifest, []byte(`"preparation_version": 25`)) {
		t.Fatal("not Preparation25", e)
	}
}
