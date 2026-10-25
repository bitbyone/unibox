package unibox

import (
	"fmt"
	"slices"
)

// EnvConfig is an env as the user keeps it in config.yaml: the sbxenv files
// sbx merges in this order, the name its sandbox gets, and the user's own
// ports, which are the machine's and never the team's.
type EnvConfig struct {
	Files []string        `yaml:"files" json:"files"`
	Name  string          `yaml:"name,omitempty" json:"name,omitempty"`
	Ports map[string]Port `yaml:"ports,omitempty" json:"ports,omitempty"`
	// Repo is the git repository the files are in, as init found them.
	Repo string `yaml:"repo,omitempty" json:"repo,omitempty"`
	// Source is the address unibox cloned Repo from; a repository of the
	// user's own has none, and unibox never pulls it.
	Source string `yaml:"source,omitempty" json:"source,omitempty"`
}

// Env is an env with what its files say.
type Env struct {
	Name   string
	Config EnvConfig
	SBX    SBXEnv
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
	return Env{Name: name, Config: ec, SBX: sbx}, nil
}

// ValidEnvConfig checks what an env says before it is kept: a name, files
// that read and ports that parse.
func ValidEnvConfig(name string, ec EnvConfig) error {
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
	return validatePorts(ec.Ports)
}
