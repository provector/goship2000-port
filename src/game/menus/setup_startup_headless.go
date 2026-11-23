//go:build headless

package menus

import (
	"fmt"
	assetdata "gunship2000-assets/game/assets"
)

func ConfigurationWarning(message string) error { return fmt.Errorf("%s", message) }
func PreparationWindow(work func(func(assetdata.PreparationProgress)) error) error {
	return work(func(p assetdata.PreparationProgress) {
		fmt.Printf("Preparation: %s %d/%d\n", p.Stage, p.Completed, p.Total)
	})
}
