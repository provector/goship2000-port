package assets

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Resolve only the selected installation and the conventional CD wrapper.
// Unrelated subdirectories, including another installation's backup, are never searched.
func OriginalDirectory(root string) (string, error) {
	list, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	var nested string
	for _, v := range list {
		if strings.EqualFold(v.Name(), "GS.GS2") || strings.EqualFold(v.Name(), "GS2000.CAT") {
			return root, nil
		}
	}
	for _, v := range list {
		if v.IsDir() && strings.EqualFold(v.Name(), "GS2000CD") {
			if nested != "" {
				return "", fmt.Errorf("ambiguous GS2000CD directories in %s", root)
			}
			nested = filepath.Join(root, v.Name())
		}
	}
	if nested != "" {
		return nested, nil
	}
	return root, nil
}

func preparationFiles() map[string]bool {
	result := map[string]bool{"OBJDBASE.GS2": true, "REPLAY.PIC": true}
	for _, s := range IslandIce469085.Sources {
		switch s.Role {
		case roleLauncher, roleCDLauncher, roleSetupLauncher, roleOfficeScreen, roleOfficePalette:
			continue
		}
		result[s.Name] = true
	}
	for _, family := range []string{"APA", "BLK", "COB", "COM", "DEF", "KIA"} {
		for _, suffix := range []string{"_CPIT.PIC", "_CPIT.DAT", "_STAT.DAT"} {
			result[family+suffix] = true
		}
	}
	for _, world := range IslandIce469085.WorldLayout.WorldNames {
		for _, suffix := range []string{"", "MAP.GS2", "SMP.GS2", "BMP.GS2"} {
			result[world+suffix] = true
		}
		for _, suffix := range []string{"DAY.PAL", "GREY.PAL", "DARK.PAL", "FORM.DAT", "WEAP.DAT"} {
			result[world[:4]+suffix] = true
		}
	}
	return result
}

func selectedFiles(root string, wanted map[string]bool) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	found := map[string]string{}
	var files []os.DirEntry
	for _, d := range entries {
		name := strings.ToUpper(d.Name())
		if !wanted[name] {
			continue
		}
		if prev := found[name]; prev != "" {
			return nil, fmt.Errorf("conflicting DOS input %s and %s in %s", prev, d.Name(), root)
		}
		found[name] = d.Name()
		files = append(files, d)
	}
	var missing []string
	for n := range wanted {
		if found[n] == "" {
			missing = append(missing, n)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required input in %s: %s", root, strings.Join(missing, ", "))
	}
	sort.Slice(files, func(i, j int) bool { return strings.ToUpper(files[i].Name()) < strings.ToUpper(files[j].Name()) })
	return files, nil
}

func (r *Resource) fromCatalog(name string) bool {
	return r.Source.Kind == "archive_entry" && strings.EqualFold(filepath.Base(r.Source.Container), name)
}
