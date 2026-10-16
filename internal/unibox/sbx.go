package unibox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/tobola/unibox/internal/unibox/api"
)

type SBX struct {
	Cache string
	// Bindings is sbx's credentials.yaml, where a generated kit's
	// credentials are approved; empty leaves it alone.
	Bindings string
}

func (r *SBX) Name() string               { return "sbx" }
func (r *SBX) Capabilities() Capabilities { return Capabilities{"vm", true, true, true} }

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

func (r *SBX) Available(ctx context.Context) error {
	if _, err := exec.LookPath("sbx"); err != nil {
		return fmt.Errorf("sbx is not installed; install Docker Sandboxes and run sbx login")
	}
	forwarding, err := sbxForwarding(ctx)
	if err != nil {
		return err
	}
	if forwarding {
		return Fail(4, "host_access", "sbx forwards the host SSH agent; run sbx settings set ssh.agentForwardingEnabled false, then sbx daemon restart")
	}
	_, err = r.List(ctx)
	return err
}
func (r *SBX) List(ctx context.Context) ([]Box, error) {
	b, err := commandOutput(hostCommand(ctx, "sbx", "ls", "--json"))
	if err != nil {
		return nil, err
	}
	var response struct {
		Sandboxes []struct{ Name, Status, Memory, ID string } `json:"sandboxes"`
	}
	if err = json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("cannot parse sbx sandbox list; update sbx: %w", err)
	}
	boxes := []Box{}
	for _, v := range response.Sandboxes {
		boxes = append(boxes, Box{v.Name, v.Status, v.Memory, v.ID})
	}
	return boxes, nil
}
func (r *SBX) Images(ctx context.Context) ([]Image, error) {
	b, err := commandOutput(hostCommand(ctx, "sbx", "template", "ls", "--json"))
	if err != nil {
		return nil, err
	}
	var response struct {
		Images []struct{ ID, Repository, Tag string } `json:"images"`
	}
	if err = json.Unmarshal(b, &response); err != nil {
		return nil, err
	}
	images := []Image{}
	for _, v := range response.Images {
		ref := strings.TrimPrefix(v.Repository, "docker.io/") + ":" + v.Tag
		images = append(images, Image{ref, v.ID})
	}
	return images, nil
}
func (r *SBX) Create(ctx context.Context, s Spec, log io.Writer) error {
	args := []string{"create", "--name", s.Name, "--pull", "never", "--skills", "off"}
	if len(s.Agents) <= 1 {
		args = append(args, "--template", s.Image)
	}
	cpus, memory, err := sbxResources(s.Resources, HostResources(), log)
	if err != nil {
		return err
	}
	if cpus != "" {
		args = append(args, "--cpus", cpus)
	}
	if memory != "" {
		args = append(args, "--memory", memory)
	}
	for _, h := range s.Access.DenyNetwork {
		args = append(args, "--deny-network", h)
	}
	for _, k := range s.Access.Env {
		args = append(args, "-e", k)
	}
	for _, c := range s.Access.Caches {
		args = append(args, "-e", c.Env+"="+c.Host)
	}
	if len(s.Agents) > 1 {
		// Several agents: a generated v3 workload kit carries their
		// credential declarations; built-in v2 kinds cannot be combined.
		dir, err := r.writeKit(s, oauthFirst(ctx, s))
		if err != nil {
			return err
		}
		bound := slices.DeleteFunc(slices.Clone(s.Agents), func(id string) bool { return sbxKitToken(s, id) })
		if err = writeBindings(r.Bindings, bound, log); err != nil {
			return err
		}
		args = append(args, dir)
	} else {
		kind := s.Agent
		if kind == "" {
			kind = "shell"
		}
		args = append(args, kind)
	}
	for _, c := range s.Access.Caches {
		args = append(args, c.Host+":ro")
	}
	cmd := hostCommand(ctx, "sbx", args...)
	for _, k := range s.Access.Env {
		v, ok := os.LookupEnv(k)
		if !ok {
			return fmt.Errorf("profile requires environment variable %s; set it before creating the sandbox", k)
		}
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	if err := runLogged(cmd, log); err != nil {
		return err
	}
	// Service hosts must be reachable, so they widen the allow policy with
	// any the profile already set.
	allow := append([]string{}, s.Access.AllowNetwork...)
	for _, svc := range s.Access.Services {
		for _, h := range svc.ProxyHosts() {
			if !slices.Contains(allow, h) {
				allow = append(allow, h)
			}
		}
	}
	if len(allow) > 0 {
		// sbx create takes deny rules only; an allow rule is added to the
		// sandbox's own policy, which a deny rule still overrides.
		if err := runLogged(hostCommand(ctx, "sbx", "policy", "allow", "network", "--sandbox", s.Name, strings.Join(allow, ",")), log); err != nil {
			return err
		}
	}
	// A service's token is fetched and stored on the host; the sandbox gets
	// a placeholder the proxy swaps. unibox never sees the value - the
	// command runs on the host and sbx keeps the result.
	for name, svc := range s.Access.Services {
		args := []string{"secret", "set-custom", "--sandbox", s.Name, "--env", ServiceEnv(name, svc), "--placeholder", ServicePlaceholder(name, svc)}
		// Two services may share a host, told apart by path; sbx keeps a
		// secret for each and swaps each placeholder for its own value.
		for _, h := range svc.ProxyHosts() {
			args = append(args, "--host", h)
		}
		if svc.Refresh != "" {
			args = append(args, "--refresh", svc.Refresh)
		}
		resolved := svc
		var err error
		if resolved.Command, err = ResolveCommand(svc.Command); err != nil {
			return fmt.Errorf("service %s: %w", name, err)
		}
		args = append(args, "--command", serviceSecretCommand(resolved))
		// sbx explains each stored secret at length - a note on commands, a
		// reminder about variables in running sandboxes that git services
		// do not read - so its words are kept for a failure, and the
		// service is told in one line.
		if err := runQuiet(hostCommand(ctx, "sbx", args...)); err != nil {
			return fmt.Errorf("sbx could not store the %s service login: %w", name, err)
		}
		if svc.Git {
			fmt.Fprintf(log, "service %s: git signs in at %s through the proxy\n", name, strings.Join(svc.GitURLs(), ", "))
		} else {
			fmt.Fprintf(log, "service %s: %s carries its placeholder to %s\n", name, ServiceEnv(name, svc), strings.Join(svc.ProxyHosts(), ", "))
		}
	}
	return nil
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
func (r *SBX) Remove(ctx context.Context, name string) error {
	// Confirmation is unibox's: unpushed work and active sessions are
	// checked before any removal, so sbx must not prompt a stdin it lacks.
	_, err := commandOutput(hostCommand(ctx, "sbx", "rm", "--force", name))
	return err
}

func (r *SBX) Build(ctx context.Context, p Profile, tag string, log io.Writer) (string, error) {
	if err := os.MkdirAll(r.Cache, 0o700); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(r.Cache, "build-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	if p.Image != "" {
		if err = runLogged(hostCommand(ctx, "docker", "pull", p.Image), log); err != nil {
			return "", err
		}
		if err = runLogged(hostCommand(ctx, "docker", "tag", p.Image, tag), log); err != nil {
			return "", err
		}
	} else {
		for name, file := range p.BuildFiles() {
			dest := filepath.Join(dir, filepath.FromSlash(name))
			if err = AtomicWrite(dest, file.Data, os.FileMode(file.Mode)); err != nil {
				return "", err
			}
		}
		install, err := p.InstallContext()
		if err != nil {
			return "", err
		}
		installDir, err := os.MkdirTemp(r.Cache, "install-")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(installDir)
		if err = writeInstallContext(installDir, install); err != nil {
			return "", err
		}
		args := []string{"build", "--tag", tag, "--file", filepath.Join(dir, p.Dockerfile), "--build-context", "unibox=" + installDir}
		buildArgs, err := p.BuildArgs(r.Name())
		if err != nil {
			return "", err
		}
		for _, arg := range buildArgs {
			args = append(args, "--build-arg", arg)
		}
		args = append(args, dir)
		if err = runLogged(hostCommand(ctx, "docker", args...), log); err != nil {
			return "", err
		}
	}
	for _, id := range p.Agents {
		a, _ := FindAgent(id)
		fmt.Fprintf(log, "checking agent %s\n", id)
		cmd := hostCommand(ctx, "docker", "run", "--rm", "--network", "none", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--entrypoint", "/bin/sh", tag, "-c", "command -v "+Quote(a.Command))
		if err = runLogged(cmd, log); err != nil {
			return "", fmt.Errorf("profile %s declares %s but its image cannot find it; install it in the Dockerfile", p.Name, id)
		}
	}
	archive := filepath.Join(dir, "image.tar")
	if err = runLogged(hostCommand(ctx, "docker", "image", "save", "--output", archive, tag), log); err != nil {
		return "", err
	}
	if err = runLogged(hostCommand(ctx, "sbx", "template", "load", archive), log); err != nil {
		return "", err
	}
	b, err := commandOutput(hostCommand(ctx, "docker", "image", "inspect", "--format", "{{.Id}}", tag))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// UpdateNetwork changes a running sandbox's rules in place: sbx applies a
// sandbox's policy live (checked: an allow, a deny and their removal took
// effect without a restart). Removals go first, since sbx refuses a deny
// for a host an allow of the sandbox names exactly. Only the rules named
// are touched; one the user added with sbx stays.
func (r *SBX) UpdateNetwork(ctx context.Context, sandbox string, change NetworkChange, log io.Writer) error {
	if len(change.RemoveAllow)+len(change.RemoveDeny) > 0 {
		b, err := commandOutput(hostCommand(ctx, "sbx", "policy", "ls", "--json"))
		if err != nil {
			return err
		}
		var list struct {
			Rules []struct {
				ID           string   `json:"id"`
				SandboxID    string   `json:"sandbox_id"`
				ResourceType string   `json:"resource_type"`
				Decision     string   `json:"decision"`
				Resources    []string `json:"resources"`
			} `json:"rules"`
		}
		if err = json.Unmarshal(b, &list); err != nil {
			return fmt.Errorf("sbx policy ls answered in a form unibox does not know; upgrade unibox: %w", err)
		}
		for decision, hosts := range map[string][]string{"allow": change.RemoveAllow, "deny": change.RemoveDeny} {
			for _, h := range hosts {
				for _, rule := range list.Rules {
					if rule.SandboxID == sandbox && rule.ResourceType == "network" && rule.Decision == decision && len(rule.Resources) == 1 && rule.Resources[0] == h {
						if err := runQuiet(hostCommand(ctx, "sbx", "policy", "rm", "network", "--id", rule.ID, "--sandbox", sandbox, "--force")); err != nil {
							return fmt.Errorf("sbx did not remove the %s rule for %s: %w", decision, h, err)
						}
					}
				}
			}
		}
	}
	if len(change.AddDeny) > 0 {
		if err := runQuiet(hostCommand(ctx, "sbx", "policy", "deny", "network", "--sandbox", sandbox, strings.Join(change.AddDeny, ","))); err != nil {
			return fmt.Errorf("sbx did not add the deny rules: %w", err)
		}
	}
	if len(change.AddAllow) > 0 {
		if err := runQuiet(hostCommand(ctx, "sbx", "policy", "allow", "network", "--sandbox", sandbox, strings.Join(change.AddAllow, ","))); err != nil {
			return fmt.Errorf("sbx did not add the allow rules: %w", err)
		}
	}
	return nil
}

// sbxResources turns a profile's resources into sbx create's flags: a
// share of the host's CPUs into a count, all of them into 0, which sbx reads
// as every host CPU; memory into MiB, kept within the 75% of the host's sbx
// gives a sandbox (sbx create --help), so asking for all of it still starts
// one.
func sbxResources(r Resources, h Host, log io.Writer) (cpus, memory string, err error) {
	if r.CPUs.share() == 100 {
		cpus = "0"
	} else if r.CPUs != "" {
		n, _, err := (Resources{CPUs: r.CPUs}).Resolve(h)
		if err != nil {
			return "", "", err
		}
		cpus = fmt.Sprint(n)
	}
	if r.Memory == "" {
		return cpus, "", nil
	}
	_, memory, err = (Resources{Memory: r.Memory}).Resolve(h)
	if err != nil {
		return "", "", err
	}
	limit := max(h.Memory*3/4, 512<<20)
	if h.Memory > 0 && memoryBytes(memory) > limit {
		capped := fmt.Sprintf("%dm", limit>>20)
		fmt.Fprintf(log, "memory %s is more than sbx gives a sandbox, 75%% of the host's; using %s\n", r.Memory, capped)
		memory = capped
	}
	return cpus, memory, nil
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

// writeInstallContext lays the install context out for COPY --from=unibox
// / /, which gives each directory it copies the mode it has here: /usr and
// /usr/local among them, so every directory is 0755 - a 0700 one, as a
// temporary directory is made, shut apt's sandbox user out of /usr and
// failed every later apt-get on its signatures.
func writeInstallContext(dir string, files map[string]SnapshotFile) error {
	for name, file := range files {
		if err := AtomicWrite(filepath.Join(dir, filepath.FromSlash(name)), file.Data, os.FileMode(file.Mode)); err != nil {
			return err
		}
	}
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		return os.Chmod(path, 0o755)
	})
}

// sbxKitTag is the tag sbx gives the image it makes of a sandbox's kit:
// the kit directory's name, which unibox names after the sandbox, and a
// digest of the kit.
var sbxKitTag = regexp.MustCompile(`^(unibox-.+)-[0-9a-f]{12}$`)

// UniboxImages lists unibox's images in sbx's store - each profile's
// builds and each sandbox's kit images - and the builds left in Docker,
// where a build makes them first.
func (r *SBX) UniboxImages(ctx context.Context) ([]RuntimeImage, error) {
	b, err := commandOutput(hostCommand(ctx, "sbx", "template", "ls", "--json"))
	if err != nil {
		return nil, err
	}
	var list struct {
		Images []struct {
			Repository string    `json:"repository"`
			Tag        string    `json:"tag"`
			Created    time.Time `json:"created_at"`
		} `json:"images"`
	}
	if err = json.Unmarshal(b, &list); err != nil {
		return nil, fmt.Errorf("sbx template ls answered in a form unibox does not know; upgrade unibox: %w", err)
	}
	seen := map[string]bool{}
	out := []RuntimeImage{}
	for _, im := range list.Images {
		repo := strings.TrimPrefix(strings.TrimPrefix(im.Repository, "docker.io/"), "library/")
		ref := repo + ":" + im.Tag
		switch {
		case strings.HasPrefix(repo, "unibox/"):
			out = append(out, RuntimeImage{Ref: ref, Profile: strings.TrimPrefix(repo, "unibox/"), Created: im.Created})
		case repo == "sbx-kit-src":
			if m := sbxKitTag.FindStringSubmatch(im.Tag); m != nil {
				out = append(out, RuntimeImage{Ref: ref, Sandbox: m[1], Created: im.Created})
			}
		default:
			continue
		}
		seen[ref] = true
	}
	// Docker keeps a build's image too; one already gone from sbx's store
	// is listed for removal all the same.
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

// RemoveImage removes an image from sbx's store and from Docker. A sandbox
// made from it is not affected: checked, it keeps working without it.
func (r *SBX) RemoveImage(ctx context.Context, ref string) error {
	errSbx := runQuiet(hostCommand(ctx, "sbx", "template", "rm", "--force", ref))
	if strings.HasPrefix(ref, "unibox/") && runQuiet(hostCommand(ctx, "docker", "image", "rm", ref)) == nil {
		return nil
	}
	return errSbx
}

// Mount bind-mounts a host directory into the running sandbox at the same
// path (sbx mount, checked: live, writable, kept across a stop). It is
// idempotent in sbx, so a task prepared again mounts again harmlessly.
func (r *SBX) Mount(ctx context.Context, sandbox, host string) error {
	if err := runQuiet(hostCommand(ctx, "sbx", "mount", sandbox, host)); err != nil {
		return fmt.Errorf("sbx did not mount %s: %w", host, err)
	}
	return nil
}

func (r *SBX) Unmount(ctx context.Context, sandbox, host string) error {
	if err := runQuiet(hostCommand(ctx, "sbx", "umount", sandbox, host)); err != nil {
		return fmt.Errorf("sbx did not unmount %s: %w", host, err)
	}
	return nil
}
