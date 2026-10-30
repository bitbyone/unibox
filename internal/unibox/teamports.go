package unibox

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// TeamFile is the file of unibox's own beside an sbxenv file, in which a
// team names the ports of the env: dev.sbxenv.yaml has dev.unibox.yaml.
// sbx does not read it, so it limits nobody who uses sbx alone.
func TeamFile(sbxenv string) string {
	dir, base := filepath.Split(sbxenv)
	for _, ext := range []string{".sbxenv.yaml", ".sbxenv.yml", ".yaml", ".yml"} {
		if strings.HasSuffix(base, ext) {
			base = strings.TrimSuffix(base, ext)
			break
		}
	}
	return filepath.Join(dir, base+".unibox.yaml")
}

// RepoFile is the file of unibox's own at the root of the repository an
// sbxenv file is in, unibox.yaml, which names ports for every env of the
// repository; empty when the file is in none.
func RepoFile(sbxenv string) string {
	dir, err := filepath.Abs(filepath.Dir(sbxenv))
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return filepath.Join(dir, "unibox.yaml")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// TeamPort is a port a team names: Spec as a person's port says it, a
// description of what listens there, and whether a sandbox starts with
// it published, off unless the team says on. From is where it is named:
// FromRepo for the repository's unibox.yaml, FromEnv for the env's own
// file.
type TeamPort struct {
	Spec        string
	Description string
	Default     bool
	From        string
}

// Where a port is named, from the team's repository to the person's env.
const (
	FromRepo         = "repo"
	FromEnv          = "env"
	FromYoursAll     = "yours-all"
	FromYoursEnvRepo = "yours-env-repo"
	FromYours        = "yours"
)

// IsYours says whether a layer is the person's config.yaml rather than a
// team's file.
func IsYours(from string) bool {
	return from == FromYoursAll || from == FromYoursEnvRepo || from == FromYours
}

// yourLayer is one scope of the person's config.yaml as an env gets it.
type yourLayer struct {
	from string
	EnvRepoConfig
}

// yourLayers are the scopes of the person's config.yaml an env gets, from
// the widest: every env, every env of the repository its files are in,
// and the env as ec says it.
func (c Config) yourLayers(ec EnvConfig) []yourLayer {
	var repo EnvRepoConfig
	// Finding an env's repository may ask git, so it is looked for only
	// when the person says something for one.
	if len(c.EnvRepos) > 0 {
		if key := EnvRepoKey(ec); key != "" {
			repo = c.EnvRepos[key]
		}
	}
	return []yourLayer{
		{FromYoursAll, EnvRepoConfig{Ports: c.Ports, Repos: c.Repos, Instructions: c.Instructions}},
		{FromYoursEnvRepo, repo},
		{FromYours, EnvRepoConfig{Ports: ec.Ports, Repos: ec.Repos, Instructions: ec.Instructions}},
	}
}

func (p *TeamPort) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: a team's port is {port: 5173, description: ..., default: off}", n.Line)
	}
	*p = TeamPort{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if v.Kind != yaml.ScalarNode {
			return fmt.Errorf("line %d: %s is one value", v.Line, k.Value)
		}
		switch k.Value {
		case "port":
			p.Spec = v.Value
		case "description":
			p.Description = v.Value
		case "default":
			on, ok := onOff(v.Value)
			if !ok {
				return fmt.Errorf("line %d: a port's default is on or off", v.Line)
			}
			p.Default = on
		default:
			return fmt.Errorf("line %d: a team's port says port, description and default, not %s", k.Line, k.Value)
		}
	}
	if p.Spec == "" {
		return fmt.Errorf("line %d: the port says no port, such as port: 5173", n.Line)
	}
	return nil
}

// teamDoc is what a team's unibox.yaml or <env>.unibox.yaml says.
type teamDoc struct {
	Ports     map[string]TeamPort    `yaml:"ports"`
	Secrets   map[string]TokenGuide  `yaml:"secrets"`
	Resources Resources              `yaml:"resources"`
	Repos     map[string]IndexedRepo `yaml:"repos"`
	// Instructions are what every task of the env's agents are told.
	Instructions string `yaml:"instructions"`
	// Author says whether each person gives the env a commit author of
	// their own: required, else optional, as when it is left out.
	Author string `yaml:"author"`
}

// readTeamFiles reads the team's files of an env in the order they apply:
// the repository's unibox.yaml, then the file beside each of the env's
// sbxenv files, as sbx merges the files themselves. A file that is not
// there says nothing.
func readTeamFiles(sbxenvFiles []string, each func(name, from string, doc teamDoc) error) error {
	read := func(name, from string) error {
		b, err := os.ReadFile(name)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		var doc teamDoc
		d := yaml.NewDecoder(bytes.NewReader(b))
		d.KnownFields(true)
		if err := d.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("%s: %w", name, err)
		}
		return each(name, from, doc)
	}
	repos := map[string]bool{}
	for _, f := range sbxenvFiles {
		if r := RepoFile(f); r != "" && !repos[r] {
			repos[r] = true
			if err := read(r, FromRepo); err != nil {
				return err
			}
		}
	}
	for _, f := range sbxenvFiles {
		if err := read(TeamFile(f), FromEnv); err != nil {
			return err
		}
	}
	return nil
}

// ReadTeamPorts reads the ports the team names for an env, a later file's
// over an earlier one's.
func ReadTeamPorts(sbxenvFiles []string) (map[string]TeamPort, error) {
	out := map[string]TeamPort{}
	err := readTeamFiles(sbxenvFiles, func(name, from string, doc teamDoc) error {
		for k, v := range doc.Ports {
			if !portName.MatchString(k) {
				return fmt.Errorf("%s: port name %q must be lower case letters, digits and dashes, starting with a letter", name, k)
			}
			if _, err := ParsePortSpec(v.Spec); err != nil {
				return fmt.Errorf("%s: port %s: %w", name, k, err)
			}
			if t, ok := out[k]; ok {
				v.From = t.From
				if v.Description == "" {
					v.Description = t.Description
				}
			} else {
				v.From = from
			}
			out[k] = v
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// envPorts are an env's ports as a sandbox gets them, each layer over the
// one before: the repository's, the env's file's, then the person's for
// every env, for every env of its repository and for this one. A person's
// port with no spec only switches a port of its name on or off; one said
// for more envs than this that this env does not have is left out, as
// other envs may.
func envPorts(ec EnvConfig, c Config) (map[string]Port, error) {
	team, err := ReadTeamPorts(ec.Files)
	if err != nil {
		return nil, err
	}
	out := map[string]Port{}
	for k, v := range team {
		out[k] = Port{Spec: v.Spec, Default: v.Default, Description: v.Description, Team: true, From: v.From, DefaultFrom: v.From}
	}
	for _, layer := range c.yourLayers(ec) {
		for k, v := range layer.Ports {
			p, known := out[k]
			if v.Spec == "" && !known {
				if layer.from != FromYours {
					continue
				}
				return nil, fmt.Errorf("port %s says only whether it is on, and the env names no such port; take it out of the env's ports: in your config.yaml", k)
			}
			if !known {
				p.From = layer.from
			}
			if v.Spec != "" {
				p.Spec = v.Spec
			}
			p.Default, p.DefaultFrom = v.Default, layer.from
			out[k] = p
		}
	}
	if err := validatePorts(out); err != nil {
		return nil, err
	}
	return out, nil
}
