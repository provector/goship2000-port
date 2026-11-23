package state

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func legalWeaponRows(t *testing.T) ([][36]byte, []int, string) {
	t.Helper()
	raw, err := os.ReadFile("../../../test-data/weapon-table/rows.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Rows           []string
		ActualRawOrder []int  `json:"actual_raw_order"`
		Hellfire       string `json:"hellfire_compact16"`
	}
	if err = json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	rows := make([][36]byte, len(f.Rows))
	for i, row := range f.Rows {
		b, err := hex.DecodeString(row)
		if err != nil || len(b) != 36 {
			t.Fatal("invalid public legal row", i, err)
		}
		copy(rows[i][:], b)
	}
	return rows, f.ActualRawOrder, f.Hellfire
}

func TestCompleteWeaponTableLegal17AndHistoricalLimit(t *testing.T) {
	a := initializationAssets(t)
	rows, order, hellfire := legalWeaponRows(t)
	m := &MissionData{}
	if _, err := BuildRuntimeStores(a.Missions, m, rows); err == nil || !strings.Contains(err.Error(), "unsupported16..19") {
		t.Fatal("historical limit changed", err)
	}
	stores, err := BuildRuntimeStores(a.Missions, m, rows, true)
	if err != nil {
		t.Fatal(err)
	}
	if stores.WeaponTable == nil || len(stores.Definitions) != 15 {
		t.Fatal("actual count plus two sentinels", stores.WeaponTable, len(stores.Definitions))
	}
	for i, raw := range order {
		w, err := stores.LookupDefinition(byte(i + 2))
		if err != nil || w.RawWSHDID != raw {
			t.Fatal("first-use source identity", i, w, err)
		}
	}
	w, err := stores.LookupDefinition(16)
	if err != nil || hex.EncodeToString(w.Raw[:]) != hellfire {
		t.Fatal("independent source Hellfire16", w, err)
	}
	if st := stores.Ships[4].Stations[2]; st.CompactID != 16 || st.ShortName != "HELLFIRE" || st.Ammunition != 2 {
		t.Fatal(st)
	}
	for _, id := range []byte{0, 1, 17, 63, 64, 255} {
		if _, err = stores.LookupDefinition(id); err == nil {
			t.Fatal("padding/sentinel/nonexistent admitted", id)
		}
	}
	for i := range stores.Ships {
		skill := byte(127)
		stores.Ships[i].Proficiency = &skill
	}
	shipRows, err := stores.SourceShipRows()
	if err != nil || shipRows[4][3] != 16 {
		t.Fatal("serialized source station identity", shipRows[4], err)
	}
	before := *w
	stores.Ships[4].Stations[2].Ammunition = 0
	if err = stores.Refill(4, &stores.Damage[4]); err != nil || stores.Ships[4].Stations[2].Ammunition != 2 || *w != before {
		t.Fatal("refill changed frozen definition or lost upper station", err)
	}
	raw, err := json.Marshal(stores)
	if err != nil {
		t.Fatal(err)
	}
	var restored RuntimeStores
	if err = json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if err = restored.ValidateWeaponTable(); err != nil || !reflect.DeepEqual(restored.Definitions, stores.Definitions) || !reflect.DeepEqual(restored.WeaponTable, stores.WeaponTable) {
		t.Fatal("checkpoint table", err)
	}
	// Source result/status/credit are distinct owners; changing their bytes
	// cannot substitute for compact16's retained source definition.
	var context SortieResultContext
	context.SetupFlags = 0xff
	for i := range context.DynamicKinds {
		context.DynamicKinds[i] = byte(i)
	}
	if got, err := restored.LookupDefinition(16); err != nil || *got != before {
		t.Fatal("unrelated status changed definition", err)
	}
	// Ordinary lower-domain records are unchanged under the new policy.
	legacy, err := BuildRuntimeStores(a.Missions, m, rows[:1])
	if err != nil {
		t.Fatal(err)
	}
	modern, err := BuildRuntimeStores(a.Missions, m, rows[:1], true)
	if err != nil || !reflect.DeepEqual(legacy.Ships, modern.Ships) || !reflect.DeepEqual(legacy.Definitions, modern.Definitions) {
		t.Fatal("lower-domain behavior changed", err)
	}
}

func TestWeaponTableValidationAndSixBitBoundary(t *testing.T) {
	source := map[int][]byte{}
	stores := &RuntimeStores{}
	for i := 0; i < 62; i++ {
		source[i] = make([]byte, 214)
		stores.Definitions = append(stores.Definitions, RuntimeWeapon{RawWSHDID: i, CompactID: byte(i + 2)})
	}
	if err := stores.freezeWeaponTable(source); err != nil {
		t.Fatal(err)
	}
	w, err := stores.LookupDefinition(63)
	if err != nil || w.RawWSHDID != 61 || byte(w.CompactID<<2|3)>>2 != 63 {
		t.Fatal("six-bit endpoint", w, err)
	}
	for _, change := range []string{"duplicate-raw", "duplicate-compact", "noncontiguous", "too-many", "sentinel", "changed-bytes", "source-identity"} {
		t.Run(change, func(t *testing.T) {
			copy := *stores
			meta := *stores.WeaponTable
			copy.WeaponTable = &meta
			copy.Definitions = append([]RuntimeWeapon(nil), stores.Definitions...)
			switch change {
			case "duplicate-raw":
				copy.Definitions[1].RawWSHDID = copy.Definitions[0].RawWSHDID
			case "duplicate-compact":
				copy.Definitions[1].CompactID = 2
			case "noncontiguous":
				copy.Definitions[1].CompactID = 63
			case "too-many":
				copy.Definitions = append(copy.Definitions, RuntimeWeapon{RawWSHDID: 62, CompactID: 64})
			case "sentinel":
				copy.WeaponTable.Sentinels = [2]byte{1, 0}
			case "changed-bytes":
				copy.Definitions[0].Raw[4]++
			case "source-identity":
				copy.WeaponTable.SourceSHA256 = ""
			}
			if err := copy.ValidateWeaponTable(); err == nil {
				t.Fatal("invalid frozen table admitted")
			}
			if _, err := copy.LookupDefinition(2); err == nil {
				t.Fatal("lookup bypassed table validation")
			}
		})
	}
}

func TestWeaponSupportPrefixAndNPCBucketProjection(t *testing.T) {
	a := initializationAssets(t)
	rows, _, _ := legalWeaponRows(t)
	m := &MissionData{Support: &SupportConfiguration{Profile: SupportInitializationProfile, PrefixRaw: [2]byte{4, 16}}}
	s, err := BuildRuntimeStores(a.Missions, m, rows, true)
	if err != nil {
		t.Fatal(err)
	}
	for id, raw := range map[byte]int{2: 4, 3: 16} {
		w, err := s.LookupDefinition(id)
		if err != nil || w.RawWSHDID != raw {
			t.Fatal("Support prefix order", id, w, err)
		}
	}
	for i := range s.Ships {
		skill := byte(101 + i)
		s.Ships[i].Proficiency = &skill
	}
	stat := make([]byte, 80)
	stat[0x1d], stat[0x3b] = 33, 44
	if err = s.EnableNPCDamage(stat); err != nil {
		t.Fatal(err)
	}
	w, _ := s.LookupDefinition(2)
	if s.NPCDamage.BucketBytes[9] != w.Raw[21] || s.NPCDamage.BucketBytes[5] != 33 || s.NPCDamage.BucketBytes[6] != 44 || s.NPCDamage.BucketBytes[8] != 'E' {
		t.Fatal("NPC operand projection changed", s.NPCDamage)
	}
}
