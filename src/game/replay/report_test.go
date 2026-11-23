package replay

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiagnosticRecordingContiguousPrefixAndComplete(t *testing.T) {
	s := Store{t.TempDir()}
	id := strings.Repeat("a", 32)
	f := captureBlocks(t, s, id, 140)
	test := func(name, want string) {
		t.Helper()
		var buf bytes.Buffer
		z := zip.NewWriter(&buf)
		status, n, e := CopyReportFilm(z, s.Root, id)
		if e != nil || n == 0 || !strings.Contains(status, want) {
			t.Fatal(status, n, e)
		}
		if e = z.Close(); e != nil {
			t.Fatal(e)
		}
		a, e := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		if e != nil || len(a.File) != 1 || a.File[0].Name != name {
			t.Fatal(e, a)
		}
		r, _ := a.File[0].Open()
		out, e := io.ReadAll(r)
		r.Close()
		if e != nil {
			t.Fatal(e)
		}
		returnPath := filepath.Join(t.TempDir(), "copy.g2r")
		os.WriteFile(returnPath, out, 0600)
		if name == "mission-prefix.g2r" {
			k, ops, l, _ := InspectPrefix(returnPath)
			if k < 2 || ops != n || l != int64(len(out)) {
				t.Fatal("invalid recovered prefix")
			}
			if _, e := OpenFile(returnPath); e == nil {
				t.Fatal("partial published")
			}
		}
	}
	test("mission.g2r", "private sealed")
	if e := s.PublishLast(f); e != nil {
		t.Fatal(e)
	}
	test("mission.g2r", "published")
	path := filepath.Join(s.Root, "Last Mission.g2r")
	d, e := os.OpenFile(path, os.O_RDWR, 0600)
	if e != nil {
		t.Fatal(e)
	}
	desc, _ := readDescriptor(d, 12)
	pos := int64(12+descriptorSize) + int64(desc.Stored)
	first, _ := readDescriptor(d, pos)
	d.Close()
	if e = os.Truncate(path, pos+descriptorSize+int64(first.Stored)+5); e != nil {
		t.Fatal(e)
	}
	test("mission-prefix.g2r", "contiguous")
}
