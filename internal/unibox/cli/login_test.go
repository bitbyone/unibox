package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tobola/unibox/internal/testenv"
	"github.com/tobola/unibox/internal/unibox"
)

// fakeSBX puts on PATH an sbx that keeps its secrets' names in a file and
// logs every call; a kit asks for anthropic.
func fakeSBX(t *testing.T) (log string) {
	t.Helper()
	dir := t.TempDir()
	log = filepath.Join(dir, "calls")
	secrets := filepath.Join(dir, "secrets")
	script := `#!/bin/sh
echo "$*" >> ` + unibox.Quote(log) + `
case "$1 $2" in
"settings get") echo '{"value":false}' ;;
"version ") echo "sbx version v$(cat ` + unibox.Quote(filepath.Join(dir, "version")) + ` 2>/dev/null || echo 0.49.2) 1234abc" ;;
"diagnose --format") echo '{"checks":[]}' ;;
"env plan")
	if [ -f ` + unibox.Quote(filepath.Join(dir, "plan-error")) + ` ]; then echo "Error: $(cat ` + unibox.Quote(filepath.Join(dir, "plan-error")) + `)" >&2; echo "  try: sbx env --help" >&2; exit 1; fi
	printf 'SANDBOX\n  + team\n\n  1 to add · 0 to change · 0 to destroy\n' ;;
"ls --json") echo '[]' ;;
"kit inspect") echo '{"kind":"sandbox","name":"agent","credentials":[{"service":"anthropic","description":"An Anthropic API key"}]}' ;;
"secret ls")
	printf '['; sep=''
	[ -f ` + unibox.Quote(secrets) + ` ] && while read -r n; do printf '%s{"scope":"global","type":"service","name":"%s"}' "$sep" "$n"; sep=,; done < ` + unibox.Quote(secrets) + `
	echo ']' ;;
"secret set") echo "$3" >> ` + unibox.Quote(secrets) + ` ;;
esac
`
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := testenv.WriteExecutable(filepath.Join(dir, "bin", "sbx"), []byte(script)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Join(dir, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

// teamEnv writes a team's env file with a kit and a binding.
func teamEnv(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "kits", "agent"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "sandboxes", "team.sbxenv.yaml")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("name: team\nargs:\n  user: {required: true, description: Your user name}\nkits: [../kits/agent]\nbindings:\n  gitlab:\n    apiKey: {domains: [gitlab.example.com]}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return file
}

// runAt runs unibox with its input in a file, as a terminal is: a command
// it starts shares the file, rather than being copied what is left of it.
func runAt(t *testing.T, paths unibox.Paths, input string, args ...string) (string, string, error) {
	t.Helper()
	name := filepath.Join(t.TempDir(), "input")
	if err := os.WriteFile(name, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out, log bytes.Buffer
	c := New(paths, f, &out, &log)
	c.SetArgs(args)
	err = c.Execute()
	return out.String(), log.String(), err
}

func calls(t *testing.T, log string) string {
	t.Helper()
	b, _ := os.ReadFile(log)
	return string(b)
}

func TestLoginsAreListedAndTheMissingOnesStoredBySbx(t *testing.T) {
	log := fakeSBX(t)
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	if _, _, err := run(t, paths, "", "env", "add", "team", teamEnv(t)); err != nil {
		t.Fatal(err)
	}
	out, _, err := run(t, paths, "", "login", "ls")
	if err != nil || !strings.Contains(out, "anthropic  no      yes     An Anthropic API key") || !strings.Contains(out, "gitlab     no      yes     the env's bindings") {
		t.Fatalf("%v\n%s", err, out)
	}
	// Away from a terminal sbx can be given one token, piped in.
	if _, _, err = run(t, paths, "", "login", "set"); err == nil || !strings.Contains(err.Error(), "pipe one service's token in") {
		t.Fatalf("%v", err)
	}
	if _, _, err = run(t, paths, "token", "login", "set", "gitlab"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = run(t, paths, "", "login", "set", "anthropic", "--command", "security find-generic-password -s anthropic -w"); err != nil {
		t.Fatal(err)
	}
	if c := calls(t, log); !strings.Contains(c, "secret set gitlab\n") || !strings.Contains(c, "secret set anthropic --command security find-generic-password -s anthropic -w\n") {
		t.Fatalf("%s", c)
	}
	if out, _, err = run(t, paths, "", "login", "set"); err != nil || !strings.Contains(out, "sbx has every token env team needs") {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, _, err = run(t, paths, "", "login", "set", "Not A Name"); err == nil {
		t.Fatal("a name sbx does not take was passed on")
	}
}

func TestDoctorChecksSbxsVersionAndPlansEachEnv(t *testing.T) {
	log := fakeSBX(t)
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	if _, _, err := run(t, paths, "", "env", "add", "team", teamEnv(t)); err != nil {
		t.Fatal(err)
	}
	// An env is planned only once its args are given.
	out, _, _ := run(t, paths, "", "doctor")
	if !strings.Contains(out, "0.49.2") || !strings.Contains(out, "sbx asks before an env's host commands run") || strings.Contains(calls(t, log), "env plan") {
		t.Fatalf("%s", out)
	}
	if _, _, err := run(t, paths, "", "var", "set", "-e", "team", "user", "me"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, paths, "token", "login", "set", "gitlab"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, paths, "token", "login", "set", "anthropic"); err != nil {
		t.Fatal(err)
	}
	if out, _, err := run(t, paths, "", "doctor"); err != nil || !strings.Contains(out, "1 to add · 0 to change · 0 to destroy") {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(calls(t, log), "env plan --name team --env-args-file ") {
		t.Fatalf("%s", calls(t, log))
	}
	answer := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(filepath.Dir(log), name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	answer("plan-error", "kit ../kits/agent: no such kit")
	if out, _, err := run(t, paths, "", "doctor"); err == nil || !strings.Contains(out, "kit ../kits/agent: no such kit") || strings.Contains(out, "try: sbx") {
		t.Fatalf("%v\n%s", err, out)
	}
	if err := os.Remove(filepath.Join(filepath.Dir(log), "plan-error")); err != nil {
		t.Fatal(err)
	}
	answer("version", "0.48.0")
	if out, _, err := run(t, paths, "", "doctor"); err == nil || !strings.Contains(out, "0.48.0, older than the "+unibox.MinSBXVersion) {
		t.Fatalf("%v\n%s", err, out)
	}
}

func TestInitWalksThroughAnEnvItsArgsAndLogins(t *testing.T) {
	log := fakeSBX(t)
	t.Setenv("ACCESSIBLE", "1")
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	file := teamEnv(t)
	// SSH agent: no; the file; its name as the file says; the arg; store
	// the logins; no sandbox yet.
	_, said, err := runAt(t, paths, "1\n"+file+"\n\nperson\ny\nn\n", "init")
	if err != nil {
		t.Fatalf("%v\n%s", err, said)
	}
	cfg, _ := paths.LoadConfig()
	if on, chosen := cfg.SSHAgent(); on || !chosen {
		t.Fatal("the SSH agent was not decided")
	}
	if cfg.DefaultEnv != "team" || len(cfg.Envs["team"].Files) != 1 {
		t.Fatalf("%+v", cfg)
	}
	values, _ := paths.LoadEnvValues()
	if values.Envs["team"]["user"] != "person" {
		t.Fatalf("%+v", values)
	}
	if c := calls(t, log); !strings.Contains(c, "secret set anthropic\n") || !strings.Contains(c, "secret set gitlab\n") || strings.Contains(c, "env create") {
		t.Fatalf("%s", c)
	}
	if !strings.Contains(said, "unibox run creates it when it first needs it") {
		t.Fatalf("%s", said)
	}
	// Again, everything is in place and only said.
	if _, said, err = runAt(t, paths, "1\nn\n", "init"); err != nil || !strings.Contains(said, "has every value it needs") || !strings.Contains(said, "sbx has every token") {
		t.Fatalf("%v\n%s", err, said)
	}
}

func TestAMissingArgIsAskedForBeforeTheSandboxIsCreated(t *testing.T) {
	log := fakeSBX(t)
	t.Setenv("ACCESSIBLE", "1")
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	if _, _, err := run(t, paths, "", "env", "add", "team", teamEnv(t)); err != nil {
		t.Fatal(err)
	}
	// The stand-in creates nothing, so creation fails after it is asked.
	_, said, _ := runAt(t, paths, "person\n", "sandbox", "create")
	if values, _ := paths.LoadEnvValues(); values.Envs["team"]["user"] != "person" {
		t.Fatalf("%+v\n%s", values, said)
	}
	if !strings.Contains(calls(t, log), "env create --name team") {
		t.Fatalf("%s", calls(t, log))
	}
}
