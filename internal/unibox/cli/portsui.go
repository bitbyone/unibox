package cli

import (
	"cmp"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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
	note    string
	done    bool
	// hostPort is being typed while asking: the host port to publish the
	// named port under the cursor at, as port on name=HOST_PORT does.
	asking   bool
	hostPort string
	// switchPort publishes or unpublishes a port and reads the ports anew,
	// with the last line it would have said.
	switchPort func(key string, on bool) ([]unibox.PortState, string, error)
	// setDefault has a named port on or off at each start, for the person,
	// and reads the ports anew.
	setDefault func(key string, on bool) ([]unibox.PortState, string, error)
	// width and height are the terminal's: the keys are wrapped to the
	// one, the rows scroll within the other.
	width, height int
}

type portsSwitched struct {
	rows []unibox.PortState
	note string
	err  error
}

func (m portsModel) Init() tea.Cmd { return nil }

func (m portsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case portsSwitched:
		m.busy, m.err, m.note = false, msg.err, msg.note
		if msg.err == nil {
			m.rows = msg.rows
			m.cursor = min(m.cursor, max(len(m.rows)-1, 0))
		}
	case tea.KeyMsg:
		if m.asking {
			return m.ask(msg)
		}
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
		case "d":
			if m.busy || len(m.rows) == 0 || m.setDefault == nil {
				break
			}
			row := m.rows[m.cursor]
			if !row.Named {
				m.err, m.note = nil, "a port no file names is on only until the sandbox stops; name it under ports: in your config.yaml"
				break
			}
			m.busy, m.err, m.note = true, nil, ""
			return m, func() tea.Msg {
				rows, note, err := m.setDefault(row.Name, !row.Default)
				return portsSwitched{rows, note, err}
			}
		case "m":
			if m.busy || len(m.rows) == 0 {
				break
			}
			if !m.rows[m.cursor].Named {
				m.err, m.note = nil, "a port no file names moves with unibox port on --map"
				break
			}
			m.asking, m.hostPort, m.err, m.note = true, "", nil, ""
		}
	}
	return m, nil
}

// ask takes the host port typed for the port under the cursor, and on
// enter publishes the port there.
func (m portsModel) ask(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		m.asking = false
	case tea.KeyBackspace:
		if m.hostPort != "" {
			m.hostPort = m.hostPort[:len(m.hostPort)-1]
		}
	case tea.KeyEnter:
		m.asking = false
		if m.hostPort == "" {
			return m, nil
		}
		key := m.rows[m.cursor].Name + "=" + m.hostPort
		m.busy = true
		return m, func() tea.Msg {
			rows, note, err := m.switchPort(key, true)
			return portsSwitched{rows, note, err}
		}
	case tea.KeyRunes:
		for _, r := range msg.Runes {
			if r >= '0' && r <= '9' && len(m.hostPort) < 5 {
				m.hostPort += string(r)
			}
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
	m.busy, m.err, m.note = true, nil, ""
	return m, func() tea.Msg {
		rows, note, err := m.switchPort(row.Name, on)
		return portsSwitched{rows, note, err}
	}
}

func (m portsModel) View() string {
	if m.done {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", listTitle.Render("Ports of sandbox "+m.sandbox+", until it next starts"))
	nameW, specW, startW, urlW := 4, 7, 0, 0
	url := func(r unibox.PortState) string { return cmp.Or(strings.Join(portURLs(r), " "), "-") }
	for _, r := range m.rows {
		nameW, specW, startW, urlW = max(nameW, len(r.Name)), max(specW, len(r.Spec)), max(startW, len(atStart(r))), max(urlW, len(url(r)))
	}
	keys := helpLines(m.width, "↑/↓ j/k", "move", "←/h", "off", "→/l", "on", "space", "switch", "m", "on another host port", "d", "on/off at start", "enter/q", "done")
	// The title, the line of how many more, the blank one, the note, the
	// keys and the line the cursor is left on take the rest.
	first, end := rowWindow(len(m.rows), m.cursor, listRoom(m.height, len(keys)+7))
	for i := first; i < end; i++ {
		r := m.rows[i]
		mark, state := "  ", portOff.Render("off")
		if r.On {
			state = portOn.Render("on ")
		}
		name := fmt.Sprintf("%-*s", nameW, r.Name)
		if i == m.cursor {
			mark, name = cursorMark(), portCursor.Render(name)
		}
		line := fmt.Sprintf("%s%s  %s  %-*s  %-*s  %-*s  %s", mark, state, name, startW, atStart(r), specW, r.Spec, urlW, url(r), r.Description)
		b.WriteString(strings.TrimRight(line, " ") + "\n")
	}
	if more := moreLine(first, len(m.rows)-end); more != "" {
		b.WriteString(more + "\n")
	}
	b.WriteString("\n")
	if m.asking {
		fmt.Fprintf(&b, "host port for %s: %s_\n", m.rows[m.cursor].Name, m.hostPort)
		b.WriteString(strings.Join(helpLines(m.width, "enter", "publish it there", "esc", "cancel"), "\n") + "\n")
		return b.String()
	}
	switch {
	case m.busy:
		b.WriteString("switching...\n")
	case m.err != nil:
		b.WriteString(portErr.Render(m.err.Error()) + "\n")
	case m.note != "":
		b.WriteString(portHelp.Render(m.note) + "\n")
	default:
		b.WriteString("\n")
	}
	b.WriteString(strings.Join(keys, "\n") + "\n")
	return b.String()
}

// atStart says whether a port is published at each start, and when that is
// the person's say rather than the files'.
func atStart(r unibox.PortState) string {
	if !r.Named {
		return "-"
	}
	s := "start " + map[bool]string{true: "on", false: "off"}[r.Default]
	if r.DefaultYours {
		s += " (yours)"
	}
	return s
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
