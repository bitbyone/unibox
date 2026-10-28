package unibox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tobola/unibox/internal/unibox/api"
)

// SBX runs the sbx command line, the one runtime unibox works with.
type SBX struct {
	// SSHAgent is the user's leave for sandboxes to use their SSH agent,
	// which sbx forwards when its setting says so.
	SSHAgent bool
}

// SSHForwarding says whether sbx forwards the host's SSH agent now.
func (r *SBX) SSHForwarding(ctx context.Context) (bool, error) { return sbxForwarding(ctx) }

// SetSetting overrides one of sbx's settings, or with value "" takes the
// override away, back to sbx's default.
func (r *SBX) SetSetting(ctx context.Context, key, value string) error {
	args := []string{"settings", "set", key, value}
	if value == "" {
		args = []string{"settings", "unset", key}
	}
	if err := runQuiet(hostCommand(ctx, "sbx", args...)); err != nil {
		return fmt.Errorf("sbx did not change %s: %w", key, err)
	}
	return nil
}

// SetSSHForwarding turns sbx's forwarding of the host's SSH agent on or
// off, and restarts its daemon, which a change takes effect with; running
// sandboxes keep the connections they have until they restart.
func (r *SBX) SetSSHForwarding(ctx context.Context, on bool) error {
	if err := runQuiet(hostCommand(ctx, "sbx", "settings", "set", "ssh.agentForwardingEnabled", strconv.FormatBool(on))); err != nil {
		return fmt.Errorf("sbx did not change ssh.agentForwardingEnabled: %w", err)
	}
	if err := runQuiet(hostCommand(ctx, "sbx", "daemon", "restart")); err != nil {
		return fmt.Errorf("sbx changed ssh.agentForwardingEnabled, but its daemon did not restart; run sbx daemon restart: %w", err)
	}
	return nil
}

// EditorSSH reads sbx's feature.ssh, a flag whose value is {enabled}, or
// a bare true or false where it is set as one.
func (r *SBX) EditorSSH(ctx context.Context) (bool, error) {
	b, err := commandOutput(hostCommand(ctx, "sbx", "settings", "get", "feature.ssh", "--json"))
	if err != nil {
		return false, err
	}
	var setting struct {
		Value json.RawMessage `json:"value"`
	}
	if i := bytes.IndexByte(b, '{'); i >= 0 {
		b = b[i:]
	}
	if err := json.Unmarshal(b, &setting); err != nil {
		return false, fmt.Errorf("cannot read sbx settings get feature.ssh --json; update sbx or retry: %w", err)
	}
	var on bool
	if json.Unmarshal(setting.Value, &on) == nil {
		return on, nil
	}
	var flag struct{ Enabled bool }
	if err := json.Unmarshal(setting.Value, &flag); err != nil {
		return false, fmt.Errorf("cannot read sbx's feature.ssh, which is %s; update sbx or retry", setting.Value)
	}
	return flag.Enabled, nil
}

// SetEditorSSH turns sbx's SSH endpoint on or off. sbx serves it only from
// a daemon started with the flag on, so the daemon restarts either way;
// sbx setup ssh then adds its host entry to the SSH client's
// configuration, which sbx setup ssh remove takes out first when it goes
// off.
func (r *SBX) SetEditorSSH(ctx context.Context, on bool) error {
	if !on {
		if err := runQuiet(hostCommand(ctx, "sbx", "setup", "ssh", "remove")); err != nil {
			return fmt.Errorf("sbx did not take its host entry out of your SSH client's configuration: %w", err)
		}
	}
	if err := runQuiet(hostCommand(ctx, "sbx", "settings", "set", "feature.ssh", strconv.FormatBool(on))); err != nil {
		return fmt.Errorf("sbx did not change feature.ssh: %w", err)
	}
	if err := runQuiet(hostCommand(ctx, "sbx", "daemon", "restart")); err != nil {
		return fmt.Errorf("sbx changed feature.ssh, but its daemon did not restart; unibox config set editor_ssh %t tries again: %w", on, err)
	}
	if on {
		if err := runQuiet(hostCommand(ctx, "sbx", "setup", "ssh")); err != nil {
			return fmt.Errorf("sbx serves SSH, but did not add its host entry to your SSH client's configuration; unibox config set editor_ssh true tries again: %w", err)
		}
	}
	return nil
}

// sbxForwarding reads whether sbx forwards the host's SSH agent. It takes
// the JSON from its first brace, past any line sbx says before it, and asks
// once more if the answer does not read - a daemon starting can answer
// oddly once - naming what sbx answered if it still does not.
func sbxForwarding(ctx context.Context) (bool, error) {
	var answer []byte
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Second)
		}
		b, err := commandOutput(hostCommand(ctx, "sbx", "settings", "get", "ssh.agentForwardingEnabled", "--json"))
		if err != nil {
			return false, err
		}
		answer = b
		var setting struct {
			Value *bool `json:"value"`
		}
		if i := bytes.IndexByte(b, '{'); i >= 0 && json.Unmarshal(b[i:], &setting) == nil && setting.Value != nil {
			return *setting.Value, nil
		}
	}
	said := strings.TrimSpace(string(answer))
	if len(said) > 300 {
		said = said[:300] + "..."
	}
	return false, fmt.Errorf("cannot determine sbx SSH forwarding from sbx settings get ssh.agentForwardingEnabled --json, which answered %q; update sbx or retry", said)
}

func (r *SBX) AllowSSHAgent(on bool) { r.SSHAgent = on }

func (r *SBX) Present() bool {
	_, err := exec.LookPath("sbx")
	return err == nil
}

func (r *SBX) Available(ctx context.Context) error {
	if _, err := exec.LookPath("sbx"); err != nil {
		return fmt.Errorf("sbx is not installed; install Docker Sandboxes, then unibox init signs you in")
	}
	forwarding, err := sbxForwarding(ctx)
	if err != nil {
		return err
	}
	if forwarding && !r.SSHAgent {
		return Fail(4, "host_access", "sbx forwards your SSH agent, which lets agents sign with every SSH key you have; unibox config set ssh_agent false turns it off, or true allows it")
	}
	_, err = r.List(ctx)
	return err
}
func (r *SBX) List(ctx context.Context) ([]Box, error) {
	b, err := commandOutput(hostCommand(ctx, "sbx", "ls", "--json"))
	if err != nil {
		return nil, err
	}
	sandboxes, err := sbxList[struct{ Name, Status, Memory, ID string }](b, "sandboxes")
	if err != nil {
		return nil, fmt.Errorf("cannot parse sbx sandbox list; update sbx: %w", err)
	}
	boxes := []Box{}
	for _, v := range sandboxes {
		boxes = append(boxes, Box{v.Name, v.Status, v.Memory, v.ID})
	}
	return boxes, nil
}
func (r *SBX) Exec(ctx context.Context, name string, c Command) *exec.Cmd {
	args := []string{"exec"}
	if c.Interactive {
		args = append(args, "-it")
	} else if c.Input != nil {
		args = append(args, "-i")
	}
	if c.Directory != "" {
		args = append(args, "-w", c.Directory)
	}
	if c.User != "" {
		args = append(args, "--user", c.User)
	}
	args = append(args, name)
	args = append(args, c.Args...)
	cmd := hostCommand(ctx, "sbx", args...)
	cmd.Stdin = c.Input
	cmd.Stdout = c.Output
	cmd.Stderr = c.Error
	return cmd
}
func (r *SBX) Stop(ctx context.Context, name string) error {
	_, err := commandOutput(hostCommand(ctx, "sbx", "stop", name))
	return err
}

// EnvCreate creates a sandbox from sbxenv files with sbx env create, on the
// user's terminal: sbx asks there what it needs to, and runs the files'
// host commands. args is the file of the person's args, or empty.
func (r *SBX) EnvCreate(ctx context.Context, name, args string, files []string, in io.Reader, out, errout io.Writer) error {
	cmd := hostCommand(ctx, "sbx", envArgs("create", name, args, files)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = in, out, errout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sbx env create did not create sandbox %s; see what it said above, then retry: %w", name, err)
	}
	return nil
}

// EnvRemove removes a sandbox made from sbxenv files, with what sbx made
// for it alone: its sandbox-scoped secrets and the files' host-side work.
func (r *SBX) EnvRemove(ctx context.Context, name, args string, files []string, log io.Writer) error {
	return runLogged(hostCommand(ctx, "sbx", append(envArgs("rm", name, args, files), "--force")...), log)
}

func envArgs(verb, name, args string, files []string) []string {
	out := []string{"env", verb, "--name", name}
	if args != "" {
		out = append(out, "--env-args-file", args)
	}
	return append(out, files...)
}

// MinSBXVersion is the oldest sbx unibox was checked against: sbxenv
// files, args files, kit args and mounts as unibox uses them.
const MinSBXVersion = "0.49.2"

var semver = regexp.MustCompile(`v?(\d+)\.(\d+)\.(\d+)`)

// Version is the release of sbx, as sbx version prints it.
func (r *SBX) Version(ctx context.Context) (string, error) {
	b, err := commandOutput(hostCommand(ctx, "sbx", "version"))
	if err != nil {
		return "", err
	}
	m := semver.FindStringSubmatch(string(b))
	if m == nil {
		return "", fmt.Errorf("sbx version printed no version unibox can read; update unibox")
	}
	return strings.Join(m[1:], "."), nil
}

// VersionAtLeast compares two releases given as major.minor.patch.
func VersionAtLeast(version, min string) bool {
	a, b := semver.FindStringSubmatch(version), semver.FindStringSubmatch(min)
	if a == nil || b == nil {
		return false
	}
	for i := 1; i <= 3; i++ {
		x, _ := strconv.Atoi(a[i])
		y, _ := strconv.Atoi(b[i])
		if x != y {
			return x > y
		}
	}
	return true
}

// EnvPlan is sbx env plan: it checks an env's files and args and prints
// what creating or updating the sandbox would do, changing nothing.
func (r *SBX) EnvPlan(ctx context.Context, name, args string, files []string) (string, error) {
	b, err := hostCommand(ctx, "sbx", envArgs("plan", name, args, files)...).CombinedOutput()
	if err != nil {
		return string(b), fmt.Errorf("sbx env plan failed: %w", err)
	}
	return string(b), nil
}

// Setting is the value of one of sbx's settings, as JSON.
func (r *SBX) Setting(ctx context.Context, key string) (string, error) {
	b, err := commandOutput(hostCommand(ctx, "sbx", "settings", "get", key, "--json"))
	if err != nil {
		return "", err
	}
	var setting struct {
		Value json.RawMessage `json:"value"`
	}
	if i := bytes.IndexByte(b, '{'); i < 0 || json.Unmarshal(b[i:], &setting) != nil {
		return "", fmt.Errorf("sbx settings get %s answered in a form unibox does not know; update unibox", key)
	}
	return string(setting.Value), nil
}

// Kit is what sbx kit inspect says of a kit that unibox reads.
type Kit struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Sandbox struct {
		Entrypoint []string `json:"entrypoint"`
		Command    struct {
			Default []string `json:"default"`
		} `json:"command"`
	} `json:"sandbox"`
	Credentials []KitCredential `json:"credentials"`
}

// KitCredential is a service a kit takes a token for, which the person
// stores with sbx secret set.
type KitCredential struct {
	Service     string `json:"service"`
	Description string `json:"description"`
	Optional    bool   `json:"optional"`
	// APIKey says the hosts the proxy puts the token into requests for.
	APIKey struct {
		Inject []struct {
			Domain string `json:"domain"`
		} `json:"inject"`
	} `json:"apiKey"`
}

// Domains are the hosts the kit has the token sent to.
func (c KitCredential) Domains() []string {
	out := []string{}
	for _, in := range c.APIKey.Inject {
		if in.Domain != "" && !slices.Contains(out, in.Domain) {
			out = append(out, in.Domain)
		}
	}
	return out
}

// KitInspect reads a kit as sbx resolves it: a registry reference or a
// local directory. sbx says what it cached before the JSON.
func (r *SBX) KitInspect(ctx context.Context, ref string, args map[string]string) (Kit, error) {
	var k Kit
	argv := []string{"kit", "inspect", "--json"}
	for _, name := range sortedKeys(args) {
		argv = append(argv, "--kit-arg", name+"="+args[name])
	}
	b, err := commandOutput(hostCommand(ctx, "sbx", append(argv, ref)...))
	if err != nil {
		return k, fmt.Errorf("cannot read kit %s: %w", ref, err)
	}
	if i := bytes.IndexByte(b, '{'); i > 0 {
		b = b[i:]
	}
	if err := json.Unmarshal(b, &k); err != nil {
		return k, fmt.Errorf("sbx kit inspect answered for %s in a form unibox does not know; update unibox: %w", ref, err)
	}
	return k, nil
}

// Secret is one of sbx's stored secrets, as sbx secret ls lists it: its
// name and scope, never its value.
type Secret struct {
	Scope string `json:"scope"`
	Type  string `json:"type"`
	Name  string `json:"name"`
}

func (r *SBX) Secrets(ctx context.Context) ([]Secret, error) {
	b, err := commandOutput(hostCommand(ctx, "sbx", "secret", "ls", "--json"))
	if err != nil {
		return nil, err
	}
	list, err := sbxList[Secret](b, "secrets")
	if err != nil {
		return nil, fmt.Errorf("cannot parse sbx secret list; update sbx: %w", err)
	}
	return list, nil
}

// SecretSet has sbx ask the person for a service's token on their own
// terminal, which unibox only hands over: the value goes from the
// keyboard, or a pipe, to sbx's store and never through unibox. With a
// command, sbx stores the command instead and runs it on the host when it
// needs the token - a keychain read, say. Either is global: sbx env rm
// deletes a sandbox's own.
func (r *SBX) SecretSet(ctx context.Context, service, command string, in io.Reader, out, errout io.Writer) error {
	args := []string{"secret", "set", service}
	if command != "" {
		args = append(args, "--command", command)
	}
	cmd := hostCommand(ctx, "sbx", args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = in, out, errout
	if err := cmd.Run(); err != nil {
		// sbx has said on the terminal why, a cancelled entry say; its exit
		// status adds nothing to that.
		if _, exited := errors.AsType[*exec.ExitError](err); exited {
			return fmt.Errorf("sbx did not store the %s token", service)
		}
		return fmt.Errorf("sbx did not store the %s token: %w", service, err)
	}
	return nil
}

func (r *SBX) Remove(ctx context.Context, name string) error {
	// Confirmation is unibox's: unpushed work and active sessions are
	// checked before any removal, so sbx must not prompt a stdin it lacks.
	_, err := commandOutput(hostCommand(ctx, "sbx", "rm", "--force", name))
	return err
}

func (r *SBX) PublishedPorts(ctx context.Context, sandbox string) ([]api.PublishedPort, error) {
	b, err := commandOutput(hostCommand(ctx, "sbx", "ports", sandbox, "--json"))
	if err != nil {
		return nil, err
	}
	var ports []api.PublishedPort
	if err = json.Unmarshal(b, &ports); err != nil {
		return nil, fmt.Errorf("sbx ports answered in a form unibox does not know; upgrade unibox: %w", err)
	}
	return ports, nil
}

func (r *SBX) PublishPort(ctx context.Context, sandbox, spec string, log io.Writer) error {
	if err := runQuiet(hostCommand(ctx, "sbx", "ports", sandbox, "--publish", spec)); err != nil {
		return fmt.Errorf("sbx did not publish it: %w", err)
	}
	return nil
}

// UnpublishPort names the port as sbx published it: sbx takes it back
// only by its host port, which it picks anew for an ephemeral port at
// each start, and quietly does nothing for another.
func (r *SBX) UnpublishPort(ctx context.Context, sandbox string, p api.PublishedPort, log io.Writer) error {
	ip := p.HostIP
	if strings.Contains(ip, ":") {
		ip = "[" + ip + "]"
	}
	spec := fmt.Sprintf("%s:%d:%d/%s", ip, p.HostPort, p.SandboxPort, p.Protocol)
	if err := runQuiet(hostCommand(ctx, "sbx", "ports", sandbox, "--unpublish", spec)); err != nil {
		return fmt.Errorf("sbx did not unpublish %s: %w", spec, err)
	}
	return nil
}

// Mount bind-mounts a host directory at a path of the running sandbox
// (sbx mount HOST:TARGET, checked: live, writable, kept across a stop). It
// is idempotent in sbx, so a task prepared again mounts again harmlessly.
func (r *SBX) Mount(ctx context.Context, sandbox, host, target string) error {
	mount := func() error { return runQuiet(hostCommand(ctx, "sbx", "mount", sandbox, host+":"+target)) }
	err := mount()
	// sbx mounts only into a running sandbox, and exec starts a stopped one.
	if err != nil && strings.Contains(err.Error(), "is not running") {
		if err := runQuiet(hostCommand(ctx, "sbx", "exec", sandbox, "true")); err != nil {
			return fmt.Errorf("sbx did not start sandbox %s to mount %s into it: %w", sandbox, host, err)
		}
		err = mount()
	}
	if err != nil {
		return fmt.Errorf("sbx did not mount %s at %s: %w", host, target, err)
	}
	return nil
}

// Unmount names the mount as it was made: sbx takes one with a target back
// only by both. A mount that is not there is taken out already: a removal
// that stopped after it goes on.
func (r *SBX) Unmount(ctx context.Context, sandbox, host, target string) error {
	if err := runQuiet(hostCommand(ctx, "sbx", "umount", sandbox, host+":"+target)); err != nil && !strings.Contains(err.Error(), "nothing to unmount") {
		return fmt.Errorf("sbx did not unmount %s from %s: %w", host, target, err)
	}
	return nil
}

// RuntimeCheck is one of a runtime's own checks of itself: whether it is
// signed in, whether its daemon answers.
type RuntimeCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // pass, warn, fail or skip
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
	// Link is a check's detail when it is a web address - where an update
	// is - and nothing else of it, as a detail may name the user's account.
	Link string `json:"link,omitempty"`
}

// SignedIn says whether sbx is signed in to Docker, as sbx diagnose sees
// it. A release without the check is taken as signed in; listing its
// sandboxes says otherwise.
func (r *SBX) SignedIn(ctx context.Context) (bool, error) {
	checks, err := r.Diagnose(ctx)
	if err != nil {
		return false, err
	}
	// sbx knows who is signed in only through its daemon, which a fresh
	// install or a restarted machine has not started yet.
	if slices.ContainsFunc(checks, func(c RuntimeCheck) bool { return c.Name == "Daemon" && c.Status == "fail" }) {
		if err := runQuiet(hostCommand(ctx, "sbx", "daemon", "start", "--detach")); err != nil {
			return false, fmt.Errorf("sbx's daemon did not start, so whether sbx is signed in is unknown; unibox doctor says what sbx lacks: %w", err)
		}
		if checks, err = r.Diagnose(ctx); err != nil {
			return false, err
		}
	}
	for _, c := range checks {
		if c.Name == "Authentication" {
			return c.Status == "pass", nil
		}
	}
	return true, nil
}

// NetworkPolicies are the global network policies sbx starts from, the
// one it recommends first.
var NetworkPolicies = []string{"balanced", "allow-all", "deny-all"}

// NetworkPolicyChosen says whether sbx's global network policy has been
// chosen, which sbx creates no sandbox without. What diagnose cannot say,
// with its daemon down or in a release without the check, is taken as
// chosen; creating a sandbox says otherwise.
func (r *SBX) NetworkPolicyChosen(ctx context.Context) (bool, error) {
	checks, err := r.Diagnose(ctx)
	if err != nil {
		return false, err
	}
	for _, c := range checks {
		if c.Name == "Global network policy" {
			return c.Status != "fail" || !strings.Contains(c.Message, "not initialized"), nil
		}
	}
	return true, nil
}

// ChooseNetworkPolicy has sbx start every sandbox from one of
// NetworkPolicies, once; rules added later change it.
func (r *SBX) ChooseNetworkPolicy(ctx context.Context, policy string) error {
	if !slices.Contains(NetworkPolicies, policy) {
		return fmt.Errorf("%q is no network policy sbx has; choose %s", policy, strings.Join(NetworkPolicies, ", "))
	}
	if err := runQuiet(hostCommand(ctx, "sbx", "policy", "init", policy)); err != nil {
		return fmt.Errorf("sbx did not take the %s network policy: %w", policy, err)
	}
	return nil
}

// Login runs sbx login on the user's terminal, where sbx asks for what it
// needs; nothing of it passes through unibox.
func (r *SBX) Login(ctx context.Context, in io.Reader, out, errout io.Writer) error {
	c := hostCommand(ctx, "sbx", "login")
	c.Stdin, c.Stdout, c.Stderr = in, out, errout
	if err := c.Run(); err != nil {
		return fmt.Errorf("sbx login did not finish; run unibox init again to sign in: %w", err)
	}
	return nil
}

// Diagnose is sbx diagnose: its installation, the daemon, the sign-in.
// Its details, which may name the user's account, are left out.
func (r *SBX) Diagnose(ctx context.Context) ([]RuntimeCheck, error) {
	b, err := commandOutput(hostCommand(ctx, "sbx", "diagnose", "--format", "json"))
	if i := bytes.IndexByte(b, '{'); i >= 0 {
		b = b[i:]
	} else if err != nil {
		return nil, fmt.Errorf("sbx diagnose did not finish: %w", err)
	}
	var report struct {
		Checks []struct {
			RuntimeCheck
			Detail string `json:"detail"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(b, &report); err != nil {
		return nil, fmt.Errorf("could not read sbx diagnose --format json: %w", err)
	}
	checks := []RuntimeCheck{}
	for _, c := range report.Checks {
		if d := strings.TrimSpace(c.Detail); strings.HasPrefix(d, "https://") && !strings.ContainsAny(d, " \n") {
			c.Link = d
		}
		checks = append(checks, c.RuntimeCheck)
	}
	return checks, nil
}

// sbxList reads a list sbx prints as JSON: a list itself since sbx 0.48,
// an object holding it under key before.
func sbxList[T any](b []byte, key string) ([]T, error) {
	b = bytes.TrimSpace(b)
	if i := bytes.IndexAny(b, "[{"); i > 0 {
		b = b[i:]
	}
	var list []T
	if bytes.HasPrefix(b, []byte("[")) {
		err := json.Unmarshal(b, &list)
		return list, err
	}
	var wrapped map[string]json.RawMessage
	if err := json.Unmarshal(b, &wrapped); err != nil {
		return nil, err
	}
	if raw, ok := wrapped[key]; ok && string(raw) != "null" {
		err := json.Unmarshal(raw, &list)
		return list, err
	}
	return list, nil
}
