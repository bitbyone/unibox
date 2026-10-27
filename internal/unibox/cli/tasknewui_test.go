package cli

import (
	"context"
	"os/exec"
	"path/filepath"
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
	asked := []string{}
	m := taskPickModel{env: "dev", rows: taskRows(context.Background(), repos), states: map[string]branchState{}, defaults: map[string]string{}, existing: map[string]bool{}, check: func(r taskRepo, branch string) (string, branchState) {
		asked = append(asked, r.name+"@"+branch)
		switch {
		case branch == "" || r.name == "api":
			return "master", branchThere
		}
		return "master", branchNew
	}}
	var tm tea.Model = runCmd(m, m.Init())
	// The task is named by what is picked, mounted first, before anything
	// is asked - after the first worked in, not the decisions every task
	// reads.
	want := unibox.TaskID("api", []api.Repository{
		{Name: "web", URL: "https://forge.example.com/x/web.git", Branch: "trunk", HostPath: top},
		{Name: "adrs", URL: "https://forge.example.com/arch/adrs.git", Branch: "main"},
		{Name: "api", URL: "https://forge.example.com/x/api.git", Branch: "develop"},
	})
	if v := tm.View(); !strings.Contains(v, "task "+want+" (generated)") || !strings.HasPrefix(want, "api-") || len(asked) != 0 || !strings.Contains(v, "mount  trunk") {
		t.Fatalf("%v\n%s", asked, v)
	}
	// The branch typed goes to the ones worked in, pushed unless told not;
	// a context one keeps its ref.
	tm = taskKeys(tm, "b", "feat/x", "enter", "j", "j", " ")
	v := tm.View()
	if !strings.Contains(v, "context  clone  main") || !strings.Contains(v, "feat/x        yes") || !strings.Contains(v, "feat/x (new)  yes") || !strings.Contains(v, "made in 2, pushed from 3") {
		t.Fatalf("%v\n%s", asked, v)
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
