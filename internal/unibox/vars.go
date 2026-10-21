package unibox

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

// A profile shared by a team says what each person has to set for
// themselves (vars); the values are each person's, in their home, and never
// in the environments repository. Shared values apply to every profile, a
// profile's own win over them. A run stops while one is missing.

// UserVars is ~/.config/unibox/vars.toml: global values, and a profile's
// own over them; and how a secret:<name> is read, if not from the
// platform's store, again for every profile or for one.
type UserVars struct {
	Vars     map[string]string
	Profiles map[string]map[string]string
	// Secrets is the command a secret:<name> is read by, {name} standing
	// for the name; ProfileSecrets a profile's own, over it.
	Secrets        string
	ProfileSecrets map[string]string
	// Names are commands for one secret each, by its name - gh auth token
	// for copilot - over the commands above; ProfileNames a profile's own.
	Names        map[string]string
	ProfileNames map[string]map[string]string
}

const varsHeader = `# Your own values for what profiles ask of you (their vars), kept here and
# never in the environments repository. Values under [global] apply to every
# profile; those under [profiles.<name>] to that profile alone, and win.
# [secrets] command = "..." reads a profile's secret:<name> by a command of
# yours, {name} in it, instead of the platform's store, and [secrets.names]
# one secret by its own command (copilot = "gh auth token"); the same under
# [profiles.<name>.secrets] for one profile. unibox var and unibox secret
# source change this file; no secret belongs here - keep tokens with unibox
# secret set or your own store.

`

func (p Paths) VarsFile() string { return filepath.Join(p.Config, "vars.toml") }

// legacyVarsFile is where the values were kept in YAML, moved into
// vars.toml the first time they are read.
func (p Paths) legacyVarsFile() string { return filepath.Join(p.Config, "vars.yaml") }

func emptyVars() UserVars {
	return UserVars{Vars: map[string]string{}, Profiles: map[string]map[string]string{}, ProfileSecrets: map[string]string{}, Names: map[string]string{}, ProfileNames: map[string]map[string]string{}}
}

func (p Paths) LoadVars() (UserVars, error) {
	u := emptyVars()
	b, err := os.ReadFile(p.VarsFile())
	if os.IsNotExist(err) {
		return p.migrateVars(u)
	}
	if err != nil {
		return u, err
	}
	var doc map[string]any
	if err = toml.Unmarshal(b, &doc); err != nil {
		return u, fmt.Errorf("%s is not valid TOML; fix it or set the values again with unibox var set: %w", p.VarsFile(), err)
	}
	if err = u.read(doc); err != nil {
		return u, fmt.Errorf("%s: %w", p.VarsFile(), err)
	}
	return u, nil
}

// read takes the file's tables, naming what does not belong.
func (u *UserVars) read(doc map[string]any) error {
	strs := func(where string, v any, skip string) (map[string]string, error) {
		t, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s is a table", where)
		}
		out := map[string]string{}
		for k, v := range t {
			if k == skip {
				continue
			}
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("%s.%s is a string, in quotes", where, k)
			}
			out[k] = s
		}
		return out, nil
	}
	// A secrets table: a command for every secret, and one for each by
	// its name.
	command := func(where string, v any) (string, map[string]string, error) {
		t, err := strs(where, v, "names")
		if err != nil {
			return "", nil, err
		}
		for k := range t {
			if k != "command" {
				return "", nil, fmt.Errorf("%s takes command and names, not %s", where, k)
			}
		}
		if c, ok := t["command"]; ok {
			if err = ValidSecretCommand(c); err != nil {
				return "", nil, fmt.Errorf("%s.command: %w", where, err)
			}
		}
		names := map[string]string{}
		if n, ok := v.(map[string]any)["names"]; ok {
			if names, err = strs(where+".names", n, ""); err != nil {
				return "", nil, err
			}
			for name, c := range names {
				if err = ValidName(name); err != nil {
					return "", nil, fmt.Errorf("%s.names: %w", where, err)
				}
				if strings.TrimSpace(c) == "" || strings.ContainsAny(c, "\n\r") {
					return "", nil, fmt.Errorf("%s.names.%s is a command of one line", where, name)
				}
			}
		}
		return t["command"], names, nil
	}
	for key, v := range doc {
		var err error
		switch key {
		case "global":
			u.Vars, err = strs("global", v, "")
		case "secrets":
			u.Secrets, u.Names, err = command("secrets", v)
		case "profiles":
			profiles, ok := v.(map[string]any)
			if !ok {
				return fmt.Errorf("profiles is a table of [profiles.<name>]")
			}
			for name, body := range profiles {
				where := "profiles." + name
				if u.Profiles[name], err = strs(where, body, "secrets"); err != nil {
					return err
				}
				if sec, ok := body.(map[string]any)["secrets"]; ok {
					if u.ProfileSecrets[name], u.ProfileNames[name], err = command(where+".secrets", sec); err != nil {
						return err
					}
				}
			}
		default:
			err = fmt.Errorf("%s belongs in no table here; values go under [global] or [profiles.<name>], a command for secrets under [secrets]", key)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// ValidSecretCommand says whether a command can read secrets: one line,
// naming the secret as {name}.
func ValidSecretCommand(command string) error {
	if strings.TrimSpace(command) == "" || strings.ContainsAny(command, "\n\r") {
		return fmt.Errorf("a command for secrets is one line")
	}
	if !strings.Contains(command, SecretNameField) {
		return fmt.Errorf("a command for secrets names the secret as %s, as op read op://Work/%s/credential", SecretNameField, SecretNameField)
	}
	return nil
}

// SecretVia is the command a profile's secret:<name> is read by, or "" for
// the platform's store: the secret's own command, the profile's before the
// global one, then the command for every secret, the profile's first.
func (u UserVars) SecretVia(profile, name string) string {
	if c := u.ProfileNames[profile][name]; c != "" {
		return c
	}
	if c := u.Names[name]; c != "" {
		return c
	}
	return u.SecretsFor(profile)
}

// SecretsFor is the command for every secret of a profile, or "" for the
// platform's store.
func (u UserVars) SecretsFor(profile string) string {
	if c := u.ProfileSecrets[profile]; c != "" {
		return c
	}
	return u.Secrets
}

func (p Paths) migrateVars(u UserVars) (UserVars, error) {
	b, err := os.ReadFile(p.legacyVarsFile())
	if os.IsNotExist(err) {
		return u, nil
	}
	if err != nil {
		return u, err
	}
	var old struct {
		Vars     map[string]string            `yaml:"vars"`
		Profiles map[string]map[string]string `yaml:"profiles"`
	}
	if err = yaml.Unmarshal(b, &old); err != nil {
		return u, fmt.Errorf("%s is not valid YAML, so it could not be moved into %s; fix it or set the values again with unibox var set: %w", p.legacyVarsFile(), p.VarsFile(), err)
	}
	for k, v := range old.Vars {
		u.Vars[k] = v
	}
	for name, values := range old.Profiles {
		u.Profiles[name] = values
	}
	if err = p.SaveVars(u); err != nil {
		return u, err
	}
	return u, os.Remove(p.legacyVarsFile())
}

func (p Paths) SaveVars(u UserVars) error {
	// A struct keeps the tables in the order the header tells them.
	type secrets struct {
		Command string            `toml:"command,omitempty"`
		Names   map[string]string `toml:"names,omitempty"`
	}
	var doc struct {
		Global   map[string]string `toml:"global"`
		Secrets  *secrets          `toml:"secrets,omitempty"`
		Profiles map[string]any    `toml:"profiles,omitempty"`
	}
	doc.Global = u.Vars
	if doc.Global == nil {
		doc.Global = map[string]string{}
	}
	if u.Secrets != "" || len(u.Names) > 0 {
		doc.Secrets = &secrets{u.Secrets, u.Names}
	}
	profiles := map[string]any{}
	for name, values := range u.Profiles {
		if len(values) > 0 {
			t := map[string]any{}
			for k, v := range values {
				t[k] = v
			}
			profiles[name] = t
		}
	}
	for _, name := range sortedKeys(mergeKeys(u.ProfileSecrets, u.ProfileNames)) {
		command, names := u.ProfileSecrets[name], u.ProfileNames[name]
		if command == "" && len(names) == 0 {
			continue
		}
		t, _ := profiles[name].(map[string]any)
		if t == nil {
			t = map[string]any{}
			profiles[name] = t
		}
		t["secrets"] = secrets{command, names}
	}
	if len(profiles) > 0 {
		doc.Profiles = profiles
	}
	var out bytes.Buffer
	e := toml.NewEncoder(&out)
	e.SetTablesInline(false)
	if err := e.Encode(doc); err != nil {
		return err
	}
	return AtomicWrite(p.VarsFile(), append([]byte(varsHeader), out.Bytes()...), 0o600)
}

// EnsureVarsFile lays the file out when there is none, so a person finds
// where their values go.
func (p Paths) EnsureVarsFile() (bool, error) {
	if _, err := os.Stat(p.VarsFile()); err == nil {
		return false, nil
	}
	if _, err := os.Stat(p.legacyVarsFile()); err == nil {
		_, err = p.LoadVars()
		return false, err
	}
	return true, p.SaveVars(emptyVars())
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

// A value in a profile may refer to a person's value, ${vars.<name>}, with
// what stands for it while the person sets none, ${vars.<name>:-default};
// or - in env alone - to a service, ${services.<name>}, which stands for the
// placeholder of its token.
var anyRef = regexp.MustCompile(`\$\{([^}]*)\}`)
var namedRef = regexp.MustCompile(`^(vars|services)\.([a-z][a-z0-9-]*)(:-(.*))?$`)

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
		case parts[1] == "services" && parts[3] != "":
			return fmt.Errorf("profile %s: %s refers to ${%s}; a service has no default, only a var", p.Name, where, m[1])
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

// refsIn are the vars and the services a value refers to, and the vars it
// needs a value of: those it names without a default.
func refsIn(value string) (vars, services, required []string) {
	for _, m := range anyRef.FindAllStringSubmatch(value, -1) {
		if parts := namedRef.FindStringSubmatch(m[1]); parts != nil {
			if parts[1] == "vars" {
				vars = append(vars, parts[2])
				if parts[3] == "" {
					required = append(required, parts[2])
				}
			} else {
				services = append(services, parts[2])
			}
		}
	}
	return vars, services, required
}

// usedRefs are the vars and services the profile refers to anywhere.
func (p Profile) usedRefs() (vars, services map[string]bool) {
	vars, services, _ = p.refs()
	return vars, services
}

// refs are the vars and services the profile refers to anywhere, and the
// vars it needs a value of.
func (p Profile) refs() (vars, services, required map[string]bool) {
	vars, services, required = map[string]bool{}, map[string]bool{}, map[string]bool{}
	values := []string{string(p.ResourceSpec.CPUs), p.ResourceSpec.Memory}
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
		vs, ss, rs := refsIn(v)
		for _, n := range vs {
			vars[n] = true
		}
		for _, n := range ss {
			services[n] = true
		}
		for _, n := range rs {
			required[n] = true
		}
	}
	return vars, services, required
}

// fill puts the values in place of a value's references.
func (p Profile) fill(value string) string {
	return anyRef.ReplaceAllStringFunc(value, func(ref string) string {
		parts := namedRef.FindStringSubmatch(anyRef.FindStringSubmatch(ref)[1])
		if parts == nil {
			return ref
		}
		if parts[1] == "vars" {
			if v, ok := p.VarValues[parts[2]]; ok {
				return v
			}
			return parts[4]
		}
		return ServicePlaceholder(parts[2], p.Services[parts[2]])
	})
}

// withSecrets has each of the profile's secret:<name> read by the person's
// own command for it, if they keep it elsewhere than the platform's store.
func (p Profile) withSecrets(u UserVars) Profile {
	via := func(command string) string {
		name, ok := SecretRef(command)
		if !ok {
			return ""
		}
		c := u.SecretVia(p.Name, name)
		if c != "" {
			if p.SecretVia == nil {
				p.SecretVia = map[string]string{}
			}
			p.SecretVia[name] = c
		}
		return c
	}
	if len(p.Auth) > 0 {
		auth := map[string]AgentAuth{}
		for id, a := range p.Auth {
			a.Via = via(a.Command)
			auth[id] = a
		}
		p.Auth = auth
	}
	if len(p.Services) > 0 {
		services := map[string]Service{}
		for name, s := range p.Services {
			s.Via = via(s.Command)
			services[name] = s
		}
		p.Services = services
	}
	return p
}

// ReadBy is the person's own command a secret of the profile is read by,
// or "" when it is in the platform's store.
func (p Profile) ReadBy(name string) string { return p.SecretVia[name] }

func mergeKeys[A, B any](a map[string]A, b map[string]B) map[string]bool {
	out := map[string]bool{}
	for k := range a {
		out[k] = true
	}
	for k := range b {
		out[k] = true
	}
	return out
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
	r := p.ResourceSpec
	if r == (Resources{}) {
		r = p.Resources
	}
	r.CPUs, r.Memory = Amount(p.fill(string(r.CPUs))), p.fill(r.Memory)
	if err := r.validate(); err != nil {
		return p, fmt.Errorf("profile %s: %w, as your vars fill it in", p.Name, err)
	}
	p.Resources = r
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

// VarDefaults are what stands for each var while the person sets none:
// the first default the profile gives it.
func (p Profile) VarDefaults() map[string]string {
	out := map[string]string{}
	values := []string{string(p.ResourceSpec.CPUs), p.ResourceSpec.Memory}
	for _, k := range sortedKeys(p.EnvValues) {
		values = append(values, p.EnvValues[k])
	}
	for _, k := range sortedKeys(p.BuildArgSpec) {
		values = append(values, p.BuildArgSpec[k])
	}
	for _, k := range sortedKeys(p.Services) {
		values = append(values, p.Services[k].User, p.Services[k].Author)
	}
	for _, v := range values {
		for _, m := range anyRef.FindAllStringSubmatch(v, -1) {
			if parts := namedRef.FindStringSubmatch(m[1]); parts != nil && parts[1] == "vars" && parts[3] != "" {
				if _, ok := out[parts[2]]; !ok {
					out[parts[2]] = parts[4]
				}
			}
		}
	}
	return out
}

// MissingVars are the vars the profile needs a value of and has none:
// those it refers to without a default.
func (p Profile) MissingVars() []string {
	_, _, used := p.refs()
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
