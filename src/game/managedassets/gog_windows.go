package managedassets

import "golang.org/x/sys/windows/registry"

func gogRoots() []string {
	var out []string
	for _, hive := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		for _, view := range []uint32{registry.WOW64_32KEY, registry.WOW64_64KEY} {
			k, e := registry.OpenKey(hive, `Software\GOG.com\Games`, registry.ENUMERATE_SUB_KEYS|view)
			if e != nil {
				continue
			}
			names, _ := k.ReadSubKeyNames(-1)
			for _, name := range names {
				game, e := registry.OpenKey(k, name, registry.QUERY_VALUE|view)
				if e != nil {
					continue
				}
				title, _, _ := game.GetStringValue("gameName")
				path, _, _ := game.GetStringValue("path")
				game.Close()
				if gogTitle(title) {
					out = append(out, path)
				}
			}
			k.Close()
		}
	}
	return out
}
