package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/tobola/unibox/internal/version"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/tobola/unibox/internal/testenv"
	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
)

func TestMain(m *testing.M) { testenv.Main(m) }

func TestBranchOrderSupportsDefaultsAndIndividualOverrides(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		args     []string
		global   string
		branches []string
	}{
		{"shared", []string{"--branch", "feat/shared", "--repo", "./api", "--repo", "./web"}, "feat/shared", []string{"", ""}},
		{"override", []string{"--branch=feat/shared", "--repo=./api", "--repo", "./web", "--branch", "feat/web", "--repo", "./worker"}, "feat/shared", []string{"", "feat/web", ""}},
		{"per-repo", []string{"--repo", "https://forge.invalid/api.git", "--branch", "feat/api", "--repo", "https://forge.invalid/web.git", "--branch=feat/web"}, "", []string{"feat/api", "feat/web"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var req unibox.Request
			c := &cobra.Command{Use: "test", Run: func(*cobra.Command, []string) {}}
			taskFlags(c, &req)
			c.SetArgs(tc.args)
			if err := c.Execute(); err != nil {
				t.Fatal(err)
			}
			if req.Branch != tc.global {
				t.Fatal(req.Branch)
			}
			got := []string{}
			for _, r := range req.Repositories {
				got = append(got, r.Branch)
			}
			if !reflect.DeepEqual(got, tc.branches) {
				t.Fatalf("%v != %v", got, tc.branches)
			}
		})
	}
}

func cliFixture(t *testing.T) (unibox.Paths, string) {
	t.Helper()
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sandboxes", "node"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sandboxes", "node", "sandbox.yaml"), []byte("version: 6\nimage: example/image\nagents: [claude]\nenv: [BUILD_MODE]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := paths.SaveConfig(unibox.Config{Environments: root, Runtimes: map[string]unibox.RuntimeConfig{}}); err != nil {
		t.Fatal(err)
	}
	return paths, root
}

func TestJSONListsAreSingleVersionedObjects(t *testing.T) {
	t.Parallel()
	paths, _ := cliFixture(t)
	for _, args := range [][]string{{"profile", "ls", "--json"}, {"config", "get", "--json"}, {"sandbox", "ls", "--json"}, {"instructions", "-p", "node", "--json"}, {"context", "--json"}} {
		var out, log bytes.Buffer
		c := New(paths, strings.NewReader(""), &out, &log)
		c.SetArgs(args)
		if err := c.Execute(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		var value map[string]any
		d := json.NewDecoder(&out)
		if err := d.Decode(&value); err != nil {
			t.Fatal(err)
		}
		if value["api_version"] != float64(1) {
			t.Fatal(value)
		}
		var extra any
		if err := d.Decode(&extra); err != io.EOF {
			t.Fatalf("extra stdout: %v", err)
		}
	}
}

func TestAChangedProfileIsNotAskedAboutAndJSONDoesNotPrompt(t *testing.T) {
	t.Parallel()
	paths, _ := cliFixture(t)
	var out, log bytes.Buffer
	c := New(paths, strings.NewReader("yes\n"), &out, &log)
	c.SetArgs([]string{"sandbox", "create", "dev", "-p", "node", "--json"})
	// The profile asks for host access; what stops this is the runtime
	// missing here, not a question about the profile.
	err := c.Execute()
	if err == nil || strings.Contains(err.Error(), "approve") || strings.Contains(err.Error(), "unapproved") {
		t.Fatalf("%v", err)
	}
	if strings.Contains(log.String(), "[y/N]") {
		t.Fatal("JSON prompted")
	}
}

func TestStrictConfig(t *testing.T) {
	t.Parallel()
	paths, _ := cliFixture(t)
	for _, args := range [][]string{{"config", "set", "runtimes.sbx.stop_idle", "false"}, {"config", "set", "default_runtime", "sbx"}} {
		c := New(paths, strings.NewReader(""), io.Discard, io.Discard)
		c.SetArgs(args)
		if err := c.Execute(); err != nil {
			t.Fatal(err)
		}
	}
	cfg, _ := paths.LoadConfig()
	if cfg.StopIdle("sbx") || cfg.DefaultRuntime != "sbx" {
		t.Fatal(cfg)
	}
	c := New(paths, strings.NewReader(""), io.Discard, io.Discard)
	c.SetArgs([]string{"config", "set", "runtimes.sbx.stop_idel", "false"})
	if err := c.Execute(); err == nil {
		t.Fatal("unknown setting accepted")
	}
}

func TestInitLaysTheRepositoryOutAndKeepsWhatExists(t *testing.T) {
	t.Parallel()
	paths := unibox.Paths{Config: t.TempDir(), Cache: t.TempDir()}
	root := t.TempDir()
	if err := paths.SaveConfig(unibox.Config{Environments: root}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	c := New(paths, strings.NewReader(""), &out, io.Discard)
	c.SetArgs([]string{"init", "my2n"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{".shared/@install/base.sh", ".shared/@install/agents/copilot.sh", ".shared/@instructions/unibox.md", "_shared/@install/custom.sh", "_shared/@instructions/custom.md", "sandboxes/my2n/sandbox.yaml", "sandboxes/my2n/sandbox.dockerfile", "sandboxes/my2n/instructions.md", "sandboxes/my2n/.sbx/build.args"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
	// Each person's values get a file of their own in their home.
	if _, err := os.Stat(paths.VarsFile()); err != nil || !strings.Contains(out.String(), "unibox var set") {
		t.Fatalf("no vars file: %v\n%s", err, out.String())
	}
	p, err := unibox.LoadProfile(root, "my2n")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.Agents, []string{"copilot"}) {
		t.Fatal(p.Agents)
	}
	if text, err := p.InstructionText(nil); err != nil || !strings.Contains(text, "Unibox sandbox") {
		t.Fatalf("%v %q", err, text)
	}
	if info, _ := os.Stat(filepath.Join(root, ".shared", "@install", "base.sh")); info.Mode().Perm()&0o100 == 0 {
		t.Fatal("an installation script is not executable")
	}
	if dockerfile, _ := os.ReadFile(filepath.Join(root, "sandboxes", "my2n", "sandbox.dockerfile")); !strings.Contains(string(dockerfile), "COPY --from=unibox / /\nRUN @install base tools/nvm agents/copilot\n") {
		t.Fatalf("%s", dockerfile)
	}
	// A second init adds a profile, keeps the user's files as they are and
	// brings unibox's managed ones back to its own: an edited one rewritten,
	// one this unibox no longer has taken out, the user's beside them kept.
	original, _ := os.ReadFile(filepath.Join(root, ".shared", "@install", "base.sh"))
	if !strings.Contains(string(original), "Managed by unibox") {
		t.Fatalf("%s", original)
	}
	if err := testenv.WriteExecutable(filepath.Join(root, ".shared", "@install", "base.sh"), []byte("#!/bin/sh\necho mine\n")); err != nil {
		t.Fatal(err)
	}
	if err := testenv.WriteExecutable(filepath.Join(root, ".shared", "@install", "tools", "gone.sh"), []byte("#!/bin/sh\n")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "_shared", "@install", "tools"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := testenv.WriteExecutable(filepath.Join(root, "_shared", "@install", "tools", "mine.sh"), []byte("#!/bin/sh\n")); err != nil {
		t.Fatal(err)
	}
	if err := testenv.WriteExecutable(filepath.Join(root, "_shared", "@install", "custom.sh"), []byte("#!/bin/sh\necho my custom\n")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sandboxes", "my2n", "instructions.md"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var second bytes.Buffer
	c = New(paths, strings.NewReader(""), &second, io.Discard)
	c.SetArgs([]string{"init", "personal", "--agents", "claude,codex", "--cpus", "2", "--memory", "4g"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, ".shared", "@install", "base.sh")); string(b) != string(original) {
		t.Fatal("init kept an edited managed file")
	}
	for rel, want := range map[string]bool{"_shared/@install/tools/mine.sh": true, ".shared/@install/tools/gone.sh": false} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); (err == nil) != want {
			t.Fatalf("%s: %v", rel, err)
		}
	}
	for rel, want := range map[string]string{"sandboxes/my2n/instructions.md": "mine\n", "_shared/@install/custom.sh": "#!/bin/sh\necho my custom\n"} {
		if b, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))); string(b) != want {
			t.Fatalf("init rewrote the user's %s", rel)
		}
	}
	if !strings.Contains(second.String(), "updated .shared/@install/base.sh to unibox's own") || !strings.Contains(second.String(), "removed .shared/@install/tools/gone.sh") {
		t.Fatalf("%s", second.String())
	}
	personal, err := unibox.LoadProfile(root, "personal")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(personal.Agents, []string{"claude", "codex"}) || personal.CPUs != "2" || personal.Memory != "4g" {
		t.Fatalf("%v %s %s", personal.Agents, personal.CPUs, personal.Memory)
	}
	c = New(paths, strings.NewReader(""), io.Discard, io.Discard)
	c.SetArgs([]string{"init", "bad", "--agents", "nonsense"})
	if err := c.Execute(); err == nil {
		t.Fatal("unknown agent accepted")
	}
	// validate approves the generated repository and names what breaks it.
	var report bytes.Buffer
	c = New(paths, strings.NewReader(""), &report, io.Discard)
	c.SetArgs([]string{"profile", "validate"})
	if err := c.Execute(); err != nil {
		t.Fatalf("%v: %s", err, report.String())
	}
	// The user's own files anywhere else are theirs to arrange.
	if err := os.WriteFile(filepath.Join(root, "stray.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "_shared", "@instructions", "notes.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	report.Reset()
	c = New(paths, strings.NewReader(""), &report, io.Discard)
	c.SetArgs([]string{"profile", "validate"})
	if err := c.Execute(); err == nil || !strings.Contains(report.String(), "_shared/@instructions/notes.txt") || strings.Contains(report.String(), "stray.txt") {
		t.Fatalf("%v: %s", err, report.String())
	}
}

func TestHelpListsLifecycleAndMultipleRepositories(t *testing.T) {
	t.Parallel()
	paths, _ := cliFixture(t)
	var out bytes.Buffer
	c := New(paths, strings.NewReader(""), &out, io.Discard)
	c.SetArgs([]string{"run", "--help"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--repo", "repeatable", "--branch", "shared branch", "--runtime", "--task", "--push"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("help lacks %s", want)
		}
	}
}

func TestTasksListsEverySandboxAndResumeNeedsOnlyTheTask(t *testing.T) {
	t.Parallel()
	paths, _ := cliFixture(t)
	err := paths.State(func(s *unibox.State) error {
		for _, name := range []string{"work", "personal"} {
			s.Sandboxes[name] = api.Sandbox{Name: name, Runtime: "gone", RuntimeName: "unibox-" + name, Profile: "node", Runs: []api.Run{}, Tasks: []api.Task{
				{ID: name + "-task", Sandbox: name, Directory: "/work/" + name + "-task", Agent: "claude", State: "ready", Repositories: []api.Repository{{Name: "api", Branch: "feat/" + name}}},
			}}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	c := New(paths, strings.NewReader(""), &out, io.Discard)
	c.SetArgs([]string{"task", "ls"})
	if err = c.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"SANDBOX", "REPOSITORIES", "personal-task  personal", "work-task      work", "api@feat/work"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("tasks lacks %q:\n%s", want, out.String())
		}
	}
	out.Reset()
	c = New(paths, strings.NewReader(""), &out, io.Discard)
	c.SetArgs([]string{"task", "ls", "-s", "personal"})
	if err = c.Execute(); err != nil || strings.Contains(out.String(), "work-task") || !strings.Contains(out.String(), "personal-task") {
		t.Fatalf("%v:\n%s", err, out.String())
	}
	// The sandbox is found from the task; the error then comes from its
	// runtime, which this fixture does not have.
	c = New(paths, strings.NewReader(""), io.Discard, io.Discard)
	c.SetArgs([]string{"resume", "personal-task"})
	if err = c.Execute(); err == nil || strings.Contains(err.Error(), "no sandbox chosen") || !strings.Contains(err.Error(), "gone") {
		t.Fatalf("%v", err)
	}
	c = New(paths, strings.NewReader(""), io.Discard, io.Discard)
	c.SetArgs([]string{"resume", "nothing"})
	if err = c.Execute(); err == nil || !strings.Contains(err.Error(), "unibox task ls lists them") {
		t.Fatalf("%v", err)
	}
	// An alias is listed beside the ID and resumes as the ID does.
	out.Reset()
	c = New(paths, strings.NewReader(""), &out, io.Discard)
	c.SetArgs([]string{"task", "rename", "personal-task", "blog"})
	if err = c.Execute(); err != nil || !strings.Contains(out.String(), "personal-task is also blog") {
		t.Fatalf("%v:\n%s", err, out.String())
	}
	out.Reset()
	c = New(paths, strings.NewReader(""), &out, io.Discard)
	c.SetArgs([]string{"task", "ls"})
	if err = c.Execute(); err != nil || !strings.Contains(out.String(), "ALIAS") || !strings.Contains(out.String(), "personal-task  blog   personal") || !strings.Contains(out.String(), "work-task      -      work") {
		t.Fatalf("%v:\n%s", err, out.String())
	}
	c = New(paths, strings.NewReader(""), io.Discard, io.Discard)
	c.SetArgs([]string{"resume", "blog"})
	if err = c.Execute(); err == nil || !strings.Contains(err.Error(), "gone") {
		t.Fatalf("%v", err)
	}
}

func TestVersionIsTheReleaseAloneAndJSONAddsTheFormats(t *testing.T) {
	t.Parallel()
	paths, _ := cliFixture(t)
	for _, args := range [][]string{{"--version"}, {"version"}} {
		var out bytes.Buffer
		c := New(paths, strings.NewReader(""), &out, io.Discard)
		c.SetArgs(args)
		if err := c.Execute(); err != nil || strings.TrimSpace(out.String()) != version.String() {
			t.Fatalf("%v: %v %q", args, err, out.String())
		}
	}
	var out bytes.Buffer
	c := New(paths, strings.NewReader(""), &out, io.Discard)
	c.SetArgs([]string{"version", "--json"})
	if err := c.Execute(); err != nil || !strings.Contains(out.String(), fmt.Sprintf(`"profile_format": %d`, unibox.ProfileFormat)) || !strings.Contains(out.String(), `"version": "`+version.String()+`"`) {
		t.Fatalf("%v %s", err, out.String())
	}
}

func TestTheContextSaysTheSandboxAndProfileWhenNotTold(t *testing.T) {
	t.Parallel()
	paths, _ := cliFixture(t)
	add := func(names ...string) {
		err := paths.State(func(s *unibox.State) error {
			for _, name := range names {
				s.Sandboxes[name] = api.Sandbox{Name: name, Runtime: "gone", RuntimeName: "unibox-" + name, Profile: "node", Runs: []api.Run{}, Tasks: []api.Task{}}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	add("work")
	// The only sandbox and the only profile need no flag.
	out, _, err := run(t, paths, "", "context")
	if err != nil || !strings.Contains(out, "sandbox  work (the only one)") || !strings.Contains(out, "profile  node (the only one)") {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, _, err = run(t, paths, "", "agent", "ls"); err != nil {
		t.Fatal(err)
	}
	add("personal")
	if _, _, err = run(t, paths, "", "script", "ls"); err == nil || !strings.Contains(err.Error(), "pass -s (one of personal, work)") {
		t.Fatalf("%v", err)
	}
	if _, _, err = run(t, paths, "", "context", "use", "-s", "missing"); err == nil {
		t.Fatal("a missing sandbox became the context")
	}
	if out, _, err = run(t, paths, "", "context", "use", "-s", "personal"); err != nil || !strings.Contains(out, "sandbox  personal\n") {
		t.Fatalf("%v\n%s", err, out)
	}
	// The sandbox is found; the error then comes from its runtime, which
	// this fixture does not have.
	if _, _, err = run(t, paths, "", "sandbox", "stop"); err == nil || strings.Contains(err.Error(), "no sandbox chosen") {
		t.Fatalf("%v", err)
	}
	if out, _, err = run(t, paths, "", "context", "clear"); err != nil || strings.Contains(out, "personal") {
		t.Fatalf("%v\n%s", err, out)
	}
}

func TestEveryFieldOfAProfileIsReadAndWrittenThroughUnibox(t *testing.T) {
	t.Parallel()
	paths, root := cliFixture(t)
	file := filepath.Join(root, "sandboxes", "node", "sandbox.yaml")
	if err := os.MkdirAll(filepath.Join(root, "sandboxes", "node", "_data"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sandboxes", "node", "_data", "settings.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"resources.cpus", "6"},
		{"agents.claude.args", "[--model, opus]"},
		{"agents.claude.args_override", "true"},
		{"agents.claude.config[settings.json]", "_data/settings.json"},
		{"network.ports.web", "{port: '3000', default: off}"},
		{"env[0]", "BUILD_KIND"},
		{"vars.artifactory-user", "your Artifactory user name"},
		{"env[]", "SPRING_PROFILES_ACTIVE"},
		{"network.allow[]", "jitpack.io"},
		{"agents[]", "codex"},
	} {
		if _, _, err := run(t, paths, "", append([]string{"profile", "set"}, args...)...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	data, _ := os.ReadFile(file)
	for _, want := range []string{"cpus: 6", "  claude:\n    args: [--model, opus]\n    args_override: true\n    config:\n      settings.json: _data/settings.json", "web: {port: '3000', default: off}", "env: [BUILD_KIND, SPRING_PROFILES_ACTIVE]", "allow: [jitpack.io]", "  codex: {}", "artifactory-user: your Artifactory user name"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("no %q in\n%s", want, data)
		}
	}
	// A field the profile has not, or a value of the wrong form, is not kept.
	for _, args := range [][]string{{"resources.gpus", "1"}, {"resources.cpus", "lots"}, {"agents.claude.args", "--model"}, {"env[3]", "X"}, {"version", "1"}, {"image.name", "x"}, {"resources[]", "1"}, {"env[]x", "Y"}} {
		if _, _, err := run(t, paths, "", append([]string{"profile", "set"}, args...)...); err == nil {
			t.Fatalf("%v was kept", args)
		}
	}
	if after, _ := os.ReadFile(file); string(after) != string(data) {
		t.Fatalf("a refused change was written:\n%s", after)
	}
	// The document in JSON, in the file's order, for a program to render.
	out, _, err := run(t, paths, "", "profile", "show", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var shown struct {
		Profile  string          `json:"profile"`
		Document json.RawMessage `json:"document"`
	}
	var compact bytes.Buffer
	if err = json.Unmarshal([]byte(out), &shown); err == nil {
		err = json.Compact(&compact, shown.Document)
	}
	if err != nil || shown.Profile != "node" || !strings.Contains(compact.String(), `"args":["--model","opus"],"args_override":true`) {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, path := range []string{"agents.claude.config[settings.json]", "agents.claude.args", "agents.claude.args_override", "network.ports.web", "network.allow[0]", "vars.artifactory-user", "agents.codex"} {
		if _, _, err = run(t, paths, "", "profile", "unset", path); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
	data, _ = os.ReadFile(file)
	if strings.Contains(string(data), "\nnetwork:") || strings.Contains(string(data), "\nvars:") || !strings.Contains(string(data), "agents: [claude]") {
		t.Fatalf("%s", data)
	}
	if _, _, err = run(t, paths, "", "profile", "unset", "caches"); err == nil || !strings.Contains(err.Error(), "sets no caches") {
		t.Fatalf("%v", err)
	}
	// A profile without a Dockerfile keeps its image.
	if _, _, err = run(t, paths, "", "profile", "unset", "image"); err == nil || !strings.Contains(err.Error(), "was not kept") {
		t.Fatalf("%v", err)
	}
}

func TestInteractiveInitOffersChoicesAndAnAnswerOfOnesOwn(t *testing.T) {
	// Accessible mode asks the same questions as numbered lines, which a
	// test can answer.
	t.Setenv("ACCESSIBLE", "1")
	paths, root := cliFixture(t)
	// Name; agents: claude besides the preset copilot, then done; CPUs:
	// something else, 3; memory: the second offered, 8g.
	answers := "web\n1\n0\n6\n3\n2\n"
	out, log, err := run(t, paths, answers, "init", "-i", "--dir", root)
	if err != nil {
		t.Fatalf("%v\n%s", err, log)
	}
	data, _ := os.ReadFile(filepath.Join(root, "sandboxes", "web", "sandbox.yaml"))
	for _, want := range []string{"cpus: 3", "memory: 8g", "\nagents:\n  claude:\n", "\n  copilot:\n"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("no %q in\n%s\n%s", want, data, out)
		}
	}
	for _, want := range []string{"Profile name", "Agents", "something else", "Memory"} {
		if !strings.Contains(log, want) {
			t.Fatalf("not asked %q:\n%s", want, log)
		}
	}
	// An answer of the wrong form is asked again, not taken.
	if _, log, err = run(t, paths, "Bad Name\nmobile\n0\n1\n1\n", "init", "-i", "--dir", root); err != nil || !strings.Contains(log, "invalid name") {
		t.Fatalf("%v\n%s", err, log)
	}
	if _, err = os.Stat(filepath.Join(root, "sandboxes", "mobile", "sandbox.yaml")); err != nil {
		t.Fatal(err)
	}
}
