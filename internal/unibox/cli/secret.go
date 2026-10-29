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
	var envFlag, sandboxFlag string
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
--generate opens it without asking.

A token is global unless --scope says otherwise: env gives the env's
sandboxes one of their own, sandbox one sandbox (-s) its own over its
env's. Typed in, unibox has the system's keychain ask for it and keep it -
security on macOS, secret-tool on Linux, PowerShell on Windows - and sbx
reads it from there at every start; --command gives a command of yours
instead. rm --scope gives the scope the token over it again. Alone at a
terminal, s in the list moves a token between the scopes.`, Example: `  unibox secret ls -e dev
  unibox secret set -e dev
  unibox secret set github -s personal --scope sandbox
  unibox secret rm github -s personal --scope sandbox
  unibox secret set copilot --generate
  unibox secret set copilot --command 'security find-generic-password -s copilot -w'
  security find-generic-password -s artifactory -w | unibox secret set artifactory`}
	c.PersistentFlags().StringVarP(&envFlag, "env", "e", "", "Env (default: UNIBOX_ENV, the context's, or the only one)")
	c.PersistentFlags().StringVarP(&sandboxFlag, "sandbox", "s", "", "Sandbox whose tokens to show or keep (default: the env's)")
	needed := func(cmd *cobra.Command) (unibox.Env, []unibox.NeededSecret, error) {
		name, err := o.secretEnv(envFlag, sandboxFlag)
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
			l, err = o.manager.NeededSecrets(cmd.Context(), env, sandboxFlag)
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
			stored, needed := storedText(l), "yes"
			if l.Optional {
				needed = "optional"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", l.Service, stored, needed, cmp.Or(l.Description, strings.Join(l.Kits, ", "), "the env's bindings"))
		}
		return w.Flush()
	}})
	var command, scopeFlag string
	var all, generate, envOnly bool
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
		scope, err := o.secretScope(env, scopeFlag, envOnly, sandboxFlag)
		if err != nil {
			return err
		}
		if scope != (unibox.SecretScope{}) {
			if len(args) != 1 {
				return fmt.Errorf("--scope %s keeps one service's token; name the service", scopeFlag)
			}
			return o.setOwnSecret(cmd, env, scope, args[0], command, generate)
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
			var way unibox.TokenWay
			if command == "" && o.canAsk() {
				if way, err = o.offerTokenPage(service, guide, generate); err != nil {
					return err
				}
			}
			if way.Agent {
				// Nothing is stored: the agent signs in, and sbx keeps that.
				continue
			}
			set := func() error {
				return o.manager.SBX.SecretSet(cmd.Context(), service, command, o.input, o.output, o.log)
			}
			switch {
			case way.SignIn:
				set = func() error { return o.manager.SBX.SecretSignIn(cmd.Context(), service, o.input, o.output, o.log) }
			case way.Env != "":
				set = func() error {
					return o.manager.SBX.SecretSetCustom(cmd.Context(), way.Env, way.Placeholder, way.Hosts, o.input, o.output, o.log)
				}
			}
			if err := set(); err != nil {
				return tokenNotStored{err}
			}
			stored = append(stored, service)
		}
		if len(stored) == 0 {
			return o.success("sbx stored no token; the agents sign in themselves when they first start")
		}
		return o.success(fmt.Sprintf("sbx has stored %s", strings.Join(stored, ", ")))
	}}
	set.Flags().StringVar(&command, "command", "", "A command sbx runs on the host for the token, instead of asking for it")
	set.Flags().BoolVar(&all, "all", false, "Ask for the optional tokens too")
	set.Flags().BoolVarP(&generate, "generate", "g", false, "Open the page where a new token is made, then ask for it")
	set.Flags().StringVar(&scopeFlag, "scope", "global", "global, the one sbx keeps for every sandbox; env, the env's sandboxes' own; sandbox, one sandbox's own (-s)")
	set.Flags().BoolVar(&envOnly, "env-only", false, "The same as --scope env")
	_ = set.Flags().MarkHidden("env-only")
	c.AddCommand(set)
	var rmEnvOnly bool
	var rmScope string
	rm := &cobra.Command{Use: "rm service...", Args: cobra.MinimumNArgs(1), Short: "Have sbx forget a service's global token, or a scope its own", RunE: func(cmd *cobra.Command, args []string) error {
		env, _, err := needed(cmd)
		if err != nil {
			return err
		}
		scope, err := o.secretScope(env, rmScope, rmEnvOnly, sandboxFlag)
		if err != nil {
			return err
		}
		if scope != (unibox.SecretScope{}) {
			for _, service := range args {
				if err := o.manager.RemoveOwnSecret(cmd.Context(), scope, env.Name, service); err != nil {
					return err
				}
			}
			over := "global"
			if scope.Sandbox != "" && len(env.Config.Secrets) > 0 {
				over = "env's or the global"
			}
			return o.success(fmt.Sprintf("%s takes the %s %s token again", scope, over, strings.Join(args, ", ")))
		}
		guides, err := unibox.TokenGuides(env.Config.Files)
		if err != nil {
			return err
		}
		secrets, err := o.manager.SBX.Secrets(cmd.Context())
		if err != nil {
			return err
		}
		removed := []string{}
		for _, service := range args {
			for _, s := range secrets {
				if s.Scope != "global" {
					continue
				}
				switch {
				case s.Type != "custom" && s.Name == service:
					err = o.manager.SBX.SecretRemove(cmd.Context(), service, "")
				case s.Type == "custom" && slices.ContainsFunc(guides[service].AllWays(), func(w unibox.TokenWay) bool { return w.Env != "" && w.Env == s.Env }):
					// A subscription's token kept in a variable is the
					// service's too.
					err = o.manager.SBX.SecretRemove(cmd.Context(), "", s.Placeholder)
				default:
					continue
				}
				if err != nil {
					return err
				}
				removed = append(removed, service)
			}
		}
		if len(removed) == 0 {
			return o.success("sbx has no global token of " + strings.Join(args, ", "))
		}
		return o.success("sbx forgot the tokens of " + strings.Join(slices.Compact(removed), ", "))
	}}
	rm.Flags().StringVar(&rmScope, "scope", "global", "global, sbx's own; env or sandbox (-s), the token kept for them, which then take the one over them")
	rm.Flags().BoolVar(&rmEnvOnly, "env-only", false, "The same as --scope env")
	_ = rm.Flags().MarkHidden("env-only")
	c.AddCommand(rm)
	c.Args = cobra.NoArgs
	c.RunE = func(cmd *cobra.Command, args []string) error {
		ls := func() error { return o.runStep(cmd, append([]string{"secret", "ls"}, envArgs(envFlag)...)...) }
		if !o.interactive || o.json {
			return ls()
		}
		name, err := o.secretEnv(envFlag, sandboxFlag)
		if err != nil {
			return err
		}
		listArgs := []string{"secret", "ls", "-e", name}
		if sandboxFlag != "" {
			listArgs = append(listArgs, "-s", sandboxFlag)
		}
		return o.secretUI(cmd, name, sandboxFlag, func() error { return o.runStep(cmd, listArgs...) })
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
func (o *options) offerTokenPage(service string, guide unibox.TokenGuide, generate bool) (way unibox.TokenWay, err error) {
	ways := guide.AllWays()
	if len(ways) == 0 {
		if generate {
			return way, fmt.Errorf("unibox knows no way the %s token is made; the env's <env>.unibox.yaml can name one under secrets:", service)
		}
		return way, nil
	}
	picks := []pick{}
	if len(guide.Ways) == 0 && !generate {
		picks = append(picks, pick{"have", "I have a token"})
	}
	for i, w := range ways {
		picks = append(picks, pick{strconv.Itoa(i), w.Name})
	}
	way = ways[0]
	if len(picks) > 1 {
		choice, err := o.askPick("The "+service+" token", guide.Hint, picks, picks[0].value)
		if err != nil {
			return way, err
		}
		if choice == "have" {
			return unibox.TokenWay{}, nil
		}
		i, _ := strconv.Atoi(choice)
		way = ways[i]
	}
	if way.Hint != "" {
		fmt.Fprintf(o.log, "  %s\n", way.Hint)
	}
	switch {
	case way.SignIn, way.Agent:
		return way, nil
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
	return way, nil
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

// secretEnv is the env whose tokens are meant: the one named, else the
// named sandbox's, else the default.
func (o *options) secretEnv(envFlag, sandbox string) (string, error) {
	if envFlag == "" && sandbox != "" {
		if state, err := o.paths.ReadState(); err == nil {
			if b, ok := state.Sandboxes[sandbox]; ok && b.Env != "" {
				return b.Env, nil
			}
		}
	}
	return o.envName(envFlag)
}

// secretScope reads --scope: where a token is kept.
func (o *options) secretScope(env unibox.Env, flag string, envOnly bool, sandbox string) (unibox.SecretScope, error) {
	if envOnly {
		flag = "env"
	}
	switch flag {
	case "", "global":
		return unibox.SecretScope{}, nil
	case "env":
		return unibox.SecretScope{Env: env.Name}, nil
	case "sandbox":
		return unibox.SecretScope{Sandbox: cmp.Or(sandbox, env.SandboxName())}, nil
	}
	return unibox.SecretScope{}, fmt.Errorf("--scope is global, env or sandbox, not %s", flag)
}

// storedText says where a token an env needs is kept, or that none is.
func storedText(l unibox.NeededSecret) string {
	switch {
	case !l.Stored:
		return "no"
	case l.Own != "":
		return l.Scope + ", " + l.Own
	}
	return l.Scope
}

// setOwnSecret keeps a token of a scope's own: read by the command given,
// or typed into the system's keychain, which its own program asks for on
// this terminal. The page where it is made is offered first, as for a
// global one.
func (o *options) setOwnSecret(cmd *cobra.Command, env unibox.Env, scope unibox.SecretScope, service, command string, generate bool) error {
	ctx := cmd.Context()
	v := unibox.EnvSecret{Command: command}
	if command == "" {
		if !o.canAsk() {
			return fmt.Errorf("the keychain asks for the %s token on a terminal; run this in one, or give --command", service)
		}
		if err := unibox.KeychainAvailable(); err != nil {
			return err
		}
		guides, err := unibox.TokenGuides(env.Config.Files)
		if err != nil {
			return err
		}
		way, err := o.offerTokenPage(service, guides[service], generate)
		if err != nil {
			return err
		}
		if way.SignIn || way.Agent || way.Env != "" {
			return fmt.Errorf("%s is kept by sbx itself, which keeps it for every sandbox alike; give %s a token of its own, typed in or with --command", way.Name, scope)
		}
		item := unibox.KeychainItem(scope, service)
		fmt.Fprintf(o.log, "%s asks for the %s token of %s; it goes there, not through unibox\n", unibox.KeychainName(), service, scope)
		if err := o.paths.KeychainStore(ctx, item, "unibox: the "+service+" token of "+scope.String(), o.input, o.output, o.log); err != nil {
			return err
		}
		v = unibox.EnvSecret{Command: o.paths.KeychainCommand(item), Keychain: item}
	}
	if err := o.manager.SetOwnSecret(ctx, scope, env.Name, service, v); err != nil {
		return err
	}
	if v.Keychain != "" {
		return o.success(fmt.Sprintf("%s takes its own %s token, kept in %s", scope, service, unibox.KeychainName()))
	}
	return o.success(fmt.Sprintf("%s takes its own %s token, which sbx reads with %s", scope, service, command))
}
