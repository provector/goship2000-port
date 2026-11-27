// Package legacyreplay imports released JSON/zstd recordings on explicit request.
package legacyreplay

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"gunship2000-assets/game/replay"
	"io"
	"os"
)

var invalidBlock = errors.New("invalid legacy operation block")

func strictJSON(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return invalidBlock
	}
	return nil
}

const oldCore = "GS2FILM\x00\x02\x00\x00\x00"
const oldPublished = "GS2FILM\x00\x03\x00\x00\x00"
const current = "GS2FILM\x00\x04\x00\x04\x00"

func prefix(path string) (string, error) {
	st, e := os.Lstat(path)
	if e != nil {
		return "", e
	}
	if !st.Mode().IsRegular() {
		return "", fmt.Errorf("expected a regular recording")
	}
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	var b [12]byte
	_, e = io.ReadFull(f, b[:])
	return string(b[:]), e
}
func Convert(input, output string) error {
	p, e := prefix(input)
	if e != nil {
		return e
	}
	if p != oldCore && p != oldPublished {
		return fmt.Errorf("unsupported legacy recording header %x", p)
	}
	return replay.ConvertContainer(input, output, format())
}
func decodeRows(raw []byte, count uint32) ([]replay.Operation, error) {
	rows := make([]replay.Operation, 0, min(count, 1024))
	for len(raw) > 0 {
		line, rest, ok := bytes.Cut(raw, []byte{'\n'})
		if !ok || len(line) > (2<<20)+(64<<10) || len(rows) >= int(count) {
			return nil, invalidBlock
		}
		var op replay.Operation
		if e := strictJSON(line, &op); e != nil {
			return nil, e
		}
		// The current encoder validates every historical operation before publishing it.
		rows = append(rows, op)
		raw = rest
	}
	if len(rows) != int(count) {
		return nil, invalidBlock
	}
	return rows, nil
}

func format() replay.ImportFormat {
	return replay.ImportFormat{Core: oldCore, Published: oldPublished, Rows: decodeRows}
}
