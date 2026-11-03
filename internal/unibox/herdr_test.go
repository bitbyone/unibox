package unibox

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tobola/unibox/internal/unibox/api"
)

func TestThePluginIsLinkedAsHerdrTakesItThroughItsServer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the herdr stand-in is a shell script")
	}
	m, _ := managerFixture(t)
	seen := filepath.Join(t.TempDir(), "args")
	bin := filepath.Join(t.TempDir(), "herdr")
	// Like herdr 0.9.1 asked through its server's socket, the stand-in
	// refuses the argument after --enabled or --plugin.
	mustWriteExecutable(t, bin, []byte(`#!/bin/sh
[ "$1" = --version ] && { echo "herdr 0.9.1"; exit 0; }
printf '%s\n' "$*" >> `+Quote(seen)+`
[ "$3" = --enabled ] || [ "$4" = --plugin ] && { echo "unknown option: $4" >&2; exit 2; }
exit 0
`))
	t.Setenv("HERDR_BIN_PATH", bin)
	t.Setenv("HERDR_SOCKET_PATH", filepath.Join(t.TempDir(), "herdr.sock"))
	if err := m.InstallPlugin(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Linked, a running server is asked to start the observer at once, as
	// herdr starts a plugin only with its server.
	got, _ := os.ReadFile(seen)
	if want := "plugin link " + filepath.Dir(m.pluginPath()) + " --enabled\nplugin action invoke start-observer --plugin unibox"; strings.TrimSpace(string(got)) != want {
		t.Fatalf("%q, want %q", got, want)
	}
	if manifest, _ := os.ReadFile(m.pluginPath()); !strings.Contains(string(manifest), `id = "start-observer"`) {
		t.Fatalf("%s", manifest)
	}
}

func TestTheObserverLogsAPanesFailureOnceUntilItChanges(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the herdr stand-in is a shell script")
	}
	m, _ := managerFixture(t)
	ctx := context.Background()
	if _, err := m.Create(ctx, Request{Sandbox: "dev", Env: "dev"}, false); err != nil {
		t.Fatal(err)
	}
	socket, state := filepath.Join(t.TempDir(), "herdr.sock"), t.TempDir()
	run := api.Run{ID: "1-1", Sandbox: "dev", Agent: "copilot", Pane: "wY:p1", Socket: socket}
	lock, err := m.runLock(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err = m.Paths.State(func(s *State) error {
		box := s.Sandboxes["dev"]
		box.Runs = append(box.Runs, run)
		s.Sandboxes["dev"] = box
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "herdr")
	mustWriteExecutable(t, bin, []byte(`#!/bin/sh
case "$1 $2" in
"agent explain") echo '{"state":"working"}' ;;
"pane report-agent") echo "no such pane" >&2; exit 1 ;;
esac
exit 0
`))
	t.Setenv("HERDR_BIN_PATH", bin)
	var log strings.Builder
	m.Log = &log
	previous, failed := map[string]observed{}, map[string]string{}
	for range 3 {
		if err = m.observeOnce(ctx, socket, state, previous, failed); err != nil {
			t.Fatal(err)
		}
	}
	if got := log.String(); strings.Count(got, "\n") != 1 || !strings.Contains(got, "pane wY:p1: reporting the agent") || !strings.Contains(got, "no such pane") {
		t.Fatalf("%q", got)
	}
}
