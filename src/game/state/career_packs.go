package state

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/internal/userpack"
)

const packIntent = ".player-pack-intent.json"

type slotHeader struct {
	Version int    `json:"version"`
	Store   string `json:"store_id"`
	Slot    int    `json:"slot"`
	Pilot   string `json:"pilot_id,omitempty"`
}
type storeHeader struct {
	Version      int              `json:"version"`
	Revision     uint64           `json:"revision"`
	Selected     string           `json:"selected_pilot_id,omitempty"`
	Legacy       *legacyReference `json:"legacy_import,omitempty"`
	ImportRoster []byte           `json:"original_roster_base64,omitempty"`
}
type careerPacks struct {
	dir, cfg, id string
	roster       rosterDocument
	meta         []byte
	importRoster []byte
	slots        [6]userpack.Entries
	records      userpack.Entries // File extents, not decoded histories.
	owners       map[string]string
	home         map[string]int
}

func playerPack(i int) string { return fmt.Sprintf("player%d.g2pak", i) }
func existingPacks(d string) bool {
	for i := 0; i < 6; i++ {
		if _, e := os.Lstat(filepath.Join(d, playerPack(i))); !os.IsNotExist(e) {
			return true
		}
	}
	_, e := os.Lstat(filepath.Join(d, packIntent))
	return !os.IsNotExist(e)
}
func (p *careerPacks) metaName() string { return "careers/" + p.id + ".json" }
func validStoreID(id string) bool {
	b, e := hex.DecodeString(id)
	return e == nil && len(b) == 16 && strings.ToLower(id) == id
}
func (p *careerPacks) refresh() error {
	if _, e := os.Lstat(filepath.Join(p.dir, packIntent)); !os.IsNotExist(e) {
		return fmt.Errorf("pending career transaction; reopen to recover")
	}
	p.records = userpack.Entries{}
	p.home = map[string]int{}
	p.owners = map[string]string{}
	p.roster = rosterDocument{SchemaVersion: rosterSchemaVersion}
	for i := range p.slots {
		a, e := userpack.Open(filepath.Join(p.dir, playerPack(i)))
		if e != nil {
			return e
		}
		p.slots[i] = a
		b, e := a.Get("slot.json")
		if e != nil {
			return e
		}
		var h slotHeader
		if e = assetdata.DecodeObject(b, &h); e != nil || h.Version != 1 || h.Slot != i || !validStoreID(h.Store) || h.Pilot != "" && !validPilotID(h.Pilot) {
			return fmt.Errorf("invalid slot header %d", i)
		}
		if p.id == "" {
			p.id = h.Store
		}
		if h.Store != p.id {
			return fmt.Errorf("mixed career generations")
		}
		p.roster.Slots[i] = h.Pilot
		for n, v := range a {
			if n == "slot.json" {
				continue
			}
			id, key, ok := strings.Cut(n, "/")
			if !ok || !validPilotID(id) || !safeSaveTarget(key) || key == saveFilename || !isSortieTarget(key) && key != pilotFilename(id) && key != backupFilename(id) && key != sortieReferenceFilename(id) {
				return fmt.Errorf("invalid player entry %s", n)
			}
			if j, known := p.home[id]; known && j != i {
				return fmt.Errorf("split pilot ownership %s", id)
			}
			p.home[id] = i
			if _, dup := p.records[key]; dup {
				return fmt.Errorf("duplicate career entry %s", key)
			}
			p.records[key] = v
			p.owners[key] = id
		}
	}
	for i, id := range p.roster.Slots {
		if id != "" {
			j, ok := p.home[id]
			if !ok || j != i {
				return fmt.Errorf("slot %d lacks its pilot", i)
			}
		}
	}
	b, e := userpack.ReadConfig(filepath.Dir(p.cfg), p.metaName())
	if e != nil {
		return fmt.Errorf("career selection metadata: %w", e)
	}
	var m storeHeader
	if e = assetdata.DecodeObject(b, &m); e != nil || m.Version != 1 {
		return fmt.Errorf("invalid career metadata")
	}
	p.meta = b
	p.importRoster = m.ImportRoster
	p.roster.Revision = m.Revision
	p.roster.SelectedPilotID = m.Selected
	p.roster.Legacy = m.Legacy
	raw, e := assetdata.JsonBytes(p.roster)
	if e != nil {
		return e
	}
	p.records[saveFilename] = userpack.Value(raw)
	return nil
}
func (s *SaveStore) readRecord(name string) ([]byte, error) {
	if s.packed != nil {
		return s.packed.records.Get(name)
	}
	return ReadRegularSave(filepath.Join(s.Directory, filepath.FromSlash(name)))
}
func (s *SaveStore) currentRecord(name string) ([]byte, error) {
	b, e := s.readRecord(name)
	if os.IsNotExist(e) {
		return nil, nil
	}
	return b, e
}
func (s *SaveStore) sortieNames() ([]string, error) {
	var names []string
	if s.packed != nil {
		for k := range s.packed.records {
			if isSortieTarget(k) {
				names = append(names, strings.TrimPrefix(k, "sorties/"))
			}
		}
		sort.Strings(names)
		return names, nil
	}
	entries, e := os.ReadDir(filepath.Join(s.Directory, "sorties"))
	for _, ent := range entries {
		if ent.IsDir() || ent.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("invalid sortie directory entry")
		}
		names = append(names, ent.Name())
	}
	return names, e
}
func (s *SaveStore) commitRecords(expected, desired map[string][]byte, extra map[string]*[]byte) error {
	if s.packed == nil {
		return commitSaveFiles(s.Directory, expected, desired, extra)
	}
	p := s.packed
	for n, want := range expected {
		b, e := s.readRecord(n)
		if e != nil || !bytes.Equal(b, want) {
			return fmt.Errorf("save changed outside this session (%s)", n)
		}
	}
	changes := map[string]*[]byte{}
	for n, b := range desired {
		if !bytes.Equal(expected[n], b) {
			v := b
			changes[n] = &v
		}
	}
	for n, b := range extra {
		changes[n] = b
	}
	for n, b := range changes {
		if !safeSaveTarget(n) {
			return fmt.Errorf("invalid save target")
		}
		if b != nil && expected[n] == nil && !strings.HasSuffix(n, ".backup.json") {
			if _, exists := p.records[n]; exists {
				return fmt.Errorf("unreferenced career record %s already exists", n)
			}
		}
	}
	return p.publish(changes, false)
}

type packImage struct {
	Slot                 int
	Stage, Before, After string
}
type packJournal struct {
	Version       int
	Store         string
	Images        []packImage
	Before, After []byte
}

func (p *careerPacks) publish(changes map[string]*[]byte, initial bool) error {
	if len(changes) == 0 && !initial {
		return nil
	}
	roster := p.roster
	if b, ok := changes[saveFilename]; ok {
		if b == nil {
			return fmt.Errorf("missing roster")
		}
		if e := assetdata.DecodeObject(*b, &roster); e != nil {
			return e
		}
	}
	homes := map[string]int{}
	for id, i := range p.home {
		homes[id] = i
	}
	for i, id := range roster.Slots {
		if id != "" {
			homes[id] = i
		}
	}
	var images [6]userpack.Entries
	var dirty [6]bool
	for i := range images {
		images[i] = userpack.Entries{}
		dirty[i] = initial || roster.Slots[i] != p.roster.Slots[i]
		h, _ := json.Marshal(slotHeader{1, p.id, i, roster.Slots[i]})
		images[i]["slot.json"] = userpack.Value(h)
	}
	for n, v := range p.records {
		if n == saveFilename {
			continue
		}
		id := p.owners[n]
		i := homes[id]
		images[i][id+"/"+n] = v
		if i != p.home[id] {
			dirty[i] = true
			dirty[p.home[id]] = true
		}
	}
	for n, b := range changes {
		if n == saveFilename {
			continue
		}
		id := p.owners[n]
		if id == "" {
			if isSortieTarget(n) {
				if b == nil {
					return fmt.Errorf("unknown sortie")
				}
				d, e := decodeSortieRecord(*b, strings.TrimSuffix(strings.TrimPrefix(n, "sorties/"), ".json"))
				if e != nil {
					return e
				}
				id = d.Start.PilotID
			} else {
				id = strings.Split(strings.TrimPrefix(n, "pilots/"), ".")[0]
			}
		}
		slot, ok := homes[id]
		if !ok {
			return fmt.Errorf("unknown pilot owner for %s", n)
		}
		dirty[slot] = true
		if b == nil {
			delete(images[slot], id+"/"+n)
		} else {
			images[slot][id+"/"+n] = userpack.Value(*b)
		}
	}
	source := p.importRoster
	if l := roster.Legacy; l != nil {
		if source == nil {
			var e error
			source, e = readArchivedRoster(p.dir, l)
			if e != nil {
				return e
			}
		}
		if len(source) != 7*LegacyRecordSize || assetdata.Hash(source) != l.SourceSHA256 {
			return fmt.Errorf("legacy import bytes changed")
		}
	} else {
		source = nil
	}
	meta, e := assetdata.JsonBytes(storeHeader{Version: 1, Revision: roster.Revision, Selected: roster.SelectedPilotID, Legacy: roster.Legacy, ImportRoster: source})
	if e != nil {
		return e
	}
	return userpack.Locked(p.cfg, func() error {
		current, e := userpack.ReadConfig(filepath.Dir(p.cfg), p.metaName())
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		if !bytes.Equal(current, p.meta) {
			return fmt.Errorf("shared career selection/revision conflict")
		}
		journal := packJournal{Version: 1, Store: p.id, Before: p.meta, After: meta}
		durable := false
		defer func() {
			if !durable {
				for _, v := range journal.Images {
					os.Remove(filepath.Join(p.dir, v.Stage))
				}
			}
		}()
		for i, a := range images {
			if !dirty[i] {
				continue
			}
			before, e := userpack.HashFile(filepath.Join(p.dir, playerPack(i)))
			if e != nil && !(initial && os.IsNotExist(e)) {
				return e
			}
			if initial && before != "" {
				return fmt.Errorf("occupied slot pack %d", i)
			}
			stage, e := userpack.Stage(p.dir, a)
			if e != nil {
				return e
			}
			after, e := userpack.HashFile(stage)
			if e != nil {
				return e
			}
			journal.Images = append(journal.Images, packImage{i, filepath.Base(stage), before, after})
		}
		raw, e := assetdata.JsonBytes(journal)
		if e != nil {
			return e
		}
		if _, e = os.Lstat(filepath.Join(p.dir, packIntent)); !os.IsNotExist(e) {
			return fmt.Errorf("pending pack transaction already exists")
		}
		if e = writeSaveAtomic(p.dir, packIntent, raw); e != nil {
			_, statErr := os.Lstat(filepath.Join(p.dir, packIntent))
			durable = !os.IsNotExist(statErr)
			return e
		}
		durable = true
		if e = userpack.SyncDir(p.dir); e != nil {
			return e
		}
		if e = p.rollForward(journal); e != nil {
			return e
		}
		return p.refresh()
	})
}
func (p *careerPacks) rollForward(j packJournal) error {
	if j.Version != 1 || !validStoreID(j.Store) || len(j.Images) > 6 || p.id != "" && p.id != j.Store {
		return fmt.Errorf("invalid career journal")
	}
	p.id = j.Store
	var header storeHeader
	if e := assetdata.DecodeObject(j.After, &header); e != nil || header.Version != 1 {
		return fmt.Errorf("invalid journal career metadata")
	}
	current, e := userpack.ReadConfig(filepath.Dir(p.cfg), p.metaName())
	if e != nil && !os.IsNotExist(e) {
		return e
	}
	if !bytes.Equal(current, j.Before) && !bytes.Equal(current, j.After) {
		return fmt.Errorf("career journal metadata conflict")
	}
	seen := [6]bool{}
	var pending []packImage
	for _, v := range j.Images {
		if v.Slot < 0 || v.Slot >= 6 || seen[v.Slot] || filepath.Base(v.Stage) != v.Stage || !strings.HasPrefix(v.Stage, ".g2pak-stage-") || !validSortieHash(v.After) || v.Before != "" && !validSortieHash(v.Before) {
			return fmt.Errorf("invalid career pack journal image")
		}
		seen[v.Slot] = true
		actual, e := userpack.HashFile(filepath.Join(p.dir, playerPack(v.Slot)))
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		if actual == v.After {
			continue
		}
		if actual != v.Before {
			return fmt.Errorf("slot %d changed outside pending transaction", v.Slot)
		}
		staged, e := userpack.HashFile(filepath.Join(p.dir, v.Stage))
		if e != nil || staged != v.After {
			return fmt.Errorf("missing or changed staged slot %d", v.Slot)
		}
		pending = append(pending, v)
	}
	for _, v := range pending {
		if e := userpack.Replace(filepath.Join(p.dir, v.Stage), filepath.Join(p.dir, playerPack(v.Slot))); e != nil {
			return e
		}
	}
	if !bytes.Equal(current, j.After) {
		if e := userpack.PatchLocked(p.cfg, []userpack.Change{{Name: p.metaName(), Before: current, After: j.After}}); e != nil {
			return e
		}
	}
	if e := os.Remove(filepath.Join(p.dir, packIntent)); e != nil {
		return e
	}
	return userpack.SyncDir(p.dir)
}
func (s *SaveStore) quarantine(reason string) error {
	if s.packed == nil {
		return s.quarantineUnfinishedSorties(reason)
	}
	p := s.packed
	discard := map[string]*[]byte{}
	committed := map[string]string{}
	for n, id := range p.owners {
		if !isSortieTarget(n) {
			continue
		}
		b, e := p.records.Get(n)
		if e != nil {
			return e
		}
		d, e := decodeSortieRecord(b, strings.TrimSuffix(strings.TrimPrefix(n, "sorties/"), ".json"))
		if e == nil && d.Start.PilotID == id && d.Phase == SortieCommitted {
			committed[d.Start.ID] = id
		} else {
			discard[n] = nil
		}
	}
	for n, id := range p.owners {
		if !isSortieReferenceTarget(n) {
			continue
		}
		b, e := p.records.Get(n)
		if e != nil {
			return e
		}
		ref, e := decodeSortieReference(b, id)
		if e != nil || committed[ref.SortieID] != id {
			discard[n] = nil
		}
	}
	if len(discard) == 0 {
		return nil
	}
	dir := filepath.Join(p.dir, abandonedSessionDirectory)
	if e := plainDir(dir); e != nil {
		return e
	}
	for n := range discard {
		b, e := p.records.Get(n)
		if e != nil {
			return e
		}
		name := filepath.Base(n) + "." + assetdata.Hash(b)
		if e = writeSaveAtomic(dir, name, b); e != nil {
			return e
		}
		if e = writeSaveAtomic(dir, name+".txt", []byte(reason+"\nOriginal path: "+n+"\n")); e != nil {
			return e
		}
	}
	return p.publish(discard, false)
}
func newStoreID() (string, error) {
	var b [16]byte
	_, e := rand.Read(b[:])
	return hex.EncodeToString(b[:]), e
}

// ValidateStoredRoster checks an existing SETUP destination without importing,
// initializing or recovering files in that store.
func ValidateStoredRoster(dir, config string) error {
	p := careerPacks{dir: dir, cfg: config}
	if err := p.refresh(); err != nil {
		return err
	}
	for _, id := range p.roster.Slots {
		if id != "" {
			if _, ok := p.records[pilotFilename(id)]; !ok {
				return fmt.Errorf("missing pilot record for %s", id)
			}
		}
	}
	for _, entry := range p.records {
		if _, err := entry.Read(); err != nil {
			return err
		}
	}
	return nil
}
