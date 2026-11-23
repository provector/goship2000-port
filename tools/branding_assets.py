#!/usr/bin/env python3
"""Regenerate desktop icons with Pillow; see src/internal/graphics/README.md for Windows resources."""
from pathlib import Path
from PIL import Image, ImageDraw

ROOT = Path(__file__).resolve().parents[1]
GRAPHICS = ROOT / 'src/internal/graphics'
OUTPUT = ROOT / 'src/internal/branding'
SIZES = (16, 24, 32, 48, 64, 128, 256)


def desktop_icon(master, checker):
    image = Image.open(master).convert('RGBA')
    if checker:
        # Remove only edge-connected light neutral canvas. Enclosed highlights
        # and the dark outline of the aircraft survive even at pale ink values.
        mask = Image.new('L', image.size)
        mask.putdata([255 if min(r, g, b) >= 175 and max(r, g, b)-min(r, g, b) <= 22
                      else 0 for r, g, b, _ in image.getdata()])
        for corner in ((0, 0), (image.width-1, 0), (0, image.height-1),
                       (image.width-1, image.height-1)):
            if mask.getpixel(corner) == 255:
                ImageDraw.floodfill(mask, corner, 128)
        # The rotor, tail and sun enclose one triangle of the same checker.
        ImageDraw.floodfill(mask, (1220, 370), 128)
        image.putalpha(mask.point(lambda v: 0 if v == 128 else 255))
    image = image.crop(image.getchannel('A').getbbox())
    image.thumbnail((248, 248), Image.Resampling.LANCZOS)
    square = Image.new('RGBA', (256, 256))
    square.paste(image, ((256-image.width)//2, (256-image.height)//2))
    return square


def splash_art():
    image = Image.open(GRAPHICS / 'Provector Retro VGA Splash - bg black.png').convert('RGB')
    foreground = image.convert('L').point(lambda value: 255 if value > 12 else 0)
    bounds = foreground.getbbox()
    # The skid is a separate connected component in this master. Retain the
    # dark blue pixels in it, and leave the lettering entirely transparent.
    ImageDraw.floodfill(foreground, (600, 583), 128)
    mask = foreground.point(lambda value: 255 if value == 128 else 0)
    assert mask.getbbox() == (174, 559, 1371, 673)
    art = image.crop(bounds)
    art.paste((0, 0, 0), mask=art.convert('L').point(lambda value: 255 if value <= 7 else 0))
    art.save(OUTPUT / 'splash.png')
    shine = Image.new('RGBA', art.size, (182, 212, 252))
    shine.putalpha(mask.crop(bounds))
    shine.save(OUTPUT / 'skid.png')


if __name__ == '__main__':
    splash_art()
    for name, master in [('goship2000', 'icon'), ('goship2000-setup', 'setup')]:
        icon = desktop_icon(GRAPHICS / (master+'.png'), master == 'icon')
        icon.save(OUTPUT / (name+'.png'))
        icon.save(OUTPUT / (name+'.ico'), sizes=[(s, s) for s in SIZES])

    # windres emits a COFF resource object; Go links the platform-suffixed
    # object automatically into the corresponding Windows main package.
    import os
    import subprocess
    import tempfile
    compiler = os.environ.get('WINDRES', 'x86_64-w64-mingw32-windres')
    for name, package in [('goship2000', 'game'), ('goship2000-setup', 'cmd/setup')]:
        with tempfile.TemporaryDirectory() as directory:
            rc = Path(directory) / 'icon.rc'
            rc.write_text(f'1 ICON "{name}.ico"\n')
            subprocess.run([compiler, '--preprocessor=cpp', '-I', str(OUTPUT),
                            '-i', str(rc), '-O', 'coff', '-o',
                            str(ROOT / 'src' / package / 'icon_windows_amd64.syso')], check=True)
