package assets

import (
	"os"
	"strings"
)

// Parser tests supply their own tiny input sets independently of the retail installation.
func prepareAssets(input, output string) (*Manifest, error) {
	files, e := os.ReadDir(input)
	if e != nil {
		return nil, e
	}
	selected := map[string]bool{}
	for _, f := range files {
		selected[strings.ToUpper(f.Name())] = true
	}
	return prepareSelected(input, output, nil, nil, selected, nil)
}
