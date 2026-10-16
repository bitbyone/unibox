package unibox

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/tobola/unibox/internal/unibox/api"
)

// livePorts is the fake runtime with sbx's ports: published ones stay
// across a stop, an ephemeral one gets a host port of its own, and
// unpublishing takes the exact host port.
type livePorts struct {
	*fakeRuntime
	mu        sync.Mutex
	published map[string][]api.PublishedPort
	next      int
}

func (l *livePorts) PublishedPorts(_ context.Context, sandbox string) ([]api.PublishedPort, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.published[sandbox]), nil
}

func (l *livePorts) PublishPort(_ context.Context, sandbox, spec string, _ io.Writer) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	p, err := ParsePortSpec(spec)
	if err != nil {
		return err
	}
	q := api.PublishedPort{HostIP: "127.0.0.1", HostPort: p.HostPort, SandboxPort: p.SandboxPort, Protocol: "tcp4"}
	if p.HostPort == 0 {
		l.next++
		q.HostPort = 49151 + l.next
	}
	for _, o := range l.published[sandbox] {
		if o.HostPort == q.HostPort {
			return fmt.Errorf("409 Conflict: port %d is already published", q.HostPort)
		}
	}
	if l.published == nil {
		l.published = map[string][]api.PublishedPort{}
	}
	l.published[sandbox] = append(l.published[sandbox], q)
	return nil
}

func (l *livePorts) UnpublishPort(_ context.Context, sandbox string, q api.PublishedPort, _ io.Writer) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.published[sandbox] = slices.DeleteFunc(l.published[sandbox], func(o api.PublishedPort) bool { return o == q })
	return nil
}

func (l *livePorts) hostPorts(sandbox string) []int {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := []int{}
	for _, q := range l.published[sandbox] {
		out = append(out, q.HostPort)
	}
	slices.Sort(out)
	return out
}

func (l *livePorts) setState(name, state string) {
	l.fakeRuntime.mu.Lock()
	defer l.fakeRuntime.mu.Unlock()
	for i := range l.boxes {
		if l.boxes[i].Name == name {
			l.boxes[i].State = state
		}
	}
}

func TestPortsArePublishedByTheProfileAndSwitchedWhileASandboxRuns(t *testing.T) {
	t.Parallel()
	m, f := managerFixture(t)
	live := &livePorts{fakeRuntime: f}
	m.Runtimes = map[string]Runtime{"fake": live}
	ctx := context.Background()
	file := filepath.Join(m.Config.Environments, "sandboxes", "node", "sandbox.yaml")
	profile := "version: 5\nstart: start.sh\nagents: [claude]\nnetwork:\n  ports:\n    fe: '3000:3000'\n    devtools: {port: '9333:9222', default: off}\n"
	write(t, file, profile)
	if err := m.Build(ctx, "node", "fake", false); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(ctx, Request{NewSandbox: "dev", Profile: "node", Runtime: "fake"}); err != nil {
		t.Fatal(err)
	}
	// A port published by hand with the runtime's own command is not
	// unibox's to take back.
	if err := live.PublishPort(ctx, "unibox-dev", "7000:7000", io.Discard); err != nil {
		t.Fatal(err)
	}
	if got := live.hostPorts("unibox-dev"); !slices.Equal(got, []int{3000, 7000}) {
		t.Fatalf("at start: %v", got)
	}
	// Switched while it runs: by name, and by a spec the profile lacks.
	if err := m.SwitchPorts(ctx, "dev", map[string]bool{"devtools": true, "fe": false, "3001": true}); err != nil {
		t.Fatal(err)
	}
	if got := live.hostPorts("unibox-dev"); !slices.Equal(got, []int{7000, 9333, 49152}) {
		t.Fatalf("switched: %v", got)
	}
	if data, _ := os.ReadFile(file); string(data) != profile {
		t.Fatalf("a switch changed the profile:\n%s", data)
	}
	ports, running, err := m.SandboxPorts(ctx, "dev")
	if err != nil || !running || len(ports) != 3 || ports[0].Name != "devtools" || !ports[0].On || ports[1].On || ports[2].Name != "3001" || ports[2].Named || ports[2].Published[0].HostPort != 49152 {
		t.Fatalf("%v %+v", err, ports)
	}
	if err := m.SwitchPorts(ctx, "dev", map[string]bool{"nope": true}); err == nil || !strings.Contains(err.Error(), "has no port nope") {
		t.Fatalf("%v", err)
	}

	// The runtime keeps ports across a stop, with an ephemeral one moved;
	// the next start goes back to the profile's defaults.
	live.setState("unibox-dev", "stopped")
	if err := m.SwitchPorts(ctx, "dev", map[string]bool{"fe": true}); err == nil || !strings.Contains(err.Error(), "is not running") {
		t.Fatalf("%v", err)
	}
	live.mu.Lock()
	for i, q := range live.published["unibox-dev"] {
		if q.HostPort == 49152 {
			live.published["unibox-dev"][i].HostPort = 49153
		}
	}
	live.mu.Unlock()
	b, _ := m.sandbox("dev")
	r, _ := m.Runtime("fake")
	p, _ := m.profile("node")
	creates := f.creates
	if err := m.ensure(ctx, &b, r, p, false, false); err != nil {
		t.Fatal(err)
	}
	if got := live.hostPorts("unibox-dev"); !slices.Equal(got, []int{3000, 7000}) {
		t.Fatalf("after a start: %v", got)
	}
	if f.creates != creates {
		t.Fatal("ports recreated the sandbox")
	}
	live.setState("unibox-dev", "running")

	// A change of the profile's ports is applied live to a running sandbox.
	write(t, file, "version: 5\nstart: start.sh\nagents: [claude]\nnetwork:\n  ports:\n    fe: '3100:3000'\n")
	if err := m.ApplyPorts(ctx, "node"); err != nil {
		t.Fatal(err)
	}
	if got := live.hostPorts("unibox-dev"); !slices.Equal(got, []int{3100, 7000}) {
		t.Fatalf("after a profile change: %v", got)
	}
	b, _ = m.sandbox("dev")
	p, _ = m.profile("node")
	if err := m.ensure(ctx, &b, r, p, false, false); err != nil || f.creates != creates {
		t.Fatalf("%v; %d creates", err, f.creates-creates)
	}
}

func TestAPortIsASpecOrASpecWithItsDefault(t *testing.T) {
	t.Parallel()
	for spec, want := range map[string]PortSpec{
		"3000":                 {SandboxPort: 3000},
		"8080:3000":            {HostPort: 8080, SandboxPort: 3000},
		"127.0.0.1:8080:3000":  {HostIP: "127.0.0.1", HostPort: 8080, SandboxPort: 3000},
		"[::1]:8080:3000/tcp6": {HostIP: "::1", HostPort: 8080, SandboxPort: 3000, Protocol: "tcp6"},
		"53/udp":               {SandboxPort: 53, Protocol: "udp"},
	} {
		if got, err := ParsePortSpec(spec); err != nil || got != want {
			t.Errorf("%s: %+v %v", spec, got, err)
		}
	}
	for _, spec := range []string{"", "0", "70000", "a:3000", "3000/sctp", ":3000", "08:3000"} {
		if _, err := ParsePortSpec(spec); err == nil {
			t.Errorf("%q was taken", spec)
		}
	}
	root := profileFixture(t)
	file := filepath.Join(root, "sandboxes", "node", "sandbox.yaml")
	write(t, file, "version: 5\nstart: start.sh\nagents: [claude]\nnetwork:\n  ports:\n    fe: '3000:3000'\n    devtools: {port: '9333:9222', default: off}\n    api: {port: 8080}\n")
	p, err := LoadProfile(root, "node")
	if err != nil {
		t.Fatal(err)
	}
	if p.Ports["fe"] != (Port{"3000:3000", true}) || p.Ports["devtools"] != (Port{"9333:9222", false}) || p.Ports["api"] != (Port{"8080", true}) {
		t.Fatalf("%+v", p.Ports)
	}
	for body, want := range map[string]string{
		"ports: {Fe: '3000'}":                         "lower case",
		"ports: {fe: '3000:x'}":                       "is not [[HOST_IP:]HOST_PORT:]SANDBOX_PORT",
		"ports: {fe: '3000:3000', be: '3000:4000'}":   "ask for the same host port 3000",
		"ports: {fe: {port: '3000', colour: red}}":    "not colour",
		"ports: {fe: {port: '3000', default: maybe}}": "on or off",
	} {
		write(t, file, "version: 5\nstart: start.sh\nagents: [claude]\nnetwork: {"+body+"}\n")
		if _, err := LoadProfile(root, "node"); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", body, err)
		}
	}
}

func TestFormat2NetworkRulesMoveUnderNetwork(t *testing.T) {
	t.Parallel()
	root := profileFixture(t)
	file := filepath.Join(root, "sandboxes", "node", "sandbox.yaml")
	write(t, file, "version: 2\ndockerfile: Dockerfile\nstart: start.sh\nagents: [claude]\n"+format2NetworkComment+"\nallow_network: [jitpack.io] # Kotlin\n# Mine.\ndeny_network: [gist.github.com]\n")
	report, err := Init(root, "node", nil, "2", "1g")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Migrated) != 1 || report.Migrated[0] != "sandboxes/node/sandbox.yaml from format 2 to 5" {
		t.Fatalf("%q", report.Migrated)
	}
	p, err := LoadProfile(root, "node")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(p.AllowNetwork, []string{"jitpack.io"}) || !slices.Equal(p.DenyNetwork, []string{"gist.github.com"}) {
		t.Fatalf("%+v", p.Network)
	}
	data, _ := os.ReadFile(file)
	s := string(data)
	if !strings.Contains(s, "network:\n  allow: [jitpack.io] # Kotlin\n  # Mine.\n  deny: [gist.github.com]\n") || strings.Contains(s, "Hosts beyond the runtime's policy") {
		t.Fatalf("%s", s)
	}
}

func TestFormat3BootstrapIsDroppedAndSaidSo(t *testing.T) {
	t.Parallel()
	root := profileFixture(t)
	file := filepath.Join(root, "sandboxes", "node", "sandbox.yaml")
	write(t, file, "version: 3\ndockerfile: Dockerfile\nstart: start.sh\nbootstrap: .agent/bootstrap.sh\nagents: [claude]\n")
	report, err := Init(root, "node", nil, "2", "1g")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Migrated) != 2 || !strings.HasPrefix(report.Migrated[1], "bootstrap .agent/bootstrap.sh left out") {
		t.Fatalf("%q", report.Migrated)
	}
	if data, _ := os.ReadFile(file); strings.Contains(string(data), "bootstrap") {
		t.Fatalf("%s", data)
	}
	write(t, file, "version: 5\nstart: start.sh\nbootstrap: x.sh\nagents: [claude]\n")
	if _, err := LoadProfile(root, "node"); err == nil || !strings.Contains(err.Error(), "bootstrap") {
		t.Fatalf("format 4 took bootstrap: %v", err)
	}
}

func TestFormat4DockerfileBecomesSandboxDockerfile(t *testing.T) {
	t.Parallel()
	root := profileFixture(t)
	dir := filepath.Join(root, "sandboxes", "node")
	_ = os.Remove(filepath.Join(dir, "sandbox.dockerfile"))
	write(t, filepath.Join(dir, "Dockerfile"), "FROM example/shell\n")
	write(t, filepath.Join(dir, "sandbox.yaml"), "version: 4\n# Mine.\ndockerfile: Dockerfile\nstart: start.sh\nagents: [claude]\n")
	if _, err := LoadProfile(root, "node"); err == nil || !strings.Contains(err.Error(), "run unibox init node") {
		t.Fatalf("%v", err)
	}
	if _, err := Init(root, "node", nil, "2", "1g"); err != nil {
		t.Fatal(err)
	}
	p, err := LoadProfile(root, "node")
	if err != nil || p.Dockerfile != "sandbox.dockerfile" {
		t.Fatalf("%v %q", err, p.Dockerfile)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "sandbox.yaml")); strings.Contains(string(data), "dockerfile:") || !strings.Contains(string(data), "# Mine.") {
		t.Fatalf("%s", data)
	}
	if _, err := os.Stat(filepath.Join(dir, "Dockerfile")); err == nil {
		t.Fatal("the old Dockerfile stayed")
	}
	// A Dockerfile left beside it, both or neither of a file and an image,
	// are named.
	for setup, want := range map[string]string{
		"Dockerfile": "rename Dockerfile to it",
		"image":      "has sandbox.dockerfile and names image",
		"neither":    "has no image: add sandbox.dockerfile",
	} {
		profile := "version: 5\nstart: start.sh\nagents: [claude]\n"
		switch setup {
		case "Dockerfile":
			write(t, filepath.Join(dir, "Dockerfile"), "FROM x\n")
		case "image":
			profile += "image: example/image\n"
		case "neither":
			_ = os.Rename(filepath.Join(dir, "sandbox.dockerfile"), filepath.Join(dir, "_data", "aside"))
		}
		write(t, filepath.Join(dir, "sandbox.yaml"), profile)
		if _, err := LoadProfile(root, "node"); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", setup, err)
		}
		_ = os.Remove(filepath.Join(dir, "Dockerfile"))
		_ = os.Rename(filepath.Join(dir, "_data", "aside"), filepath.Join(dir, "sandbox.dockerfile"))
	}
}
