package cli

import (
	"cmp"
	"fmt"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
)

// secretCommand shows and stores the tokens an env's sandboxes use. They
// are sbx's secrets: unibox reads which are stored, never a value, and has
// sbx ask for a missing one on the person's own terminal.
func (o *options) secretCommand() *cobra.Command {
	var envFlag string
	c := &cobra.Command{Use: "secret", Short: "The tokens an env's sandboxes use, kept by sbx", Long: `An env's kits declare the services they take a token for, and its
bindings name the hosts each token may reach. The tokens are sbx's secrets,
stored once on this machine and used by every sandbox that needs them; the
proxy puts them into requests, so none enters a sandbox.

ls lists them and whether sbx has each. set has sbx ask for the missing
ones on this terminal - the value goes from the keyboard to sbx, never
through unibox - or with --command stores a command sbx runs on the host
for it, a keychain read say. Before sbx asks, set offers to open the page
where a new token is made, when it knows one: GitHub's for github and
copilot, and what the team names under secrets: in its unibox.yaml files;
--generate opens it without asking.`, Example: `  unibox secret ls -e dev
  unibox secret set -e dev
  unibox secret set copilot --generate
  unibox secret set copilot --command 'security find-generic-password -s copilot -w'
  security find-generic-password -s artifactory -w | unibox secret set artifactory`}
	c.PersistentFlags().StringVarP(&envFlag, "env", "e", "", "Env (default: UNIBOX_ENV, the context's, or the only one)")
	needed := func(cmd *cobra.Command) (unibox.Env, []unibox.NeededSecret, error) {
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
		var l []unibox.NeededSecret
		err = o.waiting("reading the kits of env "+env.Name, func() (err error) {
			l, err = o.manager.NeededSecrets(cmd.Context(), env, "")
			return err
		})
		return env, l, err
	}
	c.AddCommand(&cobra.Command{Use: "ls", Args: cobra.NoArgs, Short: "List the tokens an env needs and whether sbx has each", RunE: func(cmd *cobra.Command, args []string) error {
		env, list, err := needed(cmd)
		if err != nil {
			return err
		}
		if o.json {
			return o.write(map[string]any{"api_version": api.Version, "env": env.Name, "secrets": list})
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
	var all, generate bool
	set := &cobra.Command{Use: "set [service...]", Short: "Have sbx store the tokens an env lacks, or those named", RunE: func(cmd *cobra.Command, args []string) error {
		env, list, err := needed(cmd)
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
		if command != "" && generate {
			return fmt.Errorf("--generate makes a token to type in, --command has sbx run one; give one of them")
		}
		// Away from a terminal sbx reads the value from stdin, which holds
		// one.
		if command == "" && !o.canAsk() && len(services) != 1 {
			return fmt.Errorf("sbx asks for each token on a terminal; run this in one, or pipe one service's token in")
		}
		if generate && !o.canAsk() {
			return fmt.Errorf("--generate opens the page where the token is made, then sbx asks for it; run this at a terminal, or pipe the token in without --generate")
		}
		guides, err := unibox.TokenGuides(env.Config.Files)
		if err != nil {
			return err
		}
		stored := []string{}
		for _, service := range services {
			if !unibox.ValidSecretName(service) {
				return fmt.Errorf("%q is no service name sbx takes; use letters, digits and dashes", service)
			}
			guide := guides[service]
			i := slices.IndexFunc(list, func(l unibox.NeededSecret) bool { return l.Service == service })
			if i < 0 {
				fmt.Fprintf(o.log, "env %s does not ask for %s; it is stored all the same\n", env.Name, service)
			} else if o.canAsk() && command == "" {
				fmt.Fprintf(o.log, "%s: %s\n", service, cmp.Or(list[i].Description, "the token for "+service))
			}
			signIn := false
			if command == "" && o.canAsk() {
				if signIn, err = o.offerTokenPage(service, guide, generate); err != nil {
					return err
				}
			}
			set := func() error {
				return o.manager.SBX.SecretSet(cmd.Context(), service, command, o.input, o.output, o.log)
			}
			if signIn {
				set = func() error { return o.manager.SBX.SecretSignIn(cmd.Context(), service, o.input, o.output, o.log) }
			}
			if err := set(); err != nil {
				return tokenNotStored{err}
			}
			stored = append(stored, service)
		}
		return o.success(fmt.Sprintf("sbx has stored %s", strings.Join(stored, ", ")))
	}}
	set.Flags().StringVar(&command, "command", "", "A command sbx runs on the host for the token, instead of asking for it")
	set.Flags().BoolVar(&all, "all", false, "Ask for the optional tokens too")
	set.Flags().BoolVarP(&generate, "generate", "g", false, "Open the page where a new token is made, then ask for it")
	c.AddCommand(set)
	c.Args = cobra.NoArgs
	c.RunE = func(cmd *cobra.Command, args []string) error {
		ls := func() error { return o.runStep(cmd, append([]string{"secret", "ls"}, envArgs(envFlag)...)...) }
		if !o.interactive || o.json {
			return ls()
		}
		name, err := o.envName(envFlag)
		if err != nil {
			return err
		}
		return o.secretUI(cmd, name, func() error { return o.runStep(cmd, "secret", "ls", "-e", name) })
	}
	return c
}

// tokenNotStored is a token sbx did not store, which unibox secret asks
// for again with the others still missing; init says how to go on there.
type tokenNotStored struct{ err error }

func (e tokenNotStored) Error() string {
	return e.err.Error() + "; run unibox secret to set the tokens still missing"
}

func (e tokenNotStored) Unwrap() error { return e.err }

// offerTokenPage asks, before sbx asks for a token, how the person gives
// it: each way the team or unibox knows - a subscription's token a command
// prints, a sign-in sbx does in the browser, a key made on a page - or, for
// a token made one way, whether they have one already. It says whether sbx
// is to sign them in rather than ask for the token. A service with no way
// goes straight to sbx.
func (o *options) offerTokenPage(service string, guide unibox.TokenGuide, generate bool) (signIn bool, err error) {
	ways := guide.AllWays()
	if len(ways) == 0 {
		if generate {
			return false, fmt.Errorf("unibox knows no way the %s token is made; the env's <env>.unibox.yaml can name one under secrets:", service)
		}
		return false, nil
	}
	picks := []pick{}
	if len(guide.Ways) == 0 && !generate {
		picks = append(picks, pick{"have", "I have a token"})
	}
	for i, w := range ways {
		picks = append(picks, pick{strconv.Itoa(i), w.Name})
	}
	way := ways[0]
	if len(picks) > 1 {
		choice, err := o.askPick("The "+service+" token", guide.Hint, picks, picks[0].value)
		if err != nil {
			return false, err
		}
		if choice == "have" {
			return false, nil
		}
		i, _ := strconv.Atoi(choice)
		way = ways[i]
	}
	if way.Hint != "" {
		fmt.Fprintf(o.log, "  %s\n", way.Hint)
	}
	switch {
	case way.SignIn:
		return true, nil
	case way.Command != "":
		// unibox runs nothing for a token: what the command prints is the
		// person's to paste, and goes to sbx alone.
		fmt.Fprintf(o.log, "  run %s in another terminal, then paste the token it prints here\n", way.Command)
	case way.Page != "":
		if err := openPage(way.Page); err != nil {
			fmt.Fprintf(o.log, "  make it at %s, then paste it here\n", way.Page)
		} else {
			fmt.Fprintf(o.log, "  the browser opens %s; make the token there, then paste it here\n", way.Page)
		}
	}
	return false, nil
}

// openPage opens a web address in the person's browser. A browser that
// keeps its opener waiting is left to it after a moment.
func openPage(page string) error {
	name, args := "xdg-open", []string{page}
	switch runtime.GOOS {
	case "darwin":
		name = "open"
	case "windows":
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler", page}
	}
	c := exec.Command(name, args...)
	if err := c.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		return nil
	}
}
