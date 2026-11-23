package managedassets

import (
	"golang.org/x/sys/windows/registry"
	"path/filepath"
)

func steamRoots() []string {
	var roots []string
	for _, hive := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		for _, view := range []uint32{0, registry.WOW64_32KEY, registry.WOW64_64KEY} {
			k, e := registry.OpenKey(hive, `Software\Valve\Steam`, registry.QUERY_VALUE|view)
			if e != nil {
				continue
			}
			for _, name := range []string{"SteamPath", "InstallPath", "SteamExe"} {
				p, _, e := k.GetStringValue(name)
				if e == nil && p != "" {
					if name == "SteamExe" {
						p = filepath.Dir(p)
					}
					roots = append(roots, p)
				}
			}
			k.Close()
		}
	}
	return roots
}
