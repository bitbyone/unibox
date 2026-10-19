package unibox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// templateFixture is a team's environments repository to copy from: a
// profile whose image installs a script of the template's own and whose
// agent gets one of its instruction texts.
func templateFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "sandboxes", "team", "sandbox.yaml"), "version: 6\nagents: [claude]\ninstructions: [team-notes]\nvars:\n  artifactory-user: your Artifactory user name\nservices:\n  artifactory: {host: artifactory.example.com, command: secret:artifactory}\nenv:\n  - ORG_GRADLE_PROJECT_artifactoryUser2N: ${vars.artifactory-user}\n  - ORG_GRADLE_PROJECT_artifactoryPassword2N: ${services.artifactory}\n")
	write(t, filepath.Join(dir, "sandboxes", "team", "sandbox.dockerfile"), "FROM example/shell\nCOPY --from=unibox / /\nRUN @install team/gradle\n")
	write(t, filepath.Join(dir, "sandboxes", "team", "_data", "claude", "settings.json"), "{}\n")
	write(t, filepath.Join(dir, "_shared", "@install", "team", "gradle.sh"), "echo team gradle\n")
	write(t, filepath.Join(dir, "_shared", "@instructions", "team-notes.md"), "Team notes.\n")
	return dir
}

func TestAProfileIsCopiedFromATemplateWithItsSharedFilesApart(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	if _, err := Init(root, "", nil, "", ""); err != nil {
		t.Fatal(err)
	}
	// The user's own shared file of the same name wins over the template's.
	write(t, filepath.Join(root, "_shared", "@instructions", "team-notes.md"), "My notes.\n")
	src := templateFixture(t)
	tpl, err := FetchTemplate(ctx, src)
	if err != nil {
		t.Fatal(err)
	}
	if profiles, _ := tpl.Profiles(); !slices.Equal(profiles, []string{"team"}) {
		t.Fatal(profiles)
	}
	if _, err = tpl.CopyProfile(root, "dev", "team", "acme"); err != nil {
		t.Fatal(err)
	}
	p, err := LoadProfile(root, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if p.Template != "acme/team" || p.TemplateName() != "acme" {
		t.Fatalf("template %q", p.Template)
	}
	install, err := p.InstallContext()
	if err != nil {
		t.Fatal(err)
	}
	if string(install["usr/local/lib/unibox/install/team/gradle.sh"].Data) != "echo team gradle\n" {
		t.Fatalf("the template's install script is not built in: %v", sortedKeys(install))
	}
	if text, err := p.InstructionText(nil); err != nil || !strings.Contains(text, "My notes.") {
		t.Fatalf("%q %v", text, err)
	}
	// Nothing of the template went into the user's _shared.
	if _, err := os.Stat(filepath.Join(root, "_shared", "@install", "team")); err == nil {
		t.Fatal("the template's script landed in the user's _shared")
	}
	if _, err := os.Stat(filepath.Join(root, "sandboxes", "dev", "_data", "claude", "settings.json")); err != nil {
		t.Fatal("the profile's own files were not copied")
	}
	// A second copy needs a name of its own; a template of another source
	// does not take over the name.
	if _, err = tpl.CopyProfile(root, "dev", "team", "acme"); err == nil {
		t.Fatal("a profile was overwritten")
	}
	other := templateFixture(t)
	tpl2, _ := FetchTemplate(ctx, other)
	if _, err = tpl2.CopyProfile(root, "dev2", "team", "acme"); err == nil || !strings.Contains(err.Error(), "--template") {
		t.Fatalf("%v", err)
	}
	// An update tells what changes in the template's shared files and how
	// a profile now differs from its own; the profile stays as it is.
	write(t, filepath.Join(src, "_shared", "@install", "team", "gradle.sh"), "echo newer gradle\n")
	write(t, filepath.Join(src, "_shared", "@install", "team", "node.sh"), "echo node\n")
	write(t, filepath.Join(src, "sandboxes", "team", "sandbox.dockerfile"), "FROM example/shell\nCOPY --from=unibox / /\nRUN @install team/gradle team/node\n")
	changes, err := tpl.SharedChanges(root, "acme")
	if err != nil || !slices.Equal(changes.Changed, []string{"@install/team/gradle.sh"}) || !slices.Equal(changes.Added, []string{"@install/team/node.sh"}) {
		t.Fatalf("%+v %v", changes, err)
	}
	if err = tpl.WriteShared(root, "acme"); err != nil {
		t.Fatal(err)
	}
	p, _ = LoadProfile(root, "dev")
	install, _ = p.InstallContext()
	if string(install["usr/local/lib/unibox/install/team/gradle.sh"].Data) != "echo newer gradle\n" {
		t.Fatal("the update did not reach the build")
	}
	differ, err := tpl.ProfileDiff(root, p)
	if err != nil || !slices.Equal(differ, []string{"sandbox.dockerfile"}) {
		t.Fatalf("%v %v", differ, err)
	}
	if names := TemplateProfiles(root, "acme"); len(names) != 1 || names[0].Name != "dev" {
		t.Fatalf("%v", names)
	}
	// Inspecting tells what a copy would need, and how the template stands
	// to the repository, writing nothing.
	info, err := tpl.Inspect(root, "acme")
	if err != nil {
		t.Fatal(err)
	}
	team := info.Profiles[0]
	if info.Kept == nil || info.Kept.Source != src || !slices.Equal(info.Kept.Profiles, []string{"dev"}) || team.Exists || team.Error != "" || team.Vars["artifactory-user"] == "" || !slices.Equal(team.Secrets, []string{"artifactory"}) || len(team.Services) != 1 || team.Services[0].Git || !slices.Contains(info.Shared, "@install/team/node.sh") {
		t.Fatalf("%+v", info)
	}
}

func TestATemplateIsClonedFromGit(t *testing.T) {
	t.Parallel()
	src := templateFixture(t)
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}, {"-c", "user.name=T", "-c", "user.email=t@example.com", "commit", "-qm", "template"}} {
		if out, err := exec.Command("git", append([]string{"-C", src}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v", out, err)
		}
	}
	tpl, err := FetchTemplate(context.Background(), "file://"+src)
	if err != nil {
		t.Fatal(err)
	}
	defer tpl.Close()
	if len(tpl.Commit) != 40 || tpl.Source != "file://"+src {
		t.Fatalf("%+v", tpl)
	}
	if TemplateNameOf("git@gitlab.example.com:team/Agent_Templates.git") != "agent-templates" {
		t.Fatal(TemplateNameOf("git@gitlab.example.com:team/Agent_Templates.git"))
	}
}
