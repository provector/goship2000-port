#!/bin/sh
# Optional for portable packages. Most file managers only resolve an absolute
# icon path or an installed theme name. Bind these two launchers in place.
set -eu
directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
for entry in 'GoShip 2000' 'GoShip 2000 Setup'; do
    case "$entry" in *Setup) id=org.goship2000.Setup;; *) id=org.goship2000.Game;; esac
    file="$directory/$entry.desktop"
    icon="$directory/share/icons/hicolor/256x256/apps/$id.png"
    test -r "$icon"
    tmp=$(mktemp "$file.XXXXXX")
    trap 'rm -f -- "$tmp"' EXIT
    while IFS= read -r line; do
        case "$line" in Icon=*) printf 'Icon=%s\n' "$icon";; *) printf '%s\n' "$line";; esac
    done < "$file" > "$tmp"
    chmod 755 "$tmp"
    mv -- "$tmp" "$file"
    trap - EXIT
done
