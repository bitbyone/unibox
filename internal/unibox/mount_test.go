package unibox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// liveMounts is the fake runtime with sbx's mounts. Its guest runs on the
// host's files under the sandbox's root, so a mount is a link there to the
// host directory, and an unmount leaves the empty mount point sbx leaves.
type liveMounts struct {
	*fakeRuntime
	mu                       sync.Mutex
	calls                    []string
	failUnmount, keepMounted bool
}

func (l *liveMounts) guest(sandbox, target string) string {
	return filepath.Join(l.root, sandbox, "work", strings.TrimPrefix(target, "/work/"))
}

func (l *liveMounts) Mount(_ context.Context, sandbox, host, target string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, "mount "+host+":"+target)
	at := l.guest(sandbox, target)
	if err := os.MkdirAll(filepath.Dir(at), 0o755); err != nil {
		return err
	}
	_ = os.Remove(at)
	return os.Symlink(host, at)
}

func (l *liveMounts) Unmount(_ context.Context, sandbox, host, target string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, "unmount "+host+":"+target)
	if l.failUnmount {
		return errors.New("sbx is away")
	}
	if l.keepMounted {
		return nil
	}
	at := l.guest(sandbox, target)
	if err := os.Remove(at); err != nil {
		return err
	}
	return os.Mkdir(at, 0o755)
}

func TestAMountedDirectoryIsWorkedOnInPlaceAndNeverDeletedWithItsTask(t *testing.T) {
	t.Parallel()
	_, host := remoteFixture(t)
	host, _ = filepath.EvalSymlinks(host)
	m, f := managerFixture(t)
	live := &liveMounts{fakeRuntime: f}
	m.Runtimes = map[string]Runtime{"fake": live}
	ctx := context.Background()
	if _, err := m.Create(ctx, Request{NewSandbox: "dev", Profile: "node", Runtime: "fake"}); err != nil {
		t.Fatal(err)
	}
	config, _ := os.ReadFile(filepath.Join(host, ".git", "config"))
	// unibox does not push a mounted directory.
	if _, err := m.Prepare(ctx, Request{Sandbox: "dev", Task: "x", Agent: "claude", Mount: true, Push: true, Repositories: []RepoInput{{Source: host}}}); err == nil || !strings.Contains(err.Error(), "leave out --push") {
		t.Fatalf("%v", err)
	}
	if _, err := m.Prepare(ctx, Request{Sandbox: "dev", Task: "local", Agent: "claude", Author: "Person <person@example.com>", Mount: true, Repositories: []RepoInput{{Source: host}}}); err != nil {
		t.Fatal(err)
	}
	target := "/work/local/" + filepath.Base(host)
	if len(live.calls) != 1 || live.calls[0] != "mount "+host+":"+target {
		t.Fatalf("%v", live.calls)
	}
	// The host's .git/config is left as it was; git in the guest gets the
	// task's author and hook by an include of its own configuration.
	if now, _ := os.ReadFile(filepath.Join(host, ".git", "config")); string(now) != string(config) {
		t.Fatalf("the host's git configuration changed:\n%s", now)
	}
	box := filepath.Join(f.root, "unibox-dev")
	global, _ := os.ReadFile(filepath.Join(box, "home", ".gitconfig"))
	if !strings.Contains(string(global), `[includeIf "gitdir:`+filepath.Join(box, "work", "local", filepath.Base(host))+`/.git/"]`) {
		t.Fatalf("%s", global)
	}
	own, _ := os.ReadFile(filepath.Join(box, "state", "mounted", "local", filepath.Base(host), "gitconfig"))
	if !strings.Contains(string(own), "name = Person") || !strings.Contains(string(own), "hooksPath") || !strings.Contains(string(own), "worktreePruneExpire = never") {
		t.Fatalf("%s", own)
	}
	write(t, filepath.Join(host, "uncommitted.txt"), "the user's")
	hostKept := func() {
		t.Helper()
		if data, err := os.ReadFile(filepath.Join(host, "uncommitted.txt")); err != nil || string(data) != "the user's" {
			t.Fatalf("the host's files were touched: %v", err)
		}
	}
	// An unmount that fails, or one that leaves the directory mounted,
	// keeps the task: deleting it would go into the host's files.
	live.failUnmount = true
	if err := m.Remove(ctx, "dev", "local"); err == nil || !strings.Contains(err.Error(), "nothing of the host is deleted") {
		t.Fatalf("%v", err)
	}
	live.failUnmount, live.keepMounted = false, true
	if err := m.Remove(ctx, "dev", "local"); err == nil {
		t.Fatal("a task was removed with its directory still mounted")
	}
	hostKept()
	// Unmounted, the task goes, and only it.
	live.keepMounted = false
	if err := m.Remove(ctx, "dev", "local"); err != nil {
		t.Fatal(err)
	}
	hostKept()
	if _, err := os.Lstat(filepath.Join(box, "work", "local")); err == nil {
		t.Fatal("the task's directory stayed")
	}
	if global, _ = os.ReadFile(filepath.Join(box, "home", ".gitconfig")); strings.Contains(string(global), "local") {
		t.Fatalf("%s", global)
	}
}

func TestAWorktreeIsNotMounted(t *testing.T) {
	t.Parallel()
	_, host := remoteFixture(t)
	tree := filepath.Join(t.TempDir(), "tree")
	gitTest(t, host, "worktree", "add", "-b", "feat/tree", tree)
	if _, _, err := ResolveRepositories(context.Background(), Request{Mount: true, Repositories: []RepoInput{{Source: tree}}}); err == nil || !strings.Contains(err.Error(), "is a git worktree, which cannot be mounted") {
		t.Fatalf("%v", err)
	}
	if _, _, err := ResolveRepositories(context.Background(), Request{Mount: true, Repositories: []RepoInput{{Source: "https://forge.invalid/x.git"}}}); err == nil {
		t.Fatal("a URL was mounted")
	}
}

func TestAMountedDirectoryIsSwitchedToTheBranchAskedForAndMadeLocally(t *testing.T) {
	t.Parallel()
	_, host := remoteFixture(t)
	ctx := context.Background()
	req := Request{Mount: true, Branch: "feat/local", Repositories: []RepoInput{{Source: host}}}
	// check says what preparing will do, and does not do it.
	repos, needs, err := ResolveRepositories(ctx, req)
	if err != nil || len(repos) != 1 || repos[0].Branch != "feat/local" || len(needs) != 1 || needs[0].Code != "branch_switch" || !strings.Contains(needs[0].Message, "made there from HEAD") || !needs[0].Warning {
		t.Fatalf("%v %+v %+v", err, repos, needs)
	}
	if got := gitTest(t, host, "branch", "--show-current"); got != "main" {
		t.Fatalf("check switched the host to %s", got)
	}
	// Preparing switches it, making the branch there; the forge has none.
	if _, err := resolveForTask(ctx, req); err != nil {
		t.Fatal(err)
	}
	if got := gitTest(t, host, "branch", "--show-current"); got != "feat/local" {
		t.Fatal(got)
	}
	// Back to an existing branch, a change in the way is refused by git.
	write(t, filepath.Join(host, "code.txt"), "edited on feat/local\n")
	gitTest(t, host, "commit", "-qam", "edit")
	write(t, filepath.Join(host, "code.txt"), "uncommitted\n")
	if _, err := resolveForTask(ctx, Request{Mount: true, Branch: "main", Repositories: []RepoInput{{Source: host}}}); err == nil || !strings.Contains(err.Error(), "commit or stash what is in the way") {
		t.Fatalf("%v", err)
	}
	if got := gitTest(t, host, "branch", "--show-current"); got != "feat/local" {
		t.Fatal(got)
	}
}
