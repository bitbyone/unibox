package unibox

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tobola/unibox/internal/testenv"
)

func mustWriteExecutable(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := testenv.WriteExecutable(path, content); err != nil {
		t.Fatal(err)
	}
}

func TestSBXCreatesFromTheEnvAndPassesNoHostSecret(t *testing.T) {
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "argv")
	env := filepath.Join(t.TempDir(), "env")
	mustWriteExecutable(t, filepath.Join(bin, "sbx"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" >> "+Quote(log)+"\nenv > "+Quote(env)+"\n"))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GITHUB_TOKEN", "fake-secret-for-test")
	t.Setenv("SSH_AUTH_SOCK", "fake-ssh-socket")
	r := &SBX{}
	ctx := context.Background()
	if err := r.EnvCreate(ctx, "dev", "/tmp/args", []string{"a.sbxenv.yaml", "b.sbxenv.yaml"}, nil, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := r.EnvRemove(ctx, "dev", "", []string{"a.sbxenv.yaml"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := r.Mount(ctx, "dev", "/host/x", "/work/t/x"); err != nil {
		t.Fatal(err)
	}
	argv, _ := os.ReadFile(log)
	environment, _ := os.ReadFile(env)
	for _, bad := range []string{"fake-secret-for-test", "fake-ssh-socket", "GITHUB_TOKEN", "SSH_AUTH_SOCK"} {
		if strings.Contains(string(argv)+string(environment), bad) {
			t.Errorf("leaked %s", bad)
		}
	}
	want := "env\ncreate\n--name\ndev\n--env-args-file\n/tmp/args\na.sbxenv.yaml\nb.sbxenv.yaml\n" +
		"env\nrm\n--name\ndev\na.sbxenv.yaml\n--force\n" +
		"mount\ndev\n/host/x:/work/t/x\n"
	if string(argv) != want {
		t.Fatalf("%s", argv)
	}
}

func TestSBXChecksSSHForwardingAndReadsWhatItLists(t *testing.T) {
	bin := t.TempDir()
	settings := filepath.Join(t.TempDir(), "settings")
	write(t, settings, `{"value":true}`)
	mustWriteExecutable(t, filepath.Join(bin, "sbx"), []byte(`#!/bin/sh
case "$1 $2" in
settings*) cat `+Quote(settings)+` ;;
ls*) echo '[{"name":"dev","id":"vm-id","status":"stopped","agent":"copilot"}]' ;;
"kit inspect") echo "Cached kit ./kits/agent"; echo '{"kind":"sandbox","name":"agent","sandbox":{"entrypoint":["copilot"],"command":{"default":["--yolo"]}},"credentials":[{"service":"copilot","optional":false}]}' ;;
"secret ls") echo '[{"scope":"global","type":"service","name":"github"}]' ;;
esac
`))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	r := &SBX{}
	ctx := context.Background()
	if err := r.Available(ctx); err == nil {
		t.Fatal("forwarding was accepted")
	}
	r.AllowSSHAgent(true)
	if err := r.Available(ctx); err != nil {
		t.Fatal(err)
	}
	r.AllowSSHAgent(false)
	write(t, settings, `{"value":false}`)
	if err := r.Available(ctx); err != nil {
		t.Fatal(err)
	}
	boxes, err := r.List(ctx)
	if err != nil || len(boxes) != 1 || boxes[0].ID != "vm-id" || boxes[0].State != "stopped" {
		t.Fatalf("%+v %v", boxes, err)
	}
	kit, err := r.KitInspect(ctx, "./kits/agent", nil)
	if err != nil || kit.Kind != "sandbox" || !slices.Equal(kit.Sandbox.Entrypoint, []string{"copilot"}) || kit.Sandbox.Command.Default[0] != "--yolo" || kit.Credentials[0].Service != "copilot" {
		t.Fatalf("%+v %v", kit, err)
	}
	secrets, err := r.Secrets(ctx)
	if err != nil || len(secrets) != 1 || secrets[0].Name != "github" || secrets[0].Scope != "global" {
		t.Fatalf("%+v %v", secrets, err)
	}
}

func TestSignedInStartsTheDaemonSbxNeedsToSay(t *testing.T) {
	bin, dir := t.TempDir(), t.TempDir()
	started := filepath.Join(dir, "started")
	// sbx diagnose exits 1 whenever a check fails, the daemon's or the
	// network policy's on a fresh install, and prints its report all the same.
	mustWriteExecutable(t, filepath.Join(bin, "sbx"), []byte(`#!/bin/sh
case "$1 $2" in
"daemon start") touch `+Quote(started)+` ;;
diagnose*)
	if [ -e `+Quote(started)+` ]; then
		echo '{"checks":[{"name":"Daemon","status":"pass"},{"name":"Authentication","status":"pass"},{"name":"Global network policy","status":"fail"}]}'
	else
		echo '{"checks":[{"name":"Daemon","status":"fail"},{"name":"Authentication","status":"skip"}]}'
	fi
	exit 1 ;;
esac
`))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	signedIn, err := (&SBX{}).SignedIn(context.Background())
	if err != nil || !signedIn {
		t.Fatalf("%v %v", signedIn, err)
	}
	if _, err := os.Stat(started); err != nil {
		t.Fatal("the daemon was not started")
	}
}

func TestASessionUsingTheSSHAgentTrustsAServerItMeetsFirst(t *testing.T) {
	t.Parallel()
	m, _ := managerFixture(t)
	if env := m.sshEnv(); len(env) != 0 {
		t.Fatalf("%v", env)
	}
	on := true
	m.Config.SSHAgentOn = &on
	if env := m.sshEnv(); !slices.Contains(env, "GIT_SSH_COMMAND=ssh -o StrictHostKeyChecking=accept-new") {
		t.Fatalf("%v", env)
	}
}

func TestSbxForwardingReadsPastWhatSbxSaysFirst(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for answer, want := range map[string]string{
		"A new version of sbx is available\n{\"key\": \"ssh.agentForwardingEnabled\", \"value\": false}\n": "",
		"{\"value\": true}\n":  "forwards",
		"daemon is starting\n": "which answered \"daemon is starting\"",
	} {
		mustWriteExecutable(t, filepath.Join(bin, "sbx"), []byte("#!/bin/sh\nprintf '%s' "+Quote(answer)+"\n"))
		forwarding, err := sbxForwarding(context.Background())
		switch want {
		case "":
			if err != nil || forwarding {
				t.Errorf("%q: %v %v", answer, forwarding, err)
			}
		case "forwards":
			if err != nil || !forwarding {
				t.Errorf("%q: %v %v", answer, forwarding, err)
			}
		default:
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("%q: %v", answer, err)
			}
		}
	}
}

func TestSbxListsReadInTheFormOfEitherVersion(t *testing.T) {
	t.Parallel()
	type box struct{ Name, Status string }
	for _, in := range []string{
		`{"sandboxes":[{"name":"unibox-dev","status":"running"}]}`,
		`[{"name":"unibox-dev","status":"running","last_used_at":"2026-09-08T14:49:59Z"}]`,
		"note from sbx\n[{\"name\":\"unibox-dev\",\"status\":\"running\"}]",
	} {
		got, err := sbxList[box]([]byte(in), "sandboxes")
		if err != nil || len(got) != 1 || got[0].Name != "unibox-dev" {
			t.Fatalf("%s: %v %+v", in, err, got)
		}
	}
	if got, err := sbxList[box]([]byte(`[]`), "sandboxes"); err != nil || len(got) != 0 {
		t.Fatalf("%v %+v", err, got)
	}
}

func TestVersionsAreComparedByNumberNotText(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		version, min string
		ok           bool
	}{
		{"0.49.2", "0.49.2", true},
		{"0.49.10", "0.49.2", true},
		{"v1.0.0", "0.49.2", true},
		{"0.48.9", "0.49.2", false},
		{"0.49.1", "0.49.2", false},
		{"nightly", "0.49.2", false},
	} {
		if got := VersionAtLeast(c.version, c.min); got != c.ok {
			t.Errorf("%s >= %s: %v", c.version, c.min, got)
		}
	}
}
