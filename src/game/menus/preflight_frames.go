package menus

import (
	assetdata "gunship2000-assets/game/assets"
)

// POINTER contains nine original 5x5 bevel tiles at y100 and two fill/ink
// samples at y105. A1d926 caches those samples; A1da72 stretches their edges.
func drawPreflightFrame(a *assetdata.MissionAssets, frame []byte, r [4]int, style, fill int) {
	if a == nil || len(frame) != 64000 || len(a.Images["POINTER.PIC"]) != 64000 || style < 0 || style > 8 || fill < 0 || fill > 1 || r[2] < 5 || r[3] < 5 || !assetdata.OfficeRectFits(r) {
		return
	}
	art := a.Images["POINTER.PIC"]
	sx := style * 5
	x, y, w, h := r[0], r[1], r[2], r[3]
	for _, d := range [][2]int{{x, y}, {x + w - 5, y}, {x, y + h - 5}, {x + w - 5, y + h - 5}} {
		_ = assetdata.OfficeBlit(art, frame, [4]int{sx, 100, 5, 5}, d[0], d[1], false)
	}
	pixel := func(dx, dy int) byte { return art[(100+dy)*320+sx+dx] }
	assetdata.OfficeFill(frame, x+2, y, w-4, 1, pixel(2, 0))
	assetdata.OfficeFill(frame, x+2, y+h-1, w-4, 1, pixel(2, 4))
	assetdata.OfficeFill(frame, x, y+2, 1, h-4, pixel(0, 2))
	assetdata.OfficeFill(frame, x+w-1, y+2, 1, h-4, pixel(4, 2))
	assetdata.OfficeFill(frame, x+2, y+1, w-4, 1, pixel(2, 1))
	assetdata.OfficeFill(frame, x+2, y+h-2, w-4, 1, pixel(2, 3))
	assetdata.OfficeFill(frame, x+1, y+2, 1, h-4, pixel(1, 2))
	assetdata.OfficeFill(frame, x+w-2, y+2, 1, h-4, pixel(3, 2))
	assetdata.OfficeFill(frame, x+2, y+2, w-4, h-4, art[105*320+sx+fill])
}

func drawPreflightOutline(frame []byte, r [4]int, colour byte) {
	x, y, w, h := r[0], r[1], r[2], r[3]
	assetdata.OfficeFill(frame, x, y, w, 1, colour)
	assetdata.OfficeFill(frame, x, y+h-1, w, 1, colour)
	assetdata.OfficeFill(frame, x, y, 1, h, colour)
	assetdata.OfficeFill(frame, x+w-1, y, 1, h, colour)
}
