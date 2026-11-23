package menus

import "testing"

func TestEditorCaptureReachesBottomWithoutChangingOffice(t *testing.T) {
	for _, height := range []int{0, 200} {
		p := mouseCapture{}
		s := mouseCaptureSample{Enabled: true, Focused: true, Client: true, AbsoluteInside: true, AbsoluteX: 160, AbsoluteY: 180, LogicalHeight: height, Viewport: Viewport{Width: 320, Height: 200}, Epoch: "x"}
		p.Read(s)
		s.Left = true
		p.Read(s)
		s.Left = false
		p.Read(s)
		p.Read(s)
		s.DeltaY = 30
		r := p.Read(s)
		want := 190
		if height == 200 {
			want = 199
		}
		if r.Y != want {
			t.Fatal(height, r.Y, want)
		}
	}
}
