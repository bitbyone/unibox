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
	kind    string // tasks, shell, ports, inspect, stop, recreate, remove
	sandbox string
}

// sandboxesModel is unibox sandbox at a terminal: the sandboxes, one to a
// line, each acted on by a key. What discards something is asked again.
type sandboxesModel struct {
	rows    []api.Sandbox
	cursor  int
	confirm string
	chosen  *sandboxAction
	done    bool
}

func (m sandboxesModel) Init() tea.Cmd { return nil }

func (m sandboxesModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if key.String() == "ctrl+c" || len(m.rows) == 0 {
		m.done = true
		return m, tea.Quit
	}
	choose := func(kind string) (tea.Model, tea.Cmd) {
		m.chosen = &sandboxAction{kind: kind, sandbox: m.rows[m.cursor].Name}
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
	b.WriteString("Sandboxes\n\n")
	cols := [][]string{{"SANDBOX", "ENV", "AGENT", "STATE", "TASKS", "SESSIONS"}}
	for _, r := range m.rows {
		cols = append(cols, []string{r.Name, r.Env, cmp.Or(r.Agent, "-"), r.State, fmt.Sprint(len(r.Tasks)), fmt.Sprint(len(r.Runs))})
	}
	b.WriteString(drawTable(cols, m.cursor))
	b.WriteString("\n")
	name := m.rows[m.cursor].Name
	switch m.confirm {
	case "recreate":
		fmt.Fprintf(&b, "Recreate sandbox %s and discard what it holds outside its tasks? [y/N]\n", name)
	case "remove":
		fmt.Fprintf(&b, "Remove sandbox %s? [y/N]\n", name)
	default:
		b.WriteString(portHelp.Render("↑/↓ j/k move · enter tasks · s shell · p ports · i inspect · t stop · c recreate · d remove · q quit") + "\n")
	}
	return b.String()
}
