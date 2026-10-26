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
// team's files, and no command stands between a person and the file.
func (o *options) portCommand() *cobra.Command {
	c := &cobra.Command{Use: "port", Short: "Switch a running sandbox's ports, and list them", Long: `A port is published on the host's loopback. The ports are named in files,
by hand: the team's in unibox.yaml at the root of its repository and in
the unibox file beside an env's sbxenv file (dev.unibox.yaml beside
dev.sbxenv.yaml), yours in ~/.config/unibox/config.yaml, for every env or
for one, each over the one before. What a port says as default is
whether a sandbox starts with it published.

on and off switch a running sandbox's ports for as long as it runs: by
name, by name=HOST_PORT for a named port on another host port, or by
--map for a mapping no file names. Its next start goes back to the
files.

Alone at a terminal, unibox port shows a running sandbox's ports to
switch with the arrows or h and l.

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
	c.AddCommand(turn(true), turn(false), ls)
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
	}}
	// The theme asks the terminal for its background; asked here, before
	// the program reads keys, the answer is not taken for typing.
	lipgloss.HasDarkBackground()
	if _, err := tea.NewProgram(m, tea.WithInput(o.input), tea.WithOutput(o.log), tea.WithContext(ctx)).Run(); err != nil {
		return err
	}
	return o.listPorts(cmd, name)
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
			out = append(out, api.Port{Name: p.Name, Port: p.Spec, Description: p.Description, Team: p.Team, From: p.From, Default: p.Default, Named: p.Named, On: p.On, Published: published})
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
		from := cmp.Or(p.From, "you")
		if !p.Named {
			from = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", p.Name, p.Spec, from, at, now, cmp.Or(strings.Join(urls, " "), "-"), p.Description)
	}
	return w.Flush()
}
