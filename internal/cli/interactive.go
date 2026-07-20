package cli

import (
	"fmt"
	"os"
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

func runInteractiveStart(client *toggl.Client, args []string) {
	fmt.Println("Fetching recent tasks and projects...")
	me, err := client.GetMe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching user: %v\n", err)
		os.Exit(1)
	}
	workspaceID := me.DefaultWorkspace

	endDate := time.Now()
	startDate := endDate.Add(-14 * 24 * time.Hour) // Last 14 days

	entries, err := client.GetTimeEntries(startDate, endDate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching time entries: %v\n", err)
		os.Exit(1)
	}

	projects, err := client.GetProjects(workspaceID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching projects: %v\n", err)
		os.Exit(1)
	}

	projMap := make(map[int64]string)
	for _, p := range projects {
		projMap[p.ID] = p.Name
	}

	var items []list.Item
	seen := make(map[string]bool)

	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		key := e.Description
		if e.ProjectID != nil {
			key += fmt.Sprintf("-%d", *e.ProjectID)
		}
		if !seen[key] {
			seen[key] = true
			var pName string
			if e.ProjectID != nil {
				pName = projMap[*e.ProjectID]
			}
			items = append(items, taskItem{
				desc:      e.Description,
				projectID: e.ProjectID,
				project:   pName,
			})
		}
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
