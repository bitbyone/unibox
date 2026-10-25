package unibox

import (
	"bytes"
	"context"
	"fmt"
	"github.com/tobola/unibox/internal/unibox/api"
	"io"
	"os"
	"os/exec"
	"strings"
)

type Box struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Memory string `json:"memory,omitempty"`
	ID     string `json:"id"`
}

// Runtime is what unibox asks of sbx. SBX is the one implementation; the
// tests put a stand-in in its place.
type Runtime interface {
	Present() bool
	Available(ctx context.Context) error
	List(ctx context.Context) ([]Box, error)
	Exec(ctx context.Context, name string, c Command) *exec.Cmd
	Stop(ctx context.Context, name string) error
	Remove(ctx context.Context, name string) error
	EnvCreate(ctx context.Context, name, args string, files []string, in io.Reader, out, errout io.Writer) error
	EnvRemove(ctx context.Context, name, args string, files []string, log io.Writer) error
	KitInspect(ctx context.Context, ref string, args map[string]string) (Kit, error)
	Secrets(ctx context.Context) ([]Secret, error)
	SecretSet(ctx context.Context, service, command string, in io.Reader, out, errout io.Writer) error
	Mount(ctx context.Context, sandbox, host, target string) error
	Unmount(ctx context.Context, sandbox, host, target string) error
	PublishedPorts(ctx context.Context, sandbox string) ([]api.PublishedPort, error)
	PublishPort(ctx context.Context, sandbox, spec string, log io.Writer) error
	UnpublishPort(ctx context.Context, sandbox string, p api.PublishedPort, log io.Writer) error
	Version(ctx context.Context) (string, error)
	EnvPlan(ctx context.Context, name, args string, files []string) (string, error)
	Setting(ctx context.Context, key string) (string, error)
	Diagnose(ctx context.Context) ([]RuntimeCheck, error)
	SSHForwarding(ctx context.Context) (bool, error)
	SetSSHForwarding(ctx context.Context, on bool) error
	// AllowSSHAgent records the user's leave for sandboxes to use their
	// SSH agent, which Available holds sbx's forwarding to.
	AllowSSHAgent(on bool)
}

type Command struct {
	Args            []string
	Directory, User string
	Interactive     bool
	Input           io.Reader
	Output, Error   io.Writer
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
func guestOutput(ctx context.Context, r Runtime, name string, args ...string) (string, error) {
	b, err := commandOutput(r.Exec(ctx, name, Command{Args: args}))
	return strings.TrimSpace(string(b)), err
}
func Quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
