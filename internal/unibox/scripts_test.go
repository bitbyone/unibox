package unibox

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const scriptsProfile = "version: 5\nstart: start.sh\nagents: [claude]\nscripts:\n  greet: greet.sh\n  plain: _data/plain.sh\n  fail: fail.sh\n"

func writeScripts(t *testing.T, dir string) {
	t.Helper()
	write(t, filepath.Join(dir, "sandbox.yaml"), scriptsProfile)
	write(t, filepath.Join(dir, "greet.sh"), "#!/bin/sh\nprintf '%s %s %s\\n' \"$UNIBOX_SCRIPT\" \"$UNIBOX_SANDBOX\" \"$UNIBOX_TASK\" >> \"$HOME/ran\"\n")
	// No #! line: sh runs it.
	write(t, filepath.Join(dir, "_data", "plain.sh"), "echo \"$UNIBOX_SCRIPT\" >> \"$HOME/ran\"\n")
	write(t, filepath.Join(dir, "fail.sh"), "#!/bin/sh\necho failing\nexit 3\n")
}

func TestScriptsRunInTheSandboxWhenAskedFor(t *testing.T) {
	t.Parallel()
	m, f := managerFixture(t)
	ctx := context.Background()
	dir := filepath.Join(m.Config.Environments, "sandboxes", "node")
	writeScripts(t, dir)
	if err := m.Build(ctx, "node", "fake", false); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(ctx, Request{NewSandbox: "dev", Profile: "node", Runtime: "fake"}); err != nil {
		t.Fatal(err)
	}
	ran := filepath.Join(f.root, "unibox-dev", "home", "ran")
	var out bytes.Buffer
	if err := m.RunScripts(ctx, "dev", []string{"greet", "plain"}, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if data, _ := os.ReadFile(ran); string(data) != "greet dev \nplain\n" {
		t.Fatalf("%q", data)
	}
	if !strings.Contains(out.String(), "unibox: running script greet") {
		t.Fatalf("%s", out.String())
	}
	// The first to fail stops the rest, and says which.
	out.Reset()
	if err := m.RunScripts(ctx, "dev", []string{"fail", "greet"}, &out); err == nil || !strings.Contains(out.String(), "failing") || !strings.Contains(out.String(), "unibox: script fail failed") {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if data, _ := os.ReadFile(ran); strings.Count(string(data), "greet") != 1 {
		t.Fatalf("a script ran after one failed: %q", data)
	}
	if err := m.RunScripts(ctx, "dev", []string{"nope"}, &out); err == nil || !strings.Contains(err.Error(), "has no script nope; it has fail, greet, plain") {
		t.Fatalf("%v", err)
	}
	// Before a session: a failing script keeps the agent from starting.
	remote, _ := remoteFixture(t)
	url := "https://forge.invalid/team/payments.git"
	f.remotes[url] = "file://" + remote
	err := m.Run(ctx, Request{Sandbox: "dev", Task: "t1", Agent: "claude", Repositories: []RepoInput{{Source: url, Branch: "feat/refunds"}}, Scripts: []string{"fail"}}, nil, &out, &out, nil)
	if err == nil || !strings.Contains(err.Error(), "a script failed, so claude was not started") {
		t.Fatalf("%v", err)
	}
	if err = m.Run(ctx, Request{Task: "t1", Scripts: []string{"nope"}}, nil, &out, &out, nil); err == nil || !strings.Contains(err.Error(), "has no script nope") {
		t.Fatalf("%v", err)
	}
}

func TestScriptsAreNeitherTheImageNorWhatMakesASandbox(t *testing.T) {
	t.Parallel()
	root := profileFixture(t)
	dir := filepath.Join(root, "sandboxes", "node")
	writeScripts(t, dir)
	p, err := LoadProfile(root, "node")
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "greet.sh"), "#!/bin/sh\necho changed\n")
	changed, err := LoadProfile(root, "node")
	if err != nil {
		t.Fatal(err)
	}
	if p.Revision != changed.Revision {
		t.Fatal("a script's change outdated the image")
	}
	without := p
	without.Scripts = nil
	if string(settingsSnapshot(p, &fakeRuntime{})) != string(settingsSnapshot(without, &fakeRuntime{})) {
		t.Fatal("scripts would recreate a sandbox")
	}
	// Files a profile does not name do not stay in its root.
	_ = os.Remove(filepath.Join(dir, "fail.sh"))
	write(t, filepath.Join(dir, "_data", "greet.sh"), "#!/bin/sh\n")
	_ = os.Remove(filepath.Join(dir, "greet.sh"))
	for body, want := range map[string]string{
		"scripts: {Greet: _data/greet.sh}": "script name \"Greet\"",
		"scripts: {greet: absent.sh}":      "script greet: missing absent.sh",
		"scripts: {greet: ../x.sh}":        "must stay inside their directory",
		"scripts: {greet: ''}":             "names no file",
	} {
		write(t, filepath.Join(dir, "sandbox.yaml"), "version: 5\nstart: start.sh\nagents: [claude]\n"+body+"\n")
		if _, err := LoadProfile(root, "node"); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", body, err)
		}
	}
}
