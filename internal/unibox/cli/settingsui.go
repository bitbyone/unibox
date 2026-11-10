package cli

import (
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tobola/unibox/internal/unibox"
)

// setting is one of the person's settings as the settings list shows it:
// what it is about, and whether turning it on is theirs to weigh first.
type setting struct {
	key, about string
	// risk, when set, is asked before the setting is turned on.
	risk string
	// sbx is a setting of sbx's, for every sandbox of the machine.
	sbx bool
}

// globalSettings are the settings of the Global tab, unibox's then sbx's;
// sbx's are its daemon's, so no env has them of its own.
var globalSettings = []setting{
	{key: "stop_idle", about: "stop a sandbox when its last session ends"},
	{key: "update_check", about: "tell of a newer unibox or sbx and of new env commits"},
	{key: "author", about: "who your commits in a sandbox are by"},
	{key: "co_author", about: "agents name themselves co-author of commits"},
	{key: "cpus", about: "CPUs a sandbox gets: a count, 0 for all, or a share"},
	{key: "memory", about: "memory a sandbox gets: a size such as 16g, or a share"},
	{key: "ssh_agent", sbx: true, about: "sandboxes may sign with your SSH agent", risk: sshAgentRisk},
	{key: "ssh_sandbox", sbx: true, about: "editors and apps reach a sandbox as ssh <sandbox>.sbx"},
	{key: "remember_host_commands", sbx: true, about: "an approved env's host commands run again unasked", risk: "sbx then runs an env's host commands again without showing them to you."},
	{key: "claude_remote_control", sbx: true, about: "claude.ai and its app may drive a Claude session"},
	{key: "clipboard_image_paste", sbx: true, about: "agents paste images from your clipboard", risk: "Any process in a sandbox can then read an image you copy."},
	{key: "clipboard_text_copy", sbx: true, about: "text agents copy reaches your clipboard", risk: "Any process in a sandbox can then put what you next paste here."},
}

// envSettings are those an env has of its own, over the Global tab's.
var envSettings = []string{"stop_idle", "author", "co_author", "cpus", "memory"}

func settingOf(key string) setting {
	i := slices.IndexFunc(globalSettings, func(s setting) bool { return s.key == key })
	return globalSettings[i]
}

func onOff(on bool) string {
	if on {
		return portOn.Render("on")
	}
	return portOff.Render("off")
}

// settingsUI is unibox config at a terminal: a tab of the settings for
// every env, and one for each env with what it has of its own over them
// and the args it asks for.
func (o *options) settingsUI(cmd *cobra.Command) error {
	tab := 0
	if env := o.defaultEnv(); env != "" {
		tab = slices.Index(sorted(o.manager.Config.Envs), env) + 1
	}
	values := map[string]string{}
	build := func() (listModel, error) {
		envs := sorted(o.manager.Config.Envs)
		tab = min(tab, len(envs))
		m := listModel{tabs: append([]string{"Global"}, envs...), tab: tab, header: []string{"", "SETTING", "VALUE", "FROM", "ABOUT"}, actions: []listAction{
			{key: "enter", name: "change", help: "change"},
			{key: "d", name: "default", help: "take the wider one"},
			{key: "a", name: "add", help: "add a value", any: true},
		}}
		v, err := o.paths.LoadEnvValues()
		if err != nil {
			return m, err
		}
		clear(values)
		row := func(group, id, name, value, from, about string) {
			m.rows = append(m.rows, []string{portHelp.Render(group), name, value, from, portHelp.Render(about)})
			m.ids = append(m.ids, id)
		}
		if tab == 0 {
			cfg := o.manager.Config
			for _, s := range globalSettings {
				value, from := "", ""
				switch s.key {
				case "stop_idle":
					value, from = onOff(cfg.StopIdle()), saidFrom(cfg.StopIdleSandbox != nil)
				case "update_check":
					value, from = onOff(cfg.UpdateChecks()), saidFrom(cfg.UpdateCheck != nil)
				case "author":
					value, from = v.Author, saidFrom(v.Author != "")
				case "co_author":
					on := v.CoAuthor == nil || *v.CoAuthor
					value, from = onOff(on), saidFrom(v.CoAuthor != nil)
				case "cpus", "memory":
					value, from = v.Resources.Get(s.key), saidFrom(v.Resources.Get(s.key) != "")
				case "ssh_agent":
					on, said := cfg.SSHAgent()
					value, from = onOff(on), saidFrom(said)
				case "ssh_sandbox":
					on, said := cfg.EditorSSH()
					value, from = onOff(on), saidFrom(said)
				default:
					said := *cfg.SBXSetting(s.key)
					if said != nil {
						value, from = onOff(*said), "yours"
					} else if now, err := o.manager.SBX.Setting(cmd.Context(), unibox.SBXSettings[s.key]); err == nil {
						value, from = onOff(now == "true"), "sbx"
					}
				}
				group := "unibox"
				if s.sbx {
					group = "sbx"
				}
				values[s.key] = value
				row(group, "set:"+s.key, s.key, value, from, s.about)
			}
			for _, name := range sorted(v.Args) {
				values["arg:"+name] = v.Args[name]
				row("value", "arg:"+name, name, v.Args[name], "yours", "for every env that asks for it")
			}
			return m, nil
		}
		env, err := o.manager.Config.LoadEnv(envs[tab-1])
		if err != nil {
			return m, err
		}
		for _, key := range envSettings {
			value, from := "", ""
			switch key {
			case "stop_idle":
				on, said := o.manager.Config.StopIdleFor(env.Name)
				value, from = onOff(on), layer(said, "env", "global")
			case "author":
				var at string
				value, at = v.AuthorFor(env, "")
				from = layer(at, unibox.AuthorOfEnv, unibox.AuthorOfAll)
				values[key] = value
			case "co_author":
				on, at := v.CoAuthorFor(env.Name)
				value, from = onOff(on), layer(at, unibox.AuthorOfEnv, unibox.AuthorOfAll)
			case "cpus", "memory":
				value = v.ResourcesFor(env, "").Get(key)
				from = layer(v.ResourceFrom(env, "", key), unibox.ResourceOfEnv, unibox.ResourceOfAll)
				values[key] = v.EnvResources[env.Name].Get(key)
			}
			row("unibox", "set:"+key, key, value, from, settingOf(key).about)
		}
		mine, ofRepo := v.Envs[env.Name], envRepoValues(v, env)
		for _, name := range sorted(env.SBX.Args) {
			a := env.SBX.Args[name]
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
			values["arg:"+name] = value
			row("arg", "arg:"+name, name, value, from, a.Description)
		}
		for _, name := range sorted(mine) {
			if _, ok := env.SBX.Args[name]; !ok {
				values["arg:"+name] = mine[name]
				row("arg", "arg:"+name, name, mine[name], "undeclared", "the env declares no such arg")
			}
		}
		return m, nil
	}
	act := func(action, id string) (string, error) {
		if n, ok := strings.CutPrefix(action, "tab:"); ok {
			tab, _ = strconv.Atoi(n)
			return "", nil
		}
		envName := ""
		if tab > 0 {
			envName = sorted(o.manager.Config.Envs)[tab-1]
		}
		kind, key, _ := strings.Cut(id, ":")
		if action == "add" {
			name, err := o.askText("Name", "The arg the value is for", "", func(s string) error { return unibox.ValidArg(s, "") })
			if err != nil {
				return "", err
			}
			kind, key, action = "arg", name, "change"
		}
		if kind == "arg" {
			scope := []string{"-g"}
			if envName != "" {
				scope = []string{"-e", envName}
			}
			if action == "default" {
				return o.quietStep(cmd, append([]string{"var", "rm", key}, scope...)...)
			}
			value, err := o.askText("Value of "+key, "Enter keeps what is there", values[id], func(s string) error { return checkArg(key, s) })
			if err != nil {
				return "", err
			}
			return o.quietStep(cmd, append([]string{"var", "set", key, value}, scope...)...)
		}
		if action == "default" {
			return o.unsetSetting(cmd, key, envName)
		}
		return o.changeSetting(cmd, key, envName, values[key])
	}
	return o.actList(cmd, build, act, func() error { return nil })
}

// saidFrom is a Global setting's source: the person's, or the default.
func saidFrom(said bool) string {
	if said {
		return "yours"
	}
	return portHelp.Render("default")
}

// layer is where an env's setting comes from, as its tab says it: the
// env's own, every env's (Global), or the default.
func layer(at, own, global string) string {
	switch at {
	case own:
		return portOn.Render("own")
	case global:
		return "Global"
	case "":
		return portHelp.Render("default")
	}
	return at
}

// changeSetting changes a setting as its command would: a switch turns
// over, asking first where turning it on lets a sandbox do more; the rest
// are asked for.
func (o *options) changeSetting(cmd *cobra.Command, key, env, now string) (string, error) {
	scope := []string{}
	if env != "" {
		scope = []string{"-e", env}
	}
	v, err := o.paths.LoadEnvValues()
	if err != nil {
		return "", err
	}
	switch key {
	case "author":
		author, err := o.askText("Author", "Name <email>; enter keeps what is there", now, nil)
		if err != nil {
			return "", err
		}
		if env == "" {
			return o.quietStep(cmd, "author", "set", author, "--scope", "global")
		}
		return o.quietStep(cmd, "author", "set", author, "-e", env)
	case "co_author":
		on, _ := v.CoAuthorFor(env)
		if env == "" {
			on = v.CoAuthor == nil || *v.CoAuthor
		}
		return o.quietStep(cmd, append([]string{"co-author", map[bool]string{true: "off", false: "on"}[on]}, scope...)...)
	case "cpus", "memory":
		value, err := o.askText(key, settingOf(key).about+"; empty takes the wider one", now, func(s string) error {
			if s == "" {
				return nil
			}
			return unibox.ValidResource(key, s)
		})
		if err != nil {
			return "", err
		}
		return o.setResource(env, key, value)
	}
	on := false
	switch key {
	case "stop_idle":
		if env != "" {
			on, _ = o.manager.Config.StopIdleFor(env)
		} else {
			on = o.manager.Config.StopIdle()
		}
	case "update_check":
		on = o.manager.Config.UpdateChecks()
	case "ssh_agent":
		on, _ = o.manager.Config.SSHAgent()
	case "ssh_sandbox":
		on, _ = o.manager.Config.EditorSSH()
	default:
		if said := *o.manager.Config.SBXSetting(key); said != nil {
			on = *said
		} else if s, err := o.manager.SBX.Setting(cmd.Context(), unibox.SBXSettings[key]); err == nil {
			on = s == "true"
		}
	}
	if risk := settingOf(key).risk; !on && risk != "" {
		yes, err := o.askYesAbout("Turn "+key+" on?", risk, false)
		if err != nil || !yes {
			return "", err
		}
	}
	return o.quietStep(cmd, append([]string{"config", "set", key, strconv.FormatBool(!on)}, scope...)...)
}

// unsetSetting forgets the person's say of a setting, so the wider one,
// or the default, stands.
func (o *options) unsetSetting(cmd *cobra.Command, key, env string) (string, error) {
	scope := []string{"--scope", "global"}
	if env != "" {
		scope = []string{"-e", env}
	}
	switch key {
	case "author":
		return o.quietStep(cmd, append([]string{"author", "rm"}, scope...)...)
	case "co_author":
		if env == "" {
			return o.quietStep(cmd, "co-author", "default")
		}
		return o.quietStep(cmd, "co-author", "default", "-e", env)
	case "cpus", "memory":
		return o.setResource(env, key, "")
	}
	if env != "" {
		return o.quietStep(cmd, "config", "unset", key, "-e", env)
	}
	return o.quietStep(cmd, "config", "unset", key)
}

// setResource keeps the CPUs or memory every env's sandboxes get, or one
// env's; "" forgets it, so the wider one stands.
func (o *options) setResource(env, key, value string) (string, error) {
	v, err := o.paths.LoadEnvValues()
	if err != nil {
		return "", err
	}
	if env == "" {
		v.Resources.Set(key, value)
	} else {
		r := v.EnvResources[env]
		r.Set(key, value)
		if r == (unibox.Resources{}) {
			delete(v.EnvResources, env)
		} else {
			v.EnvResources[env] = r
		}
	}
	if err := o.paths.SaveEnvValues(v); err != nil {
		return "", err
	}
	return key + " changed; a sandbox gets it when it is next recreated", nil
}
