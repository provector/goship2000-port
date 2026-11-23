package assets

import (
	"fmt"
	"strings"
)

func OfficeBlit(source, destination []byte, r [4]int, dx, dy int, transparent bool) error {
	if len(source) != 64000 || len(destination) != 64000 || !OfficeRectFits(r) || !OfficeRectFits([4]int{dx, dy, r[2], r[3]}) {
		return fmt.Errorf("unsupported office blit source %v destination (%d,%d)", r, dx, dy)
	}
	// Extract first: source and destination can alias or overlap.
	tmp := make([]byte, r[2]*r[3])
	for y := 0; y < r[3]; y++ {
		copy(tmp[y*r[2]:(y+1)*r[2]], source[(r[1]+y)*320+r[0]:(r[1]+y)*320+r[0]+r[2]])
	}
	for y := 0; y < r[3]; y++ {
		for x := 0; x < r[2]; x++ {
			v := tmp[y*r[2]+x]
			if v != 0 || !transparent {
				destination[(dy+y)*320+dx+x] = v
			}
		}
	}
	return nil
}

func OfficeFill(dst []byte, x, y, w, h int, v byte) {
	for row := y; row < y+h; row++ {
		for col := x; col < x+w; col++ {
			dst[row*320+col] = v
		}
	}
}

// Original inclusive rasterization; tie-breaking differs from some generic
// Bresenham implementations and matters for the theatre polygon's shallow edge.
func OfficeLine(dst []byte, x0, y0, x1, y1 int, v byte) error {
	if len(dst) != 64000 || x0 < 0 || x0 >= 320 || x1 < 0 || x1 >= 320 || y0 < 0 || y0 >= 200 || y1 < 0 || y1 >= 200 {
		return fmt.Errorf("office line outside bank")
	}
	if x0 > x1 {
		x0, x1 = x1, x0
		y0, y1 = y1, y0
	}
	dx, dy := x1-x0, y1-y0
	vertical := 1
	if dy < 0 {
		vertical = -1
		dy = -dy
	}
	major, minor, ax, ay, bx, by := dx, dy, 1, 0, 0, vertical
	if dx < dy {
		major, minor, ax, ay, bx, by = dy, dx, bx, by, ax, ay
	}
	errorTerm := -((major + 1) / 2)
	x, y := x0, y0
	for i := 0; i <= major; i++ {
		dst[y*320+x] = v
		if i == major {
			break
		}
		x += ax
		y += ay
		errorTerm += minor
		if errorTerm >= 0 {
			errorTerm -= major
			x += bx
			y += by
		}
	}
	return nil
}

// The editor's 35-byte name input and the office's 30-byte final-token copy are
// different limits; neither is a declaration of the serialized record layout.
func OfficePilotLabel(pilotName, rank string) string {
	name := strings.TrimSpace(pilotName)
	if i := strings.LastIndexByte(name, ' '); i >= 0 {
		name = name[i+1:]
	}
	if len(name) > 30 {
		name = name[:30]
	}
	return strings.TrimSpace(rank) + " " + name
}
