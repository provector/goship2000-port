package state

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

// Expected mapping and constructor stopping domain are transcribed from
// GS 191CE/191D7 and A1A90C/AA86/ABE6, not from MissionTables or the old model.
func TestObjectiveSourceEpochAndSpans(t *testing.T) {
	raw, err := os.ReadFile(testfixture.Path("installation/GS.GS2"))
	if os.IsNotExist(err) {
		t.Skip("original operand fixture required by release gate")
	}
	if err != nil {
		t.Fatal(err)
	}
	mapped := []int{0, 3, 3, 3, 2, 1, 2, 4}
	for k, v := range mapped {
		if int(binary.LittleEndian.Uint16(raw[0x3ee84+2*k:])) != v {
			t.Fatal("source mapped table", k)
		}
	}
	for _, off := range []int{0x1a90c - 0xe000, 0x1aa86 - 0xe000, 0x1abe6 - 0xe000} {
		if raw[off] != 0x83 || raw[off+4] != 2 {
			t.Fatal("source mapped2 CMP", off)
		}
	}
	for role := 2; role <= 4; role++ {
		for k := 1; k <= 7; k++ {
			for _, reused := range []bool{false, true} {
				for _, n := range []int{0, 1, 2, 3, 9} {
					rows := make([]MissionObject, n+2)
					for i := range rows {
						rows[i].Raw[0] = 3
						rows[i].Raw[15] = byte(i)
					}
					mark := byte(0x10 << uint(role-2))
					markMissionObjectiveSpan(rows, 1, n+1, mark, !reused && mapped[k] == 2)
					for i, row := range rows {
						want := byte(3)
						if i >= 1 && i < n+1 && (reused || mapped[k] != 2 || i == 1) {
							want |= mark
						}
						if row.Raw[0] != want || row.Raw[15] != byte(i) {
							t.Fatalf("role%d/raw%d/reused%v/span%d row%d", role, k, reused, n, i)
						}
					}
				}
			}
		}
	}
	// Existing retained bytes are value-owned; helper does not modify a source copy.
	a := []MissionObject{{}}
	b := append([]MissionObject(nil), a...)
	markMissionObjectiveSpan(a, 0, 1, 0x10, false)
	if !bytes.Equal(b[0].Raw[:], make([]byte, 32)) {
		t.Fatal("source alias")
	}
}

// Step23 fixtures remain immutable. Their retained append/target event ledger
// supplies spans independently of production. Supersede ONLY the proved role-bit
// epoch error; all other original fixture bytes and RNG remain a strict oracle.
func correctedReferenceHash(t *testing.T, raw []byte) string {
	t.Helper()
	var ref struct {
		Population struct {
			Hash    string `json:"objects_sha256"`
			Objects []struct {
				Raw string `json:"raw_hex"`
			}
			Decisions []struct {
				Operation          string
				First              int `json:"first_object"`
				Count, Reused, Row int
				Kind               int `json:"raw_kind"`
			}
		}
	}
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatal(err)
	}
	all := []byte{}
	for _, o := range ref.Population.Objects {
		b, e := hex.DecodeString(o.Raw)
		if e != nil || len(b) != 32 {
			t.Fatal("historical row extent")
		}
		all = append(all, b...)
	}
	old := sha256.Sum256(all)
	if hex.EncodeToString(old[:]) != ref.Population.Hash {
		t.Fatal("historical oracle bytes/hash differ")
	}
	first, count, reused := -1, 0, -1
	mapping := [8]int{0, 3, 3, 3, 2, 1, 2, 4}
	for _, d := range ref.Population.Decisions {
		if d.Operation == "outer_append_result" {
			first, count, reused = d.First, d.Count, d.Reused
		}
		if d.Operation != "target_record" || d.Row < 2 || d.Row > 4 {
			continue
		}
		if first < 0 || first+count > len(all)/32 || d.Kind < 0 || d.Kind > 7 {
			t.Fatal("unproven target span")
		}
		bit := byte(0x10 << uint(d.Row-2))
		for i := 0; i < count; i++ {
			at := (first + i) * 32
			all[at] &^= bit
			if reused >= 0 || mapping[d.Kind] != 2 || i == 0 {
				all[at] |= bit
			}
		}
	}
	sum := sha256.Sum256(all)
	return hex.EncodeToString(sum[:])
}
