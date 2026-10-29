package cli

import (
	"cmp"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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
	kind  string // resume, shell, repos, new, rename, remove
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
	// width and height are the terminal's: the keys and repositories are
	// wrapped to the one, the tasks scroll within the other.
	width, height int
}

func (m tasksModel) Init() tea.Cmd { return nil }

func (m tasksModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = size.Width, size.Height
		return m, nil
	}
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
		case "enter":
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
		case "enter":
			return choose("resume", "")
		case "r":
			return choose("repos", "")
		case "t":
			return choose("new", "")
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
			kind, where := repoWhere(repo)
			if repo.Mounted {
				kind = repoMount.Render(kind)
			} else {
				kind = portHelp.Render(kind)
			}
			repos = append(repos, repo.Name+"@"+cmp.Or(repo.Branch, "default")+"  "+kind+"\t"+where)
		}
		cols = append(cols, []string{name, r.Sandbox, cmp.Or(r.Agent, "-"), r.State, fmt.Sprint(r.sessions), strings.Join(repos, "\n")})
	}
	row := m.rows[m.cursor]
	var foot []string
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
		foot = append([]string{fmt.Sprintf("Resume %s with %s", cmp.Or(row.Alias, row.ID), strings.Join(agents, ""))}, helpLines(m.width, "←/→ h/l", "agent", "enter", "resume", "esc", "back")...)
	case tasksRemove:
		foot = []string{fmt.Sprintf("Remove task %s from sandbox %s? [y/N]", cmp.Or(row.Alias, row.ID), row.Sandbox)}
	default:
		foot = helpLines(m.width, "↑/↓ j/k", "move", "enter", "resume", "t", "new task", "a", "attach agent", "r", "add repos", "s", "shell", "n", "rename", "d", "remove", "q", "quit")
	}
	header, blocks, gap := tallTable(cols, m.cursor, m.width)
	heights := []int{}
	for _, bl := range blocks {
		heights = append(heights, len(bl))
	}
	// The title, the header, the line of how many more, the blank one,
	// the keys and the line the cursor is left on take the rest.
	first, end := blockWindow(heights, m.cursor, gap, listRoom(m.height, len(foot)+6))
	b.WriteString(header + "\n")
	for i := first; i < end; i++ {
		if i > first && gap > 0 {
			b.WriteString("\n")
		}
		b.WriteString(strings.Join(blocks[i], "\n") + "\n")
	}
	if more := moreLine(first, len(blocks)-end); more != "" {
		b.WriteString(more + "\n")
	}
	b.WriteString("\n" + strings.Join(foot, "\n") + "\n")
	return b.String()
}

// repoWhere says how a task holds a repository and from where: mounted
// from a directory of the host, which the agent writes into, or cloned in
// the sandbox, from the host's checkout or from the forge.
func repoWhere(r api.Repository) (kind, where string) {
	switch {
	case r.Mounted:
		return "mount", tildePath(r.HostPath)
	case r.HostPath != "":
		return "clone", tildePath(r.HostPath)
	}
	return "clone", cmp.Or(strings.TrimSuffix(strings.TrimPrefix(r.URL, "https://"), ".git"), "-")
}

// tallTable draws a table whose last column's cells may hold several
// lines, each wrapped to the room the terminal leaves it - width 0 is one
// that has not said its width: its header, and each row's lines. A line's
// part after a tab, a path, goes on a line of its own beneath it when the
// two do not fit, so a path is not broken where it starts. gap is the
// blank line that sets the rows apart once one is more than a line.
func tallTable(cols [][]string, cursor, width int) (header string, rows [][]string, gap int) {
	last := len(cols[0]) - 1
	widths := make([]int, last)
	for _, c := range cols {
		for i, v := range c[:last] {
			widths[i] = max(widths[i], lipgloss.Width(v))
		}
	}
	room := 0
	if width > 0 {
		// The cursor's mark and the gaps between the columns take the rest.
		room = width - 1 - 2 - 2*last
		for _, w := range widths {
			room -= w
		}
		// Too narrow to wrap into, the terminal cuts the line instead.
		if room < 20 {
			room = 0
		}
	}
	cells := make([][]string, len(cols))
	tall := false
	for n, c := range cols {
		for _, l := range strings.Split(c[last], "\n") {
			head, tail, split := strings.Cut(l, "\t")
			if !split || room == 0 || lipgloss.Width(head)+1+lipgloss.Width(tail) <= room {
				l = strings.Replace(l, "\t", " ", 1)
				if room > 0 && lipgloss.Width(l) > room {
					l = ansi.Wrap(l, room, "/ ")
				}
				cells[n] = append(cells[n], strings.Split(l, "\n")...)
				continue
			}
			cells[n] = append(cells[n], head)
			for _, t := range strings.Split(ansi.Wrap(tail, room-2, "/ "), "\n") {
				cells[n] = append(cells[n], "  "+t)
			}
		}
		tall = tall || n > 0 && len(cells[n]) > 1
	}
	rows = make([][]string, len(cols)-1)
	for n, c := range cols {
		for k, more := range cells[n] {
			parts := []string{}
			for i, v := range c[:last] {
				if k > 0 {
					v = ""
				}
				parts = append(parts, v+strings.Repeat(" ", widths[i]-lipgloss.Width(v)))
			}
			parts = append(parts, more)
			if n-1 == cursor {
				// Each cell bold on its own, as drawTable has it.
				for i, v := range parts {
					parts[i] = portCursor.Render(v)
				}
			}
			line := strings.TrimRight(strings.Join(parts, "  "), " ")
			switch {
			case n == 0:
				line = "  " + portHelp.Render(line)
			case n-1 == cursor && k == 0:
				line = cursorMark() + line
			default:
				line = "  " + line
			}
			if n == 0 {
				header = line
			} else {
				rows[n-1] = append(rows[n-1], line)
			}
		}
	}
	if tall {
		gap = 1
	}
	return header, rows, gap
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
