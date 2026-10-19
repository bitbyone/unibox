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

// ValidVar says whether a name and value can be kept: a variable name, no
// secret by its name, one line.
func ValidVar(name, value string) error {
	if !toolEnvName.MatchString(name) {
		return fmt.Errorf("%q is not a variable name; use letters, digits and _", name)
	}
	if sensitiveEnv.MatchString(name) {
		return fmt.Errorf("%s names a secret; keep it with unibox secret set instead", name)
	}
	if strings.ContainsAny(value, "\n\r") {
		return fmt.Errorf("a value is one line")
	}
	return nil
}

var varRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// VarRefs are the names a field refers to as ${NAME}.
func VarRefs(s string) []string {
	out := []string{}
	for _, m := range varRef.FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	return out
}

// withVars gives the profile its person's values: kept for the sessions,
// and put into the services' user and author where they name them.
func (p Profile) withVars(values map[string]string) (Profile, error) {
	p.VarValues = map[string]string{}
	for name := range p.Vars {
		if v, ok := values[name]; ok {
			p.VarValues[name] = v
		}
	}
	if len(p.MissingVars()) > 0 || len(p.Services) == 0 {
		return p, nil
	}
	services := map[string]Service{}
	for name, s := range p.Services {
		fill := func(field string) string {
			return varRef.ReplaceAllStringFunc(field, func(ref string) string { return p.VarValues[varRef.FindStringSubmatch(ref)[1]] })
		}
		s.User, s.Author = fill(s.User), fill(s.Author)
		if s.Author != "" {
			if _, _, err := AuthorParts(s.Author); err != nil {
				return p, fmt.Errorf("profile %s: service %s author is %q from your vars, which is not Name <email>: %w", p.Name, name, s.Author, err)
			}
		}
		services[name] = s
	}
	p.Services = services
	return p, nil
}

// MissingVars are the profile's vars without a value.
func (p Profile) MissingVars() []string {
	out := []string{}
	for _, name := range sortedKeys(p.Vars) {
		if _, ok := p.VarValues[name]; !ok {
			out = append(out, name)
		}
	}
	return out
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
// a shell's, the scripts' and start's: the person's values, then each env
// service's placeholder.
func (p Profile) SessionEnv() []string {
	out := []string{}
	for _, name := range sortedKeys(p.VarValues) {
		out = append(out, name+"="+p.VarValues[name])
	}
	return append(out, p.ServiceSessionEnv()...)
}

func quoteAll(xs []string) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = Quote(x)
	}
	return out
}
