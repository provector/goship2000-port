package navigation

import (
	"bytes"
	"encoding/hex"
	"gunship2000-assets/game/cockpit"
	"gunship2000-assets/internal/testfixture"
	"os"
	"reflect"
	"testing"
)

func TestSupportSourcePredicatesAndLegend(t *testing.T) {
	b, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	// Independent actual source bytes. Section4 maps at file24290, not the
	// section1 dispatcher and not a live DOS address.
	for ip, value := range map[int]string{
		0xd24: "26f606a40af0", 0xd2f: "26f606a40a0f", 0xd3a: "2e803e23ad00",
		0x13c4: "26f606a40af0", 0x13cf: "26f606a40a0f", 0x13da: "2e803e23ad00",
		0xb39a: "6a206a0a6a056a4b68ac00688700", 0xb46e: "6a206a0a6a056a4b68b2006a04",
	} {
		want, _ := hex.DecodeString(value)
		if !bytes.Equal(b[0x24290+ip:][:len(want)], want) {
			t.Fatalf("original Mission Assets predicate/paint%x differs", ip)
		}
	}
	for _, low := range []bool{false, true} {
		for _, tc := range []struct {
			name         string
			availability byte
			request      uint16
			selected     int
			edit         bool
			want         bool
		}{{"artillery", 0x31, 0, 0, false, true}, {"air", 0x12, 0, 0, false, true},
			{"none", 0, 0, 0, false, false}, {"empty high", 1, 0, 0, false, false},
			{"empty kind", 0x30, 0, 0, false, false}, {"pending entry", 0x31, 1, 0, false, false},
			{"entry lowbyte", 0x31, 256, 0, false, true}, {"other member", 0x31, 0, 1, false, false},
			{"route edit", 0x31, 0, 0, true, false}} {
			t.Run(tc.name+map[bool]string{false: "/overview", true: "/detail"}[low], func(t *testing.T) {
				g := routeFixture()
				g.Low = low
				g.Selected = tc.selected
				g.Editing = tc.edit
				g.EnableSupport(tc.availability, tc.request, 0)
				before := *g.Routes
				if g.SupportAvailable() != tc.want {
					t.Fatal("admission", g.Support)
				}
				_, regions := g.legend()
				if tc.selected == 0 {
					x, y := 4, 178
					if low {
						x, y = 135, 172
					}
					disabled := false
					for _, r := range regions {
						if r.x == x && r.y == y && r.width == 75 && r.next == 32 {
							disabled = true
						}
					}
					if disabled == tc.want {
						t.Fatal("paint and command disagree", regions)
					}
				}
				g.Handle(Input{Key: "m"})
				if g.Support.Editing != tc.want || !reflect.DeepEqual(before, *g.Routes) {
					t.Fatal("actual key owner", g.Support)
				}
				if tc.want {
					_, regions = g.legend()
					x, y := 4, 178
					if low {
						x, y = 135, 172
					}
					selected := false
					for _, r := range regions {
						if r.x == x && r.y == y && r.width == 12 && r.next == 15 {
							selected = true
						}
					}
					if !selected {
						t.Fatal("M selected tag missing", regions)
					}
				}
			})
		}
	}
}

func TestSupportCaptionUsesSourceTypeAndAvailability(t *testing.T) {
	// One-pixel glyph advances make source caption origins/count cells directly
	// distinguishable without using the renderer's output as its own oracle.
	a := &cockpit.Assets{Font: cockpit.Font{First: 32, Last: 126, Width: 1, Height: 6, Advances: make([]byte, 95), Rows: make([]byte, 95*6)}, Literals: map[string]string{
		"gps-controls-overview": "\n", "gps-controls": "\n", "gps-support-aircraft": "- Aircraft", "gps-support-artillery": "- Artillery : 3"}}
	for i := range a.Font.Advances {
		a.Font.Advances[i] = 1
		a.Font.Rows[i*6] = 0x80
	}
	for _, low := range []bool{false, true} {
		for _, availability := range []byte{0, 0x31, 0x21, 0x11, 0x12} {
			g := routeFixture()
			g.Low = low
			g.EnableSupport(availability, 0, 0)
			frame := make([]byte, 64000)
			before := g.ReplayState()
			if err := g.drawLegend(frame, a, cockpit.Full); err != nil {
				t.Fatal(err)
			}
			x, y := 76, 178
			if low {
				x, y = 207, 172
			}
			if (frame[y*320+x] == 2) != (availability != 0) {
				t.Fatal("source type caption eligibility", low, availability)
			}
			if !reflect.DeepEqual(before, g.ReplayState()) {
				t.Fatal("drawing changed GPS state")
			}
			if availability&15 == 1 && frame[y*320+x+14] != 2 {
				t.Fatal("count source cell missing", availability)
			}
		}
	}
}
