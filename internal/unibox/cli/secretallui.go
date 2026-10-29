package cli

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tobola/unibox/internal/unibox"
)

// secretAllUI is unibox secret --all at a terminal: every token kept, in
// every scope, and every one a sandbox needs that is kept nowhere, to
// store, move to another scope or forget. Without a terminal it is the
// table secret ls --all prints.
func (o *options) secretAllUI(cmd *cobra.Command) error {
	ctx := cmd.Context()
	if err := o.manager.SBX.Available(ctx); err != nil {
		return err
	}
	var uses []unibox.SecretUse
	build := func() (listModel, error) {
		if err := o.reloadConfig(); err != nil {
			return listModel{}, err
		}
		var skipped []string
		var err error
		if uses, skipped, err = o.manager.SecretOverview(ctx); err != nil {
			return listModel{}, err
		}
		m := listModel{title: "every token kept, and those needed", header: []string{"SERVICE", "SCOPE", "KEPT", "USED BY"}, actions: []listAction{
			{key: "enter", name: "set", help: "store"},
			{key: "s", name: "scope", help: "change scope"},
			{key: "d", name: "rm", help: "forget", confirm: "forget the token %s? the sandboxes that take it lose it"},
		}}
		if len(skipped) > 0 {
			m.note = "left out, as their files cannot be read: env " + strings.Join(skipped, ", ")
		}
		for _, u := range uses {
			m.rows = append(m.rows, []string{u.Service, cmp.Or(u.Scope, "-"), u.Kept, usedText(u)})
			m.ids = append(m.ids, useID(u))
		}
		return m, nil
	}
	act := func(action, id string) (string, error) {
		i := slices.IndexFunc(uses, func(u unibox.SecretUse) bool { return useID(u) == id })
		if i < 0 {
			return "", nil
		}
		u := uses[i]
		if (u.Kept == "variable" || strings.HasPrefix(u.Scope, "sbx sandbox ")) && action != "rm" {
			return u.Service + " is kept by sbx itself; unibox stores it with unibox secret set, s on another row moves a sandbox's", nil
		}
		switch action {
		case "rm":
			return o.forgetUse(cmd, u)
		case "scope":
			return o.moveUse(cmd, u)
		}
		return "", o.storeUse(cmd, u)
	}
	return o.actList(cmd, build, act, func() error { return o.runStep(cmd, "secret", "ls", "--all") })
}

// useID names a row of the overview: its service in its scope, which the
// confirmation shows.
func useID(u unibox.SecretUse) string {
	return u.Service + " (" + cmp.Or(u.Scope, "kept nowhere") + ")"
}

func usedText(u unibox.SecretUse) string {
	used := strings.Join(u.UsedBy, ", ")
	switch {
	case used == "":
		return "-"
	case u.Kept == "missing" && u.Needed:
		return used + " (needed)"
	}
	return used
}

// users are the sandboxes that take a token, by name, with the env each
// is of, those not made yet included.
func (o *options) users(u unibox.SecretUse) ([]string, map[string]unibox.Env) {
	names, envs := []string{}, map[string]unibox.Env{}
	state, _ := o.paths.ReadState()
	for _, who := range u.UsedBy {
		name := strings.TrimSuffix(who, " (not made)")
		envName := state.Sandboxes[name].Env
		if envName == "" {
			for _, e := range sorted(o.manager.Config.Envs) {
				if env, err := o.manager.Config.LoadEnv(e); err == nil && env.SandboxName() == name {
					envName = e
				}
			}
		}
		env, err := o.manager.Config.LoadEnv(envName)
		if err != nil {
			continue
		}
		names, envs[name] = append(names, name), env
	}
	return names, envs
}

// pickUser asks which of the sandboxes taking a token is meant, when more
// than one does.
func (o *options) pickUser(service string, names []string) (string, error) {
	if len(names) == 1 {
		return names[0], nil
	}
	picks := []pick{}
	for _, n := range names {
		picks = append(picks, pick{n, "sandbox " + n})
	}
	return o.askPick("Which sandbox's "+service+" token", "", picks, names[0])
}

// storeUse stores a token again where it is kept, the global one by sbx,
// or one kept nowhere where the person picks.
func (o *options) storeUse(cmd *cobra.Command, u unibox.SecretUse) error {
	names, envs := o.users(u)
	scope := u.Scope
	if scope == "" {
		if len(names) == 0 {
			return fmt.Errorf("no sandbox takes %s; unibox secret set %s stores it", u.Service, u.Service)
		}
		picks := []pick{{"global", "global: the one sbx keeps for every sandbox"}}
		seen := map[string]bool{}
		for _, n := range names {
			if e := envs[n].Name; !seen[e] {
				seen[e] = true
				picks = append(picks, pick{"env " + e, "env " + e + ": its sandboxes' own"})
			}
		}
		for _, n := range names {
			picks = append(picks, pick{"sandbox " + n, "sandbox " + n + ": its own, over its env's"})
		}
		var err error
		if scope, err = o.askPick("Where the "+u.Service+" token is kept", "Taken by "+strings.Join(names, ", "), picks, "global"); err != nil {
			return err
		}
	}
	kind, name, _ := strings.Cut(scope, " ")
	switch kind {
	case "global":
		args := []string{"secret", "set", u.Service}
		if len(names) > 0 {
			args = append(args, "-e", envs[names[0]].Name)
		}
		return o.runStep(cmd, args...)
	case "env":
		env, err := o.manager.Config.LoadEnv(name)
		if err != nil {
			return err
		}
		sandbox := env.SandboxName()
		for _, n := range names {
			if envs[n].Name == name {
				sandbox = n
				break
			}
		}
		return o.setInScope(cmd, env, sandbox, u.Service, "env")
	}
	env, ok := envs[name]
	if !ok {
		e, err := o.secretEnv("", name)
		if err != nil {
			return err
		}
		if env, err = o.manager.Config.LoadEnv(e); err != nil {
			return err
		}
	}
	return o.setInScope(cmd, env, name, u.Service, "sandbox")
}

// moveUse moves a token of one sandbox that takes it to another scope, as
// s does in one sandbox's list.
func (o *options) moveUse(cmd *cobra.Command, u unibox.SecretUse) (string, error) {
	names, envs := o.users(u)
	if len(names) == 0 {
		return "no sandbox takes " + u.Service + " to move it for; d forgets it", nil
	}
	sandbox, err := o.pickUser(u.Service, names)
	if err != nil {
		return "", err
	}
	env := envs[sandbox]
	return o.moveSecret(cmd, env, sandbox, u.Service, []string{"-e", env.Name, "-s", sandbox})
}

// forgetUse forgets a token where it is kept.
func (o *options) forgetUse(cmd *cobra.Command, u unibox.SecretUse) (string, error) {
	ctx := cmd.Context()
	kind, name, _ := strings.Cut(u.Scope, " ")
	var err error
	switch {
	case u.Scope == "":
		return u.Service + " is kept nowhere; enter stores it", nil
	case u.Scope == "global" && u.Kept == "variable":
		err = o.manager.SBX.SecretRemove(ctx, "", u.Placeholder)
	case u.Scope == "global":
		err = o.manager.SBX.SecretRemove(ctx, u.Service, "")
	case kind == "env":
		err = o.manager.RemoveOwnSecret(ctx, unibox.SecretScope{Env: name}, name, u.Service)
	case kind == "sandbox":
		envName, _ := o.secretEnv("", name)
		err = o.manager.RemoveOwnSecret(ctx, unibox.SecretScope{Sandbox: name}, envName, u.Service)
	case u.Kept == "variable":
		return "sbx keeps " + u.Service + " for " + strings.TrimPrefix(u.Scope, "sbx ") + " alone; it goes with the sandbox", nil
	default:
		err = o.manager.SBX.SecretRemoveSandbox(ctx, u.Service, strings.TrimPrefix(u.Scope, "sbx sandbox "))
	}
	if err != nil {
		return "", err
	}
	return "forgot the " + useID(u) + " token", nil
}
