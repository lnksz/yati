package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"yati/internal/toggl"
)

var docStyle = lipgloss.NewStyle().Margin(1, 2)

type taskItem struct {
	desc      string
	projectID *int64
	project   string
}

func (i taskItem) Title() string { return i.desc }
func (i taskItem) Description() string {
	if i.project != "" {
		return "Project: " + i.project
	}
	return "No project"
}
func (i taskItem) FilterValue() string { return i.desc + " " + i.project }

type startModel struct {
	list     list.Model
	client   *toggl.Client
	selected *taskItem
	quitting bool
	err      error
}

func (m startModel) Init() tea.Cmd {
	return nil
}

func (m startModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit

		case "enter":
			i, ok := m.list.SelectedItem().(taskItem)
			if ok {
				m.selected = &i
				m.quitting = true
				return m, tea.Quit
			}
		}
	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m startModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n", m.err)
	}
	if m.quitting {
		return ""
	}
	return docStyle.Render(m.list.View())
}

func interactiveHistoryStart(endDate time.Time) time.Time {
	return endDate.AddDate(0, -3, 0)
}

func runInteractiveStart(client *toggl.Client, args []string) {
	fmt.Println("Fetching recent tasks and projects...")
	me, err := client.GetMe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching user: %v\n", err)
		os.Exit(1)
	}
	workspaceID := me.DefaultWorkspace

	endDate := time.Now()
	startDate := interactiveHistoryStart(endDate)

	entries, err := client.GetTimeEntries(startDate, endDate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching time entries: %v\n", err)
		os.Exit(1)
	}

	projects, _, err := client.GetTargetsWithTimeEntries(workspaceID, entries)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching projects: %v\n", err)
		os.Exit(1)
	}

	projMap := make(map[int64]string)
	for _, p := range projects {
		projMap[p.ID] = p.Name
	}

	tasks := taskSuggestionsByLastUsed(entries, projMap)
	items := make([]list.Item, len(tasks))
	for i, task := range tasks {
		items[i] = task
	}

	if len(items) == 0 {
		fmt.Println("No recent tasks found.")
		return
	}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Select a task to start"

	m := startModel{
		list:   l,
		client: client,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}

	if startM, ok := finalModel.(startModel); ok && startM.selected != nil {
		item := startM.selected
		fmt.Printf("Starting task: %s\n", item.desc)
		_, err := client.StartTimeEntry(workspaceID, item.projectID, item.desc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error starting time entry: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Started successfully.")
	}
}

type taskSuggestion struct {
	item     taskItem
	lastUsed time.Time
}

func taskSuggestionsByLastUsed(entries []toggl.TimeEntry, projectNames map[int64]string) []taskItem {
	type taskKey struct {
		description string
		projectID   int64
		hasProject  bool
	}

	suggestions := make(map[taskKey]taskSuggestion)
	for _, entry := range entries {
		key := taskKey{description: entry.Description}
		var projectName string
		if entry.ProjectID != nil {
			key.projectID = *entry.ProjectID
			key.hasProject = true
			projectName = projectNames[*entry.ProjectID]
		}
		if existing, ok := suggestions[key]; ok && !entry.Start.After(existing.lastUsed) {
			continue
		}
		suggestions[key] = taskSuggestion{
			item: taskItem{
				desc:      entry.Description,
				projectID: entry.ProjectID,
				project:   projectName,
			},
			lastUsed: entry.Start,
		}
	}

	ordered := make([]taskSuggestion, 0, len(suggestions))
	for _, suggestion := range suggestions {
		ordered = append(ordered, suggestion)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if !ordered[i].lastUsed.Equal(ordered[j].lastUsed) {
			return ordered[i].lastUsed.After(ordered[j].lastUsed)
		}
		iDescription := strings.ToLower(ordered[i].item.desc)
		jDescription := strings.ToLower(ordered[j].item.desc)
		if iDescription != jDescription {
			return iDescription < jDescription
		}
		return strings.ToLower(ordered[i].item.project) < strings.ToLower(ordered[j].item.project)
	})

	items := make([]taskItem, len(ordered))
	for i, suggestion := range ordered {
		items[i] = suggestion.item
	}
	return items
}
