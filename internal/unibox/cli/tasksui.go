package cli

import (
	"cmp"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tobola/unibox/internal/unibox/api"
)

// taskRow is a task as the list shows it, with what its sandbox offers.
type taskRow struct {
	api.Task
	sessions int
	agents   []string
}

// taskAction is what a person chose to do with a task; the list ends for
// it, as resuming a task takes the terminal.
type taskAction struct {
	kind  string // resume, shell, rename, remove
	task  api.Task
	agent string
}

const (
	tasksList = iota
	tasksAgent
	tasksRemove
)

// tasksModel is unibox task and unibox resume at a terminal: the tasks,
// one to a line, each acted on by a key.
type tasksModel struct {
	rows   []taskRow
	cursor int
	mode   int
	agent  int
	chosen *taskAction
	done   bool
}

func (m tasksModel) Init() tea.Cmd { return nil }

func (m tasksModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok || len(m.rows) == 0 {
		if ok {
			return m, tea.Quit
		}
		return m, nil
	}
	row := m.rows[m.cursor]
	choose := func(kind, agent string) (tea.Model, tea.Cmd) {
		m.chosen = &taskAction{kind: kind, task: row.Task, agent: agent}
		return m, tea.Quit
	}
	if key.String() == "ctrl+c" {
		m.done = true
		return m, tea.Quit
	}
	switch m.mode {
	case tasksAgent:
		switch key.String() {
		case "left", "h":
			m.agent = max(m.agent-1, 0)
		case "right", "l":
			m.agent = min(m.agent+1, len(row.agents)-1)
		case "enter", "r":
			return choose("resume", row.agents[m.agent])
		case "esc", "q":
			m.mode = tasksList
		}
	case tasksRemove:
		if key.String() == "y" {
			return choose("remove", "")
		}
		m.mode = tasksList
	default:
		switch key.String() {
		case "q", "esc":
			m.done = true
			return m, tea.Quit
		case "up", "k":
			m.cursor = max(m.cursor-1, 0)
		case "down", "j":
			m.cursor = min(m.cursor+1, len(m.rows)-1)
		case "enter", "r":
			return choose("resume", "")
		case "s":
			return choose("shell", "")
		case "n":
			return choose("rename", "")
		case "d", "x":
			m.mode = tasksRemove
		case "a":
			if len(row.agents) > 0 {
				m.mode, m.agent = tasksAgent, 0
				for i, a := range row.agents {
					if a == row.Agent {
						m.agent = i
					}
				}
			}
		}
	}
	return m, nil
}

var taskPicked = lipgloss.NewStyle().Reverse(true)

func (m tasksModel) View() string {
	if m.chosen != nil || m.done {
		return ""
	}
	if len(m.rows) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(listTitle.Render("Tasks") + "\n\n")
	cols := [][]string{{"TASK", "SANDBOX", "AGENT", "STATE", "SESSIONS", "REPOSITORIES"}}
	for _, r := range m.rows {
		name := r.ID
		if r.Alias != "" {
			name = r.Alias + " (" + r.ID + ")"
		}
		repos := []string{}
		for _, repo := range r.Repositories {
			repos = append(repos, repo.Name+"@"+repo.Branch)
		}
		cols = append(cols, []string{name, r.Sandbox, cmp.Or(r.Agent, "-"), r.State, fmt.Sprint(r.sessions), strings.Join(repos, ", ")})
	}
	b.WriteString(drawTable(cols, m.cursor))
	b.WriteString("\n")
	row := m.rows[m.cursor]
	switch m.mode {
	case tasksAgent:
		agents := []string{}
		for i, a := range row.agents {
			if i == m.agent {
				a = taskPicked.Render(" " + a + " ")
			} else {
				a = " " + a + " "
			}
			agents = append(agents, a)
		}
		fmt.Fprintf(&b, "Resume %s with %s\n", cmp.Or(row.Alias, row.ID), strings.Join(agents, ""))
		b.WriteString(keyHelp("←/→ h/l", "agent", "enter", "resume", "esc", "back") + "\n")
	case tasksRemove:
		fmt.Fprintf(&b, "Remove task %s from sandbox %s? [y/N]\n", cmp.Or(row.Alias, row.ID), row.Sandbox)
	default:
		b.WriteString(keyHelp("↑/↓ j/k", "move", "enter/r", "resume", "a", "another agent", "s", "shell", "n", "rename", "d", "remove", "q", "quit") + "\n")
	}
	return b.String()
}

// drawTable draws a list's columns, its first row the header, the row
// under the cursor marked.
func drawTable(cols [][]string, cursor int) string {
	widths := make([]int, len(cols[0]))
	for _, c := range cols {
		for i, v := range c {
			widths[i] = max(widths[i], lipgloss.Width(v))
		}
	}
	var b strings.Builder
	for n, c := range cols {
		cells := []string{}
		for i, v := range c {
			cells = append(cells, v+strings.Repeat(" ", widths[i]-lipgloss.Width(v)))
		}
		if n-1 == cursor {
			// Each cell bold on its own: a cell of its own colour ends
			// its style, and with it a bold the whole line would share.
			for i, v := range cells {
				cells[i] = portCursor.Render(v)
			}
		}
		line := strings.TrimRight(strings.Join(cells, "  "), " ")
		switch {
		case n == 0:
			line = "  " + portHelp.Render(line)
		case n-1 == cursor:
			line = cursorMark() + line
		default:
			line = "  " + line
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}
