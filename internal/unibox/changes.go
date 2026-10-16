package unibox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"

	"github.com/tobola/unibox/internal/unibox/api"
)

// The profile is the user's, under their control and in git, so a change of
// it is not asked about: unibox says what changed and what that leads to -
// a build, a recreation, a rule applied live - and goes on.

// accessChanges says, a line each, how a profile's host access differs from
// what unibox last saw of it.
func accessChanges(old, now HostAccess) []string {
	lines := []string{}
	list := func(name string, a, b []string) {
		for _, v := range b {
			if !slices.Contains(a, v) {
				lines = append(lines, name+" + "+v)
			}
		}
		for _, v := range a {
			if !slices.Contains(b, v) {
				lines = append(lines, name+" - "+v)
			}
		}
	}
	list("env", old.Env, now.Env)
	caches := func(cs []Cache) []string {
		out := []string{}
		for _, c := range cs {
			out = append(out, c.Env+"="+c.Host)
		}
		return out
	}
	list("caches", caches(old.Caches), caches(now.Caches))
	list("network.allow", old.AllowNetwork, now.AllowNetwork)
	list("network.deny", old.DenyNetwork, now.DenyNetwork)
	keyed := func(name string, a, b map[string]string) {
		keys := []string{}
		for k := range a {
			keys = append(keys, k)
		}
		for k := range b {
			if _, ok := a[k]; !ok {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			x, inA := a[k]
			y, inB := b[k]
			switch {
			case !inA:
				lines = append(lines, name+" + "+k)
			case !inB:
				lines = append(lines, name+" - "+k)
			case x != y:
				lines = append(lines, name+" ~ "+k)
			}
		}
	}
	ports := func(ps map[string]Port) map[string]string {
		out := map[string]string{}
		for name, p := range ps {
			out[name] = fmt.Sprintf("%s %v", p.Spec, p.Default)
		}
		return out
	}
	keyed("network.ports", ports(old.Ports), ports(now.Ports))
	digests := func(v any) map[string]string {
		out := map[string]string{}
		b, _ := json.Marshal(v)
		raw := map[string]json.RawMessage{}
		_ = json.Unmarshal(b, &raw)
		for k, r := range raw {
			out[k] = string(r)
		}
		return out
	}
	keyed("auth", digests(old.Auth), digests(now.Auth))
	keyed("services", digests(old.Services), digests(now.Services))
	return lines
}

// settingsSnapshot is what a sandbox is made from, beside its image: the
// profile without what changes in a running sandbox. Instructions are
// written before every session; the network rules are left out only when
// the runtime can change them live - each adapter says what it can do, and
// what it cannot is made by a recreation.
func settingsSnapshot(p Profile, r Runtime) json.RawMessage {
	s := p
	s.Instructions = nil
	// Read by the commit hook in each session, so no recreation for it.
	s.Git.CoAuthors = nil
	// Seeds are merged into the agents' files before each session, and
	// scripts read when they run.
	s.AgentConfig = nil
	s.Scripts = nil
	if _, live := r.(NetworkUpdater); live {
		s.AllowNetwork, s.DenyNetwork = nil, nil
	}
	if _, live := r.(PortPublisher); live {
		s.Ports = nil
	}
	b, _ := json.Marshal(s)
	return b
}

// settingsChanges names the parts of the profile that differ between two
// snapshots.
func settingsChanges(old, now json.RawMessage) []string {
	if len(old) == 0 {
		return nil
	}
	var a, b map[string]json.RawMessage
	_ = json.Unmarshal(old, &a)
	_ = json.Unmarshal(now, &b)
	keys := []string{}
	for k := range b {
		keys = append(keys, k)
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	// The state is written indented, so values are compared compact.
	compact := func(r json.RawMessage) string {
		var buf bytes.Buffer
		if json.Compact(&buf, r) != nil {
			return string(r)
		}
		return buf.String()
	}
	out := []string{}
	for _, k := range keys {
		if compact(a[k]) != compact(b[k]) {
			out = append(out, k)
		}
	}
	return out
}

// noteAccess says what the profile now asks of the host that it did not
// when unibox last saw it, and remembers it. It never stops anything.
func (m *Manager) noteAccess(p Profile, log io.Writer) error {
	s, err := m.Paths.ReadState()
	if err != nil {
		return err
	}
	seen, ok := s.ApprovedFields[p.ApprovalKey()]
	if ok && s.Approvals[p.ApprovalKey()] == p.AccessHash() {
		return nil
	}
	if lines := accessChanges(seen, p.HostAccess); ok && len(lines) > 0 {
		fmt.Fprintf(log, "profile %s changed what it asks of the host:\n  %s\n", p.Name, strings.Join(lines, "\n  "))
	}
	return m.RecordAccess(p.Name)
}

// AccessChanges is what noteAccess would say, for check to show without
// remembering it.
func (m *Manager) AccessChanges(p Profile) []string {
	s, err := m.Paths.ReadState()
	if err != nil {
		return nil
	}
	seen, ok := s.ApprovedFields[p.ApprovalKey()]
	if !ok || s.Approvals[p.ApprovalKey()] == p.AccessHash() {
		return nil
	}
	return accessChanges(seen, p.HostAccess)
}

// networkRules are the rules a profile gives its sandboxes: its own, and
// the hosts of its services, which must be reachable.
func networkRules(p Profile) (allow, deny []string) {
	allow = slices.Clone(p.AllowNetwork)
	for _, name := range sortedKeys(p.Services) {
		for _, h := range p.Services[name].ProxyHosts() {
			if !slices.Contains(allow, h) {
				allow = append(allow, h)
			}
		}
	}
	return allow, slices.Clone(p.DenyNetwork)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func difference(a, b []string) []string {
	out := []string{}
	for _, v := range a {
		if !slices.Contains(b, v) {
			out = append(out, v)
		}
	}
	return out
}

// syncNetwork brings a sandbox's network rules to the profile's, live when
// the runtime can, and records them.
func (m *Manager) syncNetwork(ctx context.Context, b *api.Sandbox, r Runtime, p Profile) error {
	allow, deny := networkRules(p)
	change := NetworkChange{
		AddAllow: difference(allow, b.NetworkAllow), RemoveAllow: difference(b.NetworkAllow, allow),
		AddDeny: difference(deny, b.NetworkDeny), RemoveDeny: difference(b.NetworkDeny, deny),
	}
	if change.Empty() {
		return nil
	}
	u, ok := r.(NetworkUpdater)
	if !ok {
		// The rules are part of the sandbox's settings then, and a change
		// of them recreated it with the rules applied.
		b.NetworkAllow, b.NetworkDeny = allow, deny
		return m.saveBox(*b)
	}
	if err := u.UpdateNetwork(ctx, b.RuntimeName, change, m.Log); err != nil {
		return err
	}
	parts := []string{}
	for _, f := range []struct {
		sign string
		list []string
	}{{"+ allow ", change.AddAllow}, {"- allow ", change.RemoveAllow}, {"+ deny ", change.AddDeny}, {"- deny ", change.RemoveDeny}} {
		for _, h := range f.list {
			parts = append(parts, f.sign+h)
		}
	}
	fmt.Fprintf(m.Log, "network of sandbox %s, applied live: %s\n", b.Name, strings.Join(parts, ", "))
	b.NetworkAllow, b.NetworkDeny = allow, deny
	return m.saveBox(*b)
}

// ApplyNetwork brings every sandbox of a profile to the profile's network
// rules. A sandbox the runtime cannot change live is named, for recreate.
func (m *Manager) ApplyNetwork(ctx context.Context, profile string) ([]string, error) {
	p, err := m.profile(profile)
	if err != nil {
		return nil, err
	}
	s, err := m.Paths.ReadState()
	if err != nil {
		return nil, err
	}
	needRecreate := []string{}
	for _, name := range sortedKeys(s.Sandboxes) {
		b := s.Sandboxes[name]
		if b.Profile != p.Name {
			continue
		}
		r, err := m.Runtime(b.Runtime)
		if err != nil {
			return nil, err
		}
		if _, exists, err := m.findBox(ctx, r, b); err != nil || !exists {
			continue
		}
		if _, ok := r.(NetworkUpdater); !ok {
			needRecreate = append(needRecreate, name)
			continue
		}
		guard, err := m.guard(name)
		if err != nil {
			return nil, err
		}
		err = m.syncNetwork(ctx, &b, r, p)
		guard.Close()
		if err != nil {
			return nil, err
		}
	}
	return needRecreate, nil
}
