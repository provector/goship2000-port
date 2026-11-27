//go:build headless

package menus

import "fmt"

func RunOfficeWindow(_ *Office, _ WindowOptions) error {
	return fmt.Errorf("this binary was built with -tags headless; use dev -headless -frame NEW.png or build without that tag for a window")
}

func CheckGameWindow() error { return fmt.Errorf("check-window requires a graphical build") }
