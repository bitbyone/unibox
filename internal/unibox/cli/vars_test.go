package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVarsAreSetSharedOrForOneProfileAndListedWithWhatTheyAreFor(t *testing.T) {
	t.Parallel()
	paths, root := cliFixture(t)
	if err := os.WriteFile(filepath.Join(root, "sandboxes", "node", "sandbox.yaml"), []byte("version: 5\nimage: example/image\nagents: [claude]\nvars:\n  TEAM_USER: your user name\n  TEAM_EMAIL: your address\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, paths, "", "var", "set", "TEAM_USER", "shared-user"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, paths, "", "var", "set", "TEAM_USER", "tobola", "--profile", "node"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, paths, "", "var", "set", "TEAM_PASSWORD", "x"); err == nil {
		t.Fatal("a secret was kept as a var")
	}
	out, _, err := run(t, paths, "", "var", "ls", "--profile", "node")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"TEAM_USER   tobola", "profile node", "TEAM_EMAIL", "missing", "your address"} {
		if !strings.Contains(out, want) {
			t.Fatalf("lacks %q:\n%s", want, out)
		}
	}
	if _, _, err = run(t, paths, "", "var", "rm", "TEAM_USER", "--profile", "node"); err != nil {
		t.Fatal(err)
	}
	if out, _, _ = run(t, paths, "", "var", "ls"); !strings.Contains(out, "shared-user") || strings.Contains(out, "tobola") {
		t.Fatalf("%s", out)
	}
	data, _ := os.ReadFile(filepath.Join(paths.Config, "vars.yaml"))
	if !strings.HasPrefix(string(data), "# Your own values") {
		t.Fatalf("%s", data)
	}
}
