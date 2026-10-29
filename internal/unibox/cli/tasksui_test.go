package cli

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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
		{[]string{"t"}, "new", "t1", ""},
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

func TestATasksRepositoriesSayWhetherEachIsMountedAndFromWhere(t *testing.T) {
	t.Parallel()
	rows := []taskRow{
		{Task: api.Task{ID: "pricing", Sandbox: "dev", State: "ready", Repositories: []api.Repository{
			{Name: "web", Branch: "feat/pricing", HostPath: "/srv/checkouts/customers/acme/frontends/web", Mounted: true},
			{Name: "api", Branch: "feat/pricing", HostPath: "/srv/checkouts/api"},
			{Name: "adrs", URL: "https://github.com/acme/adrs.git", Context: true},
		}}},
		{Task: api.Task{ID: "fix", Sandbox: "dev", State: "ready", Repositories: []api.Repository{{Name: "spec", Branch: "main", URL: "https://github.com/acme/spec.git"}}}},
	}
	// Wide enough, a repository is a line of its own.
	v := ansi.Strip(tasksModel{rows: rows, width: 200}.View())
	for _, want := range []string{
		"web@feat/pricing  mount /srv/checkouts/customers/acme/frontends/web",
		"api@feat/pricing  clone /srv/checkouts/api",
		"adrs@default  clone github.com/acme/adrs",
		"spec@main  clone github.com/acme/spec",
	} {
		if !strings.Contains(v, want) {
			t.Fatalf("no %q in\n%s", want, v)
		}
	}
	// A task of more than a line is set apart from the next by a blank one.
	if !strings.Contains(v, "github.com/acme/adrs\n\n") {
		t.Fatalf("%s", v)
	}
	// Narrow, a path wraps in its column and no line is cut.
	narrow := tasksModel{rows: rows, width: 80}.View()
	for _, l := range strings.Split(narrow, "\n") {
		if w := lipgloss.Width(l); w > 79 {
			t.Fatalf("%d wide: %q", w, ansi.Strip(l))
		}
	}
	if s := ansi.Strip(narrow); !strings.Contains(s, "frontends/") || !strings.Contains(s, "web") {
		t.Fatalf("%s", s)
	}
}
