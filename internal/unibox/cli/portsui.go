package cli

import (
	"cmp"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tobola/unibox/internal/unibox"
)

// portsModel is unibox port at a terminal: a running sandbox's ports, one
// to a line, each switched on or off where it stands. A switch is applied
// at once, as port on and off apply it, so the list always says what is
// published.
type portsModel struct {
	sandbox string
	rows    []unibox.PortState
	cursor  int
	busy    bool
	err     error
	done    bool
	// switchPort publishes or unpublishes a port and reads the ports anew.
	switchPort func(key string, on bool) ([]unibox.PortState, error)
}

type portsSwitched struct {
	rows []unibox.PortState
	err  error
}

func (m portsModel) Init() tea.Cmd { return nil }

func (m portsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case portsSwitched:
		m.busy, m.err = false, msg.err
		if msg.err == nil {
			m.rows = msg.rows
			m.cursor = min(m.cursor, max(len(m.rows)-1, 0))
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc", "enter":
			m.done = true
			return m, tea.Quit
		case "up", "k":
			m.cursor = max(m.cursor-1, 0)
		case "down", "j":
			m.cursor = min(m.cursor+1, max(len(m.rows)-1, 0))
		case "left", "h":
			return m.turn(func(bool) bool { return false })
		case "right", "l":
			return m.turn(func(bool) bool { return true })
		case " ", "x":
			return m.turn(func(on bool) bool { return !on })
		}
	}
	return m, nil
}

// turn switches the port under the cursor, unless it is so already or a
// switch is under way.
func (m portsModel) turn(to func(bool) bool) (tea.Model, tea.Cmd) {
	if m.busy || len(m.rows) == 0 {
		return m, nil
	}
	row := m.rows[m.cursor]
	on := to(row.On)
	if on == row.On {
		return m, nil
	}
	m.busy, m.err = true, nil
	return m, func() tea.Msg {
		rows, err := m.switchPort(row.Name, on)
		return portsSwitched{rows, err}
	}
}

var (
	portOn     = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	portOff    = lipgloss.NewStyle().Faint(true)
	portCursor = lipgloss.NewStyle().Bold(true)
	portHelp   = lipgloss.NewStyle().Faint(true)
	portErr    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
)

func (m portsModel) View() string {
	if m.done {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Ports of sandbox %s, until it next starts\n\n", m.sandbox)
	nameW, specW := 4, 7
	for _, r := range m.rows {
		nameW, specW = max(nameW, len(r.Name)), max(specW, len(r.Spec))
	}
	for i, r := range m.rows {
		mark, state := "  ", portOff.Render("off")
		if r.On {
			state = portOn.Render("on ")
		}
		name := fmt.Sprintf("%-*s", nameW, r.Name)
		if i == m.cursor {
			mark, name = "> ", portCursor.Render(name)
		}
		line := fmt.Sprintf("%s%s  %s  %-*s  %s  %s", mark, state, name, specW, r.Spec, cmp.Or(strings.Join(portURLs(r), " "), "-"), r.Description)
		b.WriteString(strings.TrimRight(line, " ") + "\n")
	}
	b.WriteString("\n")
	switch {
	case m.busy:
		b.WriteString("switching...\n")
	case m.err != nil:
		b.WriteString(portErr.Render(m.err.Error()) + "\n")
	}
	b.WriteString(portHelp.Render("↑/↓ j/k move · ←/h off · →/l on · space switch · enter/q done") + "\n")
	return b.String()
}

// portURLs are the addresses a port is published at, for a browser.
func portURLs(p unibox.PortState) []string {
	urls := []string{}
	for _, q := range p.Published {
		if strings.HasPrefix(q.Protocol, "tcp") {
			host := q.HostIP
			if strings.Contains(host, ":") {
				host = "[" + host + "]"
			}
			urls = append(urls, fmt.Sprintf("http://%s:%d", host, q.HostPort))
		}
	}
	return urls
}
