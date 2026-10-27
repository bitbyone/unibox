package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
)

// kitPolicy is what a sandbox reaches beyond its env, as a kit's network
// policy says it: the person's rules and those added with sbx, less the
// hosts the env's kits allow already, and the hosts refused lately, which
// are the team's to decide on.
type kitPolicy struct {
	Allow   []api.NetworkRule `json:"allow"`
	Deny    []api.NetworkRule `json:"deny"`
	Refused []api.BlockedHost `json:"refused"`
}

// hostOnly is a host without its port, which a kit's allow list ignores.
func hostOnly(h string) string {
	if i := strings.LastIndexByte(h, ':'); i > 0 {
		if _, err := strconv.Atoi(h[i+1:]); err == nil {
			return h[:i]
		}
	}
	return h
}

func exportPolicy(v unibox.NetworkView) kitPolicy {
	env := map[string]bool{}
	for _, h := range v.Env {
		env[hostOnly(h)] = true
	}
	p := kitPolicy{Allow: []api.NetworkRule{}, Deny: []api.NetworkRule{}, Refused: []api.BlockedHost{}}
	seen := map[string]bool{}
	for _, r := range networkRules(v, false) {
		if seen[r.Decision+" "+r.Host] || r.Decision == "allow" && env[hostOnly(r.Host)] {
			continue
		}
		seen[r.Decision+" "+r.Host] = true
		if r.Decision == "deny" {
			p.Deny = append(p.Deny, r)
		} else {
			p.Allow = append(p.Allow, r)
		}
	}
	for _, b := range v.Blocked {
		if !env[hostOnly(b.Host)] && !seen["allow "+b.Host] && !seen["deny "+b.Host] {
			p.Refused = append(p.Refused, api.BlockedHost(b))
		}
	}
	return p
}

// yamlScalar is a host as YAML reads it back: *.name would be an alias.
func yamlScalar(s string) string {
	b, err := yaml.Marshal(s)
	if err != nil {
		return strconv.Quote(s)
	}
	return strings.TrimSpace(string(b))
}

// writeKitPolicy writes a kit's network-policy entry, the refused hosts
// as lines to uncomment.
func writeKitPolicy(w io.Writer, sandbox string, p kitPolicy) {
	fmt.Fprintf(w, `# What sandbox %s reaches beyond its env's kits, as a kit's network policy.
# A kit has one network-policy entry: merge these into it. Then unibox
# sandbox recreate %s applies the kit, and unibox network rm takes
# your own rules away.
- type: com.docker.sandbox/network-policy@2
  config:
    runtime:
`, sandbox, sandbox)
	entry := func(r api.NetworkRule) {
		line := "        - " + yamlScalar(r.Host)
		var notes []string
		if r.Protocol != "tcp" {
			notes = append(notes, r.Protocol+"; a kit's host is any protocol")
		}
		if r.From == "sbx" {
			notes = append(notes, "added with sbx")
		}
		if len(notes) > 0 {
			line += "  # " + strings.Join(notes, ", ")
		}
		fmt.Fprintln(w, line)
	}
	if len(p.Allow) > 0 || len(p.Refused) > 0 {
		comment := ""
		if len(p.Allow) == 0 {
			comment = "# "
		}
		fmt.Fprintf(w, "      %sallow:\n", comment)
		for _, r := range p.Allow {
			entry(r)
		}
		for _, b := range p.Refused {
			fmt.Fprintf(w, "        # - %s  # refused %d times, last %s\n", yamlScalar(b.Host), b.Count, ago(b.LastSeen))
		}
	}
	if len(p.Deny) > 0 {
		fmt.Fprintln(w, "      deny:")
		for _, r := range p.Deny {
			entry(r)
		}
	}
}

func (o *options) networkExportCommand() *cobra.Command {
	var sandbox string
	c := &cobra.Command{Use: "export", Args: cobra.NoArgs, Short: "Write what a sandbox reaches beyond its env as a kit's network policy", Long: `Writes the rules a sandbox has beyond its env's kits - yours and those
added with sbx, less the hosts a kit allows already - as the
network-policy entry of a kit, to merge into a kit of the env, so the team
gets them. The hosts the sandbox was refused lately follow as lines to
uncomment. A kit's host is any protocol, so a udp rule is said beside it.`, Example: "  unibox network export -s dev\n  unibox network export --json | jq -r '.allow[].host'", RunE: func(cmd *cobra.Command, args []string) error {
		name, err := o.sandboxName(sandbox)
		if err != nil {
			return err
		}
		var v unibox.NetworkView
		err = o.waiting("reading the network policy", func() (err error) {
			v, err = o.manager.Network(cmd.Context(), name)
			return err
		})
		if err != nil {
			return err
		}
		p := exportPolicy(v)
		if o.json {
			return o.write(map[string]any{"api_version": api.Version, "sandbox": name, "allow": p.Allow, "deny": p.Deny, "refused": p.Refused})
		}
		if len(p.Allow)+len(p.Deny)+len(p.Refused) == 0 {
			return o.success(fmt.Sprintf("sandbox %s reaches nothing beyond its env's kits, and was refused nothing lately", name))
		}
		writeKitPolicy(o.output, name, p)
		return nil
	}}
	sandboxFlag(c, &sandbox)
	return c
}
