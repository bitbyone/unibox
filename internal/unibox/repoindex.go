package unibox

import (
	"fmt"
	"maps"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// IndexedRepo is a repository a team or a person names, to pick for a
// task by its name: its forge address, the branch a task works on unless
// told otherwise, and whether a new task takes it - always, every new task
// of the env, mounted or cloned; on, picked to begin with in unibox task
// new; off, only when picked. A context repository - decisions, a
// specification - is one a task reads: it stays on its ref, and no task's
// branch is made or pushed there. Unsaid, one that says always is one.
type IndexedRepo struct {
	URL         string `yaml:"url,omitempty" json:"url"`
	Ref         string `yaml:"ref,omitempty" json:"ref,omitempty"`
	Default     string `yaml:"default,omitempty" json:"default,omitempty"`
	Context     *bool  `yaml:"context,omitempty" json:"context,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	// From is the layer that names it, FromRepo to FromYours, and
	// DefaultFrom the one that says its Default.
	From        string `yaml:"-" json:"from,omitempty"`
	DefaultFrom string `yaml:"-" json:"default_from,omitempty"`
}

// isPath says whether an address is a directory of the host rather than a
// forge's: absolute, or under the home or the current directory.
func isPath(address string) bool {
	return strings.HasPrefix(address, "/") || strings.HasPrefix(address, "~") || strings.HasPrefix(address, ".")
}

// LocalPath is the host directory a repository is, which a task mounts;
// "" for one a task clones from its forge.
func (r IndexedRepo) LocalPath() string {
	if !isPath(r.URL) {
		return ""
	}
	p, err := ExpandPath(r.URL)
	if err != nil {
		return ""
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	return p
}

// IsContext says whether a task reads the repository rather than works in
// it.
func (r IndexedRepo) IsContext() bool {
	if r.Context != nil {
		return *r.Context
	}
	return r.Default == RepoAlways
}

// What a repository's default may be.
const (
	RepoOff    = "off"
	RepoOn     = "on"
	RepoAlways = "always"
)

// RepoDefaults are the defaults a repository may have, from the least.
var RepoDefaults = []string{RepoOff, RepoOn, RepoAlways}

// repoDefault reads a default as a person writes it: on, off or always, or
// true and false for on and off.
func repoDefault(s string) (string, bool) {
	if strings.EqualFold(s, RepoAlways) {
		return RepoAlways, true
	}
	if on, ok := onOff(s); ok {
		return map[bool]string{true: RepoOn, false: RepoOff}[on], true
	}
	return "", false
}

// UnmarshalYAML takes a repository as its address alone, or as {url, ref,
// default, description}; a person's entry may say only default, which
// switches a repository of the team's.
func (r *IndexedRepo) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		*r = IndexedRepo{URL: n.Value}
		return nil
	case yaml.MappingNode:
		*r = IndexedRepo{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if v.Kind != yaml.ScalarNode {
				return fmt.Errorf("line %d: %s is one value", v.Line, k.Value)
			}
			switch k.Value {
			case "url":
				r.URL = v.Value
			case "ref":
				r.Ref = v.Value
			case "description":
				r.Description = v.Value
			case "context":
				on, ok := onOff(v.Value)
				if !ok {
					return fmt.Errorf("line %d: a repository's context is true or false", v.Line)
				}
				r.Context = &on
			case "default":
				d, ok := repoDefault(v.Value)
				if !ok {
					return fmt.Errorf("line %d: a repository's default is always, on or off", v.Line)
				}
				r.Default = d
			default:
				return fmt.Errorf("line %d: a repository says url, ref, default, context and description, not %s", k.Line, k.Value)
			}
		}
		return nil
	}
	return fmt.Errorf("line %d: a repository is its address, or {url: ..., ref: main, default: on}", n.Line)
}

// MarshalYAML writes a repository with its address alone as just that, as
// a person would write it.
func (r IndexedRepo) MarshalYAML() (any, error) {
	if r.Ref == "" && r.Default == "" && r.Context == nil && r.Description == "" {
		return r.URL, nil
	}
	type plain IndexedRepo
	return plain(r), nil
}

// validRepo checks a repository as a layer names it: its address a forge
// one, and a switch only for a repository named under it.
func validRepo(name string, r IndexedRepo, known bool) error {
	if err := ValidName(name); err != nil {
		return fmt.Errorf("repository %q: %w", name, err)
	}
	if r.URL == "" {
		if !known {
			return fmt.Errorf("repository %s says no url, and nothing under it names one", name)
		}
		return nil
	}
	if isPath(r.URL) {
		return nil
	}
	if _, err := CloneURL(r.URL); err != nil {
		return fmt.Errorf("repository %s: %w; or give a directory of this machine, starting with / or ~", name, err)
	}
	return nil
}

// layerRepos lays a layer's repositories over those beneath: a field it
// says wins, one it leaves out is kept.
func layerRepos(out map[string]IndexedRepo, layer map[string]IndexedRepo, from string) error {
	for name, r := range layer {
		was, known := out[name]
		// A switch for every env skips an env without the repository, as
		// another env may have it.
		if r.URL == "" && !known && from == FromYoursAll {
			continue
		}
		if err := validRepo(name, r, known); err != nil {
			return err
		}
		if !known {
			was.From = from
		}
		if r.URL != "" {
			was.URL = r.URL
		}
		if r.Ref != "" {
			was.Ref = r.Ref
		}
		if r.Description != "" {
			was.Description = r.Description
		}
		if r.Context != nil {
			was.Context = r.Context
		}
		if r.Default != "" {
			was.Default, was.DefaultFrom = r.Default, from
		}
		out[name] = was
	}
	return nil
}

// envRepos are an env's repositories, each layer over the one before: the
// repository's unibox.yaml, the env's file's, the person's for every env
// and the person's for this one.
func envRepos(ec EnvConfig, global map[string]IndexedRepo) (map[string]IndexedRepo, error) {
	out := map[string]IndexedRepo{}
	err := readTeamFiles(ec.Files, func(name, from string, doc teamDoc) error {
		if err := layerRepos(out, doc.Repos, from); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, layer := range []struct {
		repos map[string]IndexedRepo
		from  string
	}{{global, FromYoursAll}, {ec.Repos, FromYours}} {
		if err := layerRepos(out, layer.repos, layer.from); err != nil {
			return nil, fmt.Errorf("config.yaml: %w", err)
		}
	}
	for name, r := range out {
		if r.Default == "" {
			r.Default = RepoOff
		}
		out[name] = r
	}
	return out, nil
}

// SetRepo writes a repository of the person's into config.yaml: for one env,
// or with env "" for every env. A field r leaves out is kept from what the
// person had.
func (m *Manager) SetRepo(env, name string, r IndexedRepo) error {
	if p := r.LocalPath(); p != "" && !isDir(p) {
		return fmt.Errorf("repository %s: %s is no directory of this machine", name, r.URL)
	}
	return m.updateRepos(env, func(_ Config, repos map[string]IndexedRepo) error {
		was := repos[name]
		if r.URL == "" {
			r.URL = was.URL
		}
		if r.Ref == "" {
			r.Ref = was.Ref
		}
		if r.Description == "" {
			r.Description = was.Description
		}
		if r.Default == "" {
			r.Default = was.Default
		}
		if r.Context == nil {
			r.Context = was.Context
		}
		repos[name] = IndexedRepo{URL: r.URL, Ref: r.Ref, Default: r.Default, Context: r.Context, Description: r.Description}
		return nil
	})
}

// RemoveRepo takes a repository of the person's out of config.yaml, for
// one env or with env "" for every env. One of the team's is not the
// person's to remove; off keeps it from being picked.
func (m *Manager) RemoveRepo(env, name string) error {
	return m.updateRepos(env, func(c Config, repos map[string]IndexedRepo) error {
		if env == "" {
			// A switch of the person's for one env goes with the
			// repository it switched, where the team names none.
			for e, ec := range c.Envs {
				if mine, ok := ec.Repos[name]; !ok || mine.URL != "" {
					continue
				}
				team, err := envRepos(EnvConfig{Files: ec.Files}, nil)
				if err != nil {
					return err
				}
				if _, named := team[name]; !named {
					ec.Repos = maps.Clone(ec.Repos)
					delete(ec.Repos, name)
					c.Envs[e] = ec
				}
			}
		}
		if _, ok := repos[name]; !ok {
			if env == "" {
				return fmt.Errorf("your config.yaml names no repository %s for every env; unibox repo ls says where each comes from", name)
			}
			return fmt.Errorf("your config.yaml names no repository %s for env %s; one of the team's stays, and unibox repo default %s off keeps it unpicked", name, env, name)
		}
		delete(repos, name)
		return nil
	})
}

// SetRepoDefault has a repository of an env always taken, picked to begin
// with or not, for the person; "" takes their say back. A switch that says
// what the layers beneath say is dropped.
func (m *Manager) SetRepoDefault(env, name, def string) error {
	if env == "" {
		return fmt.Errorf("say the env whose repository %s to switch", name)
	}
	return m.updateRepos(env, func(c Config, repos map[string]IndexedRepo) error {
		ec := c.Envs[env]
		ec.Repos = maps.Clone(repos)
		delete(ec.Repos, name)
		under, err := envRepos(ec, c.Repos)
		if err != nil {
			return err
		}
		beneath, known := under[name]
		mine, had := repos[name]
		if !known && !had {
			return fmt.Errorf("env %s has no repository %s; unibox repo ls lists them", env, name)
		}
		switch {
		case def == "" && mine.URL == "":
			delete(repos, name)
		case def == "":
			mine.Default = ""
			repos[name] = mine
		case known && mine.URL == "" && mine.Ref == "" && mine.Context == nil && mine.Description == "" && beneath.Default == def:
			delete(repos, name)
		default:
			mine.Default = def
			repos[name] = mine
		}
		return nil
	})
}

// updateRepos changes the person's repositories in config.yaml, for one env
// or for every env, and checks the env's index still reads.
func (m *Manager) updateRepos(env string, change func(Config, map[string]IndexedRepo) error) error {
	c, err := m.Paths.LoadConfig()
	if err != nil {
		return err
	}
	c.Envs = maps.Clone(c.Envs)
	repos := c.Repos
	ec, ok := c.Envs[env]
	if env != "" {
		if !ok {
			return Fail(4, "env_not_found", "there is no env %s; unibox env ls lists them", env)
		}
		repos = ec.Repos
	}
	repos = maps.Clone(repos)
	if repos == nil {
		repos = map[string]IndexedRepo{}
	}
	if err := change(c, repos); err != nil {
		return err
	}
	if len(repos) == 0 {
		repos = nil
	}
	if env == "" {
		c.Repos = repos
	} else {
		ec.Repos = repos
		c.Envs[env] = ec
	}
	for name, e := range c.Envs {
		if _, err := envRepos(e, c.Repos); err != nil {
			return fmt.Errorf("env %s: %w", name, err)
		}
	}
	if err := m.Paths.SaveConfig(c); err != nil {
		return err
	}
	m.Config = c
	return nil
}

// RepoDefaultOf reads a repository's default as a person types it.
func RepoDefaultOf(s string) (string, bool) { return repoDefault(s) }
