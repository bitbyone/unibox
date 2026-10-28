package cli

import (
	"fmt"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
)

// varCommand keeps each person's values for the args an env's files ask
// for, which sbx env create takes as an --env-args-file.
func (o *options) varCommand() *cobra.Command {
	var envFlag string
	var global, envRepo bool
	c := &cobra.Command{Use: "var", Short: "Keep your own values for the args an env asks for", Long: `An sbxenv file shared by a team declares args each person sets for
themselves - a user name, a mirror. The values are yours, kept in
~/.config/unibox/vars.toml - with --global for every env that declares the
arg, with --env-repo for every env of the env's repository, or for one
env, each winning over the one before - and passed to sbx env create as an
args file. A sandbox is not created while a required one is missing. No
secret belongs here; store tokens with unibox secret.`}
	c.PersistentFlags().StringVarP(&envFlag, "env", "e", "", "Env (default: UNIBOX_ENV, the context's, or the only one)")
	c.PersistentFlags().BoolVarP(&global, "global", "g", false, "For every env that declares the arg, rather than one env")
	c.PersistentFlags().BoolVar(&envRepo, "env-repo", false, "For every env of the env's repository that declares the arg")
	c.MarkFlagsMutuallyExclusive("env", "global")
	c.MarkFlagsMutuallyExclusive("env-repo", "global")
	env := func() (unibox.Env, error) {
		name, err := o.envName(envFlag)
		if err != nil {
			return unibox.Env{}, err
		}
		return o.manager.Config.LoadEnv(name)
	}
	// repoOf is the env repo of the env, as vars.toml keys its values, and
	// how a person knows it.
	repoOf := func(e unibox.Env) (string, string, error) {
		key, err := o.manager.Config.EnvRepoOf(e.Name)
		return key, envRepoText(e.Config), err
	}
	c.AddCommand(&cobra.Command{Use: "set name value", Args: cobra.ExactArgs(2), Short: "Set a value", RunE: func(cmd *cobra.Command, args []string) error {
		if global {
			if err := checkArg(args[0], args[1]); err != nil {
				return err
			}
			v, err := o.paths.LoadEnvValues()
			if err != nil {
				return err
			}
			v.Args[args[0]] = args[1]
			if err = o.paths.SaveEnvValues(v); err != nil {
				return err
			}
			return o.success(fmt.Sprintf("set %s for every env that declares it", args[0]))
		}
		e, err := env()
		if err != nil {
			return err
		}
		if err = checkArg(args[0], args[1]); err != nil {
			return err
		}
		if _, err = e.SBX.CheckArgs(map[string]string{args[0]: args[1]}); err != nil {
			return err
		}
		// A value for every env of the repository may be for another of them.
		if _, declared := e.SBX.Args[args[0]]; !declared && !envRepo {
			fmt.Fprintf(o.log, "env %s declares no arg %s; it is kept, but sbx is not given it\n", e.Name, args[0])
		}
		v, err := o.paths.LoadEnvValues()
		if err != nil {
			return err
		}
		if envRepo {
			key, text, err := repoOf(e)
			if err != nil {
				return err
			}
			if v.EnvRepos[key] == nil {
				v.EnvRepos[key] = map[string]string{}
			}
			v.EnvRepos[key][args[0]] = args[1]
			if err = o.paths.SaveEnvValues(v); err != nil {
				return err
			}
			return o.success(fmt.Sprintf("set %s for every env of %s that declares it", args[0], text))
		}
		if v.Envs[e.Name] == nil {
			v.Envs[e.Name] = map[string]string{}
		}
		v.Envs[e.Name][args[0]] = args[1]
		if err = o.paths.SaveEnvValues(v); err != nil {
			return err
		}
		return o.success(fmt.Sprintf("set %s for env %s", args[0], e.Name))
	}})
	c.AddCommand(&cobra.Command{Use: "rm name", Args: cobra.ExactArgs(1), Short: "Remove a value", RunE: func(cmd *cobra.Command, args []string) error {
		if global {
			v, err := o.paths.LoadEnvValues()
			if err != nil {
				return err
			}
			if _, ok := v.Args[args[0]]; !ok {
				return fmt.Errorf("you have no value for %s for every env; an env's own is removed with -e", args[0])
			}
			delete(v.Args, args[0])
			if err = o.paths.SaveEnvValues(v); err != nil {
				return err
			}
			return o.success(fmt.Sprintf("removed %s for every env", args[0]))
		}
		name, err := o.envName(envFlag)
		if err != nil {
			return err
		}
		v, err := o.paths.LoadEnvValues()
		if err != nil {
			return err
		}
		if envRepo {
			e, err := env()
			if err != nil {
				return err
			}
			key, text, err := repoOf(e)
			if err != nil {
				return err
			}
			if _, ok := v.EnvRepos[key][args[0]]; !ok {
				return fmt.Errorf("you have no value for %s for every env of %s; unibox var ls -e %s says where its value comes from", args[0], text, e.Name)
			}
			delete(v.EnvRepos[key], args[0])
			if err = o.paths.SaveEnvValues(v); err != nil {
				return err
			}
			return o.success(fmt.Sprintf("removed %s for every env of %s", args[0], text))
		}
		if _, ok := v.Envs[name][args[0]]; !ok {
			return fmt.Errorf("env %s has no value of yours for %s", name, args[0])
		}
		delete(v.Envs[name], args[0])
		if err = o.paths.SaveEnvValues(v); err != nil {
			return err
		}
		return o.success(fmt.Sprintf("removed %s for env %s", args[0], name))
	}})
	c.AddCommand(&cobra.Command{Use: "ls", Args: cobra.NoArgs, Short: "List what an env asks for and what you set", RunE: func(cmd *cobra.Command, args []string) error {
		v, err := o.paths.LoadEnvValues()
		if err != nil {
			return err
		}
		if global || envRepo {
			values, from, out := v.Args, "global", map[string]any{"api_version": api.Version, "file": o.paths.VarsFile()}
			if envRepo {
				e, err := env()
				if err != nil {
					return err
				}
				key, _, err := repoOf(e)
				if err != nil {
					return err
				}
				values, from, out["env_repo"] = v.EnvRepos[key], "env-repo", key
			}
			rows := []map[string]string{}
			for _, name := range sorted(values) {
				rows = append(rows, map[string]string{"name": name, "value": values[name], "from": from})
			}
			if o.json {
				out["args"] = rows
				return o.write(out)
			}
			w := tabwriter.NewWriter(o.output, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tVALUE")
			for _, r := range rows {
				fmt.Fprintf(w, "%s\t%s\n", r["name"], r["value"])
			}
			return w.Flush()
		}
		e, err := env()
		if err != nil {
			return err
		}
		mine, ofRepo := v.Envs[e.Name], envRepoValues(v, e)
		rows := []map[string]string{}
		for _, name := range sorted(e.SBX.Args) {
			a := e.SBX.Args[name]
			from, value := "missing", ""
			if s, ok := mine[name]; ok {
				from, value = "yours", s
			} else if s, ok := ofRepo[name]; ok {
				from, value = "env-repo", s
			} else if s, ok := v.Args[name]; ok {
				from, value = "global", s
			} else if a.Default != nil {
				from, value = "default", *a.Default
			} else if !a.Required {
				from = "optional"
			}
			rows = append(rows, map[string]string{"name": name, "value": value, "from": from, "description": a.Description})
		}
		for _, name := range sorted(mine) {
			if _, ok := e.SBX.Args[name]; !ok {
				rows = append(rows, map[string]string{"name": name, "value": mine[name], "from": "undeclared"})
			}
		}
		if o.json {
			return o.write(map[string]any{"api_version": api.Version, "env": e.Name, "file": o.paths.VarsFile(), "args": rows})
		}
		if len(rows) == 0 {
			fmt.Fprintf(o.output, "env %s asks for no args\n", e.Name)
			return nil
		}
		w := tabwriter.NewWriter(o.output, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tVALUE\tFROM\tFOR")
		for _, r := range rows {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r["name"], r["value"], r["from"], r["description"])
		}
		return w.Flush()
	}})
	c.Args = cobra.NoArgs
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if !o.interactive || o.json {
			return o.runStep(cmd, append([]string{"var", "ls"}, varScope(envFlag, global, envRepo)...)...)
		}
		name := ""
		if !global {
			var err error
			if name, err = o.envName(envFlag); err != nil {
				return err
			}
		}
		return o.varUI(cmd, name, global, envRepo, func() error {
			return o.runStep(cmd, append([]string{"var", "ls"}, varScope(name, global, envRepo)...)...)
		})
	}
	return c
}

// envRepoValues are the person's values for every env of an env's
// repository.
func envRepoValues(v unibox.EnvValues, e unibox.Env) map[string]string {
	// Finding the repository may ask git; there is no need without values.
	if len(v.EnvRepos) == 0 {
		return nil
	}
	return v.EnvRepos[unibox.EnvRepoKey(e.Config)]
}

func varScope(env string, global, envRepo bool) []string {
	if global {
		return []string{"-g"}
	}
	if envRepo {
		return append(envArgs(env), "--env-repo")
	}
	return envArgs(env)
}

func sorted[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// askArgs asks, at a terminal, for the args an env requires and the person
// has not set, and keeps them as unibox var set would. It says whether it
// set any.
func (o *options) askArgs(env unibox.Env) (bool, error) {
	if !o.canAsk() {
		return false, nil
	}
	values, err := o.paths.LoadEnvValues()
	if err != nil {
		return false, err
	}
	missing := env.SBX.Missing(values.ArgsFor(env))
	if len(missing) == 0 {
		return false, nil
	}
	mine := values.Envs[env.Name]
	if mine == nil {
		mine = map[string]string{}
	}
	fmt.Fprintf(o.log, "env %s asks for values of your own, kept for it in vars.toml:\n", env.Name)
	for _, name := range missing {
		value, err := o.askText(name, env.SBX.Args[name].Description, "", func(s string) error {
			if s == "" {
				return fmt.Errorf("%s is required", name)
			}
			if err := checkArg(name, s); err != nil {
				return err
			}
			_, err := env.SBX.CheckArgs(map[string]string{name: s})
			return err
		})
		if err != nil {
			return false, err
		}
		mine[name] = value
	}
	values.Envs[env.Name] = mine
	return true, o.paths.SaveEnvValues(values)
}

// checkArg is unibox.ValidArg, and a share of the machine refused for an
// arg sbx reads as an amount: unibox's resources take one.
func checkArg(name, value string) error {
	if err := unibox.ValidArg(name, value); err != nil {
		return err
	}
	if slices.Contains(unibox.ResourceNames, name) && strings.HasSuffix(value, "%") {
		return fmt.Errorf("arg %s goes to sbx, which takes no share; a share is unibox's resources: unibox sandbox resources --%s %s for one sandbox, or [envs.<env>.resources] in vars.toml", name, name, value)
	}
	return nil
}
