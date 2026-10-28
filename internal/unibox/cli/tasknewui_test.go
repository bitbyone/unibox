package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
)

// runCmd feeds a model what its command says, a batch's every message.
func runCmd(m tea.Model, cmd tea.Cmd) tea.Model {
	if cmd == nil {
		return m
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			m = runCmd(m, c)
		}
	case nil:
	default:
		var next tea.Cmd
		m, next = m.Update(msg)
		m = runCmd(m, next)
	}
	return m
}

func taskKeys(m tea.Model, keys ...string) tea.Model {
	for _, k := range keys {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case " ":
			msg = tea.KeyMsg{Type: tea.KeySpace}
		}
		var cmd tea.Cmd
		m, cmd = m.Update(msg)
		m = runCmd(m, cmd)
	}
	return m
}

func TestANewTasksScreenShowsWhatEachRepositoryGetsAndTheTasksName(t *testing.T) {
	t.Parallel()
	// A checkout of the host, which the task mounts.
	checkout := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "trunk"}, {"remote", "add", "origin", "https://forge.example.com/x/web.git"}} {
		if b, err := exec.Command("git", append([]string{"-C", checkout}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, b)
		}
	}
	top, _ := filepath.EvalSymlinks(checkout)
	repos := map[string]unibox.IndexedRepo{
		"adrs": {URL: "https://forge.example.com/arch/adrs.git", Ref: "main", Default: unibox.RepoAlways},
		"api":  {URL: "https://forge.example.com/x/api.git", Ref: "develop", Default: unibox.RepoOn},
		"spec": {URL: "https://forge.example.com/biz/spec.git", Default: unibox.RepoOff},
		"web":  {URL: checkout, Default: unibox.RepoOn},
	}
	m := taskPickModel{env: "dev", rows: taskRows(context.Background(), repos), existing: map[string]bool{}}
	var tm tea.Model = m
	// The task is named by what is picked, mounted first, before anything
	// is asked - after the first worked in, not the decisions every task
	// reads.
	want := unibox.TaskID("api", []api.Repository{
		{Name: "web", URL: "https://forge.example.com/x/web.git", Branch: "trunk", HostPath: top},
		{Name: "adrs", URL: "https://forge.example.com/arch/adrs.git", Branch: "main"},
		{Name: "api", URL: "https://forge.example.com/x/api.git", Branch: "develop"},
	})
	if v := tm.View(); !strings.Contains(v, "task "+want+" (generated)") || !strings.HasPrefix(want, "api-") || !strings.Contains(v, "mount  trunk") {
		t.Fatalf("%s", v)
	}
	// The branch typed goes to the ones worked in, pushed unless told not;
	// a context one keeps its ref, one with none its forge's default.
	tm = taskKeys(tm, "b", "feat/x", "enter", "j", "j", " ")
	v := tm.View()
	if !strings.Contains(v, "context  clone  main") || !strings.Contains(v, "feat/x  yes") || !strings.Contains(v, "branch feat/x in 3, pushed from 3") {
		t.Fatalf("%s", v)
	}
	fresh := m
	fresh.rows = slices.Clone(m.rows)
	if v := taskKeys(fresh, "j", "j", " ").View(); !strings.Contains(v, "spec  work     clone  default") {
		t.Fatalf("%s", v)
	}
	// spec, picked by hand, names the task now.
	if got := tm.(taskPickModel); !strings.HasPrefix(got.taskName(), "spec-") {
		t.Fatalf("%s", got.taskName())
	}
	// spec's branch made in the sandbox alone; web's left on the host.
	tm = taskKeys(tm, "p", "j", "p")
	got := tm.(taskPickModel)
	ins := map[string]unibox.RepoInput{}
	for _, in := range got.inputs() {
		ins[in.Name] = in
	}
	if in := ins["spec"]; !in.Fork || in.Push || in.Branch != "feat/x" || !in.Clone {
		t.Fatalf("%+v", in)
	}
	if in := ins["web"]; in.Push || in.Clone || in.Source != checkout || in.Branch != "feat/x" {
		t.Fatalf("%+v", in)
	}
	if in := ins["api"]; !in.Push || in.Branch != "feat/x" {
		t.Fatalf("%+v", in)
	}
	if in := ins["adrs"]; !in.Context || in.Push || in.Branch != "main" {
		t.Fatalf("%+v", in)
	}
	// A context one has nothing to push.
	tm = taskKeys(got, "k", "k", "k", "p")
	if !strings.Contains(tm.View(), "nothing is pushed there") {
		t.Fatalf("%s", tm.View())
	}
	got = tm.(taskPickModel)
	got.existing[got.taskName()] = true
	if v := got.View(); !strings.Contains(v, "(exists; it is resumed)") {
		t.Fatalf("%s", v)
	}
	tm = taskKeys(got, "t", "pricing", "enter", "enter")
	got = tm.(taskPickModel)
	if !got.done || got.taskName() != "pricing" {
		t.Fatalf("%+v", got)
	}
	// A name a task cannot have is refused where it is typed.
	if bad := taskKeys(m, "t", "Not A Name", "enter").(taskPickModel); bad.editing == "" || !strings.Contains(bad.View(), "letters, digits and dashes") {
		t.Fatalf("%+v", bad)
	}
}

func TestADirectoryNamedToANewTaskIsOfferedFirstAndMounted(t *testing.T) {
	t.Parallel()
	gitInit := func(dir string, args ...[]string) string {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, a := range append([][]string{{"init", "-q", "-b", "trunk"}}, args...) {
			if b, err := exec.Command("git", append([]string{"-C", dir}, a...)...).CombinedOutput(); err != nil {
				t.Fatalf("%v %s", err, b)
			}
		}
		top, _ := filepath.EvalSymlinks(dir)
		return top
	}
	// A checkout of a repository the index clones takes its place and name.
	api := gitInit(t.TempDir(), []string{"remote", "add", "origin", "git@forge.example.com:x/api.git"})
	// One the index does not know is named after its directory.
	scratch := gitInit(filepath.Join(t.TempDir(), "scratch"))
	read := true
	repos := map[string]unibox.IndexedRepo{
		"adrs": {URL: "https://forge.example.com/arch/adrs.git", Default: unibox.RepoOn, Context: &read},
		"api":  {URL: "https://forge.example.com/x/api.git", Default: unibox.RepoOff, Instructions: "Money is in cents."},
	}
	index := taskRows(context.Background(), repos)
	host, replaced, err := hostRows(context.Background(), []string{api, scratch}, index)
	if err != nil || !slices.Equal(replaced, []string{"api"}) || len(host) != 2 {
		t.Fatalf("%+v %v %v", host, replaced, err)
	}
	if h := host[0]; h.name != "api" || !h.picked || h.context || !h.push || h.current != "trunk" || h.repo.Instructions == "" {
		t.Fatalf("%+v", h)
	}
	if h := host[1]; h.name != "scratch" || h.push {
		t.Fatalf("%+v", h)
	}
	index = slices.DeleteFunc(index, func(r taskRepo) bool { return slices.Contains(replaced, r.name) })
	var tm tea.Model = taskPickModel{env: "dev", rows: append(host, index...), existing: map[string]bool{}}
	v := tm.View()
	if !strings.Contains(v, "api      work     mount  trunk") || !strings.Contains(v, "mounted from the host") || strings.Count(v, "api ") != 1 {
		t.Fatalf("%s", v)
	}
	// The task's branch goes to the checkout too, pushed to its forge.
	got := taskKeys(tm, "b", "feat/x", "enter").(taskPickModel)
	ins := got.inputs()
	if len(ins) != 3 || ins[0].Source != api || ins[0].Name != "api" || ins[0].Clone || ins[0].Branch != "feat/x" || !ins[0].Push || ins[1].Push {
		t.Fatalf("%+v", ins)
	}
	if _, _, err := hostRows(context.Background(), []string{filepath.Join(scratch, "missing")}, nil); err == nil {
		t.Fatal("a missing directory was taken")
	}
}

func TestRecentRepositoriesAreSearchedShownWhenAskedAndNamedInTheIndex(t *testing.T) {
	t.Parallel()
	checkout := filepath.Join(t.TempDir(), "Billing")
	for _, args := range [][]string{{"init", "-q", "-b", "trunk", checkout}, {"-C", checkout, "remote", "add", "origin", "https://forge.example.com/x/billing.git"}} {
		if b, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, b)
		}
	}
	top, _ := filepath.EvalSymlinks(checkout)
	repos := map[string]unibox.IndexedRepo{"api": {URL: "https://forge.example.com/x/api.git", Default: unibox.RepoOn}}
	index := taskRows(context.Background(), repos)
	// One the index names already is not offered twice; a name the index
	// has is not taken again.
	recent := []unibox.RecentRepo{{Path: top}, {URL: "https://forge.example.com/x/api.git"}, {URL: "https://forge.example.com/y/api.git"}, {Path: filepath.Join(t.TempDir(), "gone")}}
	rows := append(index, recentRows(context.Background(), recent, index, map[string]bool{"api": true})...)
	if len(rows) != 3 || rows[1].name != "billing" || rows[1].top != top || rows[2].name != "api-2" {
		t.Fatalf("%+v", rows)
	}
	var named []string
	m := taskPickModel{env: "dev", rows: rows, existing: map[string]bool{}, name: func(name string, r unibox.IndexedRepo) (string, error) {
		named = append(named, name+"="+r.URL)
		return "for env dev", nil
	}}
	var tm tea.Model = m
	if v := tm.View(); strings.Contains(v, "billing") || !strings.Contains(v, "r recent") {
		t.Fatalf("recent shown before asked for:\n%s", v)
	}
	// A search finds it; picked, it stays in sight.
	tm = taskKeys(tm, "/", "b", "i", "l", "enter", " ")
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if v := tm.View(); !strings.Contains(v, "billing") || !strings.Contains(v, "mount  trunk") {
		t.Fatalf("%s", v)
	}
	// m clones it from its forge instead, on the forge's default branch.
	tm = taskKeys(tm, "j", "m")
	in := tm.(taskPickModel).inputs()
	if len(in) != 2 || in[1].Source != "https://forge.example.com/x/billing.git" || !in[1].Clone {
		t.Fatalf("%+v", in)
	}
	tm = taskKeys(tm, "a")
	if got := tm.(taskPickModel); len(named) != 1 || named[0] != "billing="+tildePath(top) || got.rows[1].recent || !strings.Contains(got.View(), "named billing for env dev") {
		t.Fatalf("%v %s", named, got.View())
	}
	// r shows the rest.
	tm = taskKeys(tm, "r")
	if v := tm.View(); !strings.Contains(v, "api-2") {
		t.Fatalf("%s", v)
	}
}
