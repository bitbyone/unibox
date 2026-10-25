package cli

import (
	"fmt"
	"slices"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
)

// varCommand keeps each person's values for the args an env's files ask
// for, which sbx env create takes as an --env-args-file.
func (o *options) varCommand() *cobra.Command {
	var envFlag string
	c := &cobra.Command{Use: "var", Short: "Keep your own values for the args an env asks for", Long: `An sbxenv file shared by a team declares args each person sets for
themselves - a user name, a mirror. The values are yours, kept per env in
~/.config/unibox/vars.toml and passed to sbx env create as an args file. A
sandbox is not created while a required one is missing. No secret belongs
here; store tokens with unibox login.`}
	c.PersistentFlags().StringVarP(&envFlag, "env", "e", "", "Env (default: UNIBOX_ENV, the context's, or the only one)")
	env := func() (unibox.Env, error) {
		name, err := o.envName(envFlag)
		if err != nil {
			return unibox.Env{}, err
		}
		return o.manager.Config.LoadEnv(name)
	}
	c.AddCommand(&cobra.Command{Use: "set name value", Args: cobra.ExactArgs(2), Short: "Set a value", RunE: func(cmd *cobra.Command, args []string) error {
		e, err := env()
		if err != nil {
			return err
		}
		if err = unibox.ValidArg(args[0], args[1]); err != nil {
			return err
		}
		if _, err = e.SBX.CheckArgs(map[string]string{args[0]: args[1]}); err != nil {
			return err
		}
		if _, declared := e.SBX.Args[args[0]]; !declared {
			fmt.Fprintf(o.log, "env %s declares no arg %s; it is kept, but sbx is not given it\n", e.Name, args[0])
		}
		v, err := o.paths.LoadEnvValues()
		if err != nil {
			return err
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
		name, err := o.envName(envFlag)
		if err != nil {
			return err
		}
		v, err := o.paths.LoadEnvValues()
		if err != nil {
			return err
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
		e, err := env()
		if err != nil {
			return err
		}
		v, err := o.paths.LoadEnvValues()
		if err != nil {
			return err
		}
		mine := v.Envs[e.Name]
		rows := []map[string]string{}
		for _, name := range sorted(e.SBX.Args) {
			a := e.SBX.Args[name]
			from, value := "missing", ""
			if s, ok := mine[name]; ok {
				from, value = "yours", s
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
	return c
}

func sorted[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
