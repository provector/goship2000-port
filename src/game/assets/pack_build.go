package assets

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// PackPrepared reads only a caller-selected verified loose generation and writes a new directory.
func PackPrepared(loose, dest string) error {
	r, err := newPreparedReader(loose)
	if err != nil {
		return err
	}
	defer r.close()
	if r.packs != nil {
		return fmt.Errorf("expected loose preparation as packing input")
	}
	r.observed = map[string]bool{}
	a, err := r.frontEnd()
	if err != nil {
		return err
	}
	if err = ValidateCurrentFrontEndCapabilities(a); err != nil {
		return err
	}
	used := r.observed
	r.observed = nil
	delete(used, "manifest.json")
	raw, err := r.read("manifest.json", 64<<20)
	if err != nil {
		return err
	}
	var m Manifest
	if err = DecodeObject(raw, &m); err != nil {
		return err
	}
	if m.PreparationVersion != 34 && m.PreparationVersion != PreparationVersion {
		return fmt.Errorf("unsupported preparation %d", m.PreparationVersion)
	}
	// Retain canonical proofs for conversion-only leaves referenced by historical metadata.
	if m.PreparationVersion >= 35 {
		// Conversion-only files will not be in the pack. Capture their historical
		// projection while the verified loose bytes are still available.
		_, r.historical, err = releaseProjection()
		if err != nil {
			return err
		}
	}
	r.canonical = map[string]leafProof{}
	if _, err = canonicalLogicalResourceDataMode(r, &m, true); err != nil {
		return err
	}
	if err = os.Mkdir(dest, 0700); err != nil {
		return err
	}
	c := packCatalog{Version: 1, Files: map[string]*packedFile{}, Omitted: map[string]leafProof{}}
	for p, proof := range r.canonical {
		if !used[p] {
			c.Omitted[p] = proof
		}
	}
	names := make([]string, 0, len(used))
	for p := range used {
		names = append(names, p)
	}
	sort.Strings(names)
	enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(3)), zstd.WithEncoderConcurrency(1))
	if err != nil {
		return err
	}
	defer enc.Close()
	for pack, name := range preparedPacks {
		f, err := os.OpenFile(filepath.Join(dest, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		h := sha256.New()
		w := zip.NewWriter(io.MultiWriter(f, h))
		for _, p := range names {
			o, ok := r.outputs[p]
			if !ok {
				f.Close()
				return fmt.Errorf("unregistered runtime asset %s", p)
			}
			bucket := 0
			if strings.HasPrefix(p, "audio/") || o.Kind == "audio metadata" {
				bucket = 1
			}
			if strings.HasPrefix(o.Kind, "startup_") {
				bucket = 2
			}
			if bucket != pack {
				continue
			}
			b, e := r.verified(o, maxPackedResource)
			if e != nil {
				f.Close()
				return e
			}
			item := &packedFile{Pack: pack, Size: len(b), Codec: "zstd"}
			chunkSize := max(1, len(b))
			switch {
			case strings.HasSuffix(p, ".glb"), strings.HasSuffix(p, ".png"):
				item.Codec = "store"
			case o.Kind == "startup_frame_indices":
				item.Codec = "frames"
				chunkSize = introChunkBytes
			case strings.HasSuffix(p, ".wav") && len(b) >= 44 && le16(b[34:]) == 16:
				if _, bits, e := decodeAudioWAV(b); e != nil || bits != 16 || string(b[36:40]) != "data" {
					f.Close()
					return fmt.Errorf("noncanonical prepared WAV %s", p)
				}
				item.Codec = "flac"
				item.Header = append([]byte(nil), b[:44]...)
				b = b[44:]
				chunkSize = musicChunkFrames * 2
			}
			for i := 0; i == 0 || len(b) > 0; i++ {
				n := min(chunkSize, len(b))
				data := b[:n]
				b = b[n:]
				switch item.Codec {
				case "zstd", "frames":
					data = enc.EncodeAll(data, nil)
				case "flac":
					data, e = encodePCMChunk(data)
					if e != nil {
						f.Close()
						return fmt.Errorf("encode %s: %w", p, e)
					}
				}
				member, e := w.CreateHeader(&zip.FileHeader{Name: chunkName(p, item.Codec, i), Method: zip.Store})
				if e == nil {
					_, e = member.Write(data)
				}
				if e != nil {
					f.Close()
					return e
				}
				item.Chunks = append(item.Chunks, packChunk{n, Hash(data)})
			}
			c.Files[p] = item
		}
		err = w.Close()
		if err == nil {
			err = f.Sync()
		}
		ce := f.Close()
		if err != nil {
			return err
		}
		if ce != nil {
			return ce
		}
		c.Digests[pack] = fmt.Sprintf("%x", h.Sum(nil))
	}
	raw, err = json.Marshal(c)
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(dest, packCatalogName), raw, 0600); err != nil {
		return err
	}
	m.InputDirectory = "."
	raw, err = JsonBytes(m)
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(dest, "manifest.json"), raw, 0600); err != nil {
		return err
	}
	// Current profiles verify every included pack and load the full frontend.
	// Older packs use their historical fingerprint, so check capability separately.
	if m.PreparationVersion < 35 {
		loaded, err := LoadFrontEndAssets(dest)
		if err != nil {
			return err
		}
		if err = ValidateCurrentFrontEndCapabilities(loaded); err != nil {
			return err
		}
	}
	_, err = LogicalResourceProfile(dest)
	return err
}
