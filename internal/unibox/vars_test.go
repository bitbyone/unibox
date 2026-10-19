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
	write(t, file, "version: 5\nstart: start.sh\nagents: [claude]\nvars:\n  ORG_GRADLE_PROJECT_artifactoryUser2N: your Artifactory user name\n  GIT_AUTHOR_2N: you on gitlab.2n.cz, as Name <email>\nservices:\n  gitlab-2n:\n    git: true\n    host: gitlab.2n.cz\n    user: oauth2\n    command: secret:gitlab-2n\n    author: ${GIT_AUTHOR_2N}\n")
	orig, err := LoadProfile(root, "node")
	if err != nil {
		t.Fatal(err)
	}
	p, err := orig.withVars(map[string]string{"ORG_GRADLE_PROJECT_artifactoryUser2N": "tobola"})
	if err != nil {
		t.Fatal(err)
	}
	// A missing value stops the run, and says what it is for and how to
	// set it.
	var f *Failure
	if err := p.missingVarsError(); !errors.As(err, &f) || f.Code != "var_missing" || !strings.Contains(err.Error(), "GIT_AUTHOR_2N (you on gitlab.2n.cz") || !strings.Contains(err.Error(), "unibox var set") {
		t.Fatalf("%v", err)
	}
	u := UserVars{Vars: map[string]string{"GIT_AUTHOR_2N": "Someone <someone@example.com>", "ORG_GRADLE_PROJECT_artifactoryUser2N": "tobola"}, Profiles: map[string]map[string]string{"node": {"GIT_AUTHOR_2N": "Jan Tobola <tobola@2n.example>"}}}
	if p, err = orig.withVars(u.For("node")); err != nil {
		t.Fatal(err)
	}
	// The profile's own value wins over the shared one, and fills the
	// service's author.
	if p.missingVarsError() != nil || p.Services["gitlab-2n"].Author != "Jan Tobola <tobola@2n.example>" {
		t.Fatalf("%+v", p.Services["gitlab-2n"])
	}
	if env := p.SessionEnv(); !slices.Contains(env, "ORG_GRADLE_PROJECT_artifactoryUser2N=tobola") {
		t.Fatalf("session env %v", env)
	}
	// start gets them too.
	if s := initializeScript(p, ""); !strings.Contains(s, "env 'GIT_AUTHOR_2N=Jan Tobola <tobola@2n.example>' 'ORG_GRADLE_PROJECT_artifactoryUser2N=tobola' /bin/sh /var/lib/unibox/start.sh") {
		t.Fatalf("start runs without the values:\n%s", s)
	}
	if _, err := orig.withVars(map[string]string{"GIT_AUTHOR_2N": "no address", "ORG_GRADLE_PROJECT_artifactoryUser2N": "x"}); err == nil {
		t.Fatal("an author that is no Name <email> was taken")
	}
	for _, bad := range []string{
		"version: 5\nstart: start.sh\nagents: [claude]\nvars:\n  ARTIFACTORY_PASSWORD: the password\n",
		"version: 5\nstart: start.sh\nagents: [claude]\nservices:\n  gitlab:\n    git: true\n    host: gitlab.com\n    user: ${WHO}\n    command: secret:gitlab\n",
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
	write(t, filepath.Join(m.Config.Environments, "sandboxes", "node", "sandbox.yaml"), "version: 5\nagents: [claude, codex]\nresources: {cpus: 2, memory: 1g}\nstart: start.sh\nvars:\n  TEAM_USER: your user name\n")
	if err := m.Build(ctx, "node", "fake", false); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(ctx, Request{NewSandbox: "dev", Profile: "node"}); err == nil || !strings.Contains(err.Error(), "TEAM_USER (your user name)") {
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
	u.Vars["TEAM_USER"] = "tobola"
	if err := m.Paths.SaveVars(u); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(ctx, Request{NewSandbox: "dev", Profile: "node"}); err != nil {
		t.Fatal(err)
	}
}
