package unibox

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// An env's own stop_idle wins over every env's, which wins over the
// default of stopping.
func TestStopIdleIsAnEnvsOwnOverEveryEnvs(t *testing.T) {
	t.Parallel()
	off, on := false, true
	c := Config{Envs: map[string]EnvConfig{"dev": {}, "web": {StopIdle: &on}}}
	if got, from := c.StopIdleFor("dev"); !got || from != "default" {
		t.Fatalf("%v %s", got, from)
	}
	c.StopIdleSandbox = &off
	if got, from := c.StopIdleFor("dev"); got || from != "global" {
		t.Fatalf("%v %s", got, from)
	}
	if got, from := c.StopIdleFor("web"); !got || from != "env" {
		t.Fatalf("%v %s", got, from)
	}
}

// editor_ssh, ssh_sandbox's name before, is read as it and written
// under the new name.
func TestEditorSSHIsReadAsSSHSandbox(t *testing.T) {
	t.Parallel()
	p := Paths{Config: t.TempDir(), Cache: t.TempDir()}
	if err := os.WriteFile(filepath.Join(p.Config, "config.yaml"), []byte("editor_ssh: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := p.LoadConfig()
	if on, said := c.EditorSSH(); err != nil || !on || !said {
		t.Fatalf("%v %v %v", on, said, err)
	}
	if err := p.SaveConfig(c); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(p.Config, "config.yaml"))
	if !strings.Contains(string(b), "ssh_sandbox: true") || strings.Contains(string(b), "editor_ssh") {
		t.Fatalf("%s", b)
	}
}

// default_env, what the context says since 0.10, is read into the context
// and not written again; a context already set wins over it.
func TestDefaultEnvIsReadAsTheContext(t *testing.T) {
	t.Parallel()
	p := Paths{Config: t.TempDir(), Cache: t.TempDir()}
	file := filepath.Join(p.Config, "config.yaml")
	if err := os.WriteFile(file, []byte("default_env: dev\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := p.LoadConfig()
	if err != nil || c.Context != (Context{Env: "dev"}) {
		t.Fatalf("%+v %v", c.Context, err)
	}
	if err := p.SaveConfig(c); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(file)
	if !strings.Contains(string(b), "env: dev") || strings.Contains(string(b), "default_env") {
		t.Fatalf("%s", b)
	}
	if err := os.WriteFile(file, []byte("default_env: dev\ncontext: {sandbox: mine}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if c, err = p.LoadConfig(); err != nil || c.Context != (Context{Sandbox: "mine"}) {
		t.Fatalf("%+v %v", c.Context, err)
	}
}

// sbx that lost its SSH endpoint, as after a reset, gets it back at the
// next start while no sandbox runs; with one running it is only told, as
// sbx's daemon restarts for it.
func TestSandboxSSHIsTurnedBackOnWhileNoSandboxRuns(t *testing.T) {
	t.Parallel()
	m, f := managerFixture(t)
	ctx := context.Background()
	on := true
	m.Config.EditorSSHOn = &on
	f.boxes = []Box{{Name: "other", State: "running"}}
	var log bytes.Buffer
	m.Log = &log
	if err := m.syncSandboxSSH(ctx); err != nil || f.servesSSH || !strings.Contains(log.String(), "unibox config set ssh_sandbox true") {
		t.Fatalf("%v %v %s", err, f.servesSSH, log.String())
	}
	f.boxes[0].State = "stopped"
	if err := m.syncSandboxSSH(ctx); err != nil || !f.servesSSH {
		t.Fatalf("%v %v", err, f.servesSSH)
	}
	calls := len(f.calls)
	if err := m.syncSandboxSSH(ctx); err != nil || len(f.calls) != calls || slices.Contains(f.calls[calls:], "ssh true") {
		t.Fatalf("turned on again: %v", f.calls[calls:])
	}
}
