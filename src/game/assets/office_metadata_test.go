package assets

import (
	"encoding/binary"
	"encoding/json"
	"testing"
)

func syntheticOfficeFORM(tag string, bodies ...struct {
	tag  string
	body []byte
}) []byte {
	raw := make([]byte, 12)
	copy(raw, "FORM")
	copy(raw[8:], tag)
	for _, c := range bodies {
		head := make([]byte, 8)
		copy(head, c.tag)
		binary.BigEndian.PutUint32(head[4:], uint32(len(c.body)))
		raw = append(raw, head...)
		raw = append(raw, c.body...)
	}
	binary.BigEndian.PutUint32(raw[4:], uint32(len(raw)-8))
	return raw
}

func TestOfficeChunksOddLengthAndResourceBoundary(t *testing.T) {
	b := syntheticOfficeFORM("SCRN", struct {
		tag  string
		body []byte
	}{"TEST", []byte{1}}, struct {
		tag  string
		body []byte
	}{"NEXT", []byte{2, 3}})
	c, err := officeChunks(b, 2042)
	if err != nil {
		t.Fatal(err)
	}
	if len(c) != 2 || c[1].HeaderRelative != 21 || c[1].PayloadAbsolute != 2071 {
		t.Fatalf("padding or source mapping invented: %+v", c)
	}
	binary.BigEndian.PutUint32(b[25:29], 3)
	if _, err = officeChunks(b, 0); err == nil {
		t.Fatal("chunk crossing resource boundary accepted")
	}
	b = b[:len(b)-2]
	binary.BigEndian.PutUint32(b[4:8], uint32(len(b)-8))
	if _, err = officeChunks(b, 0); err == nil {
		t.Fatal("truncated chunk accepted")
	}
}

func TestOfficeComponentGroupAndShortAREA(t *testing.T) {
	component := make([]byte, 14)
	component[0] = 3
	component[13] = 1
	for i, v := range []uint16{1, 2, 4, 5, 6, 7} {
		binary.LittleEndian.PutUint16(component[1+2*i:], v)
	}
	continuation := append([]byte(nil), component...)
	continuation[0] = 255
	area := []byte{6, 0, 136, 0, 85, 0, 15, 0, 9}
	b := syntheticOfficeFORM("SCRN", struct {
		tag  string
		body []byte
	}{"CMPX", component}, struct {
		tag  string
		body []byte
	}{"CMPX", continuation}, struct {
		tag  string
		body []byte
	}{"AREA", area})
	m, err := parseOfficeScreen(&Resource{Payload: b})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Groups) != 1 || len(m.Groups[0].Records) != 2 || !m.Components[1].Continuation || m.Components[1].GroupKey != 3 || m.Components[0].SourceAsset != "BASE_AN1.PIC" {
		t.Fatalf("wrong component model: %+v", m)
	}
	if m.Areas[0].VisualFlag != 0 || m.Areas[0].Rectangle != [4]int{136, 85, 15, 9} || m.Areas[0].InclusiveEnd != [2]int{150, 93} {
		t.Fatal("short AREA padding/height semantics incorrect")
	}
	b = syntheticOfficeFORM("SCRN", struct {
		tag  string
		body []byte
	}{"CMPX", continuation})
	if _, err = parseOfficeScreen(&Resource{Payload: b}); err == nil {
		t.Fatal("orphan continuation accepted")
	}
}

func TestOfficeAnimationVerticalGrid(t *testing.T) {
	b := make([]byte, 38)
	b[0] = 2
	b[15] = 21
	b[16] = 4
	for i, v := range []uint16{30, 0, 15, 9, 20, 136, 85} {
		binary.LittleEndian.PutUint16(b[1+2*i:], v)
	}
	a, err := parseOfficeAnimation(OfficeChunk{Payload: b})
	if err != nil {
		t.Fatal(err)
	}
	if a.Frames[19].Source != [4]int{30, 171, 15, 9} || a.Frames[20].Source != [4]int{45, 0, 15, 9} {
		t.Fatal("vertical quotient/remainder frame grid incorrect")
	}
	binary.LittleEndian.PutUint16(b[9:], 0)
	if _, err = parseOfficeAnimation(OfficeChunk{Payload: b}); err == nil {
		t.Fatal("zero frame divisor accepted")
	}
}

func TestOfficeLetteringUsesSourceLORI(t *testing.T) {
	letters, err := officeLettering([]officeLori{{Key: 2, Base: [2]int{0, 190}, Cell: [2]int{4, 5}, Divisor: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if len(letters) != 96 || letters[33].Byte != 65 || letters[33].Selector != 66 || letters[33].Source != [4]int{132, 190, 4, 5} || letters[48].Source != [4]int{0, 195, 4, 5} {
		t.Fatal("atlas character permutation incorrect")
	}
	if _, err = officeLettering(nil); err == nil {
		t.Fatal("absent source layout accepted")
	}
}

func TestOfficeCallbackUsesSeparateOldCounterParity(t *testing.T) {
	b := make([]byte, 38)
	b[0], b[15], b[16] = 4, 2, 8
	for i, v := range []uint16{0, 0, 15, 9, 0, 136, 85} {
		binary.LittleEndian.PutUint16(b[1+2*i:], v)
	}
	a, err := parseOfficeAnimation(OfficeChunk{Payload: b})
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Frames) != 0 || a.CallbackSources["even"] != [4]int{0, 191, 15, 9} || a.CallbackSources["odd"] != [4]int{0, 182, 15, 9} {
		t.Fatal("callback parity confused with direct ANIM frame order")
	}
}

func TestOfficeRulesAndNoTargetSkip(t *testing.T) {
	if !json.Valid([]byte(officeInstructionRules)) || !json.Valid([]byte(officeInitializationRules)) {
		t.Fatal("instruction-derived rules are not valid JSON")
	}
	m := &Manifest{Resources: []*Resource{{Source: SourceLocation{PayloadSHA256: "unrelated"}}}}
	if err := exportOfficeMetadata(nil, nil, m); err != nil || m.OfficeMetadata != "" {
		t.Fatal("unrelated installation caused office metadata writes")
	}
	if _, err := officeSourceString([]byte{0}, 0x1e67); err == nil {
		t.Fatal("truncated original table accepted")
	}
}
