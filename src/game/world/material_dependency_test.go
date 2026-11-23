package world

import (
	"strings"
	"testing"
)

func hasMemoryStatus(g *Geometry) bool {
	for _, text := range g.Status {
		if strings.Contains(text, "live CS/DS carrier") {
			return true
		}
	}
	return false
}

func TestMaterialDependencyIncludesDirectNestedAndPointGroups(t *testing.T) {
	// Source VGA03f8 tests material bytes, not resource/model IDs. Point01 also
	// reaches1960 after its2275 span generator; outline commands consume DL only.
	for _, c := range []struct {
		op   byte
		word uint16
		want bool
	}{{1, 0x20, true}, {3, 0x24, true}, {4, 0x98, true}, {6, 0x77, true}, {0x82, 0x24, false}, {0x83, 0x24, false}, {3, 0, false}, {3, 0x2424, false}, {3, 0x2400, false}, {3, 0xffff, false}} {
		g := &Geometry{DirectGroups: []Group{{Children: []Group{{Primitive: &Primitive{Opcode: c.op, Colour: c.word}}}}}}
		markMaterialDependency(g)
		if hasMemoryStatus(g) != c.want {
			t.Fatalf("op%02x material%04x status%v", c.op, c.word, g.Status)
		}
	}
}

func TestOriginalWorldMaterialDependencyCoverage(t *testing.T) {
	worlds := []string{"ANTC1", "ANTC2", "ANTC3", "EURO6", "EURO7", "EURO8", "EURO9", "GULF2", "GULF3", "GULF4", "GULF5", "PHIL1", "PHIL2", "PHIL3"}
	cells, compacts := 0, 0
	for _, name := range worlds {
		_, gs, err := ParseWorld(name, source(t, name), source(t, name+"BMP.GS2"))
		if err != nil {
			t.Fatal(err)
		}
		for _, g := range gs {
			if hasMemoryStatus(g) {
				if g.CompactCallback == 0 {
					cells++
				} else {
					compacts++
				}
			}
			if g.ID == "world/EURO7/compact/49d2" && !hasMemoryStatus(g) {
				t.Fatal("raw source0024 triangles were omitted from classification")
			}
		}
	}
	// Independently scanned retained Step29 source exports, corroborated against
	// actual raw near-pool words, give90 cell and89 compact dependency records.
	// The CSV/check report records every source ID; this is not a visible count.
	if cells != 90 || compacts != 89 {
		t.Fatalf("material dependency coverage cells%d compacts%d", cells, compacts)
	}
}
