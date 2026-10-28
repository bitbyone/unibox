package unibox

import (
	"fmt"
	"slices"
	"strings"
)

// EnvConfig is an env as the user keeps it in config.yaml: the sbxenv files
// sbx merges in this order, the name its sandbox gets, and the user's own
// ports, which are the machine's and never the team's.
type EnvConfig struct {
	Files []string        `yaml:"files" json:"files"`
	Name  string          `yaml:"name,omitempty" json:"name,omitempty"`
	Ports map[string]Port `yaml:"ports,omitempty" json:"ports,omitempty"`
	// Repos are the person's repositories for this env, over the team's.
	Repos map[string]IndexedRepo `yaml:"repos,omitempty" json:"repos,omitempty"`
	// Instructions are the person's for this env's tasks' agents.
	Instructions string `yaml:"instructions,omitempty" json:"instructions,omitempty"`
	// Repo is the git repository the files are in, as init found them.
	Repo string `yaml:"repo,omitempty" json:"repo,omitempty"`
	// Source is the address unibox cloned Repo from; a repository of the
	// user's own has none, and is pulled only while it is clean.
	Source string `yaml:"source,omitempty" json:"source,omitempty"`
}

// Env is an env with what its files say.
type Env struct {
	Name   string
	Config EnvConfig
	SBX    SBXEnv
	// Ports are the team's and the person's, as a sandbox gets them.
	Ports map[string]Port
	// Resources are the team's, which the person's in vars.toml win over.
	Resources Resources
	// Repos are the repositories a task of the env may be given by name,
	// the team's and the person's.
	Repos map[string]IndexedRepo
	// Instructions are what the env's tasks' agents are told, the team's
	// files' first, then the person's from the widest scope to the env.
	Instructions []string
}

// SandboxName is the name of the env's sandbox: the user's, else the
// file's, else the env's own.
func (e Env) SandboxName() string {
	if e.Config.Name != "" {
		return e.Config.Name
	}
	return e.SBX.SandboxName(e.Name)
}

// LoadEnv reads an env of the configuration and its files.
func (c Config) LoadEnv(name string) (Env, error) {
	ec, ok := c.Envs[name]
	if !ok {
		known := sortedKeys(c.Envs)
		if len(known) == 0 {
			return Env{}, Fail(4, "env_not_found", "there is no env %s; add one with unibox env add %s <file.sbxenv.yaml>", name, name)
		}
		return Env{}, Fail(4, "env_not_found", "there is no env %s; the envs are %v, unibox env add adds one", name, known)
	}
	sbx, err := ReadSBXEnv(ec.Files)
	if err != nil {
		return Env{}, fmt.Errorf("env %s: %w", name, err)
	}
	ports, err := envPorts(ec, c)
	if err != nil {
		return Env{}, fmt.Errorf("env %s: %w", name, err)
	}
	resources, err := ReadTeamResources(ec.Files)
	if err != nil {
		return Env{}, fmt.Errorf("env %s: %w", name, err)
	}
	repos, err := envRepos(ec, c)
	if err != nil {
		return Env{}, fmt.Errorf("env %s: %w", name, err)
	}
	instructions := []string{}
	if err := readTeamFiles(ec.Files, func(_, _ string, doc teamDoc) error {
		instructions = append(instructions, doc.Instructions)
		return nil
	}); err != nil {
		return Env{}, fmt.Errorf("env %s: %w", name, err)
	}
	for _, layer := range c.yourLayers(ec) {
		instructions = append(instructions, layer.Instructions)
	}
	instructions = slices.DeleteFunc(instructions, func(s string) bool { return strings.TrimSpace(s) == "" })
	return Env{Name: name, Config: ec, SBX: sbx, Ports: ports, Resources: resources, Repos: repos, Instructions: instructions}, nil
}

// ValidEnvConfig checks what an env says before it is kept: a name, files
// that read and ports that parse under what the person says for more envs
// than this one in c.
func ValidEnvConfig(name string, ec EnvConfig, c Config) error {
	if err := ValidName(name); err != nil {
		return err
	}
	if ec.Name != "" {
		if err := ValidName(ec.Name); err != nil {
			return fmt.Errorf("sandbox name: %w", err)
		}
	}
	if len(ec.Files) == 0 {
		return fmt.Errorf("env %s needs at least one sbxenv file", name)
	}
	if slices.Contains(ec.Files, "") {
		return fmt.Errorf("env %s names an empty file", name)
	}
	if _, err := ReadSBXEnv(ec.Files); err != nil {
		return err
	}
	_, err := envPorts(ec, c)
	return err
}
