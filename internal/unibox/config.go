// Package unibox owns sandbox environments independently of unagit.
package unibox

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/tobola/unibox/internal/unibox/api"
	"gopkg.in/yaml.v3"
)

var identifier = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

func ValidName(s string) error {
	if !identifier.MatchString(s) {
		return fmt.Errorf("invalid name %q; use lowercase letters, digits and hyphens (up to 63 characters)", s)
	}
	return nil
}

// legacyRuntime is how unibox 1 kept a runtime's settings; only sbx's
// carry over.
type legacyRuntime struct {
	On       *bool `yaml:"on,omitempty"`
	StopIdle *bool `yaml:"stop_idle,omitempty"`
	SSHAgent *bool `yaml:"ssh_agent,omitempty"`
}

type Config struct {
	// DefaultEnv is the env a sandbox is created from when none is named.
	DefaultEnv string               `yaml:"default_env,omitempty" json:"default_env,omitempty"`
	Envs       map[string]EnvConfig `yaml:"envs,omitempty" json:"envs,omitempty"`
	// Context is the sandbox and env a command works on when it is not
	// told, set by unibox context use.
	Context Context `yaml:"context,omitempty" json:"context,omitempty"`
	// StopIdleSandbox stops a sandbox when its last session ends; unset
	// means yes.
	StopIdleSandbox *bool `yaml:"stop_idle,omitempty" json:"stop_idle,omitempty"`
	// SSHAgentOn says the user lets sandboxes use their SSH agent: git
	// over SSH needs no token then, but the agent signs with every key the
	// user has. Unset, sbx's forwarding is refused.
	SSHAgentOn *bool `yaml:"ssh_agent,omitempty" json:"ssh_agent,omitempty"`

	// What unibox 1 kept here is read so an old file still loads, and
	// dropped when the file is next written.
	Environments   string                   `yaml:"environments,omitempty" json:"-"`
	DefaultRuntime string                   `yaml:"default_runtime,omitempty" json:"-"`
	Runtimes       map[string]legacyRuntime `yaml:"runtimes,omitempty" json:"-"`
}

type Context struct {
	Sandbox string `yaml:"sandbox,omitempty" json:"sandbox,omitempty"`
	Env     string `yaml:"env,omitempty" json:"env,omitempty"`
	// Profile is unibox 1's, read and dropped.
	Profile string `yaml:"profile,omitempty" json:"-"`
}
type Paths struct{ Config, Cache string }

func DefaultPaths() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}
	config := os.Getenv("XDG_CONFIG_HOME")
	if config == "" {
		config = filepath.Join(home, ".config")
	}
	cache := os.Getenv("XDG_CACHE_HOME")
	if cache == "" {
		cache = filepath.Join(home, ".cache")
	}
	return Paths{filepath.Join(config, "unibox"), filepath.Join(cache, "unibox")}, nil
}

func ExpandPath(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		if strings.HasSuffix(path, "/~") {
			path = home
		}
	}
	return filepath.Abs(path)
}

func DecodeYAML(data []byte, v any) error {
	d := yaml.NewDecoder(bytes.NewReader(data))
	d.KnownFields(true)
	if err := d.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		if err != nil {
			return err
		}
		return fmt.Errorf("multiple yaml documents are not supported; use one document")
	}
	return nil
}

func (p Paths) LoadConfig() (Config, error) {
	c := Config{}
	b, err := os.ReadFile(filepath.Join(p.Config, "config.yaml"))
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err = DecodeYAML(b, &c); err != nil {
		return c, fmt.Errorf("invalid unibox configuration: %w", err)
	}
	if r, ok := c.Runtimes["sbx"]; ok {
		if c.StopIdleSandbox == nil {
			c.StopIdleSandbox = r.StopIdle
		}
		if c.SSHAgentOn == nil {
			c.SSHAgentOn = r.SSHAgent
		}
	}
	c.Environments, c.DefaultRuntime, c.Runtimes, c.Context.Profile = "", "", nil, ""
	return c, nil
}

func AtomicWrite(path string, b []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".unibox-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, path)
}

func (p Paths) SaveConfig(c Config) error {
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return AtomicWrite(filepath.Join(p.Config, "config.yaml"), b, 0o600)
}

// StateVersion is the format of state.json; unibox 1 wrote version 1,
// whose sandboxes were made from profiles and are no longer unibox's.
const StateVersion = 2

type State struct {
	Version   int                    `json:"version"`
	Sandboxes map[string]api.Sandbox `json:"sandboxes"`
	// Mounted are the host directories unibox has mounted into a sandbox,
	// each told once what that lets the agent do.
	Mounted []string `json:"mounted,omitempty"`
}

func freshState() State {
	return State{Version: StateVersion, Sandboxes: map[string]api.Sandbox{}}
}

func (p Paths) statePath() string { return filepath.Join(p.Config, "state.json") }

// retireState puts aside a state of unibox 1, whose sandboxes the new
// format cannot describe, and says which sandboxes it leaves to the user.
func (p Paths) retireState(log io.Writer) error {
	b, err := os.ReadFile(p.statePath())
	if err != nil {
		return nil
	}
	var old struct {
		Version   int `json:"version"`
		Sandboxes map[string]struct {
			RuntimeName string `json:"runtime_name"`
		} `json:"sandboxes"`
	}
	if json.Unmarshal(b, &old) != nil || old.Version != 1 {
		return nil
	}
	l, err := LockFile(filepath.Join(p.Config, "state.lock"), false, true)
	if err != nil {
		return err
	}
	defer l.Close()
	kept := filepath.Join(p.Config, "state.v1.json")
	if err := os.Rename(p.statePath(), kept); err != nil {
		return err
	}
	names := []string{}
	for _, b := range old.Sandboxes {
		names = append(names, b.RuntimeName)
	}
	sort.Strings(names)
	if len(names) > 0 {
		fmt.Fprintf(log, "unibox 2 no longer manages the sandboxes of unibox 1 (%s); remove them with sbx rm when their work is safe. The old state is kept in %s\n", strings.Join(names, ", "), kept)
	}
	return nil
}

func (p Paths) State(fn func(*State) error) error {
	l, err := LockFile(filepath.Join(p.Config, "state.lock"), false, true)
	if err != nil {
		return err
	}
	defer l.Close()
	s := freshState()
	path := p.statePath()
	b, err := os.ReadFile(path)
	if err == nil {
		err = json.Unmarshal(b, &s)
	}
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot read unibox state; preserve %s and repair it: %w", path, err)
	}
	if s.Sandboxes == nil {
		s.Sandboxes = map[string]api.Sandbox{}
	}
	if s.Version != StateVersion {
		return fmt.Errorf("unsupported unibox state version %d; upgrade unibox", s.Version)
	}
	if err = fn(&s); err != nil {
		return err
	}
	b, err = json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return AtomicWrite(path, append(b, '\n'), 0o600)
}

func (p Paths) ReadState() (State, error) {
	var out State
	err := p.State(func(s *State) error { out = *s; return nil })
	return out, err
}

// SSHAgent says whether the user lets sandboxes use their SSH agent, and
// whether they said so at all.
func (c Config) SSHAgent() (on, said bool) {
	if c.SSHAgentOn == nil {
		return false, false
	}
	return *c.SSHAgentOn, true
}

func (c Config) StopIdle() bool { return c.StopIdleSandbox == nil || *c.StopIdleSandbox }

type Failure struct {
	Status        int
	Code, Message string
}

func (e *Failure) Error() string { return e.Message }
func Fail(status int, code, format string, args ...any) error {
	return &Failure{status, code, fmt.Sprintf(format, args...)}
}
