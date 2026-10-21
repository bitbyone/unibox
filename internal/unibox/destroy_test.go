package unibox

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDestroyRemovesEverySandboxOnlyWhenNoWorkIsLost(t *testing.T) {
	t.Parallel()
	m, f := managerFixture(t)
	ctx := context.Background()
	for _, name := range []string{"dev", "ci"} {
		if _, err := m.Create(ctx, Request{NewSandbox: name, Profile: "node"}); err != nil {
			t.Fatal(err)
		}
	}
	// A running session in one stops it all, before anything is removed.
	usage, err := m.usage("ci", true)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Destroy(ctx, DestroyOptions{}); err == nil || !strings.Contains(err.Error(), "nothing was removed") {
		t.Fatalf("%v", err)
	}
	if s, _ := m.Paths.ReadState(); len(s.Sandboxes) != 2 || f.removes != 0 {
		t.Fatalf("%d sandboxes, %d removed", len(s.Sandboxes), f.removes)
	}
	// Forced, it goes regardless.
	if err = m.Destroy(ctx, DestroyOptions{Force: true}); err != nil {
		t.Fatal(err)
	}
	usage.Close()
	if s, _ := m.Paths.ReadState(); len(s.Sandboxes) != 0 || f.removes != 2 {
		t.Fatalf("%d sandboxes left, %d removed", len(s.Sandboxes), f.removes)
	}
	// All takes unibox's own files too, and leaves the repository.
	if err = m.Destroy(ctx, DestroyOptions{All: true}); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{m.Paths.Config, m.Paths.Cache} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("%s stayed: %v", dir, err)
		}
	}
	if _, err := os.Stat(filepath.Join(m.Config.Environments, "sandboxes", "node", "sandbox.yaml")); err != nil {
		t.Fatal("the environments repository went too")
	}
}

func TestDestroyAllKeepsARepositoryInsideWhatItRemoves(t *testing.T) {
	t.Parallel()
	m, _ := managerFixture(t)
	m.Config.Environments = filepath.Join(m.Paths.Config, "environments")
	if err := m.Destroy(context.Background(), DestroyOptions{All: true}); err == nil || !strings.Contains(err.Error(), "move it elsewhere first") {
		t.Fatalf("%v", err)
	}
	if _, err := os.Stat(m.Paths.Config); err != nil {
		t.Fatal(err)
	}
}

func TestDestroyTakesOutOnlyTheApprovalsUniboxAdded(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "credentials.yaml")
	if err := writeBindings(file, []string{"claude", "copilot"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(file)
	// One of the user's own beside unibox's.
	write(t, file, strings.Replace(string(data), "- api.anthropic.com", "- api.anthropic.com\n        - mine.example.com", 1))
	if err := removeBindings(file, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(file)
	if !strings.Contains(string(after), "mine.example.com") || strings.Contains(string(after), "api.anthropic.com") || strings.Contains(string(after), "github") {
		t.Fatalf("%s", after)
	}
}

func TestSBXCleanRemovesOnlyTheSecretsUniboxMade(t *testing.T) {
	bin, calls := t.TempDir(), filepath.Join(t.TempDir(), "calls")
	listing := `{"secrets":[{"scope":"global","name":"openai","type":"service","secret":"oauth"},{"scope":"global","name":"anthropic","type":"command","secret":"command:security find-generic-password -s unibox-claude -w"}],"custom_secrets":[{"scope":"global","env":"CLAUDE_CODE_OAUTH_TOKEN","placeholder":"sk-ant-oat01-unibox0claude0proxy0managed"},{"scope":"unibox-dev","env":"UNIBOX_SVC_GITHUB","placeholder":"unibox-svc-github-1234"},{"scope":"global","env":"MINE","placeholder":"my-own"}]}`
	mustWriteExecutable(t, filepath.Join(bin, "sbx"), []byte("#!/bin/sh\necho \"$*\" >> "+Quote(calls)+"\n[ \"$1 $2\" = \"secret ls\" ] && echo '"+listing+"'\nexit 0\n"))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	r := &SBX{}
	var log bytes.Buffer
	if err := r.Clean(context.Background(), &log); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(calls)
	got := string(data)
	for _, want := range []string{"secret rm --placeholder sk-ant-oat01-unibox0claude0proxy0managed -f\n", "secret rm --placeholder unibox-svc-github-1234 -f --sandbox unibox-dev\n", "secret rm anthropic -f\n"} {
		if !strings.Contains(got, want) {
			t.Fatalf("no %q in\n%s", want, got)
		}
	}
	// A login made with sbx, and a secret of the user's own, stay.
	if strings.Contains(got, "openai") || strings.Contains(got, "my-own") {
		t.Fatalf("%s", got)
	}
}
