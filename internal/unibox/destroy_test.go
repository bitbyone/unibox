package unibox

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestDestroyRemovesEverySandboxOnlyWhenNoWorkIsLost(t *testing.T) {
	t.Parallel()
	m, f := managerFixture(t)
	ctx := context.Background()
	for _, name := range []string{"dev", "ci"} {
		if _, err := m.Create(ctx, Request{Sandbox: name, Env: "dev"}, false); err != nil {
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
	// All takes unibox's own files too; the env's files are the team's.
	if err = m.Destroy(ctx, DestroyOptions{All: true}); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{m.Paths.Config, m.Paths.Cache} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("%s stayed: %v", dir, err)
		}
	}
	if _, err := os.Stat(m.Config.Envs["dev"].Files[0]); err != nil {
		t.Fatal("the env's file went too")
	}
}
