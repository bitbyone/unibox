package cli

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tobola/unibox/internal/unibox"
)

// reloadConfig reads the configuration a list's action may have written,
// so the list is drawn from what is now set.
func (o *options) reloadConfig() error {
	cfg, err := o.paths.LoadConfig()
	if err == nil {
		o.manager.Config = cfg
	}
	return err
}

// secretUI is unibox secret at a terminal: the tokens an env needs, each to
// have sbx store. sbx asks for a token on the terminal itself, so storing
// one leaves the list for it.
func (o *options) secretUI(cmd *cobra.Command, envName, sandbox string, leave func() error) error {
	ctx := cmd.Context()
	env, err := o.manager.Config.LoadEnv(envName)
	if err != nil {
		return err
	}
	if err = o.manager.SBX.Available(ctx); err != nil {
		return err
	}
	// The scopes are those of the sandbox the list is for: the one named,
	// else the env's.
	sandbox = cmp.Or(sandbox, env.SandboxName())
	target := []string{"-e", env.Name, "-s", sandbox}
	build := func() (listModel, error) {
		if err := o.reloadConfig(); err != nil {
			return listModel{}, err
		}
		list, err := o.manager.NeededSecrets(ctx, env, sandbox)
		if err != nil {
			return listModel{}, err
		}
		m := listModel{title: "tokens sandbox " + sandbox + " of env " + env.Name + " needs", header: []string{"SERVICE", "STORED", "NEEDED", "FOR"}, actions: []listAction{
			{key: "enter", name: "set", help: "store"},
			{key: "g", name: "generate", help: "generate a new token"},
			{key: "c", name: "command", help: "store a command for it"},
			{key: "s", name: "scope", help: "global, env or sandbox"},
		}}
		for _, l := range list {
			stored, needed := storedText(l), "yes"
			if l.Optional {
				needed = "optional"
			}
			m.rows = append(m.rows, []string{l.Service, stored, needed, cmp.Or(l.Description, strings.Join(l.Kits, ", "), "the env's bindings")})
			m.ids = append(m.ids, l.Service)
		}
		return m, nil
	}
	act := func(action, service string) (string, error) {
		if action == "scope" {
			return o.moveSecret(cmd, env, sandbox, service, target)
		}
		if action == "command" {
			command, err := o.askText("The command sbx runs on the host for the "+service+" token", "A keychain read, say; it prints the token", "", func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("say the command")
				}
				return nil
			})
			if err != nil {
				return "", err
			}
			return o.quietStep(cmd, "secret", "set", service, "--command", command, "-e", env.Name)
		}
		// sbx asks for the value on this terminal; it goes from the
		// keyboard to sbx.
		args := []string{"secret", "set", service, "-e", env.Name}
		if action == "generate" {
			args = append(args, "--generate")
		}
		if err := o.runStep(cmd, args...); err != nil {
			return "", err
		}
		return "", nil
	}
	return o.actList(cmd, build, act, leave)
}

// envUI is unibox env at a terminal: the envs, one of them the default, to
// show, update, create a sandbox from or forget.
func (o *options) envUI(cmd *cobra.Command, leave func() error) error {
	build := func() (listModel, error) {
		if err := o.reloadConfig(); err != nil {
			return listModel{}, err
		}
		cfg := o.manager.Config
		m := listModel{title: "envs", header: []string{"ENV", "SANDBOX", "FILES", "REPO"}, actions: []listAction{
			{key: "enter", name: "show", help: "show"},
			{key: " ", name: "default", help: "make default"},
			{key: "u", name: "update", help: "update"},
			{key: "c", name: "create", help: "create a sandbox"},
			{key: "d", name: "rm", help: "forget", confirm: "forget env %s? its files and sandbox stay"},
		}}
		for _, name := range sorted(cfg.Envs) {
			mark, sandbox := name, "?"
			if name == cfg.DefaultEnv {
				mark = portOn.Render(name + " (default)")
			}
			if e, err := cfg.LoadEnv(name); err == nil {
				sandbox = e.SandboxName()
			}
			m.rows = append(m.rows, []string{mark, sandbox, strconv.Itoa(len(cfg.Envs[name].Files)), envRepoText(cfg.Envs[name])})
			m.ids = append(m.ids, name)
		}
		return m, nil
	}
	act := func(action, name string) (string, error) {
		switch action {
		case "show":
			return errListDone, o.runStep(cmd, "env", "show", name)
		case "default":
			return o.quietStep(cmd, "config", "set", "default_env", name)
		case "update":
			// A pull may ask on the terminal.
			return "", o.runStep(cmd, "env", "update", name)
		case "create":
			args, err := o.askCreate(name)
			if err != nil {
				return "", err
			}
			return errListDone, o.runStep(cmd, args...)
		case "rm":
			return o.quietStep(cmd, "env", "rm", name)
		}
		return "", nil
	}
	return o.actList(cmd, build, act, leave)
}

// contextUI is unibox context at a terminal: the sandboxes and envs, the
// context's marked, one to pick for it.
func (o *options) contextUI(cmd *cobra.Command, leave func() error) error {
	build := func() (listModel, error) {
		if err := o.reloadConfig(); err != nil {
			return listModel{}, err
		}
		state, err := o.paths.ReadState()
		if err != nil {
			return listModel{}, err
		}
		m := listModel{title: "context: the sandbox commands work in when not told", header: []string{"", "SANDBOX", "ENV"}, ownLeave: true, actions: []listAction{
			{key: "enter", name: "use", help: "use"},
			{key: "c", name: "clear", help: "clear", any: true},
		}}
		// One row a sandbox, with its env, and one an env whose sandbox is
		// not made yet: picking one picks both.
		current, currentEnv := o.defaultSandbox(), o.manager.Config.Context.Env
		mark := func(on bool, cells ...string) []string {
			if !on {
				return append([]string{" "}, cells...)
			}
			for i := range cells {
				cells[i] = portOn.Render(cells[i])
			}
			return append([]string{portOn.Render("●")}, cells...)
		}
		made := map[string]bool{}
		for _, name := range sorted(state.Sandboxes) {
			env := state.Sandboxes[name].Env
			made[env] = true
			m.rows = append(m.rows, mark(name == current, name, cmp.Or(env, "-")))
			m.ids = append(m.ids, "-s="+name)
		}
		for _, name := range sorted(o.manager.Config.Envs) {
			if made[name] {
				continue
			}
			m.rows = append(m.rows, mark(current == "" && name == currentEnv, portHelp.Render("not made yet"), name))
			m.ids = append(m.ids, "-e="+name)
		}
		return m, nil
	}
	act := func(action, id string) (string, error) {
		if action == "clear" {
			_, err := o.quietStep(cmd, "context", "clear")
			return "context cleared", err
		}
		flag, name, _ := strings.Cut(id, "=")
		if _, err := o.quietStep(cmd, "context", "use", flag, name); err != nil {
			return "", err
		}
		if err := o.reloadConfig(); err != nil {
			return "", err
		}
		return o.contextText(), nil
	}
	return o.actList(cmd, build, act, leave)
}

// varUI is unibox var at a terminal: the args an env asks for, or the
// values for every env with --global or for every env of its repository
// with --env-repo, to set and remove.
func (o *options) varUI(cmd *cobra.Command, envName string, global, envRepo bool, leave func() error) error {
	scope := []string{"-e", envName}
	title := "args env " + envName + " asks for"
	repoKey := ""
	switch {
	case global:
		scope, title = []string{"-g"}, "your values for every env"
	case envRepo:
		var err error
		if repoKey, err = o.manager.Config.EnvRepoOf(envName); err != nil {
			return err
		}
		scope, title = []string{"-e", envName, "--env-repo"}, "your values for every env of "+envRepoText(o.manager.Config.Envs[envName])
	}
	values := map[string]string{}
	build := func() (listModel, error) {
		v, err := o.paths.LoadEnvValues()
		if err != nil {
			return listModel{}, err
		}
		m := listModel{title: title, actions: []listAction{
			{key: "enter", name: "set", help: "set"},
			{key: "a", name: "add", help: "add", any: true},
			{key: "d", name: "rm", help: "remove", confirm: "remove your value of %s?"},
		}}
		clear(values)
		if global || envRepo {
			mine := v.Args
			if envRepo {
				mine = v.EnvRepos[repoKey]
			}
			m.header = []string{"NAME", "VALUE"}
			for _, name := range sorted(mine) {
				m.rows = append(m.rows, []string{name, mine[name]})
				m.ids = append(m.ids, name)
				values[name] = mine[name]
			}
			return m, nil
		}
		e, err := o.manager.Config.LoadEnv(envName)
		if err != nil {
			return listModel{}, err
		}
		m.header = []string{"NAME", "VALUE", "FROM", "FOR"}
		mine, ofRepo := v.Envs[e.Name], envRepoValues(v, e)
		for _, name := range sorted(e.SBX.Args) {
			a := e.SBX.Args[name]
			from, value := portErr.Render("missing"), ""
			if s, ok := mine[name]; ok {
				from, value = portOn.Render("yours"), s
			} else if s, ok := ofRepo[name]; ok {
				from, value = "env-repo", s
			} else if s, ok := v.Args[name]; ok {
				from, value = "global", s
			} else if a.Default != nil {
				from, value = "default", *a.Default
			} else if !a.Required {
				from = "optional"
			}
			m.rows = append(m.rows, []string{name, value, from, a.Description})
			m.ids = append(m.ids, name)
			values[name] = value
		}
		for _, name := range sorted(mine) {
			if _, ok := e.SBX.Args[name]; !ok {
				m.rows = append(m.rows, []string{name, mine[name], "undeclared", ""})
				m.ids = append(m.ids, name)
				values[name] = mine[name]
			}
		}
		return m, nil
	}
	act := func(action, name string) (string, error) {
		switch action {
		case "add":
			var err error
			name, err = o.askText("Name", "The arg the value is for", "", func(s string) error { return unibox.ValidArg(s, "") })
			if err != nil {
				return "", err
			}
			fallthrough
		case "set":
			value, err := o.askText("Value of "+name, "Enter keeps what is there", values[name], func(s string) error { return checkArg(name, s) })
			if err != nil {
				return "", err
			}
			return o.quietStep(cmd, append([]string{"var", "set", name, value}, scope...)...)
		case "rm":
			return o.quietStep(cmd, append([]string{"var", "rm", name}, scope...)...)
		}
		return "", nil
	}
	return o.actList(cmd, build, act, leave)
}

// askCreate asks what a new sandbox of an env is called and what of the
// machine it gets beyond what the env's sandboxes get, as the arguments of
// unibox sandbox create.
func (o *options) askCreate(envName string) ([]string, error) {
	e, err := o.manager.Config.LoadEnv(envName)
	if err != nil {
		return nil, err
	}
	values, err := o.manager.Paths.LoadEnvValues()
	if err != nil {
		return nil, err
	}
	taken := map[string]bool{}
	if state, err := o.manager.Paths.ReadState(); err == nil {
		for name := range state.Sandboxes {
			taken[name] = true
		}
	}
	preset := e.SandboxName()
	for i := 2; taken[preset]; i++ {
		preset = fmt.Sprintf("%s-%d", e.SandboxName(), i)
	}
	name, err := o.askText("Sandbox name", "", preset, func(s string) error {
		if taken[s] {
			return fmt.Errorf("sandbox %s exists already", s)
		}
		return unibox.ValidName(s)
	})
	if err != nil {
		return nil, err
	}
	args := []string{"sandbox", "create", name, "-e", envName}
	env := values.ResourcesFor(e, "")
	for _, r := range unibox.ResourceNames {
		v, err := o.askText(r, "empty keeps the env's: "+cmp.Or(env.Get(r), "as the env's files say"), "", func(s string) error {
			if s == "" {
				return nil
			}
			return unibox.ValidResource(r, s)
		})
		if err != nil {
			return nil, err
		}
		if v != "" {
			args = append(args, "--"+r, v)
		}
	}
	return args, nil
}

// moveSecret moves a token between the scopes a sandbox takes it from:
// the global one sbx keeps, its env's own, or its own. One moved to a
// scope of its own is typed into the keychain or read by a command; one
// moved back to the global is asked for when sbx has none.
func (o *options) moveSecret(cmd *cobra.Command, env unibox.Env, sandbox, service string, target []string) (string, error) {
	cfg := o.manager.Config
	current := "global"
	if _, ok := cfg.OwnSecret(unibox.SecretScope{Sandbox: sandbox}, service); ok {
		current = "sandbox"
	} else if _, ok := cfg.OwnSecret(unibox.SecretScope{Env: env.Name}, service); ok {
		current = "env"
	}
	picks := []pick{}
	for _, p := range []pick{
		{"global", "global: the one sbx keeps for every sandbox"},
		{"env", "env " + env.Name + ": its sandboxes' own"},
		{"sandbox", "sandbox " + sandbox + ": its own, over its env's"},
	} {
		if p.value != current {
			picks = append(picks, p)
		}
	}
	to, err := o.askPick("Where sandbox "+sandbox+" takes the "+service+" token from", "Now: "+current, picks, picks[0].value)
	if err != nil {
		return "", err
	}
	scopeArgs := func(scope string) []string { return append([]string{"--scope", scope}, target...) }
	if to == "global" {
		if _, err := o.quietStep(cmd, append([]string{"secret", "rm", service}, scopeArgs(current)...)...); err != nil {
			return "", err
		}
		secrets, err := o.manager.SBX.Secrets(cmd.Context())
		if err != nil {
			return "", err
		}
		if slices.ContainsFunc(secrets, func(s unibox.Secret) bool { return s.Scope == "global" && s.Name == service }) {
			return "sandbox " + sandbox + " takes the global " + service + " token", nil
		}
		// None to fall back on: sbx asks for it now.
		if err := o.runStep(cmd, append([]string{"secret", "set", service}, target...)...); err != nil {
			return "", err
		}
		return "", nil
	}
	if err := o.setInScope(cmd, env, sandbox, service, to); err != nil {
		return "", err
	}
	if current != "global" {
		// Moved, not copied: the scope it came from takes the one over it.
		if _, err := o.quietStep(cmd, append([]string{"secret", "rm", service}, scopeArgs(current)...)...); err != nil {
			return "", err
		}
	}
	return "", nil
}

// setInScope keeps a token of an env's or a sandbox's own, typed into the
// keychain or read by a command, as the person picks.
func (o *options) setInScope(cmd *cobra.Command, env unibox.Env, sandbox, service, scope string) error {
	how, err := o.askPick("How the "+service+" token of "+scope+" "+map[string]string{"env": env.Name, "sandbox": sandbox}[scope]+" is kept", "", []pick{
		{"keychain", "typed in; it goes into " + unibox.KeychainName()},
		{"command", "a command on this machine that prints it"},
	}, "keychain")
	if err != nil {
		return err
	}
	args := []string{"secret", "set", service, "--scope", scope, "-e", env.Name, "-s", sandbox}
	if how == "command" {
		command, err := o.askText("The command sbx runs on the host for the "+service+" token", "A keychain read, say; it prints the token", "", func(s string) error {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("say the command")
			}
			return nil
		})
		if err != nil {
			return err
		}
		args = append(args, "--command", command)
	}
	// The keychain's program asks on the terminal, so the step has it.
	return o.runStep(cmd, args...)
}
