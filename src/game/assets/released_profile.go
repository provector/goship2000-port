package assets

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"gunship2000-assets/internal/legacyprofile"
)

// The historical writer hashed provenance and extraction-only entries together
// with gameplay data. This template supplies the obsolete framing only. It has
// empty slots for ALL runtime outputs; success requires the old published hash.
// This data only translates historical identities. New cache admission is independent.
type historicalProjection struct {
	sources      map[string]map[string]any
	worldSources map[string]any
}

func sourceKey(src map[string]any) string {
	name, _ := src["path"].(string)
	key := strings.ToUpper(filepath.Base(name))
	if rec, ok := src["directory_record"].(map[string]any); ok {
		member, _ := rec["name_readable"].(string)
		key += "/" + strings.ToUpper(member)
	}
	return key
}

type releasedFraming struct {
	Version  string
	Manifest map[string]any
}

func releaseProjection() (*releasedFraming, *historicalProjection, error) {
	z, err := gzip.NewReader(bytes.NewReader(legacyprofile.Preparation34))
	if err != nil {
		return nil, nil, err
	}
	defer z.Close()
	var old releasedFraming
	d := json.NewDecoder(io.LimitReader(z, 16<<20))
	d.UseNumber()
	if err = d.Decode(&old); err != nil {
		return nil, nil, err
	}
	rows, ok := old.Manifest["resources"].([]any)
	if !ok {
		return nil, nil, fmt.Errorf("historical resource framing missing")
	}
	projection := &historicalProjection{sources: map[string]map[string]any{}, worldSources: map[string]any{}}
	for _, v := range rows {
		row := v.(map[string]any)
		src := row["source"].(map[string]any)
		key := sourceKey(src)
		projection.sources[key] = src
		if src["kind"] == "loose_file" {
			projection.worldSources[key] = src["payload_sha256"]
		}
	}
	return &old, projection, nil
}

func matchesReleasedResources(r *preparedReader, m *Manifest, live map[string]string) (bool, error) {
	old, projection, err := releaseProjection()
	if err != nil {
		return false, err
	}
	rows := old.Manifest["resources"].([]any)
	r.historical = projection
	r.canonical = map[string]leafProof{}
	if _, err = canonicalLogicalResourceDataMode(r, m, true); err != nil {
		return false, err
	}
	matched := make(map[string]bool, len(live))
	for _, v := range rows {
		outputs, _ := v.(map[string]any)["outputs"].([]any)
		for _, item := range outputs {
			o := item.(map[string]any)
			if o["sha256"] != "" {
				continue
			}
			p := o["path"].(string)
			leaf, ok := r.canonical[p]
			if !ok || live[p] == "" {
				return false, nil
			}
			matched[p] = true
			o["sha256"], o["size_bytes"] = leaf.Hash, leaf.Size
		}
	}
	if len(matched) != len(live) {
		return false, nil
	}
	b, err := json.Marshal(old)
	if err != nil {
		return false, err
	}
	return Hash(b) == decodedPreparation34, nil
}
