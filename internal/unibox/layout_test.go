package unibox

import (
	"os"
	"path/filepath"
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
	write(t, file, "# Mine, from before.\ndockerfile: Dockerfile\nagents:\n  claude:\n    config: {.claude.json: _data/mine.json}\n  codex: {co_author: Codex Bot <bot@example.com>}\ncpus: 2\nmemory: 1g\nstart: start.sh\nallow_network: [jitpack.io] # Kotlin\n")
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
	if _, err := os.Stat(filepath.Join(root, "sandboxes", "node", "_data", "claude", "claude.json")); err == nil {
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
