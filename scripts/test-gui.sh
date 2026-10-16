#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
mkdir -p tmp/gui-test tmp/evidence
export TMPDIR="$PWD/tmp/gui-test"
export YATI_GUI_INTEGRATION=1
export YATI_GUI_EVIDENCE="$PWD/tmp/evidence"
export GDK_BACKEND=x11
export GSK_RENDERER=cairo
export GTK_A11Y=none
# The test bus has no access to the host session. Disable policy mediation on
# this private bus so the same harness also runs in restricted build sandboxes.
cat > tmp/gui-test/dbus.conf <<EOF
<busconfig>
  <type>session</type>
  <listen>unix:tmpdir=$TMPDIR</listen>
  <auth>EXTERNAL</auth>
  <apparmor mode="disabled"/>
  <policy context="default">
    <allow send_destination="*" eavesdrop="true"/>
    <allow eavesdrop="true"/>
    <allow own="*"/>
  </policy>
</busconfig>
EOF
exec xvfb-run -a -s '-screen 0 1280x1024x24 -nolisten tcp' dbus-run-session --config-file="$PWD/tmp/gui-test/dbus.conf" -- "${GO:-go}" test -race ./internal/gui -run '^Test(DesktopInteraction|DesktopDetachment|TrayProtocol|ShortcutPortal)$' -v -count=1
