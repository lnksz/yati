#!/bin/sh
set -eu
cd "$(dirname "$0")/.."

image="$PWD/tmp/appimage/Yati-x86_64.AppImage"
work=$(mktemp -d "$PWD/tmp/appimage/smoke.XXXXXX")
trap 'rm -rf -- "$work"' EXIT
export TMPDIR="$work"
export APPIMAGE_EXTRACT_AND_RUN=1
export XDG_DATA_HOME="$work/data" XDG_CONFIG_HOME="$work/config"
export XDG_CACHE_HOME="$work/cache" XDG_STATE_HOME="$work/state"
mkdir -m700 "$work/runtime"
export XDG_RUNTIME_DIR="$work/runtime"
export YATI_CONFIG="$work/config.json" TOGGL_API_TOKEN='' YATI_PASS_PATH=''

"$image" --help
"$image" gui --install --autostart
desktop=io.github.kristof.yati.desktop
grep -Fx "Exec=\"$image\" gui" "$XDG_DATA_HOME/applications/$desktop"
grep -Fx "Exec=\"$image\" gui --background" "$XDG_CONFIG_HOME/autostart/$desktop"

export GDK_BACKEND=x11 GSK_RENDERER=cairo GTK_A11Y=none G_DEBUG=fatal-warnings
# shellcheck disable=SC2016
timeout 30s xvfb-run -a -e /dev/stderr -s '-screen 0 1280x1024x24 -nolisten tcp -extension GLX' \
    dbus-run-session -- sh -eu -c '
    "$1" &
    pid=$!
    trap "kill $pid 2>/dev/null || true" EXIT
    window=$(xdotool search --sync --onlyvisible --name "^Yati$")
    xdotool windowfocus --sync "$window" key ctrl+q
    wait "$pid"
' sh "$image"
