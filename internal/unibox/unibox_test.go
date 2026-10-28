package unibox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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

func TestNoSandboxIsCreatedBeforeTheNetworkPolicyIsChosen(t *testing.T) {
	t.Parallel()
	m, f := managerFixture(t)
	ctx := context.Background()
	if err := m.Paths.SaveEnvValues(EnvValues{Envs: map[string]map[string]string{"dev": {"user": "person"}}}); err != nil {
		t.Fatal(err)
	}
	f.policyUnchosen = true
	check, err := m.Check(ctx, Request{Sandbox: "box", Env: "dev", Repositories: []RepoInput{{Source: t.TempDir()}}})
	if err != nil || check.Ready || !slices.ContainsFunc(check.Needs, func(n api.Need) bool { return n.Code == "network_policy" }) {
		t.Fatalf("%+v %v", check, err)
	}
	if _, err := m.Create(ctx, Request{Sandbox: "box", Env: "dev"}, false); err == nil || !strings.Contains(err.Error(), "unibox network init") || len(f.called("env create")) != 0 {
		t.Fatalf("%v %v", err, f.calls)
	}
	if b, _, _ := m.Definition(Request{Sandbox: "box", Env: "dev"}); b.State != "missing" {
		t.Fatalf("the refused sandbox was recorded as %s", b.State)
	}
}

func TestASandboxIsCreatedFromItsEnvWithTheArgsAndKnowsItsAgent(t *testing.T) {
	t.Parallel()
	m, f := managerFixture(t)
	ctx := context.Background()
	if err := m.Paths.SaveEnvValues(EnvValues{Envs: map[string]map[string]string{"dev": {"user": "person"}}}); err != nil {
		t.Fatal(err)
	}
	b, err := m.Create(ctx, Request{Sandbox: "box", Env: "dev"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if b.ID != "id-1" || b.Env != "dev" || b.Agent != "claude" || strings.Join(b.AgentCommand, " ") != "claude --dangerously-skip-permissions" || b.Fingerprint == "" {
		t.Fatalf("%+v", b)
	}
	if calls := f.called("env create box "); len(calls) != 1 || !strings.HasSuffix(calls[0], "dev.sbxenv.yaml") {
		t.Fatalf("%v", f.calls)
	}
	// The args reach sbx in a file of their own, gone once it is read.
	if f.args != "user=person\n" {
		t.Fatalf("%q", f.args)
	}
	if left, _ := os.ReadDir(filepath.Join(m.Paths.Cache, "args")); len(left) != 0 {
		t.Fatalf("the args file stayed: %v", left)
	}
	if _, err := os.Stat(filepath.Join(f.root, "box", "state", "runs")); err != nil {
		t.Fatal("the sandbox was not readied for tasks")
	}
	if _, err := m.Create(ctx, Request{Sandbox: "box", Env: "dev"}, false); err == nil || !strings.Contains(err.Error(), "exists already") {
		t.Fatalf("%v", err)
	}
	// Named by its env alone, the sandbox is the env's.
	if b, _, err := m.Definition(Request{Env: "dev"}); err != nil || b.Name != "dev" || b.State != "missing" {
		t.Fatalf("%v %+v", err, b)
	}
	if _, _, err := m.Definition(Request{Sandbox: "fresh"}); err == nil || !strings.Contains(err.Error(), "pass -e") {
		t.Fatalf("%v", err)
	}
	if _, _, err := m.Definition(Request{Sandbox: "box", Env: "other"}); err == nil {
		t.Fatal("another env was taken for an existing sandbox")
	}
}

func TestACreationThatDidNotFinishIsTriedAgain(t *testing.T) {
	t.Parallel()
	m, f := managerFixture(t)
	ctx := context.Background()
	f.failCreate = true
	if _, err := m.Create(ctx, Request{Sandbox: "box", Env: "dev"}, false); err == nil {
		t.Fatal("a failed creation passed")
	}
	f.failCreate = false
	if b, err := m.Create(ctx, Request{Sandbox: "box", Env: "dev"}, false); err != nil || b.State != "running" || b.ID != "id-1" {
		t.Fatalf("%v %+v", err, b)
	}
	// sbx made it, and unibox stopped before readying it: it is readied
	// without being created again.
	if err := m.Paths.State(func(s *State) error {
		b := s.Sandboxes["box"]
		b.State = "creating"
		s.Sandboxes["box"] = b
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if b, err := m.Create(ctx, Request{Sandbox: "box", Env: "dev"}, false); err != nil || b.State != "running" || b.ID != "id-1" || f.creates != 1 {
		t.Fatalf("%v %+v %d", err, b, f.creates)
	}
}

func TestAMissingArgStopsBeforeSbx(t *testing.T) {
	t.Parallel()
	m, f := managerFixture(t)
	file := m.Config.Envs["dev"].Files[0]
	write(t, file, "schemaVersion: \"1\"\nargs:\n  user:\n    required: true\n    description: Your user name\nkits: [../kits/agent]\n")
	ctx := context.Background()
	check, err := m.Check(ctx, Request{Env: "dev"})
	if err != nil || check.Ready || len(check.Needs) != 1 || check.Needs[0].Code != "arg_missing" || !strings.Contains(check.Needs[0].Message, "unibox var set -e dev user") {
		t.Fatalf("%v %+v", err, check)
	}
	if _, err := m.Create(ctx, Request{Env: "dev"}, false); err == nil {
		t.Fatal("created without a required arg")
	}
	if f.creates != 0 {
		t.Fatal("sbx was asked")
	}
}

func TestASandboxUniboxDidNotMakeIsAdoptedOnlyWhenAskedAndKeptUnlessForced(t *testing.T) {
	t.Parallel()
	m, f := managerFixture(t)
	ctx := context.Background()
	f.boxes = []Box{{Name: "dev", State: "running", ID: "theirs"}}
	if _, err := m.Create(ctx, Request{Env: "dev"}, false); err == nil || !strings.Contains(err.Error(), "--adopt") {
		t.Fatalf("%v", err)
	}
	if _, err := m.Create(ctx, Request{Sandbox: "absent", Env: "dev"}, true); err == nil {
		t.Fatal("adopted what sbx does not have")
	}
	b, err := m.Create(ctx, Request{Env: "dev"}, true)
	if err != nil || !b.Adopted || b.ID != "theirs" || b.Agent != "claude" {
		t.Fatalf("%v %+v", err, b)
	}
	if err := m.Recreate(ctx, "dev"); err == nil || !strings.Contains(err.Error(), "adopted") {
		t.Fatalf("%v", err)
	}
	if err := m.Remove(ctx, "dev", "", false); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("%v", err)
	}
	if err := m.Destroy(ctx, DestroyOptions{}); err != nil {
		t.Fatal(err)
	}
	if f.removes != 0 || len(f.boxes) != 1 {
		t.Fatal("destroy removed an adopted sandbox")
	}
	if s, _ := m.Paths.ReadState(); len(s.Sandboxes) != 0 {
		t.Fatal("destroy kept the adopted sandbox's record")
	}
}

func TestASandboxReplacedOutsideUniboxIsNotRemoved(t *testing.T) {
	t.Parallel()
	m, f := managerFixture(t)
	ctx := context.Background()
	if _, err := m.Create(ctx, Request{Env: "dev"}, false); err != nil {
		t.Fatal(err)
	}
	f.boxes[0].ID = "someone-else"
	if err := m.Remove(ctx, "dev", "", false); err == nil || !strings.Contains(err.Error(), "replaced outside unibox") {
		t.Fatalf("%v", err)
	}
	if f.removes != 0 {
		t.Fatal("a foreign sandbox was removed")
	}
}

func TestAChangedEnvIsToldAndAppliedOnlyByRecreate(t *testing.T) {
	t.Parallel()
	m, f := managerFixture(t)
	ctx := context.Background()
	b, err := m.Create(ctx, Request{Env: "dev"}, false)
	if err != nil {
		t.Fatal(err)
	}
	file := m.Config.Envs["dev"].Files[0]
	data, _ := os.ReadFile(file)
	write(t, file, string(data)+"bindings:\n  github: {apiKey: {domains: [api.github.com]}}\n")
	check, err := m.Check(ctx, Request{Env: "dev"})
	if err != nil || !check.Ready || len(check.Needs) != 1 || check.Needs[0].Code != "env_changed" || !check.Needs[0].Warning {
		t.Fatalf("%v %+v", err, check)
	}
	log := m.Log.(*bytes.Buffer)
	log.Reset()
	if err = m.ensure(ctx, &b, mustEnv(t, m), false); err != nil {
		t.Fatal(err)
	}
	if f.creates != 1 || !strings.Contains(log.String(), "unibox sandbox recreate dev applies it") {
		t.Fatalf("%d creates; %s", f.creates, log)
	}
	// A session blocks recreating it.
	usage, err := m.usage("dev", true)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Recreate(ctx, "dev"); err == nil {
		t.Fatal("recreated a sandbox in use")
	}
	if err = m.Stop(ctx, "dev"); err == nil {
		t.Fatal("stopped a sandbox in use")
	}
	usage.Close()
	if err = m.Recreate(ctx, "dev"); err != nil {
		t.Fatal(err)
	}
	b, _ = m.sandbox("dev")
	if f.creates != 2 || f.envRemoves != 1 || b.ID != "id-2" {
		t.Fatalf("%d creates, %d env removals, %+v", f.creates, f.envRemoves, b)
	}
	if check, _ = m.Check(ctx, Request{Env: "dev"}); len(check.Needs) != 0 {
		t.Fatalf("%+v", check.Needs)
	}
}

func mustEnv(t *testing.T, m *Manager) Env {
	t.Helper()
	env, err := m.Config.LoadEnv("dev")
	if err != nil {
		t.Fatal(err)
	}
	return env
}

func TestPruneForgetsWhatSbxNoLongerHas(t *testing.T) {
	t.Parallel()
	m, f := managerFixture(t)
	ctx := context.Background()
	for _, name := range []string{"one", "two"} {
		if _, err := m.Create(ctx, Request{Sandbox: name, Env: "dev"}, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Remove(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	gone, err := m.Prune(ctx)
	if err != nil || len(gone) != 1 || gone[0] != "one" {
		t.Fatalf("%v %v", err, gone)
	}
	boxes, _ := m.List(ctx)
	if len(boxes) != 1 || boxes[0].Name != "two" || boxes[0].State != "running" {
		t.Fatalf("%+v", boxes)
	}
}

func TestNeededSecretsAreTheKitsCredentialsAndTheEnvsBindings(t *testing.T) {
	t.Parallel()
	m, f := managerFixture(t)
	file := m.Config.Envs["dev"].Files[0]
	data, _ := os.ReadFile(file)
	write(t, file, string(data)+"bindings:\n  github: {apiKey: {domains: [api.github.com]}}\n")
	f.secrets = []Secret{{Scope: "global", Name: "github"}, {Scope: "other", Name: "anthropic"}}
	logins, err := m.NeededSecrets(context.Background(), mustEnv(t, m), "dev")
	if err != nil || len(logins) != 2 {
		t.Fatalf("%v %+v", err, logins)
	}
	if logins[0].Service != "anthropic" || logins[0].Stored || logins[0].Optional || len(logins[0].Kits) != 1 {
		t.Fatalf("%+v", logins[0])
	}
	if logins[1].Service != "github" || !logins[1].Stored || logins[1].Scope != "global" {
		t.Fatalf("%+v", logins[1])
	}
	// Without a sandbox, a token stored for the one the env creates is
	// stored all the same; the kits are not read again.
	env := mustEnv(t, m)
	f.mu.Lock()
	f.secrets = []Secret{{Scope: env.SandboxName(), Name: "anthropic"}}
	f.kits = nil
	f.mu.Unlock()
	logins, err = m.NeededSecrets(context.Background(), env, "")
	if err != nil || !logins[0].Stored || logins[0].Scope != env.SandboxName() || len(logins[0].Kits) != 1 {
		t.Fatalf("%v %+v", err, logins)
	}
}

func TestATokenTwoEnvsSendToDifferentHostsIsTold(t *testing.T) {
	t.Parallel()
	var kit Kit
	// The shape sbx kit inspect --json gives a kit's credentials in.
	if err := json.Unmarshal([]byte(`{"credentials":[{"service":"gitlab","apiKey":{"inject":[{"domain":"gitlab.acme.com"},{"domain":"gitlab.acme.com"}]}}]}`), &kit); err != nil {
		t.Fatal(err)
	}
	acme := kit.Credentials[0].Domains()
	if !slices.Equal(acme, []string{"gitlab.acme.com"}) {
		t.Fatalf("%v", acme)
	}
	needed := map[string][]NeededSecret{
		"dev": {{Service: "gitlab", Domains: acme}, {Service: "github", Domains: []string{"github.com"}}},
		"web": {{Service: "gitlab", Domains: []string{"gitlab.other.org"}}, {Service: "github", Domains: []string{"github.com"}}},
		"ops": {{Service: "gitlab"}},
	}
	clashes := SecretClashes(needed)
	if len(clashes) != 1 || clashes[0].Service != "gitlab" || len(clashes[0].Hosts) != 2 || clashes[0].Hosts["web"][0] != "gitlab.other.org" {
		t.Fatalf("%+v", clashes)
	}
}
