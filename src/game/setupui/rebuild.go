package setupui

import (
	"fmt"
	"github.com/rivo/tview"
	"gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/managedassets"
	"io"
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
	view := tview.NewTextView().SetDynamicColors(true).SetWrap(true).SetWordWrap(true)
	view.SetBackgroundColor(red)
	view.SetBorder(true).SetTitle("Rebuild Assets - one shot").SetBackgroundColor(red)
	view.SetTextColor(yellow)
	u.overlay(view, 72, 13)
	u.busy = true
	updates := make(chan assets.PreparationProgress, 1)
	done := make(chan error, 1)
	discovered := ""
	u.writers.Add(1)
	go func() {
		defer u.writers.Done()
		var logs strings.Builder
		services := managedassets.FrontEndServices(func(p assets.PreparationProgress) {
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
		services.SourceSelected = func(path string) error { discovered = path; return nil }
		paths := configuration.Paths{Directory: dir, Original: configuration.Relative(dir, c.Original), Assets: configuration.Relative(dir, c.Assets), Saves: configuration.SavePath(dir, data, c.Saves), AssetsOrigin: "configured-default-managed", Config: c}
		_, err := managedassets.Ensure(paths, true, io.MultiWriter(&logs), services)
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
					count := "Determining work..."
					bar := strings.Repeat("░", 48)
					if p.OverallTotal > 0 {
						n := int(p.Fraction() * 48)
						bar = strings.Repeat("█", n) + strings.Repeat("░", 48-n)
						count = fmt.Sprintf("Overall %3.0f%%", p.Fraction()*100)
					}
					if p.Total > 0 {
						count += fmt.Sprintf(" · stage %d / %d", p.Completed, p.Total)
					}
					view.SetText(tview.Escape(p.Stage) + "\n\n[#ffff00:#0000ff]" + bar + "[-:#800000]\n" + tview.Escape(count) + "\n\nPrevious assets are preserved until validated.\nRebuild must finish before this window can close.")
				})
			case err := <-done:
				u.App.QueueUpdateDraw(func() {
					u.busy = false
					u.closePopup()
					if err != nil {
						u.message("Rebuild failed:\n" + err.Error())
					} else {
						if discovered != "" {
							u.Doc.Config.Original = discovered
						}
						u.message("Rebuild complete. The prepared assets are ready to use.")
					}
				})
				return
			}
		}
	}()
}
