package assets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gunship2000-assets/internal/audioopl"
	"path"
	"strings"
)

// Reuse a published film identity only after independently proving the runtime
// leaves match that release. Other valid assets keep their own semantic digest.
func verifiedRuntimeProfile(r *preparedReader, m *Manifest) (string, error) {
	leaves, err := runtimeLeaves(r)
	if err != nil {
		return "", err
	}
	released, err := matchesReleasedResources(r, m, leaves)
	if err != nil {
		return "", err
	}
	if released {
		return LogicalPreparation34, nil
	}
	return runtimeDigest(leaves)
}

// Hash the inputs opened by the runtime loaders. Physical source metadata,
// optional DOS persistence and extraction-only outputs do not describe playback.
func runtimeLeaves(r *preparedReader) (map[string]string, error) {
	r.observed = map[string]bool{}
	a, err := r.frontEnd()
	if err != nil {
		return nil, err
	}
	if err = ValidateCurrentFrontEndCapabilities(a); err != nil {
		return nil, err
	}
	used := r.observed
	r.observed = nil
	delete(used, "manifest.json")
	if r.packs != nil && len(used) != len(r.packs.catalog.Files) {
		return nil, fmt.Errorf("unexpected or missing packed runtime resources")
	}
	leaves := map[string]string{}
	for p := range used {
		o, ok := r.outputs[p]
		if !ok {
			return nil, fmt.Errorf("unregistered runtime resource %s", p)
		}

		if r.packs != nil {
			if f := r.packs.catalog.Files[p]; f != nil && (f.Codec == "frames" || f.Codec == "flac") {
				if err := r.packs.verifyMedia(p, o); err != nil {
					return nil, err
				}
				leaves[p] = o.SHA256
				continue
			}
		}
		b, e := r.verified(o, maxPackedResource)
		if e != nil {
			return nil, e
		}
		if strings.HasSuffix(p, ".json") {
			var v any
			d := json.NewDecoder(bytes.NewReader(b))
			d.UseNumber()
			if err = d.Decode(&v); err != nil {
				return nil, err
			}
			if o.Kind == "audio metadata" {
				doc, _ := v.(map[string]any)
				comp, _ := doc["component"].(map[string]any)
				platform, _ := comp["platform"].(string)
				digest, _ := comp["helper_sha256"].(string)
				if _, ok := audioopl.ReferenceHelper(platform, digest); !ok {
					return nil, fmt.Errorf("unsupported audio helper %s", platform)
				}
				delete(comp, "helper_sha256")
				delete(comp, "platform")
			}
			semanticMetadata(v)
			b, e = json.Marshal(v)
			if e != nil {
				return nil, e
			}
		} else if preparedPNGOutput(o) {
			b, e = decodedPNGIdentity(b)
			if e != nil {
				return nil, e
			}
		} else if strings.EqualFold(path.Base(p), "PHIL1BMP.GS2") {
			b, _, e = CorrectKnownMapAtlas("PHIL1BMP.GS2", b)
			if e != nil {
				return nil, e
			}
		}
		leaves[p] = Hash(b)
	}
	return leaves, nil
}

func runtimeDigest(leaves map[string]string) (string, error) {
	data, err := json.Marshal(leaves)
	if err != nil {
		return "", err
	}
	return Hash(data), nil
}
func semanticMetadata(v any) {
	switch x := v.(type) {
	case []any:
		for _, k := range x {
			semanticMetadata(k)
		}
	case map[string]any:
		if _, source := x["payload_sha256"]; source && (x["kind"] == "archive_entry" || x["kind"] == "loose_file" || x["kind"] == "executable_slice") {
			clear(x)
			return
		}
		if _, bundle := x["geometries"]; bundle {
			delete(x, "source_sha256")
		}
		if _, output := x["size_bytes"]; output && x["path"] != nil && x["kind"] != nil {
			delete(x, "sha256")
			delete(x, "size_bytes")
		}
		for _, k := range x {
			semanticMetadata(k)
		}
	}
}
