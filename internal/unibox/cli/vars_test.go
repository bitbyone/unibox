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
	data, _ := os.ReadFile(filepath.Join(paths.Config, "vars.toml"))
	if !strings.HasPrefix(string(data), "# Your own values") || !strings.Contains(string(data), "[global]\nteam-user = 'shared-user'") {
		t.Fatalf("%s", data)
	}
}

func TestVarsKeptInYAMLMoveIntoTOML(t *testing.T) {
	t.Parallel()
	paths, _ := cliFixture(t)
	old := filepath.Join(paths.Config, "vars.yaml")
	if err := os.WriteFile(old, []byte("vars:\n  team-user: shared-user\nprofiles:\n  node:\n    team-user: tobola\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _, err := run(t, paths, "", "var", "ls")
	if err != nil || !strings.Contains(out, "tobola       profile node") || !strings.Contains(out, "shared-user  every profile") {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, err = os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("vars.yaml stayed: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(paths.Config, "vars.toml"))
	for _, want := range []string{"[global]\nteam-user = 'shared-user'", "[profiles.node]\nteam-user = 'tobola'"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("no %q in\n%s", want, data)
		}
	}
	// A key of neither table is named, not ignored.
	if err = os.WriteFile(filepath.Join(paths.Config, "vars.toml"), []byte("team-user = 'x'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = run(t, paths, "", "var", "ls"); err == nil || !strings.Contains(err.Error(), "[global]") {
		t.Fatalf("%v", err)
	}
}
