package cli

import (
	"cmp"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tobola/unibox/internal/unibox/api"
)

// sandboxAction is what a person chose to do with a sandbox.
type sandboxAction struct {
	kind    string // tasks, shell, ports, inspect, resources, stop, recreate, remove
	sandbox string
	box     api.Sandbox
}

// sandboxesModel is unibox sandbox at a terminal: the sandboxes, one to a
// line, each acted on by a key. What discards something is asked again.
type sandboxesModel struct {
	rows    []api.Sandbox
	cursor  int
	confirm string
	chosen  *sandboxAction
	done    bool
	// width and height are the terminal's: the keys are wrapped to the
	// one, the rows scroll within the other.
	width, height int
	// context stands in the title's corner: where commands work.
	context string
}

func (m sandboxesModel) Init() tea.Cmd { return nil }

func (m sandboxesModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = size.Width, size.Height
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if key.String() == "ctrl+c" || len(m.rows) == 0 {
		m.done = true
		return m, tea.Quit
	}
	choose := func(kind string) (tea.Model, tea.Cmd) {
		m.chosen = &sandboxAction{kind: kind, sandbox: m.rows[m.cursor].Name, box: m.rows[m.cursor]}
		return m, tea.Quit
	}
	if m.confirm != "" {
		kind := m.confirm
		m.confirm = ""
		if key.String() == "y" {
			return choose(kind)
		}
		return m, nil
	}
	switch key.String() {
	case "q", "esc":
		m.done = true
		return m, tea.Quit
	case "up", "k":
		m.cursor = max(m.cursor-1, 0)
	case "down", "j":
		m.cursor = min(m.cursor+1, len(m.rows)-1)
	case "enter":
		return choose("tasks")
	case "s":
		return choose("shell")
	case "p":
		return choose("ports")
	case "i":
		return choose("inspect")
	case "r":
		return choose("resources")
	case "t":
		return choose("stop")
	case "c":
		m.confirm = "recreate"
	case "d", "x":
		m.confirm = "remove"
	}
	return m, nil
}

func (m sandboxesModel) View() string {
	if m.chosen != nil || m.done || len(m.rows) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(titleLine(listTitle.Render("Sandboxes"), m.context, m.width) + "\n\n")
	cols := [][]string{{"SANDBOX", "ENV", "AGENT", "STATE", "TASKS", "SESSIONS"}}
	for _, r := range m.rows {
		state := r.State
		if state == "running" {
			state = portOn.Render(state)
		}
		cols = append(cols, []string{r.Name, r.Env, cmp.Or(r.Agent, "-"), state, fmt.Sprint(len(r.Tasks)), fmt.Sprint(len(r.Runs))})
	}
	name := m.rows[m.cursor].Name
	var foot []string
	switch m.confirm {
	case "recreate":
		foot = []string{fmt.Sprintf("Recreate sandbox %s and discard what it holds outside its tasks? [y/N]", name)}
	case "remove":
		foot = []string{fmt.Sprintf("Remove sandbox %s? [y/N]", name)}
	default:
		foot = helpLines(m.width, "↑/↓ j/k", "move", "enter", "tasks", "s", "shell", "p", "ports", "i", "inspect", "r", "resources", "t", "stop", "c", "recreate", "d", "remove", "q", "quit")
	}
	table := strings.Split(strings.TrimSuffix(drawTable(cols, m.cursor), "\n"), "\n")
	// The title, the header, the lines of how many more, the blank one,
	// the keys and the line the cursor is left on take the rest.
	first, end := rowWindow(len(table)-1, m.cursor, listRoom(m.height, len(foot)+7))
	b.WriteString(table[0] + "\n" + strings.Join(scrolled(table[1+first:1+end], first, end, len(table)-1), "\n") + "\n")
	b.WriteString("\n" + strings.Join(foot, "\n") + "\n")
	return b.String()
}
