package unibox

import (
	"context"
	"io"
	"os"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPortsArePublishedByTheEnvAndSwitchedWhileASandboxRuns(t *testing.T) {
	t.Parallel()
	m, f := managerFixture(t)
	ctx := context.Background()
	setPorts := func(ports map[string]Port) {
		t.Helper()
		ec := m.Config.Envs["dev"]
		ec.Ports = ports
		m.Config.Envs["dev"] = ec
		if err := m.Paths.SaveConfig(m.Config); err != nil {
			t.Fatal(err)
		}
	}
	setPorts(map[string]Port{"fe": {Spec: "3000:3000", Default: true}, "devtools": {Spec: "9333:9222", Default: false}})
	if _, err := m.Create(ctx, Request{Env: "dev"}, false); err != nil {
		t.Fatal(err)
	}
	// A port published by hand with sbx's own command is not unibox's to
	// take back.
	if err := f.PublishPort(ctx, "dev", "7000:7000", io.Discard); err != nil {
		t.Fatal(err)
	}
	if got := f.hostPorts("dev"); !slices.Equal(got, []int{3000, 7000}) {
		t.Fatalf("at start: %v", got)
	}
	// Switched while it runs: by name, and by a spec the env lacks.
	if err := m.SwitchPorts(ctx, "dev", map[string]bool{"devtools": true, "fe": false, "3001": true}); err != nil {
		t.Fatal(err)
	}
	if got := f.hostPorts("dev"); !slices.Equal(got, []int{7000, 9333, 49152}) {
		t.Fatalf("switched: %v", got)
	}
	ports, running, err := m.SandboxPorts(ctx, "dev")
	if err != nil || !running || len(ports) != 3 || ports[0].Name != "devtools" || !ports[0].On || ports[1].On || ports[2].Name != "3001" || ports[2].Named || ports[2].Published[0].HostPort != 49152 {
		t.Fatalf("%v %+v", err, ports)
	}
	if err := m.SwitchPorts(ctx, "dev", map[string]bool{"nope": true}); err == nil || !strings.Contains(err.Error(), "has no port nope") {
		t.Fatalf("%v", err)
	}
	// A named port on another host port, its sandbox port kept, until it
	// is switched again.
	if err := m.SwitchPorts(ctx, "dev", map[string]bool{"fe=13000": true}); err != nil {
		t.Fatal(err)
	}
	if got := f.hostPorts("dev"); !slices.Equal(got, []int{7000, 9333, 13000, 49152}) {
		t.Fatalf("moved: %v", got)
	}
	if ports, _, _ = m.SandboxPorts(ctx, "dev"); ports[1].Name != "fe" || ports[1].Spec != "13000:3000" || len(ports[1].Published) != 1 {
		t.Fatalf("%+v", ports)
	}
	for _, bad := range []string{"fe=x", "fe=0", "fe=070000", "nope=1", "3002=1"} {
		if err := m.SwitchPorts(ctx, "dev", map[string]bool{bad: true}); err == nil {
			t.Fatalf("%s was taken", bad)
		}
	}
	if err := m.SwitchPorts(ctx, "dev", map[string]bool{"fe=13000": false}); err == nil {
		t.Fatal("a host port was taken to switch a port off")
	}
	if err := m.SwitchPorts(ctx, "dev", map[string]bool{"fe": false}); err != nil {
		t.Fatal(err)
	}
	if b, _ := m.sandbox("dev"); b.PortMaps != nil || f.hostPorts("dev")[1] != 9333 || len(f.hostPorts("dev")) != 3 {
		t.Fatalf("%+v %v", b.PortMaps, f.hostPorts("dev"))
	}

	// sbx keeps ports across a stop, with an ephemeral one moved; the next
	// start goes back to the env's defaults.
	f.setState("dev", "stopped")
	if err := m.SwitchPorts(ctx, "dev", map[string]bool{"fe": true}); err == nil || !strings.Contains(err.Error(), "is not running") {
		t.Fatalf("%v", err)
	}
	f.mu.Lock()
	for i, q := range f.published["dev"] {
		if q.HostPort == 49152 {
			f.published["dev"][i].HostPort = 49153
		}
	}
	f.mu.Unlock()
	b, _ := m.sandbox("dev")
	if err := m.ensure(ctx, &b, mustEnv(t, m), false); err != nil {
		t.Fatal(err)
	}
	if got := f.hostPorts("dev"); !slices.Equal(got, []int{3000, 7000}) {
		t.Fatalf("after a start: %v", got)
	}
	if f.creates != 1 {
		t.Fatal("ports recreated the sandbox")
	}
	f.setState("dev", "running")

	// A change of the env's ports reaches a running sandbox at its next
	// session.
	setPorts(map[string]Port{"fe": {Spec: "3100:3000", Default: true}})
	b, _ = m.sandbox("dev")
	if err := m.ensure(ctx, &b, mustEnv(t, m), false); err != nil {
		t.Fatal(err)
	}
	if got := f.hostPorts("dev"); !slices.Equal(got, []int{3100, 7000}) {
		t.Fatalf("after an env change: %v", got)
	}
	if f.creates != 1 {
		t.Fatal("ports recreated the sandbox")
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
	var c struct {
		Ports map[string]Port `yaml:"ports"`
	}
	if err := yaml.Unmarshal([]byte("ports:\n  fe: '3000:3000'\n  devtools: {port: '9333:9222', default: off}\n  api: {port: 8080}\n"), &c); err != nil {
		t.Fatal(err)
	}
	if c.Ports["fe"] != (Port{Spec: "3000:3000", Default: true}) || c.Ports["devtools"] != (Port{Spec: "9333:9222", Default: false}) || c.Ports["api"] != (Port{Spec: "8080", Default: true}) {
		t.Fatalf("%+v", c.Ports)
	}
	for body, want := range map[string]string{
		"ports: {Fe: '3000'}":                         "lower case",
		"ports: {fe: '3000:x'}":                       "is not [[HOST_IP:]HOST_PORT:]SANDBOX_PORT",
		"ports: {fe: '3000:3000', be: '3000:4000'}":   "ask for the same host port 3000",
		"ports: {fe: {port: '3000', colour: red}}":    "not colour",
		"ports: {fe: {port: '3000', default: maybe}}": "on or off",
	} {
		var c struct {
			Ports map[string]Port `yaml:"ports"`
		}
		err := yaml.Unmarshal([]byte(body), &c)
		if err == nil {
			err = validatePorts(c.Ports)
		}
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", body, err)
		}
	}
}

func TestASandboxStartsWithTheTeamsPortsThatAreOn(t *testing.T) {
	t.Parallel()
	m, f := managerFixture(t)
	ctx := context.Background()
	file := m.Config.Envs["dev"].Files[0]
	if err := os.WriteFile(TeamFile(file), []byte("ports:\n  web: {port: '3000:3000', default: on}\n  debug: {port: '9229:9229'}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(ctx, Request{Env: "dev"}, false); err != nil {
		t.Fatal(err)
	}
	if got := f.hostPorts("dev"); !slices.Equal(got, []int{3000}) {
		t.Fatalf("%v", got)
	}
	ports, running, err := m.SandboxPorts(ctx, "dev")
	if err != nil || !running || len(ports) != 2 || !ports[0].Team || ports[0].Name != "debug" || ports[0].On || !ports[1].On {
		t.Fatalf("%+v %v %v", ports, running, err)
	}
}
