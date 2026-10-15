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
	text := strings.Replace(string(fresh), "agent_config:", "# Mine.\nagent_config:", 1)
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
	if !strings.Contains(s, "# Mine.\nagent_config:") || strings.Count(s, footerMarker) != 1 {
		t.Fatalf("%s", s)
	}
	// Laid out again, it does not change.
	again, err := arrangeText(data, "web")
	if err != nil || string(again) != s {
		t.Fatalf("%v\n%s\n---\n%s", err, s, again)
	}
}
