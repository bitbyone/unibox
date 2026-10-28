package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/tobola/unibox/internal/testenv"
	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
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
"settings get")
	if [ "$3" = feature.ssh ] && [ -f ` + unibox.Quote(filepath.Join(dir, "editor-ssh")) + ` ]; then echo '{"value":{"enabled":true}}'; else echo '{"value":false}'; fi ;;
"version ") echo "sbx version v$(cat ` + unibox.Quote(filepath.Join(dir, "version")) + ` 2>/dev/null || echo 0.50.0) 1234abc" ;;
"diagnose --format")
	if [ -f ` + unibox.Quote(filepath.Join(dir, "policy-unchosen")) + ` ]; then echo '{"checks":[{"name":"Global network policy","status":"fail","message":"not initialized"}]}'; exit 1; fi
	echo '{"checks":[]}' ;;
"policy init") rm -f ` + unibox.Quote(filepath.Join(dir, "policy-unchosen")) + ` ;;
"env plan")
	if [ -f ` + unibox.Quote(filepath.Join(dir, "plan-error")) + ` ]; then echo "Error: $(cat ` + unibox.Quote(filepath.Join(dir, "plan-error")) + `)" >&2; echo "  try: sbx env --help" >&2; exit 1; fi
	printf 'SANDBOX\n  + team\n\n  1 to add · 0 to change · 0 to destroy\n' ;;
"ls --json") cat ` + unibox.Quote(filepath.Join(dir, "boxes")) + ` 2>/dev/null || echo '[]' ;;
"policy log") echo '{"allowed":[],"blocked":[{"host":"pypi.org","reason":"no rule","count":3,"last_seen":"2025-01-01T10:00:00Z"},{"host":"x.org.dev.docker.internal","count":1}]}' ;;
"policy ls") echo '{"rules":[{"id":"1","name":"kit:team","applies_to":"all","decision":"allow","resource_type":"network","resources":["gitlab.example.com"],"status":"active","provenance":{"created_via":"provisioned"}},{"id":"2","applies_to":"sandbox:dev","decision":"deny","resource_type":"network","resources":["ads.example.com"],"status":"active","editable":true,"provenance":{"created_via":"added"}},{"id":"3","applies_to":"all","decision":"allow","resource_type":"filesystem:read","resources":["/"]}]}' ;;
"kit inspect")
	if [ -f ` + unibox.Quote(filepath.Join(dir, "kit-oauth")) + ` ]; then echo '{"kind":"sandbox","name":"agent","credentials":[{"service":"anthropic","oauth":{"tokenEndpoint":"x"}},{"service":"openai","oauth":{"tokenEndpoint":"x"}}]}'; elif [ -f ` + unibox.Quote(filepath.Join(dir, "kit-claude")) + ` ]; then echo '{"kind":"sandbox","name":"agent","sandbox":{"entrypoint":["claude"]},"credentials":[{"service":"anthropic","description":"An Anthropic API key"}]}'; else echo '{"kind":"sandbox","name":"agent","credentials":[{"service":"anthropic","description":"An Anthropic API key"}]}'; fi ;;
"secret ls")
	printf '{"secrets":['; sep=''
	[ -f ` + unibox.Quote(secrets) + ` ] && while read -r n; do printf '%s{"scope":"global","type":"service","name":"%s"}' "$sep" "$n"; sep=,; done < ` + unibox.Quote(secrets) + `
	printf '],"custom_secrets":['; sep=''
	[ -f ` + unibox.Quote(secrets+".custom") + ` ] && while read -r e p; do printf '%s{"scope":"global","env":"%s","placeholder":"%s"}' "$sep" "$e" "$p"; sep=,; done < ` + unibox.Quote(secrets+".custom") + `
	echo ']}' ;;
"secret set-custom")
	if [ -f ` + unibox.Quote(filepath.Join(dir, "secret-fails")) + ` ]; then echo "error: secret entry cancelled" >&2; exit 1; fi
	echo "$4 $6" >> ` + unibox.Quote(secrets+".custom") + ` ;;
"secret set")
	if [ -f ` + unibox.Quote(filepath.Join(dir, "secret-fails")) + ` ]; then echo "error: secret entry cancelled" >&2; exit 1; fi
	echo "$3" >> ` + unibox.Quote(secrets) + ` ;;
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

func TestSecretsAreListedAndTheMissingOnesStoredBySbx(t *testing.T) {
	log := fakeSBX(t)
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	if _, _, err := run(t, paths, "", "env", "add", "team", teamEnv(t)); err != nil {
		t.Fatal(err)
	}
	out, _, err := run(t, paths, "", "secret", "ls")
	if err != nil || !strings.Contains(out, "anthropic  no      yes     An Anthropic API key") || !strings.Contains(out, "gitlab     no      yes     the env's bindings") {
		t.Fatalf("%v\n%s", err, out)
	}
	// Away from a terminal sbx can be given one token, piped in.
	if _, _, err = run(t, paths, "", "secret", "set"); err == nil || !strings.Contains(err.Error(), "pipe one service's token in") {
		t.Fatalf("%v", err)
	}
	if _, _, err = run(t, paths, "token", "secret", "set", "gitlab"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = run(t, paths, "", "secret", "set", "anthropic", "--command", "security find-generic-password -s anthropic -w"); err != nil {
		t.Fatal(err)
	}
	if c := calls(t, log); !strings.Contains(c, "secret set gitlab\n") || !strings.Contains(c, "secret set anthropic --command security find-generic-password -s anthropic -w\n") {
		t.Fatalf("%s", c)
	}
	if out, _, err = run(t, paths, "", "secret", "set"); err != nil || !strings.Contains(out, "sbx has every token env team needs") {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, _, err = run(t, paths, "", "secret", "set", "Not A Name"); err == nil {
		t.Fatal("a name sbx does not take was passed on")
	}
}

func TestSettingATokenOffersThePageWhereANewOneIsMade(t *testing.T) {
	log := fakeSBX(t)
	t.Setenv("ACCESSIBLE", "1")
	// The browser is a stand-in that notes the page it is given.
	bin, opened := t.TempDir(), filepath.Join(t.TempDir(), "opened")
	for _, name := range []string{"open", "xdg-open"} {
		if err := testenv.WriteExecutable(filepath.Join(bin, name), []byte("#!/bin/sh\necho \"$1\" >> "+unibox.Quote(opened)+"\n")); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	file := teamEnv(t)
	page := "https://gitlab.example.com/-/user_settings/personal_access_tokens?name=unibox"
	if err := os.WriteFile(unibox.TeamFile(file), []byte("secrets:\n  gitlab:\n    page: "+page+"\n    hint: read and write repository\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, paths, "", "env", "add", "team", file); err != nil {
		t.Fatal(err)
	}
	// Asked whether there is a token, the person makes a new one.
	if _, said, err := runAt(t, paths, "2\n", "secret", "set", "gitlab"); err != nil || !strings.Contains(said, "read and write repository") {
		t.Fatalf("%v\n%s", err, said)
	}
	if b, _ := os.ReadFile(opened); string(b) != page+"\n" || !strings.Contains(calls(t, log), "secret set gitlab\n") {
		t.Fatalf("%q\n%s", b, calls(t, log))
	}
	// A service whose page is not known goes straight to sbx, and cannot
	// be generated.
	if _, _, err := runAt(t, paths, "", "secret", "set", "npm"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runAt(t, paths, "", "secret", "set", "npm", "--generate"); err == nil || !strings.Contains(err.Error(), "knows no way") {
		t.Fatalf("%v", err)
	}
	if out, _, err := run(t, paths, "", "secret", "ls", "--json"); err != nil || !strings.Contains(out, `"page": "`+page+`"`) {
		t.Fatalf("%v\n%s", err, out)
	}
	// A token made more ways than one asks which; a subscription's is a
	// command the person runs, which unibox only names.
	if err := os.WriteFile(unibox.TeamFile(file), []byte("secrets:\n  anthropic:\n    ways:\n      - {name: Claude subscription, command: claude setup-token}\n      - {name: API key, page: https://console.example.com/keys}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, said, err := runAt(t, paths, "1\n", "secret", "set", "anthropic"); err != nil || !strings.Contains(said, "Claude subscription") || !strings.Contains(said, "run claude setup-token in another terminal") {
		t.Fatalf("%v\n%s", err, said)
	}
	if strings.Contains(calls(t, log), "setup-token") {
		t.Fatal("unibox ran the command")
	}
	if err := os.WriteFile(unibox.TeamFile(file), []byte("secrets:\n  gitlab: {page: gitlab.example.com}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, paths, "", "secret", "ls"); err == nil || !strings.Contains(err.Error(), "no web address") {
		t.Fatalf("%v", err)
	}
}

func TestATokenSbxDidNotStoreSaysHowToGoOn(t *testing.T) {
	log := fakeSBX(t)
	t.Setenv("ACCESSIBLE", "1")
	if err := os.WriteFile(filepath.Join(filepath.Dir(log), "secret-fails"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	repo := filepath.Dir(filepath.Dir(teamEnv(t)))
	// sbx has said why on the terminal; its exit status is not repeated.
	_, _, err := runAt(t, paths, "1\n2\n1\nperson\ny\n2\n", "init", repo)
	if err == nil || err.Error() != "sbx did not store the anthropic token; run unibox init again to go on where you left off, or unibox secret to set the tokens still missing" {
		t.Fatalf("%v", err)
	}
	if _, _, err = runAt(t, paths, "2\n", "secret", "set", "anthropic"); err == nil || err.Error() != "sbx did not store the anthropic token; run unibox secret to set the tokens still missing" {
		t.Fatalf("%v", err)
	}
}

func TestTheNetworkPolicyIsChosenBeforeASandboxIsCreated(t *testing.T) {
	log := fakeSBX(t)
	unchosen := filepath.Join(filepath.Dir(log), "policy-unchosen")
	if err := os.WriteFile(unchosen, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	if _, _, err := run(t, paths, "", "env", "add", "team", teamEnv(t)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, paths, "", "var", "set", "user", "person", "-e", "team"); err != nil {
		t.Fatal(err)
	}
	// Away from a terminal the sandbox is refused, naming what chooses it.
	if _, _, err := run(t, paths, "", "sandbox", "create", "-e", "team"); err == nil || !strings.Contains(err.Error(), "unibox network init") || strings.Contains(calls(t, log), "env create") {
		t.Fatalf("%v\n%s", err, calls(t, log))
	}
	if _, _, err := run(t, paths, "", "network", "init"); err == nil || !strings.Contains(err.Error(), "unibox network init balanced, allow-all, deny-all") {
		t.Fatalf("%v", err)
	}
	if _, _, err := run(t, paths, "", "network", "init", "open"); err == nil || !strings.Contains(err.Error(), "no network policy sbx has") {
		t.Fatalf("%v", err)
	}
	// At a terminal it is asked, balanced first, and sbx goes on to create
	// the sandbox, which the stand-in does not list.
	t.Setenv("ACCESSIBLE", "1")
	if _, _, err := runAt(t, paths, "1\n", "sandbox", "create", "-e", "team"); err != nil && strings.Contains(err.Error(), "network") {
		t.Fatal(err)
	}
	if c := calls(t, log); !strings.Contains(c, "policy init balanced\n") || !strings.Contains(c, "env create") {
		t.Fatalf("%s", c)
	}
	if _, _, err := run(t, paths, "", "network", "init", "deny-all"); err == nil || !strings.Contains(err.Error(), "chosen already") {
		t.Fatalf("%v", err)
	}
}

func TestDoctorOffersToRunTheFixOfEachProblem(t *testing.T) {
	log := fakeSBX(t)
	if err := os.WriteFile(filepath.Join(filepath.Dir(log), "policy-unchosen"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	if _, _, err := run(t, paths, "", "doctor", "-i"); err == nil || !strings.Contains(err.Error(), "doctor -i asks at a terminal") {
		t.Fatalf("%v", err)
	}
	t.Setenv("ACCESSIBLE", "1")
	// Yes to running unibox network init, which asks the policy.
	_, said, err := runAt(t, paths, "y\n1\n", "doctor", "-i")
	if err != nil || !strings.Contains(said, "Run unibox network init?") || !strings.Contains(calls(t, log), "policy init balanced\n") {
		t.Fatalf("%v\n%s\n%s", err, said, calls(t, log))
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
	if !strings.Contains(out, "0.50.0") || !strings.Contains(out, "sbx asks before an env's host commands run") || strings.Contains(calls(t, log), "env plan") {
		t.Fatalf("%s", out)
	}
	if _, _, err := run(t, paths, "", "var", "set", "-e", "team", "user", "me"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, paths, "token", "secret", "set", "gitlab"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, paths, "token", "secret", "set", "anthropic"); err != nil {
		t.Fatal(err)
	}
	if out, _, err := run(t, paths, "", "doctor"); err != nil || !strings.Contains(out, "1 to add · 0 to change · 0 to destroy") {
		t.Fatalf("%v\n%s", err, out)
	}
	// A sandbox made from what the env no longer is is told.
	if err := paths.State(func(s *unibox.State) error {
		s.Sandboxes["team"] = api.Sandbox{Name: "team", Env: "team", Fingerprint: "before", State: "ready"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if out, _, _ := run(t, paths, "", "doctor"); !strings.Contains(out, "the env changed since the sandbox was created") {
		t.Fatalf("%s", out)
	}
	// One yes in doctor -i recreates it, the cost said in that question
	// and not asked again.
	t.Setenv("ACCESSIBLE", "1")
	_, said, _ := runAt(t, paths, "y\n", "doctor", "-i", "-e", "team")
	if !strings.Contains(said, "Run unibox sandbox recreate team? It discards what the sandbox holds outside its tasks.") || strings.Contains(said, "[y/N]") || !strings.Contains(calls(t, log), "env create") {
		t.Fatalf("%s\n%s", said, calls(t, log))
	}
	t.Setenv("ACCESSIBLE", "")
	if err := paths.State(func(s *unibox.State) error { delete(s.Sandboxes, "team"); return nil }); err != nil {
		t.Fatal(err)
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

func TestInitWalksThroughAnEnvItsArgsAndSecrets(t *testing.T) {
	log := fakeSBX(t)
	t.Setenv("ACCESSIBLE", "1")
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	repo := filepath.Dir(filepath.Dir(teamEnv(t)))
	// SSH agent: no; the one file is taken without asking; the arg; store
	// the secrets; no sandbox yet.
	_, said, err := runAt(t, paths, "1\n1\n1\nperson\ny\n2\nn\n", "init", repo)
	// The only env is what commands take already, so nothing is asked.
	if err != nil || strings.Contains(said, "as the current context") {
		t.Fatalf("%v\n%s", err, said)
	}
	// Asked, the env is what commands work in; its sandbox once made.
	if cfg, _ := paths.LoadConfig(); cfg.Context != (unibox.Context{Env: "team"}) {
		t.Fatalf("%+v", cfg.Context)
	}
	cfg, _ := paths.LoadConfig()
	if on, chosen := cfg.SSHAgent(); on || !chosen {
		t.Fatal("the SSH agent was not decided")
	}
	if on, chosen := cfg.EditorSSH(); !on || !chosen {
		t.Fatal("editors over SSH were not decided")
	}
	// No kit runs claude, so its remote control is not asked about.
	if r := cfg.RememberHostCommands; r == nil || *r || cfg.ClaudeRemoteControl != nil {
		t.Fatalf("%v %v", r, cfg.ClaudeRemoteControl)
	}
	if cfg.DefaultEnv != "team" || len(cfg.Envs["team"].Files) != 1 {
		t.Fatalf("%+v", cfg)
	}
	values, _ := paths.LoadEnvValues()
	if values.Envs["team"]["user"] != "person" {
		t.Fatalf("%+v", values)
	}
	if c := calls(t, log); !strings.Contains(c, "secret set anthropic\n") || !strings.Contains(c, "secret set gitlab\n") || strings.Contains(c, "env create") ||
		!strings.Contains(c, "settings set feature.ssh true\ndaemon restart\nsetup ssh\n") {
		t.Fatalf("%s", c)
	}
	if !strings.Contains(said, "env team, sandboxes/team.sbxenv.yaml: added") || !strings.Contains(said, "unibox run creates team when it first needs it") {
		t.Fatalf("%s", said)
	}
	// Again, everything is in place and only said.
	if _, said, err = runAt(t, paths, "n\n", "init", repo); err != nil || !strings.Contains(said, "set up already") || !strings.Contains(said, "has every value it needs") || !strings.Contains(said, "sbx has every token") {
		t.Fatalf("%v\n%s", err, said)
	}
}

func TestInitTakesEditorsOverSSHThatSbxHasOnAlready(t *testing.T) {
	log := fakeSBX(t)
	if err := os.WriteFile(filepath.Join(filepath.Dir(log), "editor-ssh"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ACCESSIBLE", "1")
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	repo := filepath.Dir(filepath.Dir(teamEnv(t)))
	// SSH agent: no; the arg; store the secrets; no sandbox yet - and no
	// question about editors.
	if _, said, err := runAt(t, paths, "1\n1\nperson\ny\n2\nn\n", "init", repo); err != nil || strings.Contains(said, "Editors over SSH") {
		t.Fatalf("%v\n%s", err, said)
	}
	if cfg, _ := paths.LoadConfig(); func() bool { on, said := cfg.EditorSSH(); return !on || !said }() {
		t.Fatal("editors over SSH were not taken from sbx")
	}
	if strings.Contains(calls(t, log), "settings set feature.ssh") {
		t.Fatal("sbx's setting was set again")
	}
}

func TestAnAgentsTokenIsASubscriptionsOrAnAPIKey(t *testing.T) {
	log := fakeSBX(t)
	if err := os.WriteFile(filepath.Join(filepath.Dir(log), "kit-oauth"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ACCESSIBLE", "1")
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	repo := filepath.Dir(filepath.Dir(teamEnv(t)))
	// SSH agent and editors: no; the arg; store the tokens: Claude's of
	// the subscription, Codex's by signing in, gitlab's typed; no sandbox.
	_, said, err := runAt(t, paths, "1\n2\n1\nperson\ny\n1\n1\nn\ny\n", "init", repo)
	if err != nil || !strings.Contains(said, "run /login in it the first time it starts") {
		t.Fatalf("%v\n%s", err, said)
	}
	// Claude's subscription signs in inside Claude: nothing is stored for
	// it; Codex's is signed in by sbx.
	c := calls(t, log)
	if strings.Contains(c, "secret set anthropic") || strings.Contains(c, "set-custom") || !strings.Contains(c, "secret set openai --oauth\n") {
		t.Fatalf("%s", c)
	}
	// unibox secret rm has sbx forget a service's token.
	if _, _, err := run(t, paths, "", "secret", "rm", "openai", "-e", "team"); err != nil || !strings.Contains(calls(t, log), "secret rm openai --force") {
		t.Fatalf("%v\n%s", err, calls(t, log))
	}
	// An API key is typed at sbx's prompt.
	if _, _, err := runAt(t, paths, "2\n", "secret", "set", "openai"); err != nil || !strings.Contains(calls(t, log), "secret set openai\n") {
		t.Fatalf("%v\n%s", err, calls(t, log))
	}
}

func TestInitAwayFromARepositoryAsksForTheFile(t *testing.T) {
	fakeSBX(t)
	t.Setenv("ACCESSIBLE", "1")
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	file := teamEnv(t)
	// SSH agent: no; the file; its name as it says; the arg; no secrets
	// stored; no sandbox.
	_, said, err := runAt(t, paths, "1\n2\n1\n"+file+"\n\nperson\nn\nn\n", "init", t.TempDir())
	if err != nil || !strings.Contains(said, "no sbxenv file in") || !strings.Contains(said, "later: unibox secret set -e team") {
		t.Fatalf("%v\n%s", err, said)
	}
	if cfg, _ := paths.LoadConfig(); cfg.DefaultEnv != "team" {
		t.Fatalf("%+v", cfg)
	}
}

// teamRepo is a team's repository with two envs, a part of it where init
// is run, and files init passes over.
func teamRepo(t *testing.T) (repo, part string) {
	t.Helper()
	repo = t.TempDir()
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	for file, text := range map[string]string{
		"sandboxes/dev.sbxenv.yaml":            "agent: shell\nargs:\n  user: {required: true}\n",
		"review/Review_Box.sbxenv.yaml":        "agent: shell\n",
		"node_modules/pkg/ignored.sbxenv.yaml": "agent: shell\n",
		".cache/ignored.sbxenv.yaml":           "agent: shell\n",
		"sandboxes/notes/readme.md":            "",
	} {
		path := filepath.Join(repo, file)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return repo, filepath.Join(repo, "sandboxes", "notes")
}

func TestInitFindsEveryEnvOfTheRepository(t *testing.T) {
	fakeSBX(t)
	t.Setenv("ACCESSIBLE", "1")
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	repo, part := teamRepo(t)
	t.Chdir(part)
	// SSH agent: no; both files as offered; review's name, as its own does
	// not make one; dev as the default, review being added first; dev's arg;
	// no sandboxes.
	_, said, err := runAt(t, paths, "1\n2\n1\n0\nreview\n2\nme\nn\nn\n", "init")
	if err != nil {
		t.Fatalf("%v\n%s", err, said)
	}
	cfg, _ := paths.LoadConfig()
	resolved := func(path string) string {
		path, _ = filepath.EvalSymlinks(path)
		return path
	}
	if len(cfg.Envs) != 2 || resolved(cfg.Envs["dev"].Files[0]) != resolved(filepath.Join(repo, "sandboxes", "dev.sbxenv.yaml")) || cfg.Envs["review"].Repo == "" || cfg.DefaultEnv != "dev" {
		t.Fatalf("%+v\n%s", cfg, said)
	}
	if strings.Contains(said, "ignored") {
		t.Fatalf("%s", said)
	}
}

func TestInitWithoutInputSetsUpEveryFileOrFails(t *testing.T) {
	fakeSBX(t)
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	repo, _ := teamRepo(t)
	// The file whose name makes no env is left out; dev lacks its value.
	_, _, err := run(t, paths, "", "init", repo, "--no-input", "--file", "sandboxes/dev.sbxenv.yaml")
	if err == nil || !strings.Contains(err.Error(), "--var user=<value> (env dev)") {
		t.Fatalf("%v", err)
	}
	if _, _, err = run(t, paths, "", "init", repo, "--file", "sandboxes/dev.sbxenv.yaml", "--var", "nobody=x"); err == nil || !strings.Contains(err.Error(), "no env of dev asks for nobody") {
		t.Fatalf("%v", err)
	}
	out, _, err := run(t, paths, "", "init", repo, "--file", "sandboxes/dev.sbxenv.yaml", "--var", "user=me")
	if err != nil || !strings.Contains(out, "set up env(s) dev") {
		t.Fatalf("%v\n%s", err, out)
	}
	if values, _ := paths.LoadEnvValues(); values.Envs["dev"]["user"] != "me" {
		t.Fatalf("%+v", values)
	}
	// Every file found: one cannot name an env, which says how to add it.
	if _, _, err = run(t, paths, "", "init", repo); err == nil || !strings.Contains(err.Error(), "unibox env add <env>") {
		t.Fatalf("%v", err)
	}
}

func TestASecondTeamsSandboxOfTheSameNameIsNamedByThePerson(t *testing.T) {
	fakeSBX(t)
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	first, _ := teamRepo(t)
	if _, _, err := run(t, paths, "", "init", first, "--file", "sandboxes/dev.sbxenv.yaml", "--var", "user=me"); err != nil {
		t.Fatal(err)
	}
	// Another team's env names its sandbox dev too.
	second := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", second).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	file := filepath.Join(second, "sandboxes", "web.sbxenv.yaml")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("name: dev\nagent: shell\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Without a terminal init says how; at one it asks the env's name, then
	// the sandbox's, and the env keeps both.
	if _, _, err := run(t, paths, "", "init", second, "--no-input"); err == nil || !strings.Contains(err.Error(), "unibox env add <env>") {
		t.Fatalf("%v", err)
	}
	t.Setenv("ACCESSIBLE", "1")
	if _, said, err := runAt(t, paths, "1\n1\n1\nweb\nweb-dev\nn\n", "init", second); err != nil || !strings.Contains(said, "Sandbox name for env web") || !strings.Contains(said, "env web, sandboxes/web.sbxenv.yaml: added") {
		t.Fatalf("%v\n%s", err, said)
	}
	if cfg, _ := paths.LoadConfig(); cfg.Envs["web"].Name != "web-dev" {
		t.Fatalf("%+v", cfg.Envs["web"])
	}
	if _, _, err := run(t, paths, "", "init", second, "--no-input"); err != nil {
		t.Fatal(err)
	}
}

func TestInitClonesARepositoryGivenByAddress(t *testing.T) {
	fakeSBX(t)
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir(), Data: t.TempDir()}
	repo, _ := teamRepo(t)
	git := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}
	git("add", ".")
	git("commit", "-qm", "envs")
	address := "file://" + filepath.ToSlash(repo)
	out, said, err := run(t, paths, "", "init", address, "--file", "sandboxes/dev.sbxenv.yaml", "--var", "user=me")
	if err != nil || !strings.Contains(said, "cloned "+address) || !strings.Contains(out, "set up env(s) dev") {
		t.Fatalf("%v\n%s\n%s", err, out, said)
	}
	dir, _ := paths.CloneDir(address)
	cfg, _ := paths.LoadConfig()
	if dev := cfg.Envs["dev"]; dev.Source != address || dev.Files[0] != filepath.Join(dir, "sandboxes", "dev.sbxenv.yaml") {
		t.Fatalf("%+v", dev)
	}
	// Again, the clone there is is used.
	if _, said, err = run(t, paths, "", "init", address, "--file", "sandboxes/dev.sbxenv.yaml"); err != nil || !strings.Contains(said, "cloned in "+dir+" already") {
		t.Fatalf("%v\n%s", err, said)
	}
	// The team changes its repository: doctor sees the clone behind, and
	// pull says what follows.
	if err := os.WriteFile(filepath.Join(repo, "sandboxes", "dev.sbxenv.yaml"), []byte("agent: shell\nargs:\n  user: {required: true}\n  mirror: {required: true}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "extra"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "extra", "new.sbxenv.yaml"), []byte("agent: shell\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-qm", "mirror")
	if out, _, _ = run(t, paths, "", "doctor"); !strings.Contains(out, "1 commit(s) behind its remote") {
		t.Fatalf("%s", out)
	}
	out, _, err = run(t, paths, "", "env", "update")
	if err != nil || !strings.Contains(out, dir+": updated") || !strings.Contains(out, "env dev now asks for mirror") || !strings.Contains(out, "new sbxenv file(s) extra/new.sbxenv.yaml") {
		t.Fatalf("%v\n%s", err, out)
	}
	if out, _, err = run(t, paths, "", "env", "update", "dev"); err != nil || !strings.Contains(out, ": up to date") {
		t.Fatalf("%v\n%s", err, out)
	}
	if out, _, _ = run(t, paths, "", "doctor"); !strings.Contains(out, "as its remote") {
		t.Fatalf("%s", out)
	}
	// A repository of the user's own is pulled while it is clean and
	// follows a remote branch, and left as it is otherwise.
	if _, _, err = run(t, paths, "", "env", "add", "mine", filepath.Join(repo, "extra", "new.sbxenv.yaml")); err != nil {
		t.Fatal(err)
	}
	if out, _, err = run(t, paths, "", "env", "update", "mine"); err != nil || !strings.Contains(out, repo+": not updated, its branch follows no remote branch") {
		t.Fatalf("%v\n%s", err, out)
	}
	own := filepath.Join(t.TempDir(), "own")
	if out, err := exec.Command("git", "clone", "-q", repo, own).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if _, _, err = run(t, paths, "", "env", "add", "own", filepath.Join(own, "extra", "new.sbxenv.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "extra", "new.sbxenv.yaml"), []byte("agent: shell\n# changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("commit", "-qam", "changed")
	if out, _, err = run(t, paths, "", "env", "update", "own"); err != nil || !strings.Contains(out, own+": updated") {
		t.Fatalf("%v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(own, "extra", "new.sbxenv.yaml"), []byte("agent: shell\n# mine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, _, err = run(t, paths, "", "env", "update", "own"); err != nil || !strings.Contains(out, own+": not updated, it has changes not committed") {
		t.Fatalf("%v\n%s", err, out)
	}
	// An address that cannot be cloned leaves nothing behind.
	missing := "file://" + filepath.ToSlash(filepath.Join(t.TempDir(), "nothing"))
	if _, _, err = run(t, paths, "", "init", missing); err == nil || !strings.Contains(err.Error(), "git could not clone") {
		t.Fatalf("%v", err)
	}
	if dir, _ := paths.CloneDir(missing); fileExists(dir) {
		t.Fatalf("%s was left", dir)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestEnvAddTakesTheOnlyFileHere(t *testing.T) {
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	repo, _ := teamRepo(t)
	t.Chdir(filepath.Join(repo, "sandboxes"))
	if out, _, err := run(t, paths, "", "env", "add"); err != nil || !strings.Contains(out, "env dev: sandbox dev") {
		t.Fatalf("%v\n%s", err, out)
	}
	t.Chdir(repo)
	if _, _, err := run(t, paths, "", "env", "add", "x"); err == nil || !strings.Contains(err.Error(), "no sbxenv file in this directory") {
		t.Fatalf("%v", err)
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

func TestASandboxCreatedWithResourcesKeepsThemInVarsToml(t *testing.T) {
	log := fakeSBX(t)
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	t.Setenv("ACCESSIBLE", "1")
	if _, _, err := run(t, paths, "", "env", "add", "dev", teamEnv(t)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, paths, "", "sandbox", "create", "big", "-e", "dev", "--cpus", "lots"); err == nil || !strings.Contains(err.Error(), "cpus is a number") {
		t.Fatalf("%v", err)
	}
	if values, _ := paths.LoadEnvValues(); len(values.SandboxResources) != 0 {
		t.Fatalf("%+v", values)
	}
	// The stand-in creates nothing; what matters is what was written first.
	runAt(t, paths, "person\n", "sandbox", "create", "big", "-e", "dev", "--cpus", "4", "--memory", "50%")
	if values, _ := paths.LoadEnvValues(); values.SandboxResources["big"] != (unibox.Resources{CPUs: "4", Memory: "50%"}) {
		t.Fatalf("%+v", values)
	}
	if !strings.Contains(calls(t, log), "--name big") {
		t.Fatalf("%s", calls(t, log))
	}
	out, _, err := run(t, paths, "", "sandbox", "resources", "big", "--memory", "-", "--cpus", "25%")
	if err != nil || !strings.Contains(out, "cpus    25% (its own)") || !strings.Contains(out, "memory  as the env's files say") {
		t.Fatalf("%v\n%s", err, out)
	}
	if values, _ := paths.LoadEnvValues(); values.SandboxResources["big"] != (unibox.Resources{CPUs: "25%"}) {
		t.Fatalf("%+v", values)
	}
	if _, _, err := run(t, paths, "", "sandbox", "resources", "nope", "--cpus", "2"); err == nil || !strings.Contains(err.Error(), "no sandbox nope") {
		t.Fatalf("%v", err)
	}
	if err := checkArg("cpus", "50%"); err == nil || !strings.Contains(err.Error(), "unibox sandbox resources --cpus 50%") {
		t.Fatalf("%v", err)
	}
	if err := checkArg("cpus", "4"); err != nil {
		t.Fatal(err)
	}
}

func TestNetworkRulesAreKeptForTheSandbox(t *testing.T) {
	log := fakeSBX(t)
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	t.Setenv("UNIBOX_SANDBOX", "dev")
	// Before the sandbox exists the rule is only kept.
	if out, _, err := run(t, paths, "", "network", "allow", "registry.npmjs.org", "*.npmjs.org"); err != nil || !strings.Contains(out, "sandbox dev may reach registry.npmjs.org, *.npmjs.org; it applies once the sandbox is created") {
		t.Fatalf("%v\n%s", err, out)
	}
	if strings.Contains(calls(t, log), "policy allow") {
		t.Fatalf("a rule reached sbx before its sandbox:\n%s", calls(t, log))
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(log), "boxes"), []byte(`[{"name":"dev","status":"running"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, _, err := run(t, paths, "", "network", "allow", "stun.example.com:3478", "--protocol", "udp"); err != nil || strings.Contains(out, "once the sandbox") {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, _, err := run(t, paths, "", "network", "deny", "ads.example.com"); err != nil {
		t.Fatal(err)
	}
	config, err := paths.LoadConfig()
	if err != nil || len(config.Sandboxes["dev"].Network) != 4 {
		t.Fatalf("%v %+v", err, config.Sandboxes)
	}
	for _, bad := range [][]string{{"network", "allow", "a,b"}, {"network", "allow", "--", "-x"}, {"network", "allow", "a", "--protocol", "icmp"}} {
		if _, _, err := run(t, paths, "", bad...); err == nil {
			t.Fatalf("%v went through", bad)
		}
	}
	out, _, err := run(t, paths, "", "network", "ls")
	if err != nil || !strings.Contains(out, "stun.example.com:3478") || !strings.Contains(out, "udp") || !strings.Contains(out, "pypi.org") || strings.Contains(out, "docker.internal") || strings.Contains(out, "gitlab.example.com") {
		t.Fatalf("%v\n%s", err, out)
	}
	if out, _, err := run(t, paths, "", "network", "ls", "--all"); err != nil || !strings.Contains(out, "gitlab.example.com") {
		t.Fatalf("%v\n%s", err, out)
	}
	// What the env lacks is written for a kit, the refused hosts to
	// uncomment; the kit's own host is left out.
	out, _, err = run(t, paths, "", "network", "export")
	var kit []struct {
		Type   string
		Config struct {
			Runtime struct{ Allow, Deny []string }
		}
	}
	if err != nil || yaml.Unmarshal([]byte(out), &kit) != nil || len(kit) != 1 || kit[0].Type != "com.docker.sandbox/network-policy@2" ||
		strings.Join(kit[0].Config.Runtime.Allow, " ") != "registry.npmjs.org *.npmjs.org stun.example.com:3478" || strings.Join(kit[0].Config.Runtime.Deny, " ") != "ads.example.com" ||
		!strings.Contains(out, "# - pypi.org  # refused 3 times") || !strings.Contains(out, "udp; a kit's host is any protocol") || strings.Contains(out, "gitlab.example.com") {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, _, err := run(t, paths, "", "network", "rm", "ads.example.com"); err != nil {
		t.Fatal(err)
	}
	if config, _ := paths.LoadConfig(); len(config.Sandboxes["dev"].Network) != 3 {
		t.Fatalf("%+v", config.Sandboxes)
	}
	got := calls(t, log)
	for _, want := range []string{"policy allow network --sandbox dev --protocol udp stun.example.com:3478", "policy allow network --sandbox dev registry.npmjs.org\n", "policy allow network --sandbox dev *.npmjs.org\n", "policy deny network --sandbox dev ads.example.com", "policy rm network --force --resource ads.example.com --sandbox dev", "policy log --json --type network --limit 500 dev"} {
		if !strings.Contains(got, want) {
			t.Fatalf("no %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, "a,b") || strings.Contains(got, "-x") {
		t.Fatalf("a bad host reached sbx:\n%s", got)
	}
}

func TestRememberingHostCommandsIsSbxsSetting(t *testing.T) {
	log := fakeSBX(t)
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	if out, _, err := run(t, paths, "", "config", "get", "remember_host_commands"); err != nil || out != "false\n" {
		t.Fatalf("%v %q", err, out)
	}
	if _, _, err := run(t, paths, "", "config", "set", "remember_host_commands", "yes"); err == nil {
		t.Fatal("a value that is no bool went through")
	}
	for _, args := range [][]string{{"config", "set", "remember_host_commands", "true"}, {"config", "unset", "remember_host_commands"}, {"config", "set", "claude_remote_control", "true"}} {
		if _, _, err := run(t, paths, "", args...); err != nil {
			t.Fatal(err)
		}
	}
	calls, _ := os.ReadFile(log)
	if !strings.Contains(string(calls), "settings set env.rememberHostCommands true\n") || !strings.Contains(string(calls), "settings unset env.rememberHostCommands\n") || !strings.Contains(string(calls), "settings set claude.remoteControl true\n") {
		t.Fatalf("%s", calls)
	}
}

func TestInitAsksAboutClaudesRemoteControlForAnEnvThatRunsIt(t *testing.T) {
	log := fakeSBX(t)
	t.Setenv("ACCESSIBLE", "1")
	if err := os.WriteFile(filepath.Join(filepath.Dir(log), "kit-claude"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	repo := filepath.Dir(filepath.Dir(teamEnv(t)))
	// SSH agent, editors, host commands; remote control: yes; the arg;
	// the secrets: later; no sandbox yet.
	_, said, err := runAt(t, paths, "1\n1\n1\n1\nperson\nn\n2\nn\n", "init", repo)
	if err != nil || !strings.Contains(said, "Claude remote control") {
		t.Fatalf("%v\n%s", err, said)
	}
	if cfg, _ := paths.LoadConfig(); cfg.ClaudeRemoteControl == nil || !*cfg.ClaudeRemoteControl {
		t.Fatalf("%v", cfg.ClaudeRemoteControl)
	}
	if c := calls(t, log); !strings.Contains(c, "settings set claude.remoteControl true\n") || !strings.Contains(c, "settings set env.rememberHostCommands false\n") {
		t.Fatalf("%s", c)
	}
}

func TestTheTeamsPortsComeFromItsFilesUnderYours(t *testing.T) {
	fakeSBX(t)
	t.Setenv("ACCESSIBLE", "1")
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	file := teamEnv(t)
	if err := os.WriteFile(unibox.TeamFile(file), []byte("ports:\n  vite: {port: 5173, description: Vite dev server}\n  api: {port: '8080:8080', description: The API, default: on}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// SSH agent: no; editors: no; host commands: ask; the arg; store the
	// secrets; no sandbox yet. The walk asks nothing of ports.
	_, said, err := runAt(t, paths, "1\n2\n1\nperson\ny\n2\nn\n", "init", filepath.Dir(filepath.Dir(file)))
	if err != nil {
		t.Fatalf("%v\n%s", err, said)
	}
	out, _, err := run(t, paths, "", "env", "show", "team")
	if err != nil || !strings.Contains(out, "port vite 5173, the env's, off at start: Vite dev server") || !strings.Contains(out, "port api 8080:8080, the env's, on at start: The API") {
		t.Fatalf("%v\n%s", err, out)
	}
	// One's own mapping and switch for the team's port, by hand; its
	// description stays.
	writeMine := func(ports string) {
		t.Helper()
		cfg, err := os.ReadFile(filepath.Join(paths.Config, "config.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		s := strings.Replace(string(cfg), "    team:\n", "    team:\n        ports:\n"+ports, 1)
		if err := os.WriteFile(filepath.Join(paths.Config, "config.yaml"), []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeMine("            vite: {port: '9173:5173', default: on}\n")
	out, _, err = run(t, paths, "", "env", "show", "team")
	if err != nil || !strings.Contains(out, "port vite 9173:5173, the env's, on at start (yours): Vite dev server") {
		t.Fatalf("%v\n%s", err, out)
	}
	// Switched for oneself by command, and taken back.
	if out, _, err = run(t, paths, "", "port", "default", "api", "off"); err != nil || !strings.Contains(out, "port api is off at each start of env team's sandboxes, by your config.yaml") {
		t.Fatalf("%v\n%s", err, out)
	}
	if out, _, err = run(t, paths, "", "env", "show", "team"); err != nil || !strings.Contains(out, "port api 8080:8080, the env's, off at start (yours)") {
		t.Fatalf("%v\n%s", err, out)
	}
	if out, _, err = run(t, paths, "", "port", "default", "api", "-"); err != nil || !strings.Contains(out, "on at each start of env team's sandboxes, as the env's files say") {
		t.Fatalf("%v\n%s", err, out)
	}
	if cfg, _ := os.ReadFile(filepath.Join(paths.Config, "config.yaml")); strings.Contains(string(cfg), "api") {
		t.Fatalf("taking it back left\n%s", cfg)
	}
	if _, _, err = run(t, paths, "", "port", "default", "nope", "on"); err == nil || !strings.Contains(err.Error(), "has no port nope") {
		t.Fatalf("%v", err)
	}
}

func TestTheEnvsReposAreNamedAndEveryNewTaskTakesThoseThatSayAlways(t *testing.T) {
	fakeSBX(t)
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	file := teamEnv(t)
	if err := os.WriteFile(unibox.TeamFile(file), []byte("repos:\n  adrs: {url: https://gitlab.example.com/arch/adrs.git, ref: main, default: always, description: Decisions}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, paths, "", "env", "add", "team", file); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, paths, "", "var", "set", "user", "person", "-e", "team"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, paths, "", "repo", "add", "api", "https://gitlab.example.com/x/api.git", "--ref", "develop"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, paths, "", "repo", "default", "api", "on"); err != nil {
		t.Fatal(err)
	}
	out, _, err := run(t, paths, "", "repo", "ls")
	if err != nil || !strings.Contains(out, "adrs  context  main     always   the env's") || !strings.Contains(out, "api   work     develop  on       yours") {
		t.Fatalf("%v\n%s", err, out)
	}
	// Named, a repository of the index is its address; the one that says
	// always comes along, cloned.
	out, _, err = run(t, paths, "", "task", "check", "api", "--json")
	var check api.Check
	if json.Unmarshal([]byte(out), &check) != nil || len(check.Repositories) != 2 || check.Repositories[0].URL != "https://gitlab.example.com/x/api.git" || check.Repositories[0].Branch != "develop" || check.Repositories[1].Name != "adrs" || check.Repositories[1].Mounted {
		t.Fatalf("%v\n%s", err, out)
	}
	// A branch given wins over the index's.
	out, _, _ = run(t, paths, "", "task", "check", "api", "--branch", "fix-1", "--json")
	if json.Unmarshal([]byte(out), &check) != nil || check.Repositories[0].Branch != "fix-1" || check.Repositories[1].Branch != "main" {
		t.Fatalf("%s", out)
	}
	// A checkout of the repository every task takes is not given it again.
	checkout := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"remote", "add", "origin", "git@gitlab.example.com:arch/adrs.git"}} {
		if b, err := exec.Command("git", append([]string{"-C", checkout}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, b)
		}
	}
	out, _, _ = run(t, paths, "", "task", "check", checkout, "--json")
	if json.Unmarshal([]byte(out), &check) != nil || len(check.Repositories) != 1 || !check.Repositories[0].Mounted {
		t.Fatalf("%s", out)
	}
	// Beside a mounted directory it is cloned still.
	dir := t.TempDir()
	out, _, _ = run(t, paths, "", "task", "check", dir, "--json")
	if json.Unmarshal([]byte(out), &check) != nil || len(check.Repositories) != 2 || !check.Repositories[0].Mounted || check.Repositories[1].Mounted || check.Repositories[1].Name != "adrs" {
		t.Fatalf("%s", out)
	}
	// task new offers them, those that say on or always picked, then the
	// branch of those worked in and the task's name.
	t.Setenv("ACCESSIBLE", "1")
	_, said, _ := runAt(t, paths, "\n\n\n", "task", "new")
	if !strings.Contains(said, "Repositories for a new task in env team") || !strings.Contains(said, "always · Decisions") || !strings.Contains(said, "Branch of the repositories worked in") || !strings.Contains(said, "Task name") {
		t.Fatalf("%s", said)
	}
	t.Setenv("ACCESSIBLE", "")
	if _, _, err := run(t, paths, "", "task", "new", "nope"); err == nil || !strings.Contains(err.Error(), "nope is neither a directory nor a repository env team names") {
		t.Fatalf("%v", err)
	}
	if _, _, err := run(t, paths, "", "repo", "rm", "adrs"); err == nil || !strings.Contains(err.Error(), "one of the team's stays") {
		t.Fatalf("%v", err)
	}
	if _, _, err := run(t, paths, "", "repo", "rm", "api"); err != nil {
		t.Fatal(err)
	}
}

func TestWhatIsSaidForAnEnvRepoReachesItsEnvsAndAnEnvOfNoneRefusesIt(t *testing.T) {
	fakeSBX(t)
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	file := teamEnv(t)
	root := filepath.Dir(filepath.Dir(file))
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"remote", "add", "origin", "git@gitlab.example.com:team/envs.git"}} {
		if b, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, b)
		}
	}
	if err := os.WriteFile(unibox.TeamFile(file), []byte("ports:\n  api: {port: '8080:8080'}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, paths, "", "env", "add", "team", file); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, paths, "", "env", "add", "plain", teamEnv(t)); err != nil {
		t.Fatal(err)
	}
	out, _, err := run(t, paths, "", "repo", "add", "notes", "https://github.com/me/notes.git", "--env-repo", "-e", "team")
	if err != nil || !strings.Contains(out, "for every env of gitlab.example.com/team/envs") {
		t.Fatalf("%v\n%s", err, out)
	}
	if cfg, _ := os.ReadFile(filepath.Join(paths.Config, "config.yaml")); !strings.Contains(string(cfg), "env_repos:\n    gitlab.example.com/team/envs:\n        repos:\n            notes: https://github.com/me/notes.git") {
		t.Fatalf("%s", cfg)
	}
	if out, _, err = run(t, paths, "", "repo", "ls", "-e", "team"); err != nil || !strings.Contains(out, "yours, this env repo") {
		t.Fatalf("%v\n%s", err, out)
	}
	if out, _, err = run(t, paths, "", "repo", "ls", "-e", "plain"); err != nil || strings.Contains(out, "notes") {
		t.Fatalf("an env of another repository has it: %v\n%s", err, out)
	}
	if out, _, err = run(t, paths, "", "port", "default", "api", "on", "--env-repo", "-e", "team"); err != nil || !strings.Contains(out, "by your config.yaml") {
		t.Fatalf("%v\n%s", err, out)
	}
	if out, _, err = run(t, paths, "", "env", "show", "team"); err != nil || !strings.Contains(out, "port api 8080:8080, the env's, on at start (yours)") {
		t.Fatalf("%v\n%s", err, out)
	}
	// A value for the env repo, under the env's own.
	for _, args := range [][]string{{"var", "set", "user", "team-wide", "--env-repo", "-e", "team"}, {"var", "set", "user", "mine", "-e", "team"}} {
		if _, _, err := run(t, paths, "", args...); err != nil {
			t.Fatal(err)
		}
	}
	if out, _, err = run(t, paths, "", "var", "ls", "-e", "team"); err != nil || !strings.Contains(out, "mine") || !strings.Contains(out, "yours") {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, _, err := run(t, paths, "", "var", "rm", "user", "-e", "team"); err != nil {
		t.Fatal(err)
	}
	if out, _, err = run(t, paths, "", "var", "ls", "-e", "team"); err != nil || !strings.Contains(out, "team-wide") || !strings.Contains(out, "env-repo") {
		t.Fatalf("%v\n%s", err, out)
	}
	// An env of no repository has no env repo to say anything for.
	for _, args := range [][]string{{"repo", "add", "x", "https://github.com/me/x.git", "--env-repo", "-e", "plain"}, {"port", "default", "api", "on", "--env-repo", "-e", "plain"}, {"var", "set", "user", "x", "--env-repo", "-e", "plain"}} {
		if _, _, err := run(t, paths, "", args...); err == nil || !strings.Contains(err.Error(), "env plain has its files in no repository") {
			t.Fatalf("%v: %v", args, err)
		}
	}
	if _, _, err := run(t, paths, "", "repo", "add", "x", "https://github.com/me/x.git", "--env-repo", "--every-env"); err == nil {
		t.Fatal("--env-repo and --every-env were taken together")
	}
}

func TestADirectoryOfTheIndexStaysMountedBesideAnAddressThatIsCloned(t *testing.T) {
	fakeSBX(t)
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	if _, _, err := run(t, paths, "", "env", "add", "team", teamEnv(t)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, paths, "", "var", "set", "user", "person", "-e", "team"); err != nil {
		t.Fatal(err)
	}
	notes := t.TempDir()
	if _, _, err := run(t, paths, "", "repo", "add", "notes", notes, "--default", "always"); err != nil {
		t.Fatal(err)
	}
	out, _, _ := run(t, paths, "", "task", "check", "--branch", "main", "https://gitlab.example.com/x/raw.git", "--json")
	var check api.Check
	if err := json.Unmarshal([]byte(out), &check); err != nil || len(check.Repositories) != 2 {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, r := range check.Repositories {
		if (r.Name == "notes") != r.Mounted {
			t.Fatalf("%s", out)
		}
	}
	// One named by the index is read, as it says, on its own ref.
	if _, _, err := run(t, paths, "", "repo", "add", "adrs", "https://gitlab.example.com/x/adrs.git", "--ref", "master", "--context", "true"); err != nil {
		t.Fatal(err)
	}
	out, _, _ = run(t, paths, "", "task", "check", "--branch", "main", "--repo", "https://gitlab.example.com/x/raw.git", "--repo", "adrs", "--json")
	if err := json.Unmarshal([]byte(out), &check); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if i := slices.IndexFunc(check.Repositories, func(r api.Repository) bool { return r.Name == "adrs" }); i < 0 || !check.Repositories[i].Context || check.Repositories[i].Branch != "master" {
		t.Fatalf("%s", out)
	}
	// Those that say on come along when asked for, as task new picks them.
	if _, _, err := run(t, paths, "", "repo", "default", "adrs", "on"); err != nil {
		t.Fatal(err)
	}
	for _, defaults := range []bool{false, true} {
		args := []string{"task", "check", "--branch", "main", "https://gitlab.example.com/x/raw.git", "--json"}
		if defaults {
			args = append(args, "--defaults")
		}
		out, _, _ = run(t, paths, "", args...)
		if err := json.Unmarshal([]byte(out), &check); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		if got := slices.ContainsFunc(check.Repositories, func(r api.Repository) bool { return r.Name == "adrs" && r.Context && r.Branch == "master" }); got != defaults {
			t.Fatalf("--defaults %v: %s", defaults, out)
		}
	}
	// --only takes what is named, not even what every task takes.
	out, _, _ = run(t, paths, "", "task", "check", "--branch", "main", "https://gitlab.example.com/x/raw.git", "--only", "--json")
	if err := json.Unmarshal([]byte(out), &check); err != nil || len(check.Repositories) != 1 {
		t.Fatalf("%v\n%s", err, out)
	}
}

func TestANewTaskStopsWhereItsBranchExistsAlready(t *testing.T) {
	fakeSBX(t)
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	if _, _, err := run(t, paths, "", "env", "add", "team", teamEnv(t)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, paths, "", "var", "set", "user", "person", "-e", "team"); err != nil {
		t.Fatal(err)
	}
	web := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"commit", "-q", "--allow-empty", "-m", "first"}, {"branch", "feat/x"}} {
		if b, err := exec.Command("git", append([]string{"-C", web}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, b)
		}
	}
	if _, _, err := run(t, paths, "", "repo", "add", "web", web); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, paths, "", "task", "new", "web", "--branch", "feat/x", "--no-push"); err == nil || !strings.Contains(err.Error(), "branch feat/x exists already in web") {
		t.Fatalf("%v", err)
	}
}

func TestTheContextsSandboxAndEnvGoTogether(t *testing.T) {
	fakeSBX(t)
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	if _, _, err := run(t, paths, "", "env", "add", "team", teamEnv(t)); err != nil {
		t.Fatal(err)
	}
	if err := paths.State(func(s *unibox.State) error {
		s.Sandboxes["team"] = api.Sandbox{Name: "team", Env: "team"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// The context is a sandbox, whose env commands take; an env names its
	// sandbox once made, and stands for it until then.
	for _, args := range [][]string{{"-s", "team"}, {"-e", "team"}} {
		out, _, err := run(t, paths, "", append([]string{"context", "use"}, args...)...)
		if err != nil || !strings.Contains(out, "commands work in sandbox team, of env team") {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		if cfg, _ := paths.LoadConfig(); cfg.Context != (unibox.Context{Sandbox: "team"}) {
			t.Fatalf("%v: %+v", args, cfg.Context)
		}
	}
	if err := paths.State(func(s *unibox.State) error {
		delete(s.Sandboxes, "team")
		s.Sandboxes["other"] = api.Sandbox{Name: "other", Env: "gone"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Another env's only sandbox is not taken for one not made yet.
	out, _, err := run(t, paths, "", "context", "use", "-e", "team")
	if err != nil || !strings.Contains(out, "commands work in env team, whose sandbox is not made yet") {
		t.Fatalf("%v\n%s", err, out)
	}
}

func TestAnEnvsOwnTokenIsACommandInConfigYaml(t *testing.T) {
	fakeSBX(t)
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	if _, _, err := run(t, paths, "", "env", "add", "team", teamEnv(t)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, paths, "", "secret", "set", "anthropic", "-e", "team", "--env-only"); err == nil || !strings.Contains(err.Error(), "give --command") {
		t.Fatalf("%v", err)
	}
	if _, _, err := run(t, paths, "", "secret", "set", "anthropic", "-e", "team", "--env-only", "--command", "pass anthropic-team"); err != nil {
		t.Fatal(err)
	}
	if cfg, _ := paths.LoadConfig(); cfg.Envs["team"].Secrets["anthropic"].Command != "pass anthropic-team" {
		t.Fatalf("%+v", cfg.Envs["team"])
	}
	if out, _, err := run(t, paths, "", "secret", "ls", "-e", "team"); err != nil || !strings.Contains(out, "env team, config.yaml") {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, _, err := run(t, paths, "", "secret", "rm", "anthropic", "-e", "team", "--env-only"); err != nil {
		t.Fatal(err)
	}
	if cfg, _ := paths.LoadConfig(); cfg.Envs["team"].Secrets != nil {
		t.Fatalf("%+v", cfg.Envs["team"])
	}
}
