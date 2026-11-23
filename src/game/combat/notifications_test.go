package combat

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestNotificationQueueSourceHistoryAndSignedDuplicateDomain(t *testing.T) {
	n := NotificationState{}
	if !n.Queue(16, 0, 0) || n.Rows != [4][4]byte{{16, 0, 0, 0}, {16, 0, 24, 0}} {
		t.Fatal("6d94 mode0 mutation", n)
	}
	if n.Queue(16, 0, 0) {
		t.Fatal("first history duplicate")
	}
	n.Rows[1] = [4]byte{1, 1, 1, 1}
	n.Rows[2] = [4]byte{2, 2, 1, 2}
	n.Rows[3] = [4]byte{3, 3, 1, 3}
	if !n.Queue(3, 3, 3) {
		t.Fatal("row3 must not participate in duplicate scan")
	}
	n.Rows[1] = [4]byte{4, 255, 1, 255}
	if n.Queue(4, -1, -1) || !n.Queue(4, 255, 255) {
		t.Fatal("CBW versus caller16 domain")
	}
	n.Rows[1][2] = 0
	if !n.Queue(4, -1, -1) {
		t.Fatal("first-history counter gates entire scan")
	}
	n = NotificationState{HistoryMode: 1}
	n.Queue(17, 2, 3)
	if n.Rows != [4][4]byte{{20, 0, 12, 0}, {17, 2, 0, 3}} {
		t.Fatal("mode1 source headline/history", n)
	}
}

func TestNotificationConsumerPreCounter23E0AndHeadline(t *testing.T) {
	n := NotificationState{}
	n.Queue(16, 0, 0)
	if got := n.Advance(true, true); len(got) != 0 || n.Rows[1][2] != 23 || n.Display[1][2] != 24 {
		t.Fatal("new24 then23", n, got)
	}
	if got := n.Advance(false, true); !reflect.DeepEqual(got, []uint16{0x14}) || n.Rows[1][2] != 23 {
		t.Fatal("E0false source repeat", n, got)
	}
	if got := n.Advance(true, true); !reflect.DeepEqual(got, []uint16{0x14}) || n.Rows[1][2] != 22 {
		t.Fatal("sound before decrement", n, got)
	}
	n.Rows = [4][4]byte{{20, 0, 1}, {16, 0, 23}, {17, 0, 23}, {14, 0, 23}}
	if got := n.Advance(true, true); len(got) != 0 || n.Rows[1][2] != 23 || n.Display != [4][4]byte{{20, 0, 1}} {
		t.Fatal("headline hides history through final frame", n)
	}
	if got := n.Advance(true, true); !reflect.DeepEqual(got, []uint16{0x14, 0x14, 0x14}) {
		t.Fatal("three source calls", got)
	}
	n.Rows[1][2] = 23
	if got := n.Advance(true, false); len(got) != 0 || n.Rows[1][2] != 22 {
		t.Fatal("DE guard independent counter lifetime", n, got)
	}
}

func TestRescuedActorThanksAndHistoricalQueueProfile(t *testing.T) {
	for _, actor := range []int16{1, 4} {
		n := NotificationState{}
		if !n.Queue(21, actor, 0) || n.Rows[1] != [4]byte{21, byte(actor), 24, 0} {
			t.Fatal("source6b99 rescued actor writer", n)
		}
		if n.Queue(21, actor, 0) {
			t.Fatal("repeat pickup duplicated history")
		}
		n.Advance(true, true)
		if got := n.Advance(false, true); !reflect.DeepEqual(got, []uint16{0x14}) {
			t.Fatal("common pre23 sourcecue ownership", got)
		}
	}
	legacy := NotificationState{LegacyO: true}
	if legacy.Queue(21, 1, 0) || !legacy.Queue(22, 1, 0) {
		t.Fatal("strict historical O queue domain")
	}
	for _, bad := range []int16{-1, 0, 23, 255} {
		if (&NotificationState{}).Queue(bad, 1, 0) {
			t.Fatal("unknown source pointer admitted", bad)
		}
	}
}

func TestHistoricalNotificationPolicyIsNotCheckpointState(t *testing.T) {
	a := NotificationState{Rows: [4][4]byte{{16, 0, 24, 0}}}
	b := a
	b.LegacyO = true
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	if !reflect.DeepEqual(x, y) {
		t.Fatal("profilepolicy leaked into old checkpoint JSON", string(x), string(y))
	}
}
