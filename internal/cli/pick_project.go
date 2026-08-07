package cli

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"yati/internal/toggl"
)

type projectItem struct {
	id   int64
	name string
}

func (i projectItem) Title() string       { return i.name }
func (i projectItem) Description() string { return "" }
func (i projectItem) FilterValue() string { return i.name }

type projectPickerModel struct {
	list     list.Model
	selected *projectItem
	quitting bool
	err      error
}

func (m projectPickerModel) Init() tea.Cmd {
	return nil
}

func (m projectPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit

		case "enter":
			i, ok := m.list.SelectedItem().(projectItem)
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

func (m projectPickerModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n", m.err)
	}
	if m.quitting {
		return ""
	}
	return docStyle.Render(m.list.View())
}

// pickProject shows an interactive picker of the workspace's projects and
// returns the selected project. cancelled is true when the user aborts the
// picker instead of choosing a project.
func pickProject(client *toggl.Client, workspaceID int64) (pid *int64, name string, cancelled bool, err error) {
	projects, err := client.GetProjects(workspaceID)
	if err != nil {
		return nil, "", false, err
	}
	if len(projects) == 0 {
		fmt.Println("No projects found, starting without a project.")
		return nil, "", false, nil
	}

	items := make([]list.Item, len(projects))
	for i, p := range projects {
		items[i] = projectItem{id: p.ID, name: p.Name}
	}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Select a project"

	m := projectPickerModel{list: l}
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		return nil, "", false, err
	}

	if pm, ok := finalModel.(projectPickerModel); ok && pm.selected != nil {
		sel := *pm.selected
		return &sel.id, sel.name, false, nil
	}
	return nil, "", true, nil
}
