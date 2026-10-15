package unibox

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// fakeSecretTools puts stand-ins for security and secret-tool on PATH. They
// keep each item as a file and record every argv, so a test can say what
// reached the command line.
func fakeSecretTools(t *testing.T) (dir, argv string) {
	t.Helper()
	bin := t.TempDir()
	dir = t.TempDir()
	argv = filepath.Join(t.TempDir(), "argv")
	mustWriteExecutable(t, filepath.Join(bin, "security"), []byte(`#!/bin/sh
printf '%s\n' "$*" >> `+Quote(argv)+`
store=`+Quote(dir)+`
case "$1" in
-i)
  read -r line
  svc=$(printf '%s\n' "$line" | sed -n 's/.* -s \([^ ]*\) .*/\1/p')
  val=$(printf '%s\n' "$line" | sed -n 's/.* -X \([0-9a-f]*\).*/\1/p')
  printf '%s' "$val" > "$store/$svc" ;;
find-generic-password)
  [ -f "$store/$3" ] || exit 44
  [ "$4" = -w ] && cat "$store/$3"
  exit 0 ;;
delete-generic-password) rm "$store/$3" ;;
dump-keychain) for f in "$store"/*; do [ -f "$f" ] && echo "    \"svce\"<blob>=\"${f##*/}\""; done ;;
esac
exit 0
`))
	mustWriteExecutable(t, filepath.Join(bin, "secret-tool"), []byte(`#!/bin/sh
printf '%s\n' "$*" >> `+Quote(argv)+`
store=`+Quote(dir)+`
case "$1" in
store) cat > "$store/st-$7" ;;
lookup) [ -f "$store/st-$5" ] || exit 1; cat "$store/st-$5" ;;
clear) rm -f "$store/st-$5" ;;
search) for f in "$store"/st-*; do [ -f "$f" ] && { n=${f##*/st-}; echo "attribute.name = $n"; echo "secret = $(cat "$f")"; }; done ;;
esac
exit 0
`))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir, argv
}

func TestSecretsReachThePlatformsStoreButNeverItsCommandLine(t *testing.T) {
	dir, argv := fakeSecretTools(t)
	ctx := context.Background()
	value := []byte(`ghp_secret value "with" quotes`)
	for _, store := range []SecretStore{Keychain{}, SecretService{}} {
		if ok, err := store.Has(ctx, "github"); err != nil || ok {
			t.Fatalf("%s: %v %v", store.Name(), ok, err)
		}
		if err := store.Set(ctx, "github", value); err != nil {
			t.Fatalf("%s: %v", store.Name(), err)
		}
		if ok, err := store.Has(ctx, "github"); err != nil || !ok {
			t.Fatalf("%s did not keep it: %v", store.Name(), err)
		}
		if names, err := store.List(ctx); err != nil || len(names) != 1 || names[0] != "github" {
			t.Fatalf("%s lists %v %v", store.Name(), names, err)
		}
		// The runtime is given a command that reads it, not the value.
		if c := store.Command("github"); strings.Contains(c, "ghp_") || !strings.Contains(c, "github") {
			t.Fatalf("%q", c)
		}
		if err := store.Remove(ctx, "github"); err != nil {
			t.Fatal(err)
		}
		if ok, _ := store.Has(ctx, "github"); ok {
			t.Fatalf("%s kept a removed secret", store.Name())
		}
	}
	data, _ := os.ReadFile(argv)
	if strings.Contains(string(data), "ghp_") || strings.Contains(string(data), hex.EncodeToString(value)) {
		t.Fatalf("a value reached the command line:\n%s", data)
	}
	// It did reach the stores, through their stdin.
	_ = dir
	if err := (Keychain{}).Set(ctx, "Bad Name", value); err == nil {
		t.Fatal("a name that could inject into security -i was accepted")
	}
}

func TestAProfileNamesASecretAndTheRuntimeGetsThePlatformsLookup(t *testing.T) {
	fakeSecretTools(t)
	root := profileFixture(t)
	write(t, filepath.Join(root, "sandboxes", "node", "sandbox.yaml"), "version: 3\ndockerfile: Dockerfile\nstart: start.sh\nagents:\n  claude: {auth: {token: secret:claude}}\nservices:\n  gitlab:\n    git: true\n    host: gitlab.com\n    user: oauth2\n    command: secret:gitlab\n")
	p, err := LoadProfile(root, "node")
	if err != nil {
		t.Fatal(err)
	}
	if refs := p.SecretRefs(); len(refs) != 2 || refs[0] != "claude" || refs[1] != "gitlab" {
		t.Fatalf("%v", refs)
	}
	got, err := ResolveCommand("secret:gitlab")
	if err != nil || (!strings.Contains(got, "find-generic-password -s unibox-gitlab -w") && !strings.Contains(got, "lookup service unibox name gitlab")) {
		t.Fatalf("%q %v", got, err)
	}
	if got, _ := ResolveCommand("vault kv get -field=token secret/ci"); got != "vault kv get -field=token secret/ci" {
		t.Fatalf("a command of the user's own changed: %q", got)
	}
	write(t, filepath.Join(root, "sandboxes", "node", "sandbox.yaml"), "version: 3\ndockerfile: Dockerfile\nstart: start.sh\nagents:\n  claude: {auth: {token: 'secret:Not Valid'}}\n")
	if _, err := LoadProfile(root, "node"); err == nil {
		t.Fatal("an invalid secret name was accepted")
	}
}

func TestEditingAProfileKeepsItsCommentsAndNeverBreaksIt(t *testing.T) {
	t.Parallel()
	m, _ := managerFixture(t)
	file := filepath.Join(m.Config.Environments, "sandboxes", "node", "sandbox.yaml")
	write(t, file, "version: 3\n# Node work.\ndockerfile: Dockerfile\nstart: start.sh\nagents: # both\n  claude:\n  # How codex signs in.\n  codex:\n    auth: oauth # the browser\n")
	no := false
	err := m.EditProfile("node", func(doc *yaml.Node) error {
		SetAgentAuth(doc, "claude", AgentAuth{Method: AuthToken, Command: "secret:claude"})
		SetService(doc, "github-acme", Service{Git: true, Host: HostList{"github.com/acme"}, User: "x-access-token", Command: "secret:github-acme", Author: "Jan <jan@acme.example>", CoAuthor: &no})
		SetService(doc, "art", Service{Host: HostList{"a.example", "*.jfrog.io"}, Scheme: SchemeBearer, Env: "ART_TOKEN", Command: "secret:art"})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(file)
	for _, want := range []string{"# Node work.", "# both", "# How codex signs in.", "auth: oauth # the browser", "auth: {token: 'secret:claude'}", "host: github.com/acme", "co_author: false", "host: [a.example, '*.jfrog.io']"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("lacks %q:\n%s", want, data)
		}
	}
	p, err := LoadProfile(m.Config.Environments, "node")
	if err != nil || p.Auth["claude"].Command != "secret:claude" || p.Services["github-acme"].Author != "Jan <jan@acme.example>" || len(p.Services["art"].Host) != 2 {
		t.Fatalf("%+v %v", p.HostAccess, err)
	}
	// A change the profile would not load with is not kept.
	err = m.EditProfile("node", func(doc *yaml.Node) error {
		SetService(doc, "broken", Service{Git: true, Host: HostList{"h.example"}, Command: "c"})
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "not kept") {
		t.Fatalf("%v", err)
	}
	if after, _ := os.ReadFile(file); string(after) != string(data) {
		t.Fatal("a refused change was left in the profile")
	}
	// The last service out takes the key with it.
	_ = m.EditProfile("node", func(doc *yaml.Node) error {
		RemoveService(doc, "art")
		RemoveService(doc, "github-acme")
		return nil
	})
	if after, _ := os.ReadFile(file); strings.Contains(string(after), "\nservices:") {
		t.Fatalf("%s", after)
	}
}

func TestCheckSaysWhichNamedSecretIsMissing(t *testing.T) {
	dir, _ := fakeSecretTools(t)
	m, _ := managerFixture(t)
	write(t, filepath.Join(m.Config.Environments, "sandboxes", "node", "sandbox.yaml"), "version: 3\ndockerfile: Dockerfile\nstart: start.sh\nagents: [claude]\nservices: {gitlab: {git: true, host: gitlab.com, user: oauth2, command: 'secret:gitlab'}}\n")
	if err := m.RecordAccess("node"); err != nil {
		t.Fatal(err)
	}
	missing := func() bool {
		check, err := m.Check(context.Background(), Request{NewSandbox: "dev", Profile: "node", Runtime: "fake"})
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range check.Needs {
			if n.Code == "secret_missing" && strings.Contains(n.Message, "secret gitlab") && strings.Contains(n.Message, "unibox secret set gitlab") {
				return true
			}
		}
		return false
	}
	if !missing() {
		t.Fatal("a missing secret was not named")
	}
	store, _ := PlatformSecrets()
	if err := store.Set(context.Background(), "gitlab", []byte("x")); err != nil {
		t.Fatal(err, dir)
	}
	if missing() {
		t.Fatal("a stored secret is still reported missing")
	}
}
