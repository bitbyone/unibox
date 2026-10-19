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
	if err := os.WriteFile(filepath.Join(root, "sandboxes", "node", "sandbox.yaml"), []byte("version: 6\nimage: example/image\nagents: [claude]\nvars:\n  team-user: your user name\n  team-email: your address\nenv:\n  - TEAM_USER: ${vars.team-user}\n  - TEAM_EMAIL: ${vars.team-email}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, paths, "", "var", "set", "team-user", "shared-user"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, paths, "", "var", "set", "team-user", "tobola", "--profile", "node"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, paths, "", "var", "set", "team-password", "x"); err == nil {
		t.Fatal("a secret was kept as a var")
	}
	out, _, err := run(t, paths, "", "var", "ls", "--profile", "node")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"team-user   tobola", "profile node", "team-email", "missing", "your address"} {
		if !strings.Contains(out, want) {
			t.Fatalf("lacks %q:\n%s", want, out)
		}
	}
	if _, _, err = run(t, paths, "", "var", "rm", "team-user", "--profile", "node"); err != nil {
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
