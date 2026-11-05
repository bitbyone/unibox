package cli

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/tobola/unibox/internal/unibox"
)

// A list in tabs shows them in its title's place and turns to the next
// or the one before by its keys, round, as the action "tab:N".
func TestAListsTabsTurnByTheirKeys(t *testing.T) {
	t.Parallel()
	m := listModel{tabs: []string{"Global", "dev", "web"}, header: []string{"SETTING"}, rows: [][]string{{"stop_idle"}}, ids: []string{"set:stop_idle"}}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "[Global]  dev   web") || !strings.Contains(v, "←/→ h/l tab") {
		t.Fatalf("%q", v)
	}
	var tm tea.Model = m
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRight})
	if c := tm.(listModel).chosen; c == nil || c.name != "tab:1" {
		t.Fatalf("%+v", c)
	}
	if c := listKeys(m, "h").chosen; c == nil || c.name != "tab:2" {
		t.Fatalf("%+v", c)
	}
	if c := listKeys(listModel{header: []string{"X"}, rows: [][]string{{"x"}}, ids: []string{"x"}}, "l").chosen; c != nil {
		t.Fatalf("a list without tabs turned: %+v", c)
	}
}

// stop_idle is the one setting an env has of its own in config.yaml; -e
// says it, and refuses the others.
func TestConfigKeepsAnEnvsOwnStopIdle(t *testing.T) {
	t.Parallel()
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	if _, _, err := run(t, paths, "", "env", "add", "team", teamEnv(t)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, paths, "", "config", "set", "stop_idle", "false", "-e", "team"); err != nil {
		t.Fatal(err)
	}
	if out, _, err := run(t, paths, "", "config", "get", "stop_idle", "-e", "team"); err != nil || strings.TrimSpace(out) != "false" {
		t.Fatalf("%q %v", out, err)
	}
	if out, _, _ := run(t, paths, "", "config", "get", "stop_idle"); strings.TrimSpace(out) != "true" {
		t.Fatalf("every env's changed: %q", out)
	}
	if _, _, err := run(t, paths, "", "config", "set", "update_check", "false", "-e", "team"); err == nil || !strings.Contains(err.Error(), "only stop_idle") {
		t.Fatalf("%v", err)
	}
	if _, _, err := run(t, paths, "", "config", "unset", "stop_idle", "-e", "team"); err != nil {
		t.Fatal(err)
	}
	if cfg, _ := paths.LoadConfig(); cfg.Envs["team"].StopIdle != nil {
		t.Fatalf("%v", *cfg.Envs["team"].StopIdle)
	}
}
