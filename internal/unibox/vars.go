package unibox

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// A profile shared by a team says what each person has to set for
// themselves (vars); the values are each person's, in their home, and never
// in the environments repository. Shared values apply to every profile, a
// profile's own win over them. A run stops while one is missing.

// UserVars is ~/.config/unibox/vars.yaml.
type UserVars struct {
	Vars     map[string]string            `yaml:"vars"`
	Profiles map[string]map[string]string `yaml:"profiles"`
}

const varsHeader = `# Your own values for what profiles ask of you (their vars:), kept here and
# never in the environments repository. Values under vars apply to every
# profile; those under profiles.<name> to that profile alone, and win.
# unibox var set|ls|rm changes this file; no secret belongs here - keep
# tokens with unibox secret set.
`

func (p Paths) VarsFile() string { return filepath.Join(p.Config, "vars.yaml") }

func (p Paths) LoadVars() (UserVars, error) {
	u := UserVars{Vars: map[string]string{}, Profiles: map[string]map[string]string{}}
	b, err := os.ReadFile(p.VarsFile())
	if os.IsNotExist(err) {
		return u, nil
	}
	if err != nil {
		return u, err
	}
	if err = yaml.Unmarshal(b, &u); err != nil {
		return u, fmt.Errorf("%s is not valid YAML; fix it or set the values again with unibox var set: %w", p.VarsFile(), err)
	}
	if u.Vars == nil {
		u.Vars = map[string]string{}
	}
	if u.Profiles == nil {
		u.Profiles = map[string]map[string]string{}
	}
	return u, nil
}

func (p Paths) SaveVars(u UserVars) error {
	for name, values := range u.Profiles {
		if len(values) == 0 {
			delete(u.Profiles, name)
		}
	}
	b, err := yaml.Marshal(u)
	if err != nil {
		return err
	}
	return AtomicWrite(p.VarsFile(), append([]byte(varsHeader), b...), 0o600)
}

// EnsureVarsFile lays the file out when there is none, so a person finds
// where their values go.
func (p Paths) EnsureVarsFile() (bool, error) {
	if _, err := os.Stat(p.VarsFile()); err == nil {
		return false, nil
	}
	return true, p.SaveVars(UserVars{Vars: map[string]string{}, Profiles: map[string]map[string]string{}})
}

// For is the values one profile sees: the shared ones with its own over
// them.
func (u UserVars) For(profile string) map[string]string {
	out := map[string]string{}
	for k, v := range u.Vars {
		out[k] = v
	}
	for k, v := range u.Profiles[profile] {
		out[k] = v
	}
	return out
}

// ValidVarName says whether a var's name can be used: a name like other
// unibox names, such as artifactory-user, that does not say secret.
func ValidVarName(name string) error {
	if ValidName(name) != nil {
		return fmt.Errorf("%q is lower-case letters, digits and hyphens, such as artifactory-user", name)
	}
	if sensitiveEnv.MatchString(name) || strings.Contains(name, "password") || strings.Contains(name, "token") || strings.Contains(name, "secret") {
		return fmt.Errorf("%s names a secret; keep it with unibox secret set and name it as secret:<name>", name)
	}
	return nil
}

// ValidVar says whether a value can be kept under a name: one line.
func ValidVar(name, value string) error {
	if err := ValidVarName(name); err != nil {
		return err
	}
	if strings.ContainsAny(value, "\n\r") {
		return fmt.Errorf("a value is one line")
	}
	return nil
}

// A value in a profile may refer to a person's value, ${vars.<name>}, or -
// in env alone - to a service, ${services.<name>}, which stands for the
// placeholder of its token.
var anyRef = regexp.MustCompile(`\$\{([^}]*)\}`)
var namedRef = regexp.MustCompile(`^(vars|services)\.([a-z][a-z0-9-]*)$`)

func hasRefs(s string) bool { return anyRef.MatchString(s) }

// checkRefs says what is wrong with a value's references: one of an unknown
// form, to an undeclared var, to a service that is not there or signs git
// in, or to a service where none may be named.
func (p Profile) checkRefs(where, value string, services bool) error {
	for _, m := range anyRef.FindAllStringSubmatch(value, -1) {
		parts := namedRef.FindStringSubmatch(m[1])
		switch {
		case parts == nil:
			return fmt.Errorf("profile %s: %s refers to ${%s}; write ${vars.<name>} or ${services.<name>}", p.Name, where, m[1])
		case parts[1] == "vars":
			if _, ok := p.Vars[parts[2]]; !ok {
				return fmt.Errorf("profile %s: %s refers to ${vars.%s}, which its vars do not declare", p.Name, where, parts[2])
			}
		case !services:
			return fmt.Errorf("profile %s: %s refers to ${services.%s}; only env may name a service", p.Name, where, parts[2])
		default:
			s, ok := p.Services[parts[2]]
			if !ok {
				return fmt.Errorf("profile %s: %s refers to ${services.%s}, which is no service of the profile", p.Name, where, parts[2])
			}
			if s.Git {
				return fmt.Errorf("profile %s: %s refers to ${services.%s}, a git service, which git signs in through by itself", p.Name, where, parts[2])
			}
		}
	}
	return nil
}

// refsIn are the vars and the services a value refers to.
func refsIn(value string) (vars, services []string) {
	for _, m := range anyRef.FindAllStringSubmatch(value, -1) {
		if parts := namedRef.FindStringSubmatch(m[1]); parts != nil {
			if parts[1] == "vars" {
				vars = append(vars, parts[2])
			} else {
				services = append(services, parts[2])
			}
		}
	}
	return vars, services
}

// usedRefs are the vars and services the profile refers to anywhere.
func (p Profile) usedRefs() (vars, services map[string]bool) {
	vars, services = map[string]bool{}, map[string]bool{}
	values := []string{}
	for _, v := range p.EnvValues {
		values = append(values, v)
	}
	for _, v := range p.BuildArgSpec {
		values = append(values, v)
	}
	for _, s := range p.Services {
		values = append(values, s.User, s.Author)
	}
	for _, v := range values {
		vs, ss := refsIn(v)
		for _, n := range vs {
			vars[n] = true
		}
		for _, n := range ss {
			services[n] = true
		}
	}
	return vars, services
}

// fill puts the values in place of a value's references.
func (p Profile) fill(value string) string {
	return anyRef.ReplaceAllStringFunc(value, func(ref string) string {
		parts := namedRef.FindStringSubmatch(anyRef.FindStringSubmatch(ref)[1])
		if parts == nil {
			return ref
		}
		if parts[1] == "vars" {
			return p.VarValues[parts[2]]
		}
		return ServicePlaceholder(parts[2], p.Services[parts[2]])
	})
}

// withVars gives the profile its person's values, filled into env,
// build_args and the services' user and author. Build arguments are part of
// what makes the image, so they join its revision.
func (p Profile) withVars(values map[string]string) (Profile, error) {
	p.VarValues = map[string]string{}
	for name := range p.Vars {
		if v, ok := values[name]; ok {
			p.VarValues[name] = v
		}
	}
	if len(p.MissingVars()) > 0 {
		return p, nil
	}
	p.SetEnv = map[string]string{}
	for k, v := range p.EnvValues {
		p.SetEnv[k] = p.fill(v)
	}
	p.BuildArgValues = map[string]string{}
	for k, v := range p.BuildArgSpec {
		p.BuildArgValues[k] = p.fill(v)
	}
	if len(p.BuildArgValues) > 0 {
		p.Revision = digest(struct {
			Revision string
			Args     map[string]string
		}{p.Revision, p.BuildArgValues})
	}
	if len(p.Services) > 0 {
		services := map[string]Service{}
		for name, s := range p.Services {
			s.User, s.Author = p.fill(s.User), p.fill(s.Author)
			if s.Author != "" {
				if _, _, err := AuthorParts(s.Author); err != nil {
					return p, fmt.Errorf("profile %s: service %s author is %q from your vars, which is not Name <email>: %w", p.Name, name, s.Author, err)
				}
			}
			services[name] = s
		}
		p.Services = services
	}
	return p, nil
}

// MissingVars are the vars the profile refers to without a value.
func (p Profile) MissingVars() []string {
	used, _ := p.usedRefs()
	out := []string{}
	for _, name := range sortedKeys(p.Vars) {
		if _, ok := p.VarValues[name]; !ok && used[name] {
			out = append(out, name)
		}
	}
	return out
}

// Unused are the vars nothing refers to, and the services without git that
// no variable carries, so no tool sends their token.
func (p Profile) Unused() (vars, services []string) {
	usedVars, usedServices := p.usedRefs()
	for _, name := range sortedKeys(p.Vars) {
		if !usedVars[name] {
			vars = append(vars, name)
		}
	}
	for _, name := range sortedKeys(p.Services) {
		if !p.Services[name].Git && !usedServices[name] {
			services = append(services, name)
		}
	}
	return vars, services
}

// missingVarsError is what stops a run while a value is missing.
func (p Profile) missingVarsError() error {
	missing := p.MissingVars()
	if len(missing) == 0 {
		return nil
	}
	lines := []string{}
	for _, name := range missing {
		line := name
		if d := p.Vars[name]; d != "" {
			line += " (" + d + ")"
		}
		lines = append(lines, line)
	}
	return Fail(4, "var_missing", "profile %s needs values of your own: %s; set each with unibox var set <name> <value>, or --profile %s for this profile alone", p.Name, strings.Join(lines, ", "), p.Name)
}

// SessionEnv is what unibox sets in every session it starts - an agent's,
// a shell's, the scripts' and start's: the variables the profile's env sets,
// with every reference filled.
func (p Profile) SessionEnv() []string {
	out := []string{}
	for _, name := range sortedKeys(p.SetEnv) {
		out = append(out, name+"="+p.SetEnv[name])
	}
	return out
}

func quoteAll(xs []string) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = Quote(x)
	}
	return out
}
