package unibox

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

// NetworkRule is a rule of the runtime's network policy: hosts a sandbox
// may or may not reach.
type NetworkRule struct {
	ID       string `json:"id"`
	Decision string `json:"decision"` // allow or deny
	// Sandbox is the one sandbox the rule is for; empty, it is for all.
	Sandbox string `json:"sandbox,omitempty"`
	// From is how the rule came to be: default, kit, added or approval.
	From     string   `json:"from"`
	Hosts    []string `json:"hosts"`
	Editable bool     `json:"editable"`
	Active   bool     `json:"active"`
	// Protocols are tcp, udp or both.
	Protocols []string `json:"protocols,omitempty"`
}

// SandboxConfig is what the person keeps for one sandbox in config.yaml.
type SandboxConfig struct {
	// Network are hosts they let the sandbox reach, or keep it from, over
	// what its env's kits and the runtime's defaults say.
	Network []HostRule `yaml:"network,omitempty" json:"network,omitempty"`
}

// HostRule allows or denies one host, over tcp unless Protocol says udp
// or tcp,udp.
type HostRule struct {
	Allow    string `yaml:"allow,omitempty" json:"allow,omitempty"`
	Deny     string `yaml:"deny,omitempty" json:"deny,omitempty"`
	Protocol string `yaml:"protocol,omitempty" json:"protocol,omitempty"`
}

func (r HostRule) Host() string { return r.Allow + r.Deny }

func (r HostRule) Decision() string {
	if r.Deny != "" {
		return "deny"
	}
	return "allow"
}

func (r HostRule) Proto() string {
	if r.Protocol == "" {
		return "tcp"
	}
	return r.Protocol
}

// Protocols are what a rule may say it is over.
var Protocols = []string{"tcp", "udp", "tcp,udp"}

func (r HostRule) Validate() error {
	if (r.Allow == "") == (r.Deny == "") {
		return fmt.Errorf("a rule says allow: <host> or deny: <host>, one of them")
	}
	if err := ValidHosts([]string{r.Host()}); err != nil {
		return err
	}
	if r.Protocol != "" && !slices.Contains(Protocols, r.Protocol) {
		return fmt.Errorf("protocol is tcp, udp or tcp,udp, not %s", r.Protocol)
	}
	return nil
}

// BlockedHost is a host a sandbox asked for and the policy refused.
type BlockedHost struct {
	Host     string    `json:"host"`
	Reason   string    `json:"reason"`
	Count    int       `json:"count"`
	LastSeen time.Time `json:"last_seen"`
}

// ValidHosts checks hosts before they reach the runtime, where a comma
// would make two and a dash a flag.
func ValidHosts(hosts []string) error {
	if len(hosts) == 0 {
		return fmt.Errorf("name at least one host, such as api.example.com or *.example.com")
	}
	for _, h := range hosts {
		if h == "" || strings.HasPrefix(h, "-") || strings.ContainsAny(h, ", \t\n") {
			return fmt.Errorf("%q is no host; give one per argument, such as api.example.com, *.example.com or example.com:8443", h)
		}
	}
	return nil
}

func (r *SBX) NetworkRules(ctx context.Context, sandbox string) ([]NetworkRule, error) {
	args := []string{"policy", "ls", "--json", "--type", "network"}
	if sandbox != "" {
		args = append(args, sandbox)
	}
	b, err := commandOutput(hostCommand(ctx, "sbx", args...))
	if err != nil {
		return nil, err
	}
	rules, err := sbxList[struct {
		ID           string   `json:"id"`
		Name         string   `json:"name"`
		AppliesTo    string   `json:"applies_to"`
		Decision     string   `json:"decision"`
		ResourceType string   `json:"resource_type"`
		Resources    []string `json:"resources"`
		Status       string   `json:"status"`
		Editable     bool     `json:"editable"`
		Provenance   struct {
			CreatedVia string `json:"created_via"`
		} `json:"provenance"`
		Actions []string `json:"actions"`
	}](b, "rules")
	if err != nil {
		return nil, fmt.Errorf("cannot read sbx's network policy; update sbx: %w", err)
	}
	out := []NetworkRule{}
	for _, v := range rules {
		if v.ResourceType != "network" {
			continue
		}
		from := v.Provenance.CreatedVia
		if strings.HasPrefix(v.Name, "kit:") {
			from = "kit"
		}
		protocols := []string{}
		for _, a := range v.Actions {
			if p, ok := strings.CutPrefix(a, "net:connect:"); ok {
				protocols = append(protocols, p)
			}
		}
		out = append(out, NetworkRule{
			ID: v.ID, Decision: v.Decision, Sandbox: strings.TrimPrefix(strings.TrimPrefix(v.AppliesTo, "all"), "sandbox:"),
			From: from, Hosts: v.Resources, Editable: v.Editable, Active: v.Status == "" || v.Status == "active", Protocols: protocols,
		})
	}
	return out, nil
}

func (r *SBX) SetNetworkRule(ctx context.Context, decision, sandbox, protocol string, hosts []string) error {
	if decision != "allow" && decision != "deny" {
		return fmt.Errorf("a rule allows or denies, not %s", decision)
	}
	if err := ValidHosts(hosts); err != nil {
		return err
	}
	args := []string{"policy", decision, "network"}
	if sandbox != "" {
		args = append(args, "--sandbox", sandbox)
	}
	if protocol != "" && protocol != "tcp" {
		args = append(args, "--protocol", protocol)
	}
	return runQuiet(hostCommand(ctx, "sbx", append(args, strings.Join(hosts, ","))...))
}

func (r *SBX) RemoveNetworkRule(ctx context.Context, sandbox string, hosts []string) error {
	if err := ValidHosts(hosts); err != nil {
		return err
	}
	args := []string{"policy", "rm", "network", "--force", "--resource", strings.Join(hosts, ",")}
	if sandbox != "" {
		args = append(args, "--sandbox", sandbox)
	}
	return runQuiet(hostCommand(ctx, "sbx", args...))
}

// NetworkBlocked are the hosts the policy refused a sandbox lately, the
// latest first. Names a sandbox's DNS search domain made are left out:
// nobody asked for them.
func (r *SBX) NetworkBlocked(ctx context.Context, sandbox string) ([]BlockedHost, error) {
	b, err := commandOutput(hostCommand(ctx, "sbx", "policy", "log", "--json", "--type", "network", "--limit", "500", sandbox))
	if err != nil {
		return nil, err
	}
	var log struct {
		Blocked []struct {
			Host     string    `json:"host"`
			Reason   string    `json:"reason"`
			Count    int       `json:"count"`
			LastSeen time.Time `json:"last_seen"`
		} `json:"blocked"`
	}
	if err = json.Unmarshal(b, &log); err != nil {
		return nil, fmt.Errorf("cannot read sbx's policy log; update sbx: %w", err)
	}
	out := []BlockedHost{}
	for _, v := range log.Blocked {
		if v.Host == "" || strings.HasSuffix(v.Host, ".docker.internal") {
			continue
		}
		out = append(out, BlockedHost{Host: v.Host, Reason: v.Reason, Count: v.Count, LastSeen: v.LastSeen})
	}
	slices.SortFunc(out, func(a, b BlockedHost) int { return b.LastSeen.Compare(a.LastSeen) })
	return out, nil
}
