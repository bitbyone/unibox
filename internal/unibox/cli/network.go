package cli

import (
	"cmp"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
)

// networkCommand is the runtime's network policy, so a host a sandbox
// needs is let in without its CLI. A rule is for one sandbox unless
// --global says every one: letting a host into all of them is the larger
// step, and is to be asked for.
func (o *options) networkCommand() *cobra.Command {
	c := &cobra.Command{Use: "network", Short: "The hosts sandboxes may reach", Long: `A sandbox reaches only the hosts the network policy allows: those of the
runtime's defaults, of the env's kits, and the rules added here. A rule is
for the sandbox (-s, or the context's) unless --global makes it every
sandbox's. A host is a name, *.name for its subdomains or name:port.`, Example: "  unibox network allow registry.npmjs.org *.npmjs.org -s dev\n  unibox network deny ads.example.com --global\n  unibox network log -s dev"}
	var sandbox string
	var global bool
	scope := func() (string, error) {
		if global {
			if sandbox != "" {
				return "", fmt.Errorf("a rule is for one sandbox or for every one; leave out -s or --global")
			}
			return "", nil
		}
		return o.sandboxName(sandbox)
	}
	scoped := func(cmd *cobra.Command) {
		sandboxFlag(cmd, &sandbox)
		cmd.Flags().BoolVar(&global, "global", false, "For every sandbox, not one")
	}
	rule := func(decision string) *cobra.Command {
		short := "Let a sandbox reach hosts"
		if decision == "deny" {
			short = "Keep a sandbox from hosts, even those another rule allows"
		}
		cmd := &cobra.Command{Use: decision + " host...", Args: cobra.MinimumNArgs(1), Short: short, RunE: func(cmd *cobra.Command, args []string) error {
			if err := unibox.ValidHosts(args); err != nil {
				return err
			}
			name, err := scope()
			if err != nil {
				return err
			}
			if err := o.manager.SBX.SetNetworkRule(cmd.Context(), decision, name, args); err != nil {
				return err
			}
			verb := map[string]string{"allow": "may reach", "deny": "may not reach"}[decision]
			return o.success(fmt.Sprintf("%s %s %s", who(name), verb, strings.Join(args, ", ")))
		}}
		scoped(cmd)
		return cmd
	}
	rm := &cobra.Command{Use: "rm host...", Args: cobra.MinimumNArgs(1), Short: "Take away the rule for hosts", Long: "Take away the rule added for the hosts, for the sandbox or with --global\nfor every one. The defaults' and the kits' rules are not to be removed.", RunE: func(cmd *cobra.Command, args []string) error {
		if err := unibox.ValidHosts(args); err != nil {
			return err
		}
		name, err := scope()
		if err != nil {
			return err
		}
		if err := o.manager.SBX.RemoveNetworkRule(cmd.Context(), name, args); err != nil {
			return err
		}
		return o.success(fmt.Sprintf("took the rule for %s away from %s", strings.Join(args, ", "), who(name)))
	}}
	scoped(rm)
	ls := &cobra.Command{Use: "ls", Args: cobra.NoArgs, Short: "List the rules a sandbox has, or every rule", RunE: func(cmd *cobra.Command, args []string) error {
		name := cmp.Or(sandbox, o.defaultSandbox())
		if global {
			name = ""
		}
		rules, err := o.manager.SBX.NetworkRules(cmd.Context(), name)
		if err != nil {
			return err
		}
		if o.json {
			out := []api.NetworkRule{}
			for _, r := range rules {
				hosts := r.Hosts
				if hosts == nil {
					hosts = []string{}
				}
				out = append(out, api.NetworkRule{ID: r.ID, Decision: r.Decision, Sandbox: r.Sandbox, From: r.From, Hosts: hosts, Editable: r.Editable, Active: r.Active})
			}
			return o.write(map[string]any{"api_version": api.Version, "sandbox": name, "rules": out})
		}
		w := tabwriter.NewWriter(o.output, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "DECISION\tFOR\tFROM\tHOSTS")
		for _, r := range rules {
			decision := r.Decision
			if !r.Active {
				decision += " (inactive)"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", decision, cmp.Or(r.Sandbox, "all"), r.From, strings.Join(r.Hosts, ", "))
		}
		return w.Flush()
	}}
	sandboxFlag(ls, &sandbox)
	ls.Flags().BoolVar(&global, "global", false, "Every rule, not only a sandbox's")
	var limit int
	log := &cobra.Command{Use: "log", Args: cobra.NoArgs, Short: "Show the latest requests and what the policy decided", RunE: func(cmd *cobra.Command, args []string) error {
		if o.json {
			return fmt.Errorf("network log has no --json; it shows the runtime's own table")
		}
		return o.manager.SBX.NetworkLog(cmd.Context(), cmp.Or(sandbox, o.defaultSandbox()), limit, o.output)
	}}
	sandboxFlag(log, &sandbox)
	log.Flags().IntVar(&limit, "limit", 30, "How many requests")
	c.AddCommand(ls, rule("allow"), rule("deny"), rm, log)
	return c
}

func who(sandbox string) string {
	if sandbox == "" {
		return "every sandbox"
	}
	return "sandbox " + sandbox
}
