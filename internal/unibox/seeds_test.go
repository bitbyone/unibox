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
	// claude's own byline is off: unibox's hook says the co-author.
	if data, _ := os.ReadFile(filepath.Join(root, "sandboxes", "web", "_data", "claude.settings.json")); !strings.Contains(string(data), `"includeCoAuthoredBy": false`) {
		t.Fatalf("%s", data)
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

func TestTheCoAuthorHookSaysTheProfilesCoAuthorAlone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	hook := filepath.Join(dir, "commit-msg")
	mustWriteExecutable(t, hook, []byte(coAuthorHook()))
	run := func(agent, coAuthor, credit, message string) string {
		t.Helper()
		msg := filepath.Join(dir, "MSG")
		if err := os.WriteFile(msg, []byte(message), 0o600); err != nil {
			t.Fatal(err)
		}
		_ = os.Remove(filepath.Join(dir, "credit"))
		if credit != "" {
			if err := os.WriteFile(filepath.Join(dir, "credit"), []byte(credit+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		cmd := exec.Command("sh", hook, msg)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "UNIBOX_AGENT="+agent, "UNIBOX_CO_AUTHOR="+coAuthor)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
		b, _ := os.ReadFile(msg)
		return string(b)
	}
	trailers := func(s string) []string {
		out := []string{}
		for _, l := range strings.Split(s, "\n") {
			if strings.HasPrefix(strings.ToLower(l), "co-authored-by:") {
				out = append(out, l)
			}
		}
		return out
	}
	gh := "Copilot <223556219+Copilot@users.noreply.github.com>"
	own := "Add a file\n\nCo-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>\n"
	// Copilot's own trailer gives way to the profile's, whatever it says.
	if got := trailers(run("Copilot", gh, "on", own)); len(got) != 1 || got[0] != "Co-Authored-By: "+gh {
		t.Fatalf("%q", got)
	}
	if got := trailers(run("Copilot", "Copilot <>", "on", own)); len(got) != 1 || got[0] != "Co-Authored-By: Copilot <>" {
		t.Fatalf("%q", got)
	}
	// A renamed agent: the table's name and the profile's both count.
	if got := trailers(run("Claude", "Claude Code <cc@example.com>", "on", "Fix\n\nCo-Authored-By: Claude <noreply@anthropic.com>\n")); len(got) != 1 || got[0] != "Co-Authored-By: Claude Code <cc@example.com>" {
		t.Fatalf("%q", got)
	}
	// Crediting off means none, not the agent's own.
	if got := trailers(run("Copilot", gh, "off", own)); len(got) != 0 {
		t.Fatalf("%q", got)
	}
	// Another co-author is the user's, and stays.
	if out := run("Copilot", gh, "on", "Fix\n\nCo-authored-by: Jan <jan@example.com>\n"); len(trailers(out)) != 2 {
		t.Fatalf("%q", out)
	}
	// Twice is once.
	once := run("Copilot", gh, "on", "Fix\n")
	if again := run("Copilot", gh, "on", once); again != once {
		t.Fatalf("%q\n%q", once, again)
	}
	// No agent running - a commit made by hand - is left as it is.
	if got := run("", "", "on", own); got != own {
		t.Fatalf("%q", got)
	}
}

func TestAnOldHookIsRewrittenBeforeASession(t *testing.T) {
	t.Parallel()
	m, f := managerFixture(t)
	ctx := context.Background()
	b, err := m.Create(ctx, Request{NewSandbox: "dev", Profile: "node"})
	if err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(f.root, "unibox-dev", "work", "task", "repo", ".git", "unibox-hooks", "commit-msg")
	write(t, hook, "#!/bin/sh\n# an agent fixed in it\n")
	p, _ := m.profile("node")
	if err := m.ensure(ctx, &b, f, p, false, false); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(hook); !strings.Contains(string(data), "UNIBOX_CO_AUTHOR") {
		t.Fatalf("%s", data)
	}
}

func TestTheSessionCreditsTheAgentAsTheProfileSays(t *testing.T) {
	t.Parallel()
	root := profileFixture(t)
	write(t, filepath.Join(root, "sandboxes", "node", "sandbox.yaml"), "dockerfile: Dockerfile\nstart: start.sh\nagents: [claude, codex]\ngit:\n  co_authors:\n    claude: Claude Code <claude@example.com>\n")
	p, err := LoadProfile(root, "node")
	if err != nil {
		t.Fatal(err)
	}
	if p.CoAuthor("claude") != "Claude Code <claude@example.com>" || p.CoAuthor("codex") != "Codex <noreply@openai.com>" {
		t.Fatalf("%s %s", p.CoAuthor("claude"), p.CoAuthor("codex"))
	}
	// Credited at commit time, so a change needs no recreation.
	other := p
	other.Git.CoAuthors = nil
	if string(settingsSnapshot(p, &fakeRuntime{})) != string(settingsSnapshot(other, &fakeRuntime{})) {
		t.Fatal("a co-author change would recreate the sandbox")
	}
	write(t, filepath.Join(root, "sandboxes", "node", "sandbox.yaml"), "dockerfile: Dockerfile\nstart: start.sh\nagents: [claude, codex]\ngit: {co_authors: {copilot: Copilot <c@example.com>}}\n")
	if _, err := LoadProfile(root, "node"); err == nil {
		t.Fatal("an agent the profile lacks was credited")
	}
}
