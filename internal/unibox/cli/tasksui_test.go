package cli

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tobola/unibox/internal/unibox/api"
)

func TestATaskIsPickedAndActedOnByAKey(t *testing.T) {
	t.Parallel()
	rows := []taskRow{
		{Task: api.Task{ID: "t1", Sandbox: "dev", Agent: "codex", State: "ready"}, agents: []string{"claude", "codex", "copilot"}},
		{Task: api.Task{ID: "t2", Alias: "fix", Sandbox: "dev", State: "ready"}, sessions: 1, agents: []string{"claude"}},
	}
	keys := func(m tasksModel, ks ...string) tasksModel {
		t.Helper()
		for _, k := range ks {
			var msg tea.KeyMsg
			switch k {
			case "enter":
				msg = tea.KeyMsg{Type: tea.KeyEnter}
			case "esc":
				msg = tea.KeyMsg{Type: tea.KeyEsc}
			default:
				msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
			}
			next, _ := m.Update(msg)
			m = next.(tasksModel)
		}
		return m
	}
	for _, c := range []struct {
		keys       []string
		kind, task string
		agent      string
	}{
		{[]string{"r"}, "repos", "t1", ""},
		{[]string{"j", "enter"}, "resume", "t2", ""},
		{[]string{"j", "j", "s"}, "shell", "t2", ""},
		// The picker starts at the task's agent.
		{[]string{"a", "l", "enter"}, "resume", "t1", "copilot"},
		{[]string{"a", "h", "h", "h", "enter"}, "resume", "t1", "claude"},
		{[]string{"a", "esc", "n"}, "rename", "t1", ""},
		{[]string{"d", "n", "d", "y"}, "remove", "t1", ""},
	} {
		m := keys(tasksModel{rows: rows}, c.keys...)
		if m.chosen == nil || m.chosen.kind != c.kind || m.chosen.task.ID != c.task || m.chosen.agent != c.agent {
			t.Fatalf("%v: %+v", c.keys, m.chosen)
		}
	}
	if m := keys(tasksModel{rows: rows}, "j", "q"); m.chosen != nil || m.View() != "" {
		t.Fatalf("q chose %+v, or left the list drawn", m.chosen)
	}
	m := keys(tasksModel{rows: rows}, "j")
	if v := m.View(); !strings.Contains(v, "fix (t2)") || !strings.Contains(v, "s shell") {
		t.Fatalf("%q", v)
	}
	if v := keys(m, "a").View(); !strings.Contains(v, "Resume fix with") {
		t.Fatalf("%q", v)
	}
}

func TestTaskAndResumeAloneListOrAskForATaskWithoutATerminal(t *testing.T) {
	t.Parallel()
	paths := cliFixture(t)
	if out, _, err := run(t, paths, "", "task"); err != nil || !strings.Contains(out, "TASK") {
		t.Fatalf("%v %q", err, out)
	}
	if _, _, err := run(t, paths, "", "resume"); err == nil || !strings.Contains(err.Error(), "name the task") {
		t.Fatalf("%v", err)
	}
}
