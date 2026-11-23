package setupui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"strings"
)

// Flex gives the table the full body rectangle. Measure again at draw time so
// a live terminal resize changes both its parent and actual column widths.
// tview has maximum/expansion widths, not minimum widths; the always-evaluated
// first row supplies each column's minimum without adding a dummy column.
type widthTable struct{ *tview.Table }

func (t *widthTable) Draw(s tcell.Screen) {
	_, _, width, _ := t.GetInnerRect()
	n := t.GetColumnCount()
	if n > 0 && n <= 3 && width >= n*2 {
		available := width - (n - 1) // tview's one-cell inter-column gutters; no scrollbar.
		sizes := []int{available}
		if n == 2 {
			sizes = []int{(available + 1) / 2, available / 2}
		}
		if n == 3 {
			sizes = []int{(available + 1) / 2, available / 4, available - (available+1)/2 - available/4}
		}
		for row := 0; row < t.GetRowCount(); row++ {
			for col, w := range sizes {
				c := t.GetCell(row, col)
				if c == nil {
					continue
				}
				c.SetMaxWidth(w).SetExpansion(0)
				if row == 0 {
					raw := strings.TrimRight(c.Text, " ")
					c.SetText(raw + strings.Repeat(" ", max(0, w-tview.TaggedStringWidth(raw))))
				}
			}
		}
	}
	t.Table.Draw(s)
	x, y, w, h := t.GetInnerRect()
	if w <= 0 || h <= 0 {
		return
	}
	if selected, _ := t.GetSelection(); selected >= 2 {
		offset, _ := t.GetOffset()
		line := y + selected - offset
		if cell := t.GetCell(selected, 0); cell != nil && !cell.NotSelectable && line >= y+2 && line < y+h {
			for col := 0; col < w; col++ {
				ch, comb, style, _ := s.GetContent(x+col, line)
				s.SetContent(x+col, line, ch, comb, style.Foreground(yellow).Background(blue).Bold(true))
			}
		}
	}
	if c := t.GetCell(0, 0); c != nil && c.Reference == namedHeader && h > 1 {
		style := tcell.StyleDefault.Foreground(yellow).Background(red)
		for i := 0; i < w; i++ {
			s.SetContent(x+i, y+1, '─', nil, style)
		}
		if x > 0 {
			s.SetContent(x-1, y+1, '╟', nil, style)
		}
		s.SetContent(x+w, y+1, '╢', nil, style)
		offset, _ := t.GetOffset()
		for row := 2 + offset; row < t.GetRowCount(); row++ {
			line := y + 2 + row - 2 - offset
			if line >= y+h {
				break
			}
			c := t.GetCell(row, 0)
			label, ok := c.Reference.(categoryHeader)
			if !ok {
				continue
			}
			for i := 0; i < w; i++ {
				s.SetContent(x+i, line, '-', nil, style.Foreground(white).Bold(true))
			}
			tview.Print(s, tview.Escape(string(label)), x, line, w, tview.AlignLeft, white)
		}
	}
}
func fullWidthTable(t *tview.Table) *widthTable { t.SetEvaluateAllRows(true); return &widthTable{t} }

const namedHeader = "setup:named-columns"

type categoryHeader string

func namedColumns(t *tview.Table, labels []string) {
	for col, label := range labels {
		t.SetCell(0, col, cc(label, false).SetReference(namedHeader))
		t.SetCell(1, col, cc("", false))
	}
	t.SetFixed(2, 0)
}
