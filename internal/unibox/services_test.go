package unibox

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tobola/unibox/internal/unibox/api"
)

func TestProfilesSayWhichServicesTheSandboxReaches(t *testing.T) {
	t.Parallel()
	root := profileFixture(t)
	write(t, filepath.Join(root, "sandboxes", "node", "sandbox.yaml"), "version: 4\ndockerfile: Dockerfile\nstart: start.sh\nagents: [claude]\nservices:\n  github:\n    git: true\n    host: github.com\n    scheme: basic\n    user: x-access-token\n    command: gh auth token\n  artifactory:\n    host: [\"artifactory.example.com\", \"*.jfrog.io\"]\n    scheme: bearer\n    env: ARTIFACTORY_TOKEN\n    command: op read op://vault/artifactory/token\n")
	p, err := LoadProfile(root, "node")
	if err != nil {
		t.Fatal(err)
	}
	gh, art := p.Services["github"], p.Services["artifactory"]
	if !gh.Git || gh.Scheme != SchemeBasic || gh.User != "x-access-token" || len(gh.Host) != 1 || gh.Host[0] != "github.com" {
		t.Fatalf("github service parsed wrong: %+v", gh)
	}
	if art.Git || art.Env != "ARTIFACTORY_TOKEN" || art.Scheme != SchemeBearer || len(art.Host) != 2 {
		t.Fatalf("artifactory service parsed wrong: %+v", art)
	}
	// A service is host access: it changes what the user approves.
	if !p.HostFacing() {
		t.Fatal("services need no approval")
	}
	plain := p
	plain.Services = nil
	if plain.AccessHash() == p.AccessHash() {
		t.Fatal("services do not change the approval")
	}
	// The placeholder is deterministic and carries neither user nor token.
	ph := ServicePlaceholder("github", gh)
	if ph != ServicePlaceholder("github", gh) || !strings.HasPrefix(ph, "unibox-svc-github-") || strings.Contains(ph, "x-access-token") {
		t.Fatalf("placeholder is unstable or revealing: %q", ph)
	}
	if ServiceEnv("github", gh) != "UNIBOX_SVC_GITHUB" || ServiceEnv("artifactory", art) != "ARTIFACTORY_TOKEN" {
		t.Fatal("service env derived wrong")
	}
}

func TestServiceSecretCommandNeverCarriesTheTokenItself(t *testing.T) {
	t.Parallel()
	basic := serviceSecretCommand(Service{Git: true, Scheme: SchemeBasic, User: "x-access-token", Command: "gh auth token"})
	if !strings.Contains(basic, "Basic") || !strings.Contains(basic, "base64") || !strings.Contains(basic, "gh auth token") || !strings.Contains(basic, "x-access-token") {
		t.Fatalf("basic command wrong: %s", basic)
	}
	bearer := serviceSecretCommand(Service{Git: true, Scheme: SchemeBearer, Command: "cmd"})
	if !strings.Contains(bearer, "Bearer") || strings.Contains(bearer, "base64") {
		t.Fatalf("bearer command wrong: %s", bearer)
	}
	raw := serviceSecretCommand(Service{Env: "TOK", Scheme: SchemeRaw, Command: "cmd"})
	if strings.Contains(raw, "Bearer") || strings.Contains(raw, "Basic") {
		t.Fatalf("env command should store the raw token: %s", raw)
	}
}

func TestCreateProvisionsServicesAndAllowsTheirHosts(t *testing.T) {
	bin := t.TempDir()
	calls := filepath.Join(t.TempDir(), "calls")
	mustWriteExecutable(t, filepath.Join(bin, "sbx"), []byte("#!/bin/sh\necho \"$*\" >> "+Quote(calls)+"\n"))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	r := &SBX{Cache: t.TempDir()}
	acc := HostAccess{
		Network: Network{AllowNetwork: []string{"jitpack.io"}},
		Services: map[string]Service{
			"github": {Git: true, Host: HostList{"github.com"}, Scheme: SchemeBasic, User: "x-access-token", Command: "gh auth token", Refresh: "10m"},
		},
	}
	spec := Spec{Name: "unibox-home", Image: "x", Agent: "codex", Agents: []string{"codex"}, Access: acc}
	if err := r.Create(context.Background(), spec, io.Discard); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(calls)
	ph := ServicePlaceholder("github", acc.Services["github"])
	if !strings.Contains(string(data), "policy allow network --sandbox unibox-home jitpack.io,github.com") {
		t.Fatalf("service host did not join the allow policy: %s", data)
	}
	for _, want := range []string{"secret set-custom --sandbox unibox-home", "--env UNIBOX_SVC_GITHUB", "--placeholder " + ph, "--host github.com", "--refresh 10m", "--command"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("set-custom lacks %q: %s", want, data)
		}
	}
	// unibox never passes the token value, only the command that prints it.
	if strings.Contains(string(data), "ghp_") {
		t.Fatalf("a token value leaked into the argv: %s", data)
	}
}

func TestGitServicesWireTheSandboxGitWithTheirPlaceholder(t *testing.T) {
	t.Parallel()
	svc := Service{Git: true, Host: HostList{"github.com"}, Scheme: SchemeBasic, User: "x-access-token", Command: "gh auth token"}
	p := Profile{Name: "node", Agents: []string{"claude"}, HostAccess: HostAccess{Services: map[string]Service{"github": svc}}}
	a, _ := FindAgent("claude")
	task := api.Task{Directory: "/work/t", Repositories: []api.Repository{{Name: "repo", URL: "https://github.com/o/r", Branch: "work", Author: "A <a@example.com>"}}}
	script := initializeScript(p, "")
	ph := ServicePlaceholder("github", svc)
	if !strings.Contains(script, "git config --global") || !strings.Contains(script, "http.https://github.com/.extraHeader") || !strings.Contains(script, "Authorization: "+ph) {
		t.Fatalf("git service not wired: %s", script)
	}
	// It is the sandbox's setup, so every clone has it, a task's or the
	// agent's own; preparing a task does not write it again.
	if strings.Contains(prepareScript(task, p, a), "extraHeader") {
		t.Fatal("a task rewrites the sandbox's git wiring")
	}
	// An env service builds its own header; git is not touched for it.
	envOnly := Profile{Name: "node", Agents: []string{"claude"}, HostAccess: HostAccess{Services: map[string]Service{"art": {Host: HostList{"a.example"}, Scheme: SchemeBearer, Env: "ART", Command: "c"}}}}
	if strings.Contains(initializeScript(envOnly, ""), "extraHeader") {
		t.Fatal("an env service should not wire git")
	}
}

func TestAProxylessRuntimeWarnsTheTokenWillBeReadable(t *testing.T) {
	t.Parallel()
	m, _ := managerFixture(t)
	root := m.Config.Environments
	write(t, filepath.Join(root, "sandboxes", "node", "sandbox.yaml"), "version: 4\ndockerfile: Dockerfile\nstart: start.sh\nagents: [claude]\nservices: {artifactory: {host: a.example, scheme: bearer, env: ART_TOKEN, command: true}}\n")
	if err := m.RecordAccess("node"); err != nil {
		t.Fatal(err)
	}
	check, err := m.Check(context.Background(), Request{NewSandbox: "dev", Profile: "node", Runtime: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	var warned bool
	for _, n := range check.Needs {
		if n.Code == "credential_in_container" {
			if !n.Warning || n.Service != "artifactory" {
				t.Fatalf("warning malformed: %+v", n)
			}
			warned = true
		}
	}
	if !warned {
		t.Fatal("a proxy-less runtime did not warn about the service token")
	}
}

func TestOwnersOnOneForgeUseTokensOfTheirOwn(t *testing.T) {
	root := profileFixture(t)
	write(t, filepath.Join(root, "sandboxes", "node", "sandbox.yaml"), "version: 4\ndockerfile: Dockerfile\nstart: start.sh\nagents: [claude]\nservices:\n  github:\n    git: true\n    host: github.com\n    user: x-access-token\n    command: security find-generic-password -s gh-me -w\n  github-acme:\n    git: true\n    host: github.com/acme\n    user: x-access-token\n    command: security find-generic-password -s gh-acme -w\n")
	p, err := LoadProfile(root, "node")
	if err != nil {
		t.Fatal(err)
	}
	acme := p.Services["github-acme"]
	if got := acme.GitURLs(); len(got) != 1 || got[0] != "https://github.com/acme/" {
		t.Fatalf("%v", got)
	}
	if got := acme.ProxyHosts(); len(got) != 1 || got[0] != "github.com" {
		t.Fatalf("the proxy matches hosts, not paths: %v", got)
	}
	// The path's header must replace the host's, not join it: each list is
	// reset before its header is added.
	script := initializeScript(p, "")
	for name, url := range map[string]string{"github": "https://github.com/", "github-acme": "https://github.com/acme/"} {
		key := Quote("http." + url + ".extraHeader")
		want := "git config --global --add " + key + " ''\ngit config --global --add " + key + " " + Quote("Authorization: "+ServicePlaceholder(name, p.Services[name]))
		if !strings.Contains(script, want) {
			t.Fatalf("%s not wired with a reset:\n%s", name, script)
		}
	}
	bin := t.TempDir()
	calls := filepath.Join(t.TempDir(), "calls")
	mustWriteExecutable(t, filepath.Join(bin, "sbx"), []byte("#!/bin/sh\necho \"$*\" >> "+Quote(calls)+"\n"))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	r := &SBX{Cache: t.TempDir()}
	if err := r.Create(context.Background(), Spec{Name: "unibox-home", Image: "x", Agent: "claude", Agents: []string{"claude"}, Access: p.HostAccess}, io.Discard); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(calls)
	if !strings.Contains(string(data), "policy allow network --sandbox unibox-home github.com\n") {
		t.Fatalf("the host should be allowed once:\n%s", data)
	}
	if strings.Count(string(data), "--host github.com --command") != 2 || strings.Contains(string(data), "github.com/acme") {
		t.Fatalf("each service needs a secret on the bare host:\n%s", data)
	}
}

func TestAGitServiceSaysWhoCommitsUnderIt(t *testing.T) {
	t.Parallel()
	no := false
	services := map[string]Service{
		"github":      {Git: true, Host: HostList{"github.com"}, Author: "Jan Personal <jan@example.com>", CoAuthor: &no},
		"github-acme": {Git: true, Host: HostList{"github.com/acme"}, Author: "Jan Work <jan@acme.example>"},
		"artifactory": {Env: "ART", Host: HostList{"a.example"}, Scheme: SchemeBearer},
	}
	author, co := GitIdentity(services, "https://github.com/acme/api.git")
	if author != "Jan Work <jan@acme.example>" || co == nil || *co {
		t.Fatalf("the path's author wins and the host's co_author is inherited: %q %v", author, co)
	}
	if author, co = GitIdentity(services, "https://github.com/acme2/api.git"); author != "Jan Personal <jan@example.com>" {
		t.Fatalf("acme2 is not under acme: %q", author)
	}
	if author, co = GitIdentity(services, "https://gitlab.com/x/y.git"); author != "" || co != nil {
		t.Fatal("an unrelated forge took an identity")
	}
	// --author is the user's say for this run and wins; without it the
	// service's author does, over the host's configuration.
	req := Request{Author: "Someone Else <else@example.com>", Repositories: []RepoInput{{Source: "https://github.com/acme/api.git", Branch: "feat/x"}}, services: services}
	repos, _, err := ResolveRepositories(context.Background(), req)
	if err != nil || repos[0].Author != "Someone Else <else@example.com>" || repos[0].CoAuthor == nil || *repos[0].CoAuthor {
		t.Fatalf("--author lost to the profile: %+v %v", repos, err)
	}
	req.Author = ""
	repos, _, err = ResolveRepositories(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if repos[0].Author != "Jan Work <jan@acme.example>" || repos[0].CoAuthor == nil || *repos[0].CoAuthor {
		t.Fatalf("%+v", repos[0])
	}
	// The co-author hook follows the repository, over the profile's default.
	a, _ := FindAgent("claude")
	task := api.Task{Directory: "/work/t", Repositories: repos}
	if script := prepareScript(task, Profile{}, a); strings.Contains(script, "Co-Authored-By") || !strings.Contains(script, "user.email 'jan@acme.example'") {
		t.Fatalf("%s", script)
	}
}

func TestAnAgentsOwnCloneCommitsAsItsService(t *testing.T) {
	t.Parallel()
	p := Profile{HostAccess: HostAccess{Services: map[string]Service{
		"github":      {Git: true, Host: HostList{"github.com"}, Author: `Jan "Personal" <jan@example.com>`},
		"github-acme": {Git: true, Host: HostList{"github.com/acme"}, Author: "Jan Work <jan@acme.example>"},
		"gitlab":      {Git: true, Host: HostList{"gitlab.com"}},
	}}}
	home := t.TempDir()
	env := append(os.Environ(), "HOME="+home, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+filepath.Join(home, ".gitconfig"))
	run := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir, cmd.Env = dir, env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %s", args, out)
		}
		return strings.TrimSpace(string(out))
	}
	// Twice: preparation repeats, and must not pile the includes up.
	for range 2 {
		run(home, "sh", "-eu", "-c", identityScript(p))
	}
	if got := run(home, "git", "config", "--global", "--get-all", "include.path"); got == "" || strings.Count(got, "\n") != 0 {
		t.Fatalf("includes piled up: %q", got)
	}
	who := func(url string) string {
		dir := t.TempDir()
		run(dir, "git", "init", "-q")
		run(dir, "git", "remote", "add", "origin", url)
		cmd := exec.Command("git", "config", "user.email")
		cmd.Dir, cmd.Env = dir, env
		out, _ := cmd.Output()
		return strings.TrimSpace(string(out))
	}
	for url, want := range map[string]string{
		"https://github.com/acme/api.git":  "jan@acme.example",
		"https://github.com/acme2/api.git": "jan@example.com",
		"https://github.com/other/x.git":   "jan@example.com",
		"https://gitlab.com/a/b.git":       "",
	} {
		if got := who(url); got != want {
			t.Errorf("%s commits as %q, want %q", url, got, want)
		}
	}
}

func TestStoringAServiceSaysOneLineAndKeepsSbxForAFailure(t *testing.T) {
	bin := t.TempDir()
	mustWriteExecutable(t, filepath.Join(bin, "sbx"), []byte("#!/bin/sh\nif [ \"$2\" = set-custom ]; then echo 'Note: --command custom secrets are stored and replayed'; echo 'You may need to update environment variable'; for a in \"$@\"; do [ \"$a\" = bad.example ] && { echo 'command failed: no such item' >&2; exit 1; }; done; fi\nexit 0\n"))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	r := &SBX{Cache: t.TempDir()}
	acc := HostAccess{Services: map[string]Service{"github-acme": {Git: true, Host: HostList{"github.com/acme"}, User: "x-access-token", Command: "c"}}}
	var log strings.Builder
	if err := r.Create(context.Background(), Spec{Name: "unibox-home", Image: "x", Agent: "claude", Agents: []string{"claude"}, Access: acc}, &log); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(log.String(), "service github-acme: git signs in at https://github.com/acme/ through the proxy\n") || strings.Contains(log.String(), "Note:") || strings.Contains(log.String(), "You may need") {
		t.Fatalf("%s", log.String())
	}
	acc.Services = map[string]Service{"broken": {Git: true, Host: HostList{"bad.example"}, User: "u", Command: "c"}}
	err := r.Create(context.Background(), Spec{Name: "unibox-home", Image: "x", Agent: "claude", Agents: []string{"claude"}, Access: acc}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "no such item") || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("a failure must say what sbx said: %v", err)
	}
}
