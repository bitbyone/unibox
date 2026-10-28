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
	if err := m.SetRepo(Scope{}, "notes", IndexedRepo{URL: "https://github.com/me/notes.git", Default: RepoAlways}); err != nil {
		t.Fatal(err)
	}
	if err := m.SetRepo(Scope{Env: "dev"}, "api", IndexedRepo{URL: "https://gitlab.example.com/x/api.git", Ref: "main"}); err != nil {
		t.Fatal(err)
	}
	if err := m.SetRepoDefault(Scope{Env: "dev"}, "adrs", RepoAlways); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(m.Paths.Config, "config.yaml"))
	for _, s := range []string{"notes:\n        url: https://github.com/me/notes.git\n        default: always", "api:\n                url: https://gitlab.example.com/x/api.git\n                ref: main", "adrs:\n                default: always"} {
		if !strings.Contains(string(b), s) {
			t.Fatalf("no %q in\n%s", s, b)
		}
	}
	// Back to what the team says, the switch goes.
	if err := m.SetRepoDefault(Scope{Env: "dev"}, "adrs", RepoOn); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Config.Envs["dev"].Repos["adrs"]; ok {
		t.Fatalf("%+v", m.Config.Envs["dev"].Repos)
	}
	if err := m.RemoveRepo(Scope{Env: "dev"}, "adrs"); err == nil || !strings.Contains(err.Error(), "one of the team's stays") {
		t.Fatalf("%v", err)
	}
	if err := m.RemoveRepo(Scope{Env: "dev"}, "api"); err != nil {
		t.Fatal(err)
	}
	if err := m.SetRepoDefault(Scope{Env: "dev"}, "nope", RepoOn); err == nil {
		t.Fatal("a switch for no repository was written")
	}
	if err := m.SetRepo(Scope{Env: "dev"}, "bad", IndexedRepo{URL: "not a forge"}); err == nil {
		t.Fatal("a repository with no forge address was written")
	}
	// A switch for one env goes with the repository of every env it
	// switched.
	if err := m.SetRepo(Scope{}, "gone", IndexedRepo{URL: "https://github.com/me/gone.git"}); err != nil {
		t.Fatal(err)
	}
	if err := m.SetRepoDefault(Scope{Env: "dev"}, "gone", RepoOn); err != nil {
		t.Fatal(err)
	}
	if err := m.RemoveRepo(Scope{}, "gone"); err != nil {
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

func TestAPersonsReposAndPortsAreWrittenForEveryEnvOfAnEnvRepo(t *testing.T) {
	t.Parallel()
	m, _ := managerFixture(t)
	sandboxes := filepath.Dir(m.Config.Envs["dev"].Files[0])
	write(t, filepath.Join(sandboxes, "dev.unibox.yaml"), "ports:\n  vite: {port: 5173}\nrepos:\n  adrs: {url: https://gitlab.example.com/arch/adrs.git}\n")
	scope := Scope{Env: "dev", EnvRepo: true}
	if err := m.SetRepo(scope, "notes", IndexedRepo{URL: "https://github.com/me/notes.git"}); err == nil || !strings.Contains(err.Error(), "in no repository") {
		t.Fatalf("an env of no repository took it: %v", err)
	}
	ec := m.Config.Envs["dev"]
	ec.Source = "https://GitHub.com/team/envs.git"
	m.Config.Envs["dev"] = ec
	if err := m.Paths.SaveConfig(m.Config); err != nil {
		t.Fatal(err)
	}
	if err := m.SetRepo(scope, "notes", IndexedRepo{URL: "https://github.com/me/notes.git"}); err != nil {
		t.Fatal(err)
	}
	if err := m.SetRepoDefault(scope, "adrs", RepoAlways); err != nil {
		t.Fatal(err)
	}
	on := true
	if err := m.SetPortDefault(scope, "vite", &on); err != nil {
		t.Fatal(err)
	}
	c, err := m.Paths.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	mine := c.EnvRepos["github.com/team/envs"]
	if mine.Repos["notes"].URL != "https://github.com/me/notes.git" || mine.Repos["adrs"] != (IndexedRepo{Default: RepoAlways}) || mine.Ports["vite"] != (Port{Default: true}) || len(c.Envs["dev"].Repos) != 0 || len(c.Repos) != 0 {
		t.Fatalf("%+v", c)
	}
	env, err := c.LoadEnv("dev")
	if err != nil || env.Repos["adrs"].DefaultFrom != FromYoursEnvRepo || env.Repos["notes"].From != FromYoursEnvRepo || env.Ports["vite"].DefaultFrom != FromYoursEnvRepo {
		t.Fatalf("%v %+v %+v", err, env.Repos, env.Ports)
	}
	// Back to what the team says, and with nothing left, the env repo goes.
	if err := m.SetPortDefault(scope, "vite", nil); err != nil {
		t.Fatal(err)
	}
	if err := m.SetRepoDefault(scope, "adrs", ""); err != nil {
		t.Fatal(err)
	}
	if err := m.RemoveRepo(scope, "notes"); err != nil {
		t.Fatal(err)
	}
	if err := m.RemoveRepo(scope, "notes"); err == nil || !strings.Contains(err.Error(), "for the env repo github.com/team/envs") {
		t.Fatalf("%v", err)
	}
	if c, _ := m.Paths.LoadConfig(); c.EnvRepos != nil {
		t.Fatalf("%+v", c.EnvRepos)
	}
}
