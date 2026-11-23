package menus

import "gunship2000-assets/game/cockpit"

func (p *RenderingPreview) supportStrikeSnapshot() *cockpit.MapPoint {
	if p.Initialized == nil || p.Initialized.Support == nil || p.Initialized.Support.Request == 0 {
		return nil
	}
	t := p.Initialized.Support.Target
	return &cockpit.MapPoint{H0: t.H0, H1: t.H1}
}

// Publish at reached logical/UI mutation boundaries, never while drawing.
// GPS's read-only overlay is not a checkpoint field. Cockpit composition feeds
// the canonical reference, so historical films retain their recorded no-marker
// interpretation while newly selected P7 launches get the original shared pass.
func (p *RenderingPreview) publishSupportStrike() {
	if p.Cockpit.Map != nil {
		p.Cockpit.Map.SupportStrike = nil
		if p.p7Mechanics {
			p.Cockpit.Map.SupportStrike = p.supportStrikeSnapshot()
		}
	}
	if p.GPS != nil {
		p.GPS.SupportStrike = p.supportStrikeSnapshot()
	}
}
