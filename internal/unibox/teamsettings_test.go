package unibox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tobola/unibox/internal/version"
)

func TestTheTeamsSettingsAreThoseItsEnvsAgreeOn(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	write(t, filepath.Join(root, "unibox.yaml"), "settings:\n  ssh_agent: false\n  clipboard_image_paste: true\n")
	dev := filepath.Join(root, "sandboxes", "dev.sbxenv.yaml")
	review := filepath.Join(root, "sandboxes", "review.sbxenv.yaml")
	write(t, dev, "name: dev\n")
	write(t, review, "name: review\n")
	// review's own file says otherwise of images, over the repository's.
	write(t, filepath.Join(root, "sandboxes", "review.unibox.yaml"), "settings:\n  clipboard_image_paste: false\n  ssh_sandbox: true\n")
	settings, differ, err := TeamSettings([][]string{{dev}, {review}})
	if err != nil {
		t.Fatal(err)
	}
	if len(settings) != 2 || settings["ssh_agent"] || !settings["ssh_sandbox"] || !slices.Equal(differ, []string{"clipboard_image_paste"}) {
		t.Fatalf("%v %v", settings, differ)
	}
	write(t, filepath.Join(root, "sandboxes", "review.unibox.yaml"), "settings:\n  stop_idle: false\n")
	if _, _, err := TeamSettings([][]string{{review}}); err == nil || !strings.Contains(err.Error(), "stop_idle is none of") {
		t.Fatalf("%v", err)
	}
}

func TestASandboxStartsWithTheTeamsSettingsThePersonLeftUnsaid(t *testing.T) {
	t.Parallel()
	m, f := managerFixture(t)
	ctx := context.Background()
	no := false
	m.Config.ClipboardTextCopy = &no
	if err := m.Paths.SaveConfig(m.Config); err != nil {
		t.Fatal(err)
	}
	f.sshAgent = true
	write(t, filepath.Join(filepath.Dir(m.Config.Envs["dev"].Files[0]), "dev.unibox.yaml"), "settings:\n  ssh_agent: false\n  clipboard_image_paste: true\n  clipboard_text_copy: true\n")
	if _, err := m.Create(ctx, Request{Sandbox: "dev", Env: "dev"}, false); err != nil {
		t.Fatal(err)
	}
	cfg, _ := m.Paths.LoadConfig()
	// What the person said stays theirs; the rest is the team's, and sbx
	// set to match.
	if cfg.SSHAgentOn == nil || *cfg.SSHAgentOn || cfg.ClipboardImagePaste == nil || !*cfg.ClipboardImagePaste || *cfg.ClipboardTextCopy {
		t.Fatalf("%+v", cfg)
	}
	if f.sshAgent || f.settings["clipboard.imagePaste"] != "true" || f.settings["clipboard.textCopy"] == "true" {
		t.Fatalf("%v %v", f.sshAgent, f.settings)
	}
}

func TestATeamsFileNeedingANewerUniboxIsRefused(t *testing.T) {
	// The release is the build's, which this test sets for the package.
	was := version.Version
	version.Version = "0.9"
	t.Cleanup(func() { version.Version = was })
	root := t.TempDir()
	write(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	file := filepath.Join(root, "dev.sbxenv.yaml")
	write(t, file, "name: dev\n")
	// Read before the keys this release does not know.
	write(t, filepath.Join(root, "unibox.yaml"), "min_unibox: \"0.10\"\nsomething_new: true\n")
	_, err := ReadTeamPorts([]string{file})
	var failure *Failure
	if !errors.As(err, &failure) || failure.Code != "unibox_outdated" || !strings.Contains(err.Error(), "needs unibox 0.10 or newer, and this is 0.9") {
		t.Fatalf("%v", err)
	}
	// Without it, a key it does not know says it may be newer.
	write(t, filepath.Join(root, "unibox.yaml"), "something_new: true\n")
	if _, err := ReadTeamPorts([]string{file}); err == nil || !strings.Contains(err.Error(), "newer unibox than this 0.9") {
		t.Fatalf("%v", err)
	}
	write(t, filepath.Join(root, "unibox.yaml"), "min_unibox: \"0.9\"\n")
	if _, err := ReadTeamPorts([]string{file}); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "unibox.yaml"), "min_unibox: soon\n")
	if _, err := ReadTeamPorts([]string{file}); err == nil || !strings.Contains(err.Error(), "min_unibox is a release") {
		t.Fatalf("%v", err)
	}
	_ = os.Remove(filepath.Join(root, "unibox.yaml"))
}

func TestUniboxReleasesCompareByTheirNumbers(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		have, need string
		want       bool
	}{
		{"0.10", "0.9", true},
		{"0.9", "0.10", false},
		{"0.10", "0.10", true},
		{"0.10-pre.3", "0.10", true},
		{"1.0", "0.10.2", true},
		{"0.10", "0.10.1", false},
		{"dev (4f0c5bd1a2b3)", "9.9", true},
	} {
		if got := UniboxAtLeast(c.have, c.need); got != c.want {
			t.Errorf("%s at least %s: %v", c.have, c.need, got)
		}
	}
}
