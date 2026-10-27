package cli

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tobola/unibox/internal/unibox"
)

// pressPort gives the ports list a key and what the switch it starts says.
func pressPort(m *portsModel, key string) {
	var k tea.KeyMsg
	switch key {
	case "down", "up", "enter", "esc", "backspace":
		k = tea.KeyMsg{Type: map[string]tea.KeyType{"down": tea.KeyDown, "up": tea.KeyUp, "enter": tea.KeyEnter, "esc": tea.KeyEsc, "backspace": tea.KeyBackspace}[key]}
	default:
		k = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	next, cmd := m.Update(k)
	*m = next.(portsModel)
	for cmd != nil {
		msg := cmd()
		if _, quit := msg.(tea.QuitMsg); quit {
			return
		}
		next, cmd = m.Update(msg)
		*m = next.(portsModel)
	}
}

func TestANamedPortIsSwitchedOnAtAnotherHostPortWithM(t *testing.T) {
	t.Parallel()
	calls := []string{}
	m := portsModel{sandbox: "dev", rows: []unibox.PortState{{Name: "spring", Spec: "8080:8080", Named: true}, {Name: "9090", Spec: "9090:9090"}}, switchPort: func(key string, on bool) ([]unibox.PortState, string, error) {
		calls = append(calls, key)
		return []unibox.PortState{{Name: "spring", Spec: "8888:8080", Named: true, On: true}}, "", nil
	}}
	// Only digits are typed; esc leaves it without a switch.
	for _, k := range []string{"m", "8", "x", "8", "esc"} {
		pressPort(&m, k)
	}
	if m.asking || len(calls) != 0 {
		t.Fatalf("%v %v", m.asking, calls)
	}
	for _, k := range []string{"m", "8", "8", "8", "7", "backspace"} {
		pressPort(&m, k)
	}
	if v := m.View(); !strings.Contains(v, "host port for spring: 888_") {
		t.Fatalf("%q", v)
	}
	for _, k := range []string{"8", "enter"} {
		pressPort(&m, k)
	}
	if len(calls) != 1 || calls[0] != "spring=8888" || m.done || !m.rows[0].On {
		t.Fatalf("%v %+v", calls, m)
	}
	// A port no file names has no name to map.
	m.rows = append(m.rows, unibox.PortState{Name: "9090", Spec: "9090:9090"})
	pressPort(&m, "j")
	pressPort(&m, "m")
	if m.asking || !strings.Contains(m.View(), "--map") {
		t.Fatalf("%q", m.View())
	}
}

func TestDSaysWhetherANamedPortIsOnAtStartForThePerson(t *testing.T) {
	t.Parallel()
	calls := []string{}
	m := portsModel{sandbox: "dev", rows: []unibox.PortState{{Name: "spring", Spec: "8080:8080", Named: true}, {Name: "9090", Spec: "9090:9090"}}, setDefault: func(key string, on bool) ([]unibox.PortState, string, error) {
		calls = append(calls, fmt.Sprintf("%s %v", key, on))
		return []unibox.PortState{{Name: "spring", Spec: "8080:8080", Named: true, Default: on, DefaultYours: true}, {Name: "9090", Spec: "9090:9090"}}, "port spring is on", nil
	}}
	pressPort(&m, "d")
	if len(calls) != 1 || calls[0] != "spring true" || !strings.Contains(m.View(), "start on (yours)") || !strings.Contains(m.View(), "port spring is on") {
		t.Fatalf("%v\n%s", calls, m.View())
	}
	pressPort(&m, "j")
	pressPort(&m, "d")
	if len(calls) != 1 || !strings.Contains(m.View(), "name it under ports:") {
		t.Fatalf("%v\n%s", calls, m.View())
	}
}

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
	press := func(key string) { t.Helper(); pressPort(&m, key) }
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
