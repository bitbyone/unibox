package cli

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
)

func TestAListTakesAPartOfTheTerminalBetweenItsLeastAndMost(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ height, others, want int }{
		{0, 5, listMaxLines},    // not said: the most
		{100, 10, listMaxLines}, // tall: no more than the most
		{20, 10, 10},            // what is left beside the list's own
		{8, 10, listMinLines},   // short: the least, scrolled by the terminal
	} {
		if got := listRoom(c.height, c.others); got != c.want {
			t.Fatalf("%+v: %d", c, got)
		}
	}
	if f, e := rowWindow(5, 4, 10); f != 0 || e != 5 {
		t.Fatalf("%d %d", f, e)
	}
	if f, e := rowWindow(40, 39, 10); f != 30 || e != 40 {
		t.Fatalf("%d %d", f, e)
	}
	if f, e := rowWindow(40, 20, 10); f != 15 || e != 25 {
		t.Fatalf("%d %d", f, e)
	}
	// Rows of three lines a line apart: three fit nine lines, a row of
	// more than the room still shows.
	if f, e := blockWindow([]int{3, 3, 3, 3, 3}, 4, 1, 11); f != 2 || e != 5 {
		t.Fatalf("%d %d", f, e)
	}
	if f, e := blockWindow([]int{1, 9, 1}, 1, 0, 3); f != 1 || e != 2 {
		t.Fatalf("%d %d", f, e)
	}
}

// fits checks a list's view is no taller than the terminal and shows the
// row the cursor is on and how many more there are.
func fits(t *testing.T, view string, height int, cursor, more string) {
	t.Helper()
	plain := ansi.Strip(view)
	if n := strings.Count(plain, "\n"); n > height {
		t.Fatalf("%d lines for %d:\n%s", n, height, plain)
	}
	if !strings.Contains(plain, cursor) || !strings.Contains(plain, more) {
		t.Fatalf("no %q or %q in:\n%s", cursor, more, plain)
	}
}

func TestEveryListScrollsWithinTheTerminal(t *testing.T) {
	t.Parallel()
	size := tea.WindowSizeMsg{Width: 100, Height: 18}
	end := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}

	lm := listModel{title: "secrets", header: []string{"SERVICE"}}
	sm := sandboxesModel{}
	pm := portsModel{sandbox: "dev"}
	tm := tasksModel{}
	pick := taskPickModel{env: "dev", existing: map[string]bool{}}
	for i := range 40 {
		name := fmt.Sprintf("row%02d", i)
		lm.rows, lm.ids = append(lm.rows, []string{name}), append(lm.ids, name)
		pick.rows = append(pick.rows, taskRepo{name: name, repo: unibox.IndexedRepo{URL: "https://github.com/acme/" + name + ".git"}})
		sm.rows = append(sm.rows, api.Sandbox{Name: name, Env: "dev", State: "stopped"})
		pm.rows = append(pm.rows, unibox.PortState{Name: name, Spec: "80:80"})
		tm.rows = append(tm.rows, taskRow{Task: api.Task{ID: name, Sandbox: "dev", State: "ready", Repositories: []api.Repository{
			{Name: "web", Branch: "main", HostPath: "/srv/web", Mounted: true}, {Name: "api", Branch: "main", URL: "https://github.com/acme/api.git"},
		}}})
	}
	walk := func(m tea.Model) tea.Model {
		m, _ = m.Update(size)
		for range 39 {
			m, _ = m.Update(end)
		}
		return m
	}
	for name, m := range map[string]tea.Model{"task new": pick, "list": lm, "sandboxes": sm, "ports": pm, "tasks": tm} {
		top, _ := m.Update(size)
		fits(t, top.View(), size.Height, "row00", "more")
		if strings.Contains(ansi.Strip(top.View()), "row39") {
			t.Fatalf("%s: drawn whole", name)
		}
		fits(t, walk(m).View(), size.Height, "row39", "↑ ")
	}
	// The most a list shows, however tall the terminal.
	tall, _ := lm.Update(tea.WindowSizeMsg{Width: 100, Height: 200})
	if v := ansi.Strip(tall.View()); !strings.Contains(v, fmt.Sprintf("↓ %d more", 40-listMaxLines)) {
		t.Fatalf("%s", v)
	}
}
