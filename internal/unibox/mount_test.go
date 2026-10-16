package unibox

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/tobola/unibox/internal/unibox/api"
)

// liveMounts is the fake runtime with sbx's mounts. Its guest commands run
// on the host's files, so a mount is the host path itself.
type liveMounts struct {
	*fakeRuntime
	mu      sync.Mutex
	mounted []string
	calls   []string
}

func (l *liveMounts) Mount(_ context.Context, _, host string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, "mount "+host)
	if !slices.Contains(l.mounted, host) {
		l.mounted = append(l.mounted, host)
	}
	return nil
}

func (l *liveMounts) Unmount(_ context.Context, _, host string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, "unmount "+host)
	l.mounted = slices.DeleteFunc(l.mounted, func(p string) bool { return p == host })
	return nil
}

func TestAMountedDirectoryIsWorkedOnInPlaceAndOnlyUnmountedOnRemoval(t *testing.T) {
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
	// A branch or a push has no say over what is checked out on the host.
	if _, err := m.Prepare(ctx, Request{Sandbox: "dev", Task: "x", Agent: "claude", Mount: true, Branch: "feat/x", Repositories: []RepoInput{{Source: host}}}); err == nil || !strings.Contains(err.Error(), "leave out --branch") {
		t.Fatalf("%v", err)
	}
	prepared, err := m.Prepare(ctx, Request{Sandbox: "dev", Task: "local", Agent: "claude", Author: "Person <person@example.com>", Mount: true, Repositories: []RepoInput{{Source: host}}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(live.mounted, []string{host}) {
		t.Fatalf("%v", live.calls)
	}
	link := filepath.Join(f.root, "unibox-dev", "work", "local", filepath.Base(host))
	if target, err := os.Readlink(link); err != nil || target != host {
		t.Fatalf("%v %s (%s)", err, target, prepared.Directory)
	}
	// The host's .git/config is left as it was; git in the guest gets the
	// task's author and hook by an include of its own configuration.
	if now, _ := os.ReadFile(filepath.Join(host, ".git", "config")); string(now) != string(config) {
		t.Fatalf("the host's git configuration changed:\n%s", now)
	}
	global, _ := os.ReadFile(filepath.Join(f.root, "unibox-dev", "home", ".gitconfig"))
	if !strings.Contains(string(global), `[includeIf "gitdir:`+host+`/.git/"]`) {
		t.Fatalf("%s", global)
	}
	own, _ := os.ReadFile(filepath.Join(f.root, "unibox-dev", "state", "mounted", "local", filepath.Base(host), "gitconfig"))
	if !strings.Contains(string(own), "name = Person") || !strings.Contains(string(own), "hooksPath") {
		t.Fatalf("%s", own)
	}
	// Removing the task unlinks and unmounts; the host's files stay, with
	// no question of unpushed work, as the work is on the host.
	write(t, filepath.Join(host, "uncommitted.txt"), "the user's")
	if err := m.Remove(ctx, "dev", "local"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); err == nil {
		t.Fatal("the link stayed")
	}
	if data, err := os.ReadFile(filepath.Join(host, "uncommitted.txt")); err != nil || string(data) != "the user's" {
		t.Fatalf("the host's files were touched: %v", err)
	}
	if len(live.mounted) != 0 {
		t.Fatalf("%v", live.calls)
	}
	if global, _ = os.ReadFile(filepath.Join(f.root, "unibox-dev", "home", ".gitconfig")); strings.Contains(string(global), host) {
		t.Fatalf("%s", global)
	}
}

func TestAMountedWorktreeBringsItsGitDirectory(t *testing.T) {
	t.Parallel()
	_, host := remoteFixture(t)
	host, _ = filepath.EvalSymlinks(host)
	tree := filepath.Join(t.TempDir(), "tree")
	gitTest(t, host, "worktree", "add", "-b", "feat/tree", tree)
	tree, _ = filepath.EvalSymlinks(tree)
	repos, _, err := ResolveRepositories(context.Background(), Request{Mount: true, Repositories: []RepoInput{{Source: tree}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0].HostPath != tree || repos[0].GitDir != filepath.Join(host, ".git") || repos[0].Branch != "feat/tree" || !repos[0].Mounted {
		t.Fatalf("%+v", repos)
	}
	if got := mountedPaths(taskOf(repos)); !slices.Equal(got, []string{tree, filepath.Join(host, ".git")}) {
		t.Fatalf("%v", got)
	}
	if _, _, err := ResolveRepositories(context.Background(), Request{Mount: true, Repositories: []RepoInput{{Source: "https://forge.invalid/x.git"}}}); err == nil {
		t.Fatal("a URL was mounted")
	}
}

func taskOf(repos []api.Repository) api.Task { return api.Task{Repositories: repos} }
