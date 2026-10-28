package unibox

import (
	"cmp"
	"fmt"
	"slices"
	"time"

	"github.com/tobola/unibox/internal/unibox/api"
)

// RecentRepo is a repository a task of the person's has held: the host's
// checkout it was mounted or cloned from, or the forge's address of one
// cloned from there alone, with the env of the sandbox the task was in.
// They are offered beside that env's index alone, as a sandbox of another
// env may reach neither the forge nor the secrets the repository needs.
type RecentRepo struct {
	Env  string    `json:"env"`
	Path string    `json:"path,omitempty"`
	URL  string    `json:"url,omitempty"`
	Used time.Time `json:"used,omitzero"`
}

// recentKeep is how many recent repositories are kept, the oldest dropped.
const recentKeep = 200

// Source is how a person names a recent repository: a checkout by its
// path, as two checkouts of one forge are both worked in, else its
// address.
func (r RecentRepo) Source() string { return cmp.Or(r.Path, r.URL) }

func (r RecentRepo) same(o RecentRepo) bool { return r.Env == o.Env && r.Source() == o.Source() }

// noteRecent keeps the repositories a task of env holds as recent, the
// newest first.
func noteRecent(s *State, env string, repos []api.Repository, now time.Time) {
	for _, r := range repos {
		rec := RecentRepo{Env: env, Path: r.HostPath, URL: r.URL, Used: now}
		if rec.Source() == "" {
			continue
		}
		s.Recent = slices.DeleteFunc(s.Recent, rec.same)
		s.Recent = append([]RecentRepo{rec}, s.Recent...)
	}
	if len(s.Recent) > recentKeep {
		s.Recent = s.Recent[:recentKeep]
	}
}

// RecentRepos are the repositories the person's tasks in sandboxes of env
// have held, or of every env for "", the newest first: those recorded, and
// those of tasks from before unibox recorded them.
func (p Paths) RecentRepos(env string) ([]RecentRepo, error) {
	s, err := p.ReadState()
	if err != nil {
		return nil, err
	}
	out := []RecentRepo{}
	add := func(rec RecentRepo) {
		if rec.Source() != "" && rec.Env != "" && (env == "" || rec.Env == env) && !slices.ContainsFunc(out, rec.same) {
			out = append(out, rec)
		}
	}
	for _, r := range s.Recent {
		add(r)
	}
	for _, name := range sortedKeys(s.Sandboxes) {
		b := s.Sandboxes[name]
		for _, t := range b.Tasks {
			for _, r := range t.Repositories {
				add(RecentRepo{Env: b.Env, Path: r.HostPath, URL: r.URL})
			}
		}
	}
	return out, nil
}

// ForgetRecent takes a repository, by its path or address, out of the
// recent ones of env, or of every env for "". One a task holds now is
// offered still, as the task says it was worked with.
func (p Paths) ForgetRecent(env, source string) error {
	return p.State(func(s *State) error {
		n := len(s.Recent)
		s.Recent = slices.DeleteFunc(s.Recent, func(r RecentRepo) bool {
			return (env == "" || r.Env == env) && (r.Path == source || r.URL == source)
		})
		if len(s.Recent) < n {
			return nil
		}
		for _, name := range sortedKeys(s.Sandboxes) {
			b := s.Sandboxes[name]
			for _, t := range b.Tasks {
				if (env == "" || b.Env == env) && slices.ContainsFunc(t.Repositories, func(r api.Repository) bool { return r.HostPath == source || r.URL == source }) {
					return fmt.Errorf("task %s holds %s, which is offered while it does; unibox task rm %s removes the task", t.ID, source, t.ID)
				}
			}
		}
		return fmt.Errorf("no recent repository %s; unibox repo recent lists them", source)
	})
}
