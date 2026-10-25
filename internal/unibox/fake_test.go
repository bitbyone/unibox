package unibox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/tobola/unibox/internal/unibox/api"
)

// fakeRuntime stands in for sbx. Its guest runs on the host's files under
// a directory of each sandbox, a mount is a link there to the host
// directory, and its ports behave as sbx's do: published ones stay across
// a stop, an ephemeral one gets a host port of its own.
type fakeRuntime struct {
	mu                                  sync.Mutex
	root                                string
	boxes                               []Box
	creates, removes, envRemoves, stops int
	calls                               []string
	args                                string
	kits                                map[string]Kit
	secrets                             []Secret
	remotes                             map[string]string
	published                           map[string][]api.PublishedPort
	// mounted is what each sandbox has mounted, host and target, which
	// the guest's /proc/self/mountinfo shows.
	mounted                              map[string][][2]string
	next                                 int
	failCreate, failUnmount, keepMounted bool
	unavailable                          error
	sshAgent                             bool
}

func (f *fakeRuntime) record(call string) {
	f.calls = append(f.calls, call)
}

func (f *fakeRuntime) called(prefix string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []string{}
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			out = append(out, c)
		}
	}
	return out
}

func (f *fakeRuntime) Present() bool                               { return true }
func (f *fakeRuntime) Available(context.Context) error             { return f.unavailable }
func (f *fakeRuntime) AllowSSHAgent(on bool)                       { f.sshAgent = on }
func (f *fakeRuntime) SSHForwarding(context.Context) (bool, error) { return f.sshAgent, nil }
func (f *fakeRuntime) SetSSHForwarding(_ context.Context, on bool) error {
	f.sshAgent = on
	return nil
}
func (f *fakeRuntime) Diagnose(context.Context) ([]RuntimeCheck, error) { return nil, nil }
func (f *fakeRuntime) SignedIn(context.Context) (bool, error)           { return true, nil }
func (f *fakeRuntime) Login(context.Context, io.Reader, io.Writer, io.Writer) error {
	return nil
}

func (f *fakeRuntime) List(context.Context) ([]Box, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.boxes), nil
}

func (f *fakeRuntime) setState(name, state string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.boxes {
		if f.boxes[i].Name == name {
			f.boxes[i].State = state
		}
	}
}

func (f *fakeRuntime) EnvCreate(_ context.Context, name, args string, files []string, _ io.Reader, _, _ io.Writer) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("env create " + name + " " + strings.Join(files, " "))
	if args != "" {
		data, err := os.ReadFile(args)
		if err != nil {
			return err
		}
		f.args = string(data)
	}
	if f.failCreate {
		return errors.New("sbx env create failed")
	}
	f.creates++
	f.boxes = append(f.boxes, Box{Name: name, State: "running", ID: fmt.Sprint("id-", f.creates)})
	return nil
}

func (f *fakeRuntime) EnvRemove(ctx context.Context, name, args string, files []string, _ io.Writer) error {
	f.mu.Lock()
	f.envRemoves++
	f.mu.Unlock()
	return f.Remove(ctx, name)
}

func (f *fakeRuntime) Stop(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stops++
	return nil
}

func (f *fakeRuntime) Remove(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removes++
	f.record("rm " + name)
	f.boxes = slices.DeleteFunc(f.boxes, func(b Box) bool { return b.Name == name })
	delete(f.published, name)
	delete(f.mounted, name)
	return os.RemoveAll(filepath.Join(f.root, name))
}

func (f *fakeRuntime) KitInspect(_ context.Context, ref string, args map[string]string) (Kit, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(args) > 0 {
		f.record("kit inspect " + ref + " " + fmt.Sprint(args))
	}
	if k, ok := f.kits[ref]; ok {
		return k, nil
	}
	return Kit{Name: filepath.Base(ref), Kind: "mixin"}, nil
}

func (f *fakeRuntime) Version(context.Context) (string, error) { return MinSBXVersion, nil }

func (f *fakeRuntime) EnvPlan(_ context.Context, name, args string, files []string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("env plan " + name)
	return "  1 to add · 0 to change · 0 to destroy\n", nil
}

func (f *fakeRuntime) Setting(context.Context, string) (string, error) { return "false", nil }

func (f *fakeRuntime) Secrets(context.Context) ([]Secret, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.secrets), nil
}

func (f *fakeRuntime) SecretSet(_ context.Context, service, command string, _ io.Reader, _, _ io.Writer) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("secret set " + service)
	f.secrets = append(f.secrets, Secret{Scope: "global", Type: "service", Name: service})
	return nil
}

func (f *fakeRuntime) guest(sandbox, target string) string {
	return filepath.Join(f.root, sandbox, "work", strings.TrimPrefix(target, "/work/"))
}

func (f *fakeRuntime) Mount(_ context.Context, sandbox, host, target string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("mount " + host + ":" + target)
	if f.mounted == nil {
		f.mounted = map[string][][2]string{}
	}
	f.mounted[sandbox] = append(f.mounted[sandbox], [2]string{host, target})
	at := f.guest(sandbox, target)
	if err := os.MkdirAll(filepath.Dir(at), 0o755); err != nil {
		return err
	}
	_ = os.Remove(at)
	return os.Symlink(host, at)
}

// Unmount leaves the empty mount point sbx leaves.
func (f *fakeRuntime) Unmount(_ context.Context, sandbox, host, target string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("unmount " + host + ":" + target)
	if f.failUnmount {
		return errors.New("sbx is away")
	}
	if f.keepMounted {
		return nil
	}
	f.mounted[sandbox] = slices.DeleteFunc(f.mounted[sandbox], func(mt [2]string) bool { return mt == [2]string{host, target} })
	at := f.guest(sandbox, target)
	if err := os.Remove(at); err != nil {
		return err
	}
	return os.Mkdir(at, 0o755)
}

func (f *fakeRuntime) PublishedPorts(_ context.Context, sandbox string) ([]api.PublishedPort, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.published[sandbox]), nil
}

func (f *fakeRuntime) PublishPort(_ context.Context, sandbox, spec string, _ io.Writer) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, err := ParsePortSpec(spec)
	if err != nil {
		return err
	}
	q := api.PublishedPort{HostIP: "127.0.0.1", HostPort: p.HostPort, SandboxPort: p.SandboxPort, Protocol: "tcp4"}
	if p.HostPort == 0 {
		f.next++
		q.HostPort = 49151 + f.next
	}
	for _, o := range f.published[sandbox] {
		if o.HostPort == q.HostPort {
			return fmt.Errorf("409 Conflict: port %d is already published", q.HostPort)
		}
	}
	if f.published == nil {
		f.published = map[string][]api.PublishedPort{}
	}
	f.published[sandbox] = append(f.published[sandbox], q)
	return nil
}

func (f *fakeRuntime) UnpublishPort(_ context.Context, sandbox string, q api.PublishedPort, _ io.Writer) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.published[sandbox] = slices.DeleteFunc(f.published[sandbox], func(o api.PublishedPort) bool { return o == q })
	return nil
}

func (f *fakeRuntime) hostPorts(sandbox string) []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []int{}
	for _, q := range f.published[sandbox] {
		out = append(out, q.HostPort)
	}
	slices.Sort(out)
	return out
}

func (f *fakeRuntime) Exec(ctx context.Context, name string, c Command) *exec.Cmd {
	boxRoot := filepath.Join(f.root, name)
	_ = os.MkdirAll(filepath.Join(boxRoot, "home"), 0o700)
	replace := func(s string) string {
		s = strings.ReplaceAll(s, "/var/lib/unibox", filepath.Join(boxRoot, "state"))
		s = strings.ReplaceAll(s, "/work", filepath.Join(boxRoot, "work"))
		s = strings.ReplaceAll(s, "/proc/sys/kernel/random/boot_id", filepath.Join(f.root, "boot"))
		s = strings.ReplaceAll(s, "chown agent:agent", ":")
		s = strings.ReplaceAll(s, "chown -R agent:agent", ":")
		f.mu.Lock()
		for from, to := range f.remotes {
			s = strings.ReplaceAll(s, from, to)
		}
		f.mu.Unlock()
		return s
	}
	args := slices.Clone(c.Args)
	for i, a := range args {
		args[i] = replace(a)
	}
	f.mu.Lock()
	if slices.Contains(c.Args, "/proc/self/mountinfo") {
		info := "99 69 0:29 / / rw,relatime - overlay overlay rw\n"
		for i, mt := range f.mounted[name] {
			info += fmt.Sprintf("%d 99 0:55 %s %s rw,nosuid - virtiofs host rw\n", 100+i, strings.ReplaceAll(mt[0], " ", `\040`), strings.ReplaceAll(mt[1], " ", `\040`))
		}
		at := filepath.Join(f.root, name+".mountinfo")
		_ = os.WriteFile(at, []byte(info), 0o600)
		args = []string{"cat", at}
	}
	f.record("exec " + name + " " + strings.Join(c.Args, " "))
	f.mu.Unlock()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	// The guest's global git configuration is its home's, as in a VM.
	cmd.Env = append(HostEnvironment(), "HOME="+filepath.Join(boxRoot, "home"), "GIT_CONFIG_GLOBAL="+filepath.Join(boxRoot, "home", ".gitconfig"))
	if c.Directory != "" {
		cmd.Dir = replace(c.Directory)
	}
	if c.Input != nil {
		b, _ := io.ReadAll(c.Input)
		cmd.Stdin = strings.NewReader(replace(string(b)))
	}
	cmd.Stdout = c.Output
	cmd.Stderr = c.Error
	return cmd
}

// managerFixture is a manager with an env dev, from a team's sbxenv whose
// workload kit runs claude, over the fake runtime.
func managerFixture(t *testing.T) (*Manager, *fakeRuntime) {
	t.Helper()
	root := t.TempDir()
	write(t, filepath.Join(root, "sandboxes", "dev.sbxenv.yaml"), `schemaVersion: "1"
agent: agent
args:
  user:
    default: me
kits:
  - ../kits/agent
  - ../kits/git
`)
	write(t, filepath.Join(root, "kits", "agent", "agent.yaml"), "kind: sandbox\n")
	write(t, filepath.Join(root, "kits", "git", "git.yaml"), "kind: mixin\n")
	paths := Paths{Config: t.TempDir(), Cache: t.TempDir()}
	var log bytes.Buffer
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(log.String())
		}
	})
	m, err := Open(paths, &log)
	if err != nil {
		t.Fatal(err)
	}
	m.Config.Envs = map[string]EnvConfig{"dev": {Files: []string{filepath.Join(root, "sandboxes", "dev.sbxenv.yaml")}}}
	if err := paths.SaveConfig(m.Config); err != nil {
		t.Fatal(err)
	}
	agent := Kit{Name: "agent", Kind: "sandbox"}
	agent.Sandbox.Entrypoint = []string{"claude"}
	agent.Sandbox.Command.Default = []string{"--dangerously-skip-permissions"}
	agent.Credentials = []KitCredential{{Service: "anthropic", Description: "Claude's API key"}}
	f := &fakeRuntime{root: t.TempDir(), remotes: map[string]string{}, kits: map[string]Kit{filepath.Join(root, "kits", "agent"): agent}}
	write(t, filepath.Join(f.root, "boot"), "first-boot")
	m.SBX = f
	m.In, m.Out, m.Err = strings.NewReader(""), &log, &log
	return m, f
}

func (f *fakeRuntime) NetworkRules(context.Context, string) ([]NetworkRule, error) {
	return nil, nil
}
func (f *fakeRuntime) SetNetworkRule(context.Context, string, string, []string) error { return nil }
func (f *fakeRuntime) RemoveNetworkRule(context.Context, string, []string) error      { return nil }
func (f *fakeRuntime) NetworkLog(context.Context, string, int, io.Writer) error       { return nil }
