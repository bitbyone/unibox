package cli

import (
	"cmp"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
)

// resourcesCommand gives one sandbox its own share of the machine, over
// what its env's sandboxes get; sbx takes it at the next create.
func (o *options) resourcesCommand() *cobra.Command {
	var set unibox.Resources
	c := &cobra.Command{Use: "resources [sandbox]", Args: cobra.MaximumNArgs(1), Short: "Show or set the CPUs and memory of one sandbox", Long: `Show or set the CPUs and memory one sandbox gets, over those of its env and
of every env. cpus is a count, 0 for all of them, or a share of this
machine's such as 50%; memory a size such as 16g, or a share. A value of
- gives back the env's. They are kept in vars.toml as
[sandboxes.<name>.resources] and reach sbx when the sandbox is created
again: unibox sandbox recreate applies them.`, Example: "  unibox sandbox resources dev\n  unibox sandbox resources dev --cpus 50% --memory 16g\n  unibox sandbox resources dev --memory -", RunE: func(cmd *cobra.Command, args []string) error {
		name, err := o.sandboxName(first(args))
		if err != nil {
			return err
		}
		state, err := o.manager.Paths.ReadState()
		if err != nil {
			return err
		}
		b, ok := state.Sandboxes[name]
		if !ok {
			return fmt.Errorf("unibox has no sandbox %s; unibox sandbox create %s --cpus/--memory makes one with its own", name, name)
		}
		values, err := o.manager.Paths.LoadEnvValues()
		if err != nil {
			return err
		}
		own := values.SandboxResources[name]
		changed := false
		for _, r := range unibox.ResourceNames {
			if !cmd.Flags().Changed(r) {
				continue
			}
			v := set.Get(r)
			if v == "-" {
				v = ""
			} else if err := unibox.ValidResource(r, v); err != nil {
				return err
			}
			if own.Get(r) != v {
				own.Set(r, v)
				changed = true
			}
		}
		if changed {
			if err = o.manager.Paths.SetSandboxResources(name, own); err != nil {
				return err
			}
			values.SandboxResources[name] = own
		}
		return o.printResources(b, values, changed)
	}}
	c.Flags().StringVar(&set.CPUs, "cpus", "", "CPUs: a count, 0 for all, or a share such as 50%; - for the env's")
	c.Flags().StringVar(&set.Memory, "memory", "", "Memory: a size such as 16g, or a share such as 75%; - for the env's")
	return c
}

func (o *options) printResources(b api.Sandbox, values unibox.EnvValues, changed bool) error {
	own := values.SandboxResources[b.Name]
	all := values.ResourcesFor(b.Env, b.Name)
	if o.json {
		return o.write(map[string]any{"api_version": api.Version, "sandbox": b.Name, "env": b.Env, "own": own, "resources": all})
	}
	for _, r := range unibox.ResourceNames {
		fmt.Fprintf(o.output, "%-7s %s\n", r, cmp.Or(all.Get(r), "as the env's files say")+resourceFrom(values, b, r))
	}
	if changed && b.State != "missing" {
		fmt.Fprintf(o.output, "unibox sandbox recreate %s applies it\n", b.Name)
	}
	return nil
}

// resourceFrom says where a sandbox's resource comes from.
func resourceFrom(values unibox.EnvValues, b api.Sandbox, r string) string {
	switch {
	case values.SandboxResources[b.Name].Get(r) != "":
		return " (its own)"
	case values.EnvResources[b.Env].Get(r) != "":
		return " (env " + b.Env + "'s)"
	case values.Resources.Get(r) != "":
		return " (every env's)"
	}
	return ""
}
