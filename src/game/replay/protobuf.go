package replay

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/klauspost/compress/s2"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"gunship2000-assets/internal/replaypb"
	"math/bits"
	"slices"
)

const protoS2 = "exp1-fullhash-s2"

func decodeS2(stored []byte, want int) ([]byte, error) {
	n, e := s2.DecodedLen(stored)
	if e != nil || n != want {
		return nil, invalidBlock
	}
	return s2.Decode(make([]byte, 0, n), stored)
}

// Fixed dictionary IDs from the prototype, independent of Go map order.
var expKinds = [...]string{"", "step", "boundary", "end", "terminal"}
var expDomains = [...]string{"appearance", "combat", "collective-owner", "controls-q3", "environment", "flight", "mission", "modal", "navigation", "orders-r1", "palette", "pending-hit", "reference", "visual"}

func protoRow(o Operation) ([]byte, error) {
	kind := slices.Index(expKinds[:], o.Kind)
	if kind <= 0 {
		return nil, invalidBlock
	}
	v := &replaypb.Operation{Kind: uint32(kind), PayloadJson: o.Payload, EventsPresent: o.Events != nil}
	for i, name := range expDomains {
		digest, ok := o.Checkpoint[name]
		if !ok {
			continue
		}
		raw, e := hex.DecodeString(digest)
		if e != nil || len(raw) != 32 || hex.EncodeToString(raw) != digest {
			return nil, fmt.Errorf("unsupported digest in %s", name)
		}
		v.Present |= 1 << i
		v.Hashes = append(v.Hashes, raw...)
	}
	if len(o.Checkpoint) == 0 || bits.OnesCount64(v.Present) != len(o.Checkpoint) {
		return nil, fmt.Errorf("unsupported checkpoint domain")
	}
	for _, e := range o.Events {
		v.Events = append(v.Events, &replaypb.Event{Index: e.Index, Kind: uint32(e.Kind), Actor: uint32(e.Actor), Target: uint32(e.Target), Sequence: uint32(e.Sequence), Timestamp: uint32(e.Timestamp)})
	}
	data, e := (proto.MarshalOptions{Deterministic: true}).Marshal(v)
	if e != nil {
		return nil, e
	}
	return protowire.AppendBytes(protowire.AppendTag(nil, 1, protowire.BytesType), data), nil
}

// Reject extreme nested element counts before the generated decoder allocates
// event objects. Even the largest supported JSON row fits below this event cap.
func boundedProtoOperation(data []byte) bool {
	var events int
	for len(data) > 0 {
		field, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return false
		}
		data = data[n:]
		switch field {
		case 1, 3, 7:
			if typ != protowire.VarintType {
				return false
			}
		case 2, 5:
			if typ != protowire.BytesType {
				return false
			}
		case 6:
			events++
			if typ != protowire.BytesType || events > 65536 {
				return false
			}
		default:
			return false
		}
		n = protowire.ConsumeFieldValue(field, typ, data)
		if n < 0 {
			return false
		}
		data = data[n:]
	}
	return true
}

// Read the repeated field one row at a time, enforcing the authenticated row
// count and decoded byte budget before allocating the next message.
func decodeProtoRows(raw []byte, count uint32) ([]Operation, error) {
	rows := make([]Operation, 0, min(count, 1024))
	for len(raw) > 0 {
		if raw[0] != 0x0a || uint32(len(rows)) >= count {
			return nil, invalidBlock
		}
		data, n := protowire.ConsumeBytes(raw[1:])
		if n < 0 || len(data) > maxRow {
			return nil, invalidBlock
		}
		raw = raw[1+n:]
		if !boundedProtoOperation(data) {
			return nil, invalidBlock
		}
		var v replaypb.Operation
		if e := (proto.UnmarshalOptions{RecursionLimit: 8}).Unmarshal(data, &v); e != nil {
			return nil, e
		}
		if len(v.ProtoReflect().GetUnknown()) > 0 || !v.EventsPresent && len(v.Events) != 0 || !json.Valid(v.PayloadJson) {
			return nil, invalidBlock
		}
		if v.Kind == 0 || uint64(v.Kind) >= uint64(len(expKinds)) || v.Present == 0 || v.Present>>len(expDomains) != 0 || len(v.Hashes) != 32*bits.OnesCount64(v.Present) {
			return nil, invalidBlock
		}
		o := Operation{Kind: expKinds[v.Kind], Payload: v.PayloadJson, Checkpoint: make(map[string]string, bits.OnesCount64(v.Present))}
		for i, name := range expDomains {
			if v.Present&(1<<i) != 0 {
				o.Checkpoint[name] = hex.EncodeToString(v.Hashes[:32])
				v.Hashes = v.Hashes[32:]
			}
		}

		if v.EventsPresent {
			o.Events = make([]Event, 0, len(v.Events))
		}
		for _, e := range v.Events {
			if e.Kind > 255 || e.Actor > 255 || e.Target > 255 || e.Sequence > 255 || e.Timestamp > 65535 || len(e.ProtoReflect().GetUnknown()) > 0 {
				return nil, invalidBlock
			}
			o.Events = append(o.Events, Event{e.Index, byte(e.Kind), byte(e.Actor), byte(e.Target), byte(e.Sequence), uint16(e.Timestamp)})
		}
		if e := validOperation(o); e != nil {
			return nil, e
		}
		rows = append(rows, o)
	}
	if uint32(len(rows)) != count {
		return nil, invalidBlock
	}
	return rows, nil
}
