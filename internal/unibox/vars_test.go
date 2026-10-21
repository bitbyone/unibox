package unibox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestAProfileAsksEachPersonForValuesOfTheirOwn(t *testing.T) {
	t.Parallel()
	root := profileFixture(t)
	file := filepath.Join(root, "sandboxes", "node", "sandbox.yaml")
	write(t, file, "version: 6\nstart: start.sh\nagents: [claude]\nvars:\n  artifactory-user: your Artifactory user name\n  gitlab-2n-author: you on gitlab.2n.cz, as Name <email>\n  npm-mirror: your npm mirror\n  unused-one: nothing refers to it\nservices:\n  gitlab-2n:\n    git: true\n    host: gitlab.2n.cz\n    user: oauth2\n    command: secret:gitlab-2n\n    author: ${vars.gitlab-2n-author}\nenv:\n  - ORG_GRADLE_PROJECT_artifactoryUser2N: ${vars.artifactory-user}\nbuild_args:\n  NPM_MIRROR: ${vars.npm-mirror}\n")
	orig, err := LoadProfile(root, "node")
	if err != nil {
		t.Fatal(err)
	}
	p, err := orig.withVars(map[string]string{"artifactory-user": "tobola"})
	if err != nil {
		t.Fatal(err)
	}
	// A missing value stops the run, and says what it is for and how to
	// set it; one nothing refers to does not.
	var f *Failure
	if err := p.missingVarsError(); !errors.As(err, &f) || f.Code != "var_missing" || !strings.Contains(err.Error(), "gitlab-2n-author (you on gitlab.2n.cz") || strings.Contains(err.Error(), "unused-one") || !strings.Contains(err.Error(), "unibox var set") {
		t.Fatalf("%v", err)
	}
	if vars, _ := p.Unused(); !slices.Equal(vars, []string{"unused-one"}) {
		t.Fatalf("unused %v", vars)
	}
	u := UserVars{Vars: map[string]string{"gitlab-2n-author": "Someone <someone@example.com>", "artifactory-user": "tobola", "npm-mirror": "https://npm.example"}, Profiles: map[string]map[string]string{"node": {"gitlab-2n-author": "Jan Tobola <tobola@2n.example>"}}}
	if p, err = orig.withVars(u.For("node")); err != nil {
		t.Fatal(err)
	}
	// The profile's own value wins over the shared one, and fills the
	// service's author.
	if p.missingVarsError() != nil || p.Services["gitlab-2n"].Author != "Jan Tobola <tobola@2n.example>" {
		t.Fatalf("%+v", p.Services["gitlab-2n"])
	}
	// A var reaches a session only where env refers to it.
	if env := p.SessionEnv(); !slices.Equal(env, []string{"ORG_GRADLE_PROJECT_artifactoryUser2N=tobola"}) {
		t.Fatalf("session env %v", env)
	}
	if s := initializeScript(p, ""); !strings.Contains(s, "env 'ORG_GRADLE_PROJECT_artifactoryUser2N=tobola' /bin/sh /var/lib/unibox/start.sh") {
		t.Fatalf("start runs without the values:\n%s", s)
	}
	// A build argument is passed to the build, and a change of it builds
	// the image again.
	args, err := p.BuildArgs("sbx")
	if err != nil || !slices.Contains(args, "NPM_MIRROR=https://npm.example") {
		t.Fatalf("%v %v", args, err)
	}
	u.Vars["npm-mirror"] = "https://other.example"
	other, _ := orig.withVars(u.For("node"))
	if other.Revision == p.Revision {
		t.Fatal("a build argument's value did not change the image's revision")
	}
	if _, err := orig.withVars(map[string]string{"gitlab-2n-author": "no address", "artifactory-user": "x", "npm-mirror": "x"}); err == nil {
		t.Fatal("an author that is no Name <email> was taken")
	}
	for _, bad := range []string{
		"version: 6\nstart: start.sh\nagents: [claude]\nvars:\n  artifactory-password: the password\n",
		"version: 6\nstart: start.sh\nagents: [claude]\nvars:\n  ARTIFACTORY_USER: upper case\n",
		"version: 6\nstart: start.sh\nagents: [claude]\nservices:\n  gitlab:\n    git: true\n    host: gitlab.com\n    user: ${vars.who}\n    command: secret:gitlab\n",
		"version: 6\nstart: start.sh\nagents: [claude]\nservices:\n  art: {host: a.example, command: c}\nbuild_args:\n  TOKEN_HOLDER: ${services.art}\n",
	} {
		write(t, file, bad)
		if _, err := LoadProfile(root, "node"); err == nil {
			t.Fatalf("accepted:\n%s", bad)
		}
	}
}

func TestARunStopsWhileAValueIsMissing(t *testing.T) {
	m, _ := managerFixture(t)
	ctx := context.Background()
	write(t, filepath.Join(m.Config.Environments, "sandboxes", "node", "sandbox.yaml"), "version: 6\nagents: [claude, codex]\nresources: {cpus: 2, memory: 1g}\nstart: start.sh\nvars:\n  team-user: your user name\nenv:\n  - TEAM_USER: ${vars.team-user}\n")
	if _, err := m.Create(ctx, Request{NewSandbox: "dev", Profile: "node"}); err == nil || !strings.Contains(err.Error(), "team-user (your user name)") {
		t.Fatalf("created without the value: %v", err)
	}
	if created, err := m.Paths.EnsureVarsFile(); err != nil || !created {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(m.Paths.VarsFile())
	if !strings.Contains(string(data), "never in the environments repository") {
		t.Fatalf("%s", data)
	}
	u, _ := m.Paths.LoadVars()
	u.Vars["team-user"] = "tobola"
	if err := m.Paths.SaveVars(u); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(ctx, Request{NewSandbox: "dev", Profile: "node"}); err != nil {
		t.Fatal(err)
	}
}

func TestAVarsDefaultStandsWhileNobodySetsIt(t *testing.T) {
	t.Parallel()
	root := profileFixture(t)
	file := filepath.Join(root, "sandboxes", "node", "sandbox.yaml")
	write(t, file, "version: 6\nstart: start.sh\nagents: [claude]\nresources:\n  cpus: ${vars.cpus:-100%}\n  memory: ${vars.memory:-8g}\nvars:\n  cpus: CPUs for your machine\n  memory: memory for your machine\n  mirror: your npm mirror\nenv:\n  - NPM_MIRROR: ${vars.mirror:-https://registry.npmjs.org}\n")
	orig, err := LoadProfile(root, "node")
	if err != nil {
		t.Fatal(err)
	}
	// None set: the defaults, and nothing missing.
	p, err := orig.withVars(nil)
	if err != nil || p.missingVarsError() != nil || p.Resources.CPUs != "100%" || p.Resources.Memory != "8g" || !slices.Equal(p.SessionEnv(), []string{"NPM_MIRROR=https://registry.npmjs.org"}) {
		t.Fatalf("%v %+v %v", err, p.Resources, p.SessionEnv())
	}
	if vars, _ := p.Unused(); len(vars) != 0 {
		t.Fatalf("unused %v", vars)
	}
	if d := p.VarDefaults(); d["cpus"] != "100%" || d["mirror"] != "https://registry.npmjs.org" {
		t.Fatal(d)
	}
	// A profile's own value over the global one, the global one over the
	// default.
	u := UserVars{Vars: map[string]string{"cpus": "4", "memory": "16g"}, Profiles: map[string]map[string]string{"node": {"cpus": "6"}}}
	if p, err = orig.withVars(u.For("node")); err != nil || p.Resources.CPUs != "6" || p.Resources.Memory != "16g" {
		t.Fatalf("%v %+v", err, p.Resources)
	}
	// A value of the wrong form is told as the person's.
	if _, err = orig.withVars(map[string]string{"cpus": "lots"}); err == nil || !strings.Contains(err.Error(), "as your vars fill it in") {
		t.Fatalf("%v", err)
	}
	// A resource refers to declared vars alone, and a service has no default.
	write(t, file, "version: 6\nstart: start.sh\nagents: [claude]\nresources: {cpus: '${vars.cores:-2}'}\n")
	if _, err = LoadProfile(root, "node"); err == nil || !strings.Contains(err.Error(), "do not declare") {
		t.Fatalf("%v", err)
	}
	write(t, file, "version: 6\nstart: start.sh\nagents: [claude]\nservices:\n  art: {host: a.example, command: secret:art}\nenv:\n  - TOKEN_ART: ${services.art:-x}\n")
	if _, err = LoadProfile(root, "node"); err == nil || !strings.Contains(err.Error(), "only a var") {
		t.Fatalf("%v", err)
	}
}

func TestSecretsAreReadByThePersonsOwnCommandWithoutAnotherApproval(t *testing.T) {
	t.Parallel()
	root := profileFixture(t)
	write(t, filepath.Join(root, "sandboxes", "node", "sandbox.yaml"), "version: 6\nstart: start.sh\nagents:\n  claude: {auth: {token: 'secret:claude'}}\nservices:\n  art: {host: a.example, command: 'secret:art'}\n  vault: {host: v.example, command: vault read x}\nenv:\n  - TOKEN_ART: ${services.art}\n  - TOKEN_VAULT: ${services.vault}\n")
	orig, err := LoadProfile(root, "node")
	if err != nil {
		t.Fatal(err)
	}
	u := emptyVars()
	u.Secrets = "op read op://Work/{name}/credential"
	u.ProfileSecrets["other"] = "pass show {name}"
	p := orig.withSecrets(u.SecretsFor("node"))
	if p.AccessHash() != orig.AccessHash() {
		t.Fatal("where a person keeps their secrets asked for another approval")
	}
	for command, via := range map[string]string{p.Auth["claude"].Command: p.Auth["claude"].Via, p.Services["art"].Command: p.Services["art"].Via, p.Services["vault"].Command: p.Services["vault"].Via} {
		got, err := ResolveCommand(command, via)
		want := map[string]string{"secret:claude": "op read op://Work/claude/credential", "secret:art": "op read op://Work/art/credential", "vault read x": "vault read x"}[command]
		if err != nil || got != want {
			t.Errorf("%s: %q %v", command, got, err)
		}
	}
	// A change of where secrets come from gives the sandboxes the new one.
	if string(settingsSnapshot(p, &fakeRuntime{})) == string(settingsSnapshot(orig, &fakeRuntime{})) {
		t.Fatal("a new source of secrets would not reach the sandbox")
	}
	if u.SecretsFor("other") != "pass show {name}" {
		t.Fatal(u.SecretsFor("other"))
	}
}

func TestVarsTOMLKeepsSecretsCommandsBesideTheValues(t *testing.T) {
	t.Parallel()
	paths := Paths{Config: t.TempDir(), Cache: t.TempDir()}
	u := emptyVars()
	u.Vars["cpus"] = "4"
	u.Secrets = "op read op://Work/{name}/credential"
	u.Profiles["dev"] = map[string]string{"cpus": "8"}
	u.ProfileSecrets["dev"] = "pass show {name}"
	if err := paths.SaveVars(u); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(paths.VarsFile())
	for _, want := range []string{"[global]\ncpus = '4'\n", "[secrets]\ncommand = 'op read op://Work/{name}/credential'\n", "[profiles.dev]\ncpus = '8'\n", "[profiles.dev.secrets]\ncommand = 'pass show {name}'\n"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("no %q in\n%s", want, data)
		}
	}
	if !(strings.Index(string(data), "[global]") < strings.Index(string(data), "[secrets]") && strings.Index(string(data), "[secrets]") < strings.Index(string(data), "[profiles.dev]")) {
		t.Fatalf("tables out of order:\n%s", data)
	}
	back, err := paths.LoadVars()
	if err != nil || back.SecretsFor("dev") != "pass show {name}" || back.SecretsFor("web") != u.Secrets || back.For("dev")["cpus"] != "8" {
		t.Fatalf("%v %+v", err, back)
	}
	for _, bad := range []string{"[secrets]\ncommand = 'op read x'\n", "[secrets]\nscript = 'x {name}'\n", "[global]\ncpus = 4\n", "cpus = '4'\n"} {
		write(t, paths.VarsFile(), bad)
		if _, err := paths.LoadVars(); err == nil {
			t.Errorf("%q was taken", bad)
		}
	}
}
