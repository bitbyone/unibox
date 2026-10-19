package unibox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestAProfileIsLaidOutTheSameWhoeverWroteIt(t *testing.T) {
	t.Parallel()
	m, _ := managerFixture(t)
	root := m.Config.Environments
	if _, err := Init(root, "web", []string{"claude", "codex"}, "2", "4g"); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "sandboxes", "web", "sandbox.yaml")
	fresh, _ := os.ReadFile(file)
	tail := strings.Index(string(fresh), footerMarker)
	if tail < 0 || !strings.Contains(string(fresh)[tail:], "# services:") || strings.Contains(string(fresh)[tail:], "# git:") {
		t.Fatalf("%s", fresh)
	}
	// The user's own comment over a section is theirs.
	text := strings.Replace(string(fresh), "\ninstructions:", "\n# Mine.\ninstructions:", 1)
	if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.EditProfile("web", func(doc *yaml.Node) error {
		SetNetworkList(doc, "allow", []string{"jitpack.io"})
		SetService(doc, "github", Service{Git: true, Host: HostList{"github.com"}, User: "x-access-token", Command: "secret:github"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(file)
	s := string(data)
	tail = strings.Index(s, footerMarker)
	for _, key := range []string{"\nservices:", "\nnetwork:\n  allow: [jitpack.io]"} {
		if i := strings.Index(s, key); i < 0 || i > tail {
			t.Fatalf("%q is not above the examples:\n%s", key, s)
		}
	}
	// vars and env are shown where they go, above agents, not in the tail.
	if strings.Contains(s[tail:], "# services:") || strings.Contains(s[tail:], "# network:") || strings.Contains(s[tail:], "# env:") || !strings.Contains(s[:strings.Index(s, "\nagents:")], "# env:") {
		t.Fatalf("the examples were not recomputed:\n%s", s)
	}
	if !strings.Contains(s, "# Mine.\ninstructions:") || strings.Count(s, footerMarker) != 1 {
		t.Fatalf("%s", s)
	}
	// Laid out again, it does not change.
	again, err := arrangeText(data, "web")
	if err != nil || string(again) != s {
		t.Fatalf("%v\n%s\n---\n%s", err, s, again)
	}
}

func TestInitCompletesAnExistingProfileAndKeepsWhatItSays(t *testing.T) {
	t.Parallel()
	root := profileFixture(t)
	file := filepath.Join(root, "sandboxes", "node", "sandbox.yaml")
	write(t, filepath.Join(root, "sandboxes", "node", "_data", "mine.json"), `{"theme": "light"}`)
	write(t, file, "version: 6\n# Mine, from before.\nagents:\n  claude:\n    config: {.claude.json: _data/mine.json}\n  codex: {co_author: Codex Bot <bot@example.com>}\nresources: {cpus: 2, memory: 1g}\nstart: start.sh\nnetwork:\n  allow: [jitpack.io] # Kotlin\n")
	// A repository init laid out already, as a real one is.
	if _, err := Init(root, "", nil, "0", ""); err != nil {
		t.Fatal(err)
	}
	before, err := LoadProfile(root, "node")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Init(root, "node", []string{"copilot"}, "2", "1g"); err == nil || !strings.Contains(err.Error(), "does not change a profile's agents") {
		t.Fatalf("%v", err)
	}
	report, err := Init(root, "node", nil, "4", "8g")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Completed) != 1 {
		t.Fatalf("%+v", report)
	}
	p, err := LoadProfile(root, "node")
	if err != nil {
		t.Fatal(err)
	}
	// Its own seed and co-author stay; what it lacked is added.
	if p.AgentConfig["claude/.claude.json"] != "_data/mine.json" || p.AgentConfig["claude/settings.json"] != "_data/claude/settings.json" || p.AgentConfig["codex/config.toml"] != "_data/codex/config.toml" {
		t.Fatalf("%v", p.AgentConfig)
	}
	if p.Git.CoAuthors["codex"] != "Codex Bot <bot@example.com>" || p.Git.CoAuthors["claude"] != "Claude <noreply@anthropic.com>" {
		t.Fatalf("%v", p.Git.CoAuthors)
	}
	if p.CPUs != "2" || p.Memory != "1g" || len(p.Agents) != 2 {
		t.Fatalf("init changed what the profile says: %s %s %v", p.CPUs, p.Memory, p.Agents)
	}
	data, _ := os.ReadFile(file)
	if !strings.Contains(string(data), "# Mine, from before.") || !strings.Contains(string(data), "  allow: [jitpack.io] # Kotlin") || !strings.Contains(string(data), footerMarker) {
		t.Fatalf("%s", data)
	}
	if _, err := os.Stat(filepath.Join(root, "sandboxes", "node", "_data", "claude", ".claude.json")); err == nil {
		t.Fatal("a seed the profile does not name was written")
	}
	// Seeds are not the image's inputs: nothing to rebuild.
	if p.Revision != before.Revision {
		t.Fatal("completing the profile outdated its image")
	}
	// Complete already: a second init changes nothing.
	again, err := Init(root, "node", nil, "4", "8g")
	if err != nil || len(again.Completed) != 0 {
		t.Fatalf("%+v %v", again, err)
	}
	if after, _ := os.ReadFile(file); string(after) != string(data) {
		t.Fatal("a second init changed the profile")
	}
}

func TestAProfileOfAnotherFormatIsRefusedUntilInitMigratesIt(t *testing.T) {
	t.Parallel()
	root := profileFixture(t)
	file := filepath.Join(root, "sandboxes", "node", "sandbox.yaml")
	for version, want := range map[string]string{
		"":             "is of format none and this unibox reads format 6; run unibox init node to migrate it",
		"version: 0\n": "is of format 0 and this unibox reads format 6; run unibox init node",
		"version: 5\n": "is of format 5 and this unibox reads format 6; run unibox init node",
		"version: 7\n": "is of format 7, newer than the 6 this unibox reads; upgrade unibox",
	} {
		// An older form fails on its version, not on a field it has.
		write(t, file, version+"dockerfile: Dockerfile\nagents: [claude]\nstart: start.sh\nauth: {claude: oauth}\n")
		if _, err := LoadProfile(root, "node"); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%q: %v", version, err)
		}
	}
	// A newer one init leaves alone.
	write(t, file, "version: 7\nagents: [claude]\nstart: start.sh\n")
	if _, err := Init(root, "node", nil, "2", "1g"); err == nil || !strings.Contains(err.Error(), "upgrade unibox") {
		t.Fatalf("%v", err)
	}

	// init migrates format 0, one format at a time: what it said in three
	// sections moves under each agent, cpus and memory under resources,
	// comments stay, and what has no place is said.
	write(t, filepath.Join(root, "sandboxes", "node", "_data", "c.json"), "{}\n")
	write(t, filepath.Join(root, "sandboxes", "node", "_data", "claude.settings.json"), "{\"mine\": 1}\n")
	write(t, file, `# Node work.
dockerfile: Dockerfile
start: start.sh
agents: [claude, copilot]
auth:
  claude: {token: secret:claude} # mine
  codex: oauth
cpus: 6 # mine
memory: 8g
git:
  author: user
  co_authors: {claude: C <c@example.com>, copilot: Cop <>}
agent_config:
  claude: _data/c.json
  claude.settings: _data/claude.settings.json
  copilot.theme: _data/c.json
# How each agent signs in: unibox login node writes this
# auth:
#   codex: oauth
# deny_network: [gist.github.com]
# Mine, after.
instructions: [unibox]
`)
	report, err := Init(root, "node", nil, "2", "1g")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(report.Migrated, "|") != "sandboxes/node/sandbox.yaml from format 0 to 6|auth codex left out|agent_config copilot.theme left out" {
		t.Fatalf("%q", report.Migrated)
	}
	p, err := LoadProfile(root, "node")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(file)
	for _, want := range []string{"# Node work.\nstart: start.sh", "auth: {token: 'secret:claude'} # mine", ".claude.json: _data/c.json", "co_author: C <c@example.com>", "co_author: Cop <>", "author: user", "# Mine, after.", "settings.json: _data/claude/settings.json", "resources: {cpus: 6, memory: 8g} # mine"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("no %q in\n%s", want, data)
		}
	}
	if moved, _ := os.ReadFile(filepath.Join(root, "sandboxes", "node", "_data", "claude", "settings.json")); string(moved) != "{\"mine\": 1}\n" {
		t.Fatalf("%s", moved)
	}
	if strings.Contains(string(data), "How each agent signs in") {
		t.Fatalf("%s", data)
	}
	if !slices.Equal(p.Agents, []string{"claude", "copilot"}) || p.Auth["claude"].Method != AuthToken {
		t.Fatalf("%+v", p)
	}
	// Migrated once, it is of this format.
	if report, err = Init(root, "node", nil, "2", "1g"); err != nil || len(report.Migrated) != 0 {
		t.Fatalf("%v %q", err, report.Migrated)
	}

	// What does not load after migrating is not kept.
	broken := "dockerfile: Dockerfile\nstart: start.sh\nagents: [claude]\nmemmory: 1g\n"
	write(t, file, broken)
	if _, err := Init(root, "node", nil, "2", "1g"); err == nil || !strings.Contains(err.Error(), "did not migrate") {
		t.Fatalf("%v", err)
	}
	if data, _ := os.ReadFile(file); string(data) != broken {
		t.Fatalf("%s", data)
	}

	// What init writes is of this format.
	if _, err := Init(root, "fresh", []string{"claude"}, "2", "1g"); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(root, "sandboxes", "fresh", "sandbox.yaml")); !strings.Contains(string(data), "\nversion: 6\n") {
		t.Fatalf("%s", data)
	}
}

func TestManagedFilesAreKeptCurrentOnlyWhereInitLaidTheRepositoryOut(t *testing.T) {
	t.Parallel()
	m, _ := managerFixture(t)
	ctx := context.Background()
	root := m.Config.Environments
	if err := m.Build(ctx, "node", "fake", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "_shared")); err == nil {
		t.Fatal("unibox wrote into a repository init did not lay out")
	}
	// Laid out, with a managed file edited: the next command rewrites it.
	if _, _, _, err := SyncManaged(root); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, ".shared", "@install", "agents", "claude.sh")
	original, _ := os.ReadFile(file)
	write(t, file, "echo edited\n")
	again, err := Open(m.Paths, m.Log)
	if err != nil {
		t.Fatal(err)
	}
	again.Config.Environments, again.Runtimes = root, m.Runtimes
	if _, err := again.Check(ctx, Request{Profile: "node", Repositories: []RepoInput{{Source: "https://forge.invalid/x.git", Branch: "feat/x"}}}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(file); string(b) != string(original) {
		t.Fatalf("%s", b)
	}
	if !strings.Contains(m.Log.(interface{ String() string }).String(), "updated .shared/@install/agents/claude.sh to unibox's own") {
		t.Fatalf("the rewrite was not said: %s", m.Log.(interface{ String() string }).String())
	}
}

func TestInitMovesARepositoryFromTheOlderLayout(t *testing.T) {
	t.Parallel()
	root := profileFixture(t)
	managed, err := managedFiles()
	if err != nil {
		t.Fatal(err)
	}
	// What an older init wrote: the runner, unibox's scripts and text as
	// they were, one script edited, one of the user's, a text of theirs.
	write(t, filepath.Join(root, "_shared", ".install"), "#!/bin/sh\n")
	write(t, filepath.Join(root, "_shared", ".unibox-managed"), "_shared/.install\n")
	write(t, filepath.Join(root, "_shared", "installations", "base.sh"), withoutHeader(managed[".shared/@install/base.sh"]))
	write(t, filepath.Join(root, "_shared", "installations", "agents", "claude.sh"), "#!/bin/sh\necho my claude\n")
	write(t, filepath.Join(root, "_shared", "installations", "tools", "own.sh"), "#!/bin/sh\n")
	write(t, filepath.Join(root, "instructions", "unibox.md"), string(managed[".shared/@instructions/unibox.md"]))
	write(t, filepath.Join(root, "instructions", "team.md"), "team")
	write(t, filepath.Join(root, "sandboxes", "node", "sandbox.dockerfile"), "FROM x\n"+generatedRunLines[0]+"\nbase\nEOF\n")
	if _, err := LoadProfile(root, "node"); err == nil || !strings.Contains(err.Error(), "run unibox init") {
		t.Fatalf("%v", err)
	}
	report, err := Init(root, "", nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	for rel, want := range map[string]bool{
		"_shared/.install": false, "_shared/.unibox-managed": false, "_shared/installations": false, "instructions": false,
		"_shared/@install/agents/claude.sh": true, "_shared/@install/tools/own.sh": true, "_shared/@install/base.sh": false,
		"_shared/@instructions/team.md": true, "_shared/@instructions/unibox.md": false, ".shared/@install/base.sh": true,
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); (err == nil) != want {
			t.Errorf("%s: %v", rel, err)
		}
	}
	if data, _ := os.ReadFile(filepath.Join(root, "sandboxes", "node", "sandbox.dockerfile")); !strings.Contains(string(data), "COPY --from=unibox / /\nRUN @install <<EOF\nbase\nEOF\n") {
		t.Fatalf("%s", data)
	}
	joined := strings.Join(report.Relaid, "\n")
	if !strings.Contains(joined, "moved _shared/installations/agents/claude.sh to _shared/@install/agents/claude.sh, which differs from unibox's and now wins over it") || !strings.Contains(joined, "sandboxes/node/sandbox.dockerfile runs @install") {
		t.Fatalf("%s", joined)
	}
	if _, err := LoadProfile(root, "node"); err != nil {
		t.Fatal(err)
	}
}

func TestInstallRunsTheUsersScriptOverUnibox(t *testing.T) {
	t.Parallel()
	root := profileFixture(t)
	if _, _, _, err := SyncManaged(root); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, ".shared", "@install", "x.sh"), "echo from unibox\n")
	write(t, filepath.Join(root, "_shared", "@install", "x.sh"), "echo mine\n")
	write(t, filepath.Join(root, ".shared", "@install", "only.sh"), "echo only from unibox\n")
	p, err := LoadProfile(root, "node")
	if err != nil {
		t.Fatal(err)
	}
	install, err := p.InstallContext()
	if err != nil {
		t.Fatal(err)
	}
	if string(install["usr/local/lib/unibox/install/x.sh"].Data) != "echo mine\n" || install["usr/local/bin/@install"].Mode != 0o755 || install["usr/local/lib/unibox/install/agents/claude.sh"].Data == nil {
		t.Fatalf("%v", install)
	}
	// The runner, pointed at the merged scripts, runs them by name.
	dir := t.TempDir()
	for name, file := range install {
		if err := AtomicWrite(filepath.Join(dir, filepath.FromSlash(name)), file.Data, os.FileMode(file.Mode)); err != nil {
			t.Fatal(err)
		}
	}
	runner := strings.Replace(string(install["usr/local/bin/@install"].Data), "dir=/usr/local/lib/unibox/install", "dir="+filepath.Join(dir, "usr/local/lib/unibox/install"), 1)
	out, err := exec.Command("sh", "-c", runner, "@install", "x", "only.sh").CombinedOutput()
	if err != nil || !strings.Contains(string(out), "mine") || strings.Contains(string(out), "\nfrom unibox") || !strings.Contains(string(out), "only from unibox") {
		t.Fatalf("%v\n%s", err, out)
	}
	if out, err = exec.Command("sh", "-c", runner, "@install", "absent").CombinedOutput(); err == nil || !strings.Contains(string(out), "no installation script absent") {
		t.Fatalf("%v\n%s", err, out)
	}
	// A change of the user's script outdates the image.
	write(t, filepath.Join(root, "_shared", "@install", "x.sh"), "echo mine, changed\n")
	if changed, err := LoadProfile(root, "node"); err != nil || changed.Revision == p.Revision {
		t.Fatalf("%v", err)
	}
}

func TestTheNetworkCommentShowsHowToWritePortsAndReplacesTheFormerOne(t *testing.T) {
	t.Parallel()
	var doc yaml.Node
	text := "version: 6\nagents: [claude]\n" + strings.ReplaceAll(formerComments[0], "\\n", "\n") + "\nnetwork: {allow: [jitpack.io]}\n"
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		t.Fatal(err)
	}
	ArrangeProfile(&doc, "web")
	out, err := encodeProfile(&doc)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, "for now.") || !strings.Contains(s, "#     web: '3000:3000'") || !strings.Contains(s, "devtools: {port: '9333:9222', default: off}") {
		t.Fatalf("%s", s)
	}
	// Absent, it is an example in the footer, ports and all.
	if err := yaml.Unmarshal([]byte("version: 6\nagents: [claude]\n"), &doc); err != nil {
		t.Fatal(err)
	}
	ArrangeProfile(&doc, "web")
	out, _ = encodeProfile(&doc)
	if !strings.Contains(string(out), "#   ports:") || !strings.Contains(string(out), "#     api: '8080'") {
		t.Fatalf("%s", out)
	}
}
