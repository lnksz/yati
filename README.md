# YATI (Yet Another Toggl Integration)

CLI for *my* [toggl](https://engineering.toggl.com/docs/track/) usage

A native Linux desktop window and tray icon are also available with `yati gui`.
Bind `yati gui --toggle` to a global desktop shortcut for quick entry from
anywhere. See [desktop setup, controls, and build instructions](docs/gui.md).

- [x] short and sweet for simple tasks
    - [x] sane auth support
        - [x] env var
        - [x] keyfile
        - [x] password provider (`pass`)
    - [x] sane config: config file < env vars < cli-flags
    - [x] end running task, if any
    - [x] continue previous task
    - [x] start task (stopping running one if any) with optional project
- [x] interactive and glamourous for complex ones
    - [x] start a task with real-time suggestions from prev. entries and projects
        - [x] if previous task is selected, its project is also re-applied
    - [x] list tasks of day (d) / work-week (w) / month (m) and iteractively select/filter (fzf?)
- [x] cache slow-changing listings (projects, customers)

**NOTE**: From the get-go I only target whatever I need from toggl, this won't
be a *full-client* of any sort

## Fish Shell Completions

Install completions for [Fish shell](https://fishshell.com/):

```fish
yati completion fish > ~/.config/fish/completions/yati.fish
```

Or use it directly in your session:

```fish
yati completion fish | source
```
