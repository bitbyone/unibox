package cli

import (
	"bytes"
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
	c.AddCommand(&cobra.Command{Use: "set name", Args: cobra.ExactArgs(1), Short: "Store a token, asked for on a hidden prompt or read from stdin", RunE: func(cmd *cobra.Command, args []string) error {
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

func (o *options) loginCommand() *cobra.Command {
	var renew, viaGH bool
	var rt string
	c := &cobra.Command{Use: "login profile [agent...]", Args: cobra.MinimumNArgs(1), Short: "Sign a profile's agents in: tokens into the platform's store, the profile's auth written", RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		req := unibox.Request{Profile: args[0], Runtime: rt}
		_, r, p, err := o.manager.Definition(req)
		if err != nil {
			return err
		}
		agents := args[1:]
		if len(agents) == 0 {
			agents = p.Agents
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
			switch id {
			case "codex":
				auths[id] = unibox.AgentAuth{Method: unibox.AuthOAuth}
				fmt.Fprintln(o.log, "  the ChatGPT login is kept by the runtime, which opens the browser for it")
			case "claude":
				if ok, _ := store.Has(ctx, "claude"); !ok || renew {
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
				if p.Auth[id] != a {
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
				fmt.Fprintf(o.log, "stored %s's %s login in %s\n", id, method, r.Name())
			}
		}
		return o.success(fmt.Sprintf("profile %s signs its agents in", p.Name))
	}}
	c.Flags().BoolVar(&renew, "renew", false, "Ask for every token again, replacing the stored ones")
	c.Flags().BoolVar(&viaGH, "gh", false, "Sign copilot in through your gh login (gh auth token) instead of a token of its own")
	c.Flags().StringVar(&rt, "runtime", "", "Runtime that keeps the logins (default: the configured one)")
	return c
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
	"artifactory": {scheme: unibox.SchemeBearer, env: "ARTIFACTORY_TOKEN", explain: []string{
		"an identity token from your Artifactory user profile (Edit Profile > Generate an Identity Token)",
	}},
	"generic": {},
}

func (o *options) serviceCommand() *cobra.Command {
	c := &cobra.Command{Use: "service", Short: "Sign a profile's sandboxes in to forges and other host services"}
	var kind, user, scheme, env, author, refresh, command string
	var hosts []string
	var git, noCoAuthor, renew, viaGH bool
	add := &cobra.Command{Use: "add profile name", Args: cobra.ExactArgs(2), Short: "Store a service's token and write its entry into the profile", RunE: func(cmd *cobra.Command, args []string) error {
		profile, name := args[0], args[1]
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
		s := unibox.Service{Git: k.git || git, Scheme: k.scheme, User: k.user, Env: k.env, Author: author, Refresh: refresh}
		if len(hosts) > 0 {
			s.Host = hosts
		} else if k.host != "" {
			s.Host = unibox.HostList{k.host}
		}
		for flag, set := range map[string]func(string){"user": func(v string) { s.User = v }, "scheme": func(v string) { s.Scheme = v }, "env": func(v string) { s.Env = v; s.Git = false }} {
			if cmd.Flags().Changed(flag) {
				set(map[string]string{"user": user, "scheme": scheme, "env": env}[flag])
			}
		}
		if s.Env != "" && s.Scheme == "" {
			s.Scheme = unibox.SchemeBearer
		}
		if s.Git && s.Scheme == unibox.SchemeBearer || s.Scheme == unibox.SchemeRaw {
			s.User = ""
		}
		if noCoAuthor {
			no := false
			s.CoAuthor = &no
		}
		if len(s.Host) == 0 {
			return fmt.Errorf("give %s its host with --host", name)
		}
		gh := false
		if kind == "github" && command == "" {
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
			return nil
		}); err != nil {
			return err
		}
		where := strings.Join(s.ProxyHosts(), ", ")
		if s.Git {
			where = "git at " + strings.Join(s.GitURLs(), ", ")
		}
		return o.success(fmt.Sprintf("profile %s signs %s in to %s; the next start applies it", profile, name, where))
	}}
	f := add.Flags()
	f.StringVar(&kind, "kind", "", "github, gitlab, artifactory or generic (default: from the name)")
	f.StringSliceVar(&hosts, "host", nil, "Host, or host/path for one owner on a git forge (repeatable)")
	f.BoolVar(&git, "git", false, "Wire the sandbox's git to the hosts (generic services)")
	f.StringVar(&user, "user", "", "User the forge pairs with the token (basic scheme)")
	f.StringVar(&scheme, "scheme", "", "basic, bearer or raw")
	f.StringVar(&env, "env", "", "Variable a tool reads the placeholder from, instead of git")
	f.StringVar(&author, "author", "", "Who commits under the service, as Name <email> (--author on a run wins)")
	f.BoolVar(&noCoAuthor, "no-co-author", false, "No Co-Authored-By trailer for the agent in its repositories")
	f.StringVar(&refresh, "refresh", "", "How often the runtime fetches the token again, e.g. 10m")
	f.StringVar(&command, "command", "", "A host command printing the token, for a store unibox does not keep (Vault, 1Password)")
	f.BoolVar(&renew, "renew", false, "Ask for the token again, replacing the stored one")
	f.BoolVar(&viaGH, "gh", false, "Sign a github service in through your gh login (gh auth token)")
	c.AddCommand(add)

	c.AddCommand(&cobra.Command{Use: "ls profile", Args: cobra.ExactArgs(1), Short: "List a profile's services and whether their tokens are stored", RunE: func(cmd *cobra.Command, args []string) error {
		p, err := o.manager.Profile(args[0])
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
			if ref, ok := unibox.SecretRef(s.Command); ok {
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
	}})

	var keep bool
	rm := &cobra.Command{Use: "rm profile name", Args: cobra.ExactArgs(2), Short: "Take a service out of the profile, and its token unless another profile names it", RunE: func(cmd *cobra.Command, args []string) error {
		p, err := o.manager.Profile(args[0])
		if err != nil {
			return err
		}
		s, ok := p.Services[args[1]]
		if !ok {
			return fmt.Errorf("profile %s has no service %s; unibox service ls %s lists them", p.Name, args[1], p.Name)
		}
		if err = o.manager.EditProfile(p.Name, func(doc *yaml.Node) error {
			unibox.RemoveService(doc, args[1])
			return nil
		}); err != nil {
			return err
		}
		ref, isRef := unibox.SecretRef(s.Command)
		if isRef && !keep {
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
	c.AddCommand(rm)
	return c
}

// networkCommand changes a profile's network rules and applies them to its
// sandboxes at once - live where the runtime can, and by a recreation,
// asked for first, where it cannot.
func (o *options) networkCommand() *cobra.Command {
	c := &cobra.Command{Use: "network", Short: "Allow or deny hosts for a profile's sandboxes, applied to the running ones"}
	change := func(use, short string, edit func(allow, deny []string, hosts []string) ([]string, []string)) *cobra.Command {
		return &cobra.Command{Use: use + " profile host...", Args: cobra.MinimumNArgs(2), Short: short, RunE: func(cmd *cobra.Command, args []string) error {
			profile, hosts := args[0], args[1:]
			p, err := o.manager.Profile(profile)
			if err != nil {
				return err
			}
			allow, deny := edit(slices.Clone(p.AllowNetwork), slices.Clone(p.DenyNetwork), hosts)
			if err = o.manager.EditProfile(p.Name, func(doc *yaml.Node) error {
				unibox.SetStringList(doc, "allow_network", allow)
				unibox.SetStringList(doc, "deny_network", deny)
				return nil
			}); err != nil {
				return err
			}
			recreate, err := o.manager.ApplyNetwork(cmd.Context(), p.Name)
			if err != nil {
				return err
			}
			for _, name := range recreate {
				if err := o.confirm("Runtime of sandbox " + name + " cannot change network rules live; recreate it to apply them?"); err != nil {
					fmt.Fprintf(o.log, "sandbox %s keeps its old rules until it is recreated\n", name)
					continue
				}
				if err := o.manager.Recreate(cmd.Context(), name, false); err != nil {
					return err
				}
			}
			return o.success(fmt.Sprintf("profile %s allows %s and denies %s", p.Name, orNone(allow), orNone(deny)))
		}}
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
	c.AddCommand(&cobra.Command{Use: "ls profile", Args: cobra.ExactArgs(1), Short: "List a profile's network rules, with its services' hosts", RunE: func(cmd *cobra.Command, args []string) error {
		p, err := o.manager.Profile(args[0])
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
	}})
	return c
}

func orNone(list []string) string {
	if len(list) == 0 {
		return "nothing more"
	}
	return strings.Join(list, ", ")
}
