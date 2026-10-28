# Product

<!-- impeccable:product-schema 1 -->

## Platform

Native Linux desktop, with Ubuntu and GNOME as the first target. The existing CLI and TUI remain available.

## Stack

Go with GTK4 and libadwaita through a small C adapter. One executable contains the desktop, CLI, and TUI; desktop builds use the system GTK4 and libadwaita libraries.

## Users

People tracking their own work in Toggl who want to switch tasks quickly from their Linux desktop or terminal.

## Product Purpose

Make starting, stopping, and continuing a Toggl task quick enough to fit between pieces of work. Yati is a focused personal integration, rather than a full Toggl client.

## Operating Context

The resident, single-instance desktop window is reached from a global shortcut, launcher, or optional StatusNotifierItem tray icon. Closing the window hides it; stopping the timer is an explicit action. Existing CLI/TUI workflows can operate independently.

## Capabilities and Constraints

- Search recent task descriptions and project names locally, continue an entry with its original project, or start a new description with a selected project.
- Show the current timer and elapsed time, and provide an explicit Stop action or Continue last action.
- Edit the running task's title and project in place while preserving its timer.
- Filter history by today, the current week, or the current month and show the filtered total.
- Reuse existing credential sources, show cached history before refresh completes, and preserve unfinished input when reopening the window.
- Require a compatible desktop host for the optional tray. Desktop focus and placement remain controlled by the compositor.
- Keep the desktop interface native to GTK4/libadwaita, including system theme, accent, typography, icons, and controls.

## Evidence on Hand

The implementation is in `internal/gui/bridge.c` and `internal/gui/native.go`; desktop setup and controls are in `docs/gui.md`. `internal/gui/assets/yati.svg` is the application icon. Captures under `tmp/evidence/` use synthetic test data and are validation artifacts.

## Product Principles

- Put keyboard task switching within easy reach.
- Preserve descriptions and project associations when continuing work.
- Make the running timer and mutation results clear.
- Retain the terminal workflows alongside the desktop interface.
