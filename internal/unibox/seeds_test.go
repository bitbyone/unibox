package unibox

import (
	"context"
	"encoding/json"
	"os"
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
