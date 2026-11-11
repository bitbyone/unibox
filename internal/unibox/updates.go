package unibox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// How often unibox asks whether something newer is out: its own release
// and sbx's change seldom, the team's repository of envs more often.
const (
	ReleaseCheckEvery = 24 * time.Hour
	RepoCheckEvery    = 4 * time.Hour
)

// Updates are what the last check found newer than what this computer
// has, kept in the cache so a command shows them without waiting on the
// network.
type Updates struct {
	// Released and Fetched are when the releases and the env repositories
	// were last checked, or a check of them was started.
	Released time.Time `json:"released"`
	Fetched  time.Time `json:"fetched"`
	// Unibox and SBX are newer releases than the ones installed, or "".
	Unibox string `json:"unibox,omitempty"`
	SBX    string `json:"sbx,omitempty"`
	// Behind are the env repositories behind their remotes, by directory,
	// with how many commits.
	Behind map[string]int `json:"behind,omitempty"`
}

func (p Paths) updatesPath() string { return filepath.Join(p.Cache, "updates.json") }

// ReadUpdates are the last check's findings; none when there was none.
func (p Paths) ReadUpdates() Updates {
	var u Updates
	if b, err := os.ReadFile(p.updatesPath()); err == nil {
		_ = json.Unmarshal(b, &u)
	}
	return u
}

func (p Paths) SaveUpdates(u Updates) error {
	b, err := json.MarshalIndent(u, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(p.Cache, 0o755); err != nil {
		return err
	}
	return AtomicWrite(p.updatesPath(), append(b, '\n'), 0o600)
}

// Due says whether the releases and the repositories are to be checked
// again at now.
func (u Updates) Due(now time.Time) (releases, repos bool) {
	return now.Sub(u.Released) >= ReleaseCheckEvery, now.Sub(u.Fetched) >= RepoCheckEvery
}

// ClaimCheck marks the check as started, so commands run while it is
// under way do not start it again, and says what is due; nothing is when
// another command claimed it first.
func (p Paths) ClaimCheck(now time.Time) (releases, repos bool, err error) {
	l, err := LockFile(filepath.Join(p.Cache, "updates.lock"), false, false)
	if err != nil {
		return false, false, nil
	}
	defer l.Close()
	u := p.ReadUpdates()
	releases, repos = u.Due(now)
	if !releases && !repos {
		return false, false, nil
	}
	if releases {
		u.Released = now
	}
	if repos {
		u.Fetched = now
	}
	return releases, repos, p.SaveUpdates(u)
}

// UpdateCheck is what is compared with what is out: this unibox's release
// ("dev" while developing, which is never compared), and the env
// repositories.
type UpdateCheck struct {
	Unibox string
	Repos  []string
}

// Check finds what is newer and keeps it, the releases and the
// repositories as asked. A source that cannot be reached keeps what was
// found of it before.
func (p Paths) Check(ctx context.Context, sbx Runtime, c UpdateCheck, releases, repos bool) Updates {
	var newUnibox, newSBX *string
	if releases {
		if v, err := NewerUnibox(ctx, c.Unibox); err == nil {
			newUnibox = &v
		}
		if installed, err := sbx.Version(ctx); err == nil {
			if v, err := NewerRelease(ctx, "docker/sbx-releases", installed); err == nil {
				newSBX = &v
			}
		}
	}
	behind, reached := map[string]int{}, map[string]bool{}
	if repos {
		for _, dir := range c.Repos {
			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			n, err := Behind(ctx, dir)
			cancel()
			reached[dir] = err == nil
			if err == nil && n > 0 {
				behind[dir] = n
			}
		}
	}
	return p.keep(func(u *Updates) {
		if newUnibox != nil {
			u.Unibox = *newUnibox
		}
		if newSBX != nil {
			u.SBX = *newSBX
		}
		if repos {
			for dir, n := range u.Behind {
				if _, still := behind[dir]; !still && !reached[dir] && slices.Contains(c.Repos, dir) {
					behind[dir] = n
				}
			}
			u.Behind = behind
		}
	})
}

// CaughtUp forgets that env repositories were behind their remotes, as
// unibox env update has brought them up to them.
func (p Paths) CaughtUp(dirs []string) {
	if len(dirs) == 0 {
		return
	}
	p.keep(func(u *Updates) {
		for _, dir := range dirs {
			delete(u.Behind, dir)
		}
	})
}

// KeepReleases keeps what doctor found of the releases, as a check of
// them at now.
func (p Paths) KeepReleases(now time.Time, unibox, sbx string) {
	p.keep(func(u *Updates) { u.Released, u.Unibox, u.SBX = now, unibox, sbx })
}

func (p Paths) keep(change func(*Updates)) Updates {
	l, err := LockFile(filepath.Join(p.Cache, "updates.lock"), false, true)
	if err == nil {
		defer l.Close()
	}
	u := p.ReadUpdates()
	change(&u)
	_ = p.SaveUpdates(u)
	return u
}

// IsRelease says whether a version is a release's, not a build's while
// developing.
func IsRelease(v string) bool { return releaseTag.MatchString(v) }

// NewerUnibox is unibox's newest release past installed, on its channel:
// a nightly one (-pre.N) is offered nightlies too, a stable one only
// stable releases. A build while developing is offered nothing.
func NewerUnibox(ctx context.Context, installed string) (string, error) {
	if !releaseTag.MatchString(installed) {
		return "", nil
	}
	return NewerRelease(ctx, "bitbyone/unibox", installed)
}

// NewerRelease is a GitHub repository's newest release past installed,
// or "": prereleases count only when installed is one.
func NewerRelease(ctx context.Context, repo, installed string) (string, error) {
	tags, err := releaseTags(ctx, repo)
	if err != nil {
		return "", err
	}
	pre := strings.Contains(installed, "-")
	newest := strings.TrimPrefix(installed, "v")
	for _, t := range tags {
		t = strings.TrimPrefix(t, "v")
		if !releaseTag.MatchString(t) || (strings.Contains(t, "-") && !pre) {
			continue
		}
		if CompareReleases(t, newest) > 0 {
			newest = t
		}
	}
	if newest == strings.TrimPrefix(installed, "v") {
		return "", nil
	}
	return newest, nil
}

// releasesAPI is GitHub's API, a variable for the tests.
var releasesAPI = "https://api.github.com"

func releaseTags(ctx context.Context, repo string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesAPI+"/repos/"+repo+"/releases?per_page=30", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub answered %s for the releases of %s", res.Status, repo)
	}
	var list []struct {
		Tag   string `json:"tag_name"`
		Draft bool   `json:"draft"`
	}
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		return nil, err
	}
	out := []string{}
	for _, r := range list {
		if !r.Draft {
			out = append(out, r.Tag)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no release of " + repo)
	}
	return out, nil
}

// releaseTag is a release as unibox and sbx tag them: 0.7, 0.51.0, or a
// prerelease 0.7-pre.4.
var releaseTag = regexp.MustCompile(`^v?(\d+)\.(\d+)(?:\.(\d+))?(?:-pre\.(\d+))?$`)

// CompareReleases orders two releases: a prerelease before the release
// it leads to, 0.7-pre.4 before 0.7.
func CompareReleases(a, b string) int {
	pa, pb := releaseParts(a), releaseParts(b)
	return slices.Compare(pa, pb)
}

func releaseParts(v string) []int {
	m := releaseTag.FindStringSubmatch(v)
	if m == nil {
		return []int{-1}
	}
	n := func(s string, missing int) int {
		if s == "" {
			return missing
		}
		i, _ := strconv.Atoi(s)
		return i
	}
	// A release without -pre sorts after every prerelease of it.
	return []int{n(m[1], 0), n(m[2], 0), n(m[3], 0), n(m[4], int(^uint(0)>>1))}
}

// Notice is one thing newer than what this computer has: what it is,
// and the command or page that brings it.
type Notice struct {
	What string `json:"what"`
	Run  string `json:"run"`
}

// brewed says whether a program's executable is Homebrew's.
func brewed(exe string) bool {
	exe = strings.ReplaceAll(exe, `\`, "/")
	return strings.Contains(exe, "/Cellar/") || strings.Contains(exe, "/Caskroom/") || strings.Contains(exe, "/homebrew/") || strings.Contains(exe, "/linuxbrew/")
}

// UpgradeHint says how this unibox is brought to a newer release, by how
// it was installed: Homebrew and Scoop each have its formula, on its
// channel; anywhere else the release's page.
func UpgradeHint(executable, release string) string {
	name := "unibox"
	if strings.Contains(release, "-pre") {
		name = "unibox-nightly"
	}
	switch {
	case brewed(executable):
		return "brew upgrade " + name
	case strings.Contains(strings.ToLower(strings.ReplaceAll(executable, `\`, "/")), "/scoop/"):
		return "scoop update " + name
	}
	return "https://github.com/bitbyone/unibox/releases/tag/v" + release
}

// SBXUpgradeHint is Homebrew's command where sbx is its cask, else the
// release's page: an installer on Windows, a package on Linux.
func SBXUpgradeHint(executable, release string) string {
	if brewed(executable) {
		return "brew upgrade sbx"
	}
	return "https://github.com/docker/sbx-releases/releases/tag/v" + release
}

// Notices are the updates found, for a person at a terminal; repos names
// each env repository by the envs it holds, and executables are where
// unibox and sbx are, which say how each was installed.
func (u Updates) Notices(unibox, sbx string, repos map[string][]string) []Notice {
	out := []Notice{}
	if u.Unibox != "" {
		out = append(out, Notice{"unibox " + u.Unibox + " available", UpgradeHint(unibox, u.Unibox)})
	}
	if u.SBX != "" {
		out = append(out, Notice{"sbx " + u.SBX + " available", SBXUpgradeHint(sbx, u.SBX)})
	}
	for _, dir := range slices.Sorted(maps.Keys(u.Behind)) {
		envs, ok := repos[dir]
		if !ok {
			// No env is in it any longer.
			continue
		}
		update := "unibox env update"
		if len(envs) == 1 {
			update += " " + envs[0]
		}
		out = append(out, Notice{fmt.Sprintf("%s has %d new commit(s)", filepath.Base(dir), u.Behind[dir]), update})
	}
	return out
}

// ReposOfEnvs are the repositories the person's envs are in, each with its
// envs' names.
func (c Config) ReposOfEnvs() map[string][]string {
	out := map[string][]string{}
	for _, name := range slices.Sorted(maps.Keys(c.Envs)) {
		if repo := c.Envs[name].Repo; repo != "" {
			out[repo] = append(out[repo], name)
		}
	}
	return out
}

// UpdateChecks says whether unibox may ask the network what is newer:
// unless config.yaml or UNIBOX_NO_UPDATE_CHECK says not to.
func (c Config) UpdateChecks() bool {
	if os.Getenv("UNIBOX_NO_UPDATE_CHECK") != "" {
		return false
	}
	return c.UpdateCheck == nil || *c.UpdateCheck
}
