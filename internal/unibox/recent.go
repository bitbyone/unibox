package unibox

import (
	"cmp"
	"slices"
	"time"

	"github.com/tobola/unibox/internal/unibox/api"
)

// RecentRepo is a repository a task of the person's has held: the host's
// checkout it was mounted or cloned from, or the forge's address of one
// cloned from there alone. They are offered beside an env's index, where
// a person finds them by name and may name them there.
type RecentRepo struct {
	Path string    `json:"path,omitempty"`
	URL  string    `json:"url,omitempty"`
	Used time.Time `json:"used"`
}

// recentKeep is how many recent repositories are kept, the oldest dropped.
const recentKeep = 200

// key tells two records of one repository apart from those of others: a
// checkout by its path, as two checkouts of one forge are both worked in.
func (r RecentRepo) key() string { return cmp.Or(r.Path, r.URL) }

// noteRecent keeps the repositories a task holds as recent, the newest
// first.
func noteRecent(s *State, repos []api.Repository, now time.Time) {
	for _, r := range repos {
		rec := RecentRepo{Path: r.HostPath, URL: r.URL, Used: now}
		if rec.key() == "" {
			continue
		}
		s.Recent = slices.DeleteFunc(s.Recent, func(o RecentRepo) bool { return o.key() == rec.key() })
		s.Recent = append([]RecentRepo{rec}, s.Recent...)
	}
	if len(s.Recent) > recentKeep {
		s.Recent = s.Recent[:recentKeep]
	}
}

// RecentRepos are the repositories the person's tasks have held, the
// newest first: those recorded, and those of tasks from before unibox
// recorded them.
func (p Paths) RecentRepos() ([]RecentRepo, error) {
	s, err := p.ReadState()
	if err != nil {
		return nil, err
	}
	out := slices.Clone(s.Recent)
	seen := map[string]bool{}
	for _, r := range out {
		seen[r.key()] = true
	}
	for _, b := range s.Sandboxes {
		for _, t := range b.Tasks {
			for _, r := range t.Repositories {
				rec := RecentRepo{Path: r.HostPath, URL: r.URL}
				if rec.key() != "" && !seen[rec.key()] {
					seen[rec.key()] = true
					out = append(out, rec)
				}
			}
		}
	}
	return out, nil
}
