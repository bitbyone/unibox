package unibox

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestAMountedDirectoryIsWorkedOnInPlaceAndNeverDeletedWithItsTask(t *testing.T) {
	t.Parallel()
	_, host := remoteFixture(t)
	host, _ = filepath.EvalSymlinks(host)
	m, f := managerFixture(t)
	live := f
	ctx := context.Background()
	if _, err := m.Create(ctx, Request{Sandbox: "dev", Env: "dev"}, false); err != nil {
		t.Fatal(err)
	}
	config, _ := os.ReadFile(filepath.Join(host, ".git", "config"))
	// unibox does not push a mounted directory.
	if _, err := m.Prepare(ctx, Request{Sandbox: "dev", Task: "x", Mount: true, Push: true, Repositories: []RepoInput{{Source: host}}}); err == nil || !strings.Contains(err.Error(), "leave out --push") {
		t.Fatalf("%v", err)
	}
	if _, err := m.Prepare(ctx, Request{Sandbox: "dev", Task: "local", Author: "Person <person@example.com>", Mount: true, Repositories: []RepoInput{{Source: host}}}); err != nil {
		t.Fatal(err)
	}
	target := "/work/local/" + filepath.Base(host)
	if calls := live.called("mount "); len(calls) != 1 || calls[0] != "mount "+host+":"+target {
		t.Fatalf("%v", calls)
	}
	// What a mount lets the agent do is told once for a directory.
	if log := m.Log.(*bytes.Buffer).String(); strings.Count(log, "note: the agent can write to "+host) != 1 {
		t.Fatalf("%s", log)
	}
	// The host's .git/config is left as it was; git in the guest gets the
	// task's author and hook by an include of its own configuration.
	if now, _ := os.ReadFile(filepath.Join(host, ".git", "config")); string(now) != string(config) {
		t.Fatalf("the host's git configuration changed:\n%s", now)
	}
	box := filepath.Join(f.root, "dev")
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
	if err := m.Remove(ctx, "dev", "local", false); err == nil || !strings.Contains(err.Error(), "nothing of the host is deleted") {
		t.Fatalf("%v", err)
	}
	live.failUnmount, live.keepMounted = false, true
	if err := m.Remove(ctx, "dev", "local", false); err == nil {
		t.Fatal("a task was removed with its directory still mounted")
	}
	hostKept()
	// Unmounted, the task goes, and only it.
	live.keepMounted = false
	if err := m.Remove(ctx, "dev", "local", false); err != nil {
		t.Fatal(err)
	}
	hostKept()
	if _, err := os.Lstat(filepath.Join(box, "work", "local")); err == nil {
		t.Fatal("the task's directory stayed")
	}
	if global, _ = os.ReadFile(filepath.Join(box, "home", ".gitconfig")); strings.Contains(string(global), "local") {
		t.Fatalf("%s", global)
	}
	if _, err := m.Prepare(ctx, Request{Sandbox: "dev", Task: "again", Mount: true, Repositories: []RepoInput{{Source: host}}}); err != nil {
		t.Fatal(err)
	}
	if log := m.Log.(*bytes.Buffer).String(); strings.Count(log, "note: the agent can write to") != 1 {
		t.Fatalf("told again:\n%s", log)
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

func TestATaskMountsItsDirectoriesAndClonesBesideThemWhatIsAlwaysCloned(t *testing.T) {
	t.Parallel()
	remote, host := remoteFixture(t)
	host, _ = filepath.EvalSymlinks(host)
	m, f := managerFixture(t)
	url := "https://forge.invalid/arch/adrs.git"
	f.remotes[url] = "file://" + remote
	ctx := context.Background()
	if _, err := m.Create(ctx, Request{Sandbox: "dev", Env: "dev"}, false); err != nil {
		t.Fatal(err)
	}
	req := Request{Sandbox: "dev", Task: "mixed", Author: "Person <person@example.com>", Repositories: []RepoInput{{Source: host}, {Source: url, Name: "adrs", Branch: "main", Clone: true}}}
	req.decide()
	if !req.Mount {
		t.Fatal("the directory was not to be mounted")
	}
	if _, err := m.Prepare(ctx, req); err != nil {
		t.Fatal(err)
	}
	if calls := f.called("mount "); len(calls) != 1 || calls[0] != "mount "+host+":/work/mixed/"+filepath.Base(host) {
		t.Fatalf("%v", calls)
	}
	if got := gitTest(t, filepath.Join(f.root, "dev", "work", "mixed", "adrs"), "branch", "--show-current"); got != "main" {
		t.Fatal(got)
	}
	// A task made before a repository every task takes is found by the
	// repositories asked for, and keeps what it holds.
	before, err := m.Prepare(ctx, Request{Sandbox: "dev", Author: "Person <person@example.com>", Mount: true, Repositories: []RepoInput{{Source: host}}})
	if err != nil {
		t.Fatal(err)
	}
	again := Request{Sandbox: "dev", Author: "Person <person@example.com>", Repositories: []RepoInput{{Source: host}, {Source: url, Name: "adrs", Branch: "main", Clone: true, Always: true}}}
	again.decide()
	after, err := m.Prepare(ctx, again)
	if err != nil || after.Task != before.Task {
		t.Fatalf("%v %s %s", err, before.Task, after.Task)
	}
	if _, err := os.Stat(filepath.Join(f.root, "dev", "work", before.Task, "adrs")); err == nil {
		t.Fatal("a task made before was given the repository every task takes")
	}
	// Two of the same name, one mounted and one cloned, are refused.
	req.Task, req.Repositories[1].Name = "clash", filepath.Base(host)
	if _, err := m.Prepare(ctx, req); err == nil || !strings.Contains(err.Error(), "duplicate repository directory") {
		t.Fatalf("%v", err)
	}
}

func TestATaskMountsDirectoriesAndClonesTheRestUnlessTold(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, c := range []struct {
		req   Request
		mount bool
	}{
		{Request{Repositories: []RepoInput{{Source: dir}}}, true},
		{Request{Repositories: []RepoInput{{Source: dir}}, Clone: true}, false},
		{Request{Repositories: []RepoInput{{Source: dir}, {Source: "https://forge.invalid/x.git"}}}, false},
		// One every task takes is cloned beside the directory, which is
		// mounted still.
		{Request{Repositories: []RepoInput{{Source: dir}, {Source: "https://forge.invalid/x.git", Clone: true}}}, true},
		{Request{Repositories: []RepoInput{{Source: "https://forge.invalid/x.git", Clone: true}}}, false},
		{Request{Repositories: []RepoInput{{Source: filepath.Join(dir, "absent")}}}, false},
		{Request{}, false},
	} {
		req := c.req
		req.decide()
		if req.Mount != c.mount {
			t.Errorf("%+v: mount %v", c.req, req.Mount)
		}
	}
}

func TestMountsTheSandboxLacksOrNoTaskHasAreFoundAndRecreateMountsAgain(t *testing.T) {
	t.Parallel()
	_, host := remoteFixture(t)
	host, _ = filepath.EvalSymlinks(host)
	m, f := managerFixture(t)
	ctx := context.Background()
	if _, err := m.Create(ctx, Request{Sandbox: "dev", Env: "dev"}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Prepare(ctx, Request{Sandbox: "dev", Task: "local", Author: "Person <person@example.com>", Mount: true, Repositories: []RepoInput{{Source: host}}}); err != nil {
		t.Fatal(err)
	}
	target := "/work/local/" + filepath.Base(host)
	drift := func() (missing, stray [][2]string) {
		t.Helper()
		b, err := m.sandbox("dev")
		if err != nil {
			t.Fatal(err)
		}
		missing, stray, err = m.MountDrift(ctx, b)
		if err != nil {
			t.Fatal(err)
		}
		return missing, stray
	}
	if missing, stray := drift(); len(missing) != 0 || len(stray) != 0 {
		t.Fatalf("%v %v", missing, stray)
	}
	odd := filepath.Join(t.TempDir(), "with space")
	if err := f.Mount(ctx, "dev", odd, "/work/gone/x"); err != nil {
		t.Fatal(err)
	}
	if err := f.Unmount(ctx, "dev", host, target); err != nil {
		t.Fatal(err)
	}
	missing, stray := drift()
	if len(missing) != 1 || missing[0] != [2]string{host, target} {
		t.Fatalf("%v", missing)
	}
	if len(stray) != 1 || stray[0] != [2]string{odd, "/work/gone/x"} {
		t.Fatalf("%v", stray)
	}
	// Resuming the ready task mounts what it lost, and nothing else.
	_, _, usage, taskLock, err := m.prepare(ctx, Request{Sandbox: "dev", Task: "local"}, true)
	if err != nil {
		t.Fatal(err)
	}
	usage.Close()
	taskLock.Close()
	if missing, stray := drift(); len(missing) != 0 || len(stray) != 1 {
		t.Fatalf("%v %v", missing, stray)
	}
	// Prune unmounts what no task has, but not while the sandbox is busy.
	guard, err := m.guard("dev")
	if err != nil {
		t.Fatal(err)
	}
	if unmounted, busy, err := m.PruneMounts(ctx); err != nil || len(unmounted) != 0 || !slices.Equal(busy, []string{"dev"}) {
		t.Fatalf("%v %v %v", unmounted, busy, err)
	}
	guard.Close()
	if unmounted, busy, err := m.PruneMounts(ctx); err != nil || len(busy) != 0 || !slices.Equal(unmounted, []string{"dev: " + odd + " at /work/gone/x"}) {
		t.Fatalf("%v %v %v", unmounted, busy, err)
	}
	if missing, stray := drift(); len(missing) != 0 || len(stray) != 0 {
		t.Fatalf("%v %v", missing, stray)
	}
	// A recreated sandbox has no mounts; its tasks wait to be prepared
	// again, which mounts them.
	if err := m.Recreate(ctx, "dev"); err != nil {
		t.Fatal(err)
	}
	if missing, stray := drift(); len(missing) != 0 || len(stray) != 0 {
		t.Fatalf("%v %v", missing, stray)
	}
	_, _, usage, taskLock, err = m.prepare(ctx, Request{Sandbox: "dev", Task: "local"}, true)
	if err != nil {
		t.Fatal(err)
	}
	usage.Close()
	taskLock.Close()
	if calls := f.called("mount "); len(calls) != 4 || calls[3] != "mount "+host+":"+target {
		t.Fatalf("%v", calls)
	}
	if missing, stray := drift(); len(missing) != 0 || len(stray) != 0 {
		t.Fatalf("%v %v", missing, stray)
	}
}

func TestAMountedCheckoutAskedToPushHasItsNewBranchOnTheForge(t *testing.T) {
	t.Parallel()
	remote, host := remoteFixture(t)
	m, _ := managerFixture(t)
	ctx := context.Background()
	if _, err := m.Create(ctx, Request{Sandbox: "dev", Env: "dev"}, false); err != nil {
		t.Fatal(err)
	}
	req := Request{Sandbox: "dev", Task: "pushed", Author: "Person <person@example.com>", Repositories: []RepoInput{{Source: host, Name: "web", Branch: "feat/web", Push: true}}}
	req.decide()
	if _, err := m.Prepare(ctx, req); err != nil {
		t.Fatal(err)
	}
	if got := gitTest(t, host, "branch", "--show-current"); got != "feat/web" {
		t.Fatal(got)
	}
	if out := gitTest(t, remote, "branch", "--list", "feat/web"); !strings.Contains(out, "feat/web") {
		t.Fatal("the branch did not reach the forge")
	}
}

func TestAMountedContextCheckoutKeepsItsBranchAndOneWithoutAForgeIsNotPushed(t *testing.T) {
	t.Parallel()
	_, host := remoteFixture(t)
	m, _ := managerFixture(t)
	ctx := context.Background()
	if _, err := m.Create(ctx, Request{Sandbox: "dev", Env: "dev"}, false); err != nil {
		t.Fatal(err)
	}
	req := Request{Sandbox: "dev", Task: "read", Branch: "feat/x", Author: "Person <person@example.com>", Repositories: []RepoInput{{Source: host, Name: "adrs", Context: true}}}
	req.decide()
	if _, err := m.Prepare(ctx, req); err != nil {
		t.Fatal(err)
	}
	if got := gitTest(t, host, "branch", "--show-current"); got != "main" {
		t.Fatalf("a context checkout was switched to %s", got)
	}
	// A checkout with no origin is refused a push before it is switched.
	alone := t.TempDir()
	gitTest(t, alone, "init", "-q", "-b", "main")
	gitTest(t, alone, "commit", "-q", "--allow-empty", "-m", "first")
	req = Request{Sandbox: "dev", Task: "alone", Author: "Person <person@example.com>", Repositories: []RepoInput{{Source: alone, Name: "alone", Branch: "feat/y", Push: true}}}
	req.decide()
	if _, err := m.Prepare(ctx, req); err == nil || !strings.Contains(err.Error(), "no origin to push") {
		t.Fatalf("%v", err)
	}
	if got := gitTest(t, alone, "branch", "--show-current"); got != "main" {
		t.Fatalf("the checkout was switched to %s", got)
	}
}

func TestGitOnTheHostRunsNothingAMountedRepositoryConfigures(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the programs are shell scripts")
	}
	ctx := context.Background()
	dir, marks := t.TempDir(), t.TempDir()
	program := func(name string) string {
		p := filepath.Join(marks, name+".sh")
		mustWriteExecutable(t, p, []byte("#!/bin/sh\ntouch "+Quote(filepath.Join(marks, name))+"\ncat\n"))
		return p
	}
	hooks := filepath.Join(dir, ".git", "hooks")
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"commit", "-q", "--allow-empty", "-m", "first"},
		// What an agent could leave in a mounted checkout's .git.
		{"config", "core.fsmonitor", program("fsmonitor")},
		{"config", "filter.evil.clean", program("filter")},
		{"config", "credential.helper", "!" + program("helper")},
		{"config", "core.hooksPath", hooks}} {
		if b, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, b)
		}
	}
	mustWriteExecutable(t, filepath.Join(hooks, "post-checkout"), []byte("#!/bin/sh\ntouch "+Quote(filepath.Join(marks, "hook"))+"\n"))
	write(t, filepath.Join(dir, ".gitattributes"), "* filter=evil\n")
	write(t, filepath.Join(dir, "file"), "changed\n")
	// The person's own git runs them, as the checks below rely on.
	if b, err := exec.Command("git", "-C", dir, "checkout", "-q", "-b", "plain").CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, b)
	}
	if _, err := os.Stat(filepath.Join(marks, "hook")); err != nil {
		t.Fatal("the hook did not run for git itself")
	}
	for _, name := range []string{"fsmonitor", "filter", "helper", "hook"} {
		_ = os.Remove(filepath.Join(marks, name))
	}
	for _, args := range [][]string{{"status", "--porcelain"}, {"add", "file"}, {"checkout", "-q", "-b", "other"}, {"config", "--get", "remote.origin.url"}} {
		_, _ = git(ctx, dir, args...)
	}
	for _, name := range []string{"fsmonitor", "filter", "helper", "hook"} {
		if _, err := os.Stat(filepath.Join(marks, name)); err == nil {
			t.Errorf("unibox's git ran the repository's %s", name)
		}
	}
}
