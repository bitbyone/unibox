package unibox

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// The args an env's files ask of a person are theirs, kept in their home and
// never in the team's repository: [args] in vars.toml for every env that
// declares them, [envs.<name>.args] for one. unibox hands them to sbx as an
// --env-args-file at each create, so nobody types them twice. The
// resources a sandbox gets of the machine are the person's alike:
// [resources], [envs.<name>.resources] and, for one sandbox,
// [sandboxes.<name>.resources].

const envValuesHeader = `# Your own values for the args your envs' sbxenv files declare, kept here
# and never in the team's repository: [args] for every env that declares
# them, [envs.<env>.args] for one, which wins. unibox var set and unibox
# var rm change them, and unibox passes them to sbx env create as an
# --env-args-file. [resources], [envs.<env>.resources] and
# [sandboxes.<sandbox>.resources] are the CPUs and memory your sandboxes
# get, a size or a share of this machine such as 75%, written here by hand
# or by unibox sandbox create --cpus/--memory. No secret belongs here -
# store tokens with unibox secret set.

`

// EnvValues are the person's args and resources: for every env, and each
// env's own, by env.
type EnvValues struct {
	Args         map[string]string
	Envs         map[string]map[string]string
	Resources    Resources
	EnvResources map[string]Resources
	// SandboxResources are one sandbox's, over its env's.
	SandboxResources map[string]Resources
	// other are tables of an older unibox, kept as they are.
	other map[string]any
}

// VarsFile holds each env's args of the person's, in TOML.
func (p Paths) VarsFile() string { return filepath.Join(p.Config, "vars.toml") }

func (p Paths) LoadEnvValues() (EnvValues, error) {
	v := EnvValues{Args: map[string]string{}, Envs: map[string]map[string]string{}, EnvResources: map[string]Resources{}, SandboxResources: map[string]Resources{}, other: map[string]any{}}
	b, err := os.ReadFile(p.VarsFile())
	if os.IsNotExist(err) {
		return v, nil
	}
	if err != nil {
		return v, err
	}
	var doc map[string]any
	if err = toml.Unmarshal(b, &doc); err != nil {
		return v, fmt.Errorf("%s is not valid TOML; fix it or set the values again with unibox var set: %w", p.VarsFile(), err)
	}
	readArgs := func(at string, body any) (map[string]string, error) {
		args, ok := body.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s: %s is a table", p.VarsFile(), at)
		}
		values := map[string]string{}
		for name, value := range args {
			s, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("%s: %s.%s is a string, in quotes", p.VarsFile(), at, name)
			}
			if err := ValidArg(name, s); err != nil {
				return nil, fmt.Errorf("%s: %s: %w", p.VarsFile(), at, err)
			}
			values[name] = s
		}
		return values, nil
	}
	readResources := func(at string, body any) (Resources, error) {
		var r Resources
		table, ok := body.(map[string]any)
		if !ok {
			return r, fmt.Errorf("%s: %s is a table", p.VarsFile(), at)
		}
		for name, value := range table {
			s, ok := value.(string)
			if !ok {
				return r, fmt.Errorf("%s: %s.%s is a string, in quotes", p.VarsFile(), at, name)
			}
			if err := ValidResource(name, s); err != nil {
				return r, fmt.Errorf("%s: %s: %w", p.VarsFile(), at, err)
			}
			r.Set(name, s)
		}
		return r, nil
	}
	for key, body := range doc {
		switch key {
		case "args":
			if v.Args, err = readArgs("args", body); err != nil {
				return v, err
			}
			continue
		case "resources":
			if v.Resources, err = readResources("resources", body); err != nil {
				return v, err
			}
			continue
		case "sandboxes":
			boxes, ok := body.(map[string]any)
			if !ok {
				return v, fmt.Errorf("%s: sandboxes is a table of [sandboxes.<sandbox>.resources]", p.VarsFile())
			}
			for name, t := range boxes {
				table, ok := t.(map[string]any)
				if !ok {
					return v, fmt.Errorf("%s: sandboxes.%s is a table with resources in it", p.VarsFile(), name)
				}
				for k, body := range table {
					if k != "resources" {
						return v, fmt.Errorf("%s: sandboxes.%s takes resources, not %s", p.VarsFile(), name, k)
					}
					if v.SandboxResources[name], err = readResources("sandboxes."+name+".resources", body); err != nil {
						return v, err
					}
				}
			}
			continue
		case "envs":
		default:
			v.other[key] = body
			continue
		}
		envs, ok := body.(map[string]any)
		if !ok {
			return v, fmt.Errorf("%s: envs is a table of [envs.<env>.args]", p.VarsFile())
		}
		for env, t := range envs {
			table, ok := t.(map[string]any)
			if !ok {
				return v, fmt.Errorf("%s: envs.%s is a table with args or resources in it", p.VarsFile(), env)
			}
			for k, body := range table {
				switch k {
				case "args":
					if v.Envs[env], err = readArgs("envs."+env+".args", body); err != nil {
						return v, err
					}
				case "resources":
					if v.EnvResources[env], err = readResources("envs."+env+".resources", body); err != nil {
						return v, err
					}
				default:
					return v, fmt.Errorf("%s: envs.%s takes args and resources, not %s", p.VarsFile(), env, k)
				}
			}
		}
	}
	return v, nil
}

// ArgsFor are the person's args for an env: those for every env that it
// declares, and its own over them.
func (v EnvValues) ArgsFor(env Env) map[string]string {
	out := map[string]string{}
	for name, value := range v.Args {
		if _, ok := env.SBX.Args[name]; ok {
			out[name] = value
		}
	}
	maps.Copy(out, v.Envs[env.Name])
	return out
}

// ResourcesFor are the person's resources for a sandbox of an env: the
// sandbox's own, then the env's, then those for every env, each taking
// what the one before leaves unset.
func (v EnvValues) ResourcesFor(env, sandbox string) Resources {
	return v.SandboxResources[sandbox].Over(v.EnvResources[env].Over(v.Resources))
}

// SetSandboxResources makes r a sandbox's own resources; none forgets
// them.
func (p Paths) SetSandboxResources(sandbox string, r Resources) error {
	v, err := p.LoadEnvValues()
	if err != nil {
		return err
	}
	if v.SandboxResources[sandbox] == r {
		return nil
	}
	if r == (Resources{}) {
		delete(v.SandboxResources, sandbox)
	} else {
		v.SandboxResources[sandbox] = r
	}
	return p.SaveEnvValues(v)
}

func (p Paths) SaveEnvValues(v EnvValues) error {
	doc := map[string]any{}
	for k, body := range v.other {
		doc[k] = body
	}
	if len(v.Args) > 0 {
		doc["args"] = v.Args
	}
	if v.Resources != (Resources{}) {
		doc["resources"] = v.Resources
	}
	envs := map[string]any{}
	for env, values := range v.Envs {
		if len(values) > 0 {
			envs[env] = map[string]any{"args": values}
		}
	}
	for env, r := range v.EnvResources {
		if r == (Resources{}) {
			continue
		}
		t, _ := envs[env].(map[string]any)
		if t == nil {
			t = map[string]any{}
			envs[env] = t
		}
		t["resources"] = r
	}
	if len(envs) > 0 {
		doc["envs"] = envs
	}
	boxes := map[string]any{}
	for name, r := range v.SandboxResources {
		if r != (Resources{}) {
			boxes[name] = map[string]any{"resources": r}
		}
	}
	if len(boxes) > 0 {
		doc["sandboxes"] = boxes
	}
	var out bytes.Buffer
	e := toml.NewEncoder(&out)
	e.SetTablesInline(false)
	if err := e.Encode(doc); err != nil {
		return err
	}
	return AtomicWrite(p.VarsFile(), append([]byte(envValuesHeader), withoutBareTables(out.Bytes())...), 0o600)
}

// withoutBareTables leaves out the headers of tables that hold nothing but
// the next one, such as [envs] before [envs.dev.args], which the encoder
// writes and a person need not read.
func withoutBareTables(b []byte) []byte {
	lines := strings.SplitAfter(string(b), "\n")
	var out strings.Builder
	for i, line := range lines {
		head := strings.TrimSpace(line)
		if strings.HasPrefix(head, "[") && !strings.HasPrefix(head, "[[") {
			next := ""
			for _, l := range lines[i+1:] {
				if next = strings.TrimSpace(l); next != "" {
					break
				}
			}
			if strings.HasPrefix(next, strings.TrimSuffix(head, "]")+".") {
				continue
			}
		}
		out.WriteString(line)
	}
	return []byte(out.String())
}

// ValidArg says whether a value can be kept for an arg: a name sbx takes, a
// line, and no secret - the args file is plain text, and so is vars.toml.
func ValidArg(name, value string) error {
	if !argName.MatchString(name) {
		return fmt.Errorf("%q is not an arg name; use the name the sbxenv file declares", name)
	}
	if strings.ContainsAny(value, "\n\r") {
		return fmt.Errorf("arg %s: a value is one line", name)
	}
	if strings.HasPrefix(value, "secret:") {
		return fmt.Errorf("arg %s: no secret belongs in vars.toml; store a token with unibox secret set", name)
	}
	return nil
}
