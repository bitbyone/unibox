package cli

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
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
know which secrets a sandbox needs. Envs are kept in
~/.config/unibox/config.yaml.`}
	var sandbox string
	var makeDefault bool
	add := &cobra.Command{Use: "add [env] [file...]", Args: cobra.ArbitraryArgs, Short: "Add an env, or change the files of one", Long: `Names sbxenv files as an env. Without a file it takes the one sbxenv file
in this directory, and without a name the name that file gives its sandbox,
else its own. unibox init finds every file of a repository.`, Example: "  unibox env add\n  unibox env add dev ~/team/agent-sandboxes/sandboxes/dev.sbxenv.yaml\n  unibox env add dev dev.sbxenv.yaml local.sbxenv.yaml --name dev-mine", RunE: func(cmd *cobra.Command, args []string) error {
		files := []string{}
		if len(args) > 1 {
			files = args[1:]
		} else {
			here, err := unibox.SBXEnvsIn(".")
			if err != nil {
				return err
			}
			switch len(here) {
			case 0:
				return fmt.Errorf("no sbxenv file in this directory; name one, or run unibox init in your team's repository to find them all")
			case 1:
				files = here
			default:
				return fmt.Errorf("this directory has several sbxenv files (%s); name the one, as in unibox env add <env> <file>", strings.Join(here, ", "))
			}
		}
		paths := []string{}
		for _, f := range files {
			path, err := unibox.ExpandPath(f)
			if err != nil {
				return err
			}
			if _, err = os.Stat(path); err != nil {
				return fmt.Errorf("cannot read %s; give the path of an sbxenv file", f)
			}
			paths = append(paths, path)
		}
		name := firstArg(args)
		if name == "" {
			name = unibox.EnvNameFor(paths[0])
		}
		var rename *string
		if cmd.Flags().Changed("name") {
			rename = &sandbox
		}
		e, err := o.addEnv(cmd.Context(), name, paths, rename, makeDefault)
		if err != nil {
			return err
		}
		values, err := o.paths.LoadEnvValues()
		if err != nil {
			return err
		}
		message := fmt.Sprintf("env %s: sandbox %s from %d file(s)", e.Name, e.SandboxName(), len(e.SBX.Files))
		if missing := e.SBX.Missing(values.ArgsFor(e)); len(missing) > 0 && !o.json {
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

	c.AddCommand(&cobra.Command{Use: "update [env]", Args: cobra.MaximumNArgs(1), Short: "Update the repositories unibox cloned, and say what that changes", Long: `Brings each repository unibox init cloned from an address up to its
remote, by a fast-forward alone, and says what it changes for the envs of
it: values they now ask for, sandboxes to recreate, new sbxenv files. A
repository of your own is never pulled; update it with git.`, RunE: func(cmd *cobra.Command, args []string) error {
		cfg := o.manager.Config
		repos := map[string][]string{}
		if len(args) == 1 {
			ec, ok := cfg.Envs[args[0]]
			if !ok {
				return fmt.Errorf("there is no env %s; unibox env ls lists them", args[0])
			}
			if ec.Source == "" || ec.Repo == "" {
				return unibox.Fail(4, "not_a_clone", "env %s is in a repository of yours, which unibox does not update; update it with git", args[0])
			}
		}
		for _, name := range sorted(cfg.Envs) {
			if ec := cfg.Envs[name]; ec.Source != "" && ec.Repo != "" && (len(args) == 0 || cfg.Envs[args[0]].Repo == ec.Repo) {
				repos[ec.Repo] = append(repos[ec.Repo], name)
			}
		}
		if len(repos) == 0 {
			return o.success("no env is in a repository unibox cloned; unibox init <git-address> clones one")
		}
		values, err := o.paths.LoadEnvValues()
		if err != nil {
			return err
		}
		state, err := o.paths.ReadState()
		if err != nil {
			return err
		}
		said, updated := []string{}, false
		for _, repo := range sorted(repos) {
			moved, err := unibox.Pull(cmd.Context(), repo, o.canAsk(), o.input, o.log)
			if err != nil {
				return err
			}
			if !moved {
				said = append(said, repo+": up to date")
				continue
			}
			said, updated = append(said, repo+": updated"), true
			for _, name := range repos[repo] {
				env, err := o.manager.Config.LoadEnv(name)
				if err != nil {
					said = append(said, fmt.Sprintf("  env %s: %v", name, err))
					continue
				}
				if missing := env.SBX.Missing(values.ArgsFor(env)); len(missing) > 0 {
					said = append(said, fmt.Sprintf("  env %s now asks for %s: unibox var set -e %s %s <value>", name, strings.Join(missing, ", "), name, missing[0]))
				}
				if changed(state, env, values) {
					said = append(said, fmt.Sprintf("  env %s changed for sandbox %s: unibox sandbox recreate %s", name, env.SandboxName(), env.SandboxName()))
				}
			}
			found, err := unibox.FindSBXEnvs(repo)
			if err != nil {
				return err
			}
			fresh := []string{}
			for _, f := range found {
				if _, ok := o.manager.Config.EnvOf(f); !ok {
					fresh = append(fresh, relative(repo, f))
				}
			}
			if len(fresh) > 0 {
				said = append(said, fmt.Sprintf("  new sbxenv file(s) %s: unibox init %s sets them up", strings.Join(fresh, ", "), repo))
			}
		}
		if updated {
			said = append(said, "unibox doctor checks the secrets they need")
		}
		return o.success(strings.Join(said, "\n"))
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
		mine := values.ArgsFor(e)
		if o.json {
			type port struct {
				Port        string `json:"port"`
				Description string `json:"description,omitempty"`
				Team        bool   `json:"team,omitempty"`
				Default     bool   `json:"default"`
			}
			ports := map[string]port{}
			for k, v := range e.Ports {
				ports[k] = port{v.Spec, v.Description, v.Team, v.Default}
			}
			return o.write(map[string]any{"api_version": api.Version, "env": name, "sandbox": e.SandboxName(), "sbxenv": e.SBX, "values": mine, "missing": e.SBX.Missing(mine), "resources": values.ResourcesFor(name, e.SandboxName()), "ports": ports})
		}
		fmt.Fprintf(o.output, "env %s, sandbox %s, agent %s\n", name, e.SandboxName(), e.SBX.Agent)
		for _, f := range e.SBX.Files {
			fmt.Fprintf(o.output, "  file %s\n", f)
		}
		if e.Config.Repo != "" {
			fmt.Fprintf(o.output, "  repo %s\n", e.Config.Repo)
		}
		if r := values.ResourcesFor(name, e.SandboxName()); r != (unibox.Resources{}) {
			resolved, err := r.Resolve(unibox.HostResources(), io.Discard)
			for _, n := range unibox.ResourceNames {
				v := r.Get(n)
				if v == "" {
					continue
				}
				if got := resolved.Get(n); err == nil && got != v {
					v += " (" + got + " here)"
				}
				fmt.Fprintf(o.output, "  %s %s, from vars.toml\n", n, v)
			}
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
		for _, p := range sorted(e.Ports) {
			port := e.Ports[p]
			from := map[string]string{unibox.FromRepo: "the repository's", unibox.FromEnv: "the env's", unibox.FromYoursAll: "yours for every env"}[port.From]
			if from == "" {
				from = "yours"
			}
			at := map[bool]string{true: "on", false: "off"}[port.Default]
			about := ""
			if port.Description != "" {
				about = ": " + port.Description
			}
			fmt.Fprintf(o.output, "  port %s %s, %s, %s at start%s\n", p, port.Spec, from, at, about)
		}
		return nil
	}})
	c.Args = cobra.NoArgs
	c.RunE = func(cmd *cobra.Command, args []string) error {
		ls := func() error { return o.runStep(cmd, "env", "ls") }
		if !o.interactive || o.json {
			return ls()
		}
		return o.envUI(cmd, ls)
	}
	return c
}

// envArgs passes on an env flag a command was given.
func envArgs(env string) []string {
	if env == "" {
		return nil
	}
	return []string{"-e", env}
}

// addEnv keeps an env of files, or gives one new files, with the
// repository they are in; rename, when given, names its sandbox. It is
// made the default when told or when there is none.
func (o *options) addEnv(ctx context.Context, name string, files []string, rename *string, makeDefault bool) (unibox.Env, error) {
	cfg := o.manager.Config
	ec := cfg.Envs[name]
	ec.Files = files
	ec.Repo = unibox.RepoRoot(ctx, filepath.Dir(files[0]))
	ec.Source = o.paths.CloneSource(ctx, ec.Repo)
	if rename != nil {
		ec.Name = *rename
	}
	if err := unibox.ValidEnvConfig(name, ec, cfg.Ports); err != nil {
		return unibox.Env{}, err
	}
	cfg.Envs = maps.Clone(cfg.Envs)
	if cfg.Envs == nil {
		cfg.Envs = map[string]unibox.EnvConfig{}
	}
	cfg.Envs[name] = ec
	if makeDefault || cfg.DefaultEnv == "" {
		cfg.DefaultEnv = name
	}
	if err := o.paths.SaveConfig(cfg); err != nil {
		return unibox.Env{}, err
	}
	o.manager.Config = cfg
	return cfg.LoadEnv(name)
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
