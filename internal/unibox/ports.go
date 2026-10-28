package unibox

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/tobola/unibox/internal/unibox/api"
	"gopkg.in/yaml.v3"
)

// Port is one of an env's ports, published on the host's loopback by
// name: Spec is [[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL], as Docker
// says it; Default is whether a sandbox starts with it published.
//
// In config.yaml Spec may be empty: the entry then only switches the
// team's port of its name on or off. Description and Team are the team's,
// never kept with the person's.
type Port struct {
	Spec        string `yaml:"port" json:"port,omitempty"`
	Default     bool   `yaml:"default" json:"default"`
	Description string `yaml:"-" json:"-"`
	Team        bool   `yaml:"-" json:"-"`
	// From is the layer that names it, FromRepo to FromYours, and
	// DefaultFrom the one that says whether it is on at start.
	From        string `yaml:"-" json:"-"`
	DefaultFrom string `yaml:"-" json:"-"`
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

// MarshalYAML writes a port on by default as its spec alone, as a person
// would write it.
func (p Port) MarshalYAML() (any, error) {
	if p.Spec == "" {
		return map[string]string{"default": map[bool]string{true: "on", false: "off"}[p.Default]}, nil
	}
	if p.Default {
		return p.Spec, nil
	}
	return map[string]string{"port": p.Spec, "default": "off"}, nil
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

// validatePorts checks an env's ports: names a command can say, specs
// that parse, and no host port asked for twice. A port with no spec is a
// person's switch of the team's port, checked with the team's.
func validatePorts(ports map[string]Port) error {
	hosts := map[string]string{}
	for _, name := range sortedKeys(ports) {
		if !portName.MatchString(name) {
			return fmt.Errorf("port name %q must be lower case letters, digits and dashes, starting with a letter", name)
		}
		if ports[name].Spec == "" {
			continue
		}
		spec, err := ParsePortSpec(ports[name].Spec)
		if err != nil {
			return fmt.Errorf("port %s: %w", name, err)
		}
		if spec.HostPort != 0 {
			key := spec.HostIP + ":" + strconv.Itoa(spec.HostPort) + "/" + strings.TrimRight(spec.Protocol, "46")
			if other, ok := hosts[key]; ok {
				return fmt.Errorf("ports %s and %s ask for the same host port %d", other, name, spec.HostPort)
			}
			hosts[key] = name
		}
	}
	return nil
}

// wantedPorts is what a sandbox should have published: each of the env's
// ports by its default unless the sandbox's own switch says otherwise, and
// the ports switched on for it alone, by their spec.
func wantedPorts(ports map[string]Port, b api.Sandbox) map[string]string {
	want := map[string]string{}
	for name, port := range ports {
		on := port.Default
		if v, ok := b.PortSwitches[name]; ok {
			on = v
		}
		if on {
			want[name] = cmp.Or(b.PortMaps[name], port.Spec)
		}
	}
	for key, on := range b.PortSwitches {
		if _, named := ports[key]; !named && on {
			want[key] = key
		}
	}
	return want
}

// syncPorts brings a running sandbox's published ports to what it should
// have, touching only ports unibox published or should; a port published
// by hand with sbx ports stays.
func (m *Manager) syncPorts(ctx context.Context, b *api.Sandbox, env Env) error {
	want := wantedPorts(env.Ports, *b)
	if len(want) == 0 && len(b.Ports) == 0 {
		return nil
	}
	actual, err := m.SBX.PublishedPorts(ctx, b.Name)
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
					if err := m.SBX.UnpublishPort(ctx, b.Name, q, m.Log); err != nil {
						return err
					}
					removed = true
				}
			}
		}
		changes = append(changes, "- "+name)
	}
	if removed {
		if actual, err = m.SBX.PublishedPorts(ctx, b.Name); err != nil {
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
			if err := m.SBX.PublishPort(ctx, b.Name, spec, m.Log); err != nil {
				return fmt.Errorf("port %s (%s) of sandbox %s: %w", name, spec, b.Name, err)
			}
		}
		if b.Ports[name] != spec {
			changes = append(changes, "+ "+name)
		}
		published[name] = spec
	}
	if len(changes) > 0 {
		fmt.Fprintf(m.Log, "ports of sandbox %s: %s\n", b.Name, strings.Join(changes, ", "))
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

// PortState is one port of a sandbox as unibox port ls shows it.
type PortState struct {
	Name, Spec, Description string
	// Team is whether the team named it, in its unibox file.
	Team    bool
	From    string
	Default bool
	// DefaultYours says the person's config.yaml decides Default.
	DefaultYours bool
	// Named is false for a port switched on for the sandbox alone.
	Named     bool
	On        bool
	Published []api.PublishedPort
}

// SandboxPorts lists a sandbox's ports: its env's and those switched on
// for it alone, each with where it is published now.
func (m *Manager) SandboxPorts(ctx context.Context, name string) ([]PortState, bool, error) {
	b, env, err := m.Definition(Request{Sandbox: name})
	if err != nil {
		return nil, false, err
	}
	if b.State == "missing" {
		return nil, false, Fail(4, "sandbox_not_found", "sandbox %s is not unibox's; unibox sandbox ls lists them", name)
	}
	ports := env.Ports
	box, exists, err := m.findBox(ctx, b)
	if err != nil {
		return nil, false, err
	}
	running := exists && box.State == "running"
	var actual []api.PublishedPort
	if running {
		if actual, err = m.SBX.PublishedPorts(ctx, b.Name); err != nil {
			return nil, false, err
		}
	}
	want := map[string]string{}
	if running {
		want = wantedPorts(ports, b)
	}
	out := []PortState{}
	add := func(name string, p Port, named bool) {
		s := PortState{Name: name, Spec: p.Spec, Description: p.Description, Team: p.Team, From: p.From, Default: p.Default, DefaultYours: IsYours(p.DefaultFrom), Named: named}
		spec := p.Spec
		if w, on := want[name]; on {
			spec, s.Spec, s.On = w, w, true
			parsed, _ := ParsePortSpec(spec)
			for _, q := range actual {
				if parsed.matches(q) {
					s.Published = append(s.Published, q)
				}
			}
		}
		out = append(out, s)
	}
	for _, n := range sortedKeys(ports) {
		add(n, ports[n], true)
	}
	for _, key := range sortedKeys(b.PortSwitches) {
		if _, named := ports[key]; !named && b.PortSwitches[key] {
			add(key, Port{Spec: key}, false)
		}
	}
	return out, running, nil
}

// withHost is the spec with another host port, its host address and
// protocol kept.
func (p PortSpec) withHost(port int) string {
	s := strconv.Itoa(port) + ":" + strconv.Itoa(p.SandboxPort)
	if p.HostIP != "" {
		ip := p.HostIP
		if strings.Contains(ip, ":") {
			ip = "[" + ip + "]"
		}
		s = ip + ":" + s
	}
	if p.Protocol != "" {
		s += "/" + p.Protocol
	}
	return s
}

// SetPortDefault has a port of an env on or off at each start of its
// sandboxes, in the person's config.yaml under the env, or with EnvRepo
// under every env of its repository; with neither, the person's say there
// is taken back, leaving what the layers under it say. A port the person
// names only to switch is dropped once the layers under it say the same,
// so config.yaml keeps no switch that does nothing.
func (m *Manager) SetPortDefault(s Scope, port string, on *bool) error {
	c, err := m.Paths.LoadConfig()
	if err != nil {
		return err
	}
	ec, ok := c.Envs[s.Env]
	if !ok {
		return Fail(4, "env_not_found", "there is no env %s; unibox env ls lists them", s.Env)
	}
	key := ""
	if s.EnvRepo {
		if key, err = c.EnvRepoOf(s.Env); err != nil {
			return err
		}
	}
	ports, err := envPorts(ec, c)
	if err != nil {
		return err
	}
	if _, named := ports[port]; !named {
		return fmt.Errorf("env %s has no port %s; unibox port ls lists them", s.Env, port)
	}
	c.Envs, c.EnvRepos = maps.Clone(c.Envs), maps.Clone(c.EnvRepos)
	er := c.EnvRepos[key]
	scoped := ec.Ports
	if s.EnvRepo {
		scoped = er.Ports
	}
	mine := scoped[port]
	scoped = without(scoped, port)
	if scoped == nil {
		scoped = map[string]Port{}
	}
	// What the layers beneath say is what is left with the port taken out
	// of this scope and those over it.
	lowerEnv, lower := ec, c
	lowerEnv.Ports = without(ec.Ports, port)
	if s.EnvRepo {
		lowerRepo := er
		lowerRepo.Ports = scoped
		lower.EnvRepos = maps.Clone(c.EnvRepos)
		lower.setEnvRepo(key, lowerRepo)
	}
	under, err := envPorts(lowerEnv, lower)
	if err != nil {
		return err
	}
	beneath, below := under[port]
	switch {
	case on == nil && mine.Spec == "":
	case on == nil:
		// A port of the person's own mapping keeps it; with no default
		// written, the format reads it as on, so the one beneath is written.
		mine.Default = !below || beneath.Default
		scoped[port] = mine
	case below && mine.Spec == "" && beneath.Default == *on:
	default:
		mine.Default = *on
		scoped[port] = mine
	}
	if len(scoped) == 0 {
		scoped = nil
	}
	if s.EnvRepo {
		er.Ports = scoped
		c.setEnvRepo(key, er)
	} else {
		ec.Ports = scoped
		c.Envs[s.Env] = ec
	}
	if err = m.Paths.SaveConfig(c); err != nil {
		return err
	}
	m.Config.Envs, m.Config.EnvRepos = c.Envs, c.EnvRepos
	return nil
}

// SwitchPorts turns a running sandbox's ports on or off for as long as it
// runs: by the env's name, by name=HOST_PORT for one of them on another
// host port, or, for a port the env does not name, by its spec. The next
// start of the sandbox goes back to the env's defaults.
func (m *Manager) SwitchPorts(ctx context.Context, name string, switches map[string]bool) error {
	guard, err := m.guard(name)
	if err != nil {
		return err
	}
	defer guard.Close()
	b, env, err := m.Definition(Request{Sandbox: name})
	if err != nil {
		return err
	}
	box, exists, err := m.findBox(ctx, b)
	if err != nil {
		return err
	}
	if b.State == "missing" || !exists || box.State != "running" {
		return fmt.Errorf("sandbox %s is not running; a port switched now lasts while it runs - start it with unibox resume or unibox sandbox shell, or say default: on for the port in your config.yaml", name)
	}
	if b.PortSwitches == nil {
		b.PortSwitches = map[string]bool{}
	}
	if b.PortMaps == nil {
		b.PortMaps = map[string]string{}
	}
	for key, on := range switches {
		key, host, moved := strings.Cut(key, "=")
		port, named := env.Ports[key]
		if !named {
			if _, err := ParsePortSpec(key); moved || err != nil {
				return fmt.Errorf("env %s has no port %s, and it is not a mapping either; unibox port ls -s %s lists them", env.Name, key, name)
			}
		}
		delete(b.PortMaps, key)
		if moved {
			n, err := strconv.Atoi(host)
			if !on || err != nil || n < 1 || n > 65535 || host != strconv.Itoa(n) {
				return fmt.Errorf("%s=%s: give a host port from 1 to 65535 to switch the port on at, e.g. %s=15173", key, host, key)
			}
			spec, _ := ParsePortSpec(port.Spec)
			b.PortMaps[key] = spec.withHost(n)
		}
		b.PortSwitches[key] = on
	}
	if len(b.PortMaps) == 0 {
		b.PortMaps = nil
	}
	return m.syncPorts(ctx, &b, env)
}
