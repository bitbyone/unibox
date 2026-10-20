package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tobola/unibox/internal/testenv"
	"github.com/tobola/unibox/internal/unibox"
)

// fakeHostTools puts stand-ins for security, secret-tool and sbx on PATH.
// The stores keep each item as a file; every argv is recorded, so a test
// can say a token never reached a command line.
func fakeHostTools(t *testing.T) (argv string) {
	t.Helper()
	bin, dir := t.TempDir(), t.TempDir()
	argv = filepath.Join(t.TempDir(), "argv")
	write := func(name, body string) {
		if err := testenv.WriteExecutable(filepath.Join(bin, name), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> "+unibox.Quote(argv)+"\nstore="+unibox.Quote(dir)+"\n"+body+"\nexit 0\n")); err != nil {
			t.Fatal(err)
		}
	}
	write("security", `case "$1" in
-i) read -r line; svc=$(printf '%s\n' "$line" | sed -n 's/.* -s \([^ ]*\) .*/\1/p'); printf x > "$store/$svc" ;;
find-generic-password) [ -f "$store/$3" ] || exit 44 ;;
delete-generic-password) rm "$store/$3" ;;
dump-keychain) for f in "$store"/*; do [ -f "$f" ] && echo "    \"svce\"<blob>=\"${f##*/}\""; done ;;
esac`)
	write("secret-tool", `case "$1" in
store) cat > /dev/null; : > "$store/unibox-$7" ;;
lookup) [ -f "$store/unibox-$5" ] || exit 1; echo x ;;
clear) rm -f "$store/unibox-$5" ;;
search) for f in "$store"/unibox-*; do [ -f "$f" ] && echo "attribute.name = ${f##*/unibox-}"; done ;;
esac`)
	write("sbx", `[ "$1 $2" = "secret ls" ] && echo '{"secrets":[],"custom_secrets":[]}'`)
	// gh is signed in when the store holds a marker; ghSignedIn puts it.
	write("gh", `[ "$1 $2" = "auth status" ] && { [ -f "$store/gh" ] || exit 1; }`)
	ghDirs[argv] = dir
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return argv
}

var ghDirs = map[string]string{}

func ghSignedIn(t *testing.T, argv string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(ghDirs[argv], "gh"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

func run(t *testing.T, paths unibox.Paths, stdin string, args ...string) (string, string, error) {
	t.Helper()
	var out, log bytes.Buffer
	c := New(paths, strings.NewReader(stdin), &out, &log)
	c.SetArgs(args)
	err := c.Execute()
	return out.String(), log.String(), err
}

func TestASecretGoesInByStdinAndNeverComesOut(t *testing.T) {
	argv := fakeHostTools(t)
	paths, _ := cliFixture(t)
	out, log, err := run(t, paths, "ghp_tokenvalue\n", "secret", "set", "github")
	if err != nil || !strings.Contains(out, "stored secret github") {
		t.Fatalf("%v %s %s", err, out, log)
	}
	if out, _, err = run(t, paths, "", "secret", "ls"); err != nil || strings.TrimSpace(out) != "github" {
		t.Fatalf("%q %v", out, err)
	}
	if _, _, err = run(t, paths, "", "secret", "set", "empty"); err == nil {
		t.Fatal("an empty token was stored")
	}
	if _, _, err = run(t, paths, "", "secret", "rm", "github"); err != nil {
		t.Fatal(err)
	}
	if out, _, _ = run(t, paths, "", "secret", "ls"); strings.Contains(out, "github") {
		t.Fatal(out)
	}
	data, _ := os.ReadFile(argv)
	if strings.Contains(string(data), "ghp_") || strings.Contains(out+log, "ghp_") {
		t.Fatalf("the token was shown:\n%s", data)
	}
}

func TestAServiceIsAddedListedAndTakenOutThroughUnibox(t *testing.T) {
	argv := fakeHostTools(t)
	paths, root := cliFixture(t)
	file := filepath.Join(root, "sandboxes", "node", "sandbox.yaml")
	if err := os.WriteFile(file, []byte("version: 6\n# My profile.\nimage: example/image\nagents: [claude]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, log, err := run(t, paths, "glpat-tokenvalue\n", "service", "add", "gitlab-2n", "-p", "node", "--host", "gitlab.2n.cz", "--author", "Jan Tobola <jan@2n.example>")
	if err != nil {
		t.Fatalf("%v\n%s", err, log)
	}
	if !strings.Contains(log, "personal_access_tokens?name=unibox+gitlab-2n&scopes=read_repository,write_repository") {
		t.Fatalf("the page to make the token was not shown:\n%s", log)
	}
	data, _ := os.ReadFile(file)
	for _, want := range []string{"# My profile.", "gitlab-2n:", "git: true", "host: gitlab.2n.cz", "user: oauth2", "command: secret:gitlab-2n", "author: Jan Tobola <jan@2n.example>"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("lacks %q:\n%s", want, data)
		}
	}
	out, _, err := run(t, paths, "", "service", "ls", "-p", "node")
	if err != nil || !strings.Contains(out, "gitlab-2n") || !strings.Contains(out, "stored") {
		t.Fatalf("%v\n%s", err, out)
	}
	// A store unibox does not keep is the user's command, stored nowhere.
	if _, log, err = run(t, paths, "", "service", "add", "artifactory", "-p", "node", "--host", "repo.example.com", "--command", "vault kv get -field=token secret/art"); err != nil {
		t.Fatalf("%v\n%s", err, log)
	}
	data, _ = os.ReadFile(file)
	if !strings.Contains(string(data), "ARTIFACTORY_TOKEN: ${services.artifactory}") || !strings.Contains(string(data), "command: vault kv get -field=token secret/art") || strings.Contains(string(data), "scheme") {
		t.Fatalf("%s\n%s", log, data)
	}
	// The variable carrying a service's placeholder goes with it.
	if _, _, err = run(t, paths, "", "service", "rm", "artifactory", "-p", "node"); err != nil {
		t.Fatal(err)
	}
	if q, err := unibox.LoadProfile(root, "node"); err != nil || len(q.EnvValues) != 0 {
		t.Fatalf("%v %v", err, q.EnvValues)
	}
	// A token stored on its own is named, not asked for, and outlives the
	// service.
	if _, _, err = run(t, paths, "", "service", "add", "jfrog", "-p", "node", "--kind", "artifactory", "--host", "jfrog.example.com", "--secret", "jfrog-token"); err == nil || !strings.Contains(err.Error(), "unibox secret set jfrog-token") {
		t.Fatalf("a missing secret was named: %v", err)
	}
	if _, _, err = run(t, paths, "jf-tokenvalue\n", "secret", "set", "jfrog-token"); err != nil {
		t.Fatal(err)
	}
	if _, log, err = run(t, paths, "", "service", "add", "jfrog", "-p", "node", "--kind", "artifactory", "--host", "jfrog.example.com", "--secret", "jfrog-token"); err != nil {
		t.Fatalf("%v\n%s", err, log)
	}
	if data, _ = os.ReadFile(file); !strings.Contains(string(data), "command: secret:jfrog-token") {
		t.Fatalf("%s", data)
	}
	if _, _, err = run(t, paths, "", "service", "rm", "jfrog", "-p", "node"); err != nil {
		t.Fatal(err)
	}
	if out, _, _ = run(t, paths, "", "secret", "ls"); !strings.Contains(out, "jfrog-token") {
		t.Fatal("a secret stored on its own went with the service")
	}
	if _, _, err = run(t, paths, "", "service", "rm", "gitlab-2n", "-p", "node"); err != nil {
		t.Fatal(err)
	}
	if out, _, _ = run(t, paths, "", "secret", "ls"); strings.Contains(out, "gitlab-2n") {
		t.Fatal("the service's token outlived it")
	}
	recorded, _ := os.ReadFile(argv)
	if strings.Contains(string(recorded), "glpat-") {
		t.Fatalf("the token reached a command line:\n%s", recorded)
	}
	// A service that would break the profile is not written.
	before, _ := os.ReadFile(file)
	if _, _, err = run(t, paths, "x\n", "service", "add", "broken", "-p", "node", "--kind", "generic", "--git", "--host", "h.example"); err == nil {
		t.Fatal("a git service without a user was written")
	}
	if after, _ := os.ReadFile(file); !bytes.Equal(before, after) {
		t.Fatal("a refused service changed the profile")
	}
}

func TestLoginStoresAnAgentsTokenAndWritesItsAuth(t *testing.T) {
	argv := fakeHostTools(t)
	paths, root := cliFixture(t)
	file := filepath.Join(root, "sandboxes", "node", "sandbox.yaml")
	if err := os.WriteFile(file, []byte("version: 6\nimage: example/image\nagents: [claude]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, log, err := run(t, paths, "sk-ant-oat01-tokenvalue\n", "agent", "login", "claude", "-p", "node")
	if err != nil {
		t.Fatalf("%v\n%s", err, log)
	}
	data, _ := os.ReadFile(file)
	if !strings.Contains(string(data), "  claude:\n    auth: {token: 'secret:claude'}") {
		t.Fatalf("%s", data)
	}
	recorded, _ := os.ReadFile(argv)
	// The runtime is told where the token is, never the token.
	if !strings.Contains(string(recorded), "secret set-custom --env CLAUDE_CODE_OAUTH_TOKEN") || !(strings.Contains(string(recorded), "unibox-claude") || strings.Contains(string(recorded), "lookup service unibox name claude")) || strings.Contains(string(recorded)+log, "tokenvalue") {
		t.Fatalf("%s\n%s", recorded, log)
	}
	// Signed in already: no question, the token stays.
	if _, log, err = run(t, paths, "", "agent", "login", "claude", "-p", "node"); err != nil || !strings.Contains(log, "secret claude is stored") {
		t.Fatalf("%v\n%s", err, log)
	}
	_ = io.Discard
}

// TestLoginTakesAnAgentsTokenFromACommand: a token another program keeps
// - unagit's vault - is named by the command that prints it; unibox asks
// for nothing and the runtime is told the command.
func TestLoginTakesAnAgentsTokenFromACommand(t *testing.T) {
	argv := fakeHostTools(t)
	paths, root := cliFixture(t)
	file := filepath.Join(root, "sandboxes", "node", "sandbox.yaml")
	if err := os.WriteFile(file, []byte("version: 6\nimage: example/image\nagents: [claude, copilot]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, log, err := run(t, paths, "", "agent", "login", "--command", "/usr/local/bin/unagit credential get claude", "-p", "node"); err == nil || !strings.Contains(err.Error(), "name it") {
		t.Fatalf("a command for every agent was taken: %v\n%s", err, log)
	}
	_, log, err := run(t, paths, "", "agent", "login", "claude", "--command", "/usr/local/bin/unagit credential get claude", "-p", "node")
	if err != nil {
		t.Fatalf("%v\n%s", err, log)
	}
	data, _ := os.ReadFile(file)
	if !strings.Contains(string(data), "  claude:\n    auth: {token: /usr/local/bin/unagit credential get claude}") {
		t.Fatalf("%s", data)
	}
	recorded, _ := os.ReadFile(argv)
	if !strings.Contains(string(recorded), "secret set-custom --env CLAUDE_CODE_OAUTH_TOKEN") || !strings.Contains(string(recorded), "unagit credential get claude") {
		t.Fatalf("%s\n%s", recorded, log)
	}
	if out, _, _ := run(t, paths, "", "secret", "ls"); strings.Contains(out, "claude") {
		t.Fatal("unibox stored a token it was told another program keeps")
	}
}

func TestNetworkRulesAreChangedThroughUnibox(t *testing.T) {
	t.Parallel()
	paths, root := cliFixture(t)
	file := filepath.Join(root, "sandboxes", "node", "sandbox.yaml")
	if err := os.WriteFile(file, []byte("version: 6\nimage: example/image\nagents: [claude]\nnetwork:\n  allow: [jitpack.io] # Kotlin\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, log, err := run(t, paths, "", "network", "allow", "maven.google.com", "-p", "node"); err != nil {
		t.Fatalf("%v\n%s", err, log)
	}
	// Denying an allowed host moves it: sbx refuses both at once.
	if _, log, err := run(t, paths, "", "network", "deny", "jitpack.io", "gist.github.com", "-p", "node"); err != nil {
		t.Fatalf("%v\n%s", err, log)
	}
	data, _ := os.ReadFile(file)
	if !strings.Contains(string(data), "  allow: [maven.google.com] # Kotlin") || !strings.Contains(string(data), "  deny: [jitpack.io, gist.github.com]") {
		t.Fatalf("%s", data)
	}
	out, _, err := run(t, paths, "", "network", "ls", "-p", "node")
	if err != nil || !strings.Contains(out, "deny   gist.github.com") || !strings.Contains(out, "allow  maven.google.com") {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, _, err = run(t, paths, "", "network", "rm", "jitpack.io", "gist.github.com", "maven.google.com", "-p", "node"); err != nil {
		t.Fatal(err)
	}
	if data, _ = os.ReadFile(file); strings.Contains(string(data), "\nnetwork:") {
		t.Fatalf("emptied lists stayed:\n%s", data)
	}

	// Ports by name, on at start or only when switched on.
	if _, log, err := run(t, paths, "", "network", "port", "set", "fe", "--map", "3000:3000", "-p", "node"); err != nil {
		t.Fatalf("%v\n%s", err, log)
	}
	if _, log, err := run(t, paths, "", "network", "port", "set", "devtools", "--map", "9333:9222", "--off", "-p", "node"); err != nil {
		t.Fatalf("%v\n%s", err, log)
	}
	if _, _, err := run(t, paths, "", "network", "port", "set", "be", "--map", "x:1", "-p", "node"); err == nil {
		t.Fatal("a bad spec was written")
	}
	data, _ = os.ReadFile(file)
	if !strings.Contains(string(data), "  ports:\n    fe: '3000:3000'\n    devtools: {port: '9333:9222', default: off}\n") {
		t.Fatalf("%s", data)
	}
	if out, _, err = run(t, paths, "", "network", "port", "ls", "-p", "node"); err != nil || !strings.Contains(out, "devtools  9333:9222  off") || !strings.Contains(out, "fe        3000:3000  on") {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, _, err = run(t, paths, "", "network", "port", "unset", "fe", "devtools", "-p", "node"); err != nil {
		t.Fatal(err)
	}
	if data, _ = os.ReadFile(file); strings.Contains(string(data), "\nnetwork:") {
		t.Fatalf("emptied ports stayed:\n%s", data)
	}
}

func TestAFirstStartIsToldToInitAndInitSaysWhereItWrites(t *testing.T) {
	t.Parallel()
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	// Nothing configured: init without --dir refuses and says how.
	if _, _, err := run(t, paths, "", "init", "web"); err == nil || !strings.Contains(err.Error(), "unibox init --dir") {
		t.Fatalf("%v", err)
	}
	if _, log, err := run(t, paths, "", "profile", "ls"); err != nil || !strings.Contains(log, "unibox init --dir") {
		t.Fatalf("%v %s", err, log)
	}
	if _, log, err := run(t, paths, "", "sandbox", "ls"); err != nil || !strings.Contains(log, "no environments repository is configured") {
		t.Fatalf("%v %s", err, log)
	}
	// init --dir lays the repository out and configures it, with no profile.
	root := filepath.Join(t.TempDir(), "envs")
	out, log, err := run(t, paths, "", "init", "--dir", root)
	if err != nil || !strings.Contains(out, "environments repository: "+root+" (configured)") || !strings.Contains(out, "add a profile with unibox init <profile>") {
		t.Fatalf("%v\n%s\n%s", err, out, log)
	}
	if _, err := os.Stat(filepath.Join(root, ".shared", "@install", "base.sh")); err != nil {
		t.Fatal("the repository was not laid out")
	}
	// A profile goes into the configured one, which init names.
	if out, _, err = run(t, paths, "", "init", "web", "--agents", "claude"); err != nil || !strings.Contains(out, "environments repository: "+root+"\n") {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(root, "sandboxes", "web", "sandbox.yaml")); err != nil {
		t.Fatal(err)
	}
	// Claude's onboarding is answered once, in a seed the profile names.
	seed, err := os.ReadFile(filepath.Join(root, "sandboxes", "web", "_data", "claude", ".claude.json"))
	if err != nil || !strings.Contains(string(seed), `"hasCompletedOnboarding": true`) {
		t.Fatalf("%v %s", err, seed)
	}
	if profile, _ := os.ReadFile(filepath.Join(root, "sandboxes", "web", "sandbox.yaml")); !strings.Contains(string(profile), ".claude.json: _data/claude/.claude.json") || !strings.Contains(string(profile), "  claude:\n    co_author: Claude <noreply@anthropic.com>") {
		t.Fatalf("%s", profile)
	}
	// Again on the profile it made: complete already, kept, agents its own.
	if out, _, err = run(t, paths, "", "init", "web"); err != nil || !strings.Contains(out, "kept sandboxes/web/sandbox.yaml") {
		t.Fatalf("%v\n%s", err, out)
	}
	// Unset forgets it and leaves the files.
	if _, _, err = run(t, paths, "", "config", "unset", "environments"); err != nil {
		t.Fatal(err)
	}
	if cfg, _ := paths.LoadConfig(); cfg.Environments != "" {
		t.Fatal(cfg.Environments)
	}
	if _, err := os.Stat(filepath.Join(root, "sandboxes", "web")); err != nil {
		t.Fatal("unset removed the repository")
	}
	// A configured repository that is gone is named.
	gone := filepath.Join(t.TempDir(), "gone")
	if _, _, err = run(t, paths, "", "config", "set", "environments", gone); err != nil {
		t.Fatal(err)
	}
	if _, log, err = run(t, paths, "", "profile", "ls"); err != nil || !strings.Contains(log, gone+" does not exist") {
		t.Fatalf("%v %s", err, log)
	}
}

func TestCopilotAndGitHubCanSignInThroughGh(t *testing.T) {
	argv := fakeHostTools(t)
	paths, root := cliFixture(t)
	file := filepath.Join(root, "sandboxes", "node", "sandbox.yaml")
	if err := os.WriteFile(file, []byte("version: 6\nimage: example/image\nagents: [copilot]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// gh not signed in: --gh says what to do.
	if _, _, err := run(t, paths, "", "agent", "login", "copilot", "--gh", "-p", "node"); err == nil || !strings.Contains(err.Error(), "gh auth login") {
		t.Fatalf("%v", err)
	}
	ghSignedIn(t, argv)
	_, log, err := run(t, paths, "", "agent", "login", "copilot", "--gh", "-p", "node")
	if err != nil || !strings.Contains(log, "your whole gh login") {
		t.Fatalf("%v\n%s", err, log)
	}
	if _, _, err = run(t, paths, "", "service", "add", "github", "--gh", "-p", "node"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(file)
	if !strings.Contains(string(data), "auth: {token: gh auth token}") || !strings.Contains(string(data), "command: gh auth token") || !strings.Contains(string(data), "user: x-access-token") {
		t.Fatalf("%s", data)
	}
	// Copilot's token login keeps gh's token off github.com, and nothing
	// was stored of unibox's own.
	recorded, _ := os.ReadFile(argv)
	if !strings.Contains(string(recorded), "set-custom --env COPILOT_GITHUB_TOKEN") || !strings.Contains(string(recorded), "--command gh auth token") || strings.Contains(string(recorded), "secret set github") {
		t.Fatalf("%s", recorded)
	}
	if out, _, _ := run(t, paths, "", "secret", "ls"); strings.TrimSpace(out) != "no secrets in the login keychain" && strings.TrimSpace(out) != "no secrets in the Secret Service" {
		t.Fatalf("%q", out)
	}
	if _, _, err = run(t, paths, "", "service", "add", "gitlab", "--gh", "-p", "node"); err == nil {
		t.Fatal("--gh was taken for gitlab")
	}
}

func TestAProfileSaysHowEachAgentIsCredited(t *testing.T) {
	t.Parallel()
	paths, root := cliFixture(t)
	file := filepath.Join(root, "sandboxes", "node", "sandbox.yaml")
	if err := os.WriteFile(file, []byte("version: 6\nimage: example/image\nagents: [claude, copilot]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _, err := run(t, paths, "", "agent", "co-author", "-p", "node")
	if err != nil || !strings.Contains(out, "copilot\tCopilot <Copilot@users.noreply.github.com>\t(default)") {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, _, err = run(t, paths, "", "agent", "co-author", "copilot", "--as", "GitHub Copilot <copilot@example.com>", "-p", "node"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(file)
	// The short list became a mapping, so the agent can say more.
	if !strings.Contains(string(data), "  copilot:\n    co_author: GitHub Copilot <copilot@example.com>") {
		t.Fatalf("%s", data)
	}
	if out, _, _ = run(t, paths, "", "agent", "co-author", "copilot", "-p", "node"); !strings.Contains(out, "GitHub Copilot <copilot@example.com>\t(profile)") {
		t.Fatalf("%s", out)
	}
	if _, _, err = run(t, paths, "", "agent", "co-author", "copilot", "--as", "nobody", "-p", "node"); err == nil {
		t.Fatal("an identity without an address was taken")
	}
	// No address at all is a choice: credited, linked to no account.
	if _, _, err = run(t, paths, "", "agent", "co-author", "claude", "--as", "Claude <>", "-p", "node"); err != nil {
		t.Fatal(err)
	}
	if out, _, _ = run(t, paths, "", "agent", "ls", "-p", "node"); !strings.Contains(out, "Claude <>") {
		t.Fatalf("%s", out)
	}
	if _, _, err = run(t, paths, "", "agent", "co-author", "claude", "--reset", "-p", "node"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = run(t, paths, "", "agent", "co-author", "codex", "--as", "Codex <c@example.com>", "-p", "node"); err == nil {
		t.Fatal("an agent the profile lacks was credited")
	}
	if _, _, err = run(t, paths, "", "agent", "co-author", "copilot", "--reset", "-p", "node"); err != nil {
		t.Fatal(err)
	}
	if data, _ = os.ReadFile(file); strings.Contains(string(data), "co_author:") {
		t.Fatalf("a reset co-author stayed:\n%s", data)
	}
}
