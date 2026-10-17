package unibox

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/tobola/unibox/internal/unibox/api"
)

type Box struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Memory string `json:"memory,omitempty"`
	ID     string `json:"id"`
}
type Image struct {
	Reference string
	ID        string
}
type Spec struct {
	Name, Image string
	// Resources are as the profile says them; the adapter turns a share of
	// the host's into what its runtime takes.
	Resources Resources
	Access    HostAccess
	// Agent is the runtime's own kind for the sandbox. sbx binds its stored
	// service credentials by this kind, so a profile with one agent gets the
	// proxy-managed login; empty means a plain shell sandbox. Agents names
	// every declared agent: several mean a generated v3 workload kit.
	Agent  string
	Agents []string
	// OAuth is the agent the profile signs in by OAuth, if any.
	OAuth string
}
type Command struct {
	Args            []string
	Directory, User string
	Interactive     bool
	Input           io.Reader
	Output, Error   io.Writer
}
type Capabilities struct {
	Isolation       string `json:"isolation"`
	ReadOnlyMounts  bool   `json:"read_only_mounts"`
	NetworkRules    bool   `json:"network_rules"`
	CredentialProxy bool   `json:"credential_proxy"`
}

type Runtime interface {
	Name() string
	Capabilities() Capabilities
	Available(context.Context) error
	List(context.Context) ([]Box, error)
	Images(context.Context) ([]Image, error)
	Build(context.Context, Profile, string, io.Writer) (string, error)
	Create(context.Context, Spec, io.Writer) error
	Exec(context.Context, string, Command) *exec.Cmd
	Stop(context.Context, string) error
	Remove(context.Context, string) error
}

// Authenticator is a runtime that keeps the agents' logins on the host. It
// says which of a profile's sign-ins it does not have yet, and takes one on
// the user's terminal - the runtime asks for the value, never unibox.
type Authenticator interface {
	AuthNeeds(ctx context.Context, sandbox string, p Profile) []api.Need
	Login(ctx context.Context, agent string, a AgentAuth, in io.Reader, out, errout io.Writer) error
}

// SessionEnver is a runtime that gives an agent's session variables of its
// own - placeholders its proxy swaps for a login, never a value.
type SessionEnver interface {
	SessionEnv(p Profile, agent string) []string
}

// Mounter is a runtime that mounts a directory of the host at a path of a
// running sandbox, and takes it out, without a recreation.
type Mounter interface {
	Mount(ctx context.Context, sandbox, host, target string) error
	Unmount(ctx context.Context, sandbox, host, target string) error
}

// Starter is a runtime that does not start a stopped sandbox on its own
// when a command is run in it. msb resolves a sandbox's secrets from the
// environment of the command that boots it, so unibox starts it with them.
type Starter interface {
	Start(ctx context.Context, sandbox string, log io.Writer) error
}

// SecretRefresher is a runtime whose running sandboxes get their tokens
// fetched again before each session, as the runtime does not refresh them
// itself.
type SecretRefresher interface {
	RefreshSecrets(ctx context.Context, sandbox string, log io.Writer) error
}

// Present says whether a runtime is there to use without asking it
// anything: an installed program, or one unibox brings along. Choosing a
// runtime for a new sandbox takes the first present one.
type Present interface {
	Present() bool
}

// ImagePruner is a runtime whose images unibox can list and remove, so old
// ones do not pile up: a sandbox does not need the image it was made from
// once it exists.
type ImagePruner interface {
	UniboxImages(ctx context.Context) ([]RuntimeImage, error)
	RemoveImage(ctx context.Context, ref string) error
}

// RuntimeImage is an image unibox made in a runtime: a build of a profile,
// or one the runtime made for a sandbox (sbx's kit image).
type RuntimeImage struct {
	Ref, Profile, Sandbox string
	Created               time.Time
}

// NetworkUpdater is a runtime that changes a running sandbox's network
// rules in place, so a change of them needs no recreation. It touches the
// rules it is told of and no others.
type NetworkUpdater interface {
	UpdateNetwork(ctx context.Context, sandbox string, change NetworkChange, log io.Writer) error
}

// NetworkChange is the difference between the rules a sandbox has and the
// profile's.
type NetworkChange struct {
	AddAllow, RemoveAllow, AddDeny, RemoveDeny []string
}

func (c NetworkChange) Empty() bool {
	return len(c.AddAllow)+len(c.RemoveAllow)+len(c.AddDeny)+len(c.RemoveDeny) == 0
}

// Child processes receive only the host context they need. In particular, an
// installed runtime must not discover an unrelated token or SSH agent by accident.
func HostEnvironment() []string {
	keys := []string{"PATH", "HOME", "USER", "LOGNAME", "SHELL", "TMPDIR", "TMP", "TEMP", "TERM", "COLORTERM", "LANG", "LC_ALL", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "XDG_RUNTIME_DIR", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM"}
	env := []string{}
	for _, key := range keys {
		if v, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+v)
		}
	}
	return env
}
func hostCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	c := exec.CommandContext(ctx, name, args...)
	c.Env = HostEnvironment()
	return c
}
func commandOutput(c *exec.Cmd) ([]byte, error) {
	var errout bytes.Buffer
	c.Stderr = &errout
	b, err := c.Output()
	if err != nil {
		return nil, fmt.Errorf("%s failed; check its configuration and retry: %s", c.Path, strings.TrimSpace(errout.String()))
	}
	return b, nil
}
func runLogged(c *exec.Cmd, log io.Writer) error {
	c.Stdout = log
	c.Stderr = log
	if err := c.Run(); err != nil {
		return fmt.Errorf("%s failed; see the operation log: %w", c.Path, err)
	}
	return nil
}

// runQuiet keeps a command's chatter to itself and shows it only when the
// command fails, where it is what explains the failure.
func runQuiet(c *exec.Cmd) error {
	var out bytes.Buffer
	c.Stdout = &out
	c.Stderr = &out
	if err := c.Run(); err != nil {
		return fmt.Errorf("%s failed: %s", c.Path, strings.TrimSpace(out.String()))
	}
	return nil
}
func runtimeOutput(ctx context.Context, r Runtime, name string, args ...string) (string, error) {
	b, err := commandOutput(r.Exec(ctx, name, Command{Args: args}))
	return strings.TrimSpace(string(b)), err
}
func Quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
