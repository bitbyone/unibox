package cli

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
)

// The repositories an env's tasks may be given by name: the team's, under
// repos: in its unibox.yaml files, and the person's in config.yaml, for
// every env or one. A task takes those that say always, cloned beside what
// it mounts; unibox task new offers the rest, those that say on picked.

// repoFrom says where an indexed repository, or its default, comes from.
func repoFrom(from string) string {
	return map[string]string{unibox.FromRepo: "the repository's", unibox.FromEnv: "the env's", unibox.FromYoursAll: "yours, every env", unibox.FromYours: "yours"}[from]
}

// repoRole says whether a task works in a repository or reads it.
func repoRole(r unibox.IndexedRepo) string {
	if r.IsContext() {
		return "context"
	}
	return "work"
}

// repoDefaultText is a repository's default, marked when it is the
// person's say over the team's.
func repoDefaultText(r unibox.IndexedRepo) string {
	if (r.DefaultFrom == unibox.FromYours || r.DefaultFrom == unibox.FromYoursAll) && r.From != r.DefaultFrom {
		return r.Default + " (yours)"
	}
	return r.Default
}

func (o *options) repoCommand() *cobra.Command {
	var envFlag string
	c := &cobra.Command{Use: "repo", Args: cobra.NoArgs, Short: "The repositories an env's tasks are given by name", Long: `The repositories an env's tasks may be given by name, each a forge
address with the branch a task works on and a default: always, every new
task of the env takes it, cloned beside what it mounts - architecture
decisions, a business specification; on, picked to begin with in unibox
task new; off, there to be picked. The team names them under repos: in its
unibox.yaml files, you in config.yaml, for every env or one, over the
team's.

unibox task new picks from them, and run, prepare and --repo take a name
for its address. Alone at a terminal, unibox repo lists them to filter
with /, add one, switch its default with space, remove yours, or start a
task with one.`, Example: "  unibox repo ls\n  unibox repo add adrs https://gitlab.example.com/arch/adrs.git --ref main --default always\n  unibox repo default spec on\n  unibox task new adrs payments"}
	c.PersistentFlags().StringVarP(&envFlag, "env", "e", "", "Env (default: UNIBOX_ENV, the context's, or the only one)")
	load := func() (unibox.Env, error) {
		name, err := o.envName(envFlag)
		if err != nil {
			return unibox.Env{}, err
		}
		return o.manager.Config.LoadEnv(name)
	}
	ls := &cobra.Command{Use: "ls", Args: cobra.NoArgs, Short: "List an env's repositories, their branch, default and where each comes from", RunE: func(cmd *cobra.Command, args []string) error {
		env, err := load()
		if err != nil {
			return err
		}
		if o.json {
			return o.write(map[string]any{"api_version": api.Version, "env": env.Name, "repos": env.Repos})
		}
		if len(env.Repos) == 0 {
			fmt.Fprintf(o.output, "env %s names no repositories; unibox repo add adds one, or the team's unibox.yaml under repos:\n", env.Name)
			return nil
		}
		w := tabwriter.NewWriter(o.output, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "REPO\tROLE\tREF\tDEFAULT\tFROM\tURL\tDESCRIPTION")
		for _, name := range sorted(env.Repos) {
			r := env.Repos[name]
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", name, repoRole(r), cmp.Or(r.Ref, "-"), repoDefaultText(r), repoFrom(r.From), r.URL, r.Description)
		}
		return w.Flush()
	}}
	var add unibox.IndexedRepo
	var everyEnv bool
	var contextFlag string
	addCmd := &cobra.Command{Use: "add name url|directory", Args: cobra.ExactArgs(2), Short: "Name a repository of yours, a forge's to clone or a directory to mount, for the env or every env", RunE: func(cmd *cobra.Command, args []string) error {
		env, err := o.repoScope(envFlag, everyEnv)
		if err != nil {
			return err
		}
		if add.Default != "" {
			d, ok := unibox.RepoDefaultOf(add.Default)
			if !ok {
				return fmt.Errorf("--default is always, on or off")
			}
			add.Default = d
		}
		if contextFlag != "" {
			on, err := strconv.ParseBool(contextFlag)
			if err != nil {
				return fmt.Errorf("--context is true or false")
			}
			add.Context = &on
		}
		add.URL = args[1]
		if err := o.manager.SetRepo(env, args[0], add); err != nil {
			return err
		}
		return o.success(fmt.Sprintf("repository %s is named %s", args[0], scopeText(env)))
	}}
	addCmd.Flags().StringVar(&add.Ref, "ref", "", "Branch a task works on (default: the forge's default branch)")
	addCmd.Flags().StringVar(&add.Default, "default", "", "always, on or off (default off)")
	addCmd.Flags().StringVar(&add.Description, "description", "", "What it is, shown where it is picked")
	addCmd.Flags().StringVar(&contextFlag, "context", "", "true for one a task reads and keeps on its ref (default: true when always)")
	addCmd.Flags().BoolVar(&everyEnv, "every-env", false, "Name it for every env rather than this one")
	var rmEvery bool
	rm := &cobra.Command{Use: "rm name", Args: cobra.ExactArgs(1), Short: "Forget a repository of yours", RunE: func(cmd *cobra.Command, args []string) error {
		env, err := o.repoScope(envFlag, rmEvery)
		if err != nil {
			return err
		}
		if err := o.manager.RemoveRepo(env, args[0]); err != nil {
			return err
		}
		return o.success(fmt.Sprintf("forgot repository %s %s", args[0], scopeText(env)))
	}}
	rm.Flags().BoolVar(&rmEvery, "every-env", false, "Forget the one named for every env")
	def := &cobra.Command{Use: "default name always|on|off|-", Args: cobra.ExactArgs(2), Short: "Say whether a new task takes a repository, has it picked, or neither, for you", RunE: func(cmd *cobra.Command, args []string) error {
		name, err := o.envName(envFlag)
		if err != nil {
			return err
		}
		d := ""
		if args[1] != "-" {
			var ok bool
			if d, ok = unibox.RepoDefaultOf(args[1]); !ok {
				return fmt.Errorf("say always, on, off, or - to take your say back, not %s", args[1])
			}
		}
		if err := o.manager.SetRepoDefault(name, args[0], d); err != nil {
			return err
		}
		env, err := o.manager.Config.LoadEnv(name)
		if err != nil {
			return err
		}
		r := env.Repos[args[0]]
		return o.success(fmt.Sprintf("repository %s of env %s is %s, %s", args[0], name, r.Default, cmp.Or(repoFrom(r.DefaultFrom), "as nothing says otherwise")))
	}}
	c.AddCommand(ls, addCmd, rm, def)
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if o.json || !o.interactive {
			return o.runStep(cmd, append([]string{"repo", "ls"}, envArgs(envFlag)...)...)
		}
		name, err := o.envName(envFlag)
		if err != nil {
			return err
		}
		return o.repoUI(cmd, name)
	}
	return c
}

// repoScope is the env a repository of the person's is named for, "" for
// every env.
func (o *options) repoScope(envFlag string, every bool) (string, error) {
	if every {
		if envFlag != "" {
			return "", fmt.Errorf("-e and --every-env do not mix; give one")
		}
		return "", nil
	}
	return o.envName(envFlag)
}

func scopeText(env string) string {
	if env == "" {
		return "for every env"
	}
	return "for env " + env
}

// repoUI is unibox repo at a terminal: an env's repositories, to filter,
// add, switch, remove or start a task with.
func (o *options) repoUI(cmd *cobra.Command, envName string) error {
	build := func() (listModel, error) {
		if err := o.reloadConfig(); err != nil {
			return listModel{}, err
		}
		env, err := o.manager.Config.LoadEnv(envName)
		if err != nil {
			return listModel{}, err
		}
		m := listModel{title: "repositories of env " + envName, header: []string{"REPO", "ROLE", "REF", "DEFAULT", "FROM", "ABOUT"}, actions: []listAction{
			{key: "n", name: "new", help: "new task with it"},
			{key: " ", name: "default", help: "off/on/always"},
			{key: "a", name: "add", help: "add", any: true},
			{key: "d", name: "rm", help: "remove yours", confirm: "forget repository %s?"},
		}}
		for _, name := range sorted(env.Repos) {
			r := env.Repos[name]
			def := repoDefaultText(r)
			switch r.Default {
			case unibox.RepoAlways:
				def = portOn.Render(def)
			case unibox.RepoOff:
				def = portOff.Render(def)
			}
			role := repoRole(r)
			if r.IsContext() {
				role = portOff.Render(role)
			}
			m.rows = append(m.rows, []string{name, role, cmp.Or(r.Ref, "-"), def, repoFrom(r.From), cmp.Or(r.Description, r.URL)})
			m.ids = append(m.ids, name)
		}
		return m, nil
	}
	act := func(action, name string) (string, error) {
		switch action {
		case "new":
			return errListDone, o.runStep(cmd, "task", "new", name, "-e", envName)
		case "default":
			env, err := o.manager.Config.LoadEnv(envName)
			if err != nil {
				return "", err
			}
			now := slices.Index(unibox.RepoDefaults, env.Repos[name].Default)
			next := unibox.RepoDefaults[(now+1)%len(unibox.RepoDefaults)]
			return o.quietStep(cmd, "repo", "default", name, next, "-e", envName)
		case "rm":
			env, err := o.manager.Config.LoadEnv(envName)
			if err != nil {
				return "", err
			}
			args := []string{"repo", "rm", name, "-e", envName}
			if env.Repos[name].From == unibox.FromYoursAll {
				args = []string{"repo", "rm", name, "--every-env"}
			}
			return o.quietStep(cmd, args...)
		case "add":
			name, err := o.askText("Repository name", "How a task names it, and its directory there", "", unibox.ValidName)
			if err != nil {
				return "", err
			}
			url, err := o.askText("Address of "+name, "https://host/group/repo.git or git@host:group/repo.git, cloned; or a directory of this machine, mounted", "", func(s string) error {
				if p := (unibox.IndexedRepo{URL: s}).LocalPath(); p != "" {
					if !isDirectory(p) {
						return fmt.Errorf("%s is no directory of this machine", s)
					}
					return nil
				}
				_, err := unibox.CloneURL(s)
				return err
			})
			if err != nil {
				return "", err
			}
			ref, err := o.askText("Branch a task works on", "Empty for the forge's default branch", "", nil)
			if err != nil {
				return "", err
			}
			scope, err := o.askPick("For which envs", "", []pick{{"env", "env " + envName}, {"every", "every env"}}, "env")
			if err != nil {
				return "", err
			}
			args := []string{"repo", "add", name, url}
			if ref != "" {
				args = append(args, "--ref", ref)
			}
			if scope == "every" {
				args = append(args, "--every-env")
			} else {
				args = append(args, "-e", envName)
			}
			return o.quietStep(cmd, args...)
		}
		return "", nil
	}
	return o.actList(cmd, build, act, func() error { return o.runStep(cmd, "repo", "ls", "-e", envName) })
}

// withIndexedRepos gives a request the addresses of the repositories it
// names from its env's index, and a new task the ones every task of the
// env takes, cloned beside what it mounts. A name that is a directory here
// stays the directory; a task that exists keeps what it holds.
func (o *options) withIndexedRepos(req *unibox.Request, always bool) error {
	_, env, err := o.manager.Definition(*req)
	if err != nil || len(env.Repos) == 0 {
		// Without an env to read, the task's own preparation says why.
		return nil
	}
	mounts := false
	for i, in := range req.Repositories {
		r, named := env.Repos[in.Source]
		if !named || in.Name != "" {
			continue
		}
		if _, err := os.Stat(in.Source); err == nil {
			continue
		}
		// A branch the person gives wins over the index's.
		req.Repositories[i] = indexedInput(in.Source, r, cmp.Or(in.Branch, req.Branch, r.Ref))
		mounts = mounts || r.LocalPath() != ""
	}
	defer func() {
		// A directory of the index is mounted, so an address beside it is
		// cloned rather than turning the task to clones.
		if mounts {
			for i, in := range req.Repositories {
				if p, err := unibox.ExpandPath(in.Source); err != nil || !isDirectory(p) {
					req.Repositories[i].Clone = true
				}
			}
		}
	}()
	if !always || req.Task != "" && o.taskExists(req.Task) {
		return nil
	}
	// What the person asked for, by its forge address, so a checkout of a
	// repository every task takes is not given it twice.
	asked := map[string]bool{}
	for _, in := range req.Repositories {
		asked[unibox.SourceURL(context.Background(), in.Source)] = true
	}
	for _, name := range sorted(env.Repos) {
		r := env.Repos[name]
		if r.Default != unibox.RepoAlways || asked[unibox.SourceURL(context.Background(), cmp.Or(r.LocalPath(), r.URL))] || slices.ContainsFunc(req.Repositories, func(in unibox.RepoInput) bool { return in.Name == name }) {
			continue
		}
		in := indexedInput(name, r, r.Ref)
		in.Always, in.Context = true, r.IsContext()
		req.Repositories = append(req.Repositories, in)
		mounts = mounts || r.LocalPath() != ""
	}
	return nil
}

// indexedInput is a repository of the index as a task takes it: a host
// directory mounted, a forge's cloned.
func indexedInput(name string, r unibox.IndexedRepo, branch string) unibox.RepoInput {
	if p := r.LocalPath(); p != "" {
		return unibox.RepoInput{Source: p, Name: name, Branch: branch}
	}
	return unibox.RepoInput{Source: r.URL, Name: name, Branch: branch, Clone: true}
}

// taskExists says whether a task of that name or alias is unibox's.
func (o *options) taskExists(name string) bool {
	state, err := o.paths.ReadState()
	if err != nil {
		return false
	}
	for _, b := range state.Sandboxes {
		if slices.ContainsFunc(b.Tasks, func(t api.Task) bool { return t.ID == name || t.Alias == name }) {
			return true
		}
	}
	return false
}

// taskNewCommand starts a task on repositories of the env's index, picked
// at a terminal or named, each cloned from its forge.
func (o *options) taskNewCommand() *cobra.Command {
	var sandbox, envFlag, task, agent, author, branch string
	var shell, noPush bool
	c := &cobra.Command{Use: "new [repo...]", Short: "Start a task on repositories of the env's index, picked or named, each cloned", Long: `Start a task on repositories the env's index names (unibox repo ls),
each cloned from its forge. A repository is worked in, or read - a
context one, such as decisions or a specification, which stays on its ref
and is never pushed to. Those worked in get the task's branch, made on the
forge from its default branch when it has none, else their ref.

At a terminal without names, one screen offers them: space picks, c
switches work and context, b types the branch, t the task's name, / filters,
and each row shows the branch it gets and whether it is made; enter
starts. Named, or without a terminal, the task takes those named and every
one that says always - with no names, those that say on as well - with
--branch and --task. The task then runs as unibox run does.`, Example: "  unibox task new\n  unibox task new payments adrs --branch feat/pricing\n  unibox task new spec --task pricing --shell"}
	c.RunE = func(cmd *cobra.Command, args []string) error {
		req := unibox.Request{Sandbox: sandbox, Env: envFlag, Task: task, Agent: agent, Author: author, Shell: shell}
		o.defaultTarget(&req)
		_, env, err := o.manager.Definition(req)
		if err != nil {
			return err
		}
		if len(env.Repos) == 0 {
			return fmt.Errorf("env %s names no repositories; unibox repo add adds one, or the team's unibox.yaml under repos:", env.Name)
		}
		for _, a := range args {
			if _, ok := env.Repos[a]; !ok {
				return fmt.Errorf("env %s names no repository %s; unibox repo ls lists them", env.Name, a)
			}
		}
		// What the task takes: picked at a terminal, else named, else the
		// index's defaults; each worked in or read as the index says.
		picked := taskPickModel{branch: branch, task: task, rows: taskRows(cmd.Context(), env.Repos)}
		for i, r := range picked.rows {
			if len(args) > 0 {
				named := slices.Contains(args, r.name)
				picked.rows[i].picked = named || r.repo.Default == unibox.RepoAlways
				picked.rows[i].preset = !named
			}
			picked.rows[i].push = picked.rows[i].push && !noPush
		}
		if len(args) == 0 && o.canAsk() {
			if picked, err = o.pickTask(cmd, env, nil, branch, noPush); err != nil {
				return err
			}
		}
		// The name the screen showed is the one the task gets.
		req.Task = cmp.Or(picked.taskName(), req.Task)
		if err := o.confirmBranch(cmd, picked); err != nil {
			return err
		}
		// A work branch is pushed, or made on the forge, where asked; a
		// context repository is never pushed to.
		req.Repositories = picked.inputs()
		if len(req.Repositories) == 0 {
			return fmt.Errorf("no repository picked; pick one with space, or name it: unibox task new <repo>")
		}
		if o.json {
			return fmt.Errorf("task new owns the terminal, as run does; use task check or prepare with --json")
		}
		if err := o.preflight(cmd, &req); err != nil {
			return err
		}
		return o.manager.Run(cmd.Context(), req, o.input, o.output, o.log, nil)
	}
	f := c.Flags()
	f.StringVarP(&sandbox, "sandbox", "s", "", "Sandbox to work in (default: UNIBOX_SANDBOX, the context's, or the only one)")
	f.StringVarP(&envFlag, "env", "e", "", "Env whose index and sandbox to use (default with no sandbox: UNIBOX_ENV, the context's, or the only one)")
	f.StringVar(&task, "task", "", "Name for the task, unique across sandboxes")
	f.StringVar(&branch, "branch", "", "Branch of the repositories worked in, made where it is missing (default: each its ref, its checkout's branch or the forge's default)")
	f.BoolVar(&noPush, "no-push", false, "Make the branch in the sandbox, or the host's checkout, alone; never on the forge")
	f.StringVar(&author, "author", "", "Commit author, as Name <email>")
	f.BoolVar(&shell, "shell", false, "Open a shell in the task's directory instead of running an agent")
	f.StringVar(&agent, "agent", "", "Agent to run, when the sandbox has it")
	c.MarkFlagsMutuallyExclusive("shell", "agent")
	return c
}

// pickItem is one of the repositories offered: its name, the line it
// shows, and whether it is picked to begin with.
type pickItem struct {
	value, label string
	picked       bool
}

// repoItems are an index's repositories to pick from, aligned in columns,
// those a task takes or has picked to begin with picked.
func repoItems(repos map[string]unibox.IndexedRepo) []pickItem {
	names := sorted(repos)
	nameW, refW := 0, 0
	for _, n := range names {
		nameW, refW = max(nameW, len(n)), max(refW, len(cmp.Or(repos[n].Ref, "-")))
	}
	items := []pickItem{}
	for _, n := range names {
		r := repos[n]
		about := cmp.Or(r.Description, r.URL)
		if r.Default == unibox.RepoAlways {
			about = "always · " + about
		}
		label := strings.TrimRight(fmt.Sprintf("%-*s  %-*s  %s", nameW, n, refW, cmp.Or(r.Ref, "-"), about), " ")
		items = append(items, pickItem{value: n, label: label, picked: r.Default != unibox.RepoOff})
	}
	return items
}
