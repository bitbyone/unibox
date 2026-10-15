package unibox

import (
	"context"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/tobola/unibox/internal/unibox/api"
	"gopkg.in/yaml.v3"
)

// Port is one of a profile's ports, published on the host's loopback by
// name: Spec is [[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL], as Docker
// says it; Default is whether a sandbox starts with it published.
type Port struct {
	Spec    string `yaml:"port" json:"port"`
	Default bool   `yaml:"default" json:"default"`
}

// UnmarshalYAML takes a port as its spec alone, on by default, or as
// {port, default}; default is on or off as well as true or false.
func (p *Port) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		*p = Port{Spec: n.Value, Default: true}
		return nil
	case yaml.MappingNode:
		*p = Port{Default: true}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			switch k.Value {
			case "port":
				p.Spec = v.Value
			case "default":
				on, ok := onOff(v.Value)
				if !ok {
					return fmt.Errorf("line %d: a port's default is on or off", v.Line)
				}
				p.Default = on
			default:
				return fmt.Errorf("line %d: a port says port and default, not %s", k.Line, k.Value)
			}
		}
		return nil
	}
	return fmt.Errorf("line %d: a port is its spec, such as '3000:3000', or {port: ..., default: off}", n.Line)
}

func onOff(s string) (bool, bool) {
	switch strings.ToLower(s) {
	case "on", "true", "yes":
		return true, true
	case "off", "false", "no":
		return false, true
	}
	return false, false
}

// PortSpec is a spec read into its parts; HostPort 0 lets the runtime pick
// one, and an empty HostIP is the loopback.
type PortSpec struct {
	HostIP      string
	HostPort    int
	SandboxPort int
	Protocol    string
}

var (
	portName  = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	protocols = []string{"tcp", "tcp4", "tcp6", "udp", "udp4", "udp6"}
)

func ParsePortSpec(s string) (PortSpec, error) {
	bad := fmt.Errorf("port %q is not [[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL], e.g. 3000 or 8080:3000", s)
	var p PortSpec
	rest, proto, hasProto := strings.Cut(s, "/")
	if hasProto {
		if !slices.Contains(protocols, proto) {
			return p, bad
		}
		p.Protocol = proto
	}
	number := func(v string) (int, bool) {
		n, err := strconv.Atoi(v)
		return n, err == nil && n >= 1 && n <= 65535 && v == strconv.Itoa(n)
	}
	i := strings.LastIndex(rest, ":")
	var ok bool
	if p.SandboxPort, ok = number(rest[i+1:]); !ok {
		return p, bad
	}
	if i < 0 {
		return p, nil
	}
	rest = rest[:i]
	i = strings.LastIndex(rest, ":")
	if p.HostPort, ok = number(rest[i+1:]); !ok {
		return p, bad
	}
	if i >= 0 {
		p.HostIP = strings.Trim(rest[:i], "[]")
		if p.HostIP == "" {
			return p, bad
		}
	}
	return p, nil
}

// matches says whether a published port is this spec's, as the runtime
// lists it: tcp4 when the spec names no protocol, tcp either family.
func (p PortSpec) matches(q api.PublishedPort) bool {
	if q.SandboxPort != p.SandboxPort || (p.HostPort != 0 && q.HostPort != p.HostPort) || (p.HostIP != "" && q.HostIP != p.HostIP) {
		return false
	}
	switch p.Protocol {
	case "":
		return q.Protocol == "tcp4"
	case "tcp", "udp":
		return strings.TrimRight(q.Protocol, "46") == p.Protocol
	}
	return q.Protocol == p.Protocol
}

// validatePorts checks a profile's ports: names a command can say, specs
// that parse, and no host port asked for twice.
func validatePorts(ports map[string]Port) error {
	hosts := map[string]string{}
	for _, name := range sortedKeys(ports) {
		if !portName.MatchString(name) {
			return fmt.Errorf("network.ports name %q must be lower case letters, digits and dashes, starting with a letter", name)
		}
		spec, err := ParsePortSpec(ports[name].Spec)
		if err != nil {
			return fmt.Errorf("network.ports %s: %w", name, err)
		}
		if spec.HostPort != 0 {
			key := spec.HostIP + ":" + strconv.Itoa(spec.HostPort) + "/" + strings.TrimRight(spec.Protocol, "46")
			if other, ok := hosts[key]; ok {
				return fmt.Errorf("network.ports %s and %s ask for the same host port %d", other, name, spec.HostPort)
			}
			hosts[key] = name
		}
	}
	return nil
}

// PortPublisher is a runtime that publishes a running sandbox's ports on
// the host and takes them back, without a recreation.
type PortPublisher interface {
	PublishedPorts(ctx context.Context, sandbox string) ([]api.PublishedPort, error)
	PublishPort(ctx context.Context, sandbox, spec string, log io.Writer) error
	UnpublishPort(ctx context.Context, sandbox string, port api.PublishedPort, log io.Writer) error
}

// wantedPorts is what a sandbox should have published: each of the
// profile's ports by its default unless the sandbox's own switch says
// otherwise, and the ports switched on for it alone, by their spec.
func wantedPorts(p Profile, b api.Sandbox) map[string]string {
	want := map[string]string{}
	for name, port := range p.Ports {
		on := port.Default
		if v, ok := b.PortSwitches[name]; ok {
			on = v
		}
		if on {
			want[name] = port.Spec
		}
	}
	for key, on := range b.PortSwitches {
		if _, named := p.Ports[key]; !named && on {
			want[key] = key
		}
	}
	return want
}

// syncPorts brings a running sandbox's published ports to what it should
// have, touching only ports unibox published or should; a port published
// by hand with the runtime's own command stays.
func (m *Manager) syncPorts(ctx context.Context, b *api.Sandbox, r Runtime, p Profile) error {
	pub, ok := r.(PortPublisher)
	if !ok {
		return nil
	}
	want := wantedPorts(p, *b)
	actual, err := pub.PublishedPorts(ctx, b.RuntimeName)
	if err != nil {
		return err
	}
	changes, removed := []string{}, false
	for _, name := range sortedKeys(b.Ports) {
		spec := b.Ports[name]
		if want[name] == spec {
			continue
		}
		if parsed, err := ParsePortSpec(spec); err == nil {
			for _, q := range actual {
				if parsed.matches(q) {
					if err := pub.UnpublishPort(ctx, b.RuntimeName, q, m.Log); err != nil {
						return err
					}
					removed = true
				}
			}
		}
		changes = append(changes, "- "+name)
	}
	if removed {
		if actual, err = pub.PublishedPorts(ctx, b.RuntimeName); err != nil {
			return err
		}
	}
	published := map[string]string{}
	for _, name := range sortedKeys(want) {
		spec := want[name]
		parsed, err := ParsePortSpec(spec)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(actual, parsed.matches) {
			if err := pub.PublishPort(ctx, b.RuntimeName, spec, m.Log); err != nil {
				return fmt.Errorf("port %s (%s) of sandbox %s: %w", name, spec, b.Name, err)
			}
		}
		if b.Ports[name] != spec {
			changes = append(changes, "+ "+name)
		}
		published[name] = spec
	}
	if len(changes) > 0 {
		fmt.Fprintf(m.Log, "ports of sandbox %s, applied live: %s\n", b.Name, strings.Join(changes, ", "))
	}
	if len(published) == 0 {
		published = nil
	}
	if maps.Equal(b.Ports, published) {
		return nil
	}
	b.Ports = published
	return m.saveBox(*b)
}

// PortState is one port of a sandbox as unibox port shows it.
type PortState struct {
	Name, Spec string
	Default    bool
	// Named is false for a port switched on for the sandbox alone.
	Named     bool
	On        bool
	Published []api.PublishedPort
}

// SandboxPorts lists a sandbox's ports: the profile's and those switched
// on for it alone, each with where it is published now.
func (m *Manager) SandboxPorts(ctx context.Context, name string) ([]PortState, bool, error) {
	b, err := m.sandbox(name)
	if err != nil {
		return nil, false, err
	}
	r, err := m.Runtime(b.Runtime)
	if err != nil {
		return nil, false, err
	}
	p, err := m.profile(b.Profile)
	if err != nil {
		return nil, false, err
	}
	box, exists, err := m.findBox(ctx, r, b)
	if err != nil {
		return nil, false, err
	}
	running := exists && box.State == "running"
	var actual []api.PublishedPort
	if pub, ok := r.(PortPublisher); ok && running {
		if actual, err = pub.PublishedPorts(ctx, b.RuntimeName); err != nil {
			return nil, false, err
		}
	}
	want := map[string]string{}
	if running {
		want = wantedPorts(p, b)
	}
	out := []PortState{}
	add := func(name, spec string, def, named bool) {
		s := PortState{Name: name, Spec: spec, Default: def, Named: named}
		if _, s.On = want[name]; s.On {
			parsed, _ := ParsePortSpec(spec)
			for _, q := range actual {
				if parsed.matches(q) {
					s.Published = append(s.Published, q)
				}
			}
		}
		out = append(out, s)
	}
	for _, n := range sortedKeys(p.Ports) {
		add(n, p.Ports[n].Spec, p.Ports[n].Default, true)
	}
	for _, key := range sortedKeys(b.PortSwitches) {
		if _, named := p.Ports[key]; !named && b.PortSwitches[key] {
			add(key, key, false, false)
		}
	}
	return out, running, nil
}

// SwitchPorts turns a running sandbox's ports on or off for as long as it
// runs, by the profile's name or, for a port the profile does not name, by
// its spec; the profile stays as it is, and the next start of the sandbox
// goes back to the profile's defaults.
func (m *Manager) SwitchPorts(ctx context.Context, name string, switches map[string]bool) error {
	guard, err := m.guard(name)
	if err != nil {
		return err
	}
	defer guard.Close()
	b, err := m.sandbox(name)
	if err != nil {
		return err
	}
	r, err := m.Runtime(b.Runtime)
	if err != nil {
		return err
	}
	if _, ok := r.(PortPublisher); !ok {
		return fmt.Errorf("runtime %s cannot publish ports", b.Runtime)
	}
	p, err := m.profile(b.Profile)
	if err != nil {
		return err
	}
	box, exists, err := m.findBox(ctx, r, b)
	if err != nil {
		return err
	}
	if !exists || box.State != "running" {
		return fmt.Errorf("sandbox %s is not running; a port switched now lasts while it runs - start it with unibox resume or unibox shell, or change the profile's default with unibox network port", name)
	}
	if b.PortSwitches == nil {
		b.PortSwitches = map[string]bool{}
	}
	for key, on := range switches {
		if _, named := p.Ports[key]; !named {
			if _, err := ParsePortSpec(key); err != nil {
				return fmt.Errorf("profile %s has no port %s, and it is not a port spec either; unibox port %s lists them", p.Name, key, name)
			}
		}
		b.PortSwitches[key] = on
	}
	return m.syncPorts(ctx, &b, r, p)
}

// ApplyPorts brings every running sandbox of a profile to its ports.
func (m *Manager) ApplyPorts(ctx context.Context, profile string) error {
	p, err := m.profile(profile)
	if err != nil {
		return err
	}
	s, err := m.Paths.ReadState()
	if err != nil {
		return err
	}
	for _, name := range sortedKeys(s.Sandboxes) {
		b := s.Sandboxes[name]
		if b.Profile != p.Name {
			continue
		}
		r, err := m.Runtime(b.Runtime)
		if err != nil {
			return err
		}
		box, exists, err := m.findBox(ctx, r, b)
		if err != nil || !exists || box.State != "running" {
			continue
		}
		guard, err := m.guard(name)
		if err != nil {
			return err
		}
		err = m.syncPorts(ctx, &b, r, p)
		guard.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
