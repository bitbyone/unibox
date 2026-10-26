package cli

import (
	"cmp"
	"fmt"
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
func (o *options) secretUI(cmd *cobra.Command, envName string, leave func() error) error {
	ctx := cmd.Context()
	env, err := o.manager.Config.LoadEnv(envName)
	if err != nil {
		return err
	}
	if err = o.manager.SBX.Available(ctx); err != nil {
		return err
	}
	build := func() (listModel, error) {
		list, err := o.manager.NeededSecrets(ctx, env, "")
		if err != nil {
			return listModel{}, err
		}
		m := listModel{title: "tokens env " + env.Name + " needs", header: []string{"SERVICE", "STORED", "NEEDED", "FOR"}, actions: []listAction{
			{key: "enter", name: "set", help: "store"},
			{key: "c", name: "command", help: "store a command for it"},
		}}
		for _, l := range list {
			stored, needed := "no", "yes"
			if l.Stored {
				stored = l.Scope
			}
			if l.Optional {
				needed = "optional"
			}
			m.rows = append(m.rows, []string{l.Service, stored, needed, cmp.Or(l.Description, strings.Join(l.Kits, ", "), "the env's bindings")})
			m.ids = append(m.ids, l.Service)
		}
		return m, nil
	}
	act := func(action, service string) (string, error) {
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
		if err := o.runStep(cmd, "secret", "set", service, "-e", env.Name); err != nil {
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
		m := listModel{title: "envs", header: []string{"ENV", "SANDBOX", "FILES"}, actions: []listAction{
			{key: "enter", name: "show", help: "show"},
			{key: " ", name: "default", help: "make default"},
			{key: "u", name: "update", help: "update"},
			{key: "c", name: "create", help: "create its sandbox"},
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
			m.rows = append(m.rows, []string{mark, sandbox, strconv.Itoa(len(cfg.Envs[name].Files))})
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
			return errListDone, o.runStep(cmd, "sandbox", "create", "-e", name)
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
		m := listModel{title: "context: what commands work in when not told", header: []string{"", "KIND", "NAME"}, ownLeave: true, actions: []listAction{
			{key: "enter", name: "use", help: "use"},
			{key: "c", name: "clear", help: "clear", any: true},
		}}
		add := func(kind, name, current string) {
			mark := " "
			if name == current {
				mark, name = portOn.Render("●"), portOn.Render(name)
			}
			m.rows = append(m.rows, []string{mark, kind, name})
		}
		for _, name := range sorted(state.Sandboxes) {
			add("sandbox", name, o.defaultSandbox())
			m.ids = append(m.ids, "-s="+name)
		}
		for _, name := range sorted(o.manager.Config.Envs) {
			add("env", name, o.defaultEnv())
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
		kind := map[string]string{"-s": "sandbox", "-e": "env"}[flag]
		_, err := o.quietStep(cmd, "context", "use", flag, name)
		return fmt.Sprintf("commands work in %s %s when not told", kind, name), err
	}
	return o.actList(cmd, build, act, leave)
}

// varUI is unibox var at a terminal: the args an env asks for, or the
// values for every env with --global, to set and remove.
func (o *options) varUI(cmd *cobra.Command, envName string, global bool, leave func() error) error {
	scope := []string{"-e", envName}
	title := "args env " + envName + " asks for"
	if global {
		scope, title = []string{"-g"}, "your values for every env"
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
		if global {
			m.header = []string{"NAME", "VALUE"}
			for _, name := range sorted(v.Args) {
				m.rows = append(m.rows, []string{name, v.Args[name]})
				m.ids = append(m.ids, name)
				values[name] = v.Args[name]
			}
			return m, nil
		}
		e, err := o.manager.Config.LoadEnv(envName)
		if err != nil {
			return listModel{}, err
		}
		m.header = []string{"NAME", "VALUE", "FROM", "FOR"}
		mine := v.Envs[e.Name]
		for _, name := range sorted(e.SBX.Args) {
			a := e.SBX.Args[name]
			from, value := portErr.Render("missing"), ""
			if s, ok := mine[name]; ok {
				from, value = portOn.Render("yours"), s
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
			value, err := o.askText("Value of "+name, "Enter keeps what is there", values[name], func(s string) error { return unibox.ValidArg(name, s) })
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
