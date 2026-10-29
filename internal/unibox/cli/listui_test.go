package cli

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func listKeys(m listModel, ks ...string) listModel {
	var tm tea.Model = m
	for _, k := range ks {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case " ":
			msg = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
		}
		tm, _ = tm.Update(msg)
	}
	return tm.(listModel)
}

func TestAListActsOnTheRowUnderTheCursor(t *testing.T) {
	t.Parallel()
	m := listModel{title: "things", header: []string{"NAME"}, rows: [][]string{{"a"}, {"b"}}, ids: []string{"a", "b"}, actions: []listAction{
		{key: "enter", name: "open", help: "open"},
		{key: " ", name: "pick", help: "pick"},
		{key: "n", name: "new", help: "new", any: true},
		{key: "d", name: "rm", help: "remove", confirm: "remove %s?"},
	}}
	for _, c := range []struct {
		keys     []string
		name, id string
	}{
		{[]string{"enter"}, "open", "a"},
		{[]string{"j", " "}, "pick", "b"},
		{[]string{"j", "j", "k", "n"}, "new", "a"},
		{[]string{"d", "n", "j", "d", "y"}, "rm", "b"},
	} {
		got := listKeys(m, c.keys...)
		if got.chosen == nil || got.chosen.name != c.name || got.id() != c.id {
			t.Fatalf("%q: %+v %s", c.keys, got.chosen, got.id())
		}
		if got.View() != "" {
			t.Fatalf("%q left the list drawn", c.keys)
		}
	}
	if got := listKeys(m, "j", "d"); got.chosen != nil || !strings.Contains(got.View(), "remove b? [y/N]") {
		t.Fatalf("%q", got.View())
	}
	if got := listKeys(m, "q"); got.chosen != nil || !got.done || got.View() != "" {
		t.Fatal("q chose something or left the list drawn")
	}
	empty := m
	empty.rows, empty.ids = nil, nil
	if got := listKeys(empty, "enter"); got.chosen != nil {
		t.Fatal("an action needing a row was chosen without one")
	}
	if got := listKeys(empty, "n"); got.chosen == nil {
		t.Fatal("an action needing no row was not chosen")
	}
	long := m
	long.rows, long.ids = nil, nil
	for i := range 40 {
		long.rows = append(long.rows, []string{fmt.Sprintf("row%02d %s", i, strings.Repeat("x", 200))})
		long.ids = append(long.ids, fmt.Sprint(i))
	}
	var tm tea.Model = long
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 60, Height: 15})
	long = listKeys(tm.(listModel), slices.Repeat([]string{"j"}, 30)...)
	v := long.View()
	if n := strings.Count(v, "\n"); n > 15 || !strings.Contains(v, "row30") || strings.Contains(v, "row00") {
		t.Fatalf("%d lines: %q", n, v)
	}
	for _, l := range strings.Split(v, "\n") {
		if lipgloss.Width(l) >= 60 {
			t.Fatalf("a line of %d: %q", lipgloss.Width(l), l)
		}
	}
	m.note = "first\nsecond"
	if v := m.View(); !strings.Contains(v, "second") || !strings.Contains(v, "space pick") {
		t.Fatalf("%q", v)
	}
}

// A list's action runs a command as given now, not with the flags an
// earlier one had, and keeps what it says out of the list.
func TestAListsStepsKeepTheirFlagsApart(t *testing.T) {
	t.Parallel()
	paths := cliFixture(t)
	var out, log bytes.Buffer
	o := &options{paths: paths, input: strings.NewReader(""), output: &out, log: &log}
	root := o.root()
	root.SetArgs([]string{"var", "ls"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if said, err := o.quietStep(root, "var", "set", "region", "us", "-g"); err != nil || !strings.Contains(said, "region") {
		t.Fatalf("%v %q", err, said)
	}
	if _, err := o.quietStep(root, "var", "set", "user", "toby", "-e", "dev"); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 || log.Len() != 0 {
		t.Fatalf("a step said %q %q", out.String(), log.String())
	}
	v, err := paths.LoadEnvValues()
	if err != nil || v.Args["region"] != "us" || v.Args["user"] != "" || v.Envs["dev"]["user"] != "toby" {
		t.Fatalf("%v %+v", err, v)
	}
}

func TestNounsAloneListWithoutATerminal(t *testing.T) {
	t.Parallel()
	paths := cliFixture(t)
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"env"}, "dev (default)"},
		{[]string{"var"}, "Your user name"},
		{[]string{"var", "-g"}, "NAME"},
		{[]string{"context"}, "commands work in env dev"},
	} {
		if out, _, err := run(t, paths, "", c.args...); err != nil || !strings.Contains(out, c.want) {
			t.Fatalf("%v: %v %q", c.args, err, out)
		}
	}
}

func TestFuzzyMatchesLettersInOrderTheCloserTheBetter(t *testing.T) {
	t.Parallel()
	texts := []string{"payments-api main", "architecture-decisions", "ad-hoc", "spec business"}
	if got := fuzzyFilter("arch dec", texts); !slices.Equal(got, []int{1}) {
		t.Fatalf("%v", got)
	}
	if got := fuzzyFilter("ad", texts); !slices.Equal(got, []int{2, 1}) {
		t.Fatalf("ad-hoc, its letters side by side at a word's start, is not first: %v", got)
	}
	if got := fuzzyFilter("spcbus", texts); !slices.Equal(got, []int{3}) {
		t.Fatalf("%v", got)
	}
	if got := fuzzyFilter("", texts); len(got) != len(texts) {
		t.Fatalf("%v", got)
	}
}

func TestAListIsFilteredAfterASlashAndActsOnWhatItLeaves(t *testing.T) {
	t.Parallel()
	m := listModel{title: "repos", header: []string{"NAME"}, rows: [][]string{{"payments"}, {"adrs"}, {"spec"}}, ids: []string{"payments", "adrs", "spec"}, actions: []listAction{
		{key: "enter", name: "open", help: "open"},
	}}
	// Typed after /, a q or a j is the filter's, not the list's.
	got := listKeys(m, "/", "s", "p", "q")
	if got.done || !got.filtering || got.query != "spq" || !strings.Contains(got.View(), "nothing matches") {
		t.Fatalf("%+v\n%s", got, got.View())
	}
	got = listKeys(m, "/", "s", "p")
	got = listKeys(got, "enter")
	if got.filtering || got.query != "sp" || got.id() != "spec" || got.chosen != nil {
		t.Fatalf("%+v", got)
	}
	if got = listKeys(got, "enter"); got.chosen == nil || got.id() != "spec" {
		t.Fatalf("%+v", got)
	}
	// Esc clears a filter before it leaves.
	var tm tea.Model = listKeys(m, "/", "a", "enter")
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if got := tm.(listModel); got.done || got.query != "" {
		t.Fatalf("%+v", got)
	}
}

func TestKeysWiderThanTheTerminalWrapBetweenKeysAndNoneIsLost(t *testing.T) {
	t.Parallel()
	pairs := []string{"↑/↓ j/k", "move", "enter", "tasks", "s", "shell", "p", "ports", "i", "inspect", "r", "resources", "t", "stop", "c", "recreate", "d", "remove", "q", "quit"}
	lines := helpLines(40, pairs...)
	if len(lines) < 3 {
		t.Fatalf("%q", lines)
	}
	all := ""
	for _, l := range lines {
		if w := lipgloss.Width(l); w > 39 {
			t.Fatalf("%d wide: %q", w, l)
		}
		all += ansi.Strip(l) + " · "
	}
	for i := 0; i < len(pairs); i += 2 {
		if !strings.Contains(all, pairs[i]+" "+pairs[i+1]+" · ") {
			t.Fatalf("%s lost from %q", pairs[i], all)
		}
	}
	// A list leaves its rows the room the keys do not take.
	m := listModel{title: "t", header: []string{"A"}, rows: [][]string{{"x"}}, ids: []string{"x"}, actions: []listAction{{key: "enter", name: "a", help: "a long action of the list"}, {key: "g", name: "b", help: "another long action"}}, width: 30, height: 20}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "q quit") || !strings.Contains(v, "g another long action") {
		t.Fatalf("%s", v)
	}
}
