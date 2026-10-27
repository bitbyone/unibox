package cli

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"
)

// pickItem is one of the things a pick offers: its value, the line it
// shows, and whether it is picked.
type pickItem struct {
	value, label string
	picked       bool
}

// pickModel picks any of many: what is typed narrows them, fuzzily, space
// picks the one under the cursor and enter takes what is picked. Letters
// are always the filter's, so moving is by the arrows alone.
type pickModel struct {
	title         string
	items         []pickItem
	query         string
	cursor        int
	done, gaveUp  bool
	width, height int
}

func (m pickModel) Init() tea.Cmd { return nil }

func (m pickModel) visible() []int {
	texts := make([]string, len(m.items))
	for i, it := range m.items {
		texts[i] = it.label
	}
	return fuzzyFilter(m.query, texts)
}

func (m pickModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = size.Width, size.Height
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.Type {
	case tea.KeyCtrlC:
		m.gaveUp = true
		return m, tea.Quit
	case tea.KeyEsc:
		if m.query != "" {
			m.query, m.cursor = "", 0
			return m, nil
		}
		m.gaveUp = true
		return m, tea.Quit
	case tea.KeyEnter:
		m.done = true
		return m, tea.Quit
	case tea.KeyUp, tea.KeyShiftTab:
		m.cursor = max(m.cursor-1, 0)
	case tea.KeyDown:
		m.cursor = min(m.cursor+1, max(len(m.visible())-1, 0))
	case tea.KeySpace, tea.KeyTab:
		if v := m.visible(); m.cursor < len(v) {
			m.items[v[m.cursor]].picked = !m.items[v[m.cursor]].picked
		}
	case tea.KeyBackspace:
		if r := []rune(m.query); len(r) > 0 {
			m.query, m.cursor = string(r[:len(r)-1]), 0
		}
	case tea.KeyRunes:
		m.query, m.cursor = m.query+string(key.Runes), 0
	}
	return m, nil
}

func (m pickModel) View() string {
	if m.done || m.gaveUp {
		return ""
	}
	visible := m.visible()
	picked := 0
	for _, it := range m.items {
		if it.picked {
			picked++
		}
	}
	lines := []string{listTitle.Render(m.title), "", cursorMark() + m.query + "_  " + portHelp.Render(fmt.Sprintf("%d of %d · %d picked", len(visible), len(m.items), picked)), ""}
	rows := []string{}
	for k, i := range visible {
		mark, box := "  ", portHelp.Render("•")
		if k == m.cursor {
			mark = cursorMark()
		}
		label := m.items[i].label
		if m.items[i].picked {
			box = portOn.Render("✓")
		}
		if k == m.cursor {
			label = portCursor.Render(label)
		}
		rows = append(rows, mark+box+" "+label)
	}
	if len(rows) == 0 {
		rows = []string{portHelp.Render("  nothing matches")}
	}
	if m.height > 0 {
		room := max(m.height-len(lines)-3, 1)
		if room < len(rows) {
			first := min(max(m.cursor-room/2, 0), len(rows)-room)
			rows = rows[first : first+room]
		}
	}
	lines = append(append(lines, rows...), "", keyHelp("type", "filter", "↑/↓", "move", "space", "pick", "enter", "take them", "esc", "clear, then cancel"))
	if m.width > 0 {
		for i, l := range lines {
			lines[i] = ansi.Truncate(l, m.width-1, "…")
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

// pick asks for any of items at a terminal and gives the values picked,
// in the items' order; errCancelled when left. In accessible mode it is a
// numbered question, as every other is.
func (o *options) pick(cmd *cobra.Command, title string, items []pickItem) ([]string, error) {
	if o.accessible() {
		labels, preset := []string{}, []string{}
		for _, it := range items {
			labels = append(labels, it.label)
			if it.picked {
				preset = append(preset, it.label)
			}
		}
		chosen, err := o.askChoices(title, "", labels, preset, nil)
		if err != nil {
			return nil, err
		}
		out := []string{}
		for _, it := range items {
			if slices.Contains(chosen, it.label) {
				out = append(out, it.value)
			}
		}
		return out, nil
	}
	lipgloss.HasDarkBackground()
	res, err := tea.NewProgram(pickModel{title: title, items: items}, tea.WithInput(o.input), tea.WithOutput(o.log), tea.WithContext(cmd.Context())).Run()
	if err != nil {
		return nil, err
	}
	done := res.(pickModel)
	if done.gaveUp {
		return nil, errCancelled
	}
	out := []string{}
	for _, it := range done.items {
		if it.picked {
			out = append(out, it.value)
		}
	}
	return out, nil
}
