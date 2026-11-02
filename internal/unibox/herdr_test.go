package unibox

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestThePluginIsLinkedAsHerdrTakesItThroughItsServer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the herdr stand-in is a shell script")
	}
	m, _ := managerFixture(t)
	seen := filepath.Join(t.TempDir(), "args")
	bin := filepath.Join(t.TempDir(), "herdr")
	// Like herdr 0.9.1 asked through its server's socket, the stand-in
	// refuses the argument after --enabled.
	mustWriteExecutable(t, bin, []byte(`#!/bin/sh
[ "$1" = --version ] && { echo "herdr 0.9.1"; exit 0; }
printf '%s\n' "$*" > `+Quote(seen)+`
[ "$3" = --enabled ] && { echo "unknown option: $4" >&2; exit 2; }
exit 0
`))
	t.Setenv("HERDR_BIN_PATH", bin)
	t.Setenv("HERDR_SOCKET_PATH", filepath.Join(t.TempDir(), "herdr.sock"))
	if err := m.InstallPlugin(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(seen)
	if want := "plugin link " + filepath.Dir(m.pluginPath()) + " --enabled"; strings.TrimSpace(string(got)) != want {
		t.Fatalf("%q, want %q", got, want)
	}
}
