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

func TestAnUpdateMergesTheTemplatesChangesWithTheUsersAllOrNothing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	if _, err := Init(root, "", nil, "", ""); err != nil {
		t.Fatal(err)
	}
	src := templateFixture(t)
	docker := filepath.Join(src, "sandboxes", "team", "sandbox.dockerfile")
	write(t, docker, "FROM example/shell\nCOPY --from=unibox / /\nRUN @install team/gradle\nRUN echo one\nRUN echo two\n")
	tpl, _ := FetchTemplate(ctx, src)
	if _, err := tpl.CopyProfile(root, "dev", "team", "acme"); err != nil {
		t.Fatal(err)
	}
	mine := filepath.Join(root, "sandboxes", "dev", "sandbox.dockerfile")
	// The user adds a line at the end; the template changes its base image
	// and its shared script: both merge.
	write(t, mine, "FROM example/shell\nCOPY --from=unibox / /\nRUN @install team/gradle\nRUN echo one\nRUN echo two\nRUN echo mine\n")
	write(t, docker, "FROM example/shell:2\nCOPY --from=unibox / /\nRUN @install team/gradle\nRUN echo one\nRUN echo two\n")
	write(t, filepath.Join(src, "_shared", "@install", "team", "gradle.sh"), "echo gradle 2\n")
	plan, err := tpl.PlanUpdate(ctx, root, "acme")
	if err != nil || plan.Conflicted() {
		t.Fatalf("%+v %v", plan, err)
	}
	if err = plan.Apply(root, tpl); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(mine); string(data) != "FROM example/shell:2\nCOPY --from=unibox / /\nRUN @install team/gradle\nRUN echo one\nRUN echo two\nRUN echo mine\n" {
		t.Fatalf("%s", data)
	}
	if data, _ := os.ReadFile(filepath.Join(root, ".templates", "acme", "@install", "team", "gradle.sh")); string(data) != "echo gradle 2\n" {
		t.Fatalf("%s", data)
	}
	// Both change the same line: nothing is written - neither the profile
	// nor the shared files - and what resolves it is left beside the
	// template.
	write(t, mine, "FROM example/shell:2\nCOPY --from=unibox / /\nRUN @install team/gradle\nRUN echo ONE mine\nRUN echo two\nRUN echo mine\n")
	write(t, docker, "FROM example/shell:2\nCOPY --from=unibox / /\nRUN @install team/gradle\nRUN echo one template\nRUN echo two\n")
	write(t, filepath.Join(src, "_shared", "@install", "team", "gradle.sh"), "echo gradle 3\n")
	plan, err = tpl.PlanUpdate(ctx, root, "acme")
	if err != nil || !plan.Conflicted() {
		t.Fatalf("%+v %v", plan, err)
	}
	if err = plan.Apply(root, tpl); err == nil {
		t.Fatal("a conflicted update was written")
	}
	written, err := plan.WriteConflicts(root)
	if err != nil || len(written) != 2 {
		t.Fatalf("%v %v", written, err)
	}
	merge, _ := os.ReadFile(filepath.Join(root, ".templates", "acme", "conflicts", "dev", "sandbox.dockerfile.merge"))
	if !strings.Contains(string(merge), "<<<<<<< yours") || !strings.Contains(string(merge), "RUN echo mine") {
		t.Fatalf("%s", merge)
	}
	if _, err := LoadProfile(root, "dev"); err != nil {
		t.Fatalf("the profile stopped loading: %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(root, ".templates", "acme", "@install", "team", "gradle.sh")); string(data) != "echo gradle 2\n" {
		t.Fatal("the shared files went ahead of the profile")
	}
	// Resolved, the next update goes through and clears what was left.
	write(t, mine, "FROM example/shell:2\nCOPY --from=unibox / /\nRUN @install team/gradle\nRUN echo one template\nRUN echo two\nRUN echo mine\n")
	plan, err = tpl.PlanUpdate(ctx, root, "acme")
	if err != nil || plan.Conflicted() {
		t.Fatalf("%+v %v", plan, err)
	}
	if err = plan.Apply(root, tpl); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".templates", "acme", "conflicts")); err == nil {
		t.Fatal("the conflicts stayed")
	}
	// A profile without a base cannot tell whose a difference is.
	if err := os.RemoveAll(filepath.Join(root, ".templates", "acme", "bases", "dev")); err != nil {
		t.Fatal(err)
	}
	write(t, docker, "FROM example/shell:3\nCOPY --from=unibox / /\nRUN @install team/gradle\nRUN echo one template\nRUN echo two\n")
	if plan, _ = tpl.PlanUpdate(ctx, root, "acme"); !plan.Conflicted() || !plan.Profiles[0].NoBase {
		t.Fatalf("%+v", plan)
	}
}
