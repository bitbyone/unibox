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
// declares them, [env_repos."<key>".args] for every env of one env
// repository, [envs.<name>.args] for one. unibox hands them to sbx as an
// --env-args-file at each create, so nobody types them twice. The
// resources a sandbox gets of the machine are the person's alike:
// [resources], [env_repos."<key>".resources], [envs.<name>.resources] and,
// for one sandbox, [sandboxes.<name>.resources]. Who commits are by is
// theirs alike: author for every env, [envs.<name>] author and
// [sandboxes.<name>] author, asked when the team's files require one.
// co_author = false, for every env or in [envs.<name>] for one, keeps
// agents from crediting themselves in commits, whatever their settings
// say; by default each agent does as its own settings say.

const envValuesHeader = `# Your own values for the args your envs' sbxenv files declare, kept here
# and never in the team's repository: [args] for every env that declares
# them, [env_repos."<repo>".args] for every env of one env repository,
# [envs.<env>.args] for one, each winning over the one before. unibox var
# set and unibox var rm change them, and unibox passes them to sbx env
# create as an --env-args-file. [resources], [env_repos."<repo>".resources],
# [envs.<env>.resources] and [sandboxes.<sandbox>.resources] are the CPUs
# and memory your sandboxes get, a size or a share of this machine such as
# 75%, written here by hand or by unibox sandbox create --cpus/--memory.
# author = "Name <email>" is who commits are by, for every env, with
# [envs.<env>] author and [sandboxes.<sandbox>] author winning over it;
# unibox author set and unibox author rm change them. co_author = false
# keeps any agent from naming itself co-author of its commits, for every
# env, with [envs.<env>] co_author winning over it; by default each agent
# does as its own settings say. unibox co-author changes them. No secret belongs
# here - store tokens with unibox secret set.

`

// EnvValues are the person's args and resources: for every env, for every
// env of an env repository, by its EnvRepoKey, and each env's own, by env.
type EnvValues struct {
	Args             map[string]string
	EnvRepos         map[string]map[string]string
	Envs             map[string]map[string]string
	Resources        Resources
	EnvRepoResources map[string]Resources
	EnvResources     map[string]Resources
	// SandboxResources are one sandbox's, over its env's.
	SandboxResources map[string]Resources
	// Author is who commits are by, as Name <email>, for every env; each
	// env's and each sandbox's own win over it.
	Author string
	// CoAuthor says whether agents name themselves co-author of their
	// commits, for every env; each env's own wins over it, and with
	// neither they do.
	CoAuthor       *bool
	EnvCoAuthors   map[string]bool
	EnvAuthors     map[string]string
	SandboxAuthors map[string]string
	// other are tables of an older unibox, kept as they are.
	other map[string]any
}

// VarsFile holds each env's args of the person's, in TOML.
func (p Paths) VarsFile() string { return filepath.Join(p.Config, "vars.toml") }

func (p Paths) LoadEnvValues() (EnvValues, error) {
	v := EnvValues{Args: map[string]string{}, EnvRepos: map[string]map[string]string{}, Envs: map[string]map[string]string{}, EnvRepoResources: map[string]Resources{}, EnvResources: map[string]Resources{}, SandboxResources: map[string]Resources{}, EnvAuthors: map[string]string{}, EnvCoAuthors: map[string]bool{}, SandboxAuthors: map[string]string{}, other: map[string]any{}}
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
	readAuthor := func(at string, body any) (string, error) {
		author, ok := body.(string)
		if !ok {
			return "", fmt.Errorf("%s: %s is a string, in quotes", p.VarsFile(), at)
		}
		if err := ValidAuthor(author); err != nil {
			return "", fmt.Errorf("%s: %s: %w", p.VarsFile(), at, err)
		}
		return author, nil
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
	readCoAuthor := func(at string, body any) (bool, error) {
		on, ok := body.(bool)
		if !ok {
			return false, fmt.Errorf("%s: %s is true or false", p.VarsFile(), at)
		}
		return on, nil
	}
	// readScopes reads the tables of envs or env repos, each with args and
	// resources in it, and an author and co_author where authors is given.
	readScopes := func(at string, body any, args map[string]map[string]string, resources map[string]Resources, authors map[string]string, coAuthors map[string]bool) error {
		scopes, ok := body.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: %s is a table of [%s.<name>.args]", p.VarsFile(), at, at)
		}
		for name, t := range scopes {
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("%s: %s has a table of no name; name it", p.VarsFile(), at)
			}
			table, ok := t.(map[string]any)
			if !ok {
				return fmt.Errorf("%s: %s.%s is a table with args or resources in it", p.VarsFile(), at, name)
			}
			for k, body := range table {
				var err error
				switch k {
				case "args":
					args[name], err = readArgs(at+"."+name+".args", body)
				case "resources":
					resources[name], err = readResources(at+"."+name+".resources", body)
				case "author":
					if authors == nil {
						err = fmt.Errorf("%s: %s.%s takes args and resources, not author; set it for the env or every env", p.VarsFile(), at, name)
						break
					}
					authors[name], err = readAuthor(at+"."+name+".author", body)
				case "co_author":
					if coAuthors == nil {
						err = fmt.Errorf("%s: %s.%s takes args and resources, not co_author; set it for the env or every env", p.VarsFile(), at, name)
						break
					}
					coAuthors[name], err = readCoAuthor(at+"."+name+".co_author", body)
				default:
					err = fmt.Errorf("%s: %s.%s takes args and resources, not %s", p.VarsFile(), at, name, k)
				}
				if err != nil {
					return err
				}
			}
		}
		return nil
	}
	for key, body := range doc {
		switch key {
		case "args":
			if v.Args, err = readArgs("args", body); err != nil {
				return v, err
			}
		case "resources":
			if v.Resources, err = readResources("resources", body); err != nil {
				return v, err
			}
		case "author":
			if v.Author, err = readAuthor("author", body); err != nil {
				return v, err
			}
		case "co_author":
			on, err := readCoAuthor("co_author", body)
			if err != nil {
				return v, err
			}
			v.CoAuthor = &on
		case "sandboxes":
			boxes, ok := body.(map[string]any)
			if !ok {
				return v, fmt.Errorf("%s: sandboxes is a table of [sandboxes.<sandbox>]", p.VarsFile())
			}
			for name, t := range boxes {
				table, ok := t.(map[string]any)
				if !ok {
					return v, fmt.Errorf("%s: sandboxes.%s is a table with an author or resources in it", p.VarsFile(), name)
				}
				for k, body := range table {
					switch k {
					case "resources":
						if v.SandboxResources[name], err = readResources("sandboxes."+name+".resources", body); err != nil {
							return v, err
						}
					case "author":
						if v.SandboxAuthors[name], err = readAuthor("sandboxes."+name+".author", body); err != nil {
							return v, err
						}
					default:
						return v, fmt.Errorf("%s: sandboxes.%s takes an author and resources, not %s", p.VarsFile(), name, k)
					}
				}
			}
		case "envs":
			if err = readScopes("envs", body, v.Envs, v.EnvResources, v.EnvAuthors, v.EnvCoAuthors); err != nil {
				return v, err
			}
		case "env_repos":
			if err = readScopes("env_repos", body, v.EnvRepos, v.EnvRepoResources, nil, nil); err != nil {
				return v, err
			}
		default:
			v.other[key] = body
		}
	}
	return v, nil
}

// envRepo is the key of the env's repository among the person's values,
// "" when they have none for any.
func (v EnvValues) envRepo(env Env) string {
	// Finding an env's repository may ask git, so it is looked for only
	// when there are values for one.
	if len(v.EnvRepos) == 0 && len(v.EnvRepoResources) == 0 {
		return ""
	}
	return EnvRepoKey(env.Config)
}

// ArgsFor are the person's args for an env: those for every env and for
// every env of its repository that it declares, and its own over them.
func (v EnvValues) ArgsFor(env Env) map[string]string {
	out := map[string]string{}
	for _, layer := range []map[string]string{v.Args, v.EnvRepos[v.envRepo(env)]} {
		for name, value := range layer {
			if _, ok := env.SBX.Args[name]; ok {
				out[name] = value
			}
		}
	}
	maps.Copy(out, v.Envs[env.Name])
	return out
}

// ResourcesFor are the resources of a sandbox of an env: the person's for
// the sandbox, then for the env, then for every env of its repository,
// then for every env, then the team's, each taking what the one before
// leaves unset.
func (v EnvValues) ResourcesFor(env Env, sandbox string) Resources {
	return v.SandboxResources[sandbox].Over(v.EnvResources[env.Name].Over(v.EnvRepoResources[v.envRepo(env)].Over(v.Resources.Over(env.Resources))))
}

// Where a sandbox's resource comes from, the person's first.
const (
	ResourceOfSandbox = "sandbox"
	ResourceOfEnv     = "env"
	ResourceOfEnvRepo = "env-repo"
	ResourceOfAll     = "all"
	ResourceOfTeam    = "team"
)

// ResourceFrom says which layer of ResourcesFor gives a resource, or ""
// when none does and the env's files decide.
func (v EnvValues) ResourceFrom(env Env, sandbox, name string) string {
	switch {
	case v.SandboxResources[sandbox].Get(name) != "":
		return ResourceOfSandbox
	case v.EnvResources[env.Name].Get(name) != "":
		return ResourceOfEnv
	case v.EnvRepoResources[v.envRepo(env)].Get(name) != "":
		return ResourceOfEnvRepo
	case v.Resources.Get(name) != "":
		return ResourceOfAll
	case env.Resources.Get(name) != "":
		return ResourceOfTeam
	}
	return ""
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

// Where a sandbox's author comes from, the narrowest first.
const (
	AuthorOfSandbox = "sandbox"
	AuthorOfEnv     = "env"
	AuthorOfAll     = "global"
)

// AuthorFor is who a sandbox of an env commits as - its own, else the
// env's, else every env's - and which of them it is; "" with none.
func (v EnvValues) AuthorFor(env Env, sandbox string) (author, from string) {
	switch {
	case v.SandboxAuthors[sandbox] != "":
		return v.SandboxAuthors[sandbox], AuthorOfSandbox
	case v.EnvAuthors[env.Name] != "":
		return v.EnvAuthors[env.Name], AuthorOfEnv
	case v.Author != "":
		return v.Author, AuthorOfAll
	}
	return "", ""
}

// CoAuthorFor says whether agents in an env's sandboxes name themselves
// co-author of their commits - the env's own say, else every env's, else
// they do - and where that comes from: AuthorOfEnv, AuthorOfAll or "".
func (v EnvValues) CoAuthorFor(env string) (on bool, from string) {
	if on, ok := v.EnvCoAuthors[env]; ok {
		return on, AuthorOfEnv
	}
	if v.CoAuthor != nil {
		return *v.CoAuthor, AuthorOfAll
	}
	return true, ""
}

// SetSandboxAuthor makes author, Name <email>, who a sandbox's commits are
// by; "" forgets it.
func (p Paths) SetSandboxAuthor(sandbox, author string) error {
	v, err := p.LoadEnvValues()
	if err != nil {
		return err
	}
	if v.SandboxAuthors[sandbox] == author {
		return nil
	}
	if author == "" {
		delete(v.SandboxAuthors, sandbox)
	} else {
		v.SandboxAuthors[sandbox] = author
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
	if v.Author != "" {
		doc["author"] = v.Author
	}
	if v.CoAuthor != nil {
		doc["co_author"] = *v.CoAuthor
	}
	scopes := func(key string, args map[string]map[string]string, resources map[string]Resources, authors map[string]string, coAuthors map[string]bool) {
		tables := map[string]any{}
		for name, values := range args {
			if len(values) > 0 {
				tables[name] = map[string]any{"args": values}
			}
		}
		for name, r := range resources {
			if r == (Resources{}) {
				continue
			}
			t, _ := tables[name].(map[string]any)
			if t == nil {
				t = map[string]any{}
				tables[name] = t
			}
			t["resources"] = r
		}
		for name, author := range authors {
			if author == "" {
				continue
			}
			t, _ := tables[name].(map[string]any)
			if t == nil {
				t = map[string]any{}
				tables[name] = t
			}
			t["author"] = author
		}
		for name, on := range coAuthors {
			t, _ := tables[name].(map[string]any)
			if t == nil {
				t = map[string]any{}
				tables[name] = t
			}
			t["co_author"] = on
		}
		if len(tables) > 0 {
			doc[key] = tables
		}
	}
	scopes("envs", v.Envs, v.EnvResources, v.EnvAuthors, v.EnvCoAuthors)
	scopes("env_repos", v.EnvRepos, v.EnvRepoResources, nil, nil)
	boxes := map[string]any{}
	for name, r := range v.SandboxResources {
		if r != (Resources{}) {
			boxes[name] = map[string]any{"resources": r}
		}
	}
	for name, author := range v.SandboxAuthors {
		if author == "" {
			continue
		}
		t, _ := boxes[name].(map[string]any)
		if t == nil {
			t = map[string]any{}
			boxes[name] = t
		}
		t["author"] = author
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
