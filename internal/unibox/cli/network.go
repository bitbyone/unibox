package cli

import (
	"cmp"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
)

// networkCommand is a sandbox's network beyond what its env and sbx's
// defaults say: hosts the person adds or keeps out, kept by unibox, beside
// what the policy refused lately, which shows what the env leaves out.
func (o *options) networkCommand() *cobra.Command {
	c := &cobra.Command{Use: "network", Short: "The hosts a sandbox may reach beyond its env's", Long: `A sandbox reaches the hosts sbx's defaults and its env's kits allow, and
those you add for it here. Yours are kept in config.yaml under the
sandbox's name and applied to it, again after each create, and go when it
is removed. unibox network ls lists them, rules added with sbx itself, and
the hosts the sandbox was refused lately - what its env's files may want
to say. A host is a name, *.name for its subdomains, **.name for any
depth, or name:port.`, Example: "  unibox network allow registry.npmjs.org *.npmjs.org -s dev\n  unibox network allow stun.example.com:3478 --protocol udp\n  unibox network deny ads.example.com\n  unibox network ls --all"}
	var sandbox, protocol string
	var all bool
	rule := func(decision string) *cobra.Command {
		short := "Let a sandbox reach hosts"
		if decision == "deny" {
			short = "Keep a sandbox from hosts, even those its env allows"
		}
		cmd := &cobra.Command{Use: decision + " host...", Args: cobra.MinimumNArgs(1), Short: short, RunE: func(cmd *cobra.Command, args []string) error {
			if err := unibox.ValidHosts(args); err != nil {
				return err
			}
			name, err := o.sandboxName(sandbox)
			if err != nil {
				return err
			}
			applied, err := o.manager.SetHostRules(cmd.Context(), name, decision, protocol, args)
			if err != nil {
				return err
			}
			verb := map[string]string{"allow": "may reach", "deny": "may not reach"}[decision]
			said := fmt.Sprintf("sandbox %s %s %s", name, verb, strings.Join(args, ", "))
			if protocol != "" && protocol != "tcp" {
				said += " over " + protocol
			}
			if !applied {
				said += "; it applies once the sandbox is created"
			}
			return o.success(said)
		}}
		sandboxFlag(cmd, &sandbox)
		cmd.Flags().StringVar(&protocol, "protocol", "tcp", "tcp, udp or tcp,udp")
		return cmd
	}
	rm := &cobra.Command{Use: "rm host...", Args: cobra.MinimumNArgs(1), Short: "Take away a rule for hosts", Long: "Take away your rule for the hosts, and one added with sbx itself for the\nsandbox. What the env's kits and sbx's defaults say stays.", RunE: func(cmd *cobra.Command, args []string) error {
		name, err := o.sandboxName(sandbox)
		if err != nil {
			return err
		}
		if err := o.manager.RemoveHostRules(cmd.Context(), name, args); err != nil {
			return err
		}
		return o.success(fmt.Sprintf("took the rule for %s away from sandbox %s", strings.Join(args, ", "), name))
	}}
	sandboxFlag(rm, &sandbox)
	ls := &cobra.Command{Use: "ls", Args: cobra.NoArgs, Short: "List a sandbox's rules and the hosts it was refused", RunE: func(cmd *cobra.Command, args []string) error {
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
		if o.json {
			blocked := []api.BlockedHost{}
			for _, b := range v.Blocked {
				blocked = append(blocked, api.BlockedHost(b))
			}
			return o.write(map[string]any{"api_version": api.Version, "sandbox": name, "rules": networkRules(v, true), "blocked": blocked})
		}
		return printNetwork(o.output, name, v, all)
	}}
	sandboxFlag(ls, &sandbox)
	ls.Flags().BoolVar(&all, "all", false, "List the hosts of the env's kits and sbx's defaults too")
	c.AddCommand(ls, rule("allow"), rule("deny"), rm)
	c.Args = cobra.NoArgs
	c.RunE = func(cmd *cobra.Command, args []string) error {
		ls.SetContext(cmd.Context())
		if !o.interactive || o.json {
			return ls.RunE(ls, nil)
		}
		name, err := o.sandboxName(sandbox)
		if err != nil {
			return err
		}
		return o.networkUI(cmd, name)
	}
	sandboxFlag(c, &sandbox)
	return c
}

// networkRules are a view's rules one host to a line: the person's, those
// added outside unibox, and with all the env's and sbx's defaults.
func networkRules(v unibox.NetworkView, all bool) []api.NetworkRule {
	out := []api.NetworkRule{}
	for _, r := range v.Yours {
		out = append(out, api.NetworkRule{Decision: r.Decision(), Host: r.Host(), Protocol: r.Proto(), From: "yours", Sandbox: "", Applied: r.Applied})
	}
	for _, r := range v.Others {
		for _, h := range r.Hosts {
			out = append(out, api.NetworkRule{Decision: r.Decision, Host: h, Protocol: protocols(r), From: "sbx", Sandbox: r.Sandbox, Applied: true})
		}
	}
	if !all {
		return out
	}
	for _, h := range v.Env {
		out = append(out, api.NetworkRule{Decision: "allow", Host: h, Protocol: "tcp", From: "env", Applied: true})
	}
	for _, r := range v.Defaults {
		for _, h := range r.Hosts {
			out = append(out, api.NetworkRule{Decision: r.Decision, Host: h, Protocol: protocols(r), From: "default", Applied: true})
		}
	}
	return out
}

func protocols(r unibox.NetworkRule) string {
	return cmp.Or(strings.Join(r.Protocols, ","), "tcp")
}

// fromText says where a rule comes from, for a person.
func fromText(r api.NetworkRule) string {
	switch {
	case r.From == "yours" && !r.Applied:
		return "yours, once created"
	case r.From == "sbx" && r.Sandbox == "":
		return "sbx, every sandbox"
	case r.From == "env":
		return "env's kits"
	case r.From == "default":
		return "sbx's defaults"
	}
	return r.From
}

// beyond says what a sandbox reaches besides the rules listed.
func beyond(v unibox.NetworkView) string {
	return fmt.Sprintf("the env's kits allow %d hosts, sbx's defaults %d", len(v.Env), v.DefaultHosts())
}

func printNetwork(out io.Writer, sandbox string, v unibox.NetworkView, all bool) error {
	rules := networkRules(v, all)
	if len(rules) == 0 {
		fmt.Fprintf(out, "sandbox %s has no rules of its own; unibox network allow <host> adds one\n", sandbox)
	} else {
		w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "RULE\tHOST\tPROTOCOL\tFROM")
		for _, r := range rules {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Decision, r.Host, r.Protocol, fromText(r))
		}
		if err := w.Flush(); err != nil {
			return err
		}
	}
	if !all {
		fmt.Fprintf(out, "%s; --all lists them\n", beyond(v))
	}
	if len(v.Blocked) == 0 {
		fmt.Fprintln(out, "\nnothing refused lately")
		return nil
	}
	fmt.Fprintln(out, "\nrefused lately - what the env may want to allow:")
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "HOST\tTIMES\tLAST")
	for _, b := range v.Blocked {
		fmt.Fprintf(w, "%s\t%d\t%s\n", b.Host, b.Count, ago(b.LastSeen))
	}
	return w.Flush()
}

// ago says how long ago a moment was, roughly.
func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case t.IsZero():
		return "-"
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}
