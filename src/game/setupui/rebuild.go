package setupui

import (
	"fmt"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/diagnostics"
	"gunship2000-assets/game/managedassets"
	"gunship2000-assets/internal/branding"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (u *UI) runRebuild() {
	dir := u.Doc.InstallDirectory()
	data := filepath.Dir(u.Doc.Path)
	c := u.Doc.Config
	// An explicit frozen/reference bundle is never a SETUP cache replacement.
	if filepath.IsAbs(c.Assets) || filepath.Clean(c.Assets) != "assets" {
		u.message("Rebuild preserves selected external assets. Choose the app-owned 'assets' cache in Debug > Advanced first.\nSelected: " + configuration.Relative(dir, c.Assets))
		return
	}
	if e := configuration.ValidateStorePaths(dir, data, c); e != nil {
		u.message(e.Error())
		return
	}
	if e := c.Validate(); e != nil {
		u.message(e.Error())
		return
	}
	view := &rebuildProgressView{Box: tview.NewBox().SetBorder(true).SetTitle("Rebuild Assets - one shot").SetBackgroundColor(red)}
	u.overlay(view, 72, 13)
	u.busy = true
	updates := make(chan assets.PreparationProgress, 1)
	done := make(chan error, 1)
	discovered := ""
	u.writers.Add(1)
	go func() {
		defer u.writers.Done()
		var logs strings.Builder
		stage := "Locate original installation"
		services := managedassets.FrontEndServices(func(p assets.PreparationProgress) {
			stage = p.Stage
			select {
			case updates <- p:
			default:
				select {
				case <-updates:
				default:
				}
				select {
				case updates <- p:
				default:
				}
			}
		})
		services.SourceUsed = func(path string) { discovered = path }
		paths := configuration.Paths{Directory: dir, Original: configuration.Relative(dir, c.Original), Assets: configuration.Relative(dir, c.Assets), Saves: configuration.SavePath(dir, data, c.Saves), AssetsOrigin: "configured-default-managed", Config: c}
		session, sessionErr := diagnostics.StartSession(paths.Saves, branding.ProductTitle+" "+branding.Flavor, "SETUP rebuild")
		if sessionErr != nil {
			fmt.Fprintln(os.Stderr, "SETUP diagnostics:", sessionErr)
		}
		if session != nil {
			defer session.Close()
		}
		_, err := managedassets.Ensure(paths, true, io.MultiWriter(&logs), services)
		if err != nil {
			err = diagnostics.StartupFailure(stage, fmt.Errorf("%s (original %s, destination %s): %w", stage, paths.Original, paths.Assets, err))
		}
		done <- err
	}()
	go func() {
		timer := time.NewTicker(100 * time.Millisecond)
		defer timer.Stop()
		var latest assets.PreparationProgress
		for {
			select {
			case p := <-updates:
				latest = p
			case <-timer.C:
				p := latest
				u.App.QueueUpdateDraw(func() {
					view.progress = p
				})
			case err := <-done:
				u.App.QueueUpdateDraw(func() {
					u.busy = false
					u.closePopup()
					if err != nil {
						u.message("Rebuild failed:\n" + err.Error())
					} else {
						message := "Rebuild complete. The prepared assets are ready to use."
						if discovered != "" {
							message += "\nSource used: " + discovered
						}
						if configured, ok := managedassets.PreferredInstallation(c, dir, nil); ok && configured.Path != discovered && discovered != "" {
							message += "\nPreferred source: " + configured.String() + ". See diagnostics for the preparation result."
						}
						u.message(message)
					}
				})
				return
			}
		}
	}()
}

type rebuildProgressView struct {
	*tview.Box
	progress assets.PreparationProgress
}

func (v *rebuildProgressView) Draw(screen tcell.Screen) {
	v.Box.DrawForSubclass(screen, v)
	x, y, width, height := v.GetInnerRect()
	if width <= 0 || height <= 0 {
		return
	}
	p := v.progress
	size := min(48, width)
	filled := 0
	count := "Determining work..."
	if p.OverallTotal > 0 {
		filled = int(p.Fraction() * float64(size))
		count = fmt.Sprintf("Overall %3.0f%%", p.Fraction()*100)
	}
	if p.Total > 0 {
		count += fmt.Sprintf(" · stage %d / %d", p.Completed, p.Total)
	}
	bar := "[#ffff00:#0000ff]" + strings.Repeat("█", filled) + strings.Repeat("░", size-filled) + "[-:-]"
	footer := []string{"", bar, count, ""}
	for _, notice := range []string{"Previous assets are preserved until validated.", "Rebuild must finish before this window can close."} {
		footer = append(footer, tview.WordWrap(notice, width)...)
	}
	stage := tview.WordWrap(tview.Escape(p.Stage), width)
	stage = stage[:min(len(stage), max(0, height-len(footer)))]
	lines := append(stage, footer...)
	y += max(0, (height-len(lines))/2)
	for i, line := range lines[:min(len(lines), height)] {
		tview.Print(screen, strings.TrimSpace(line), x, y+i, width, tview.AlignCenter, yellow)
	}
}
