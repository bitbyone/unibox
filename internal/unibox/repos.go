package unibox

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/tobola/unibox/internal/unibox/api"
)

// RepoInput is a repository a task is asked to hold. Clone has it cloned
// from the forge even in a task whose directories are mounted. Always is
// one the env has every new task take, not one the person asked for, which
// leaves the task's name be. Context is one the task reads: a shared
// --branch and --push leave it on its own branch. Push makes this one's
// branch on the forge when it has none, or pushes a mounted checkout's;
// Fork, a clone's branch the forge lacks is made in the sandbox alone, from
// Base or else the forge's default branch. These are told apart by Name,
// which an input that says any of them gives.
type RepoInput struct {
	Source, Branch, Name, Base         string
	Clone, Always, Context, Push, Fork bool
}

// names are the names of a request's repositories that say so.
func (req Request) names(say func(RepoInput) bool) map[string]bool {
	out := map[string]bool{}
	for _, in := range req.Repositories {
		if say(in) {
			out[in.Name] = true
		}
	}
	return out
}

// contextNames are the repositories a task reads, which no branch of its
// is pushed to.
func (req Request) contextNames() map[string]bool {
	return req.names(func(in RepoInput) bool { return in.Context })
}

// own leaves out of repos those the env has every new task take.
func (req Request) own(repos []api.Repository) []api.Repository {
	always := req.names(func(in RepoInput) bool { return in.Always })
	out := []api.Repository{}
	for _, r := range repos {
		if !always[r.Name] {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		return repos
	}
	return out
}

// SourceURL is the forge address a source stands for: an address as it
// is, a checkout of the host by its origin; "" when it has none.
func SourceURL(ctx context.Context, source string) string {
	if p, err := ExpandPath(source); err == nil && isDir(p) {
		raw, err := git(ctx, p, "config", "--get", "remote.origin.url")
		if err != nil {
			return ""
		}
		source = raw
	}
	u, err := CloneURL(source)
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(u, ".git")
}

type Request struct {
	// Sandbox names the sandbox to work in, Env the env it is made from.
	Sandbox, Env, Task, Branch, Author string
	Repositories                       []RepoInput
	Push, IgnoreLocal, Yes             bool
	// Shell opens a shell in the task's directory instead of its agent.
	Shell bool
	// Agent is the agent to run, rather than the task's or the env's.
	Agent string
	// Mount mounts the selected directories of the host into the sandbox,
	// Clone clones them in it instead; with neither, a task mounts when
	// every source is a directory of the host, and clones otherwise.
	Mount, Clone bool
}

// decide settles whether a request's repositories are mounted or cloned;
// one that is always cloned has no say in it.
func (req *Request) decide() {
	if req.Mount || req.Clone || !slices.ContainsFunc(req.Repositories, func(in RepoInput) bool { return !in.Clone }) {
		return
	}
	for _, in := range req.Repositories {
		if in.Clone {
			continue
		}
		p, err := ExpandPath(in.Source)
		if err != nil {
			return
		}
		if info, err := os.Stat(p); err != nil || !info.IsDir() {
			return
		}
	}
	req.Mount = true
}

func git(ctx context.Context, dir string, args ...string) (string, error) {
	c := hostCommand(ctx, "git", append([]string{"-C", dir}, args...)...)
	// git on the host acts as the person, with their SSH agent, but never
	// asks on the terminal: a password prompt there would take the keys of
	// whatever unibox draws, and outlive it.
	c.Env = append(c.Env, "GIT_TERMINAL_PROMPT=0")
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		c.Env = append(c.Env, "SSH_AUTH_SOCK="+sock)
	}
	if os.Getenv("GIT_SSH_COMMAND") == "" {
		c.Env = append(c.Env, "GIT_SSH_COMMAND=ssh -o BatchMode=yes")
	}
	b, err := commandOutput(c)
	return strings.TrimSpace(string(b)), err
}

func CloneURL(raw string) (string, error) {
	if !strings.Contains(raw, "://") {
		at := strings.LastIndex(raw, "@")
		colon := strings.Index(raw, ":")
		if colon < 0 {
			return "", fmt.Errorf("origin is not a forge URL; use --repo https://host/group/repository.git")
		}
		host := raw[:colon]
		if at >= 0 && at < colon {
			host = raw[at+1 : colon]
		}
		raw = "https://" + host + "/" + raw[colon+1:]
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "", fmt.Errorf("invalid clone URL; use an https forge URL")
	}
	if u.Scheme == "ssh" {
		if u.Port() != "" {
			return "", fmt.Errorf("cannot infer HTTPS from an SSH URL with a port; supply the HTTPS clone URL")
		}
		u.Scheme = "https"
	}
	if u.Scheme != "https" {
		return "", fmt.Errorf("clone URL must use https; supply the forge's HTTPS clone URL")
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	if u.Path == "" || u.Path == "/" || strings.ContainsAny(u.Path, "\n\r") {
		return "", fmt.Errorf("clone URL needs a repository path")
	}
	return u.String(), nil
}

var authorPattern = regexp.MustCompile(`^([^<>\r\n]+) <([^<>\s]+@[^<>\s]+)>$`)

func AuthorParts(author string) (string, string, error) {
	m := authorPattern.FindStringSubmatch(author)
	if m == nil {
		return "", "", fmt.Errorf("missing or invalid commit author; configure git user.name and user.email or pass --author 'Name <email>'")
	}
	return m[1], m[2], nil
}

func ResolveRepositories(ctx context.Context, req Request) ([]api.Repository, []api.Need, error) {
	if req.Mount {
		// The directories are mounted, and what is always cloned is cloned
		// beside them.
		mounted, cloned := req, req
		mounted.Repositories, cloned.Repositories = nil, nil
		for _, in := range req.Repositories {
			if in.Clone {
				cloned.Repositories = append(cloned.Repositories, in)
			} else {
				mounted.Repositories = append(mounted.Repositories, in)
			}
		}
		repos, needs, err := resolveMounted(ctx, mounted)
		if err != nil || len(cloned.Repositories) == 0 {
			return repos, needs, err
		}
		// A shared --branch reaches what is cloned beside the directories,
		// as it reaches them; Context keeps it from those only read.
		cloned.Mount, cloned.Push = false, false
		more, moreNeeds, err := ResolveRepositories(ctx, cloned)
		if err != nil {
			return nil, nil, err
		}
		for _, r := range more {
			if slices.ContainsFunc(repos, func(o api.Repository) bool { return o.Name == r.Name }) {
				return nil, nil, fmt.Errorf("duplicate repository directory %s; use --repo name=source", r.Name)
			}
		}
		return append(repos, more...), append(needs, moreNeeds...), nil
	}
	repos := []api.Repository{}
	needs := []api.Need{}
	names := map[string]bool{}
	for _, in := range req.Repositories {
		r := api.Repository{Name: in.Name, Branch: in.Branch, Author: req.Author}
		if r.Branch == "" && !in.Context {
			r.Branch = req.Branch
		}
		r.Base, r.Context = in.Base, in.Context
		_, statErr := os.Stat(in.Source)
		local := statErr == nil
		if local {
			path, err := ExpandPath(in.Source)
			if err != nil {
				return nil, nil, err
			}
			path, err = filepath.EvalSymlinks(path)
			if err != nil {
				return nil, nil, err
			}
			r.HostPath, err = git(ctx, path, "rev-parse", "--show-toplevel")
			if err != nil {
				return nil, nil, err
			}
			raw, err := git(ctx, r.HostPath, "config", "--get", "remote.origin.url")
			if err != nil {
				return nil, nil, fmt.Errorf("repository %s has no origin; set it or use --repo with a URL", r.HostPath)
			}
			r.URL, err = CloneURL(raw)
			if err != nil {
				return nil, nil, err
			}
			current, _ := git(ctx, r.HostPath, "symbolic-ref", "--quiet", "--short", "HEAD")
			if r.Branch == "" {
				r.Branch = current
			}
			if r.Branch == "" {
				return nil, nil, fmt.Errorf("repository %s has detached HEAD; pass --branch", r.HostPath)
			}
			if r.Author == "" {
				name, _ := git(ctx, r.HostPath, "config", "user.name")
				email, _ := git(ctx, r.HostPath, "config", "user.email")
				r.Author = name + " <" + email + ">"
			}
			if current == r.Branch {
				dirty, err := git(ctx, r.HostPath, "status", "--porcelain")
				if err != nil {
					return nil, nil, err
				}
				if dirty != "" {
					needs = append(needs, api.Need{Code: "uncommitted", Message: "local edits will not reach the sandbox; commit and push them or use --ignore-local", Repository: r.Name, Warning: req.IgnoreLocal})
				}
			}
		} else {
			var err error
			r.URL, err = CloneURL(in.Source)
			if err != nil {
				return nil, nil, err
			}
			// Without a branch the task works where the forge's work lands,
			// its default branch, which the clone in the sandbox finds; the
			// host asks the forge nothing. A branch made in the sandbox
			// alone starts there too, unless told where.
			if in.Fork && r.Base == "" {
				r.Base = "HEAD"
			}
			if r.Author == "" {
				name, _ := git(ctx, ".", "config", "user.name")
				email, _ := git(ctx, ".", "config", "user.email")
				r.Author = name + " <" + email + ">"
			}
		}
		if r.Name == "" {
			u, _ := url.Parse(r.URL)
			r.Name = strings.TrimSuffix(filepath.Base(u.Path), ".git")
		}
		if err := ValidName(r.Name); err != nil {
			return nil, nil, fmt.Errorf("repository directory: %w; use --repo name=source", err)
		}
		if names[r.Name] {
			return nil, nil, fmt.Errorf("duplicate repository directory %s; use --repo name=source", r.Name)
		}
		names[r.Name] = true
		if r.Branch != "" {
			if _, err := git(ctx, ".", "check-ref-format", "--branch", r.Branch); err != nil {
				return nil, nil, fmt.Errorf("invalid branch %q; use a git branch name", r.Branch)
			}
		}
		if _, _, err := AuthorParts(r.Author); err != nil {
			return nil, nil, err
		}
		if local {
			// The agent may work on any branch, the default one too: the
			// forge's protection of it is what bounds a push.
			_, remoteTip, err := remoteRefs(ctx, r.HostPath, r.URL, r.Branch)
			if err != nil {
				return nil, nil, fmt.Errorf("cannot check remote branch for %s; check forge access: %w", r.Name, err)
			}
			localTip, localErr := git(ctx, r.HostPath, "rev-parse", "--verify", "refs/heads/"+r.Branch)
			if remoteTip == "" {
				if localErr != nil {
					needs = append(needs, api.Need{Code: "branch_missing", Message: "branch exists neither locally nor on the forge; pass --push to create it from the current HEAD", Repository: r.Name})
				} else {
					needs = append(needs, api.Need{Code: "branch_not_pushed", Message: "push the selected branch first (or pass --push)", Repository: r.Name})
				}
			} else if localErr == nil && localTip != remoteTip {
				if _, err = git(ctx, r.HostPath, "merge-base", "--is-ancestor", localTip, remoteTip); err != nil {
					needs = append(needs, api.Need{Code: "local_commits", Message: "selected local branch differs from the forge; push it or use --ignore-local to use the forge's version", Repository: r.Name, Warning: req.IgnoreLocal})
				}
			}
		}
		repos = append(repos, r)
	}
	return repos, needs, nil
}

// remoteRefs reads the forge's default branch and the chosen branch's tip in
// one ls-remote, run from dir so a checkout's URL rewrites apply.
func remoteRefs(ctx context.Context, dir, url, branch string) (string, string, error) {
	out, err := git(ctx, dir, "ls-remote", "--symref", url, "HEAD", "refs/heads/"+branch)
	if err != nil {
		return "", "", err
	}
	defaultBranch, tip := "", ""
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == "ref:" && fields[2] == "HEAD" {
			defaultBranch = strings.TrimPrefix(fields[1], "refs/heads/")
		}
		if len(fields) == 2 && fields[1] == "refs/heads/"+branch {
			tip = fields[0]
		}
	}
	return defaultBranch, tip, nil
}

// createRemoteBranch starts the chosen branch on the forge from its default
// tip: --push with a URL repository means exactly that, since there is no
// host checkout to publish. A branch already on the forge is left alone.
func createRemoteBranch(ctx context.Context, r api.Repository) error {
	defaultBranch, tip, err := remoteRefs(ctx, ".", r.URL, r.Branch)
	if err != nil {
		return fmt.Errorf("cannot reach %s; check forge access: %w", r.Name, err)
	}
	if tip != "" {
		return nil
	}
	if defaultBranch == "" {
		return fmt.Errorf("cannot determine the default branch of %s; create branch %s on the forge yourself", r.Name, r.Branch)
	}
	tmp, err := os.MkdirTemp("", "unibox-branch-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if _, err = git(ctx, tmp, "clone", "--bare", "--depth", "1", "--branch", defaultBranch, "--", r.URL, "fork.git"); err != nil {
		return fmt.Errorf("cannot create branch %s of %s: %w", r.Branch, r.Name, err)
	}
	if _, err = git(ctx, filepath.Join(tmp, "fork.git"), "push", "origin", "HEAD:refs/heads/"+r.Branch); err != nil {
		return fmt.Errorf("cannot create branch %s of %s: %w", r.Branch, r.Name, err)
	}
	return nil
}

func PushRepositories(ctx context.Context, repos []api.Repository) error {
	return pushRepositories(ctx, repos, nil)
}

// pushRepositories pushes repos but those skip names: a repository the env
// has every task take is the team's, and no task's branch goes there.
func pushRepositories(ctx context.Context, repos []api.Repository, skip map[string]bool) error {
	for _, r := range repos {
		if skip[r.Name] {
			continue
		}
		if r.HostPath == "" {
			if err := createRemoteBranch(ctx, r); err != nil {
				return err
			}
			continue
		}
		if _, err := git(ctx, r.HostPath, "rev-parse", "--verify", "refs/heads/"+r.Branch); err != nil {
			// A branch nobody has yet starts on the forge from what the host
			// looks at; the host checkout stays as it is.
			if _, err := git(ctx, r.HostPath, "push", "origin", "HEAD:refs/heads/"+r.Branch); err != nil {
				return err
			}
			continue
		}
		if _, err := git(ctx, r.HostPath, "push", "--set-upstream", "origin", "refs/heads/"+r.Branch+":refs/heads/"+r.Branch); err != nil {
			return err
		}
	}
	return nil
}

// resolveMounted turns a request's directories into repositories mounted
// from the host. The agent works on the host's files and git directory as
// they are, so nothing is pushed, cloned or compared with a forge; the user
// asked for exactly that. A branch asked for is the one the host's checkout
// is switched to when the task is prepared - made there from HEAD when it
// is not, as nothing needs it on the forge - which a need says beforehand.
func resolveMounted(ctx context.Context, req Request) ([]api.Repository, []api.Need, error) {
	if req.Push {
		return nil, nil, fmt.Errorf("a mounted directory is not pushed by unibox; leave out --push")
	}
	repos := []api.Repository{}
	needs := []api.Need{}
	names := map[string]bool{}
	for _, in := range req.Repositories {
		branch := in.Branch
		if branch == "" && !in.Context {
			branch = req.Branch
		}
		path, err := ExpandPath(in.Source)
		if err == nil {
			path, err = filepath.Abs(path)
		}
		if err == nil {
			path, err = filepath.EvalSymlinks(path)
		}
		if info, statErr := os.Stat(path); err != nil || statErr != nil || !info.IsDir() {
			return nil, nil, fmt.Errorf("--mount takes directories of the host, and %s is not one", in.Source)
		}
		if strings.Contains(path, ":") {
			return nil, nil, fmt.Errorf("%s has a colon in its path, which a mount cannot name; mount a directory without one", path)
		}
		r := api.Repository{Name: in.Name, HostPath: path, Author: req.Author, Mounted: true, Context: in.Context}
		if top, err := git(ctx, path, "rev-parse", "--show-toplevel"); err == nil {
			r.HostPath = top
			// A worktree's .git names its main repository by the host's
			// path, which the sandbox mounts elsewhere or not at all.
			if info, err := os.Lstat(filepath.Join(top, ".git")); err == nil && !info.IsDir() {
				main, _ := git(ctx, top, "rev-parse", "--path-format=absolute", "--git-common-dir")
				return nil, nil, fmt.Errorf("%s is a git worktree, which cannot be mounted; mount the repository's own checkout (%s), or pass --clone to clone it", top, filepath.Dir(main))
			}
			current, _ := git(ctx, top, "symbolic-ref", "--quiet", "--short", "HEAD")
			r.Branch = current
			if branch != "" && branch != current {
				if _, err := git(ctx, ".", "check-ref-format", "--branch", branch); err != nil {
					return nil, nil, fmt.Errorf("invalid branch %q; use a git branch name", branch)
				}
				r.Branch = branch
				made := ""
				if _, err := git(ctx, top, "show-ref", "--verify", "--quiet", "refs/heads/"+branch); err != nil {
					made = ", made there from HEAD"
				}
				needs = append(needs, api.Need{Code: "branch_switch", Message: "the host's checkout is switched to " + branch + made, Repository: in.Source, Warning: true})
			}
			if raw, err := git(ctx, top, "config", "--get", "remote.origin.url"); err == nil {
				r.URL, _ = CloneURL(raw)
			}
			if r.Author == "" {
				name, _ := git(ctx, top, "config", "user.name")
				email, _ := git(ctx, top, "config", "user.email")
				if name != "" || email != "" {
					r.Author = name + " <" + email + ">"
				}
			}
		}
		if r.Name == "" {
			r.Name = filepath.Base(r.HostPath)
		}
		if err := ValidName(r.Name); err != nil {
			return nil, nil, fmt.Errorf("repository directory: %w; use --repo name=path", err)
		}
		if names[r.Name] {
			return nil, nil, fmt.Errorf("duplicate repository directory %s; use --repo name=path", r.Name)
		}
		names[r.Name] = true
		if r.Author != "" {
			if _, _, err := AuthorParts(r.Author); err != nil {
				return nil, nil, err
			}
		}
		if branch != "" && r.Branch == "" {
			return nil, nil, fmt.Errorf("%s is not a git repository, so it has no branch to switch to; leave out --branch", r.HostPath)
		}
		repos = append(repos, r)
	}
	return repos, needs, nil
}

// switchMounted switches each mounted repository's host checkout to the
// branch the task asked for, making it from HEAD when the host has none of
// that name. git refuses a switch that would overwrite uncommitted changes,
// and so does unibox, naming them.
func switchMounted(ctx context.Context, repos []api.Repository) error {
	for _, r := range repos {
		if !r.Mounted || r.Branch == "" {
			continue
		}
		if current, _ := git(ctx, r.HostPath, "symbolic-ref", "--quiet", "--short", "HEAD"); current == r.Branch {
			continue
		}
		args := []string{"switch", r.Branch}
		if _, err := git(ctx, r.HostPath, "show-ref", "--verify", "--quiet", "refs/heads/"+r.Branch); err != nil {
			args = []string{"switch", "-c", r.Branch}
		}
		if _, err := git(ctx, r.HostPath, args...); err != nil {
			return fmt.Errorf("cannot switch %s to %s; commit or stash what is in the way and retry: %w", r.HostPath, r.Branch, err)
		}
	}
	return nil
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// HostCheckout is what a task mounting a directory of the host takes from
// it: the top of its checkout, its forge address and the branch it is on.
func HostCheckout(ctx context.Context, dir string) (top, url, branch string, err error) {
	path, err := ExpandPath(dir)
	if err == nil {
		path, err = filepath.Abs(path)
	}
	if err == nil {
		path, err = filepath.EvalSymlinks(path)
	}
	if err != nil {
		return "", "", "", err
	}
	if top, err = git(ctx, path, "rev-parse", "--show-toplevel"); err != nil {
		return path, "", "", nil
	}
	if raw, err := git(ctx, top, "config", "--get", "remote.origin.url"); err == nil {
		url, _ = CloneURL(raw)
	}
	branch, _ = git(ctx, top, "symbolic-ref", "--quiet", "--short", "HEAD")
	return top, url, branch, nil
}

// HostHasBranch says whether a checkout of the host has branch.
func HostHasBranch(ctx context.Context, top, branch string) bool {
	_, err := git(ctx, top, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

// RemoteHasBranch asks a forge, with the host's git and as a task's clone
// would, whether a repository has branch.
func RemoteHasBranch(ctx context.Context, url, branch string) (bool, error) {
	url, err := CloneURL(url)
	if err != nil {
		return false, err
	}
	_, tip, err := remoteRefs(ctx, ".", url, branch)
	return tip != "", err
}
