package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"yati/internal/toggl"
)

type listItem struct {
	desc     string
	duration time.Duration
	date     string
}

func (i listItem) Title() string { return i.desc }
func (i listItem) Description() string {
	return fmt.Sprintf("Date: %s | Duration: %s", i.date, i.duration.Round(time.Second))
}
func (i listItem) FilterValue() string { return i.desc + " " + i.date }

type listModel struct {
	list     list.Model
	quitting bool
	err      error
}

func (m listModel) Init() tea.Cmd {
	return nil
}

func (m listModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "ctrl+c", "q", "esc":
			m.quitting = true
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v-1)
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m listModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n", m.err)
	}
	if m.quitting {
		return ""
	}
	return docStyle.Render(m.list.View() + "\n" + formatDuration(visibleDurationSum(m.list)))
}

func visibleDurationSum(l list.Model) time.Duration {
	var total time.Duration
	for _, item := range l.VisibleItems() {
		if li, ok := item.(listItem); ok {
			total += li.duration
		}
	}
	return total
}

func formatDuration(d time.Duration) string {
	return "Total duration: " + d.Round(time.Second).String()
}

func runInteractiveList(client *toggl.Client, args []string) {
	fmt.Println("Fetching tasks...")

	period := "d"
	if len(args) > 0 {
		period = args[0]
	}

	now := time.Now()
	var startDate, endDate time.Time

	switch period {
	case "w":
		// This week
		offset := int(now.Weekday()) - 1
		if offset < 0 {
			offset = 6
		}
		startDate = time.Date(now.Year(), now.Month(), now.Day()-offset, 0, 0, 0, 0, now.Location())
		endDate = startDate.AddDate(0, 0, 7)
	case "m":
		// This month
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		endDate = startDate.AddDate(0, 1, 0)
	default:
		// Today
		startDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		endDate = startDate.AddDate(0, 0, 1)
	}

	entries, err := client.GetTimeEntries(startDate, endDate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching time entries: %v\n", err)
		os.Exit(1)
	}

	var items []list.Item

	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		dur := time.Duration(e.Duration) * time.Second
		if e.Duration < 0 {
			dur = time.Since(e.Start)
		}

		desc := e.Description
		if desc == "" {
			desc = "(no description)"
		}
		items = append(items, listItem{
			desc:     desc,
			duration: dur,
			date:     e.Start.Local().Format("2006-01-02 15:04"),
		})
	}

	if len(items) == 0 {
		fmt.Println("No tasks found for the given period.")
		return
	}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Time Entries"

	m := listModel{
		list: l,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}
