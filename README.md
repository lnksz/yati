# YATI (Yet Another Toggl Integration)

CLI for *my* [toggl](https://engineering.toggl.com/docs/track/) usage

- [ ] short and sweet for simple tasks
    - [ ] sane auth support
        - [ ] env var
        - [ ] keyfile
        - [ ] password provider
    - [ ] sane config: config file < env vars < cli-flags
    - [ ] end running task, if any
    - [ ] continue previous task
    - [ ] start task (stopping running one if any) with optional project
- [ ] interactive and glamourous for complex ones
    - [ ] start a task with real-time suggestions from prev. entries and projects
        - [ ] if previous task is selected, its project is also re-applied
    - [ ] list tasks of day (d) / work-week (w) / month (m) and iteractively select/filter (fzf?)

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
