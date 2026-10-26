package cli

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"
)

// listAction is a key of a list and what it does.
type listAction struct {
	key  string
	name string
	help string
	// confirm, when set, is asked before the action, %s the row's ID.
	confirm string
	// any is an action that needs no row, such as adding one.
	any bool
}

// listModel is a noun's list at a terminal: a table, one row under the
// cursor, each action bound to a key. It ends with the chosen action, which
// the command carries out as its own subcommand would, and draws the list
// again.
type listModel struct {
	title   string
	header  []string
	rows    [][]string
	ids     []string
	actions []listAction
	note    string
	cursor  int
	pending *listAction
	chosen  *listAction
	done    bool
	// width and height are the terminal's, once it says them.
	width, height int
}

func (m listModel) Init() tea.Cmd { return nil }

func (m listModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = size.Width, size.Height
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if m.pending != nil {
		a := m.pending
		m.pending = nil
		if key.String() == "y" {
			m.chosen = a
			return m, tea.Quit
		}
		return m, nil
	}
	switch key.String() {
	case "ctrl+c", "q", "esc":
		m.done = true
		return m, tea.Quit
	case "up", "k":
		m.cursor = max(m.cursor-1, 0)
		return m, nil
	case "down", "j":
		m.cursor = min(m.cursor+1, max(len(m.rows)-1, 0))
		return m, nil
	}
	for i, a := range m.actions {
		if a.key != key.String() || (!a.any && len(m.rows) == 0) {
			continue
		}
		if a.confirm != "" {
			m.pending = &m.actions[i]
			return m, nil
		}
		m.chosen = &m.actions[i]
		return m, tea.Quit
	}
	return m, nil
}

// id is the ID of the row under the cursor, "" with none.
func (m listModel) id() string {
	if m.cursor < len(m.ids) {
		return m.ids[m.cursor]
	}
	return ""
}

// View draws the list to fit the terminal: a line longer than it would
// wrap, and the list would no longer be drawn over in place, so a line is
// cut; rows beyond its height scroll with the cursor.
func (m listModel) View() string {
	if m.chosen != nil || m.done {
		return ""
	}
	var foot []string
	if m.pending != nil {
		foot = []string{fmt.Sprintf(m.pending.confirm+" [y/N]", m.id())}
	} else {
		if m.note != "" {
			foot = strings.Split(m.note, "\n")
		}
		help := []string{"↑/↓ j/k move"}
		for _, a := range m.actions {
			help = append(help, displayKey(a.key)+" "+a.help)
		}
		foot = append(foot, portHelp.Render(strings.Join(append(help, "q quit"), " · ")))
	}
	lines := []string{m.title, ""}
	if len(m.rows) == 0 {
		lines = append(lines, portHelp.Render("  nothing yet"))
	} else {
		table := strings.Split(strings.TrimSuffix(drawTable(append([][]string{m.header}, m.rows...), m.cursor), "\n"), "\n")
		rows := table[1:]
		if m.height > 0 {
			// The title, the header, the blank lines and the line the
			// cursor is left on take the rest.
			room := max(m.height-len(foot)-5, 1)
			if room < len(rows) {
				first := min(max(m.cursor-room/2, 0), len(rows)-room)
				rows = rows[first : first+room]
			}
		}
		lines = append(append(lines, table[0]), rows...)
	}
	lines = append(append(lines, ""), foot...)
	if m.width > 0 {
		for i, l := range lines {
			lines[i] = ansi.Truncate(l, m.width-1, "…")
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func displayKey(k string) string {
	if k == " " {
		return "space"
	}
	return k
}

// runList shows a list until an action is chosen or it is left; nil is
// left.
func (o *options) runList(cmd *cobra.Command, m listModel) (*listAction, string, error) {
	m.cursor = min(m.cursor, max(len(m.rows)-1, 0))
	lipgloss.HasDarkBackground()
	out, err := tea.NewProgram(m, tea.WithInput(o.input), tea.WithOutput(o.log), tea.WithContext(cmd.Context())).Run()
	if err != nil {
		return nil, "", err
	}
	done := out.(listModel)
	return done.chosen, done.id(), nil
}

// actList draws a list again after each action, at the row it was on,
// with what the action said, until it is left; then leave says where
// things stand, as the noun's ls would.
func (o *options) actList(cmd *cobra.Command, build func() (listModel, error), act func(action, id string) (string, error), leave func() error) error {
	note, at := "", ""
	for {
		m, err := build()
		if err != nil {
			return err
		}
		m.note = note
		if i := slices.Index(m.ids, at); i >= 0 {
			m.cursor = i
		}
		a, id, err := o.runList(cmd, m)
		if err != nil {
			return err
		}
		if a == nil {
			return leave()
		}
		at = id
		note, err = act(a.name, id)
		if err != nil {
			note = portErr.Render(err.Error())
		}
		if note == errListDone {
			return nil
		}
	}
}

// errListDone, said by an action, ends the list: the action took the
// terminal or printed what is to stay.
const errListDone = "\x00done"

// quietStep runs a command of unibox as a list's action, keeping what it
// says, which would land among the list's lines, and gives its last line.
func (o *options) quietStep(cmd *cobra.Command, args ...string) (string, error) {
	var said bytes.Buffer
	out, log, mlog := o.output, o.log, o.manager.Log
	o.output, o.log, o.manager.Log = &said, &said, &said
	yes := o.yes
	// What a list asks is asked before the step.
	o.yes = true
	err := o.runStep(cmd, args...)
	o.output, o.log, o.manager.Log, o.yes = out, log, mlog, yes
	return lastLine(said.String()), err
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
