package playerfile

import (
	"os"
	"strconv"
	"strings"
)

// Portable titles already have a 20-byte printable ASCII limit. Restrict only
// filesystem spellings, using the same rules on all supported platforms.
func Basename(title string) string {
	title = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, title)
	title = strings.Trim(title, " .")
	if title == "" {
		return "Mission"
	}
	stem, _, _ := strings.Cut(strings.ToUpper(title), ".")
	stem = strings.TrimRight(stem, " ")
	switch stem {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$", "CLOCK$":
		title = "_" + title
	default:
		if len(stem) == 4 && (stem[:3] == "LPT" || stem[:3] == "COM") && stem[3] >= '0' && stem[3] <= '9' {
			title = "_" + title
		}
	}
	return strings.TrimRight(title[:min(20, len(title))], " .")
}

type Names map[string]int

func NamesIn(entries []os.DirEntry) Names {
	n := Names{}
	for _, e := range entries {
		n[strings.ToLower(e.Name())]++
	}
	return n
}
func (n Names) Choose(title, source, extension string) string {
	base := Basename(title)
	for i := 1; ; i++ {
		t := base
		if i > 1 {
			suffix := " (" + strconv.Itoa(i) + ")"
			t = strings.TrimRight(base[:min(len(base), 20-len(suffix))], " .") + suffix
		}
		key := strings.ToLower(t + extension)
		if n[key] == 0 || n[key] == 1 && strings.EqualFold(source, key) {
			return t
		}
	}
}
