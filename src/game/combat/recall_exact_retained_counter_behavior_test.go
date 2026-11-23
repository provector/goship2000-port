package combat

import (
	"reflect"
	"testing"
)

func TestRecallExactRetainedCounterBehavior(t *testing.T) {
	n := NotificationState{Rows: [4][4]byte{{20, 0, 12, 0}, {2, 3, 6, 17}, {4, 1, 0, 9}, {0, 2, 0, 8}}}
	n.Recall()
	want := [4][4]byte{{20, 0, 0, 0}, {2, 3, 0, 17}, {4, 1, 23, 9}, {0, 2, 0, 8}}
	if n.Rows != want || n.HistoryMode != 0 {
		t.Fatal(n.Rows)
	}
	if got := n.Advance(true, true); !reflect.DeepEqual(got, []uint16{0x14}) || n.Display[2] != want[2] || n.Rows[2][2] != 22 {
		t.Fatal("restored row's pre23 consumer", got, n)
	}
	// An active row cancels, while another expired retained row can redisplay.
	n.Recall()
	if n.Rows[1] != [4]byte{2, 3, 23, 17} || n.Rows[2] != [4]byte{4, 1, 0, 9} {
		t.Fatal(n.Rows)
	}
}

func TestRecallKeepsSourceQueueOrderAndNewArrival(t *testing.T) {
	var n NotificationState
	for i := int16(1); i <= 4; i++ {
		n.Queue(i, i, i+10)
	}
	before := n.Rows
	n.Recall() // All active rows cancel, not a re-enqueue.
	for i := 1; i <= 3; i++ {
		if n.Rows[i] != [4]byte{before[i][0], before[i][1], 0, before[i][3]} {
			t.Fatal(n.Rows)
		}
	}
	n.Recall()
	if n.Rows[1][2] != 23 || n.Rows[3][0] != 2 {
		t.Fatal(n.Rows)
	}
	n.Queue(5, 2, 19)
	if n.Rows[1] != [4]byte{5, 2, 24, 19} || n.Rows[2][0] != 4 || n.Rows[3][0] != 3 {
		t.Fatal("new arrival must use ordinary source queue", n.Rows)
	}
}
