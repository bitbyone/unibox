package cli

import (
	"fmt"
	"slices"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
)

// varCommand keeps each person's values for what profiles ask of them, in
// their home: shared by every profile, or one profile's own.
func (o *options) varCommand() *cobra.Command {
	var profile string
	c := &cobra.Command{Use: "var", Short: "Keep your own values for what profiles ask of you (their vars)", Long: `A profile shared by a team names what each person sets for themselves - a
user name, a commit identity - under vars. The values are yours, kept in
~/.config/unibox/vars.toml and never in the environments repository: a value
without --profile applies to every profile, one with --profile to that
profile alone, and wins. A run stops while a profile's value is missing.
unibox sets them in every session and for the profile's start script. No
secret belongs here; keep tokens with unibox secret set.`}
	scope := func() string {
		if profile == "" {
			return "every profile"
		}
		return "profile " + profile
	}
	c.PersistentFlags().StringVarP(&profile, "profile", "p", "", "One profile's own value, over the shared one")
	c.AddCommand(&cobra.Command{Use: "set name value", Args: cobra.ExactArgs(2), Short: "Set a value", RunE: func(cmd *cobra.Command, args []string) error {
		if err := unibox.ValidVar(args[0], args[1]); err != nil {
			return err
		}
		if profile != "" {
			if _, err := o.manager.Profile(profile); err != nil {
				return err
			}
		}
		u, err := o.paths.LoadVars()
		if err != nil {
			return err
		}
		if profile == "" {
			u.Vars[args[0]] = args[1]
		} else {
			if u.Profiles[profile] == nil {
				u.Profiles[profile] = map[string]string{}
			}
			u.Profiles[profile][args[0]] = args[1]
		}
		if err = o.paths.SaveVars(u); err != nil {
			return err
		}
		return o.success(fmt.Sprintf("set %s for %s", args[0], scope()))
	}})
	c.AddCommand(&cobra.Command{Use: "rm name", Args: cobra.ExactArgs(1), Short: "Remove a value", RunE: func(cmd *cobra.Command, args []string) error {
		u, err := o.paths.LoadVars()
		if err != nil {
			return err
		}
		values := u.Vars
		if profile != "" {
			values = u.Profiles[profile]
		}
		if _, ok := values[args[0]]; !ok {
			return fmt.Errorf("%s has no value for %s", scope(), args[0])
		}
		delete(values, args[0])
		if err = o.paths.SaveVars(u); err != nil {
			return err
		}
		return o.success(fmt.Sprintf("removed %s for %s", args[0], scope()))
	}})
	c.AddCommand(&cobra.Command{Use: "ls", Args: cobra.NoArgs, Short: "List your values, or with --profile what the profile asks and where each value comes from", RunE: func(cmd *cobra.Command, args []string) error {
		u, err := o.paths.LoadVars()
		if err != nil {
			return err
		}
		if profile == "" {
			if o.json {
				return o.write(map[string]any{"api_version": api.Version, "file": o.paths.VarsFile(), "vars": u.Vars, "profiles": u.Profiles})
			}
			w := tabwriter.NewWriter(o.output, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tVALUE\tFOR")
			for _, name := range sorted(u.Vars) {
				fmt.Fprintf(w, "%s\t%s\tevery profile\n", name, u.Vars[name])
			}
			for _, p := range sorted(u.Profiles) {
				for _, name := range sorted(u.Profiles[p]) {
					fmt.Fprintf(w, "%s\t%s\tprofile %s\n", name, u.Profiles[p][name], p)
				}
			}
			return w.Flush()
		}
		p, err := o.manager.Profile(profile)
		if err != nil {
			return err
		}
		rows := []map[string]string{}
		defaults := p.VarDefaults()
		for _, name := range sorted(p.Vars) {
			from, value := "missing", p.VarValues[name]
			if _, ok := u.Profiles[profile][name]; ok {
				from = "profile " + profile
			} else if _, ok := u.Vars[name]; ok {
				from = "every profile"
			} else if d, ok := defaults[name]; ok {
				from, value = "default", d
			}
			rows = append(rows, map[string]string{"name": name, "description": p.Vars[name], "value": value, "from": from})
		}
		if o.json {
			return o.write(map[string]any{"api_version": api.Version, "profile": profile, "vars": rows})
		}
		if len(rows) == 0 {
			fmt.Fprintf(o.output, "profile %s asks for no values\n", profile)
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
