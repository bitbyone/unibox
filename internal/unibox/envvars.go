package unibox

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// The args an env's files ask of a person are theirs, kept in their home and
// never in the team's repository: [envs.<name>.args] in vars.toml. unibox
// hands them to sbx as an --env-args-file at each create, so nobody types
// them twice.

const envValuesHeader = `# Your own values for the args your envs' sbxenv files declare, kept here
# and never in the team's repository: [envs.<env>.args]. unibox var set and
# unibox var rm change this file, and unibox passes the values to
# sbx env create as an --env-args-file. No secret belongs here - store tokens
# with sbx secret set (unibox login).

`

// EnvValues are each env's args, by env.
type EnvValues struct {
	Envs map[string]map[string]string
	// other are tables of an older unibox, kept as they are.
	other map[string]any
}

func (p Paths) LoadEnvValues() (EnvValues, error) {
	v := EnvValues{Envs: map[string]map[string]string{}, other: map[string]any{}}
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
	for key, body := range doc {
		if key != "envs" {
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
				return v, fmt.Errorf("%s: envs.%s is a table with args in it", p.VarsFile(), env)
			}
			values := map[string]string{}
			for k, a := range table {
				if k != "args" {
					return v, fmt.Errorf("%s: envs.%s takes args, not %s", p.VarsFile(), env, k)
				}
				args, ok := a.(map[string]any)
				if !ok {
					return v, fmt.Errorf("%s: envs.%s.args is a table", p.VarsFile(), env)
				}
				for name, value := range args {
					s, ok := value.(string)
					if !ok {
						return v, fmt.Errorf("%s: envs.%s.args.%s is a string, in quotes", p.VarsFile(), env, name)
					}
					if err = ValidArg(name, s); err != nil {
						return v, fmt.Errorf("%s: envs.%s.args: %w", p.VarsFile(), env, err)
					}
					values[name] = s
				}
			}
			v.Envs[env] = values
		}
	}
	return v, nil
}

func (p Paths) SaveEnvValues(v EnvValues) error {
	doc := map[string]any{}
	for k, body := range v.other {
		doc[k] = body
	}
	envs := map[string]any{}
	for env, values := range v.Envs {
		if len(values) > 0 {
			envs[env] = map[string]any{"args": values}
		}
	}
	if len(envs) > 0 {
		doc["envs"] = envs
	}
	var out bytes.Buffer
	e := toml.NewEncoder(&out)
	e.SetTablesInline(false)
	if err := e.Encode(doc); err != nil {
		return err
	}
	return AtomicWrite(p.VarsFile(), append([]byte(envValuesHeader), out.Bytes()...), 0o600)
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
		return fmt.Errorf("arg %s: no secret belongs in vars.toml; store a token with sbx secret set (unibox login)", name)
	}
	return nil
}
