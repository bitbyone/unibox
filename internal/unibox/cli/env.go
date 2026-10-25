package cli

import (
	"fmt"
	"maps"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
)

// envCommand keeps the user's envs: names for a set of the team's sbxenv
// files, which unibox reads and hands to sbx as they are.
func (o *options) envCommand() *cobra.Command {
	c := &cobra.Command{Use: "env", Short: "Name the sbxenv files a sandbox is created from", Long: `An env names one or more sbxenv files - the team's, as sbx reads them -
merged by sbx in the order given; a later file, such as a local overlay,
overrides an earlier one. unibox never changes the files: it reads the args
they declare, which you set once with unibox var set, and their kits, to
know which logins a sandbox needs. Envs are kept in
~/.config/unibox/config.yaml.`}
	var sandbox string
	var makeDefault bool
	add := &cobra.Command{Use: "add env file...", Args: cobra.MinimumNArgs(2), Short: "Add an env, or change the files of one", Example: "  unibox env add dev ~/team/agent-sandboxes/sandboxes/dev.sbxenv.yaml\n  unibox env add dev dev.sbxenv.yaml local.sbxenv.yaml --name dev-mine", RunE: func(cmd *cobra.Command, args []string) error {
		cfg := o.manager.Config
		ec := cfg.Envs[args[0]]
		ec.Files = nil
		for _, f := range args[1:] {
			path, err := unibox.ExpandPath(f)
			if err != nil {
				return err
			}
			if _, err = os.Stat(path); err != nil {
				return fmt.Errorf("cannot read %s; give the path of an sbxenv file", f)
			}
			ec.Files = append(ec.Files, path)
		}
		if cmd.Flags().Changed("name") {
			ec.Name = sandbox
		}
		if err := unibox.ValidEnvConfig(args[0], ec); err != nil {
			return err
		}
		if cfg.Envs == nil {
			cfg.Envs = map[string]unibox.EnvConfig{}
		}
		cfg.Envs[args[0]] = ec
		if makeDefault || cfg.DefaultEnv == "" {
			cfg.DefaultEnv = args[0]
		}
		if err := o.paths.SaveConfig(cfg); err != nil {
			return err
		}
		o.manager.Config = cfg
		e, err := cfg.LoadEnv(args[0])
		if err != nil {
			return err
		}
		values, err := o.paths.LoadEnvValues()
		if err != nil {
			return err
		}
		message := fmt.Sprintf("env %s: sandbox %s from %d file(s)", e.Name, e.SandboxName(), len(e.SBX.Files))
		if missing := e.SBX.Missing(values.Envs[e.Name]); len(missing) > 0 && !o.json {
			message += fmt.Sprintf("\nit needs a value of yours for %v; set each with unibox var set -e %s <name> <value>", missing, e.Name)
		}
		return o.success(message)
	}}
	add.Flags().StringVar(&sandbox, "name", "", "The sandbox's name (default: the file's name:, or the env's)")
	add.Flags().BoolVar(&makeDefault, "default", false, "Make it the env new sandboxes come from")
	c.AddCommand(add)

	c.AddCommand(&cobra.Command{Use: "ls", Args: cobra.NoArgs, Short: "List the envs", RunE: func(cmd *cobra.Command, args []string) error {
		cfg := o.manager.Config
		if o.json {
			return o.write(map[string]any{"api_version": api.Version, "default": cfg.DefaultEnv, "envs": cfg.Envs})
		}
		if len(cfg.Envs) == 0 {
			fmt.Fprintln(o.output, "no envs; add one with unibox env add <name> <file.sbxenv.yaml>")
			return nil
		}
		w := tabwriter.NewWriter(o.output, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "ENV\tSANDBOX\tFILES")
		for _, name := range sorted(cfg.Envs) {
			mark, sandbox := name, "?"
			if name == cfg.DefaultEnv {
				mark += " (default)"
			}
			if e, err := cfg.LoadEnv(name); err == nil {
				sandbox = e.SandboxName()
			}
			fmt.Fprintf(w, "%s\t%s\t%d\n", mark, sandbox, len(cfg.Envs[name].Files))
		}
		return w.Flush()
	}})

	c.AddCommand(&cobra.Command{Use: "rm env", Args: cobra.ExactArgs(1), Short: "Forget an env; its files and sandbox stay", RunE: func(cmd *cobra.Command, args []string) error {
		cfg := o.manager.Config
		if _, ok := cfg.Envs[args[0]]; !ok {
			return fmt.Errorf("there is no env %s; unibox env ls lists them", args[0])
		}
		cfg.Envs = maps.Clone(cfg.Envs)
		delete(cfg.Envs, args[0])
		if cfg.DefaultEnv == args[0] {
			cfg.DefaultEnv = ""
		}
		if cfg.Context.Env == args[0] {
			cfg.Context.Env = ""
		}
		if err := o.paths.SaveConfig(cfg); err != nil {
			return err
		}
		return o.success("removed env " + args[0])
	}})

	c.AddCommand(&cobra.Command{Use: "show [env]", Args: cobra.MaximumNArgs(1), Short: "Show what an env's files declare and what you set", RunE: func(cmd *cobra.Command, args []string) error {
		name, err := o.envName(firstArg(args))
		if err != nil {
			return err
		}
		e, err := o.manager.Config.LoadEnv(name)
		if err != nil {
			return err
		}
		values, err := o.paths.LoadEnvValues()
		if err != nil {
			return err
		}
		mine := values.Envs[name]
		if o.json {
			return o.write(map[string]any{"api_version": api.Version, "env": name, "sandbox": e.SandboxName(), "sbxenv": e.SBX, "values": mine, "missing": e.SBX.Missing(mine), "ports": e.Config.Ports})
		}
		fmt.Fprintf(o.output, "env %s, sandbox %s, agent %s\n", name, e.SandboxName(), e.SBX.Agent)
		for _, f := range e.SBX.Files {
			fmt.Fprintf(o.output, "  file %s\n", f)
		}
		w := tabwriter.NewWriter(o.output, 0, 4, 2, ' ', 0)
		if len(e.SBX.Args) > 0 {
			fmt.Fprintln(w, "ARG\tVALUE\tFROM\tFOR")
			for _, a := range sorted(e.SBX.Args) {
				decl := e.SBX.Args[a]
				value, from := mine[a], "you"
				if _, ok := mine[a]; !ok {
					switch {
					case decl.Default != nil:
						value, from = *decl.Default, "default"
					case decl.Required:
						from = "missing"
					default:
						from = "unset"
					}
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", a, value, from, decl.Description)
			}
		}
		if err = w.Flush(); err != nil {
			return err
		}
		for _, k := range e.SBX.Kits {
			fmt.Fprintf(o.output, "  kit %s\n", k.Source)
		}
		for _, p := range sorted(e.Config.Ports) {
			fmt.Fprintf(o.output, "  port %s %s\n", p, e.Config.Ports[p].Spec)
		}
		return nil
	}})
	return c
}

func firstArg(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ""
}

// defaultEnv is the env a command works with when it is not told: from
// UNIBOX_ENV, the context, the configuration's default, or the only one.
func (o *options) defaultEnv() string {
	cfg := o.manager.Config
	for _, e := range []string{os.Getenv("UNIBOX_ENV"), cfg.Context.Env, cfg.DefaultEnv} {
		if e != "" {
			return e
		}
	}
	if len(cfg.Envs) == 1 {
		for name := range cfg.Envs {
			return name
		}
	}
	return ""
}

func (o *options) envName(flag string) (string, error) {
	if flag != "" {
		return flag, nil
	}
	if e := o.defaultEnv(); e != "" {
		return e, nil
	}
	return "", fmt.Errorf("no env chosen; pass one (%s), or add one with unibox env add <name> <file.sbxenv.yaml>", orNoneOf(sorted(o.manager.Config.Envs), "unibox env ls lists them"))
}
