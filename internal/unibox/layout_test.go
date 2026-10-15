package unibox

import (
	"os"
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
	if _, err := Init(root, "web", []string{"claude", "codex"}, 2, "4g"); err != nil {
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
		SetStringList(doc, "allow_network", []string{"jitpack.io"})
		SetService(doc, "github", Service{Git: true, Host: HostList{"github.com"}, User: "x-access-token", Command: "secret:github"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(file)
	s := string(data)
	tail = strings.Index(s, footerMarker)
	for _, key := range []string{"\nservices:", "\nallow_network: [jitpack.io]"} {
		if i := strings.Index(s, key); i < 0 || i > tail {
			t.Fatalf("%q is not above the examples:\n%s", key, s)
		}
	}
	if strings.Contains(s[tail:], "# services:") || strings.Contains(s[tail:], "# allow_network:") || !strings.Contains(s[tail:], "# deny_network:") {
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
	write(t, file, "version: 1\n# Mine, from before.\ndockerfile: Dockerfile\nagents:\n  claude:\n    config: {.claude.json: _data/mine.json}\n  codex: {co_author: Codex Bot <bot@example.com>}\ncpus: 2\nmemory: 1g\nstart: start.sh\nallow_network: [jitpack.io] # Kotlin\n")
	// A repository init laid out already, as a real one is.
	if _, err := Init(root, "", nil, 0, ""); err != nil {
		t.Fatal(err)
	}
	before, err := LoadProfile(root, "node")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Init(root, "node", []string{"copilot"}, 2, "1g"); err == nil || !strings.Contains(err.Error(), "does not change a profile's agents") {
		t.Fatalf("%v", err)
	}
	report, err := Init(root, "node", nil, 4, "8g")
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
	if p.CPUs != 2 || p.Memory != "1g" || len(p.Agents) != 2 {
		t.Fatalf("init changed what the profile says: %d %s %v", p.CPUs, p.Memory, p.Agents)
	}
	data, _ := os.ReadFile(file)
	if !strings.Contains(string(data), "# Mine, from before.") || !strings.Contains(string(data), "allow_network: [jitpack.io] # Kotlin") || !strings.Contains(string(data), footerMarker) {
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
	again, err := Init(root, "node", nil, 4, "8g")
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
		"":             "is of format none and this unibox reads format 1; run unibox init node to migrate it",
		"version: 0\n": "is of format 0 and this unibox reads format 1; run unibox init node",
		"version: 2\n": "is of format 2, newer than the 1 this unibox reads; upgrade unibox",
	} {
		// An older form fails on its version, not on a field it has.
		write(t, file, version+"dockerfile: Dockerfile\nagents: [claude]\nstart: start.sh\nauth: {claude: oauth}\n")
		if _, err := LoadProfile(root, "node"); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%q: %v", version, err)
		}
	}
	// A newer one init leaves alone.
	write(t, file, "version: 2\ndockerfile: Dockerfile\nagents: [claude]\nstart: start.sh\n")
	if _, err := Init(root, "node", nil, 2, "1g"); err == nil || !strings.Contains(err.Error(), "upgrade unibox") {
		t.Fatalf("%v", err)
	}

	// init migrates format 0: what it said in three sections moves under
	// each agent, comments stay, and what has no place is said.
	write(t, filepath.Join(root, "sandboxes", "node", "_data", "c.json"), "{}\n")
	write(t, filepath.Join(root, "sandboxes", "node", "_data", "claude.settings.json"), "{\"mine\": 1}\n")
	write(t, file, `# Node work.
dockerfile: Dockerfile
start: start.sh
agents: [claude, copilot]
auth:
  claude: {token: secret:claude} # mine
  codex: oauth
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
	report, err := Init(root, "node", nil, 2, "1g")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(report.Migrated, "|") != "sandboxes/node/sandbox.yaml from format 0 to 1|auth codex left out|agent_config copilot.theme left out" {
		t.Fatalf("%q", report.Migrated)
	}
	p, err := LoadProfile(root, "node")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(file)
	for _, want := range []string{"version: 1\n# Node work.\ndockerfile", "auth: {token: 'secret:claude'} # mine", ".claude.json: _data/c.json", "co_author: C <c@example.com>", "co_author: Cop <>", "author: user", "# Mine, after.", "settings.json: _data/claude/settings.json"} {
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
	if report, err = Init(root, "node", nil, 2, "1g"); err != nil || len(report.Migrated) != 0 {
		t.Fatalf("%v %q", err, report.Migrated)
	}

	// What does not load after migrating is not kept.
	broken := "dockerfile: Dockerfile\nstart: start.sh\nagents: [claude]\nmemmory: 1g\n"
	write(t, file, broken)
	if _, err := Init(root, "node", nil, 2, "1g"); err == nil || !strings.Contains(err.Error(), "did not migrate") {
		t.Fatalf("%v", err)
	}
	if data, _ := os.ReadFile(file); string(data) != broken {
		t.Fatalf("%s", data)
	}

	// What init writes is of this format.
	if _, err := Init(root, "fresh", []string{"claude"}, 2, "1g"); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(root, "sandboxes", "fresh", "sandbox.yaml")); !strings.Contains(string(data), "\nversion: 1\n") {
		t.Fatalf("%s", data)
	}
}
