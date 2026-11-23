package logview

import "strings"

type Line struct {
	Text, Level  string
	Gap          bool
	Continuation bool
}

var SeverityRGB = map[string][3]uint8{"DEBUG": {135, 210, 255}, "INFO": {240, 240, 245}, "WARN": {255, 230, 80}, "ERROR": {255, 135, 145}}

func severity(record string) string {
	for _, field := range strings.Fields(record) {
		if strings.HasPrefix(field, "msg=") {
			break
		}
		if strings.HasPrefix(field, "level=") {
			if _, ok := SeverityRGB[field[6:]]; ok {
				return field[6:]
			}
			break
		}
	}
	return "INFO"
}
func Layout(records []string, columns int) []Line {
	columns = max(4, columns)
	var lines []Line
	for _, record := range records {
		level := severity(record)
		r := []rune(record)
		continuation := false
		for len(r) > 0 {
			indent := ""
			width := columns
			if continuation {
				indent = "  "
				width -= 2
			}
			n := min(len(r), width)
			if n < len(r) {
				for k := n - 1; k > width/2; k-- {
					if r[k] == ' ' {
						n = k + 1
						break
					}
				}
			}
			lines = append(lines, Line{Text: indent + string(r[:n]), Level: level, Continuation: continuation})
			r = r[n:]
			continuation = true
		}
		lines = append(lines, Line{Level: level, Gap: true})
	}
	return lines
}
