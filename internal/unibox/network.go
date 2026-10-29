package unibox

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// A sandbox reaches what sbx's defaults and its env's kits allow, and the
// hosts the person adds for it. Theirs are kept in config.yaml under the
// sandbox's name, since sbx keeps a sandbox's rules only while the sandbox
// is there: unibox applies them when they change and again after each
// create. Seen beside the hosts the policy refused, they show what the
// env's files leave out.

// NetworkView is what decides which hosts a sandbox reaches.
type NetworkView struct {
	// Yours are the person's rules for the sandbox; Applied says sbx has
	// them, which it cannot while the sandbox is not created.
	Yours []AppliedRule
	// Others are rules sbx has for the sandbox, or for every one, that
	// are not unibox's: added with sbx itself, or approved there.
	Others []NetworkRule
	// Env are the hosts the env's kits allow, Defaults sbx's own rules.
	Env      []string
	Defaults []NetworkRule
	Blocked  []BlockedHost
}

type AppliedRule struct {
	HostRule
	Applied bool
}

// DefaultHosts counts the hosts sbx's defaults allow.
func (v NetworkView) DefaultHosts() int {
	n := 0
	for _, r := range v.Defaults {
		n += len(r.Hosts)
	}
	return n
}

// Network reads what decides the hosts a sandbox reaches.
func (m *Manager) Network(ctx context.Context, sandbox string) (NetworkView, error) {
	v := NetworkView{Others: []NetworkRule{}, Env: []string{}, Defaults: []NetworkRule{}, Blocked: []BlockedHost{}}
	var blocked []BlockedHost
	var blockedErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		blocked, blockedErr = m.SBX.NetworkBlocked(ctx, sandbox)
	}()
	rules, err := m.SBX.NetworkRules(ctx, sandbox)
	<-done
	if err != nil {
		return v, err
	}
	// A sandbox sbx has no log for yet has nothing blocked.
	if blockedErr == nil {
		blocked = slices.DeleteFunc(blocked, func(b BlockedHost) bool { return m.allows(sandbox, b.Host) })
		v.Blocked = append(v.Blocked, blocked...)
	}
	mine := m.Config.Sandboxes[sandbox].Network
	for _, r := range mine {
		v.Yours = append(v.Yours, AppliedRule{HostRule: r, Applied: hasRule(rules, sandbox, r)})
	}
	for _, r := range rules {
		switch {
		case r.From == "default":
			v.Defaults = append(v.Defaults, r)
		case r.From == "kit" || !r.Editable:
			v.Env = append(v.Env, r.Hosts...)
		default:
			// A rule of sbx's may hold several hosts; those of the person's
			// are not told twice.
			hosts := slices.DeleteFunc(slices.Clone(r.Hosts), func(h string) bool {
				return r.Sandbox == sandbox && slices.ContainsFunc(mine, func(x HostRule) bool { return x.Host() == h && x.Decision() == r.Decision })
			})
			if len(hosts) > 0 {
				r.Hosts = hosts
				v.Others = append(v.Others, r)
			}
		}
	}
	return v, nil
}

// allows says whether the person allows a host the log names, which may
// carry a port their rule leaves out.
func (m *Manager) allows(sandbox, host string) bool {
	for _, r := range m.Config.Sandboxes[sandbox].Network {
		if r.Allow != "" && (r.Allow == host || strings.TrimSuffix(host, ":443") == r.Allow || strings.TrimSuffix(host, ":80") == r.Allow) {
			return true
		}
	}
	return false
}

func hasRule(rules []NetworkRule, sandbox string, r HostRule) bool {
	for _, v := range rules {
		if v.Sandbox == sandbox && v.Decision == r.Decision() && slices.Contains(v.Hosts, r.Host()) {
			return true
		}
	}
	return false
}

// SetHostRules makes the hosts the person's rule for a sandbox, in place
// of one they had for them, and applies it while sbx has the sandbox; it
// says whether it did.
func (m *Manager) SetHostRules(ctx context.Context, sandbox, decision, protocol string, hosts []string) (bool, error) {
	rules := []HostRule{}
	for _, h := range hosts {
		r := HostRule{Protocol: protocol}
		if decision == "deny" {
			r.Deny = h
		} else {
			r.Allow = h
		}
		if protocol == "tcp" {
			r.Protocol = ""
		}
		if err := r.Validate(); err != nil {
			return false, err
		}
		rules = append(rules, r)
	}
	exists, err := m.sbxHas(ctx, sandbox)
	if err != nil {
		return false, err
	}
	before := m.Config.Sandboxes[sandbox].Network
	err = m.updateNetwork(sandbox, func(list []HostRule) []HostRule {
		list = slices.DeleteFunc(list, func(r HostRule) bool { return slices.Contains(hosts, r.Host()) })
		return append(list, rules...)
	})
	if err != nil || !exists {
		return false, err
	}
	// A rule the host had is taken away first, so that one of another
	// decision or protocol does not stay beside the new one.
	if slices.ContainsFunc(before, func(r HostRule) bool { return slices.Contains(hosts, r.Host()) }) {
		_ = m.SBX.RemoveNetworkRule(ctx, sandbox, hosts)
	}
	if err := m.SBX.SetNetworkRule(ctx, decision, sandbox, protocol, hosts); err != nil {
		return false, err
	}
	// Rules kept while the sandbox was not there go with this one.
	return true, m.syncNetwork(ctx, sandbox)
}

// RemoveHostRules takes the person's rules for the hosts away, and sbx's
// rules for them for the sandbox, or for every one when sandbox is "".
func (m *Manager) RemoveHostRules(ctx context.Context, sandbox string, hosts []string) error {
	if err := ValidHosts(hosts); err != nil {
		return err
	}
	if sandbox == "" {
		return m.SBX.RemoveNetworkRule(ctx, "", hosts)
	}
	mine := slices.ContainsFunc(m.Config.Sandboxes[sandbox].Network, func(r HostRule) bool { return slices.Contains(hosts, r.Host()) })
	if err := m.updateNetwork(sandbox, func(list []HostRule) []HostRule {
		return slices.DeleteFunc(list, func(r HostRule) bool { return slices.Contains(hosts, r.Host()) })
	}); err != nil {
		return err
	}
	exists, err := m.sbxHas(ctx, sandbox)
	if err != nil || !exists {
		return err
	}
	err = m.SBX.RemoveNetworkRule(ctx, sandbox, hosts)
	if err != nil && mine {
		// A rule of the person's sbx never got, while the sandbox was
		// away, is gone with the configuration.
		return nil
	}
	return err
}

// forgetNetwork drops what the person kept for a sandbox that is removed.
func (m *Manager) forgetNetwork(sandbox string) error {
	if _, ok := m.Config.Sandboxes[sandbox]; !ok {
		return nil
	}
	return m.updateNetwork(sandbox, func([]HostRule) []HostRule { return nil })
}

func (m *Manager) updateNetwork(sandbox string, change func([]HostRule) []HostRule) error {
	c, err := m.Paths.LoadConfig()
	if err != nil {
		return err
	}
	if c.Sandboxes == nil {
		c.Sandboxes = map[string]SandboxConfig{}
	}
	s := c.Sandboxes[sandbox]
	s.Network = change(slices.Clone(s.Network))
	if s.empty() {
		delete(c.Sandboxes, sandbox)
	} else {
		c.Sandboxes[sandbox] = s
	}
	if err = m.Paths.SaveConfig(c); err != nil {
		return err
	}
	m.Config.Sandboxes = c.Sandboxes
	return nil
}

func (m *Manager) sbxHas(ctx context.Context, sandbox string) (bool, error) {
	boxes, err := m.SBX.List(ctx)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(boxes, func(b Box) bool { return b.Name == sandbox }), nil
}

// syncNetwork gives sbx the person's rules for a sandbox it lacks, as
// after a create: sbx's rules for a sandbox go with it.
func (m *Manager) syncNetwork(ctx context.Context, sandbox string) error {
	mine := m.Config.Sandboxes[sandbox].Network
	if len(mine) == 0 {
		return nil
	}
	rules, err := m.SBX.NetworkRules(ctx, sandbox)
	if err != nil {
		return err
	}
	for _, r := range mine {
		if hasRule(rules, sandbox, r) {
			continue
		}
		if err = m.SBX.SetNetworkRule(ctx, r.Decision(), sandbox, r.Proto(), []string{r.Host()}); err != nil {
			return fmt.Errorf("network rule for %s: %w", r.Host(), err)
		}
	}
	return nil
}
