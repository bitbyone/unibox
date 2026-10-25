package unibox

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// envFixture lays out a team's repository as agent-sandboxes has it: an
// sbxenv in sandboxes/ naming kits beside it in kits/.
func envFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, filepath.Join(root, "sandboxes", "dev.sbxenv.yaml"), `schemaVersion: "1"
name: team-dev
agent: sbx-kit-copilot
args:
  user:
    required: true
    description: Your user name
  memory:
    default: ""
    pattern: "([0-9]+[mg])?"
  cpus:
    default: "0"
kits:
  - docker.io/docker/sbx-kit-copilot:1.0.86
  - ../kits/git
  - source: ../kits/registry
    args:
      user: ${{ env.args.user }}
bindings:
  copilot: {apiKey: {domains: [api.githubcopilot.com]}}
  gitlab: {apiKey: {domains: [gitlab.example.com]}}
lifecycle:
  postCreate: [{name: x, command: "true"}]
somethingNewer: {sbx: adds fields}
`)
	write(t, filepath.Join(root, "kits", "git", "git.yaml"), "kind: mixin\n")
	write(t, filepath.Join(root, "kits", "registry", "registry.yaml"), "kind: mixin\n")
	return root
}

func TestSBXEnvIsReadAsSbxMergesIt(t *testing.T) {
	t.Parallel()
	root := envFixture(t)
	local := filepath.Join(root, "sandboxes", "local.sbxenv.yaml")
	write(t, local, `args:
  user:
    default: me
    required: false
kits:
  - ../kits/git
  - ./extra
bindings:
  registry: {apiKey: {domains: [r.example.com]}}
`)
	e, err := ReadSBXEnv([]string{filepath.Join(root, "sandboxes", "dev.sbxenv.yaml"), local})
	if err != nil {
		t.Fatal(err)
	}
	if e.Name != "team-dev" || e.Agent != "sbx-kit-copilot" || e.SandboxName("x") != "team-dev" {
		t.Fatal(e)
	}
	user := e.Args["user"]
	if user.Required || user.Default == nil || *user.Default != "me" || user.Description != "Your user name" {
		t.Fatalf("an overlay did not override the arg field by field: %+v", user)
	}
	dirs := []string{}
	for _, k := range e.Kits {
		dirs = append(dirs, k.Dir)
	}
	want := []string{"", filepath.Join(root, "kits", "git"), filepath.Join(root, "kits", "registry"), filepath.Join(root, "sandboxes", "extra")}
	if !slices.Equal(dirs, want) {
		t.Fatalf("kits %v, want %v", dirs, want)
	}
	if !slices.Equal(e.Bindings, []string{"copilot", "gitlab", "registry"}) {
		t.Fatal(e.Bindings)
	}
	if _, err = ReadSBXEnv([]string{filepath.Join(root, "absent.yaml")}); err == nil {
		t.Fatal("a missing file was read")
	}
	if _, err = ReadSBXEnv(nil); err == nil {
		t.Fatal("an env without files was read")
	}
}

func TestEnvArgsAreCheckedBeforeSbx(t *testing.T) {
	t.Parallel()
	e, err := ReadSBXEnv([]string{filepath.Join(envFixture(t), "sandboxes", "dev.sbxenv.yaml")})
	if err != nil {
		t.Fatal(err)
	}
	if missing := e.Missing(map[string]string{}); !slices.Equal(missing, []string{"user"}) {
		t.Fatal(missing)
	}
	if missing := e.Missing(map[string]string{"user": "me"}); len(missing) != 0 {
		t.Fatal(missing)
	}
	unknown, err := e.CheckArgs(map[string]string{"user": "me", "memory": "16g", "colour": "red"})
	if err != nil || !slices.Equal(unknown, []string{"colour"}) {
		t.Fatal(unknown, err)
	}
	if _, err = e.CheckArgs(map[string]string{"memory": "lots"}); err == nil || !strings.Contains(err.Error(), "unibox var set memory") {
		t.Fatal("a value the pattern refuses was taken:", err)
	}
	if d := e.Declared(map[string]string{"user": "me", "colour": "red"}); len(d) != 1 || d["user"] != "me" {
		t.Fatal(d)
	}
	for _, bad := range [][2]string{{"user", "a\nb"}, {"user", "secret:gitlab"}, {"1bad name", "x"}} {
		if ValidArg(bad[0], bad[1]) == nil {
			t.Fatal("accepted", bad)
		}
	}
}

func TestFingerprintFollowsFilesKitsAndArgsOnly(t *testing.T) {
	t.Parallel()
	root := envFixture(t)
	file := filepath.Join(root, "sandboxes", "dev.sbxenv.yaml")
	read := func() SBXEnv {
		e, err := ReadSBXEnv([]string{file})
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	args := map[string]string{"user": "me"}
	base, err := read().Fingerprint(args)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "README.md"), "not the env's\n")
	write(t, filepath.Join(root, "kits", "unused", "unused.yaml"), "kind: mixin\n")
	if same, _ := read().Fingerprint(args); same != base {
		t.Fatal("a file outside the env changed the fingerprint")
	}
	if other, _ := read().Fingerprint(map[string]string{"user": "you"}); other == base {
		t.Fatal("an arg did not change the fingerprint")
	}
	write(t, filepath.Join(root, "kits", "git", "git.yaml"), "kind: mixin\n# changed\n")
	kit, _ := read().Fingerprint(args)
	if kit == base {
		t.Fatal("a kit's file did not change the fingerprint")
	}
	b, _ := os.ReadFile(file)
	write(t, file, string(b)+"# changed\n")
	if changed, _ := read().Fingerprint(args); changed == kit {
		t.Fatal("the env's file did not change the fingerprint")
	}
}

func TestArgsFileIsPrivateAndRemoved(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path, remove, err := WriteArgsFile(dir, map[string]string{"user": "me", "memory": "16g"})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal(info, err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "memory=16g\nuser=me\n" {
		t.Fatalf("%q", b)
	}
	remove()
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("the args file stayed")
	}
}

func TestEnvValuesKeepOtherTables(t *testing.T) {
	t.Parallel()
	paths := Paths{Config: t.TempDir()}
	write(t, paths.VarsFile(), "[global]\nname = \"x\"\n\n[envs.dev.args]\nuser = \"me\"\n")
	v, err := paths.LoadEnvValues()
	if err != nil || v.Envs["dev"]["user"] != "me" {
		t.Fatal(v, err)
	}
	v.Envs["dev"]["memory"] = "16g"
	if err = paths.SaveEnvValues(v); err != nil {
		t.Fatal(err)
	}
	again, err := paths.LoadEnvValues()
	if err != nil || again.Envs["dev"]["memory"] != "16g" || again.other["global"] == nil {
		t.Fatal(again, err)
	}
	write(t, paths.VarsFile(), "[envs.dev.args]\ntoken = \"secret:gitlab\"\n")
	if _, err = paths.LoadEnvValues(); err == nil {
		t.Fatal("a secret was read as a value")
	}
}

func TestEnvConfigIsValidatedAndNamesItsSandbox(t *testing.T) {
	t.Parallel()
	root := envFixture(t)
	file := filepath.Join(root, "sandboxes", "dev.sbxenv.yaml")
	c := Config{Envs: map[string]EnvConfig{"dev": {Files: []string{file}}}}
	e, err := c.LoadEnv("dev")
	if err != nil || e.SandboxName() != "team-dev" {
		t.Fatal(e, err)
	}
	c.Envs["dev"] = EnvConfig{Files: []string{file}, Name: "mine"}
	if e, _ = c.LoadEnv("dev"); e.SandboxName() != "mine" {
		t.Fatal(e.SandboxName())
	}
	if _, err = c.LoadEnv("absent"); err == nil {
		t.Fatal("an absent env loaded")
	}
	for _, bad := range []EnvConfig{{}, {Files: []string{file}, Name: "Bad Name"}, {Files: []string{file}, Ports: map[string]Port{"web": {Spec: "nope"}}}} {
		if ValidEnvConfig("dev", bad) == nil {
			t.Fatal("accepted", bad)
		}
	}
}
