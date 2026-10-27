package unibox

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tobola/unibox/internal/unibox/api"
)

func gitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s (%v)", args, out, err)
	}
	return strings.TrimSpace(string(out))
}
func remoteFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	host := filepath.Join(root, "host")
	gitTest(t, root, "init", "--bare", "--initial-branch=main", remote)
	gitTest(t, root, "clone", remote, host)
	write(t, filepath.Join(host, "code.txt"), "initial\n")
	gitTest(t, host, "add", ".")
	gitTest(t, host, "commit", "-m", "Initial")
	gitTest(t, host, "push", "origin", "main")
	gitTest(t, host, "switch", "-c", "feat/refunds")
	write(t, filepath.Join(host, "feature.txt"), "feature\n")
	gitTest(t, host, "add", ".")
	gitTest(t, host, "commit", "-m", "Feature")
	gitTest(t, host, "push", "-u", "origin", "HEAD")
	gitTest(t, host, "switch", "main")
	return remote, host
}

func TestLocalPathWithBranchDoesNotSelectOrChangeMain(t *testing.T) {
	t.Parallel()
	remote, host := remoteFixture(t)
	url := "https://forge.invalid/team/payments.git"
	gitTest(t, host, "remote", "set-url", "origin", url)
	gitTest(t, host, "config", "url.file://"+remote+".insteadOf", url)
	write(t, filepath.Join(host, "dirty.txt"), "local main edit\n")
	repos, needs, err := ResolveRepositories(context.Background(), Request{Repositories: []RepoInput{{Source: host}}, Branch: "feat/refunds"})
	if err != nil {
		t.Fatal(err)
	}
	if len(needs) != 0 {
		t.Fatalf("main edits affected selected feature: %+v", needs)
	}
	if len(repos) != 1 || repos[0].Branch != "feat/refunds" {
		t.Fatal(repos)
	}
	if got := gitTest(t, host, "branch", "--show-current"); got != "main" {
		t.Fatalf("host branch changed to %s", got)
	}
	// The default branch is a branch like any other to work on.
	repos, needs, err = ResolveRepositories(context.Background(), Request{Repositories: []RepoInput{{Source: host}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(needs) != 1 || needs[0].Code != "uncommitted" || len(repos) != 1 || repos[0].Branch != "main" {
		t.Fatalf("%+v %+v", repos, needs)
	}
}

func TestTaskClonesFromForgeAndKeepsOwnEdits(t *testing.T) {
	t.Parallel()
	remote, host := remoteFixture(t)
	m, f := managerFixture(t)
	url := "https://forge.invalid/team/payments.git"
	f.remotes[url] = "file://" + remote
	ctx := context.Background()
	_, err := m.Create(ctx, Request{Sandbox: "dev", Env: "dev"}, false)
	if err != nil {
		t.Fatal(err)
	}
	req := Request{Sandbox: "dev", Task: "refunds", Author: "Person <person@example.com>", Repositories: []RepoInput{{Source: url, Branch: "feat/refunds"}}}
	prepared, err := m.Prepare(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Directory != "/work/refunds" {
		t.Fatal(prepared)
	}
	clone := filepath.Join(f.root, "dev", "work", "refunds", "payments")
	if got := gitTest(t, clone, "branch", "--show-current"); got != "feat/refunds" {
		t.Fatal(got)
	}
	if got := gitTest(t, host, "branch", "--show-current"); got != "main" {
		t.Fatal("host changed")
	}
	write(t, filepath.Join(clone, "agent.txt"), "uncommitted agent work\n")
	if err = m.Recreate(ctx, "dev"); err == nil {
		t.Fatal("recreated sandbox with uncommitted files")
	}
	if f.removes != 0 {
		t.Fatal("discarded guest work")
	}
	if _, err = m.Prepare(ctx, req); err == nil {
		t.Fatal("prepare overwrote guest edits")
	}
	// The failed explicit preparation marks the task incomplete; restore readiness
	// as a successfully started task would have before the agent made its edits.
	b, _ := m.sandbox("dev")
	b.Tasks[0].State = "ready"
	if err = m.saveBox(b); err != nil {
		t.Fatal(err)
	}
	_, _, usage, taskLock, err := m.prepare(ctx, Request{Sandbox: "dev", Task: "refunds"}, true)
	if err != nil {
		t.Fatalf("cannot resume own edits: %v", err)
	}
	usage.Close()
	taskLock.Close()
	if content, _ := os.ReadFile(filepath.Join(clone, "agent.txt")); string(content) != "uncommitted agent work\n" {
		t.Fatal("own edits changed")
	}
	gitTest(t, clone, "add", "agent.txt")
	// The agent commits in its session, which names it to the hook.
	commit := exec.Command("git", "-C", clone, "commit", "-m", "Agent work")
	commit.Env = append(os.Environ(), "UNIBOX_AGENT=Claude", "UNIBOX_CO_AUTHOR=Claude <noreply@anthropic.com>")
	if out, err := commit.CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	message := gitTest(t, clone, "log", "-1", "--format=%B")
	if strings.Count(message, "Co-Authored-By: Claude <noreply@anthropic.com>") != 1 {
		t.Fatal(message)
	}
	if err = m.Remove(ctx, "dev", "", false); err == nil {
		t.Fatal("removed sandbox with unpushed commit")
	}
	gitTest(t, clone, "push", "origin", "HEAD")
	if err = m.Remove(ctx, "dev", "", false); err != nil {
		t.Fatal(err)
	}
	// The check says what it checked, not every branch the fetch learned.
	if log, ok := m.Log.(*bytes.Buffer); !ok {
		t.Fatal("the fixture's log is not a buffer")
	} else {
		if !strings.Contains(log.String(), "payments: clean, every commit on the forge, no stashes") || strings.Contains(log.String(), "[new branch]") {
			t.Fatalf("the removal check narrates the wrong things:\n%s", log)
		}
	}
	if f.removes != 1 {
		t.Fatal("safe sandbox was not removed")
	}
}

func TestTheDefaultBranchIsPreparedAndAMissingOneRefusedInsideGuest(t *testing.T) {
	t.Parallel()
	for branch, accepted := range map[string]bool{"main": true, "missing": false} {
		t.Run(branch, func(t *testing.T) {
			t.Parallel()
			remote, _ := remoteFixture(t)
			m, f := managerFixture(t)
			url := "https://forge.invalid/team/payments.git"
			f.remotes[url] = "file://" + remote
			ctx := context.Background()
			if _, err := m.Create(ctx, Request{Sandbox: "dev", Env: "dev"}, false); err != nil {
				t.Fatal(err)
			}
			_, err := m.Prepare(ctx, Request{Sandbox: "dev", Task: "t-" + branch, Author: "Person <person@example.com>", Repositories: []RepoInput{{Source: url, Branch: branch}}})
			if (err == nil) != accepted {
				t.Fatalf("%s: %v", branch, err)
			}
		})
	}
}

func TestPushCreatesABranchNobodyHasFromTheHostHead(t *testing.T) {
	t.Parallel()
	remote, host := remoteFixture(t)
	url := "https://forge.invalid/team/payments.git"
	gitTest(t, host, "remote", "set-url", "origin", url)
	gitTest(t, host, "config", "url.file://"+remote+".insteadOf", url)
	ctx := context.Background()
	req := Request{Repositories: []RepoInput{{Source: host}}, Branch: "feat/new", Push: true}
	repos, needs, err := ResolveRepositories(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	missing := false
	for _, n := range needs {
		if n.Code == "branch_missing" {
			missing = true
		}
	}
	if !missing {
		t.Fatalf("a branch nobody has was not reported: %+v", needs)
	}
	if err = PushRepositories(ctx, repos); err != nil {
		t.Fatal(err)
	}
	head := gitTest(t, host, "rev-parse", "HEAD")
	if tip := gitTest(t, host, "ls-remote", "origin", "refs/heads/feat/new"); !strings.HasPrefix(tip, head) {
		t.Fatalf("forge branch is not the host HEAD: %s", tip)
	}
	if got := gitTest(t, host, "branch", "--show-current"); got != "main" {
		t.Fatalf("host branch changed to %s", got)
	}
	if locals := gitTest(t, host, "branch", "--list", "feat/new"); locals != "" {
		t.Fatalf("a local branch was made: %s", locals)
	}
	_, needs, err = ResolveRepositories(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if len(needs) != 0 {
		t.Fatalf("created branch still blocked: %+v", needs)
	}
}

func TestPushCreatesABranchOnTheForgeFromAURL(t *testing.T) {
	t.Parallel()
	remote, _ := remoteFixture(t)
	url := "https://forge-url-push.invalid/team/payments.git"
	gitTest(t, ".", "config", "--global", "url.file://"+remote+".insteadOf", url)
	// The rule is the run's, and the first one for a URL wins: a repeated
	// test must not find this one.
	t.Cleanup(func() { gitTest(t, ".", "config", "--global", "--remove-section", "url.file://"+remote) })
	ctx := context.Background()
	req := Request{Repositories: []RepoInput{{Source: url, Branch: "feat/from-url"}}, Push: true, Author: "Person <person@example.com>"}
	repos, needs, err := ResolveRepositories(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if len(needs) != 0 {
		t.Fatalf("url repository raised host needs: %+v", needs)
	}
	if err = PushRepositories(ctx, repos); err != nil {
		t.Fatal(err)
	}
	main := gitTest(t, remote, "rev-parse", "refs/heads/main")
	if got := gitTest(t, remote, "rev-parse", "refs/heads/feat/from-url"); got != main {
		t.Fatalf("forge branch is not the default tip: %s != %s", got, main)
	}
	// A second push finds the branch and leaves it alone.
	if err = PushRepositories(ctx, repos); err != nil {
		t.Fatal(err)
	}
}

func TestIdleStoppingCountsEverySessionAndOwnsOnlyItsSandbox(t *testing.T) {
	t.Parallel()
	m, f := managerFixture(t)
	b, err := m.Create(context.Background(), Request{Sandbox: "dev", Env: "dev"}, false)
	if err != nil {
		t.Fatal(err)
	}
	u1, err := m.usage("dev", true)
	if err != nil {
		t.Fatal(err)
	}
	u2, err := m.usage("dev", true)
	if err != nil {
		t.Fatal(err)
	}
	r1, l1, err := m.recordRun(b, api.Task{ID: "first"}, "claude")
	if err != nil {
		t.Fatal(err)
	}
	r2, l2, err := m.recordRun(b, api.Task{ID: "second"}, "codex")
	if err != nil {
		t.Fatal(err)
	}
	m.finishRun(b, r1, l1, u1, nil)
	if f.stops != 0 {
		t.Fatal("stopped while another session was alive")
	}
	m.finishRun(b, r2, l2, u2, nil)
	if f.stops != 1 {
		t.Fatal("did not stop last session's sandbox")
	}
}

func TestATaskNamedAloneFindsItsSandbox(t *testing.T) {
	t.Parallel()
	m, _ := managerFixture(t)
	// Two sandboxes holding one name is state from before names were unique.
	ctx := context.Background()
	for _, name := range []string{"dev", "ci"} {
		if _, err := m.Create(ctx, Request{Sandbox: name, Env: "dev"}, false); err != nil {
			t.Fatal(err)
		}
	}
	for name, ids := range map[string][]string{"dev": {"refunds", "shared"}, "ci": {"shared"}} {
		b, _ := m.sandbox(name)
		for _, id := range ids {
			b.Tasks = append(b.Tasks, api.Task{ID: id, Sandbox: name, Directory: "/work/" + id, State: "ready"})
		}
		if err := m.saveBox(b); err != nil {
			t.Fatal(err)
		}
	}
	if b, _, err := m.Definition(Request{Task: "refunds"}); err != nil || b.Name != "dev" {
		t.Fatalf("%s %v", b.Name, err)
	}
	if _, _, err := m.Definition(Request{Task: "shared"}); err == nil || !strings.Contains(err.Error(), "ci, dev; remove the one not wanted") {
		t.Fatalf("%v", err)
	}
	if _, _, err := m.Definition(Request{Task: "missing"}); err == nil || !strings.Contains(err.Error(), "unibox task ls lists them") {
		t.Fatalf("%v", err)
	}
	// A new task named by the user still needs to be told where to go.
	if _, _, err := m.Definition(Request{Task: "missing", Repositories: []RepoInput{{Source: "."}}}); err == nil || !strings.Contains(err.Error(), "no sandbox chosen") {
		t.Fatalf("%v", err)
	}
}

func TestTaskNamesAreUniqueAcrossSandboxes(t *testing.T) {
	t.Parallel()
	remote, _ := remoteFixture(t)
	m, f := managerFixture(t)
	url := "https://forge.invalid/team/payments.git"
	f.remotes[url] = "file://" + remote
	ctx := context.Background()
	for _, name := range []string{"dev", "ci"} {
		if _, err := m.Create(ctx, Request{Sandbox: name, Env: "dev"}, false); err != nil {
			t.Fatal(err)
		}
	}
	req := func(sandbox, task string) Request {
		return Request{Sandbox: sandbox, Task: task, Author: "Person <person@example.com>", Repositories: []RepoInput{{Source: url, Branch: "feat/refunds"}}}
	}
	if _, err := m.Prepare(ctx, req("dev", "refunds")); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Prepare(ctx, req("ci", "refunds")); err == nil || !strings.Contains(err.Error(), "already exists in sandbox dev") {
		t.Fatalf("%v", err)
	}
	ids := map[string]string{}
	for _, name := range []string{"dev", "ci", "ci"} {
		p, err := m.Prepare(ctx, req(name, ""))
		if err != nil {
			t.Fatal(err)
		}
		if old, ok := ids[name]; ok && old != p.Task {
			t.Fatalf("the same repositories in %s became %s and %s", name, old, p.Task)
		}
		ids[name] = p.Task
	}
	if ids["dev"] == ids["ci"] || !strings.HasPrefix(ids["ci"], "payments-") {
		t.Fatal(ids)
	}
}

func TestClaudeTrustsTheTaskItRunsIn(t *testing.T) {
	remote, _ := remoteFixture(t)
	m, f := managerFixture(t)
	url := "https://forge.invalid/team/payments.git"
	f.remotes[url] = "file://" + remote
	bin := t.TempDir()
	seen := filepath.Join(t.TempDir(), "co-author")
	mustWriteExecutable(t, filepath.Join(bin, "claude"), []byte("#!/bin/sh\nprintf '%s' \"$UNIBOX_CO_AUTHOR\" > "+Quote(seen)+"\n"))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx := context.Background()
	if _, err := m.Create(ctx, Request{Sandbox: "dev", Env: "dev"}, false); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(f.root, "dev", "home")
	write(t, filepath.Join(home, ".claude.json"), `{"theme":"dark","projects":{"/elsewhere":{"hasTrustDialogAccepted":false}}}`)
	req := Request{Sandbox: "dev", Task: "refunds", Author: "Person <person@example.com>", Repositories: []RepoInput{{Source: url, Branch: "feat/refunds"}}}
	if err := m.Run(ctx, req, strings.NewReader(""), io.Discard, io.Discard, nil); err != nil {
		t.Fatal(err)
	}
	// The session names its agent to the commit hook.
	if got, _ := os.ReadFile(seen); string(got) != "Claude <noreply@anthropic.com>" {
		t.Fatalf("the session named %q", got)
	}
	data, _ := os.ReadFile(filepath.Join(home, ".claude.json"))
	var cfg struct {
		Theme                         string
		BypassPermissionsModeAccepted bool
		Projects                      map[string]struct{ HasTrustDialogAccepted bool }
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Theme != "dark" || !cfg.BypassPermissionsModeAccepted || cfg.Projects["/elsewhere"].HasTrustDialogAccepted {
		t.Fatalf("%s", data)
	}
	for _, d := range []string{"/work", "/work/refunds", "/work/refunds/payments"} {
		if !cfg.Projects[d].HasTrustDialogAccepted {
			t.Fatalf("%s not trusted: %s", d, data)
		}
	}
}

func TestRunOpensAShellInTheTaskInsteadOfTheAgent(t *testing.T) {
	remote, _ := remoteFixture(t)
	m, f := managerFixture(t)
	url := "https://forge.invalid/team/payments.git"
	f.remotes[url] = "file://" + remote
	bin := t.TempDir()
	seen := filepath.Join(t.TempDir(), "shell")
	realBash, err := exec.LookPath("bash")
	if err != nil {
		t.Fatal(err)
	}
	// Preparing a task runs bash too; only the login shell is the session.
	mustWriteExecutable(t, filepath.Join(bin, "bash"), []byte("#!/bin/sh\n[ \"$1\" = -l ] || exec "+Quote(realBash)+" \"$@\"\nprintf '%s|%s|%s' \"$*\" \"${PWD##*/}\" \"${UNIBOX_AGENT:-none}\" > "+Quote(seen)+"\n"))
	mustWriteExecutable(t, filepath.Join(bin, "claude"), []byte("#!/bin/sh\necho claude ran >&2\nexit 9\n"))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx := context.Background()
	if _, err := m.Create(ctx, Request{Sandbox: "dev", Env: "dev"}, false); err != nil {
		t.Fatal(err)
	}
	req := Request{Sandbox: "dev", Task: "refunds", Shell: true, Author: "Person <person@example.com>", Repositories: []RepoInput{{Source: url, Branch: "feat/refunds"}}}
	if err := m.Run(ctx, req, strings.NewReader(""), io.Discard, io.Discard, nil); err != nil {
		t.Fatal(err)
	}
	// A login shell in the repository, crediting no agent.
	if got, _ := os.ReadFile(seen); string(got) != "-l|payments|none" {
		t.Fatalf("the shell saw %q", got)
	}
	// The task still names the agent a later resume runs.
	b, err := m.sandbox("dev")
	if err != nil || len(b.Tasks) != 1 || b.Tasks[0].Agent != "claude" {
		t.Fatalf("%+v %v", b.Tasks, err)
	}
	if err := m.Run(ctx, Request{Task: "refunds", Shell: true}, strings.NewReader(""), io.Discard, io.Discard, []string{"--x"}); err == nil {
		t.Fatal("a shell took agent arguments")
	}
	// While a session works in the task, a shell joins it; another agent
	// does not.
	busy, err := LockFile(filepath.Join(m.Paths.Config, "locks", "dev-task-refunds.lock"), false, false)
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	os.Remove(seen)
	if err := m.Run(ctx, Request{Task: "refunds", Shell: true}, strings.NewReader(""), io.Discard, io.Discard, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(seen); string(got) != "-l|payments|none" {
		t.Fatalf("the joining shell saw %q", got)
	}
	if err := m.Run(ctx, Request{Task: "refunds"}, strings.NewReader(""), io.Discard, io.Discard, nil); err == nil || !strings.Contains(err.Error(), "join it with unibox resume refunds --shell") {
		t.Fatalf("%v", err)
	}
}

func TestARepositoryJoinsATaskWhileItsAgentRuns(t *testing.T) {
	t.Parallel()
	payments, _ := remoteFixture(t)
	ledger, _ := remoteFixture(t)
	m, f := managerFixture(t)
	paymentsURL := "https://forge.invalid/team/payments.git"
	ledgerURL := "https://forge.invalid/team/ledger.git"
	f.remotes[paymentsURL] = "file://" + payments
	f.remotes[ledgerURL] = "file://" + ledger
	ctx := context.Background()
	if _, err := m.Create(ctx, Request{Sandbox: "dev", Env: "dev"}, false); err != nil {
		t.Fatal(err)
	}
	author := "Person <person@example.com>"
	first := Request{Sandbox: "dev", Task: "refunds", Author: author, Repositories: []RepoInput{{Source: paymentsURL, Branch: "feat/refunds"}}}
	if _, err := m.Prepare(ctx, first); err != nil {
		t.Fatal(err)
	}
	// A running session holds the task; adding must not wait for it.
	_, _, usage, taskLock, err := m.prepare(ctx, Request{Sandbox: "dev", Task: "refunds"}, true)
	if err != nil {
		t.Fatal(err)
	}
	add := Request{Task: "refunds", Author: author, Repositories: []RepoInput{{Source: ledgerURL, Branch: "feat/refunds"}}}
	task, added, err := m.AddRepositories(ctx, add)
	usage.Close()
	taskLock.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 1 || added[0].Name != "ledger" || len(task.Repositories) != 2 {
		t.Fatalf("%+v %+v", added, task)
	}
	clone := filepath.Join(f.root, "dev", "work", "refunds", "ledger")
	if got := gitTest(t, clone, "branch", "--show-current"); got != "feat/refunds" {
		t.Fatal(got)
	}
	b, _ := m.sandbox("dev")
	if len(b.Tasks) != 1 || len(b.Tasks[0].Repositories) != 2 {
		t.Fatalf("the task record did not grow: %+v", b.Tasks)
	}
	// Adding the same again is nothing to do; another source under a held
	// name is refused rather than mixed into the clone.
	if _, added, err = m.AddRepositories(ctx, add); err != nil || len(added) != 0 {
		t.Fatalf("%+v %v", added, err)
	}
	clash := Request{Task: "refunds", Author: author, Repositories: []RepoInput{{Name: "ledger", Source: paymentsURL, Branch: "feat/refunds"}}}
	if _, _, err = m.AddRepositories(ctx, clash); err == nil || !strings.Contains(err.Error(), "already has a repository named ledger") {
		t.Fatalf("%v", err)
	}
	// The task its first repository names is still found by them.
	if _, err = m.Prepare(ctx, first); err != nil {
		t.Fatalf("the grown task is no longer found by its first repository: %v", err)
	}
	// An added clone is checked for unpushed work like the first.
	write(t, filepath.Join(clone, "agent.txt"), "work\n")
	gitTest(t, clone, "add", "agent.txt")
	gitTest(t, clone, "commit", "-m", "Agent work")
	if err = m.Remove(ctx, "dev", "", false); err == nil {
		t.Fatal("removed a sandbox with an unpushed commit in an added repository")
	}
	if _, _, err = m.AddRepositories(ctx, Request{Task: "missing", Repositories: add.Repositories}); err == nil {
		t.Fatal("added to a task that does not exist")
	}
}

func TestATaskAliasIsTakenWhereverItsIDIs(t *testing.T) {
	t.Parallel()
	remote, _ := remoteFixture(t)
	m, f := managerFixture(t)
	url := "https://forge.invalid/team/payments.git"
	f.remotes[url] = "file://" + remote
	ctx := context.Background()
	for _, name := range []string{"dev", "ci"} {
		if _, err := m.Create(ctx, Request{Sandbox: name, Env: "dev"}, false); err != nil {
			t.Fatal(err)
		}
	}
	req := func(sandbox, task string) Request {
		return Request{Sandbox: sandbox, Task: task, Author: "Person <person@example.com>", Repositories: []RepoInput{{Source: url, Branch: "feat/refunds"}}}
	}
	for sandbox, task := range map[string]string{"dev": "refunds", "ci": "audit"} {
		if _, err := m.Prepare(ctx, req(sandbox, task)); err != nil {
			t.Fatal(err)
		}
	}
	task, err := m.AliasTask("refunds", "billing")
	if err != nil || task.ID != "refunds" || task.Alias != "billing" {
		t.Fatalf("%+v %v", task, err)
	}
	if b, _, err := m.Definition(Request{Task: "billing"}); err != nil || b.Name != "dev" {
		t.Fatalf("%s %v", b.Name, err)
	}
	if p, err := m.Prepare(ctx, Request{Task: "billing"}); err != nil || p.Task != "refunds" {
		t.Fatalf("%+v %v", p, err)
	}
	for _, name := range []string{"audit", "billing"} {
		if _, err := m.AliasTask("audit", name); name == "billing" && (err == nil || !strings.Contains(err.Error(), "names task refunds already")) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := m.AliasTask("refunds", "audit"); err == nil || !strings.Contains(err.Error(), "names task audit already") {
		t.Fatalf("%v", err)
	}
	if _, err := m.AliasTask("billing", "Not A Name"); err == nil {
		t.Fatal("an invalid name was taken")
	}
	// Its own ID takes the alias away.
	if task, err := m.AliasTask("billing", "refunds"); err != nil || task.Alias != "" {
		t.Fatalf("%+v %v", task, err)
	}
	if _, err := m.AliasTask("refunds", "billing"); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove(ctx, "dev", "billing", false); err != nil {
		t.Fatal(err)
	}
	if b, _ := m.sandbox("dev"); len(b.Tasks) != 0 {
		t.Fatalf("%+v", b.Tasks)
	}
}

func TestATaskWorksOnTheForgesDefaultBranchAndRunsTheAgentChosen(t *testing.T) {
	remote, _ := remoteFixture(t)
	m, f := managerFixture(t)
	url := "https://forge.invalid/team/payments.git"
	f.remotes[url] = "file://" + remote
	// The host reaches the forge as well, to read its default branch.
	global := filepath.Join(t.TempDir(), "gitconfig")
	write(t, global, "[include]\n\tpath = "+os.Getenv("GIT_CONFIG_GLOBAL")+"\n[url \"file://"+remote+"\"]\n\tinsteadOf = "+url+"\n")
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	bin := t.TempDir()
	seen := filepath.Join(t.TempDir(), "agent")
	for _, a := range []string{"claude", "codex"} {
		mustWriteExecutable(t, filepath.Join(bin, a), []byte("#!/bin/sh\nprintf '%s' \"$(basename \"$0\") $*|$UNIBOX_AGENT\" > "+Quote(seen)+"\n"))
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx := context.Background()
	if _, err := m.Create(ctx, Request{Sandbox: "dev", Env: "dev"}, false); err != nil {
		t.Fatal(err)
	}
	ran := func(req Request) string {
		t.Helper()
		os.Remove(seen)
		if err := m.Run(ctx, req, strings.NewReader(""), io.Discard, io.Discard, nil); err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(seen)
		return string(got)
	}
	// No branch: the forge's default one. The env's agent runs.
	if got := ran(Request{Sandbox: "dev", Task: "refunds", Author: "Person <person@example.com>", Repositories: []RepoInput{{Source: url}}}); got != "claude --dangerously-skip-permissions|Claude" {
		t.Fatalf("ran %q", got)
	}
	b, err := m.sandbox("dev")
	if err != nil || b.Tasks[0].Repositories[0].Branch != "main" {
		t.Fatalf("%+v %v", b.Tasks, err)
	}
	// Another agent the sandbox has, as sbx's kit for it starts it; the
	// task remembers it for the next resume.
	if got := ran(Request{Task: "refunds", Agent: "codex"}); got != "codex --dangerously-bypass-approvals-and-sandbox|Codex" {
		t.Fatalf("ran %q", got)
	}
	if got := ran(Request{Task: "refunds"}); !strings.HasPrefix(got, "codex ") {
		t.Fatalf("resumed %q", got)
	}
	// One it does not have is named, with the env's.
	err = m.Run(ctx, Request{Task: "refunds", Agent: "opencode"}, strings.NewReader(""), io.Discard, io.Discard, nil)
	if err == nil || !strings.Contains(err.Error(), "sandbox dev has no opencode; its env runs claude") {
		t.Fatalf("%v", err)
	}
}

func TestACloneFollowsTheForgesRedirectOnceAndQuietly(t *testing.T) {
	remote, _ := remoteFixture(t)
	moved := remote + "-moved.git"
	if err := os.Symlink(remote, moved); err != nil {
		t.Fatal(err)
	}
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	// git warns of a redirect on each request to the URL as given, as it
	// does for GitLab's without .git.
	bin := t.TempDir()
	mustWriteExecutable(t, filepath.Join(bin, "git"), []byte("#!/bin/sh\ncase \" $* \" in *' file://"+remote+" '*) echo 'warning: redirecting to file://"+moved+"/' >&2 ;; esac\nexec "+Quote(real)+" \"$@\"\n"))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	work := t.TempDir()
	task := api.Task{ID: "t", Directory: work, Repositories: []api.Repository{{Name: "payments", URL: "file://" + remote, Branch: "main"}}}
	for range 2 {
		var errout bytes.Buffer
		c := exec.Command("sh", "-c", prepareScript(task))
		c.Stderr = &errout
		if err := c.Run(); err != nil {
			t.Fatalf("%v: %s", err, errout.String())
		}
		if strings.Contains(errout.String(), "redirecting") {
			t.Fatalf("warned: %s", errout.String())
		}
	}
	if got := gitTest(t, filepath.Join(work, "payments"), "remote", "get-url", "origin"); got != "file://"+moved {
		t.Fatalf("origin %q", got)
	}
}

func TestABranchTheForgeLacksIsMadeInTheSandboxFromItsBaseAndNotPushed(t *testing.T) {
	t.Parallel()
	remote, _ := remoteFixture(t)
	m, f := managerFixture(t)
	url := "https://forge.invalid/team/payments.git"
	f.remotes[url] = "file://" + remote
	ctx := context.Background()
	if _, err := m.Create(ctx, Request{Sandbox: "dev", Env: "dev"}, false); err != nil {
		t.Fatal(err)
	}
	req := Request{Sandbox: "dev", Task: "local", Author: "Person <person@example.com>", Repositories: []RepoInput{{Source: url, Name: "payments", Branch: "feat/mine", Base: "main", Clone: true, Fork: true}}}
	if _, err := m.Prepare(ctx, req); err != nil {
		t.Fatal(err)
	}
	clone := filepath.Join(f.root, "dev", "work", "local", "payments")
	if got := gitTest(t, clone, "branch", "--show-current"); got != "feat/mine" {
		t.Fatal(got)
	}
	if out := gitTest(t, remote, "branch", "--list", "feat/mine"); out != "" {
		t.Fatalf("the branch reached the forge: %s", out)
	}
	// Prepared again, the task keeps its own branch - asked for with no
	// base too, as where a branch started is no part of a task.
	if _, err := m.Prepare(ctx, req); err != nil {
		t.Fatal(err)
	}
	again := req
	again.Repositories = []RepoInput{{Source: url, Name: "payments", Branch: "feat/mine", Clone: true}}
	if _, err := m.Prepare(ctx, again); err != nil {
		t.Fatal(err)
	}
	// Without a base, a branch the forge lacks is refused, naming it.
	req.Task, req.Repositories[0].Base, req.Repositories[0].Fork = "none", "", false
	if _, err := m.Prepare(ctx, req); err == nil {
		t.Fatal("a branch the forge lacks was taken without a base")
	}
}
