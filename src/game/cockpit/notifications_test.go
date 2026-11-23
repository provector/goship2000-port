package cockpit

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"
	"testing"
)

func TestNotificationSourceOperandsAndTwentyLiterals(t *testing.T) {
	b := mapMastOriginal(t)
	// Actual graphics ABI: source pushes paired colour, y+6, x163+half,
	// y, x156-half; text sets ink15 and uses font2 independently.
	for at, raw := range map[int][]byte{
		0x6fb0: {0x68, 0x29, 0x2a, 0x68, 0x71, 0x04}, // actor0 copies HQ2a29:0471
		0x701b: {0x6a, 0x0f, 0x9a, 0x50, 0, 0xe0, 7},
		0x7042: {0x05, 0x06, 0, 0x50, 0x8d, 0x85, 0xa3, 0},
		0x7061: {0x6a, 2},
		0x707c: {0x80, 0x3e, 0xe0, 0, 0},
	} {
		if !bytes.Equal(b[0xd8b0+at:][:len(raw)], raw) {
			t.Fatalf("original operand%04x", at)
		}
	}
	if !bytes.Equal(b[0x4bc21:0x4bc24], []byte{'H', 'Q', 0}) {
		t.Fatal("original actor0 HQ carrier differs")
	}
	texts := []string{": I've reached destination", ": I'm engaging enemy ", ": I'm going in...", ": I'm taking damage", ": My fuel is low", ": My fuel is critical", ": I'm formed-up", ": I've landed", ": I've reached safe zone", ": Permission to fire!", ": Objective sighted!", ": Exiting combat zone", ": Change secondary to ", ": Farp knocked out", "FARP: we're under attack", ": primary achieved!", ": secondary achieved!", ": Downed pilot sighted!", ": Downed pilot rescued!", "- MESSAGES -"}
	for i, text := range texts {
		pointer := int(binary.LittleEndian.Uint16(b[0x4b7b0+0x472+(i+1)*2:]))
		if !bytes.Equal(b[0x4b7b0+pointer:][:len(text)+1], append([]byte(text), 0)) {
			t.Fatalf("source text%d", i+1)
		}
	}
}

func TestNotificationHQNamesAndSuffixesFromOriginal(t *testing.T) {
	b := mapMastOriginal(t)
	a := testAssets()
	a.Literals["notification-hq"] = string(b[0x4bc21:0x4bc23])
	for _, kind := range []int{2, 13, 16, 17} {
		pointer := int(binary.LittleEndian.Uint16(b[0x4b7b0+0x472+kind*2:]))
		raw := b[0x4b7b0+pointer:]
		a.Literals[fmt.Sprintf("notification-%02d", kind)] = string(raw[:bytes.IndexByte(raw, 0)])
	}
	in := &NotificationInputs{ObjectNames: []string{"", "VIPER", "TARGET"}, SecondaryName: "BRIDGE"}
	for _, q := range []struct {
		row  [4]byte
		want string
	}{
		{[4]byte{16, 0, 23}, "HQ: primary achieved!"},
		{[4]byte{17, 0, 23}, "HQ: secondary achieved!"},
		{[4]byte{13, 0, 23}, "HQ: Change secondary to BRIDGE"},
		{[4]byte{16, 1, 23}, "VIPER: primary achieved!"},
		{[4]byte{2, 1, 23, 2}, "VIPER: I'm engaging enemy TARGET"},
	} {
		got, err := notificationText(a, in, q.row)
		if err != nil || got != q.want {
			t.Fatalf("row%v: %q, %v; want%q", q.row, got, err, q.want)
		}
	}
	delete(a.Literals, "notification-hq")
	if _, err := notificationText(a, in, [4]byte{16, 0, 23}); err == nil {
		t.Fatal("missing prepared HQ silently omitted")
	}
}

func TestNotificationLayoutColourAndReadOnlyScaledComposition(t *testing.T) {
	a := testAssets()
	a.Literals["notification-16"] = "AB"
	a.Literals["notification-hq"] = "HQ"
	in := &NotificationInputs{Rows: [4][4]byte{{}, {16, 0, 23}}, ObjectNames: []string{"", "C"}, LeaderA: -1, LeaderB: -1}
	before := *in
	// Testfont advance6 gives HQAB width24, half12. Background144..175;
	// text148,9. These expectations follow literal source156/163/160,y+1.
	for _, scale := range []int{1, 2} {
		p := bytes.Repeat([]byte{99}, 64000*scale*scale)
		if err := DrawNotificationsScaled(p, a, in, scale); err != nil {
			t.Fatal(err)
		}
		at := func(x, y int) byte { return p[y*scale*320*scale+x*scale] }
		if at(143, 8) != 99 || at(144, 8) != 4 || at(175, 14) != 4 || at(176, 14) != 99 || at(148, 9) != 15 || at(144, 15) != 99 {
			t.Fatal("source rectangle/text", scale)
		}
		if !reflect.DeepEqual(before, *in) {
			t.Fatal("draw mutated logical notification")
		}
	}
	for _, c := range []struct {
		actor  byte
		a, b   int8
		player byte
		colour byte
	}{{0, 0, 0, 0, 4}, {5, 0, 0, 0, 4}, {1, 0, -1, 0, 9}, {1, -1, 0, 0, 9}, {3, -1, 0, 0, 7}, {1, -1, -1, 0, 7}, {2, -1, -1, 0, 9}} {
		if got := notificationColour(c.actor, NotificationInputs{Player: c.player, LeaderA: c.a, LeaderB: c.b}); got != c.colour {
			t.Fatal("source selector colour", c, got)
		}
	}
	// Actor names and kind2 argument names use separate source OBJECT indices.
	a.Literals["notification-02"] = ":"
	in.Rows = [4][4]byte{{}, {2, 1, 23, 1}}
	if err := DrawNotifications(make([]byte, 64000), a, in); err != nil {
		t.Fatal(err)
	}
	in.Rows[1][3] = 255
	if err := DrawNotifications(make([]byte, 64000), a, in); err == nil {
		t.Fatal("unknown signed OBJECT domain must fail explicitly")
	}
}
