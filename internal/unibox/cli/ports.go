package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
)

// portCommand names the ports a person publishes from an env's sandbox.
// They are the person's, kept with the env in config.yaml: the team's
// files say what the sandbox is, not which host port someone uses.
func (o *options) portCommand() *cobra.Command {
	c := &cobra.Command{Use: "port", Short: "Your named ports of an env, and those a running sandbox publishes", Long: `A port is published on the host's loopback. set and unset change the
env's ports, for its sandbox, and apply at once while it runs; publish and
unpublish switch a running sandbox's ports alone, and its next start goes
back to the env's.

A mapping is [[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL]: 3000 takes any
free host port, 8080:3000 host port 8080.`, Example: "  unibox port set actuator --map 9080:9080 --off -e dev\n  unibox port publish actuator -s dev\n  unibox port publish --map 9090:9090 -s dev"}
	var envFlagValue, sandbox, mapping string
	var off bool
	edit := func(cmd *cobra.Command, change func(ports map[string]unibox.Port) error) (string, error) {
		name, err := o.envName(envFlagValue)
		if err != nil {
			return "", err
		}
		cfg := o.manager.Config
		ec, ok := cfg.Envs[name]
		if !ok {
			return "", fmt.Errorf("there is no env %s; unibox env ls lists them", name)
		}
		ports := map[string]unibox.Port{}
		for k, v := range ec.Ports {
			ports[k] = v
		}
		if err = change(ports); err != nil {
			return name, err
		}
		ec.Ports = ports
		if len(ports) == 0 {
			ec.Ports = nil
		}
		if err = unibox.ValidEnvConfig(name, ec); err != nil {
			return name, err
		}
		cfg.Envs[name] = ec
		if err = o.paths.SaveConfig(cfg); err != nil {
			return name, err
		}
		o.manager.Config = cfg
		return name, o.manager.ApplyPorts(cmd.Context(), name)
	}
	set := &cobra.Command{Use: "set name --map mapping", Args: cobra.ExactArgs(1), Short: "Publish a sandbox port by name, at every start unless --off", RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := unibox.ParsePortSpec(mapping); err != nil {
			return err
		}
		env, err := edit(cmd, func(ports map[string]unibox.Port) error {
			ports[args[0]] = unibox.Port{Spec: mapping, Default: !off}
			return nil
		})
		if err != nil {
			return err
		}
		when := "at every start"
		if off {
			when = "when published (unibox port publish " + args[0] + ")"
		}
		return o.success(fmt.Sprintf("env %s publishes port %s (%s) %s", env, args[0], mapping, when))
	}}
	set.Flags().StringVar(&mapping, "map", "", "The mapping, [[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL]")
	set.Flags().BoolVar(&off, "off", false, "Publish it only when asked to, not at start")
	_ = set.MarkFlagRequired("map")
	envFlag(set, &envFlagValue)
	unset := &cobra.Command{Use: "unset name...", Args: cobra.MinimumNArgs(1), Short: "Take ports out of the env", RunE: func(cmd *cobra.Command, args []string) error {
		env, err := edit(cmd, func(ports map[string]unibox.Port) error {
			for _, port := range args {
				if _, ok := ports[port]; !ok {
					return fmt.Errorf("the env has no port %s; unibox port ls lists them", port)
				}
				delete(ports, port)
			}
			return nil
		})
		if err != nil {
			return err
		}
		return o.success(fmt.Sprintf("took port %s out of env %s", strings.Join(args, ", "), env))
	}}
	envFlag(unset, &envFlagValue)
	var maps []string
	publish := func(on bool) *cobra.Command {
		use, short := "publish", "Publish ports of a running sandbox: the env's by name, others by --map"
		if !on {
			use, short = "unpublish", "Stop publishing ports of a running sandbox, by name or --map"
		}
		cmd := &cobra.Command{Use: use + " [name...]", Args: cobra.ArbitraryArgs, Short: short, RunE: func(cmd *cobra.Command, args []string) error {
			if len(args)+len(maps) == 0 {
				return fmt.Errorf("name a port of the env, or give a mapping with --map")
			}
			name, err := o.sandboxName(sandbox)
			if err != nil {
				return err
			}
			switches := map[string]bool{}
			for _, p := range append(args, maps...) {
				switches[p] = on
			}
			if err := o.manager.SwitchPorts(cmd.Context(), name, switches); err != nil {
				return err
			}
			return o.listPorts(cmd, name)
		}}
		cmd.Flags().StringArrayVar(&maps, "map", nil, "A mapping the env does not name, [[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL] (repeatable)")
		sandboxFlag(cmd, &sandbox)
		return cmd
	}
	ls := &cobra.Command{Use: "ls", Args: cobra.NoArgs, Short: "List a sandbox's ports and their URLs", RunE: func(cmd *cobra.Command, args []string) error {
		name, err := o.sandboxName(sandbox)
		if err != nil {
			return err
		}
		return o.listPorts(cmd, name)
	}}
	sandboxFlag(ls, &sandbox)
	c.AddCommand(set, unset, ls, publish(true), publish(false))
	return c
}

// listPorts shows a sandbox's ports, what each does at start and now.
func (o *options) listPorts(cmd *cobra.Command, name string) error {
	ports, running, err := o.manager.SandboxPorts(cmd.Context(), name)
	if err != nil {
		return err
	}
	if o.json {
		out := []api.Port{}
		for _, p := range ports {
			published := p.Published
			if published == nil {
				published = []api.PublishedPort{}
			}
			out = append(out, api.Port{Name: p.Name, Port: p.Spec, Default: p.Default, Named: p.Named, On: p.On, Published: published})
		}
		return o.write(map[string]any{"api_version": api.Version, "sandbox": name, "running": running, "ports": out})
	}
	if len(ports) == 0 {
		fmt.Fprintf(o.output, "sandbox %s publishes no ports; unibox port set <name> --map <mapping> adds one to its env\n", name)
		return nil
	}
	w := tabwriter.NewWriter(o.output, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "PORT\tMAPPING\tAT START\tNOW\tURL")
	for _, p := range ports {
		at := "off"
		if p.Default {
			at = "on"
		}
		if !p.Named {
			at = "-"
		}
		now := "off"
		if p.On {
			now = "on"
		}
		if !running {
			now = "stopped"
		}
		urls := []string{}
		for _, q := range p.Published {
			if strings.HasPrefix(q.Protocol, "tcp") {
				host := q.HostIP
				if strings.Contains(host, ":") {
					host = "[" + host + "]"
				}
				urls = append(urls, fmt.Sprintf("http://%s:%d", host, q.HostPort))
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", p.Name, p.Spec, at, now, strings.Join(urls, " "))
	}
	return w.Flush()
}
