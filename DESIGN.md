---
name: Yati
description: Fast keyboard-first Toggl task switching with native GNOME controls.
spacing:
  icon-label-gap: "6px"
  composer-gap: "8px"
  section-gap: "12px"
  content-gap: "16px"
  content-inset: "24px"
---

# Design System: Yati

## Overview

**Creative North Star: "Native GNOME task switching"**

Yati is a compact desktop utility built from GTK4 and libadwaita controls. Its visual identity follows the user's system theme, accent, font, and icon theme. The running timer, entry controls, and recent work carry the interface.

The window keeps a straightforward vertical hierarchy and uses native selected, suggested, destructive, disabled, and focus states. Quick entry and history share the same search field and row vocabulary.

**Key Characteristics:**
- Native system appearance.
- Compact, keyboard-first interaction.
- Clear timer and action hierarchy.
- Reusable boxed lists and two-line task rows.

## Colors

Color roles come from GTK4/libadwaita and the active system theme; the application defines no interface palette.

### Primary

The theme's suggested-action treatment identifies Start new and Connect. The theme's selection treatment identifies the selected result.

### Neutral

Window, list, text, secondary text, and separator colors remain theme-owned. Stop uses the native destructive-action treatment, and error text uses the native error treatment. The tray and application icon use the Toggl Track mark in its original pink while tracking and grayscale while stopped; those colors are scoped to the assets.

**The System Theme Rule.** Preserve native semantic color roles; do not turn colors sampled from a screenshot into fixed application tokens.

## Typography

Use the system typography supplied by GTK4/libadwaita. There is no custom font stack or application-defined type ramp.

The elapsed timer and connection title use the native secondary title role. Section names and history totals use the native heading role. Task titles and project subtitles use action-row typography. Dates, durations, keyboard hints, and status text use the native caption and secondary-text roles. Elapsed time uses the native numeric treatment.

**The Native Type Rule.** Express hierarchy with native typography roles and widget semantics rather than hard-coded font sizes or display faces.

## Layout

The resident application uses one vertically arranged window. Its default size is (640 × 690px), with a requested minimum of (420 × 470px). The header contains refresh, title, and application menu. The content contains current timer, centered page navigation, search, page content, and status.

Use the spacing tokens for icon-label groups, the horizontal entry composer, page sections, the main content stack, and horizontal content insets. Lists expand vertically and scroll; horizontal scrolling is disabled. Native controls negotiate available width. The current title allows two lines, result titles and subtitles allow one line, and the keyboard hint wraps. There are no application-defined responsive breakpoints.

## Elevation & Depth

Depth comes from native window decoration, boxed lists, cards, popovers, and toast overlays. GTK4/libadwaita owns their borders, shadows, and state transitions. The page stack uses a short native crossfade, recorded in the sidecar.

**The Native Surface Rule.** Reuse native surface treatments and overlays; do not copy theme shadows into an application shadow scale.

## Shapes

Buttons, fields, selection backgrounds, popovers, and boxed lists inherit their shape from the active theme. The implementation does not define an interface radius or border scale. The application icon is the circular Toggl Track mark, with matching geometry for its tracking and idle variants.

## Components

### Buttons

Use native buttons with symbolic icons where present. Start new and Connect are suggested actions; Stop is destructive. Continue last is a standard action shown when stopped. Sensitivity communicates unavailable actions and work in progress. Keep native hover and keyboard focus behavior.

### Inputs / Fields

The search entry serves both recent-task search and new descriptions. The project dropdown expands beside Start new and supports search. Connection uses a password entry with a visibility control and an optional save checkbox. Accessible names identify the search, project, and token fields.

### Lists / Current Timer

Boxed lists contain native action rows. Task descriptions lead, project or date/project metadata follows, and trailing date or duration plus a symbolic play icon indicates continuation. Rows support selection and single-click activation. The current timer uses the same row family, with a larger numeric elapsed label and explicit Stop or Continue last action. Preserve plain-text rendering of descriptions.

### Navigation / Feedback

A native stack switcher selects Quick entry or History. Connection is reached through the application menu. A spinner and secondary status line communicate work; errors appear in a revealed card with Retry; brief confirmations use native toasts. Retain keyboard navigation and shortcut hints alongside mouse controls.

Keyboard shortcuts are discoverable through the application menu and through **?** when search is empty, or **Ctrl+?** while typing. The help window uses native grouped action rows and shortcut labels. **Ctrl+S** stops the current timer while either the main window or shortcut help has focus.

## Do's and Don'ts

### Do:

- **Do** inherit system theme, accent, typography, symbolic icons, and widget states.
- **Do** reuse boxed lists, action rows, and semantic action styles.
- **Do** keep keyboard focus visible and native controls operable with the keyboard.
- **Do** preserve readable task descriptions and project context at narrow widths.

### Don't:

- **Don't** replace native controls with a custom web interface.
- **Don't** freeze screenshot colors, font metrics, radii, or shadows into application tokens.
- **Don't** use the application icon's colors as an inferred interface palette.
