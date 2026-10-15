package unibox

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitSeedsEveryAgentItCanAndTheProfileNamesThem(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if _, err := Init(root, "web", []string{"claude", "codex", "copilot", "opencode"}, 2, "4g"); err != nil {
		t.Fatal(err)
	}
	p, err := LoadProfile(root, "web")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"claude", "copilot", "opencode"} {
		if p.AgentConfig[id] != "_data/"+id+".json" {
			t.Fatalf("%s is not seeded: %v", id, p.AgentConfig)
		}
		data, err := os.ReadFile(filepath.Join(root, "sandboxes", "web", "_data", id+".json"))
		var seed map[string]any
		if err != nil || json.Unmarshal(data, &seed) != nil {
			t.Fatalf("%s: %v %s", id, err, data)
		}
	}
	// copilot keeps its theme in a file of its own.
	if p.AgentConfig["copilot.settings"] != "_data/copilot.settings.json" {
		t.Fatalf("%v", p.AgentConfig)
	}
	if data, _ := os.ReadFile(filepath.Join(root, "sandboxes", "web", "_data", "copilot.settings.json")); !strings.Contains(string(data), `"theme": "default"`) {
		t.Fatalf("%s", data)
	}
	if a, path, err := ConfigPath("copilot.settings"); err != nil || a.ID != "copilot" || path != ".copilot/settings.json" {
		t.Fatalf("%v %s %v", a.ID, path, err)
	}
	if _, _, err := ConfigPath("copilot.nothing"); err == nil {
		t.Fatal("an unknown file of an agent was taken")
	}
	// codex keeps TOML, set up before every session instead.
	if _, ok := p.AgentConfig["codex"]; ok {
		t.Fatal("codex was given a JSON seed")
	}
	claude, _ := os.ReadFile(filepath.Join(root, "sandboxes", "web", "_data", "claude.json"))
	if !strings.Contains(string(claude), `"hasCompletedOnboarding": true`) {
		t.Fatalf("%s", claude)
	}
	// A profile without such agents names none.
	if _, err := Init(root, "plain", []string{"codex"}, 2, "4g"); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(root, "sandboxes", "plain", "sandbox.yaml")); strings.Contains(string(data), "agent_config") {
		t.Fatalf("%s", data)
	}
}

func TestASeedKeepsWhatTheAgentAddedAndItsComments(t *testing.T) {
	t.Parallel()
	current := map[string]any{"trustedFolders": []any{"/work", "/tmp/mine"}, "firstLaunchAt": "x"}
	merged := mergeJSON(current, map[string]any{"trustedFolders": []any{"/work"}, "theme": "dark"})
	folders := merged["trustedFolders"].([]any)
	if len(folders) != 2 || folders[1] != "/tmp/mine" || merged["firstLaunchAt"] != "x" || merged["theme"] != "dark" {
		t.Fatalf("%v", merged)
	}
	head, body := splitJSONComments("// managed automatically\n// do not edit\n{\"a\": 1}\n")
	if head != "// managed automatically\n// do not edit\n" || strings.TrimSpace(body) != `{"a": 1}` {
		t.Fatalf("%q %q", head, body)
	}
	if head, body = splitJSONComments(`{"a": 1}`); head != "" || body != `{"a": 1}` {
		t.Fatalf("%q %q", head, body)
	}
}

func TestCopilotsCommentedConfigTakesItsSeed(t *testing.T) {
	t.Parallel()
	m, f := managerFixture(t)
	root := m.Config.Environments
	write(t, filepath.Join(root, "sandboxes", "node", "sandbox.yaml"), "dockerfile: Dockerfile\nagents: [claude, codex]\ncpus: 2\nmemory: 1g\nstart: start.sh\n")
	// copilot is not among the fixture's agents, so its seed is merged the
	// way a session does it.
	home := filepath.Join(f.root, "unibox-dev", "home")
	write(t, filepath.Join(home, ".copilot", "config.json"), "// User settings belong in settings.json.\n// This file is managed automatically.\n{\n  \"trustedFolders\": [\"/tmp/mine\"],\n  \"firstLaunchAt\": \"x\"\n}\n")
	b, err := m.Create(context.Background(), Request{NewSandbox: "dev", Profile: "node"})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := FindAgent("copilot")
	if err := m.mergeAgentConfig(context.Background(), b, f, a, a.Seed); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(home, ".copilot", "config.json"))
	head, body := splitJSONComments(string(data))
	var got map[string]any
	if head != "// User settings belong in settings.json.\n// This file is managed automatically.\n" || json.Unmarshal([]byte(body), &got) != nil {
		t.Fatalf("%s", data)
	}
	if folders := got["trustedFolders"].([]any); len(folders) != 2 || got["firstLaunchAt"] != "x" {
		t.Fatalf("%v", got)
	}
}

func TestTheCoAuthorHookAddsTheAgentOnce(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	hook := filepath.Join(dir, "commit-msg")
	mustWriteExecutable(t, hook, []byte(coAuthorHook("Copilot <223556219+Copilot@users.noreply.github.com>")))
	run := func(message string) string {
		t.Helper()
		msg := filepath.Join(dir, "MSG")
		if err := os.WriteFile(msg, []byte(message), 0o600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("sh", hook, msg)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
		b, _ := os.ReadFile(msg)
		return string(b)
	}
	count := func(s string) int { return strings.Count(strings.ToLower(s), "co-authored-by: copilot") }
	if got := run("Add a file\n"); count(got) != 1 || !strings.Contains(got, "Co-Authored-By: Copilot <223556219+Copilot@users.noreply.github.com>") {
		t.Fatalf("%q", got)
	}
	// The agent's own trailer, in its own case and address, is enough.
	if got := run("Add a file\n\nCo-authored-by: Copilot <someone@else.example>\n"); count(got) != 1 {
		t.Fatalf("doubled: %q", got)
	}
	// Another co-author is not the agent.
	if got := run("Add a file\n\nCo-authored-by: Jan <jan@example.com>\n"); count(got) != 1 || !strings.Contains(got, "Jan <jan@example.com>") {
		t.Fatalf("%q", got)
	}
	// Twice is once.
	once := run("Add a file\n")
	if again := run(once); again != once {
		t.Fatalf("%q\n%q", once, again)
	}
}
