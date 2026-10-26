package cli

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
)

// loginCommand shows and stores the tokens an env's sandboxes use. They
// are sbx's secrets: unibox reads which are stored, never a value, and has
// sbx ask for a missing one on the person's own terminal.
func (o *options) loginCommand() *cobra.Command {
	var envFlag string
	c := &cobra.Command{Use: "login", Short: "The tokens an env's sandboxes use, kept by sbx", Long: `An env's kits declare the services they take a token for, and its
bindings name the hosts each token may reach. The tokens are sbx's secrets,
stored once on this machine and used by every sandbox that needs them; the
proxy puts them into requests, so none enters a sandbox.

ls lists them and whether sbx has each. set has sbx ask for the missing
ones on this terminal - the value goes from the keyboard to sbx, never
through unibox - or with --command stores a command sbx runs on the host
for it, a keychain read say.`, Example: `  unibox login ls -e dev
  unibox login set -e dev
  unibox login set copilot --command 'security find-generic-password -s copilot -w'
  security find-generic-password -s artifactory -w | unibox login set artifactory`}
	c.PersistentFlags().StringVarP(&envFlag, "env", "e", "", "Env (default: UNIBOX_ENV, the context's, or the only one)")
	logins := func(cmd *cobra.Command) (unibox.Env, []unibox.Login, error) {
		name, err := o.envName(envFlag)
		if err != nil {
			return unibox.Env{}, nil, err
		}
		env, err := o.manager.Config.LoadEnv(name)
		if err != nil {
			return env, nil, err
		}
		if err = o.manager.SBX.Available(cmd.Context()); err != nil {
			return env, nil, err
		}
		l, err := o.manager.Logins(cmd.Context(), env, "")
		return env, l, err
	}
	c.AddCommand(&cobra.Command{Use: "ls", Args: cobra.NoArgs, Short: "List the tokens an env needs and whether sbx has each", RunE: func(cmd *cobra.Command, args []string) error {
		env, list, err := logins(cmd)
		if err != nil {
			return err
		}
		if o.json {
			return o.write(map[string]any{"api_version": api.Version, "env": env.Name, "logins": list})
		}
		if len(list) == 0 {
			fmt.Fprintf(o.output, "env %s needs no token\n", env.Name)
			return nil
		}
		w := tabwriter.NewWriter(o.output, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "SERVICE\tSTORED\tNEEDED\tFOR")
		for _, l := range list {
			stored, needed := "no", "yes"
			if l.Stored {
				stored = l.Scope
			}
			if l.Optional {
				needed = "optional"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", l.Service, stored, needed, cmp.Or(l.Description, strings.Join(l.Kits, ", "), "the env's bindings"))
		}
		return w.Flush()
	}})
	var command string
	var all bool
	set := &cobra.Command{Use: "set [service...]", Short: "Have sbx store the tokens an env lacks, or those named", RunE: func(cmd *cobra.Command, args []string) error {
		env, list, err := logins(cmd)
		if err != nil {
			return err
		}
		services := args
		if len(services) == 0 {
			for _, l := range list {
				if !l.Stored && (!l.Optional || all) {
					services = append(services, l.Service)
				}
			}
			if len(services) == 0 {
				return o.success(fmt.Sprintf("sbx has every token env %s needs", env.Name))
			}
		}
		if command != "" && len(services) != 1 {
			return fmt.Errorf("--command stores one service's token; name the service")
		}
		// Away from a terminal sbx reads the value from stdin, which holds
		// one.
		if command == "" && !o.canAsk() && len(services) != 1 {
			return fmt.Errorf("sbx asks for each token on a terminal; run this in one, or pipe one service's token in")
		}
		for _, service := range services {
			if !unibox.ValidSecretName(service) {
				return fmt.Errorf("%q is no service name sbx takes; use letters, digits and dashes", service)
			}
			i := slices.IndexFunc(list, func(l unibox.Login) bool { return l.Service == service })
			if i < 0 {
				fmt.Fprintf(o.log, "env %s does not ask for %s; it is stored all the same\n", env.Name, service)
			} else if o.canAsk() && command == "" {
				fmt.Fprintf(o.log, "%s: %s\n", service, cmp.Or(list[i].Description, "the token for "+service))
			}
			if err := o.manager.SBX.SecretSet(cmd.Context(), service, command, o.input, o.output, o.log); err != nil {
				return err
			}
		}
		return o.success(fmt.Sprintf("sbx has stored %s", strings.Join(services, ", ")))
	}}
	set.Flags().StringVar(&command, "command", "", "A command sbx runs on the host for the token, instead of asking for it")
	set.Flags().BoolVar(&all, "all", false, "Ask for the optional tokens too")
	c.AddCommand(set)
	c.Args = cobra.NoArgs
	c.RunE = func(cmd *cobra.Command, args []string) error {
		ls := func() error { return o.runStep(cmd, append([]string{"login", "ls"}, envArgs(envFlag)...)...) }
		if !o.interactive || o.json {
			return ls()
		}
		name, err := o.envName(envFlag)
		if err != nil {
			return err
		}
		return o.loginUI(cmd, name, func() error { return o.runStep(cmd, "login", "ls", "-e", name) })
	}
	return c
}
