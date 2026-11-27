//go:build headless

package menus

import (
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/diagnostics"
)

func PreparationWindow(saves string, work func(func(assetdata.PreparationProgress)) error) error {
	return diagnostics.GuardStartup(func() error {
		return work(func(p assetdata.PreparationProgress) {
			fmt.Printf("Preparation: %s %d/%d\n", p.Stage, p.Completed, p.Total)
		})
	})
}
