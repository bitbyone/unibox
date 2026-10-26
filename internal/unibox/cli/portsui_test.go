package cli

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tobola/unibox/internal/unibox"
)

func TestPortsAreSwitchedWhereTheyStandWithArrowsAndHL(t *testing.T) {
	t.Parallel()
	rows := []unibox.PortState{{Name: "api", Spec: "8080:8080", Named: true, On: true}, {Name: "vite", Spec: "5173:5173", Named: true}}
	calls := []string{}
	m := portsModel{sandbox: "dev", rows: append([]unibox.PortState(nil), rows...), switchPort: func(key string, on bool) ([]unibox.PortState, string, error) {
		calls = append(calls, key+map[bool]string{true: " on", false: " off"}[on])
		if key == "api" && on {
			return nil, "", errors.New("host port 8080 is taken")
		}
		for i := range rows {
			if rows[i].Name == key {
				rows[i].On = on
			}
		}
		return append([]unibox.PortState(nil), rows...), "ports of sandbox dev: " + key, nil
	}}
	press := func(key string) {
		t.Helper()
		var k tea.KeyMsg
		switch key {
		case "down", "up", "enter":
			k = tea.KeyMsg{Type: map[string]tea.KeyType{"down": tea.KeyDown, "up": tea.KeyUp, "enter": tea.KeyEnter}[key]}
		default:
			k = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
		}
		next, cmd := m.Update(k)
		m = next.(portsModel)
		for cmd != nil {
			msg := cmd()
			if _, quit := msg.(tea.QuitMsg); quit {
				return
			}
			next, cmd = m.Update(msg)
			m = next.(portsModel)
		}
	}
	// l on a port that is on asks nothing; h switches it off.
	press("l")
	press("h")
	press("j")
	press("l")
	press("j") // past the last stays on it
	if len(calls) != 2 || calls[0] != "api off" || calls[1] != "vite on" || m.cursor != 1 || m.rows[0].On || !m.rows[1].On {
		t.Fatalf("%v %d %+v", calls, m.cursor, m.rows)
	}
	if v := m.View(); !strings.Contains(v, "ports of sandbox dev: vite") || !strings.Contains(v, "> on   vite") || !strings.Contains(v, "  off  api") {
		t.Fatalf("%q", v)
	}
	// A switch that fails is said, and the list stays as it was.
	press("k")
	press(" ")
	if m.err == nil || m.rows[0].On || !strings.Contains(m.View(), "host port 8080 is taken") {
		t.Fatalf("%v %+v", m.err, m.rows)
	}
	press("enter")
	if !m.done || m.View() != "" {
		t.Fatal("enter did not end it")
	}
}
