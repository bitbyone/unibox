package unibox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReposAreLayeredFromTheTeamToThePersonsEnv(t *testing.T) {
	t.Parallel()
	m, _ := managerFixture(t)
	sandboxes := filepath.Dir(m.Config.Envs["dev"].Files[0])
	root := filepath.Dir(sandboxes)
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "unibox.yaml"), `repos:
  adrs: {url: https://gitlab.example.com/arch/adrs.git, ref: main, default: always, description: Decisions}
  spec: https://gitlab.example.com/biz/spec.git
`)
	write(t, filepath.Join(sandboxes, "dev.unibox.yaml"), "repos:\n  spec: {default: on, ref: develop}\n")
	m.Config.Repos = map[string]IndexedRepo{"mine": {URL: "git@github.com:me/notes.git"}, "elsewhere": {Default: RepoOn}}
	ec := m.Config.Envs["dev"]
	ec.Repos = map[string]IndexedRepo{"adrs": {Default: RepoOff}}
	m.Config.Envs["dev"] = ec
	env, err := m.Config.LoadEnv("dev")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]IndexedRepo{
		"adrs": {URL: "https://gitlab.example.com/arch/adrs.git", Ref: "main", Default: RepoOff, Description: "Decisions", From: FromRepo, DefaultFrom: FromYours},
		"spec": {URL: "https://gitlab.example.com/biz/spec.git", Ref: "develop", Default: RepoOn, From: FromRepo, DefaultFrom: FromEnv},
		"mine": {URL: "git@github.com:me/notes.git", Default: RepoOff, From: FromYoursAll},
	}
	if len(env.Repos) != len(want) {
		t.Fatalf("%+v", env.Repos)
	}
	for name, r := range want {
		if env.Repos[name] != r {
			t.Errorf("%s: %+v", name, env.Repos[name])
		}
	}
	for _, bad := range []string{"repos:\n  x: {default: on}\n", "repos:\n  x: not a forge\n", "repos:\n  x: {url: https://h/x.git, default: sometimes}\n"} {
		write(t, filepath.Join(sandboxes, "dev.unibox.yaml"), bad)
		if _, err := m.Config.LoadEnv("dev"); err == nil {
			t.Errorf("%q was taken", bad)
		}
	}
}

func TestAPersonsReposAreWrittenAndASwitchThatChangesNothingDropped(t *testing.T) {
	t.Parallel()
	m, _ := managerFixture(t)
	sandboxes := filepath.Dir(m.Config.Envs["dev"].Files[0])
	write(t, filepath.Join(sandboxes, "dev.unibox.yaml"), "repos:\n  adrs: {url: https://gitlab.example.com/arch/adrs.git, default: on}\n")
	if err := m.SetRepo("", "notes", IndexedRepo{URL: "https://github.com/me/notes.git", Default: RepoAlways}); err != nil {
		t.Fatal(err)
	}
	if err := m.SetRepo("dev", "api", IndexedRepo{URL: "https://gitlab.example.com/x/api.git", Ref: "main"}); err != nil {
		t.Fatal(err)
	}
	if err := m.SetRepoDefault("dev", "adrs", RepoAlways); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(m.Paths.Config, "config.yaml"))
	for _, s := range []string{"notes:\n        url: https://github.com/me/notes.git\n        default: always", "api:\n                url: https://gitlab.example.com/x/api.git\n                ref: main", "adrs:\n                default: always"} {
		if !strings.Contains(string(b), s) {
			t.Fatalf("no %q in\n%s", s, b)
		}
	}
	// Back to what the team says, the switch goes.
	if err := m.SetRepoDefault("dev", "adrs", RepoOn); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Config.Envs["dev"].Repos["adrs"]; ok {
		t.Fatalf("%+v", m.Config.Envs["dev"].Repos)
	}
	if err := m.RemoveRepo("dev", "adrs"); err == nil || !strings.Contains(err.Error(), "one of the team's stays") {
		t.Fatalf("%v", err)
	}
	if err := m.RemoveRepo("dev", "api"); err != nil {
		t.Fatal(err)
	}
	if err := m.SetRepoDefault("dev", "nope", RepoOn); err == nil {
		t.Fatal("a switch for no repository was written")
	}
	if err := m.SetRepo("dev", "bad", IndexedRepo{URL: "not a forge"}); err == nil {
		t.Fatal("a repository with no forge address was written")
	}
	// A switch for one env goes with the repository of every env it
	// switched.
	if err := m.SetRepo("", "gone", IndexedRepo{URL: "https://github.com/me/gone.git"}); err != nil {
		t.Fatal(err)
	}
	if err := m.SetRepoDefault("dev", "gone", RepoOn); err != nil {
		t.Fatal(err)
	}
	if err := m.RemoveRepo("", "gone"); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Config.Envs["dev"].Repos["gone"]; ok {
		t.Fatalf("%+v", m.Config.Envs["dev"].Repos)
	}
	env, err := m.Config.LoadEnv("dev")
	if err != nil || env.Repos["notes"].Default != RepoAlways || env.Repos["adrs"].Default != RepoOn || len(env.Repos) != 2 {
		t.Fatalf("%+v %v", env.Repos, err)
	}
}
