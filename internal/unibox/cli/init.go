package cli

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
)

// inspectCommand shows what unibox keeps about a sandbox.
func (o *options) inspectCommand() *cobra.Command {
	return &cobra.Command{Use: "inspect [sandbox]", Args: cobra.MaximumNArgs(1), Short: "Show what unibox keeps about a sandbox", RunE: func(cmd *cobra.Command, args []string) error {
		name, err := o.sandboxName(first(args))
		if err != nil {
			return err
		}
		boxes, err := o.manager.List(cmd.Context())
		if err != nil {
			return err
		}
		for _, b := range boxes {
			if b.Name != name {
				continue
			}
			if o.json {
				return o.write(map[string]any{"api_version": api.Version, "sandbox": b})
			}
			record, _ := json.MarshalIndent(b, "", "  ")
			fmt.Fprintln(o.output, string(record))
			return nil
		}
		return fmt.Errorf("sandbox %s not found; unibox sandbox ls lists them", name)
	}}
}

// initCommand sets unibox up for a team's repository: sbx, the envs of its
// sbxenv files, the person's args, logins and sandboxes. At a terminal it
// walks through each step, running the command the step stands for, so the
// walk and the commands never differ; what is in place already is only
// said. With --no-input, or away from a terminal, it asks nothing.
func (o *options) initCommand() *cobra.Command {
	var noInput bool
	var files, vars []string
	c := &cobra.Command{Use: "init [directory|file|git-address]", Args: cobra.MaximumNArgs(1), Short: "Set unibox up for a team's repository: sbx, its envs, your args, logins and sandboxes", Long: `Run in your team's repository, init finds every sbxenv file in it - the
whole repository when you are in a part of it - and makes an env of each you
pick, named as the file names its sandbox, else by the file. It then asks
for the args an env leaves to you, has sbx store the tokens it lacks, and
offers to create the sandboxes. Running it again adds what is new and only
says what is in place.

Given a git address instead, init clones the repository with your own git
into ~/.local/share/unibox/repos/<host>/<path> and sets it up from there;
unibox env update keeps such a clone current. A repository of your own is
never pulled.

With --no-input, or without a terminal, it asks nothing: it sets up every
file found, or those of --file, takes values from --var, and fails naming
what is still missing. Tokens are never given on the command line;
unibox login set stores them.`, Example: "  unibox init\n  unibox init ~/work/agent-sandboxes\n  unibox init git@github.com:team/agent-sandboxes.git\n  unibox init --no-input --file sandboxes/dev.sbxenv.yaml --var user=jan", RunE: func(cmd *cobra.Command, args []string) error {
		target := cmp.Or(firstArg(args), ".")
		if unibox.IsGitURL(target) {
			dir, cloned, err := o.paths.Clone(cmd.Context(), target, !noInput && o.canAsk(), o.input, o.log)
			if err != nil {
				return err
			}
			if cloned {
				fmt.Fprintf(o.log, "cloned %s into %s\n", target, dir)
			} else {
				fmt.Fprintf(o.log, "%s is cloned in %s already; unibox env update updates it\n", target, dir)
			}
			target = dir
		}
		root, named, err := o.initRoot(cmd.Context(), target)
		if err != nil {
			return err
		}
		chosen, err := o.resolveFiles(root, files)
		if err != nil {
			return err
		}
		chosen = append(named, chosen...)
		if noInput || !o.canAsk() {
			return o.initQuietly(cmd, root, chosen, vars)
		}
		return o.walk(cmd, root, chosen, vars)
	}}
	c.Flags().BoolVar(&noInput, "no-input", false, "Ask nothing; fail naming what is missing")
	c.Flags().StringArrayVar(&files, "file", nil, "Set up this sbxenv file, rather than each found (repeatable)")
	c.Flags().StringArrayVar(&vars, "var", nil, "A value of yours, as name=value, for each env asking for it (repeatable)")
	return c
}

// initRoot is the repository init looks through: the one dir is in, else
// dir itself. A file given in its place is the one to set up.
func (o *options) initRoot(ctx context.Context, dir string) (root string, files []string, err error) {
	path, err := unibox.ExpandPath(dir)
	if err != nil {
		return "", nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", nil, fmt.Errorf("cannot read %s; give your team's repository or an sbxenv file in it", dir)
	}
	if !info.IsDir() {
		if !strings.HasSuffix(path, unibox.SBXEnvSuffix) {
			return "", nil, fmt.Errorf("%s is no sbxenv file; give your team's repository or a file ending in %s", dir, unibox.SBXEnvSuffix)
		}
		files, path = []string{path}, filepath.Dir(path)
	}
	return cmp.Or(unibox.RepoRoot(ctx, path), path), files, nil
}

// resolveFiles finds the files --file names: in the repository, else
// where the user is.
func (o *options) resolveFiles(root string, files []string) ([]string, error) {
	out := []string{}
	for _, f := range files {
		path := f
		if !filepath.IsAbs(f) {
			if _, err := os.Stat(filepath.Join(root, f)); err == nil {
				path = filepath.Join(root, f)
			}
		}
		path, err := unibox.ExpandPath(path)
		if err != nil {
			return nil, err
		}
		if _, err = os.Stat(path); err != nil {
			return nil, fmt.Errorf("cannot read %s; give an sbxenv file of %s", f, root)
		}
		out = append(out, path)
	}
	return out, nil
}

// envFiles are the files init sets up: those named, else every one found.
func envFiles(root string, named []string) ([]string, error) {
	if len(named) > 0 {
		return named, nil
	}
	return unibox.FindSBXEnvs(root)
}

// relative names a file of the repository as the repository does.
func relative(root, file string) string {
	if rel, err := filepath.Rel(root, file); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return file
}

// envFor is the env made of file: the one there is, else one added under
// the name the file suggests, or under name when it is given.
func (o *options) envFor(ctx context.Context, root, file, name string) (env string, added bool, err error) {
	if env, ok := o.manager.Config.EnvOf(file); ok {
		return env, false, nil
	}
	name = cmp.Or(name, unibox.EnvNameFor(file))
	if _, taken := o.manager.Config.Envs[name]; taken {
		return "", false, unibox.Fail(4, "env_name", "env %s names other files; add %s under another name with unibox env add <env> %s", name, relative(root, file), file)
	}
	if err := unibox.ValidName(name); err != nil {
		return "", false, unibox.Fail(4, "env_name", "%s cannot name the env of %s: %v; add it with unibox env add <env> %s", name, relative(root, file), err, file)
	}
	if _, err = o.addEnv(ctx, name, []string{file}, nil, false); err != nil {
		return "", false, err
	}
	return name, true, nil
}

// setVars keeps the values of --var for every env of names that asks for
// them.
func (o *options) setVars(names, vars []string) error {
	if len(vars) == 0 {
		return nil
	}
	values, err := o.paths.LoadEnvValues()
	if err != nil {
		return err
	}
	for _, v := range vars {
		key, value, ok := strings.Cut(v, "=")
		if !ok || key == "" {
			return fmt.Errorf("--var %s is not name=value", v)
		}
		if err = unibox.ValidArg(key, value); err != nil {
			return err
		}
		used := false
		for _, name := range names {
			e, err := o.manager.Config.LoadEnv(name)
			if err != nil {
				return err
			}
			if _, declared := e.SBX.Args[key]; !declared {
				continue
			}
			if _, err = e.SBX.CheckArgs(map[string]string{key: value}); err != nil {
				return fmt.Errorf("env %s: %w", name, err)
			}
			if values.Envs[name] == nil {
				values.Envs[name] = map[string]string{}
			}
			values.Envs[name][key] = value
			used = true
		}
		if !used {
			return fmt.Errorf("no env of %s asks for %s; unibox env show lists what each asks for", strings.Join(names, ", "), key)
		}
	}
	return o.paths.SaveEnvValues(values)
}

// initQuietly is init without a question: every file is set up, values
// come from --var, and what is missing fails it by name.
func (o *options) initQuietly(cmd *cobra.Command, root string, named, vars []string) error {
	ctx := cmd.Context()
	found, err := envFiles(root, named)
	if err != nil {
		return err
	}
	if len(found) == 0 {
		return unibox.Fail(4, "no_sbxenv", "no sbxenv file in %s; run unibox init in your team's repository, or name one with --file", root)
	}
	names := []string{}
	for _, f := range found {
		name, _, err := o.envFor(ctx, root, f, "")
		if err != nil {
			return err
		}
		names = append(names, name)
	}
	if err = o.setVars(names, vars); err != nil {
		return err
	}
	values, err := o.paths.LoadEnvValues()
	if err != nil {
		return err
	}
	lacking := []string{}
	for _, name := range names {
		e, err := o.manager.Config.LoadEnv(name)
		if err != nil {
			return err
		}
		for _, arg := range e.SBX.Missing(values.Envs[name]) {
			lacking = append(lacking, fmt.Sprintf("--var %s=<value> (env %s)", arg, name))
		}
	}
	if len(lacking) > 0 {
		return unibox.Fail(1, "missing_values", "set up env(s) %s, which still need values of yours: pass %s, or set them with unibox var set", strings.Join(names, ", "), strings.Join(lacking, ", "))
	}
	return o.success(fmt.Sprintf("set up env(s) %s from %s, default %s; unibox doctor says what is left, such as tokens (unibox login set)", strings.Join(names, ", "), root, o.manager.Config.DefaultEnv))
}

// walkStep heads a step of the walk.
func (o *options) walkStep(n int, title string) {
	fmt.Fprintf(o.log, "\n%d/6 %s\n", n, title)
}

func (o *options) walk(cmd *cobra.Command, root string, named, vars []string) error {
	ctx := cmd.Context()
	o.walkStep(1, "sbx")
	sbx := o.manager.SBX
	if !sbx.Present() {
		return fmt.Errorf("sbx is not installed; install Docker Sandboxes, then run unibox init again")
	}
	var signedIn bool
	if err := o.working("asking sbx whether it is signed in", func() (err error) {
		signedIn, err = sbx.SignedIn(ctx)
		return err
	}); err != nil {
		return err
	}
	if !signedIn {
		if login, err := o.askYes("sbx is not signed in to Docker. Sign in now? sbx asks on this terminal", true); err != nil {
			return err
		} else if !login {
			return fmt.Errorf("init goes on once sbx is signed in; run unibox init again")
		}
		if err := sbx.Login(ctx, o.input, o.log, o.log); err != nil {
			return err
		}
	}
	// Decided before sbx is used: unibox refuses an SSH agent sbx forwards
	// without the user's word.
	if err := o.askSSHAgent(cmd); err != nil {
		return err
	}
	if err := sbx.Available(ctx); err != nil {
		return err
	}
	fmt.Fprintln(o.log, "  installed and signed in")

	o.walkStep(2, "Envs")
	names, err := o.walkEnvs(cmd, root, named)
	if err != nil {
		return err
	}
	if err = o.setVars(names, vars); err != nil {
		return err
	}
	envs := []unibox.Env{}
	for _, name := range names {
		env, err := o.manager.Config.LoadEnv(name)
		if err != nil {
			return err
		}
		envs = append(envs, env)
	}

	o.walkStep(3, "Values")
	for _, env := range envs {
		asked, err := o.askArgs(env)
		if err != nil {
			return err
		}
		if !asked {
			fmt.Fprintf(o.log, "  env %s has every value it needs\n", env.Name)
		}
	}

	o.walkStep(4, "Logins")
	lacking := []string{}
	for _, env := range envs {
		var logins []unibox.Login
		if err = o.working("reading the kits of env "+env.Name, func() (err error) {
			logins, err = o.manager.Logins(ctx, env, "")
			return err
		}); err != nil {
			return err
		}
		missing := []string{}
		for _, l := range logins {
			if !l.Stored && !l.Optional {
				missing = append(missing, l.Service)
			}
		}
		if len(missing) == 0 {
			fmt.Fprintf(o.log, "  sbx has every token env %s needs\n", env.Name)
			continue
		}
		fmt.Fprintf(o.log, "  sbx has no token for %s of env %s\n", strings.Join(missing, ", "), env.Name)
		lacking = append(lacking, env.Name)
	}
	if len(lacking) > 0 {
		fmt.Fprintln(o.log, "  sbx asks for each on this terminal, and the value goes to it alone")
		if store, err := o.askYes("Store them now?", true); err != nil {
			return err
		} else if store {
			// sbx keeps a token for every sandbox: one stored for an env is
			// not asked for again for the next.
			for _, name := range lacking {
				if err = o.runStep(cmd, "login", "set", "-e", name); err != nil {
					return err
				}
			}
		} else {
			for _, name := range lacking {
				fmt.Fprintf(o.log, "  later: unibox login set -e %s\n", name)
			}
		}
	}

	o.walkStep(5, "Ports")
	anyPorts := false
	for _, env := range envs {
		team := []string{}
		for _, name := range sorted(env.Ports) {
			if env.Ports[name].Team {
				team = append(team, name)
			}
		}
		if len(team) == 0 {
			continue
		}
		anyPorts = true
		labels, on, byLabel := []string{}, []string{}, map[string]string{}
		for _, name := range team {
			p := env.Ports[name]
			label := name + " (" + p.Spec + ")"
			if p.Description != "" {
				label += " " + p.Description
			}
			labels, byLabel[label] = append(labels, label), name
			if p.Default {
				on = append(on, label)
			}
		}
		picked, err := o.askChoices("Ports of env "+env.Name+" to publish at every start", "The team names them; the rest stay off until unibox port publish <name>", labels, on, nil)
		if err != nil {
			return err
		}
		for _, label := range labels {
			if want := slices.Contains(picked, label); want != slices.Contains(on, label) {
				if err = o.runStep(cmd, "port", map[bool]string{true: "on", false: "off"}[want], byLabel[label], "-e", env.Name); err != nil {
					return err
				}
			}
		}
	}
	if !anyPorts {
		fmt.Fprintln(o.log, "  no env names ports; unibox port set adds your own")
	}

	o.walkStep(6, "Sandboxes")
	state, err := o.paths.ReadState()
	if err != nil {
		return err
	}
	for _, env := range envs {
		sandbox := env.SandboxName()
		if b, ok := state.Sandboxes[sandbox]; ok && b.State != "creating" {
			fmt.Fprintf(o.log, "  sandbox %s is there\n", sandbox)
		} else if create, err := o.askYes(fmt.Sprintf("Create sandbox %s of env %s now? sbx shows what it makes and asks to approve it", sandbox, env.Name), true); err != nil {
			return err
		} else if create {
			if err = o.runStep(cmd, "sandbox", "create", "-e", env.Name); err != nil {
				return err
			}
		} else {
			fmt.Fprintf(o.log, "  unibox run creates %s when it first needs it\n", sandbox)
		}
	}
	fmt.Fprintf(o.log, "\nset up; start a task with unibox run <directory>, or unibox run --shell for a shell\n")
	return nil
}

// walkEnvs picks the envs the walk sets up: of the files the repository
// holds, those the user picks; with none there, one by hand.
func (o *options) walkEnvs(cmd *cobra.Command, root string, named []string) ([]string, error) {
	ctx := cmd.Context()
	found, err := envFiles(root, named)
	if err != nil {
		fmt.Fprintf(o.log, "  %v\n", err)
	} else if len(found) == 0 {
		fmt.Fprintf(o.log, "  no sbxenv file in %s\n", root)
	}
	if len(found) == 0 {
		name, err := o.walkEnv(cmd)
		return []string{name}, err
	}
	cfg := o.manager.Config
	picked := found
	if len(named) == 0 && len(found) > 1 {
		rels, preset := []string{}, []string{}
		for _, f := range found {
			rels = append(rels, relative(root, f))
			if _, ok := cfg.EnvOf(f); ok {
				preset = append(preset, relative(root, f))
			}
		}
		if len(preset) == 0 {
			preset = rels
		}
		choice, err := o.askChoices("Envs to set up", "The sbxenv files of "+root+"; each becomes an env, named as the file names its sandbox.", rels, preset, func(v []string) error {
			if len(v) == 0 {
				return fmt.Errorf("pick at least one")
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		picked = []string{}
		for _, f := range found {
			if slices.Contains(choice, relative(root, f)) {
				picked = append(picked, f)
			}
		}
	}
	defaultBefore := cfg.DefaultEnv
	names := []string{}
	for _, f := range picked {
		name, added, err := o.envFor(ctx, root, f, "")
		var failure *unibox.Failure
		if errors.As(err, &failure) && failure.Code == "env_name" {
			why := "Its file's name makes no env name"
			if _, taken := o.manager.Config.Envs[unibox.EnvNameFor(f)]; taken {
				why = "Env " + unibox.EnvNameFor(f) + " names other files"
			}
			name, err = o.askText("Env name for "+relative(root, f), why+"; what commands call it, as in unibox run -e <name>.", "", func(s string) error {
				if _, taken := o.manager.Config.Envs[s]; taken {
					return fmt.Errorf("env %s exists already", s)
				}
				return unibox.ValidName(s)
			})
			if err == nil {
				name, added, err = o.envFor(ctx, root, f, name)
			}
		}
		if err != nil {
			return nil, err
		}
		state := "set up already"
		if added {
			state = "added"
		}
		fmt.Fprintf(o.log, "  env %s, %s: %s\n", name, relative(root, f), state)
		names = append(names, name)
	}
	if len(names) > 1 && defaultBefore == "" {
		options := []pick{}
		for _, name := range names {
			options = append(options, pick{name, name})
		}
		choice, err := o.askPick("Default env", "The env unibox run and the other commands use when not told.", options, names[0])
		if err != nil {
			return nil, err
		}
		if err = o.runStep(cmd, "config", "set", "default_env", choice); err != nil {
			return nil, err
		}
	}
	return names, nil
}

// walkEnv picks the env the walk sets up away from a repository: one there
// is, or one added from a file named by hand.
func (o *options) walkEnv(cmd *cobra.Command) (string, error) {
	cfg := o.manager.Config
	const another = "\x00add"
	if len(cfg.Envs) > 0 {
		options := []pick{}
		for _, name := range slices.Sorted(maps.Keys(cfg.Envs)) {
			options = append(options, pick{name, name + " (" + strings.Join(cfg.Envs[name].Files, ", ") + ")"})
		}
		options = append(options, pick{another, "another, from an sbxenv file"})
		choice, err := o.askPick("Env to set up", "An env names your team's sbxenv files.", options, cmp.Or(cfg.DefaultEnv, options[0].value))
		if err != nil || choice != another {
			return choice, err
		}
	}
	file, err := o.askText("sbxenv file", "Your team's file, such as agent-sandboxes/sandboxes/dev.sbxenv.yaml; unibox init in the repository finds them all.", "", func(s string) error {
		path, err := unibox.ExpandPath(s)
		if err != nil {
			return err
		}
		_, err = unibox.ReadSBXEnv([]string{path})
		return err
	})
	if err != nil {
		return "", err
	}
	path, _ := unibox.ExpandPath(file)
	name, err := o.askText("Env name", "What commands call it, as in unibox run -e <name>.", unibox.EnvNameFor(path), func(s string) error {
		if _, taken := o.manager.Config.Envs[s]; taken {
			return fmt.Errorf("env %s exists already", s)
		}
		return unibox.ValidName(s)
	})
	if err != nil {
		return "", err
	}
	return name, o.runStep(cmd, "env", "add", name, path)
}

// runStep runs a command of unibox as the walk's step.
func (o *options) runStep(cmd *cobra.Command, args ...string) error {
	c, rest, err := cmd.Root().Find(args)
	if err != nil {
		return err
	}
	if err = c.ParseFlags(rest); err != nil {
		return err
	}
	c.SetContext(cmd.Context())
	return c.RunE(c, c.Flags().Args())
}
