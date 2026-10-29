package cli

import (
	"bytes"
	"cmp"
	"fmt"
	"strings"
	"text/tabwriter"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
)

// portCommand switches a running sandbox's ports. The ports themselves
// are written by hand: the team's in unibox.yaml at its repository's root
// and in the unibox file beside an env's sbxenv file, which sbx does not
// read, and a person's in config.yaml - so nobody's host port is in the
// team's files, and no command stands between a person and the file but
// port default, which says only whether one is on at start.
func (o *options) portCommand() *cobra.Command {
	c := &cobra.Command{Use: "port", Short: "Switch a running sandbox's ports, and list them", Long: `A port is published on the host's loopback. The ports are named in files,
by hand: the team's in unibox.yaml at the root of its repository and in
the unibox file beside an env's sbxenv file (dev.unibox.yaml beside
dev.sbxenv.yaml), yours in ~/.config/unibox/config.yaml, for every env,
for every env of one env repository or for one, each over the one
before. What a port says as default is whether a sandbox starts with it
published; port default name on|off writes yours for the env, or with
--env-repo for every env of its repository, and - takes it back.

on and off switch a running sandbox's ports for as long as it runs: by
name, by name=HOST_PORT for a named port on another host port, or by
--map for a mapping no file names. Its next start goes back to the
files.

Alone at a terminal, unibox port shows a running sandbox's ports to
switch with the arrows or h and l, and m publishes a named one on a host
port typed there, as name=HOST_PORT does.

A mapping is [[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL]: 3000 takes any
free host port, 8080:3000 host port 8080.`, Example: "  unibox port -s dev\n  unibox port on vite\n  unibox port on vite=15173 -s dev\n  unibox port on --map 9090:9090\n  unibox port off vite"}
	var sandbox string
	var maps []string
	turn := func(on bool) *cobra.Command {
		use, short := "on", "Publish ports of a running sandbox: by name, name=HOST_PORT, or --map"
		if !on {
			use, short = "off", "Stop publishing ports of a running sandbox, by name or --map"
		}
		cmd := &cobra.Command{Use: use + " [name[=host_port]...]", Args: cobra.ArbitraryArgs, Short: short, RunE: func(cmd *cobra.Command, args []string) error {
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
		cmd.Flags().StringArrayVar(&maps, "map", nil, "A mapping no file names, [[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL] (repeatable)")
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
	var envFlagValue string
	var envRepo bool
	def := &cobra.Command{Use: "default name on|off|-", Args: cobra.ExactArgs(2), Short: "Say whether a port of an env is on at each start, for you", Long: `Writes into your config.yaml, under the env, whether a named port is
published at each start of its sandboxes, over what the team's files say;
- takes your say back. With --env-repo it is written for every env of the
env's repository instead, under each env's own. A running sandbox keeps
its ports until it next starts, or unibox port on|off switches them now.`, Example: "  unibox port default spring on\n  unibox port default vite off -e dev\n  unibox port default vite off --env-repo\n  unibox port default spring -", RunE: func(cmd *cobra.Command, args []string) error {
		env, err := o.envName(envFlagValue)
		if err != nil {
			return err
		}
		var on *bool
		switch args[1] {
		case "on", "off":
			v := args[1] == "on"
			on = &v
		case "-":
		default:
			return fmt.Errorf("say on, off, or - to take your say back, not %s", args[1])
		}
		if err := o.manager.SetPortDefault(unibox.Scope{Env: env, EnvRepo: envRepo}, args[0], on); err != nil {
			return err
		}
		return o.success(portDefaultSaid(o.manager, env, args[0]))
	}}
	def.Flags().StringVarP(&envFlagValue, "env", "e", "", "Env (default: UNIBOX_ENV, the context's, or the only one)")
	def.Flags().BoolVar(&envRepo, "env-repo", false, "Say it for every env of the env's repository")
	c.AddCommand(turn(true), turn(false), ls, def)
	c.Args = cobra.NoArgs
	c.RunE = func(cmd *cobra.Command, args []string) error {
		name, err := o.sandboxName(sandbox)
		if err != nil {
			return err
		}
		return o.portsUI(cmd, name)
	}
	sandboxFlag(c, &sandbox)
	return c
}

// portsUI lets a person switch a running sandbox's ports at a terminal;
// anywhere else, or with the sandbox stopped, it lists them as ls does.
func (o *options) portsUI(cmd *cobra.Command, name string) error {
	ctx := cmd.Context()
	var ports []unibox.PortState
	var running bool
	err := o.waiting("reading the ports", func() (err error) {
		ports, running, err = o.manager.SandboxPorts(ctx, name)
		return err
	})
	if err != nil {
		return err
	}
	if o.json || !o.interactive || !running || len(ports) == 0 {
		if !o.json && o.interactive && !running {
			fmt.Fprintf(o.log, "sandbox %s is not running; its ports can be switched once it runs (unibox resume, unibox sandbox shell)\n", name)
		}
		return o.listPorts(cmd, name)
	}
	// What unibox and sbx would say while a switch is made would land
	// among the list's lines, which the list then draws over; it is kept
	// and its last line shown in the list instead.
	said := &bytes.Buffer{}
	log := o.manager.Log
	o.manager.Log = said
	defer func() { o.manager.Log = log }()
	m := portsModel{sandbox: name, rows: ports, switchPort: func(key string, on bool) ([]unibox.PortState, string, error) {
		said.Reset()
		err := o.manager.SwitchPorts(ctx, name, map[string]bool{key: on})
		lines := strings.Split(strings.TrimSpace(said.String()), "\n")
		note := strings.TrimSpace(lines[len(lines)-1])
		if err != nil {
			return nil, note, err
		}
		ports, _, err := o.manager.SandboxPorts(ctx, name)
		return ports, note, err
	}, setDefault: func(key string, on bool) ([]unibox.PortState, string, error) {
		b, _, err := o.manager.Definition(unibox.Request{Sandbox: name})
		if err != nil {
			return nil, "", err
		}
		if err := o.manager.SetPortDefault(unibox.Scope{Env: b.Env}, key, &on); err != nil {
			return nil, "", err
		}
		ports, _, err := o.manager.SandboxPorts(ctx, name)
		return ports, portDefaultSaid(o.manager, b.Env, key), err
	}}
	// The theme asks the terminal for its background; asked here, before
	// the program reads keys, the answer is not taken for typing.
	m.context = o.contextCorner()
	lipgloss.HasDarkBackground()
	if _, err := tea.NewProgram(m, tea.WithInput(o.input), tea.WithOutput(o.log), tea.WithContext(ctx)).Run(); err != nil {
		return err
	}
	return o.listPorts(cmd, name)
}

// portDefaultSaid says what a port of an env now does at start, and whose
// say that is.
func portDefaultSaid(m *unibox.Manager, env, port string) string {
	e, err := m.Config.LoadEnv(env)
	if err != nil {
		return "port " + port + " of env " + env + " is set"
	}
	p := e.Ports[port]
	state := map[bool]string{true: "on", false: "off"}[p.Default]
	whose := "as the env's files say"
	if unibox.IsYours(p.DefaultFrom) {
		whose = "by your config.yaml"
	}
	return fmt.Sprintf("port %s is %s at each start of env %s's sandboxes, %s", port, state, env, whose)
}

// listPorts shows a sandbox's ports, what each does at start and now.
func (o *options) listPorts(cmd *cobra.Command, name string) error {
	var ports []unibox.PortState
	var running bool
	err := o.waiting("reading the ports", func() (err error) {
		ports, running, err = o.manager.SandboxPorts(cmd.Context(), name)
		return err
	})
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
			out = append(out, api.Port{Name: p.Name, Port: p.Spec, Description: p.Description, Team: p.Team, From: p.From, Default: p.Default, DefaultYours: p.DefaultYours, Named: p.Named, On: p.On, Published: published})
		}
		return o.write(map[string]any{"api_version": api.Version, "sandbox": name, "running": running, "ports": out})
	}
	if len(ports) == 0 {
		fmt.Fprintf(o.output, "sandbox %s publishes no ports; name them under ports: in your config.yaml, or publish one now with unibox port on --map <mapping>\n", name)
		return nil
	}
	w := tabwriter.NewWriter(o.output, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "PORT\tMAPPING\tFROM\tAT START\tNOW\tURL\tDESCRIPTION")
	for _, p := range ports {
		at := "off"
		if p.Default {
			at = "on"
		}
		if p.DefaultYours {
			at += " (yours)"
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
		urls := portURLs(p)
		from := cmp.Or(repoFrom(p.From), "yours")
		if !p.Named {
			from = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", p.Name, p.Spec, from, at, now, cmp.Or(strings.Join(urls, " "), "-"), p.Description)
	}
	return w.Flush()
}
