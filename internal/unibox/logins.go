package unibox

import (
	"context"
	"fmt"
	"regexp"
	"slices"
)

var secretName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// ValidSecretName says whether a name is one sbx stores a service's token
// under, as a kit's credentials and an env's bindings name them.
func ValidSecretName(name string) bool { return secretName.MatchString(name) }

// Login is a service an env's sandbox takes a token for, and whether sbx
// has one stored: the person's logins are sbx's secrets, set once and used
// by every sandbox that needs them.
type Login struct {
	Service     string `json:"service"`
	Description string `json:"description,omitempty"`
	// Optional is a credential a kit can do without.
	Optional bool `json:"optional,omitempty"`
	// Kits are the kits asking for it; empty for one only the env's
	// bindings name.
	Kits   []string `json:"kits,omitempty"`
	Stored bool     `json:"stored"`
	// Scope is where the token is stored: global, or a sandbox's name.
	Scope string `json:"scope,omitempty"`
}

// Logins are the services an env needs tokens for: those its kits declare
// and those its bindings name, each with whether sbx has one, globally or
// for the sandbox alone. sandbox may be "" when there is none yet.
func (m *Manager) Logins(ctx context.Context, env Env, sandbox string) ([]Login, error) {
	byService := map[string]*Login{}
	order := []string{}
	add := func(service string) *Login {
		if l, ok := byService[service]; ok {
			return l
		}
		l := &Login{Service: service, Optional: true}
		byService[service] = l
		order = append(order, service)
		return l
	}
	// A kit is read with the args the env gives it, which may be the
	// person's.
	values, _ := m.Paths.LoadEnvValues()
	for _, k := range env.SBX.Kits {
		ref := k.Source
		if k.Dir != "" {
			ref = k.Dir
		}
		kit, err := m.SBX.KitInspect(ctx, ref, env.SBX.KitArgs(k, values.ArgsFor(env)))
		if err != nil {
			return nil, err
		}
		for _, c := range kit.Credentials {
			l := add(c.Service)
			if l.Description == "" {
				l.Description = c.Description
			}
			l.Optional = l.Optional && c.Optional
			l.Kits = append(l.Kits, kitName(k.Source))
		}
	}
	for _, service := range env.SBX.Bindings {
		add(service).Optional = false
	}
	secrets, err := m.SBX.Secrets(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot list sbx's secrets: %w", err)
	}
	out := []Login{}
	for _, service := range order {
		l := *byService[service]
		for _, s := range secrets {
			if s.Name != service {
				continue
			}
			if s.Scope == "global" || sandbox != "" && s.Scope == sandbox {
				l.Stored, l.Scope = true, s.Scope
			}
		}
		out = append(out, l)
	}
	slices.SortStableFunc(out, func(a, b Login) int {
		switch {
		case !a.Optional && b.Optional:
			return -1
		case a.Optional && !b.Optional:
			return 1
		}
		return 0
	})
	return out, nil
}
