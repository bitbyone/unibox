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

// portCommand names the ports a sandbox publishes. A team names its
// env's in the unibox file beside the sbxenv file, which sbx does not
// read; what a person changes of them, and their own, is kept with the
// env in config.yaml, so the team's file never says which host port
// someone uses.
func (o *options) portCommand() *cobra.Command {
	c := &cobra.Command{Use: "port", Short: "The named ports of an env, and those a running sandbox publishes", Long: `A port is published on the host's loopback. The team names an env's ports
in the unibox file beside its sbxenv file (dev.unibox.yaml beside
dev.sbxenv.yaml); set adds your own or gives one of the team's another
mapping. on and off say whether a sandbox starts with a port published,
and unset goes back to the team's. Each applies at once to a running
sandbox. publish and unpublish switch a running sandbox's ports alone,
and its next start goes back to the env's.

A mapping is [[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL]: 3000 takes any
free host port, 8080:3000 host port 8080.`, Example: "  unibox port on vite\n  unibox port set actuator --map 9080:9080 --off -e dev\n  unibox port publish actuator -s dev\n  unibox port publish --map 9090:9090 -s dev"}
	var envFlagValue, sandbox, mapping string
	var off bool
	edit := func(cmd *cobra.Command, change func(ports map[string]unibox.Port, team map[string]unibox.TeamPort) error) (string, error) {
		name, err := o.envName(envFlagValue)
		if err != nil {
			return "", err
		}
		cfg := o.manager.Config
		ec, ok := cfg.Envs[name]
		if !ok {
			return "", fmt.Errorf("there is no env %s; unibox env ls lists them", name)
		}
		team, err := unibox.ReadTeamPorts(ec.Files)
		if err != nil {
			return name, err
		}
		ports := map[string]unibox.Port{}
		for k, v := range ec.Ports {
			ports[k] = v
		}
		if err = change(ports, team); err != nil {
			return name, err
		}
		// A switch that says what the team says is no change of the person's.
		for k, v := range ports {
			if t, ok := team[k]; ok && v.Spec == "" && v.Default == t.Default {
				delete(ports, k)
			}
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
		env, err := edit(cmd, func(ports map[string]unibox.Port, _ map[string]unibox.TeamPort) error {
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
	unset := &cobra.Command{Use: "unset name...", Args: cobra.MinimumNArgs(1), Short: "Take your ports out of the env, or go back to the team's", RunE: func(cmd *cobra.Command, args []string) error {
		env, err := edit(cmd, func(ports map[string]unibox.Port, team map[string]unibox.TeamPort) error {
			for _, port := range args {
				if _, ok := ports[port]; !ok {
					if _, ok := team[port]; ok {
						return fmt.Errorf("port %s is the team's and you have not changed it; unibox port off %s keeps it from being published", port, port)
					}
					return fmt.Errorf("the env has no port %s; unibox port ls lists them", port)
				}
				delete(ports, port)
			}
			return nil
		})
		if err != nil {
			return err
		}
		return o.success(fmt.Sprintf("took your port %s out of env %s", strings.Join(args, ", "), env))
	}}
	envFlag(unset, &envFlagValue)
	turn := func(on bool) *cobra.Command {
		use, short := "on", "Publish ports at every start of the env's sandbox, and now while it runs"
		if !on {
			use, short = "off", "Stop publishing ports at start, and now while the sandbox runs"
		}
		cmd := &cobra.Command{Use: use + " name...", Args: cobra.MinimumNArgs(1), Short: short, RunE: func(cmd *cobra.Command, args []string) error {
			env, err := edit(cmd, func(ports map[string]unibox.Port, team map[string]unibox.TeamPort) error {
				for _, port := range args {
					p, mine := ports[port]
					if _, theirs := team[port]; !mine && !theirs {
						return fmt.Errorf("the env has no port %s; unibox port ls lists them, unibox port set adds one", port)
					}
					p.Default = on
					ports[port] = p
				}
				return nil
			})
			if err != nil {
				return err
			}
			return o.success(fmt.Sprintf("env %s publishes port %s %s", env, strings.Join(args, ", "), map[bool]string{true: "at every start", false: "only when asked to (unibox port publish)"}[on]))
		}}
		envFlag(cmd, &envFlagValue)
		return cmd
	}
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
	c.AddCommand(set, unset, turn(true), turn(false), ls, publish(true), publish(false))
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
			out = append(out, api.Port{Name: p.Name, Port: p.Spec, Description: p.Description, Team: p.Team, Default: p.Default, Named: p.Named, On: p.On, Published: published})
		}
		return o.write(map[string]any{"api_version": api.Version, "sandbox": name, "running": running, "ports": out})
	}
	if len(ports) == 0 {
		fmt.Fprintf(o.output, "sandbox %s publishes no ports; unibox port set <name> --map <mapping> adds one to its env\n", name)
		return nil
	}
	w := tabwriter.NewWriter(o.output, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "PORT\tMAPPING\tFROM\tAT START\tNOW\tURL\tDESCRIPTION")
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
		from := "you"
		if p.Team {
			from = "team"
		}
		if !p.Named {
			from = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", p.Name, p.Spec, from, at, now, cmp.Or(strings.Join(urls, " "), "-"), p.Description)
	}
	return w.Flush()
}
