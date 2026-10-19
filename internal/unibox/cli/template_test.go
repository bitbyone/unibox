package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitFromCopiesATemplateProfileAndSaysWhatItNeeds(t *testing.T) {
	fakeHostTools(t)
	paths, root := cliFixture(t)
	src := t.TempDir()
	files := map[string]string{
		"sandboxes/team/sandbox.yaml":    "version: 6\nimage: example/image\nagents: [claude]\nvars:\n  artifactory-user: your Artifactory user name\nservices:\n  artifactory: {host: artifactory.example.com, command: secret:artifactory}\nenv:\n  - ARTIFACTORY_USER: ${vars.artifactory-user}\n  - ARTIFACTORY_TOKEN: ${services.artifactory}\n",
		"_shared/@instructions/team.md":  "Team.\n",
		"_shared/@install/team/thing.sh": "echo thing\n",
	}
	for rel, body := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(src, rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(src, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Inspecting first writes nothing.
	out, _, err := run(t, paths, "", "template", "inspect", src, "--template", "acme", "--json")
	if err != nil || !strings.Contains(out, `"artifactory-user": "your Artifactory user name"`) || !strings.Contains(out, `"api_version": 1`) {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(root, ".templates")); err == nil {
		t.Fatal("inspect wrote into the repository")
	}
	out, log, err := run(t, paths, "", "init", "dev", "--from", src, "--use", "team", "--template", "acme")
	if err != nil {
		t.Fatalf("%v\n%s", err, log)
	}
	for _, want := range []string{"copied profile team of template acme as dev", "unibox var set artifactory-user <value>", "unibox secret set artifactory"} {
		if !strings.Contains(out, want) {
			t.Fatalf("lacks %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".templates", "acme", "@install", "team", "thing.sh")); err != nil {
		t.Fatal("the template's shared files were not kept")
	}
	if out, _, err = run(t, paths, "", "template", "ls"); err != nil || !strings.Contains(out, "acme") || !strings.Contains(out, "dev") {
		t.Fatalf("%v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(src, "_shared", "@install", "team", "thing.sh"), []byte("echo newer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, _, err = run(t, paths, "", "template", "update", "acme", "--yes"); err != nil || !strings.Contains(out, "~ @install/team/thing.sh") || !strings.Contains(out, "profile dev is as the template's team") {
		t.Fatalf("%v\n%s", err, out)
	}
	if data, _ := os.ReadFile(filepath.Join(root, ".templates", "acme", "@install", "team", "thing.sh")); string(data) != "echo newer\n" {
		t.Fatalf("%s", data)
	}
	if _, _, err = run(t, paths, "", "template", "diff", "dev"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = run(t, paths, "", "init", "other", "--use", "team"); err == nil {
		t.Fatal("--use without --from was taken")
	}
}
