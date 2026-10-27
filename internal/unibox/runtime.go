package unibox

import (
	"bytes"
	"context"
	"fmt"
	"github.com/tobola/unibox/internal/unibox/api"
	"io"
	"os"
	"os/exec"
	"runtime"
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
	SetSetting(ctx context.Context, key, value string) error
	Setting(ctx context.Context, key string) (string, error)
	Diagnose(ctx context.Context) ([]RuntimeCheck, error)
	// SignedIn says whether the runtime is signed in to its account, and
	// Login signs it in on the user's terminal.
	SignedIn(ctx context.Context) (bool, error)
	Login(ctx context.Context, in io.Reader, out, errout io.Writer) error
	// NetworkPolicyChosen says whether the global network policy every
	// sandbox starts from has been chosen, and ChooseNetworkPolicy
	// chooses it, once.
	NetworkPolicyChosen(ctx context.Context) (bool, error)
	ChooseNetworkPolicy(ctx context.Context, policy string) error
	// NetworkRules are the network rules for sandbox, or all of them;
	// SetNetworkRule allows or denies hosts to sandbox, or to every one,
	// RemoveNetworkRule takes such a rule away, and NetworkBlocked are
	// the hosts the policy refused the sandbox lately.
	NetworkRules(ctx context.Context, sandbox string) ([]NetworkRule, error)
	SetNetworkRule(ctx context.Context, decision, sandbox, protocol string, hosts []string) error
	RemoveNetworkRule(ctx context.Context, sandbox string, hosts []string) error
	NetworkBlocked(ctx context.Context, sandbox string) ([]BlockedHost, error)
	SSHForwarding(ctx context.Context) (bool, error)
	SetSSHForwarding(ctx context.Context, on bool) error
	// EditorSSH says whether the runtime serves SSH to editors, and
	// SetEditorSSH turns it on, with the host entry an SSH client needs,
	// or off and the entry out.
	EditorSSH(ctx context.Context) (bool, error)
	SetEditorSSH(ctx context.Context, on bool) error
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
	if runtime.GOOS == "windows" {
		// Without these a program on Windows finds neither its own files
		// nor the system's, and cannot start another.
		keys = append(keys, "SYSTEMROOT", "SYSTEMDRIVE", "WINDIR", "COMSPEC", "PATHEXT", "USERPROFILE", "USERNAME", "HOMEDRIVE", "HOMEPATH", "APPDATA", "LOCALAPPDATA", "PROGRAMDATA", "PROGRAMFILES", "PROGRAMFILES(X86)", "COMMONPROGRAMFILES")
	}
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
		// What the command printed goes back with the failure: sbx diagnose
		// exits non-zero whenever a check fails, and its report is the answer.
		return b, fmt.Errorf("%s failed; check its configuration and retry: %s", c.Path, strings.TrimSpace(errout.String()))
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
