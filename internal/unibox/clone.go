package unibox

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// scpLike is git's short form of an SSH address, user@host:path.
var scpLike = regexp.MustCompile(`^(?:[A-Za-z0-9._-]+@)?([A-Za-z0-9.-]+):([^/\\][^:]*)$`)

// IsGitURL says whether s is the address of a repository to clone, rather
// than a path on this host.
func IsGitURL(s string) bool {
	for _, scheme := range []string{"https://", "http://", "ssh://", "git://", "file://"} {
		if strings.HasPrefix(s, scheme) {
			return true
		}
	}
	return strings.Contains(s, "@") && scpLike.MatchString(s)
}

// ReposDir is where unibox clones the repositories init is given by
// address, one directory per host and path.
func (p Paths) ReposDir() string { return filepath.Join(cmp.Or(p.Data, p.Config), "repos") }

// CloneDir is where the repository at address is cloned:
// <repos>/<host>/<path>, without .git.
func (p Paths) CloneDir(address string) (string, error) {
	host, path := "", ""
	if m := scpLike.FindStringSubmatch(address); m != nil && !strings.Contains(address, "://") {
		host, path = m[1], m[2]
	} else {
		u, err := url.Parse(address)
		if err != nil {
			return "", fmt.Errorf("cannot read %s as a git address: %w", address, err)
		}
		host, path = cmp.Or(u.Hostname(), "local"), u.Path
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	if path == "" {
		return "", fmt.Errorf("%s names no repository", address)
	}
	parts := append([]string{p.ReposDir(), host}, strings.Split(path, "/")...)
	for _, part := range parts[1:] {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("%s names no repository unibox can keep", address)
		}
	}
	return filepath.Join(parts...), nil
}

// Clone clones the repository at address where CloneDir says, or finds the
// clone there is; it says whether it cloned. git runs as the user's own,
// with their credentials and on their terminal, asking there when it must
// and ask is set.
func (p Paths) Clone(ctx context.Context, address string, ask bool, in io.Reader, out io.Writer) (dir string, cloned bool, err error) {
	dir, err = p.CloneDir(address)
	if err != nil {
		return "", false, err
	}
	if _, err = os.Stat(dir); err == nil {
		origin, err := gitOutput(ctx, dir, "remote", "get-url", "origin")
		if err != nil {
			return "", false, fmt.Errorf("%s is there but is no clone unibox can use; move it away and run unibox init %s again", dir, address)
		}
		if origin != address {
			return "", false, fmt.Errorf("%s is a clone of %s, not %s; run unibox init %s to use it", dir, origin, address, dir)
		}
		return dir, false, nil
	}
	if err = os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", false, err
	}
	c := userGit(ctx, ask, "clone", "--", address, dir)
	c.Stdin, c.Stdout, c.Stderr = in, out, out
	if err = c.Run(); err != nil {
		_ = os.RemoveAll(dir)
		return "", false, fmt.Errorf("git could not clone %s; check the address and that you can reach it: %w", address, err)
	}
	return dir, true, nil
}

// CloneSource is the address of the repository at dir when unibox cloned
// it, or "" when it is the user's own checkout.
func (p Paths) CloneSource(ctx context.Context, dir string) string {
	if dir == "" {
		return ""
	}
	rel, err := filepath.Rel(p.ReposDir(), dir)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		// The repository may be the same directory spelled through a
		// symbolic link, as git spells the path it gives.
		repos, err := filepath.EvalSymlinks(p.ReposDir())
		if err != nil {
			return ""
		}
		if rel, err = filepath.Rel(repos, dir); err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			return ""
		}
	}
	origin, err := gitOutput(context.WithoutCancel(ctx), dir, "remote", "get-url", "origin")
	if err != nil {
		return ""
	}
	return origin
}

// userGit is git as the user runs it: their whole environment, which holds
// what reaches their repositories - an SSH agent, a credential helper. It
// asks for nothing unless ask is set.
func userGit(ctx context.Context, ask bool, args ...string) *exec.Cmd {
	c := exec.CommandContext(ctx, "git", args...)
	c.Env = os.Environ()
	if !ask {
		c.Env = append(c.Env, "GIT_TERMINAL_PROMPT=0")
		if os.Getenv("GIT_SSH_COMMAND") == "" {
			c.Env = append(c.Env, "GIT_SSH_COMMAND=ssh -o BatchMode=yes")
		}
	}
	return c
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	b, err := commandOutput(hostCommand(ctx, "git", append([]string{"-C", dir}, args...)...))
	return strings.TrimSpace(string(b)), err
}

// Pull brings a clone of unibox's up to its remote by a fast-forward alone,
// so nothing of the user's in it is merged or lost, and says whether it
// moved.
func Pull(ctx context.Context, dir string, ask bool, in io.Reader, out io.Writer) (moved bool, err error) {
	before, err := gitOutput(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return false, err
	}
	c := userGit(ctx, ask, "-C", dir, "pull", "--ff-only", "--quiet")
	c.Stdin, c.Stdout, c.Stderr = in, out, out
	if err = c.Run(); err != nil {
		return false, fmt.Errorf("git could not fast-forward %s; git says why above, and the clone is set right with git in it: %w", dir, err)
	}
	after, err := gitOutput(ctx, dir, "rev-parse", "HEAD")
	return before != after, err
}

// Behind fetches a clone's remote and says how many commits its branch is
// behind it.
func Behind(ctx context.Context, dir string) (int, error) {
	if err := runQuiet(userGit(ctx, false, "-C", dir, "fetch", "--quiet")); err != nil {
		return 0, fmt.Errorf("could not reach its remote: %w", err)
	}
	count, err := gitOutput(ctx, dir, "rev-list", "--count", "HEAD..@{upstream}")
	if err != nil {
		return 0, err
	}
	n := 0
	_, err = fmt.Sscan(count, &n)
	return n, err
}
