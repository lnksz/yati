# Yati desktop

The desktop interface uses Go, GTK4, and libadwaita. The same executable still
contains the CLI and TUI. It runs one desktop instance per session, with a
quick-entry window and a StatusNotifierItem/DBusMenu tray icon.

## Build and run

On Ubuntu 24.04 or newer:

```sh
sudo apt install build-essential pkg-config libgtk-4-dev libadwaita-1-dev
go build -o yati .
./yati gui
```

`make release` builds a stripped executable with build paths removed.
`make install` builds that release and installs it with executable permissions
to the sibling `bin/` directory of `$XDG_DATA_HOME`. With the default data home
`~/.local/share`, the destination is `~/.local/bin/yati`.

Use `./yati gui -d` (or `--detach`) to open Yati and return control to the
terminal. It starts a separate process/session and waits for desktop startup
before reporting success. Startup failures are reported to the terminal;
diagnostics go to `$XDG_STATE_HOME/yati/gui.log` (normally
`~/.local/state/yati/gui.log`), readable only by you.

Detachment and window visibility are independent: `yati gui -d --background`
starts hidden in the tray. An existing instance is reused. To move an already
running foreground instance into detached mode, quit it first with
`yati gui --quit`, then launch with `-d`.

The target desktop needs GTK4 and libadwaita runtime libraries. No Go toolchain,
webview, browser, or separate application daemon is needed at runtime. Other
modern Linux desktops can run the same GTK interface; the tray needs a
StatusNotifierItem-compatible host. Ubuntu's AppIndicators extension provides
this on GNOME. Plain GNOME installations may need that extension enabled.

For distributing one executable across distributions, build on the oldest
distribution you intend to support and verify on each target. A binary built on
a newer system can require newer glibc or desktop-library symbols. It is one
executable per CPU architecture, with compatible system libraries required.

For a CLI/TUI-only binary, build with `CGO_ENABLED=0 go build -o yati .` or
`go build -tags nogui -o yati .`. Desktop help remains available in that build,
but opening its GUI reports that desktop support is unavailable.

## Activate from anywhere

Bind an absolute path to the executable in GNOME Settings → Keyboard → View and
Customize Shortcuts → Custom Shortcuts:

```text
/path/to/yati gui
```

`yati gui` always shows/focuses the window. `--toggle` hides it if already visible.
Repeated invocations forward to the existing instance, preserving an unfinished
description and project. The keyboard shortcut and tray activation reach the
same window. On Wayland, the compositor controls focus and placement.

The application menu also offers **Set global shortcut…**, using the desktop's
GlobalShortcuts portal. Assignment happens in the desktop's permission dialog.
Once configured, the resident application recreates its shortcut session on
startup. If a portal is unavailable, the custom-command shortcut above works
without it. Run Yati at login for an app-registered shortcut to remain available.

## Install a launcher and optional login startup

Move the binary to its permanent location first. These commands record that
location and only write per-user desktop files:

```sh
yati gui --install
yati gui --install --autostart
```

The second command enables login startup with the window hidden. The application
menu's **Start at login** checkbox can enable or disable it later. No system
service is installed.

Other launch options:

```sh
yati gui --background  # start hidden, with the tray icon
yati gui -d           # open the window and release the terminal
yati gui --no-tray     # run the window without exporting a tray icon
yati gui --quit        # quit the resident desktop instance
```

Closing the window or pressing Escape hides it. **Quit Yati** exits the desktop
application. Neither action stops the Toggl timer; use **Stop** explicitly.

## User systemd service

The repository includes [contrib/systemd/yati.service](../contrib/systemd/yati.service).
It starts Yati hidden at graphical login, supervises the resident process, and
stops it when the graphical session ends. It uses the application's D-Bus name
for readiness. The service runs without `-d` so systemd retains supervision.

To use the unit with its default executable location:

```sh
install -Dm755 ./yati ~/.local/bin/yati
~/.local/bin/yati gui --install
install -Dm644 contrib/systemd/yati.service ~/.config/systemd/user/yati.service
systemctl --user daemon-reload
~/.local/bin/yati gui --quit
systemctl --user enable --now yati.service
```

Choose the systemd service or the app's **Start at login** option as your login
startup method. When using systemd, turn off **Start at login** to avoid two
launchers competing for the same instance. Existing config-file and `pass`
credentials work; shell-only environment variables are not automatically copied
into the user service manager.

```sh
systemctl --user status yati.service
journalctl --user -u yati.service -f
systemctl --user disable --now yati.service
```

Closing the window keeps the service running. **Quit Yati** exits it normally;
`Restart=on-failure` restarts crashes. To start it again in the same session, use
`systemctl --user start yati.service`. Stopping the service leaves the Toggl timer
running. Desktop sessions without `graphical-session.target` can use the app's
login-startup option instead.

## Keyboard and mouse controls

Choose **Keyboard shortcuts** in the app menu, or press **?** with an empty search,
to open the shortcuts window. **Ctrl+?** opens it while typing. Question marks in
an existing description stay ordinary text. **Ctrl+S stops the current timer**,
including while the shortcuts window is open.

| Control | Action |
| --- | --- |
| Type in the search field | Search recent descriptions and project names locally |
| Up / Down | Select another result |
| Enter, or click a result | Continue that description with its original project |
| Ctrl+Enter / Start new | Start the typed description with the selected project |
| Ctrl+S / Stop | Stop the displayed running entry |
| Ctrl+Shift+C / Continue last | Continue the last stopped entry with its original project |
| Ctrl+L | Focus search |
| Ctrl+R | Refresh from Toggl |
| Ctrl+1 / Ctrl+2 | Quick entry / History |
| ? / Ctrl+? | Show keyboard shortcuts (? when not typing a description) |
| F10 | Open the app menu |
| Escape | Hide the window, or leave Connection |
| Ctrl+Q | Quit the desktop application |

Open the project picker and type to filter project names as you go. Matches are
case-insensitive and can occur anywhere in the name. Use Down/Up and Enter, or
click a match, to select it.

To edit the running task, click the edit icon beside **Stop**. The **Edit running
task** window starts with its current title and project. Edit the title, choose an
active project (or **No project**), then click **Save**. Pressing Enter in the title
field also saves. Both changes update the existing entry while keeping its start
time and elapsed timer. **Cancel** or Escape discards the edits. The project picker
beside **Start new** still applies to new entries.

History includes today, the current Monday–Sunday
week, and the current calendar month, using local calendar boundaries. Its total
reflects the current search, matching the TUI's filtering behavior. Recent-task
results show the first 100 matches; type more to narrow a longer history.

The tray menu shows the running entry and elapsed time, plus Open Yati, Stop,
Continue last, and Quit. Desktop hosts control whether a primary click opens the
window or the menu. The tray uses the Toggl Track mark in grayscale while stopped
and its original pink while tracking. Embedded icon sizes keep it independent of
the installed icon theme. An installed desktop launcher uses the same artwork
and follows the last known timer state while Yati runs; desktop shells may cache
launcher updates briefly. Restart Yati after replacing its executable to load
new artwork. The icon source and license are in
[assets/NOTICE.md](../internal/gui/assets/NOTICE.md).

## Connection and caching

Existing `TOGGL_API_TOKEN`, `YATI_CONFIG`, config-file, and `pass` credentials work.
The GUI accepts `--token` and `--pass-path` after `gui`. Connection setup also
accepts a token in the window. Saving it is optional; saved config files use
owner-only permissions and preserve unrelated settings.

Recent history is cached per account under `$XDG_CACHE_HOME/yati` (normally
`~/.cache/yati`), without API tokens. Cached descriptions appear before network
refresh completes. Reopening the window refreshes data only when at least five
minutes old. There is no periodic network polling; elapsed time and searches are
computed locally. Ctrl+R explicitly requests a refresh.

Quota errors respect Toggl's reset/retry headers. Timed-out writes are never
automatically retried. Before stopping or switching, Yati rechecks the current
entry. If another client changed it, Yati displays it for review before making a
write. A replacement starts only after the previous entry's stop is confirmed.
If the new start then fails, the old entry remains stopped and the error is shown.

CLI/TUI invocations can still operate independently. The GUI reconciles their
changes on refresh or before a mutation. Local cache failures leave the current
session usable and display a warning.

## Validation

```sh
go test -race ./...
go vet ./...
sh scripts/test-gui.sh
```

The last command needs `xvfb`, `xauth`, `xdotool`, ImageMagick's `import`, and
`dbus-run-session`. It launches an isolated display and session bus with a
synthetic backend, and never contacts Toggl. It verifies keyboard entry,
continuation with project preservation, stop, tray actions, history, hiding,
single-instance activation, and remote quit. Screenshots are written under
`tmp/evidence`; all harness temporaries stay under the repository's `tmp/`.

After installing the launcher, an optional check against the current desktop's
real portal verifies application identity and session creation:

```sh
YATI_GUI_HOST_PORTAL=1 go test -race ./internal/gui -run '^TestHostShortcutPortalIdentity$' -v
```

This check closes its session immediately and does not assign a shortcut. The
isolated portal tests also enforce registration on the same D-Bus connection
before session creation, and cover portals without the host registry.
