package unibox

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
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
		out = append(out, NetworkRule{
			ID: v.ID, Decision: v.Decision, Sandbox: strings.TrimPrefix(strings.TrimPrefix(v.AppliesTo, "all"), "sandbox:"),
			From: from, Hosts: v.Resources, Editable: v.Editable, Active: v.Status == "" || v.Status == "active",
		})
	}
	return out, nil
}

func (r *SBX) SetNetworkRule(ctx context.Context, decision, sandbox string, hosts []string) error {
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

func (r *SBX) NetworkLog(ctx context.Context, sandbox string, limit int, out io.Writer) error {
	args := []string{"policy", "log", "--type", "network", "--limit", strconv.Itoa(limit)}
	if sandbox != "" {
		args = append(args, sandbox)
	}
	var errout strings.Builder
	c := hostCommand(ctx, "sbx", args...)
	c.Stdout, c.Stderr = out, &errout
	if err := c.Run(); err != nil {
		return fmt.Errorf("sbx policy log failed: %s", strings.TrimSpace(errout.String()))
	}
	return nil
}
