package cli

import (
	"bytes"
	"cmp"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

// readSecret takes a value from the user: on a hidden prompt at a terminal,
// else one line of stdin, so unagit can hand one over a pipe. It is never
// echoed, logged or returned in JSON.
func (o *options) readSecret(prompt string) ([]byte, error) {
	var value []byte
	if f, ok := o.input.(*os.File); ok && o.interactive {
		fmt.Fprint(o.log, prompt)
		b, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(o.log)
		if err != nil {
			return nil, err
		}
		value = b
	} else {
		// One byte at a time, so nothing after the line is taken from a
		// reader another question will read.
		var line []byte
		one := make([]byte, 1)
		for {
			n, err := o.input.Read(one)
			if n == 1 {
				if one[0] == '\n' {
					break
				}
				line = append(line, one[0])
			}
			if err != nil {
				break
			}
		}
		value = line
	}
	value = bytes.TrimSpace(value)
	if len(value) == 0 {
		return nil, fmt.Errorf("no value given; paste the token at the prompt, or pipe it on stdin")
	}
	return value, nil
}

// openPage shows the user where a token is made, in the browser when there
// is a terminal to come back to.
func (o *options) openPage(page string) {
	fmt.Fprintf(o.log, "  %s\n", page)
	if !o.interactive {
		return
	}
	opener := map[string]string{"darwin": "open", "linux": "xdg-open"}[runtime.GOOS]
	if opener != "" {
		_ = exec.Command(opener, page).Start()
	}
}

// GHCommand is the host command that prints the token of the user's gh
// login, for copilot or a GitHub service to use instead of a token of their
// own.
const GHCommand = "gh auth token"

// useGH says whether to sign in through the user's gh login: --gh says so;
// at a terminal, with gh signed in and no token of its own stored yet,
// the user is asked. It says what that gives away.
func (o *options) useGH(cmd *cobra.Command, flag bool, what string, stored bool) (bool, error) {
	signedIn := exec.CommandContext(cmd.Context(), "gh", "auth", "status", "--hostname", "github.com").Run() == nil
	if flag {
		if !signedIn {
			return false, fmt.Errorf("gh is not signed in to github.com; run gh auth login, or leave out --gh for a token of its own")
		}
	} else if stored || !signedIn || !o.interactive || o.json {
		return false, nil
	} else if !o.ask("  gh is signed in here; sign " + what + " in through it (gh auth token) instead of a token of its own?") {
		return false, nil
	}
	fmt.Fprintf(o.log, "  %s signs in through gh auth token: your whole gh login, every repository and scope it has; a token of its own is narrower\n", what)
	return true, nil
}

func (o *options) secretStore() (unibox.SecretStore, error) {
	return unibox.PlatformSecrets()
}

func (o *options) secretCommand() *cobra.Command {
	c := &cobra.Command{Use: "secret", Short: "Keep tokens in the platform's store, for profiles to name as secret:<name>"}
	c.AddCommand(&cobra.Command{Use: "set name", Args: cobra.ExactArgs(1), Short: "Store a token, asked for on a hidden prompt or read from stdin", Long: `Store a token in the platform's store - the login keychain on macOS, the
Secret Service on Linux - under a name. It is asked for on a hidden prompt,
or read from stdin, and never shown again.

A profile names it as secret:<name>; unibox service add --secret <name>
writes a service that uses it, and an agent's auth can name it too.`, Example: "  unibox secret set artifactory\n  unibox service add artifactory -p dev --host artifactory.example.com --secret artifactory", RunE: func(cmd *cobra.Command, args []string) error {
		if err := unibox.ValidName(args[0]); err != nil {
			return err
		}
		store, err := o.secretStore()
		if err != nil {
			return err
		}
		value, err := o.readSecret("token for " + args[0] + ": ")
		if err != nil {
			return err
		}
		if err = store.Set(cmd.Context(), args[0], value); err != nil {
			return err
		}
		return o.success(fmt.Sprintf("stored secret %s in %s; a profile names it as secret:%s", args[0], store.Name(), args[0]))
	}})
	c.AddCommand(&cobra.Command{Use: "ls", Args: cobra.NoArgs, Short: "List the stored secrets by name, never their values", RunE: func(cmd *cobra.Command, args []string) error {
		store, err := o.secretStore()
		if err != nil {
			return err
		}
		names, err := store.List(cmd.Context())
		if err != nil {
			return err
		}
		if o.json {
			return o.write(map[string]any{"api_version": api.Version, "store": store.Name(), "secrets": names})
		}
		if len(names) == 0 {
			fmt.Fprintf(o.output, "no secrets in %s\n", store.Name())
		}
		for _, n := range names {
			fmt.Fprintln(o.output, n)
		}
		return nil
	}})
	var profile string
	var reset bool
	source := &cobra.Command{Use: "source [command]", Args: cobra.MaximumNArgs(1), Short: "Show or set the command a profile's secret:<name> is read by, instead of the platform's store", Long: `A profile names a token as secret:<name>, read from the platform's store -
the login keychain, the Secret Service - unless you keep your tokens
elsewhere: a command of yours, with {name} standing for the secret's name,
reads them instead, for every profile or with -p for one. It is kept in
your vars.toml, not in the profile, so a profile from a template stays as
it is. The runtime runs the command on the host, as it would the store's.`, Example: "  unibox secret source 'op read op://Work/{name}/credential'\n  unibox secret source 'vault kv get -field=token secret/{name}' -p dev\n  unibox secret source --reset", RunE: func(cmd *cobra.Command, args []string) error {
		u, err := o.paths.LoadVars()
		if err != nil {
			return err
		}
		scope := "every profile"
		if profile != "" {
			scope = "profile " + profile
		}
		switch {
		case reset && len(args) > 0:
			return fmt.Errorf("give a command or --reset, not both")
		case reset:
			if profile == "" {
				u.Secrets = ""
			} else {
				delete(u.ProfileSecrets, profile)
			}
		case len(args) == 1:
			if err = unibox.ValidSecretCommand(args[0]); err != nil {
				return err
			}
			if profile == "" {
				u.Secrets = args[0]
			} else {
				u.ProfileSecrets[profile] = args[0]
			}
		default:
			command := u.Secrets
			if profile != "" {
				command = u.SecretsFor(profile)
			}
			if o.json {
				return o.write(map[string]any{"api_version": api.Version, "profile": profile, "command": command})
			}
			if command == "" {
				store, err := o.secretStore()
				if err != nil {
					return err
				}
				fmt.Fprintf(o.output, "%s reads secret:<name> from %s\n", scope, store.Name())
				return nil
			}
			fmt.Fprintf(o.output, "%s reads secret:<name> by: %s\n", scope, command)
			return nil
		}
		if err = o.paths.SaveVars(u); err != nil {
			return err
		}
		// What the runtime has stored goes from the old source until a
		// sandbox's next start gives it the new one.
		how := "from the platform's store"
		if command := u.Secrets; profile != "" {
			if command = u.SecretsFor(profile); command != "" {
				how = "by " + command
			}
		} else if command != "" {
			how = "by " + command
		}
		return o.success(fmt.Sprintf("%s reads secret:<name> %s from its sandboxes' next start", scope, how))
	}}
	source.Flags().StringVarP(&profile, "profile", "p", "", "One profile's own command, over the one of every profile")
	source.Flags().BoolVar(&reset, "reset", false, "Read from the platform's store again")
	c.AddCommand(source)
	c.AddCommand(&cobra.Command{Use: "rm name", Args: cobra.ExactArgs(1), Short: "Remove a stored secret", RunE: func(cmd *cobra.Command, args []string) error {
		store, err := o.secretStore()
		if err != nil {
			return err
		}
		if err = store.Remove(cmd.Context(), args[0]); err != nil {
			return err
		}
		return o.success("removed secret " + args[0])
	}})
	return c
}

// ensureSecret stores a secret unless it is there already and not renewed:
// it says where the token is made, then asks for it.
func (o *options) ensureSecret(cmd *cobra.Command, store unibox.SecretStore, name string, renew bool, explain []string, page string) error {
	if o.secretsVia != "" {
		fmt.Fprintf(o.log, "  secret %s is read by your command: %s\n", name, strings.ReplaceAll(o.secretsVia, unibox.SecretNameField, name))
		return nil
	}
	if ok, err := store.Has(cmd.Context(), name); err != nil {
		return err
	} else if ok && !renew {
		fmt.Fprintf(o.log, "  secret %s is stored; --renew replaces it\n", name)
		return nil
	}
	for _, line := range explain {
		fmt.Fprintf(o.log, "  %s\n", line)
	}
	if page != "" {
		o.openPage(page)
	}
	value, err := o.readSecret("  paste the token: ")
	if err != nil {
		return err
	}
	if err = store.Set(cmd.Context(), name, value); err != nil {
		return err
	}
	fmt.Fprintf(o.log, "  stored secret %s in %s\n", name, store.Name())
	return nil
}

// recreateFor offers to recreate the sandboxes whose runtime cannot take a
// change of what live; each one declined keeps the old until it is.
func (o *options) recreateFor(cmd *cobra.Command, names []string, what string) error {
	for _, name := range names {
		if err := o.confirm("Runtime of sandbox " + name + " cannot change " + what + " live; recreate it to apply them?"); err != nil {
			fmt.Fprintf(o.log, "sandbox %s keeps its old %s until it is recreated\n", name, what)
			continue
		}
		if err := o.manager.Recreate(cmd.Context(), name, false); err != nil {
			return err
		}
	}
	return nil
}

func (o *options) applyPorts(cmd *cobra.Command, profile string) error {
	recreate, err := o.manager.ApplyPorts(cmd.Context(), profile)
	if err != nil {
		return err
	}
	return o.recreateFor(cmd, recreate, "ports")
}

func (o *options) loginCommand() *cobra.Command {
	var renew, viaGH bool
	var rt, profile, command string
	c := &cobra.Command{Use: "login [agent...]", Args: cobra.ArbitraryArgs, Short: "Sign a profile's agents in: tokens into the platform's store, the profile's auth written", RunE: func(cmd *cobra.Command, args []string) error {
		name, err := o.profileName(profile)
		if err != nil {
			return err
		}
		if err = o.login(cmd, name, args, renew, viaGH, rt, command); err != nil {
			return err
		}
		return o.success(fmt.Sprintf("profile %s signs its agents in", name))
	}}
	c.Flags().BoolVar(&renew, "renew", false, "Ask for every token again, replacing the stored ones")
	c.Flags().BoolVar(&viaGH, "gh", false, "Sign copilot in through your gh login (gh auth token) instead of a token of its own")
	c.Flags().StringVar(&rt, "runtime", "", "Runtime that keeps the logins (default: the configured one)")
	c.Flags().StringVar(&command, "command", "", "A host command printing the agent's token, for a store unibox does not keep")
	c.MarkFlagsMutuallyExclusive("command", "gh")
	c.MarkFlagsMutuallyExclusive("command", "renew")
	profileFlag(c, &profile)
	return c
}

// offerLogin asks, at the end of an interactive init, whether to sign the
// profile's agents in now, as unibox agent login would, and does.
func (o *options) offerLogin(cmd *cobra.Command, name string) (bool, error) {
	if missing := o.runtimeMissing(); missing != "" {
		fmt.Fprintf(o.log, "the agents are signed in once a runtime is installed: unibox agent login -p %s\n", name)
		return false, nil
	}
	sign, err := o.askYes("Sign the agents of profile "+name+" in now?", true)
	if err != nil || !sign {
		return false, err
	}
	return true, o.login(cmd, name, nil, false, false, "", "")
}

// login signs a profile's agents in, the named ones or all: a token into
// the platform's store where unibox knows how to get one, the profile's
// auth written, and the runtime told each source.
func (o *options) login(cmd *cobra.Command, name string, args []string, renew, viaGH bool, rt, command string) error {
	ctx := cmd.Context()
	req := unibox.Request{Profile: name, Runtime: rt}
	_, r, p, err := o.manager.Definition(req)
	if err != nil {
		return err
	}
	o.secretsVia = p.SecretCommand
	agents := args
	if len(agents) == 0 {
		agents = p.Agents
	}
	if command != "" && len(args) != 1 {
		return fmt.Errorf("--command signs one agent in; name it, as in unibox agent login claude --command ...")
	}
	store, err := o.secretStore()
	if err != nil {
		return err
	}
	auths := map[string]unibox.AgentAuth{}
	for _, id := range agents {
		if !slices.Contains(p.Agents, id) {
			return fmt.Errorf("profile %s does not offer agent %s", p.Name, id)
		}
		fmt.Fprintf(o.log, "%s:\n", id)
		if command != "" {
			// A store of the user's own - another program's vault -
			// prints the token; unibox neither asks for it nor keeps it.
			method, ok := commandAuth[id]
			if !ok {
				return fmt.Errorf("unibox cannot sign %s in by a command; set its auth in sandbox.yaml", id)
			}
			auths[id] = unibox.AgentAuth{Method: method, Command: command}
			fmt.Fprintf(o.log, "  its token comes from %s\n", command)
			continue
		}
		switch id {
		case "codex":
			if r.Name() == "msb" {
				// msb keeps no ChatGPT login; an API key from the
				// platform's store signs codex in, in sbx as well.
				if err := o.ensureSecret(cmd, store, "openai", renew, []string{
					"msb signs codex in by an OpenAI API key, not a ChatGPT login",
				}, "https://platform.openai.com/api-keys"); err != nil {
					return err
				}
				auths[id] = unibox.AgentAuth{Method: unibox.AuthCommand, Command: unibox.SecretPrefix + "openai"}
				continue
			}
			auths[id] = unibox.AgentAuth{Method: unibox.AuthOAuth}
			fmt.Fprintln(o.log, "  the ChatGPT login is kept by the runtime, which opens the browser for it")
		case "claude":
			if ok, _ := store.Has(ctx, "claude"); o.secretsVia == "" && (!ok || renew) {
				if _, err := exec.LookPath("claude"); err == nil && o.interactive {
					fmt.Fprintln(o.log, "  claude setup-token signs your subscription in for a year and prints the token")
					setup := exec.CommandContext(ctx, "claude", "setup-token")
					if f, ok := o.input.(*os.File); ok {
						setup.Stdin = f
					}
					setup.Stdout, setup.Stderr = o.log, o.log
					if err := setup.Run(); err != nil {
						return fmt.Errorf("claude setup-token did not finish; run it yourself and paste the token: %w", err)
					}
				} else {
					fmt.Fprintln(o.log, "  run claude setup-token where claude is installed; it prints a token for your subscription")
				}
			}
			if err := o.ensureSecret(cmd, store, "claude", renew, nil, ""); err != nil {
				return err
			}
			auths[id] = unibox.AgentAuth{Method: unibox.AuthToken, Command: unibox.SecretPrefix + "claude"}
		case "copilot":
			stored, _ := store.Has(ctx, "copilot")
			gh, err := o.useGH(cmd, viaGH, "copilot", stored && !renew)
			if err != nil {
				return err
			}
			if gh {
				// A token login, so copilot's hosts alone get it; the
				// github secret of command would take github.com over.
				auths[id] = unibox.AgentAuth{Method: unibox.AuthToken, Command: GHCommand}
				continue
			}
			err = o.ensureSecret(cmd, store, "copilot", renew, []string{
				"make a fine-grained token on GitHub: no repositories, Account permissions > Copilot Requests: Read-only, nothing else",
				"it signs copilot in alone; git reaches GitHub through the profile's services",
			}, "https://github.com/settings/personal-access-tokens/new")
			if err != nil {
				return err
			}
			auths[id] = unibox.AgentAuth{Method: unibox.AuthToken, Command: unibox.SecretPrefix + "copilot"}
		default:
			fmt.Fprintf(o.log, "  unibox has no guided login for %s; set its auth in sandbox.yaml\n", id)
		}
	}
	changed := false
	err = o.manager.EditProfile(p.Name, func(doc *yaml.Node) error {
		for id, a := range auths {
			if old := p.Auth[id]; old.Method != a.Method || old.Command != a.Command {
				unibox.SetAgentAuth(doc, id, a)
				changed = true
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if changed {
		fmt.Fprintf(o.log, "wrote the agents' auth into profile %s\n", p.Name)
		if p, err = o.manager.Profile(p.Name); err != nil {
			return err
		}
	}
	// The runtime is told each source now, so the first run starts
	// straight away; a token's is stored again, as its command may
	// have changed.
	if a, ok := r.(unibox.Authenticator); ok {
		needs := a.AuthNeeds(ctx, "unibox-"+p.Name, p)
		for _, id := range agents {
			method := p.Auth[id].Method
			missing := slices.ContainsFunc(needs, func(n api.Need) bool { return n.Code == "login_required" && n.Agent == id })
			switch method {
			case unibox.AuthToken, unibox.AuthCommand:
				// sbx's notes on a stored secret are kept for a failure.
				var said bytes.Buffer
				if err := o.manager.Login(ctx, req, id, strings.NewReader(""), &said, &said); err != nil {
					fmt.Fprint(o.log, said.String())
					return err
				}
			case unibox.AuthOAuth, unibox.AuthAPIKey:
				if !missing && !renew {
					continue
				}
				if err := o.manager.Login(ctx, req, id, o.input, o.log, o.log); err != nil {
					return err
				}
			default:
				continue
			}
			if r.Name() == "msb" && method != unibox.AuthAPIKey {
				fmt.Fprintf(o.log, "%s's %s login is fetched each time msb boots the sandbox\n", id, method)
				continue
			}
			fmt.Fprintf(o.log, "stored %s's %s login in %s\n", id, method, r.Name())
		}
	}
	return nil
}

// commandAuth is how an agent signs in by a command's token: claude and
// copilot read a login token from their environment, codex an API key
// the runtime keeps.
var commandAuth = map[string]string{
	"claude":  unibox.AuthToken,
	"copilot": unibox.AuthToken,
	"codex":   unibox.AuthCommand,
}

// serviceKind is what unibox knows of a particular service: its defaults
// and where its token is made.
type serviceKind struct {
	git                bool
	host, user, scheme string
	env                string
	page               func(host, name string) string
	explain            []string
}

var serviceKinds = map[string]serviceKind{
	"github": {git: true, host: "github.com", user: "x-access-token", page: func(host, name string) string {
		return "https://github.com/settings/tokens/new?scopes=repo&description=" + url.QueryEscape("unibox "+name)
	}, explain: []string{
		"a classic token with repo reaches every repository you can, of every owner;",
		"a fine-grained one (Contents: Read and write) is narrower but covers one owner - give it a path, --host github.com/<owner>",
		"leave workflow unticked, so the agent cannot change GitHub Actions",
	}},
	"gitlab": {git: true, host: "gitlab.com", user: "oauth2", page: func(host, name string) string {
		return "https://" + host + "/-/user_settings/personal_access_tokens?name=" + url.QueryEscape("unibox "+name) + "&scopes=read_repository,write_repository"
	}, explain: []string{"a personal access token with read_repository and write_repository"}},
	"artifactory": {env: "ARTIFACTORY_TOKEN", explain: []string{
		"an identity token from your Artifactory user profile (Edit Profile > Generate an Identity Token)",
	}},
	"generic": {},
}

func (o *options) serviceCommand() *cobra.Command {
	c := &cobra.Command{Use: "service", Short: "Sign a profile's sandboxes in to forges and other host services"}
	var kind, user, scheme, env, author, refresh, command, secret, target string
	var hosts []string
	var git, noCoAuthor, renew, viaGH bool
	add := &cobra.Command{Use: "add name", Args: cobra.ExactArgs(1), Short: "Store a service's token and write its entry into the profile", RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		profile, err := o.profileName(target)
		if err != nil {
			return err
		}
		if p, err := o.manager.Profile(profile); err == nil {
			o.secretsVia = p.SecretCommand
		}
		if err := unibox.ValidName(name); err != nil {
			return err
		}
		if kind == "" {
			for k := range serviceKinds {
				if k != "generic" && strings.HasPrefix(name, k) {
					kind = k
				}
			}
		}
		if kind == "" {
			return fmt.Errorf("say what %s is with --kind github, gitlab, artifactory or generic", name)
		}
		k, ok := serviceKinds[kind]
		if !ok {
			return fmt.Errorf("unknown --kind %s; use github, gitlab, artifactory or generic", kind)
		}
		s := unibox.Service{Git: k.git || git, Scheme: k.scheme, User: k.user, Author: author, Refresh: refresh}
		if len(hosts) > 0 {
			s.Host = hosts
		} else if k.host != "" {
			s.Host = unibox.HostList{k.host}
		}
		envName := k.env
		if cmd.Flags().Changed("env") {
			envName = env
			s.Git, s.User, s.Scheme = false, "", ""
		}
		for flag, set := range map[string]func(string){"user": func(v string) { s.User = v }, "scheme": func(v string) { s.Scheme = v }} {
			if cmd.Flags().Changed(flag) {
				set(map[string]string{"user": user, "scheme": scheme}[flag])
			}
		}
		if !s.Git && (s.User != "" || s.Scheme != "") {
			return fmt.Errorf("--user and --scheme are a git service's; a tool makes its own header from the variable --env names")
		}
		if s.Git {
			envName = ""
		}
		if s.Git && (s.Scheme == unibox.SchemeBearer || s.Scheme == unibox.SchemeRaw) {
			s.User = ""
		}
		if noCoAuthor {
			no := false
			s.CoAuthor = &no
		}
		if len(s.Host) == 0 {
			return fmt.Errorf("give %s its host with --host", name)
		}
		if secret != "" && (command != "" || viaGH) {
			return fmt.Errorf("--secret names where the token is; leave out --command and --gh")
		}
		gh := false
		if kind == "github" && command == "" && secret == "" {
			stored := false
			if store, err := o.secretStore(); err == nil {
				stored, _ = store.Has(cmd.Context(), name)
			}
			var err error
			if gh, err = o.useGH(cmd, viaGH, name, stored && !renew); err != nil {
				return err
			}
		} else if viaGH {
			return fmt.Errorf("--gh is for a github service; %s is %s", name, kind)
		}
		if gh {
			s.Command = GHCommand
		} else if secret != "" {
			// A token stored before, perhaps for several services, is
			// named rather than asked for again.
			if err := unibox.ValidName(secret); err != nil {
				return err
			}
			store, err := o.secretStore()
			if err != nil {
				return err
			}
			if ok, err := store.Has(cmd.Context(), secret); err != nil {
				return err
			} else if !ok && o.secretsVia == "" {
				return fmt.Errorf("no secret %s is stored; store it with unibox secret set %s", secret, secret)
			}
			s.Command = unibox.SecretPrefix + secret
		} else if command != "" {
			// A store unibox does not know - Vault, 1Password - is the
			// user's command, run by the runtime as it is.
			s.Command = command
		} else {
			store, err := o.secretStore()
			if err != nil {
				return err
			}
			page := ""
			if k.page != nil {
				host, _, _ := strings.Cut(s.Host[0], "/")
				page = k.page(host, name)
			}
			fmt.Fprintf(o.log, "%s:\n", name)
			if err := o.ensureSecret(cmd, store, name, renew, k.explain, page); err != nil {
				return err
			}
			s.Command = unibox.SecretPrefix + name
		}
		if err := o.manager.EditProfile(profile, func(doc *yaml.Node) error {
			unibox.SetService(doc, name, s)
			// A tool finds the placeholder in the variable env names.
			if envName != "" {
				unibox.SetEnvValue(doc, envName, "${services."+name+"}")
			}
			return nil
		}); err != nil {
			return err
		}
		where := strings.Join(s.ProxyHosts(), ", ")
		if s.Git {
			where = "git at " + strings.Join(s.GitURLs(), ", ")
		} else if envName != "" {
			where += ", through " + envName
		} else {
			fmt.Fprintf(o.log, "name its placeholder in the profile's env for a tool to send it: - NAME: ${services.%s}, or add it again with --env NAME\n", name)
		}
		return o.success(fmt.Sprintf("profile %s signs %s in to %s; the next start applies it", profile, name, where))
	}}
	f := add.Flags()
	f.StringVar(&kind, "kind", "", "github, gitlab, artifactory or generic (default: from the name)")
	f.StringSliceVar(&hosts, "host", nil, "Host, or host/path for one owner on a git forge (repeatable)")
	f.BoolVar(&git, "git", false, "Wire the sandbox's git to the hosts (generic services)")
	f.StringVar(&user, "user", "", "User a git forge pairs with the token (basic scheme)")
	f.StringVar(&scheme, "scheme", "", "A git service's header: basic, bearer or raw")
	f.StringVar(&env, "env", "", "Variable a tool reads the placeholder from, written into the profile's env as NAME: ${services.<name>}")
	f.StringVar(&author, "author", "", "Who commits under the service, as Name <email> (--author on a run wins)")
	f.BoolVar(&noCoAuthor, "no-co-author", false, "No Co-Authored-By trailer for the agent in its repositories")
	f.StringVar(&refresh, "refresh", "", "How often the runtime fetches the token again, e.g. 10m")
	f.StringVar(&command, "command", "", "A host command printing the token, for a store unibox does not keep (Vault, 1Password)")
	f.StringVar(&secret, "secret", "", "A token stored with unibox secret set, by its name, instead of asking for one")
	f.BoolVar(&renew, "renew", false, "Ask for the token again, replacing the stored one")
	f.BoolVar(&viaGH, "gh", false, "Sign a github service in through your gh login (gh auth token)")
	profileFlag(add, &target)
	c.AddCommand(add)

	ls := &cobra.Command{Use: "ls", Args: cobra.NoArgs, Short: "List a profile's services and whether their tokens are stored", RunE: func(cmd *cobra.Command, args []string) error {
		name, err := o.profileName(target)
		if err != nil {
			return err
		}
		p, err := o.manager.Profile(name)
		if err != nil {
			return err
		}
		store, storeErr := o.secretStore()
		type row struct {
			Name   string   `json:"name"`
			Git    bool     `json:"git"`
			Hosts  []string `json:"hosts"`
			Token  string   `json:"token"`
			Author string   `json:"author,omitempty"`
		}
		rows := []row{}
		names := []string{}
		for n := range p.Services {
			names = append(names, n)
		}
		slices.Sort(names)
		for _, n := range names {
			s := p.Services[n]
			token := "command"
			if ref, ok := unibox.SecretRef(s.Command); ok && p.SecretCommand != "" {
				token = "your command"
			} else if ok {
				token = "missing"
				if storeErr == nil {
					if ok, _ := store.Has(cmd.Context(), ref); ok {
						token = "stored"
					}
				}
			}
			rows = append(rows, row{n, s.Git, s.Host, token, s.Author})
		}
		if o.json {
			return o.write(map[string]any{"api_version": api.Version, "profile": p.Name, "services": rows})
		}
		w := tabwriter.NewWriter(o.output, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "SERVICE\tUSE\tHOSTS\tTOKEN\tAUTHOR")
		for _, r := range rows {
			use := "env"
			if r.Git {
				use = "git"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.Name, use, strings.Join(r.Hosts, ","), r.Token, r.Author)
		}
		return w.Flush()
	}}
	profileFlag(ls, &target)
	c.AddCommand(ls)

	var keep bool
	rm := &cobra.Command{Use: "rm name", Args: cobra.ExactArgs(1), Short: "Take a service out of the profile, and the token stored for it unless another profile names it", RunE: func(cmd *cobra.Command, args []string) error {
		name, err := o.profileName(target)
		if err != nil {
			return err
		}
		p, err := o.manager.Profile(name)
		if err != nil {
			return err
		}
		args = []string{p.Name, args[0]}
		s, ok := p.Services[args[1]]
		if !ok {
			return fmt.Errorf("profile %s has no service %s; unibox service ls -p %s lists them", p.Name, args[1], p.Name)
		}
		if err = o.manager.EditProfile(p.Name, func(doc *yaml.Node) error {
			unibox.RemoveService(doc, args[1])
			// A variable carrying the service's placeholder goes with it.
			for _, v := range unibox.RemoveEnvRefersTo(doc, "${services."+args[1]+"}") {
				fmt.Fprintf(o.log, "took %s out of env\n", v)
			}
			return nil
		}); err != nil {
			return err
		}
		ref, isRef := unibox.SecretRef(s.Command)
		if isRef && p.SecretCommand != "" {
			// The token is in the person's own store, not unibox's to remove.
		} else if isRef && ref != args[1] {
			// A secret of another name was stored on its own and named
			// with --secret; it is the user's to remove.
			fmt.Fprintf(o.log, "kept secret %s: unibox secret rm %s removes it\n", ref, ref)
		} else if isRef && !keep {
			names, _ := unibox.ProfileNames(o.manager.Config.Environments)
			used := false
			for _, other := range names {
				if q, err := o.manager.Profile(other); err == nil && slices.Contains(q.SecretRefs(), ref) {
					used = true
				}
			}
			if used {
				fmt.Fprintf(o.log, "kept secret %s: another profile names it\n", ref)
			} else if store, err := o.secretStore(); err == nil {
				if err = store.Remove(cmd.Context(), ref); err != nil {
					return err
				}
				fmt.Fprintf(o.log, "removed secret %s\n", ref)
			}
		}
		return o.success(fmt.Sprintf("took service %s out of profile %s", args[1], p.Name))
	}}
	rm.Flags().BoolVar(&keep, "keep-secret", false, "Keep the stored token")
	profileFlag(rm, &target)
	c.AddCommand(rm)
	return c
}

// networkCommand changes a profile's network rules and applies them to its
// sandboxes at once - live where the runtime can, and by a recreation,
// asked for first, where it cannot.
func (o *options) networkCommand() *cobra.Command {
	c := &cobra.Command{Use: "network", Short: "Allow or deny hosts and publish ports for a profile's sandboxes, applied to the running ones"}
	var target string
	change := func(use, short string, edit func(allow, deny []string, hosts []string) ([]string, []string)) *cobra.Command {
		cmd := &cobra.Command{Use: use + " host...", Args: cobra.MinimumNArgs(1), Short: short, RunE: func(cmd *cobra.Command, hosts []string) error {
			name, err := o.profileName(target)
			if err != nil {
				return err
			}
			p, err := o.manager.Profile(name)
			if err != nil {
				return err
			}
			allow, deny := edit(slices.Clone(p.AllowNetwork), slices.Clone(p.DenyNetwork), hosts)
			if err = o.manager.EditProfile(p.Name, func(doc *yaml.Node) error {
				unibox.SetNetworkList(doc, "allow", allow)
				unibox.SetNetworkList(doc, "deny", deny)
				return nil
			}); err != nil {
				return err
			}
			recreate, err := o.manager.ApplyNetwork(cmd.Context(), p.Name)
			if err != nil {
				return err
			}
			if err = o.recreateFor(cmd, recreate, "network rules"); err != nil {
				return err
			}
			return o.success(fmt.Sprintf("profile %s allows %s and denies %s", p.Name, orNone(allow), orNone(deny)))
		}}
		profileFlag(cmd, &target)
		return cmd
	}
	without := func(list, hosts []string) []string {
		return slices.DeleteFunc(list, func(h string) bool { return slices.Contains(hosts, h) })
	}
	with := func(list, hosts []string) []string {
		for _, h := range hosts {
			if !slices.Contains(list, h) {
				list = append(list, h)
			}
		}
		return list
	}
	c.AddCommand(change("allow", "Let the sandboxes reach hosts the runtime's policy refuses", func(allow, deny, hosts []string) ([]string, []string) {
		return with(allow, hosts), without(deny, hosts)
	}))
	c.AddCommand(change("deny", "Refuse hosts the runtime's policy allows", func(allow, deny, hosts []string) ([]string, []string) {
		return without(allow, hosts), with(deny, hosts)
	}))
	c.AddCommand(change("rm", "Take hosts out of the profile's rules, back to the runtime's policy", func(allow, deny, hosts []string) ([]string, []string) {
		return without(allow, hosts), without(deny, hosts)
	}))
	ls := &cobra.Command{Use: "ls", Args: cobra.NoArgs, Short: "List a profile's network rules, with its services' hosts", RunE: func(cmd *cobra.Command, args []string) error {
		name, err := o.profileName(target)
		if err != nil {
			return err
		}
		p, err := o.manager.Profile(name)
		if err != nil {
			return err
		}
		services := map[string]string{}
		for name, s := range p.Services {
			for _, h := range s.ProxyHosts() {
				services[h] = name
			}
		}
		if o.json {
			return o.write(map[string]any{"api_version": api.Version, "profile": p.Name, "allow": p.AllowNetwork, "deny": p.DenyNetwork, "services": services})
		}
		w := tabwriter.NewWriter(o.output, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "RULE\tHOST\tFROM")
		for _, h := range p.AllowNetwork {
			fmt.Fprintf(w, "allow\t%s\tprofile\n", h)
		}
		hosts := []string{}
		for h := range services {
			hosts = append(hosts, h)
		}
		slices.Sort(hosts)
		for _, h := range hosts {
			if !slices.Contains(p.AllowNetwork, h) {
				fmt.Fprintf(w, "allow\t%s\tservice %s\n", h, services[h])
			}
		}
		for _, h := range p.DenyNetwork {
			fmt.Fprintf(w, "deny\t%s\tprofile\n", h)
		}
		return w.Flush()
	}}
	profileFlag(ls, &target)
	c.AddCommand(ls, o.portCommand())
	return c
}

func orNone(list []string) string {
	if len(list) == 0 {
		return "nothing more"
	}
	return strings.Join(list, ", ")
}

// coAuthorCommand says how a profile credits each agent in a commit's
// trailer, and changes it.
func (o *options) coAuthorCommand() *cobra.Command {
	var reset bool
	var as, profile string
	c := &cobra.Command{Use: "co-author [agent]", Args: cobra.MaximumNArgs(1), Short: "Show or set (--as) how a profile credits each agent in its commits", RunE: func(cmd *cobra.Command, args []string) error {
		name, err := o.profileName(profile)
		if err != nil {
			return err
		}
		p, err := o.manager.Profile(name)
		if err != nil {
			return err
		}
		if as == "" && !reset {
			agents := p.Agents
			if len(args) == 1 {
				agents = []string{args[0]}
			}
			rows := map[string]string{}
			for _, id := range agents {
				if !slices.Contains(p.Agents, id) {
					return fmt.Errorf("profile %s does not offer agent %s", p.Name, id)
				}
				rows[id] = p.CoAuthor(id)
			}
			if o.json {
				return o.write(map[string]any{"api_version": api.Version, "profile": p.Name, "co_authors": rows})
			}
			for _, id := range agents {
				from := "default"
				if _, ok := p.Git.CoAuthors[id]; ok {
					from = "profile"
				}
				fmt.Fprintf(o.output, "%s\t%s\t(%s)\n", id, rows[id], from)
			}
			return nil
		}
		if len(args) == 0 {
			return fmt.Errorf("name the agent to credit")
		}
		agent, value := args[0], ""
		if !slices.Contains(p.Agents, agent) {
			return fmt.Errorf("profile %s does not offer agent %s", p.Name, agent)
		}
		if !reset {
			if !unibox.ValidCoAuthor(as) {
				return fmt.Errorf("a co-author is \"Name <email>\", or \"Name <>\" for none")
			}
			value = as
		}
		if err := o.manager.EditProfile(p.Name, func(doc *yaml.Node) error {
			unibox.SetCoAuthor(doc, agent, value)
			return nil
		}); err != nil {
			return err
		}
		if p, err = o.manager.Profile(p.Name); err != nil {
			return err
		}
		return o.success(fmt.Sprintf("profile %s credits %s as %s from its next session", p.Name, agent, p.CoAuthor(agent)))
	}}
	c.Flags().BoolVar(&reset, "reset", false, "Credit the agent as unibox does by default")
	c.Flags().StringVar(&as, "as", "", "How to credit the agent, as \"Name <email>\" (\"Name <>\" for no address)")
	c.MarkFlagsMutuallyExclusive("as", "reset")
	profileFlag(c, &profile)
	return c
}

// agentCommand groups what is said of a profile's agents: how each signs
// in, how each is credited in commits.
func (o *options) agentCommand() *cobra.Command {
	c := &cobra.Command{Use: "agent", Short: "Sign a profile's agents in and say how they are credited"}
	c.AddCommand(o.loginCommand(), o.coAuthorCommand())
	var profile string
	ls := &cobra.Command{Use: "ls", Args: cobra.NoArgs, Short: "List a profile's agents, how each signs in and is credited", RunE: func(cmd *cobra.Command, args []string) error {
		name, err := o.profileName(profile)
		if err != nil {
			return err
		}
		p, err := o.manager.Profile(name)
		if err != nil {
			return err
		}
		type row struct {
			Agent    string   `json:"agent"`
			Auth     string   `json:"auth"`
			CoAuthor string   `json:"co_author"`
			Args     []string `json:"args"`
		}
		rows := []row{}
		for _, id := range p.Agents {
			auth := "-"
			if a, ok := p.Auth[id]; ok {
				auth = a.Method
				if a.Command != "" {
					auth += " " + a.Command
				}
			}
			a, _ := unibox.FindAgent(id)
			args, _ := p.AgentCommand(a)
			rows = append(rows, row{id, auth, p.CoAuthor(id), args[1:]})
		}
		if o.json {
			return o.write(map[string]any{"api_version": api.Version, "profile": p.Name, "agents": rows})
		}
		w := tabwriter.NewWriter(o.output, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "AGENT\tAUTH\tCO-AUTHOR\tARGS")
		for _, r := range rows {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Agent, r.Auth, r.CoAuthor, cmp.Or(strings.Join(r.Args, " "), "-"))
		}
		return w.Flush()
	}}
	profileFlag(ls, &profile)
	c.AddCommand(ls)
	return c
}

// portCommand keeps a profile's ports - set, unset - and switches a
// running sandbox's - publish, unpublish - without touching the profile.
func (o *options) portCommand() *cobra.Command {
	c := &cobra.Command{Use: "port", Short: "A profile's ports, and those a running sandbox publishes", Long: `A port is published on the host's loopback. set and unset change the
profile, for every sandbox of it, and apply at once to the running ones;
publish and unpublish switch a running sandbox's ports alone, and its next
start goes back to the profile's.

A mapping is [[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL]: 3000 takes any
free host port, 8080:3000 host port 8080.`, Example: "  unibox network port set actuator --map 9080:9080 --off -p dev\n  unibox network port publish actuator -s dev\n  unibox network port publish --map 9090:9090 -s dev"}
	var profile, sandbox, mapping string
	var off bool
	set := &cobra.Command{Use: "set name --map mapping", Args: cobra.ExactArgs(1), Short: "Publish a sandbox port by name, at every start unless --off", RunE: func(cmd *cobra.Command, args []string) error {
		if err := unibox.ValidName(args[0]); err != nil {
			return err
		}
		if _, err := unibox.ParsePortSpec(mapping); err != nil {
			return err
		}
		name, err := o.profileName(profile)
		if err != nil {
			return err
		}
		p, err := o.manager.Profile(name)
		if err != nil {
			return err
		}
		if err = o.manager.EditProfile(p.Name, func(doc *yaml.Node) error {
			unibox.SetPort(doc, args[0], unibox.Port{Spec: mapping, Default: !off})
			return nil
		}); err != nil {
			return err
		}
		if err = o.applyPorts(cmd, p.Name); err != nil {
			return err
		}
		when := "at every start"
		if off {
			when = "when published (unibox network port publish " + args[0] + ")"
		}
		return o.success(fmt.Sprintf("profile %s publishes port %s (%s) %s", p.Name, args[0], mapping, when))
	}}
	set.Flags().StringVar(&mapping, "map", "", "The mapping, [[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL]")
	set.Flags().BoolVar(&off, "off", false, "Publish it only when asked to, not at start")
	_ = set.MarkFlagRequired("map")
	profileFlag(set, &profile)
	unset := &cobra.Command{Use: "unset name...", Args: cobra.MinimumNArgs(1), Short: "Take ports out of the profile", RunE: func(cmd *cobra.Command, args []string) error {
		name, err := o.profileName(profile)
		if err != nil {
			return err
		}
		p, err := o.manager.Profile(name)
		if err != nil {
			return err
		}
		for _, port := range args {
			if _, ok := p.Ports[port]; !ok {
				return fmt.Errorf("profile %s has no port %s; unibox network port ls -p %s lists them", p.Name, port, p.Name)
			}
		}
		if err = o.manager.EditProfile(p.Name, func(doc *yaml.Node) error {
			for _, port := range args {
				unibox.RemovePort(doc, port)
			}
			return nil
		}); err != nil {
			return err
		}
		if err = o.applyPorts(cmd, p.Name); err != nil {
			return err
		}
		return o.success(fmt.Sprintf("took port %s out of profile %s", strings.Join(args, ", "), p.Name))
	}}
	profileFlag(unset, &profile)
	var maps []string
	publish := func(on bool) *cobra.Command {
		use, short := "publish", "Publish ports of a running sandbox: the profile's by name, others by --map"
		if !on {
			use, short = "unpublish", "Stop publishing ports of a running sandbox, by name or --map"
		}
		cmd := &cobra.Command{Use: use + " [name...]", Args: cobra.ArbitraryArgs, Short: short, RunE: func(cmd *cobra.Command, args []string) error {
			if len(args)+len(maps) == 0 {
				return fmt.Errorf("name a port of the profile, or give a mapping with --map")
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
		cmd.Flags().StringArrayVar(&maps, "map", nil, "A mapping the profile does not name, [[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL] (repeatable)")
		sandboxFlag(cmd, &sandbox)
		return cmd
	}
	ls := &cobra.Command{Use: "ls", Args: cobra.NoArgs, Short: "List a sandbox's ports and their URLs, or with -p a profile's", RunE: func(cmd *cobra.Command, args []string) error {
		if profile == "" {
			if name, err := o.sandboxName(sandbox); err == nil {
				return o.listPorts(cmd, name)
			} else if sandbox != "" {
				return err
			}
		}
		if sandbox != "" {
			return fmt.Errorf("pass -p or -s, not both")
		}
		name, err := o.profileName(profile)
		if err != nil {
			return err
		}
		p, err := o.manager.Profile(name)
		if err != nil {
			return err
		}
		if o.json {
			return o.write(map[string]any{"api_version": api.Version, "profile": p.Name, "ports": p.Ports})
		}
		if len(p.Ports) == 0 {
			fmt.Fprintf(o.output, "profile %s publishes no ports; unibox network port set <name> --map <mapping> adds one\n", p.Name)
			return nil
		}
		w := tabwriter.NewWriter(o.output, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "PORT\tMAPPING\tAT START")
		names := []string{}
		for n := range p.Ports {
			names = append(names, n)
		}
		slices.Sort(names)
		for _, n := range names {
			at := "on"
			if !p.Ports[n].Default {
				at = "off"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", n, p.Ports[n].Spec, at)
		}
		return w.Flush()
	}}
	ls.Flags().StringVarP(&profile, "profile", "p", "", "The profile's ports rather than a sandbox's")
	ls.Flags().StringVarP(&sandbox, "sandbox", "s", "", "Sandbox (default: UNIBOX_SANDBOX, the context's, or the only one)")
	c.AddCommand(set, unset, ls, publish(true), publish(false))
	return c
}

// listPorts shows a sandbox's ports, what each does at start and now.
func (o *options) listPorts(cmd *cobra.Command, name string) error {
	ports, running, err := o.manager.SandboxPorts(cmd.Context(), name)
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
			out = append(out, api.Port{Name: p.Name, Port: p.Spec, Default: p.Default, Named: p.Named, On: p.On, Published: published})
		}
		return o.write(map[string]any{"api_version": api.Version, "sandbox": name, "running": running, "ports": out})
	}
	if len(ports) == 0 {
		fmt.Fprintf(o.output, "sandbox %s publishes no ports; unibox network port set <name> --map <mapping> adds one to its profile\n", name)
		return nil
	}
	w := tabwriter.NewWriter(o.output, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "PORT\tMAPPING\tAT START\tNOW\tURL")
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
		urls := []string{}
		for _, q := range p.Published {
			if strings.HasPrefix(q.Protocol, "tcp") {
				host := q.HostIP
				if strings.Contains(host, ":") {
					host = "[" + host + "]"
				}
				urls = append(urls, fmt.Sprintf("http://%s:%d", host, q.HostPort))
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", p.Name, p.Spec, at, now, strings.Join(urls, " "))
	}
	return w.Flush()
}
