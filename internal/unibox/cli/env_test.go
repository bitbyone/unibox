package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tobola/unibox/internal/unibox"
)

func runCLI(t *testing.T, paths unibox.Paths, args ...string) (string, error) {
	t.Helper()
	out, _, err := run(t, paths, "", args...)
	return out, err
}

func TestEnvsNameTheTeamsFilesAndShowWhatIsMissing(t *testing.T) {
	t.Parallel()
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	dir := t.TempDir()
	file := filepath.Join(dir, "dev.sbxenv.yaml")
	if err := os.WriteFile(file, []byte("name: team-dev\nagent: sbx-kit-copilot\nargs:\n  user: {required: true}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runCLI(t, paths, "env", "add", "dev", file)
	if err != nil || !strings.Contains(out, "sandbox team-dev") || !strings.Contains(out, "[user]") {
		t.Fatal(out, err)
	}
	cfg, _ := paths.LoadConfig()
	if cfg.DefaultEnv != "dev" || cfg.Envs["dev"].Files[0] != file {
		t.Fatal(cfg)
	}
	if out, err = runCLI(t, paths, "env", "show"); err != nil || !strings.Contains(out, "missing") {
		t.Fatal(out, err)
	}
	if _, err = runCLI(t, paths, "env", "add", "bad", filepath.Join(dir, "absent.yaml")); err == nil {
		t.Fatal("an absent file was added")
	}
	if _, err = runCLI(t, paths, "env", "rm", "dev"); err != nil {
		t.Fatal(err)
	}
	if cfg, _ = paths.LoadConfig(); len(cfg.Envs) != 0 || cfg.DefaultEnv != "" {
		t.Fatal(cfg)
	}
}
