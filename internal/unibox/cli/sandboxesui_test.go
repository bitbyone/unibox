package cli

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tobola/unibox/internal/unibox/api"
)

func TestASandboxIsPickedAndActedOnByAKey(t *testing.T) {
	t.Parallel()
	rows := []api.Sandbox{{Name: "dev", Env: "dev", State: "running"}, {Name: "web", Env: "web", State: "stopped"}}
	keys := func(ks ...string) sandboxesModel {
		t.Helper()
		var m tea.Model = sandboxesModel{rows: rows}
		for _, k := range ks {
			msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
			if k == "enter" {
				msg = tea.KeyMsg{Type: tea.KeyEnter}
			}
			m, _ = m.Update(msg)
		}
		return m.(sandboxesModel)
	}
	for _, c := range []struct {
		keys          []string
		kind, sandbox string
	}{
		{[]string{"enter"}, "tasks", "dev"},
		{[]string{"j", "s"}, "shell", "web"},
		{[]string{"p"}, "ports", "dev"},
		{[]string{"i"}, "inspect", "dev"},
		{[]string{"j", "t"}, "stop", "web"},
		{[]string{"c", "y"}, "recreate", "dev"},
		{[]string{"d", "n", "j", "d", "y"}, "remove", "web"},
	} {
		m := keys(c.keys...)
		if m.chosen == nil || m.chosen.kind != c.kind || m.chosen.sandbox != c.sandbox {
			t.Fatalf("%v: %+v", c.keys, m.chosen)
		}
	}
	if m := keys("c"); m.chosen != nil || !strings.Contains(m.View(), "Recreate sandbox dev") {
		t.Fatalf("%+v %q", m.chosen, m.View())
	}
	if m := keys("q"); m.chosen != nil || m.View() != "" {
		t.Fatal("q left something chosen or drawn")
	}
}

func TestSandboxAloneListsWithoutATerminal(t *testing.T) {
	t.Parallel()
	paths := cliFixture(t)
	if out, _, err := run(t, paths, "", "sandbox"); err != nil || !strings.Contains(out, "SANDBOX") {
		t.Fatalf("%v %q", err, out)
	}
}
