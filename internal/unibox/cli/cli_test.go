package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/tobola/unibox/internal/testenv"
	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
	"github.com/tobola/unibox/internal/version"
)

func TestMain(m *testing.M) { testenv.Main(m) }

func TestBranchOrderSupportsDefaultsAndIndividualOverrides(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		args     []string
		global   string
		branches []string
	}{
		{"shared", []string{"--branch", "feat/shared", "--repo", "./api", "--repo", "./web"}, "feat/shared", []string{"", ""}},
		{"override", []string{"--branch=feat/shared", "--repo=./api", "--repo", "./web", "--branch", "feat/web", "--repo", "./worker"}, "feat/shared", []string{"", "feat/web", ""}},
		{"per-repo", []string{"--repo", "https://forge.invalid/api.git", "--branch", "feat/api", "--repo", "https://forge.invalid/web.git", "--branch=feat/web"}, "", []string{"feat/api", "feat/web"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var req unibox.Request
			c := &cobra.Command{Use: "test", Run: func(*cobra.Command, []string) {}}
			taskFlags(c, &req)
			c.SetArgs(tc.args)
			if err := c.Execute(); err != nil {
				t.Fatal(err)
			}
			if req.Branch != tc.global {
				t.Fatal(req.Branch)
			}
			got := []string{}
			for _, r := range req.Repositories {
				got = append(got, r.Branch)
			}
			if !reflect.DeepEqual(got, tc.branches) {
				t.Fatalf("%v != %v", got, tc.branches)
			}
		})
	}
}

// cliFixture is unibox configured with an env dev from a team's file,
// and no sbx: what needs it says so.
func cliFixture(t *testing.T) unibox.Paths {
	t.Helper()
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	file := filepath.Join(t.TempDir(), "dev.sbxenv.yaml")
	if err := os.WriteFile(file, []byte("name: team-dev\nargs:\n  user: {required: true, description: Your user name}\n  region: {default: eu}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := paths.SaveConfig(unibox.Config{DefaultEnv: "dev", Envs: map[string]unibox.EnvConfig{"dev": {Files: []string{file}}}}); err != nil {
		t.Fatal(err)
	}
	return paths
}

// run runs unibox with input, and returns what it printed and logged.
func run(t *testing.T, paths unibox.Paths, input string, args ...string) (string, string, error) {
	t.Helper()
	var out, log bytes.Buffer
	c := New(paths, strings.NewReader(input), &out, &log)
	c.SetArgs(args)
	err := c.Execute()
	return out.String(), log.String(), err
}

func addSandboxes(t *testing.T, paths unibox.Paths, names ...string) {
	t.Helper()
	err := paths.State(func(s *unibox.State) error {
		for _, name := range names {
			s.Sandboxes[name] = api.Sandbox{Name: name, Env: "dev", Runs: []api.Run{}, Tasks: []api.Task{
				{ID: name + "-task", Sandbox: name, Directory: "/work/" + name + "-task", Agent: "claude", State: "ready", Repositories: []api.Repository{{Name: "api", Branch: "feat/" + name}}},
			}}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestJSONListsAreSingleVersionedObjects(t *testing.T) {
	t.Parallel()
	paths := cliFixture(t)
	for _, args := range [][]string{{"env", "ls", "--json"}, {"config", "get", "--json"}, {"sandbox", "ls", "--json"}, {"task", "ls", "--json"}, {"context", "--json"}, {"var", "ls", "-e", "dev", "--json"}, {"version", "--json"}} {
		out, _, err := run(t, paths, "", args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		d := json.NewDecoder(strings.NewReader(out))
		var value map[string]any
		if err := d.Decode(&value); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if value["api_version"] != float64(api.Version) {
			t.Fatalf("%v: %v", args, value)
		}
		var extra any
		if err := d.Decode(&extra); err != io.EOF {
			t.Fatalf("%v: extra stdout: %v", args, err)
		}
	}
}

func TestConfigTakesItsKeysAlone(t *testing.T) {
	t.Parallel()
	paths := cliFixture(t)
	if _, _, err := run(t, paths, "", "config", "set", "stop_idle", "false"); err != nil {
		t.Fatal(err)
	}
	cfg, _ := paths.LoadConfig()
	if cfg.StopIdle() {
		t.Fatal("stop_idle stayed on")
	}
	for _, args := range [][]string{{"config", "set", "stop_idel", "false"}, {"config", "set", "default_env", "absent"}, {"config", "set", "stop_idle", "maybe"}} {
		if _, _, err := run(t, paths, "", args...); err == nil {
			t.Fatalf("%v was taken", args)
		}
	}
}

func TestHelpListsHowATaskIsMade(t *testing.T) {
	t.Parallel()
	paths := cliFixture(t)
	out, _, err := run(t, paths, "", "run", "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--repo", "repeat for several", "--branch", "shared branch", "--mount", "--clone", "--task", "--push", "--env"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help lacks %s", want)
		}
	}
	if _, _, err := run(t, paths, "", "run", "--mount", "--clone", "."); err == nil {
		t.Fatal("--mount and --clone were taken together")
	}
}

func TestTasksListsEverySandboxAndResumeNeedsOnlyTheTask(t *testing.T) {
	t.Parallel()
	paths := cliFixture(t)
	addSandboxes(t, paths, "work", "personal")
	out, _, err := run(t, paths, "", "task", "ls")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"SANDBOX", "REPOSITORIES", "personal-task  personal", "work-task      work", "api@feat/work"} {
		if !strings.Contains(out, want) {
			t.Fatalf("tasks lacks %q:\n%s", want, out)
		}
	}
	if out, _, err = run(t, paths, "", "task", "ls", "-s", "personal"); err != nil || strings.Contains(out, "work-task") || !strings.Contains(out, "personal-task") {
		t.Fatalf("%v:\n%s", err, out)
	}
	// The sandbox is found from the task; what stops it then is the env's
	// arg not set.
	if _, _, err = run(t, paths, "", "resume", "personal-task"); err == nil || strings.Contains(err.Error(), "no sandbox chosen") {
		t.Fatalf("%v", err)
	}
	if _, _, err = run(t, paths, "", "resume", "nothing"); err == nil || !strings.Contains(err.Error(), "unibox task ls lists them") {
		t.Fatalf("%v", err)
	}
	// An alias is listed beside the ID.
	if out, _, err = run(t, paths, "", "task", "rename", "personal-task", "blog"); err != nil || !strings.Contains(out, "personal-task is also blog") {
		t.Fatalf("%v:\n%s", err, out)
	}
	if out, _, err = run(t, paths, "", "task", "ls"); err != nil || !strings.Contains(out, "ALIAS") || !strings.Contains(out, "personal-task  blog   personal") || !strings.Contains(out, "work-task      -      work") {
		t.Fatalf("%v:\n%s", err, out)
	}
}

func TestVersionIsTheReleaseAlone(t *testing.T) {
	t.Parallel()
	paths := cliFixture(t)
	for _, args := range [][]string{{"--version"}, {"version"}} {
		if out, _, err := run(t, paths, "", args...); err != nil || strings.TrimSpace(out) != version.String() {
			t.Fatalf("%v: %v %q", args, err, out)
		}
	}
	if out, _, err := run(t, paths, "", "version", "--json"); err != nil || !strings.Contains(out, `"version": "`+version.String()+`"`) {
		t.Fatalf("%v %s", err, out)
	}
}

func TestTheContextSaysTheSandboxAndEnvWhenNotTold(t *testing.T) {
	t.Parallel()
	paths := cliFixture(t)
	addSandboxes(t, paths, "work")
	out, _, err := run(t, paths, "", "context")
	if err != nil || !strings.Contains(out, "work (the only one)") || !strings.Contains(out, "dev") {
		t.Fatalf("%v\n%s", err, out)
	}
	addSandboxes(t, paths, "personal")
	if _, _, err = run(t, paths, "", "sandbox", "stop"); err == nil || !strings.Contains(err.Error(), "personal, work") {
		t.Fatalf("%v", err)
	}
	if _, _, err = run(t, paths, "", "context", "use", "-s", "missing"); err == nil {
		t.Fatal("a missing sandbox became the context")
	}
	if _, _, err = run(t, paths, "", "context", "use", "-e", "missing"); err == nil {
		t.Fatal("a missing env became the context")
	}
	if out, _, err = run(t, paths, "", "context", "use", "-s", "personal"); err != nil || !strings.Contains(out, "personal") {
		t.Fatalf("%v\n%s", err, out)
	}
	// The sandbox is found; what stops it then is sbx, missing here.
	if _, _, err = run(t, paths, "", "sandbox", "stop"); err == nil || strings.Contains(err.Error(), "no sandbox chosen") {
		t.Fatalf("%v", err)
	}
	if out, _, err = run(t, paths, "", "context", "clear"); err != nil || strings.Contains(out, "personal") {
		t.Fatalf("%v\n%s", err, out)
	}
}

func TestArgsAreKeptByEnvAndCheckedAgainstIt(t *testing.T) {
	t.Parallel()
	paths := cliFixture(t)
	out, _, err := run(t, paths, "", "var", "ls", "-e", "dev")
	if err != nil || !strings.Contains(out, "user") || !strings.Contains(out, "missing") {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, _, err = run(t, paths, "", "var", "set", "-e", "dev", "user", "person"); err != nil {
		t.Fatal(err)
	}
	values, _ := paths.LoadEnvValues()
	if values.Envs["dev"]["user"] != "person" {
		t.Fatalf("%+v", values)
	}
	if _, _, err = run(t, paths, "", "var", "set", "-e", "absent", "user", "x"); err == nil {
		t.Fatal("an arg was set for an env that is not there")
	}
	if _, _, err = run(t, paths, "", "var", "rm", "-e", "dev", "user"); err != nil {
		t.Fatal(err)
	}
	if values, _ = paths.LoadEnvValues(); len(values.Envs["dev"]) != 0 {
		t.Fatalf("%+v", values)
	}

	// A value for every env is the env's while it has none of its own.
	if _, _, err = run(t, paths, "", "var", "set", "-g", "user", "everyone"); err != nil {
		t.Fatal(err)
	}
	if out, _, err = run(t, paths, "", "var", "ls", "-e", "dev"); err != nil || !strings.Contains(out, "everyone") || !strings.Contains(out, "global") {
		t.Fatalf("%v\n%s", err, out)
	}
	if out, _, err = run(t, paths, "", "var", "ls", "-g"); err != nil || !strings.Contains(out, "everyone") {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, _, err = run(t, paths, "", "var", "set", "-g", "-e", "dev", "user", "x"); err == nil {
		t.Fatal("--global and -e were taken together")
	}
	if _, _, err = run(t, paths, "", "var", "rm", "-g", "user"); err != nil {
		t.Fatal(err)
	}
	if values, _ = paths.LoadEnvValues(); len(values.Args) != 0 {
		t.Fatalf("%+v", values)
	}
}

func TestPortsAreSwitchedOnlyInARunningSandbox(t *testing.T) {
	t.Parallel()
	paths := cliFixture(t)
	cfg, _ := paths.LoadConfig()
	ec := cfg.Envs["dev"]
	ec.Ports = map[string]unibox.Port{"fe": {Spec: "3000:3000"}}
	cfg.Envs["dev"] = ec
	if err := paths.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, paths, "", "port", "on", "-s", "dev"); err == nil || !strings.Contains(err.Error(), "name a port") {
		t.Fatalf("%v", err)
	}
	// The ports are written by hand; no command writes them.
	if _, _, err := run(t, paths, "", "port", "set", "be", "--map", "1"); err == nil {
		t.Fatal("port set is still there")
	}
	if _, _, err := run(t, paths, "", "port", "on", "fe=13000", "-s", "dev"); err == nil {
		t.Fatal("a port was switched in a sandbox that is not running")
	}
	if cfg, _ = paths.LoadConfig(); cfg.Envs["dev"].Ports["fe"] != (unibox.Port{Spec: "3000:3000"}) {
		t.Fatalf("%+v", cfg.Envs["dev"].Ports)
	}
}

func TestDoctorSaysWhatIsMissing(t *testing.T) {
	t.Parallel()
	paths := cliFixture(t)
	out, _, err := run(t, paths, "", "doctor")
	if err == nil {
		t.Fatal("doctor passed without sbx")
	}
	for _, want := range []string{"sbx is not on PATH", "arg user", "unibox var set -e dev user"} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor lacks %q:\n%s", want, out)
		}
	}
}

func TestDestroyAsksAndAllRemovesUniboxsOwnFiles(t *testing.T) {
	t.Parallel()
	paths := cliFixture(t)
	cfg, _ := paths.LoadConfig()
	if _, _, err := run(t, paths, "", "destroy"); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("%v", err)
	}
	if out, _, err := run(t, paths, "", "destroy", "--all", "--yes"); err != nil || !strings.Contains(out, "unibox is as if never used here") {
		t.Fatalf("%v %s", err, out)
	}
	if _, err := os.Stat(filepath.Join(paths.Config, "config.yaml")); !os.IsNotExist(err) {
		t.Fatalf("config stayed: %v", err)
	}
	if _, err := os.Stat(cfg.Envs["dev"].Files[0]); err != nil {
		t.Fatal("the env's file went")
	}
}

func TestRunTakesEitherAShellOrAnAgent(t *testing.T) {
	t.Parallel()
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	for _, command := range []string{"run", "resume"} {
		_, _, err := run(t, paths, "", command, "refunds", "--shell", "--agent", "codex")
		if err == nil || !strings.Contains(err.Error(), "[agent shell] were all set") {
			t.Fatalf("%s: %v", command, err)
		}
	}
}

func TestSeveralRepositoriesAreNamedWithRepo(t *testing.T) {
	t.Parallel()
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"run", "../web", "../api"}, "name several with --repo"},
		{[]string{"task", "prepare", "../web", "--repo", "../api"}, "do not mix"},
		{[]string{"task", "add-repo", "login", "../web", "../api"}, "accepts between 1 and 2 arg(s)"},
		{[]string{"task", "add-repo", "login", "../web", "--repo", "../api"}, "do not mix"},
	} {
		_, _, err := run(t, paths, "", c.args...)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%v: %v", c.args, err)
		}
	}
}

func TestRepoRecentListsAnEnvsAloneAndForgetsThem(t *testing.T) {
	t.Parallel()
	paths := cliFixture(t)
	checkout := t.TempDir()
	err := paths.State(func(s *unibox.State) error {
		s.Recent = []unibox.RecentRepo{
			{Env: "dev", Path: checkout, URL: "https://forge.example.com/x/web.git"},
			{Env: "dev", URL: "https://forge.example.com/x/api.git"},
			{Env: "ops", URL: "https://forge.example.com/ops/infra.git"},
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := run(t, paths, "", "repo", "recent")
	if err != nil || !strings.Contains(out, "x/web.git") || !strings.Contains(out, "x/api.git") || strings.Contains(out, "infra") {
		t.Fatalf("%v\n%s", err, out)
	}
	if out, _, err = run(t, paths, "", "repo", "recent", "--all"); err != nil || !strings.Contains(out, "infra") {
		t.Fatalf("%v\n%s", err, out)
	}
	// A checkout is forgotten by its path, as typed; one never recorded is
	// said to be no recent one.
	if _, _, err = run(t, paths, "", "repo", "recent", "rm", checkout, "https://forge.example.com/x/api.git"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = run(t, paths, "", "repo", "recent", "rm", "https://forge.example.com/ops/infra.git"); err == nil || !strings.Contains(err.Error(), "no recent repository") {
		t.Fatalf("%v", err)
	}
	if out, _, err = run(t, paths, "", "repo", "recent", "--all"); err != nil || strings.Contains(out, "x/") || !strings.Contains(out, "infra") {
		t.Fatalf("%v\n%s", err, out)
	}
}
