package unibox

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/tobola/unibox/internal/testenv"
	"github.com/tobola/unibox/internal/unibox/api"
)

func TestMain(m *testing.M) {
	testenv.Tools = append(testenv.Tools, "base64", "awk", "bash")
	testenv.Main(m)
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}
func profileFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, filepath.Join(root, "sandboxes", "node", "sandbox.yaml"), "version: 6\nagents: [claude, codex]\nresources: {cpus: 2, memory: 1g}\nstart: start.sh\n")
	write(t, filepath.Join(root, "sandboxes", "node", "sandbox.dockerfile"), "FROM example/shell\n")
	write(t, filepath.Join(root, "sandboxes", "node", "start.sh"), "echo started >> \"$HOME/start-count\"\n")
	return root
}

func TestProfilesRejectUnknownFieldsAndEscapes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, body string }{
		{"typo", "version: 6\nagents: [claude]\nmemmory: 1g\n"},
		{"both", "version: 6\nimage: example/image\nagents: [claude]\n"},
		{"credential", "version: 6\nimage: example/image\nagents: [claude]\nenv: [GITHUB_TOKEN]\n"},
		{"script", "version: 6\nimage: example/image\nagents: [claude]\nstart: ../../host.sh\n"},
		{"documents", "version: 6\nimage: example/image\nagents: [claude]\n---\nimage: another\n"},
		{"agent-config-moved", "version: 6\nimage: example/image\nagents: [claude]\nagent_config: {claude: c.json}\n"},
		{"agent-unknown-field", "version: 6\nimage: example/image\nagents: {claude: {colour: red}}\n"},
		{"seed-unknown-file", "version: 6\nimage: example/image\nagents: {codex: {config: {codex.json: start.sh}}}\n"},
		{"seed-missing-file", "version: 6\nimage: example/image\nagents: {claude: {config: {.claude.json: absent.json}}}\n"},
		{"seed-not-an-object", "version: 6\nimage: example/image\nagents: {claude: {config: {.claude.json: start.sh}}}\n"},
		{"seed-not-toml", "version: 6\nimage: example/image\nagents: {codex: {config: {config.toml: start.sh}}}\n"},
		{"allow-list", "version: 6\nimage: example/image\nagents: [claude]\nnetwork: {allow: [\"a.example,b.example\"]}\n"},
		{"allow-everything-bare", "version: 6\nimage: example/image\nagents: [claude]\nnetwork: {allow: [\"*\"]}\n"},
		{"deny-space", "version: 6\nimage: example/image\nagents: [claude]\nnetwork: {deny: [\"a example\"]}\n"},
		{"allowed-and-denied", "version: 6\nimage: example/image\nagents: [claude]\nnetwork: {allow: [a.example], deny: [a.example]}\n"},
		{"service-host-denied", "version: 6\nimage: example/image\nagents: [claude]\nnetwork: {deny: [gitlab.com]}\nservices: {gitlab: {git: true, host: gitlab.com, user: oauth2, command: c}}\n"},
		{"auth-moved", "version: 6\nimage: example/image\nagents: [claude]\nauth: {claude: oauth}\n"},
		{"co-authors-moved", "version: 6\nimage: example/image\nagents: [claude]\ngit: {co_authors: {claude: C <c@example.com>}}\n"},
		{"auth-unknown", "version: 6\nimage: example/image\nagents: {claude: {auth: password}}\n"},
		{"auth-empty-command", "version: 6\nimage: example/image\nagents: {claude: {auth: {command: ''}}}\n"},
		{"auth-other-key", "version: 6\nimage: example/image\nagents: {claude: {auth: {script: x}}}\n"},
		{"service-no-host", "version: 6\nimage: example/image\nagents: [claude]\nservices: {github: {git: true, user: u, command: gh auth token}}\n"},
		{"service-no-command", "version: 6\nimage: example/image\nagents: [claude]\nservices: {github: {git: true, user: u, host: github.com}}\n"},
		{"service-env-field", "version: 6\nimage: example/image\nagents: [claude]\nservices: {x: {git: true, env: TOK, host: h.example, command: c, scheme: raw}}\n"},
		{"service-scheme-without-git", "version: 6\nimage: example/image\nagents: [claude]\nservices: {x: {host: h.example, command: c, scheme: raw}}\n"},
		{"service-basic-no-user", "version: 6\nimage: example/image\nagents: [claude]\nservices: {x: {git: true, host: h.example, command: c}}\n"},
		{"service-env-basic", "version: 6\nimage: example/image\nagents: [claude]\nservices: {x: {env: TOK, host: h.example, command: c, scheme: basic, user: u}}\n"},
		{"service-bad-scheme", "version: 6\nimage: example/image\nagents: [claude]\nservices: {x: {git: true, host: h.example, command: c, scheme: digest, user: u}}\n"},
		{"service-user-without-basic", "version: 6\nimage: example/image\nagents: [claude]\nservices: {x: {git: true, host: h.example, command: c, scheme: bearer, user: u}}\n"},
		{"env-bad-name", "version: 6\nimage: example/image\nagents: [claude]\nenv: [{1x: v}]\n"},
		{"service-host-star", "version: 6\nimage: example/image\nagents: [claude]\nservices: {x: {git: true, host: '*', command: c, user: u}}\n"},
		{"service-bad-refresh", "version: 6\nimage: example/image\nagents: [claude]\nservices: {x: {git: true, host: h.example, command: c, user: u, refresh: soon}}\n"},
		{"service-author-on-env", "version: 6\nimage: example/image\nagents: [claude]\nservices: {x: {host: h.example, command: c, author: 'A <a@example.com>'}}\n"},
		{"service-bad-author", "version: 6\nimage: example/image\nagents: [claude]\nservices: {x: {git: true, host: h.example, command: c, user: u, author: nobody}}\n"},
		{"service-path-on-env", "version: 6\nimage: example/image\nagents: [claude]\nservices: {x: {host: h.example/org, command: c}}\n"},
		{"service-path-escapes", "version: 6\nimage: example/image\nagents: [claude]\nservices: {x: {git: true, host: h.example/org/../other, command: c, user: u}}\n"},
		{"service-git-wildcard", "version: 6\nimage: example/image\nagents: [claude]\nservices: {x: {git: true, host: '*.example.com', command: c, user: u}}\n"},
		{"service-git-same-url", "version: 6\nimage: example/image\nagents: [claude]\nservices: {a: {git: true, host: h.example/org, command: c, user: u}, b: {git: true, host: h.example/org/, command: c, user: u}}\n"},
		{"service-refresh-too-short", "version: 6\nimage: example/image\nagents: [claude]\nservices: {x: {git: true, host: h.example, command: c, user: u, refresh: 5s}}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := profileFixture(t)
			write(t, filepath.Join(root, "sandboxes", "node", "sandbox.yaml"), tc.body)
			if _, err := LoadProfile(root, "node"); err == nil {
				t.Fatal("invalid profile was accepted")
			}
		})
	}
}

func TestContentRevisionsAndApprovalHaveDifferentInputs(t *testing.T) {
	t.Parallel()
	root := profileFixture(t)
	p, err := LoadProfile(root, "node")
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "sandboxes", "node", "sandbox.dockerfile"), "FROM example/second\n")
	changed, _ := LoadProfile(root, "node")
	if changed.Revision == p.Revision {
		t.Fatal("uncommitted Dockerfile edit did not change revision")
	}
	if changed.AccessHash() != p.AccessHash() {
		t.Fatal("Dockerfile changed host approval")
	}
	write(t, filepath.Join(root, "sandboxes", "node", "sandbox.dockerfile"), "FROM example/third\n")
	third, _ := LoadProfile(root, "node")
	if third.Revision == changed.Revision {
		t.Fatal("two dirty versions shared a revision")
	}
	write(t, filepath.Join(root, "sandboxes", "node", "sandbox.yaml"), "version: 6\nagents: [claude, codex]\nresources: {cpus: 8, memory: 12g}\nstart: start.sh\n")
	resource, _ := LoadProfile(root, "node")
	if resource.Revision != third.Revision {
		t.Fatal("resource-only change rebuilt image")
	}
	if resource.AccessHash() != third.AccessHash() {
		t.Fatal("resource-only change changed approval")
	}
	write(t, filepath.Join(root, "sandboxes", "node", "sandbox.yaml"), "version: 6\nagents: [claude, codex]\nenv: [BUILD_MODE]\n")
	access, _ := LoadProfile(root, "node")
	if access.AccessHash() == resource.AccessHash() {
		t.Fatal("new host environment access kept approval")
	}
}

func TestInstructionsAreSandboxWideAndConfined(t *testing.T) {
	t.Parallel()
	root := profileFixture(t)
	write(t, filepath.Join(root, "_shared", "@instructions", "team.md"), "team instructions")
	write(t, filepath.Join(root, "_shared", "@instructions", "review.md"), "review instructions")
	p, _ := LoadProfile(root, "node")
	text, err := p.InstructionText([]string{"team", "review"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(text, "team instructions") || strings.Index(text, "team instructions") > strings.Index(text, "review instructions") {
		t.Fatal(text)
	}
	if _, err = p.InstructionText([]string{"missing"}); err == nil {
		t.Fatal("missing instructions accepted")
	}
	outside := filepath.Join(t.TempDir(), "outside")
	write(t, outside, "private")
	if err = os.Symlink(outside, filepath.Join(root, "_shared", "@instructions", "escape.md")); err != nil {
		t.Fatal(err)
	}
	if _, err = p.InstructionText([]string{"escape"}); err == nil {
		t.Fatal("instructions escaped repository")
	}
}

func TestProfileInstructionsAreNamesAndAttachOnce(t *testing.T) {
	t.Parallel()
	root := profileFixture(t)
	write(t, filepath.Join(root, "_shared", "@instructions", "team.md"), "team instructions")
	write(t, filepath.Join(root, "_shared", "@instructions", "backend.md"), "backend instructions")
	// Any directory of the repository holds texts a profile names by path;
	// a name is the user's text in _shared/@instructions before unibox's.
	write(t, filepath.Join(root, "my-dir", "notes.md"), "shared notes")
	write(t, filepath.Join(root, ".shared", "@instructions", "team.md"), "unibox's team text")
	write(t, filepath.Join(root, ".shared", "@instructions", "unibox.md"), "unibox's own")
	write(t, filepath.Join(root, "sandboxes", "node", "instructions.md"), "node's own instructions")
	write(t, filepath.Join(root, "sandboxes", "node", "sandbox.yaml"), "version: 6\nagents: [claude]\nstart: start.sh\ninstructions: [backend, team, unibox, ../../my-dir/notes.md]\n")
	p, err := LoadProfile(root, "node")
	if err != nil {
		t.Fatal(err)
	}
	text, err := p.InstructionText([]string{"team"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(text, "team instructions") != 1 || !strings.HasPrefix(text, "backend instructions") || strings.Contains(text, "unibox's team text") || !strings.Contains(text, "unibox's own") {
		t.Fatal(text)
	}
	if !strings.Contains(text, "shared notes") || !strings.Contains(text, "node's own instructions") {
		t.Fatal(text)
	}
	if strings.Index(text, "node's own instructions") < strings.Index(text, "shared notes") {
		t.Fatal("the profile's own instructions.md must follow its listed texts")
	}
	if _, err = p.InstructionText([]string{"../../my-dir/notes.md"}); err == nil {
		t.Fatal("an attached instruction took a path")
	}
	write(t, filepath.Join(root, "sandboxes", "node", "sandbox.yaml"), "version: 6\nagents: [claude]\nstart: start.sh\ninstructions: [absent]\n")
	if _, err = LoadProfile(root, "node"); err == nil {
		t.Fatal("missing instruction name accepted")
	}
	write(t, filepath.Join(root, "sandboxes", "node", "sandbox.yaml"), "version: 6\nagents: [claude]\nstart: start.sh\ninstructions: [../../_shared/absent.md]\n")
	if _, err = LoadProfile(root, "node"); err == nil {
		t.Fatal("missing instruction path accepted")
	}
}

func TestOnlyImageInputsTurnTheRevision(t *testing.T) {
	t.Parallel()
	root := profileFixture(t)
	write(t, filepath.Join(root, "sandboxes", "node", "_data", "claude.json"), `{"theme": "dark"}`)
	write(t, filepath.Join(root, "sandboxes", "node", "sandbox.yaml"), "version: 6\nagents: {claude: {config: {.claude.json: _data/claude.json}}}\nstart: start.sh\n")
	before, err := LoadProfile(root, "node")
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "sandboxes", "node", "instructions.md"), "own text")
	write(t, filepath.Join(root, "sandboxes", "node", "_data", "claude.json"), `{"theme": "light"}`)
	after, err := LoadProfile(root, "node")
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision {
		t.Fatal("instructions and seeds are not image inputs; they must not outdate it")
	}
	write(t, filepath.Join(root, "sandboxes", "node", ".sbx", "build.args"), "BASE=example/sbx\n")
	changed, err := LoadProfile(root, "node")
	if err != nil {
		t.Fatal(err)
	}
	if changed.Revision == after.Revision {
		t.Fatal("build arguments are image inputs; they must turn the revision")
	}
}

func TestTheRepositoryLayoutIsHeldToItsShape(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, path, content string }{
		{"old-layout", "_shared/.install", "x"},
		{"file-in-sandboxes", "sandboxes/readme.txt", "x"},
		{"non-md-instruction", "_shared/@instructions/run.sh", "x"},
		{"reserved-instruction-name", "_shared/@instructions/instructions.md", "x"},
		{"non-md-managed-instruction", ".shared/@instructions/run.sh", "x"},
		{"unhidden-runtime-dir", "sandboxes/node/sbx/build.args", "BASE=x"},
		{"stray-profile-file", "sandboxes/node/notes.txt", "x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := profileFixture(t)
			write(t, filepath.Join(root, filepath.FromSlash(tc.path)), tc.content)
			if _, err := LoadProfile(root, "node"); err == nil {
				t.Fatalf("%s was accepted", tc.path)
			}
		})
	}
	t.Run("underscore-readme-and-hidden-pass", func(t *testing.T) {
		t.Parallel()
		root := profileFixture(t)
		write(t, filepath.Join(root, "README.md"), "about")
		write(t, filepath.Join(root, ".gitignore"), "x")
		write(t, filepath.Join(root, "_notes", "ideas.md"), "x")
		if _, err := LoadProfile(root, "node"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("profile-without-yaml", func(t *testing.T) {
		t.Parallel()
		root := profileFixture(t)
		write(t, filepath.Join(root, "sandboxes", "empty", "sandbox.dockerfile"), "FROM example/shell\n")
		if _, err := LoadProfile(root, "node"); err == nil {
			t.Fatal("profile directory without profile.yaml was accepted")
		}
	})
}

func TestSharedDirectoryJoinsTheBuildContextAndRevision(t *testing.T) {
	t.Parallel()
	root := profileFixture(t)
	before, err := LoadProfile(root, "node")
	if err != nil {
		t.Fatal(err)
	}
	for _, shared := range []string{"_shared", ".shared"} {
		write(t, filepath.Join(root, shared, "@install", "claude.sh"), "npm install -g claude\n")
		p, err := LoadProfile(root, "node")
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := p.Files[shared+"/@install/claude.sh"]; !ok {
			t.Fatalf("%s missing from the build context: %v", shared, p.Files)
		}
		if p.Revision == before.Revision {
			t.Fatalf("a %s change left the image current", shared)
		}
		before = p
		// Shared texts go to the agent, not into the image.
		write(t, filepath.Join(root, shared, "@instructions", "team.md"), "team")
		if p, err = LoadProfile(root, "node"); err != nil || p.Revision != before.Revision {
			t.Fatalf("a shared text outdated the image: %v", err)
		}
		write(t, filepath.Join(root, "sandboxes", "node", shared, "own.sh"), "x")
		if _, err = LoadProfile(root, "node"); err == nil {
			t.Fatalf("a profile's own %s was accepted", shared)
		}
		_ = os.RemoveAll(filepath.Join(root, "sandboxes", "node", shared))
	}
}

func TestRuntimeBuildArgumentsStaySeparate(t *testing.T) {
	t.Parallel()
	root := profileFixture(t)
	write(t, filepath.Join(root, "sandboxes", "node", ".sbx", "build.args"), "BASE=example/sbx\n")
	write(t, filepath.Join(root, "sandboxes", "node", ".podman", "build.args"), "BASE=example/podman\n")
	p, err := LoadProfile(root, "node")
	if err != nil {
		t.Fatal(err)
	}
	args, err := p.BuildArgs("sbx")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "UNIBOX_RUNTIME=sbx") || !strings.Contains(joined, "BASE=example/sbx") || strings.Contains(joined, "example/podman") {
		t.Fatal(args)
	}
	write(t, filepath.Join(root, "sandboxes", "node", ".sbx", "extra"), "bad")
	if _, err = LoadProfile(root, "node"); err == nil {
		t.Fatal("unknown runtime file accepted")
	}
}

func TestCloneURLsLoseCredentialsAndAmbiguousSSHIsRefused(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]string{"https://user:fake-secret@example.com/team/repo.git?token=fake": "https://example.com/team/repo.git", "git@example.com:team/repo.git": "https://example.com/team/repo.git", "ssh://git@example.com/team/repo.git": "https://example.com/team/repo.git"} {
		got, err := CloneURL(raw)
		if err != nil || got != want {
			t.Fatalf("%s: %s %v", raw, got, err)
		}
	}
	for _, raw := range []string{"ssh://git@example.com:2222/team/repo.git", "file:///tmp/repo", "/tmp/repo", "http://example.com/repo.git"} {
		if _, err := CloneURL(raw); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestStateWritesDoNotLoseConcurrentTasks(t *testing.T) {
	t.Parallel()
	paths := Paths{Config: t.TempDir(), Cache: t.TempDir()}
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- paths.State(func(s *State) error {
				name := fmt.Sprintf("box-%d", i)
				s.Sandboxes[name] = api.Sandbox{Name: name}
				return nil
			})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	state, err := paths.ReadState()
	if err != nil || len(state.Sandboxes) != 20 {
		t.Fatalf("state count %d: %v", len(state.Sandboxes), err)
	}
	info, _ := os.Stat(filepath.Join(paths.Config, "state.json"))
	if info.Mode().Perm() != 0o600 {
		t.Fatal(info.Mode())
	}
}

type fakeRuntime struct {
	mu                              sync.Mutex
	root                            string
	boxes                           []Box
	images                          []Image
	creates, removes, stops, builds int
	lastSpec                        Spec
	failBuild                       bool
	remotes                         map[string]string
	scriptError                     bool
}

func (f *fakeRuntime) Name() string                    { return "fake" }
func (f *fakeRuntime) Capabilities() Capabilities      { return Capabilities{Isolation: "container"} }
func (f *fakeRuntime) Available(context.Context) error { return nil }
func (f *fakeRuntime) List(context.Context) ([]Box, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Box{}, f.boxes...), nil
}
func (f *fakeRuntime) Images(context.Context) ([]Image, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Image{}, f.images...), nil
}
func (f *fakeRuntime) Build(_ context.Context, p Profile, tag string, _ io.Writer) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.builds++
	if f.failBuild {
		return "", fmt.Errorf("build failed")
	}
	id := fmt.Sprintf("image-%d", f.builds)
	f.images = append(f.images, Image{tag, id})
	return id, nil
}
func (f *fakeRuntime) Create(_ context.Context, s Spec, _ io.Writer) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creates++
	f.lastSpec = s
	f.boxes = append(f.boxes, Box{Name: s.Name, State: "running", ID: fmt.Sprint(f.creates)})
	return nil
}
func (f *fakeRuntime) Stop(context.Context, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stops++
	return nil
}
func (f *fakeRuntime) Remove(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removes++
	for i, b := range f.boxes {
		if b.Name == name {
			f.boxes = append(f.boxes[:i], f.boxes[i+1:]...)
			break
		}
	}
	return nil
}
func (f *fakeRuntime) Exec(ctx context.Context, name string, c Command) *exec.Cmd {
	boxRoot := filepath.Join(f.root, name)
	_ = os.MkdirAll(filepath.Join(boxRoot, "home"), 0o700)
	replace := func(s string) string {
		s = strings.ReplaceAll(s, "/var/lib/unibox", filepath.Join(boxRoot, "state"))
		s = strings.ReplaceAll(s, "/work", filepath.Join(boxRoot, "work"))
		s = strings.ReplaceAll(s, "/proc/sys/kernel/random/boot_id", filepath.Join(f.root, "boot"))
		s = strings.ReplaceAll(s, "chown -R agent:agent", ":")
		for from, to := range f.remotes {
			s = strings.ReplaceAll(s, from, to)
		}
		return s
	}
	args := append([]string{}, c.Args...)
	for i, a := range args {
		args[i] = replace(a)
	}
	if f.scriptError {
		args = []string{"sh", "-c", "exit 4"}
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	// The guest's global git configuration is its home's, as in a VM.
	cmd.Env = append(HostEnvironment(), "HOME="+filepath.Join(boxRoot, "home"), "GIT_CONFIG_GLOBAL="+filepath.Join(boxRoot, "home", ".gitconfig"))
	if c.Directory != "" {
		cmd.Dir = replace(c.Directory)
	}
	if c.Input != nil {
		b, _ := io.ReadAll(c.Input)
		cmd.Stdin = strings.NewReader(replace(string(b)))
	}
	cmd.Stdout = c.Output
	cmd.Stderr = c.Error
	return cmd
}

func managerFixture(t *testing.T) (*Manager, *fakeRuntime) {
	t.Helper()
	root := profileFixture(t)
	paths := Paths{Config: t.TempDir(), Cache: t.TempDir()}
	var log bytes.Buffer
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(log.String())
		}
	})
	m, err := Open(paths, &log)
	if err != nil {
		t.Fatal(err)
	}
	m.Config.Environments = root
	f := &fakeRuntime{root: t.TempDir(), remotes: map[string]string{}}
	write(t, filepath.Join(f.root, "boot"), "first-boot")
	m.Runtimes = map[string]Runtime{"fake": f}
	if err = m.RecordAccess("node"); err != nil {
		t.Fatal(err)
	}
	if err = m.Build(context.Background(), "node", "fake", false); err != nil {
		t.Fatal(err)
	}
	return m, f
}

func TestSandboxOwnershipAndRuntimeChoice(t *testing.T) {
	t.Parallel()
	m, f := managerFixture(t)
	ctx := context.Background()
	b, err := m.Create(ctx, Request{NewSandbox: "dev", Profile: "node", Runtime: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	if b.Runtime != "fake" || b.RuntimeName != "unibox-dev" || b.RuntimeID == "" {
		t.Fatal(b)
	}
	if _, _, _, err = m.Definition(Request{Sandbox: "dev", Runtime: "sbx"}); err == nil {
		t.Fatal("runtime mismatch accepted")
	}
	f.boxes[0].ID = "someone-else"
	if err = m.Remove(ctx, "dev", ""); err == nil {
		t.Fatal("removed externally replaced sandbox")
	}
	if f.removes != 0 {
		t.Fatal("foreign sandbox was removed")
	}
}

func TestStartRunsOncePerBootAndRecreatePreservesFailedBuild(t *testing.T) {
	t.Parallel()
	m, f := managerFixture(t)
	ctx := context.Background()
	b, err := m.Create(ctx, Request{NewSandbox: "dev", Profile: "node"})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := m.profile("node")
	if err = m.ensure(ctx, &b, f, p, false, false); err != nil {
		t.Fatal(err)
	}
	countFile := filepath.Join(f.root, b.RuntimeName, "home", "start-count")
	count, _ := os.ReadFile(countFile)
	if string(count) != "started\n" {
		t.Fatalf("start count %q", count)
	}
	write(t, filepath.Join(f.root, "boot"), "second-boot")
	if err = m.ensure(ctx, &b, f, p, false, false); err != nil {
		t.Fatal(err)
	}
	count, _ = os.ReadFile(countFile)
	if string(count) != "started\nstarted\n" {
		t.Fatalf("start count %q", count)
	}
	f.failBuild = true
	if err = m.Recreate(ctx, "dev", true); err == nil {
		t.Fatal("failed build was accepted")
	}
	if f.removes != 0 {
		t.Fatal("old sandbox lost before successful build")
	}
}

func TestActiveSessionsBlockRecreateButNotAnotherTask(t *testing.T) {
	t.Parallel()
	m, f := managerFixture(t)
	ctx := context.Background()
	b, err := m.Create(ctx, Request{NewSandbox: "dev", Profile: "node"})
	if err != nil {
		t.Fatal(err)
	}
	usage, err := m.usage("dev", true)
	if err != nil {
		t.Fatal(err)
	}
	defer usage.Close()
	p, _ := m.profile("node")
	if err = m.ensure(ctx, &b, f, p, false, false); err != nil {
		t.Fatalf("unchanged shared environment blocked: %v", err)
	}
	if err = m.Recreate(ctx, "dev", false); err == nil {
		t.Fatal("recreated active environment")
	}
	if f.removes != 0 {
		t.Fatal("removed active environment")
	}
	if err = m.Stop(ctx, "dev"); err == nil {
		t.Fatal("stopped active environment")
	}
}

func TestInstructionChangesWaitWithoutRebuildingOrRecreating(t *testing.T) {
	t.Parallel()
	m, f := managerFixture(t)
	ctx := context.Background()
	b, err := m.Create(ctx, Request{NewSandbox: "dev", Profile: "node"})
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(m.Config.Environments, "_shared", "@instructions", "team.md"), "new team rules")
	if err = m.AttachInstructions("dev", []string{"team"}); err != nil {
		t.Fatal(err)
	}
	b, _ = m.sandbox("dev")
	p, _ := m.profile("node")
	usage, err := m.usage("dev", true)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.ensure(ctx, &b, f, p, false, false); err == nil {
		t.Fatal("instructions changed under running agent")
	}
	usage.Close()
	if err = m.ensure(ctx, &b, f, p, false, false); err != nil {
		t.Fatal(err)
	}
	if f.creates != 1 || f.builds != 1 {
		t.Fatalf("creates=%d builds=%d", f.creates, f.builds)
	}
	data, _ := os.ReadFile(filepath.Join(f.root, b.RuntimeName, "home", ".claude", "CLAUDE.md"))
	if !strings.Contains(string(data), "new team rules") {
		t.Fatal("instructions not updated")
	}
}

func TestAChangedProfileIsToldNotAskedAbout(t *testing.T) {
	t.Parallel()
	m, _ := managerFixture(t)
	root := m.Config.Environments
	write(t, filepath.Join(root, "sandboxes", "node", "sandbox.yaml"), "version: 6\nstart: start.sh\nagents: [claude]\nenv: [BUILD_MODE]\nnetwork: {allow: [jitpack.io]}\n")
	p, err := LoadProfile(root, "node")
	if err != nil {
		t.Fatal(err)
	}
	// check names what changed, as a warning, and remembers nothing.
	check, err := m.Check(context.Background(), Request{NewSandbox: "dev", Profile: "node", Runtime: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, n := range check.Needs {
		if n.Code == "profile_changed" {
			found = n.Warning && strings.Contains(n.Message, "env + BUILD_MODE") && strings.Contains(n.Message, "network.allow + jitpack.io")
		}
		if n.Code == "approval_required" {
			t.Fatal("a change was asked about")
		}
	}
	if !found {
		t.Fatalf("%+v", check.Needs)
	}
	// Starting tells it once and goes on.
	var log bytes.Buffer
	if err := m.noteAccess(p, &log); err != nil || !strings.Contains(log.String(), "env + BUILD_MODE") {
		t.Fatalf("%v %s", err, log.String())
	}
	log.Reset()
	if err := m.noteAccess(p, &log); err != nil || log.Len() != 0 {
		t.Fatalf("told twice: %s", log.String())
	}
}

func TestASingleAgentProfileCreatesItsKindOfSandbox(t *testing.T) {
	t.Parallel()
	m, f := managerFixture(t)
	root := m.Config.Environments
	write(t, filepath.Join(root, "sandboxes", "solo", "sandbox.yaml"), "version: 6\nagents: [copilot]\n")
	write(t, filepath.Join(root, "sandboxes", "solo", "sandbox.dockerfile"), "FROM example/shell\n")
	ctx := context.Background()
	if err := m.Build(ctx, "solo", "fake", false); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(ctx, Request{NewSandbox: "one", Profile: "solo"}); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	kind := f.lastSpec.Agent
	f.mu.Unlock()
	if kind != "copilot" {
		t.Fatalf("single-agent profile made a %q sandbox; its agent's kind binds the runtime's credentials", kind)
	}
	if _, err := m.Create(ctx, Request{NewSandbox: "two", Profile: "node"}); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	kind = f.lastSpec.Agent
	f.mu.Unlock()
	if kind != "" {
		t.Fatalf("a profile of several agents must make a plain sandbox, got %q", kind)
	}
}

func TestOneSandboxFlagNamesASandboxThereOrOneToMake(t *testing.T) {
	t.Parallel()
	m, _ := managerFixture(t)
	ctx := context.Background()
	// Not there and no profile: what to do about it is said.
	if _, _, _, err := m.Definition(Request{Sandbox: "fresh"}); err == nil || !strings.Contains(err.Error(), "pass --profile to make it") {
		t.Fatalf("%v", err)
	}
	// Not there with a profile: it is the one to make.
	b, _, p, err := m.Definition(Request{Sandbox: "fresh", Profile: "node", Runtime: "fake"})
	if err != nil || b.Name != "fresh" || p.Name != "node" {
		t.Fatalf("%v %+v", err, b)
	}
	if _, err := m.Create(ctx, Request{NewSandbox: "fresh", Profile: "node", Runtime: "fake"}); err != nil {
		t.Fatal(err)
	}
	// There: it is used, the profile only checked.
	if b, _, _, err = m.Definition(Request{Sandbox: "fresh", Profile: "node"}); err != nil || b.Name != "fresh" || b.Runtime != "fake" {
		t.Fatalf("%v %+v", err, b)
	}
	if _, err := m.Create(ctx, Request{NewSandbox: "fresh", Profile: "node", Runtime: "fake"}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("%v", err)
	}
}
