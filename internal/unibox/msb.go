package unibox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tobola/unibox/internal/unibox/api"
)

// MSB runs sandboxes in microsandbox, an experimental runtime that shows a
// profile carries over from sbx unchanged. It is used only when asked for;
// the SDK installs msb on first use.
// unibox drives msb's CLI - a sandbox is created from a configuration file
// unibox writes, and sessions run through msb exec, whose terminal handling
// the SDK does not offer - and uses the SDK for installing the runtime and
// for rotating secrets live, which the CLI cannot do without a restart.
type MSB struct {
	Cache string
	Log   io.Writer
	// Binary is msb's path when set, so tests use a stand-in; empty finds
	// the installed runtime, installing it the first time.
	Binary string
	// DNS is how the host resolves names; nil asks the host itself.
	DNS *hostDNS

	mu   sync.Mutex
	path string
}

// msbDisk is the size a sandbox's writable root may grow to. msb's own 4
// GiB is filled by an agent's caches and a task's dependencies; the disk is
// sparse, so only what is written takes space on the host.
const msbDisk = "64G"

func (r *MSB) Name() string               { return "msb" }
func (r *MSB) Experimental() bool         { return true }
func (r *MSB) Capabilities() Capabilities { return Capabilities{"vm", true, true, true} }

// msbPlatform says whether microsandbox runs here: Apple silicon Macs and
// Linux.
func msbPlatform() bool {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "darwin/arm64", "linux/amd64", "linux/arm64":
		return true
	}
	return false
}

func (r *MSB) Present() bool {
	if !msbPlatform() {
		return false
	}
	if r.Binary != "" || msbBuiltIn {
		return true
	}
	_, err := msbResolve()
	return err == nil
}

// Available does not install the runtime: that waits for the first command
// that needs it, so asking which runtimes there are downloads nothing.
func (r *MSB) Available(ctx context.Context) error {
	if !msbPlatform() {
		return fmt.Errorf("microsandbox runs on Apple silicon Macs and on Linux; use sbx here")
	}
	if runtime.GOOS == "linux" {
		f, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
		if err != nil {
			return fmt.Errorf("microsandbox needs KVM; enable it and give your user access to /dev/kvm (the kvm group)")
		}
		f.Close()
	}
	if r.Binary != "" || msbBuiltIn {
		return nil
	}
	if _, err := msbResolve(); err != nil {
		return fmt.Errorf("msb is not installed; install it with curl -fsSL https://install.microsandbox.dev | sh")
	}
	return nil
}

// binary finds msb, installing the runtime the first time it is needed.
func (r *MSB) binary(ctx context.Context) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.path != "" {
		return r.path, nil
	}
	if r.Binary != "" {
		r.path = r.Binary
		return r.path, nil
	}
	path, err := msbResolve()
	if err != nil {
		if r.Log != nil && msbBuiltIn {
			fmt.Fprintln(r.Log, "installing the microsandbox runtime into ~/.microsandbox; this happens once")
		}
		if path, err = msbInstall(ctx); err != nil {
			return "", err
		}
	}
	r.path = path
	return path, nil
}

// command is an msb command; an msb that cannot be found or installed is
// the error the command returns when it is run.
func (r *MSB) command(ctx context.Context, args ...string) *exec.Cmd {
	path, err := r.binary(ctx)
	if err != nil {
		cmd := hostCommand(ctx, "msb", args...)
		cmd.Err = err
		return cmd
	}
	return hostCommand(ctx, path, args...)
}

// withSecrets is an msb command that boots a sandbox: msb reads each of its
// secrets from the variable the sandbox's configuration names, when it
// boots. A shell fills each variable with the output of the secret's host
// command and runs msb, so a value goes from the command to msb and never
// through unibox.
func (r *MSB) withSecrets(ctx context.Context, program string, secrets []msbSecret, args ...string) *exec.Cmd {
	if len(secrets) == 0 {
		return hostCommand(ctx, program, args...)
	}
	var s strings.Builder
	s.WriteString("set -u\n")
	for _, sec := range secrets {
		fail := func(what string) string {
			return "{ echo " + Quote("unibox: the command for "+sec.Env+" "+what) + " >&2; exit 1; }"
		}
		s.WriteString("v=$(" + sec.Command + ") || " + fail("failed") + "\n")
		s.WriteString("[ -n \"$v\" ] || " + fail("printed nothing") + "\n")
		s.WriteString("export " + sec.Source + "=\"$v\"\n")
	}
	s.WriteString("unset v\nexec \"$0\" \"$@\"\n")
	return hostCommand(ctx, "sh", append([]string{"-c", s.String(), program}, args...)...)
}

func (r *MSB) List(ctx context.Context) ([]Box, error) {
	b, err := commandOutput(r.command(ctx, "list", "--format", "json"))
	if err != nil {
		return nil, err
	}
	var list []struct{ Name, Status string }
	if err = json.Unmarshal(b, &list); err != nil {
		return nil, fmt.Errorf("msb list answered in a form unibox does not know; upgrade unibox: %w", err)
	}
	boxes := []Box{}
	for _, v := range list {
		boxes = append(boxes, Box{Name: v.Name, State: strings.ToLower(v.Status)})
	}
	return boxes, nil
}

type msbImage struct {
	Reference string    `json:"reference"`
	Digest    string    `json:"digest"`
	Created   time.Time `json:"created_at"`
}

func (r *MSB) images(ctx context.Context) ([]msbImage, error) {
	b, err := commandOutput(r.command(ctx, "image", "ls", "--format", "json"))
	if err != nil {
		return nil, err
	}
	var list []msbImage
	if err = json.Unmarshal(b, &list); err != nil {
		return nil, fmt.Errorf("msb image ls answered in a form unibox does not know; upgrade unibox: %w", err)
	}
	return list, nil
}

func (r *MSB) Images(ctx context.Context) ([]Image, error) {
	list, err := r.images(ctx)
	if err != nil {
		return nil, err
	}
	out := []Image{}
	for _, im := range list {
		out = append(out, Image{im.Reference, im.Digest})
	}
	return out, nil
}

func (r *MSB) Build(ctx context.Context, p Profile, tag string, log io.Writer) (string, error) {
	return buildImage(ctx, r.Cache, r.Name(), p, tag, log, func(archive string) error {
		return runLogged(r.command(ctx, "image", "load", "--input", archive), log)
	})
}

// msbSecret is one of a sandbox's secrets: the variable the sandbox sees
// its placeholder in, the host variable msb reads its value from when the
// sandbox boots, and the host command that prints the value. Only the
// command is kept, never what it prints.
type msbSecret struct {
	Env         string   `json:"env"`
	Source      string   `json:"source"`
	Placeholder string   `json:"placeholder"`
	Hosts       []string `json:"hosts"`
	// Headers are the request headers the value may go into; none means
	// any, for a tool whose header unibox does not know.
	Headers []string `json:"headers,omitempty"`
	Command string   `json:"command"`
}

func msbSource(env string) string { return "UNIBOX_MSB_" + env }

// msbSecrets are the secrets a sandbox is created with: each agent's login
// and each service's token.
func msbSecrets(s Spec) ([]msbSecret, error) {
	out := []msbSecret{}
	for _, id := range s.Agents {
		a, ok := s.Access.Auth[id]
		if !ok {
			continue
		}
		sec, ok, err := msbAgentSecret(id, a)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, sec)
		}
	}
	for _, name := range sortedKeys(s.Access.Services) {
		svc := s.Access.Services[name]
		resolved := svc
		var err error
		if resolved.Command, err = ResolveCommand(svc.Command); err != nil {
			return nil, fmt.Errorf("service %s: %w", name, err)
		}
		env := ServiceEnv(name, svc)
		sec := msbSecret{Env: env, Source: msbSource(env), Placeholder: ServicePlaceholder(name, svc), Command: serviceSecretCommand(resolved)}
		for _, h := range svc.ProxyHosts() {
			sec.Hosts = append(sec.Hosts, msbHost(h))
		}
		// unibox writes a git service's whole Authorization header; a
		// tool reading an env service's token picks its own header.
		if svc.Git {
			sec.Headers = []string{"Authorization"}
		}
		out = append(out, sec)
	}
	return out, nil
}

// msbHost is a host without its port: msb matches a secret's hosts by name.
func msbHost(h string) string {
	if host, port, err := net.SplitHostPort(h); err == nil && port != "" {
		return host
	}
	return h
}

// msbPorts are the ports a sandbox is created with, in msb's terms: the
// profile's ports that are on by default. A port without a host port gets a
// free one now, kept for the sandbox's life, since msb publishes a port
// only when it creates the sandbox.
func msbPorts(ports map[string]Port) ([]string, error) {
	out := []string{}
	for _, name := range sortedKeys(ports) {
		if !ports[name].Default {
			continue
		}
		spec, err := ParsePortSpec(ports[name].Spec)
		if err != nil {
			return nil, err
		}
		ip := spec.HostIP
		if ip == "" {
			ip = "127.0.0.1"
		}
		proto := "tcp"
		if strings.HasPrefix(spec.Protocol, "udp") {
			proto = "udp"
		}
		host := spec.HostPort
		if host == 0 {
			if host, err = freePort(ip, proto); err != nil {
				return nil, fmt.Errorf("port %s: %w", name, err)
			}
		}
		if strings.Contains(ip, ":") {
			ip = "[" + ip + "]"
		}
		out = append(out, fmt.Sprintf("%s:%d:%d/%s", ip, host, spec.SandboxPort, proto))
	}
	return out, nil
}

func freePort(ip, proto string) (int, error) {
	if proto == "udp" {
		c, err := net.ListenPacket("udp", net.JoinHostPort(ip, "0"))
		if err != nil {
			return 0, err
		}
		defer c.Close()
		return c.LocalAddr().(*net.UDPAddr).Port, nil
	}
	l, err := net.Listen("tcp", net.JoinHostPort(ip, "0"))
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// msbConfig is the sandbox's configuration as msb create --conf reads it.
func (r *MSB) msbConfig(ctx context.Context, s Spec, secrets []msbSecret, log io.Writer) (map[string]any, error) {
	conf := map[string]any{
		"image":       map[string]any{"oci": s.Image, "upper_size": msbDisk},
		"pull_policy": "never",
		"labels":      map[string]string{"unibox.sandbox": s.Name},
	}
	if s.Resources.CPUs != "" || s.Resources.Memory != "" {
		cpus, memory, err := s.Resources.Resolve(HostResources())
		if err != nil {
			return nil, err
		}
		if s.Resources.CPUs != "" {
			conf["cpus"] = min(cpus, 255)
		}
		if memory != "" {
			conf["memory"] = fmt.Sprintf("%dM", memoryBytes(memory)>>20)
		}
	}
	env := map[string]string{}
	for _, k := range s.Access.Env {
		v, ok := os.LookupEnv(k)
		if !ok {
			return nil, fmt.Errorf("profile requires environment variable %s; set it before creating the sandbox", k)
		}
		env[k] = v
	}
	mounts := []map[string]any{}
	for _, c := range s.Access.Caches {
		env[c.Env] = c.Host
		// msb refuses a mount whose host path goes through a symlink,
		// such as macOS's /tmp; the sandbox still sees the path the
		// profile names.
		source, err := filepath.EvalSymlinks(c.Host)
		if err != nil {
			return nil, fmt.Errorf("cache %s: %w", c.Host, err)
		}
		mounts = append(mounts, map[string]any{"bind": source, "target": c.Host, "readonly": true})
	}
	if len(env) > 0 {
		conf["env"] = env
	}
	if len(mounts) > 0 {
		conf["mounts"] = mounts
	}
	allow, deny := msbNetwork(s.Access)
	network := map[string]any{"allow": allow, "tls": map[string]any{"enabled": true}}
	dns := r.DNS
	if dns == nil {
		dns = defaultHostDNS()
	}
	named := slices.Clone(s.Access.AllowNetwork)
	for _, name := range sortedKeys(s.Access.Services) {
		named = append(named, s.Access.Services[name].ProxyHosts()...)
	}
	nameservers, addresses, notes := dns.msbDNSFor(ctx, named)
	for _, n := range notes {
		fmt.Fprintln(log, n)
	}
	if len(nameservers) > 0 {
		network["allow"] = append(allow, addresses...)
		network["dns"] = map[string]any{"nameservers": nameservers}
	}
	if len(deny) > 0 {
		network["deny"] = deny
	}
	ports, err := msbPorts(s.Access.Ports)
	if err != nil {
		return nil, err
	}
	if len(ports) > 0 {
		network["ports"] = ports
	}
	conf["network"] = network
	if len(secrets) > 0 {
		list := map[string]any{}
		for _, sec := range secrets {
			substitution := map[string]any{"headers": true}
			if len(sec.Headers) > 0 {
				substitution["header_fields"] = sec.Headers
			}
			list[sec.Env] = map[string]any{
				"value":        map[string]string{"$msb_env": sec.Source},
				"placeholder":  sec.Placeholder,
				"allow":        sec.Hosts,
				"substitution": substitution,
			}
		}
		conf["secrets"] = list
	}
	return conf, nil
}

func (r *MSB) dir(sandbox string) string { return filepath.Join(r.Cache, sandbox) }

func (r *MSB) Create(ctx context.Context, s Spec, log io.Writer) error {
	program, err := r.binary(ctx)
	if err != nil {
		return err
	}
	secrets, err := msbSecrets(s)
	if err != nil {
		return err
	}
	conf, err := r.msbConfig(ctx, s, secrets, log)
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(conf)
	if err != nil {
		return err
	}
	list, err := json.MarshalIndent(secrets, "", "  ")
	if err != nil {
		return err
	}
	dir := r.dir(s.Name)
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, "sandbox.yaml")
	if err = AtomicWrite(path, data, 0o600); err != nil {
		return err
	}
	// What boots the sandbox again: the commands, not their output.
	if err = AtomicWrite(filepath.Join(dir, "secrets.json"), list, 0o600); err != nil {
		return err
	}
	if err = runLogged(r.withSecrets(ctx, program, secrets, "create", "--name", s.Name, "--conf", path), log); err != nil {
		return err
	}
	for _, sec := range secrets {
		fmt.Fprintf(log, "%s carries its placeholder to %s\n", sec.Env, strings.Join(sec.Hosts, ", "))
	}
	return nil
}

func (r *MSB) secrets(sandbox string) ([]msbSecret, error) {
	data, err := os.ReadFile(filepath.Join(r.dir(sandbox), "secrets.json"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var list []msbSecret
	if err = json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("unibox's record of sandbox %s's secrets is damaged; recreate it: %w", sandbox, err)
	}
	return list, nil
}

// Start boots a stopped sandbox with its secrets fetched now.
func (r *MSB) Start(ctx context.Context, sandbox string, log io.Writer) error {
	program, err := r.binary(ctx)
	if err != nil {
		return err
	}
	secrets, err := r.secrets(sandbox)
	if err != nil {
		return err
	}
	if err = runQuiet(r.withSecrets(ctx, program, secrets, "start", sandbox)); err != nil {
		return fmt.Errorf("msb did not start sandbox %s: %w", sandbox, err)
	}
	return nil
}

// RefreshSecrets fetches a running sandbox's tokens again: msb keeps what
// it read when the sandbox booted and refreshes nothing. unibox runs itself
// under the shell that fills the variables, and that process hands them to
// msb through the SDK, which rotates a secret without a restart.
func (r *MSB) RefreshSecrets(ctx context.Context, sandbox string, log io.Writer) error {
	secrets, err := r.secrets(sandbox)
	if err != nil || len(secrets) == 0 {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{MSBSecretsCommand, sandbox}
	for _, s := range secrets {
		args = append(args, s.Env+"="+s.Source+"="+s.Placeholder)
	}
	if err = runQuiet(r.withSecrets(ctx, self, secrets, args...)); err != nil {
		return fmt.Errorf("could not fetch the tokens of sandbox %s again; unibox stop %s starts it with fresh ones next time: %w", sandbox, sandbox, err)
	}
	return nil
}

// MSBSecretsCommand is the hidden command RefreshSecrets runs unibox as.
const MSBSecretsCommand = "__msb-secrets"

// RotateMSBSecrets is that command's work: each argument is
// ENV=SOURCE=PLACEHOLDER, the value waiting in SOURCE.
func RotateMSBSecrets(ctx context.Context, sandbox string, args []string) error {
	list := []msbSecret{}
	for _, a := range args {
		parts := strings.SplitN(a, "=", 3)
		if len(parts) != 3 {
			return fmt.Errorf("argument %q is not ENV=SOURCE=PLACEHOLDER", a)
		}
		list = append(list, msbSecret{Env: parts[0], Source: parts[1], Placeholder: parts[2]})
	}
	return msbRotate(ctx, sandbox, list)
}

func (r *MSB) Exec(ctx context.Context, name string, c Command) *exec.Cmd {
	args := []string{"exec"}
	if c.Interactive {
		args = append(args, "--tty")
	} else {
		// Output as it comes, not when the command ends.
		args = append(args, "--stream")
		if c.Input == nil {
			args = append(args, "--no-stdin")
		}
	}
	if c.Directory != "" {
		args = append(args, "--workdir", c.Directory)
	}
	if c.User != "" {
		args = append(args, "--user", c.User)
	}
	args = append(args, name, "--")
	args = append(args, c.Args...)
	cmd := r.command(ctx, args...)
	cmd.Stdin = c.Input
	cmd.Stdout = c.Output
	cmd.Stderr = c.Error
	return cmd
}

func (r *MSB) Stop(ctx context.Context, name string) error {
	_, err := commandOutput(r.command(ctx, "stop", name))
	return err
}

func (r *MSB) Remove(ctx context.Context, name string) error {
	if _, err := commandOutput(r.command(ctx, "remove", "--force", name)); err != nil {
		return err
	}
	return os.RemoveAll(r.dir(name))
}

// PublishedPorts reads the ports msb published when it created the
// sandbox.
func (r *MSB) PublishedPorts(ctx context.Context, sandbox string) ([]api.PublishedPort, error) {
	b, err := commandOutput(r.command(ctx, "inspect", sandbox, "--format", "json"))
	if err != nil {
		return nil, err
	}
	var info struct {
		Config struct {
			Network struct {
				Ports []struct {
					GuestPort int    `json:"guest_port"`
					HostBind  string `json:"host_bind"`
					HostPort  int    `json:"host_port"`
					Protocol  string `json:"protocol"`
				} `json:"ports"`
			} `json:"network"`
		} `json:"config"`
	}
	if err = json.Unmarshal(b, &info); err != nil {
		return nil, fmt.Errorf("msb inspect answered in a form unibox does not know; upgrade unibox: %w", err)
	}
	out := []api.PublishedPort{}
	// unibox names a protocol with its address family, as sbx does: tcp4
	// for a port on an IPv4 address.
	for _, p := range info.Config.Network.Ports {
		proto := strings.ToLower(p.Protocol)
		if ip := net.ParseIP(p.HostBind); ip != nil && ip.To4() != nil {
			proto += "4"
		} else if ip != nil {
			proto += "6"
		}
		out = append(out, api.PublishedPort{HostIP: p.HostBind, HostPort: p.HostPort, SandboxPort: p.GuestPort, Protocol: proto})
	}
	return out, nil
}

// UniboxImages lists the profiles' builds in msb's store and the ones left
// in Docker, where a build makes them first.
func (r *MSB) UniboxImages(ctx context.Context) ([]RuntimeImage, error) {
	list, err := r.images(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := []RuntimeImage{}
	for _, im := range list {
		repo, _, _ := strings.Cut(im.Reference, ":")
		if !strings.HasPrefix(repo, "unibox/") {
			continue
		}
		out = append(out, RuntimeImage{Ref: im.Reference, Profile: strings.TrimPrefix(repo, "unibox/"), Created: im.Created})
		seen[im.Reference] = true
	}
	if d, err := commandOutput(hostCommand(ctx, "docker", "image", "ls", "--filter", "reference=unibox/*", "--format", "{{.Repository}}:{{.Tag}}")); err == nil {
		for _, ref := range strings.Fields(string(d)) {
			if repo, _, ok := strings.Cut(ref, ":"); ok && !seen[ref] {
				out = append(out, RuntimeImage{Ref: ref, Profile: strings.TrimPrefix(repo, "unibox/")})
				seen[ref] = true
			}
		}
	}
	return out, nil
}

// RemoveImage removes an image from msb's store and from Docker. A sandbox
// made from it keeps its own copy of the layers it uses.
func (r *MSB) RemoveImage(ctx context.Context, ref string) error {
	errMSB := runQuiet(r.command(ctx, "image", "rm", ref))
	if strings.HasPrefix(ref, "unibox/") && runQuiet(hostCommand(ctx, "docker", "image", "rm", ref)) == nil {
		return nil
	}
	return errMSB
}
