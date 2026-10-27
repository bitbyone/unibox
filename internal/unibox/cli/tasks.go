package cli

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
)

// Cobra calls a flag's Value in argument order. Keeping that order here makes
// one leading branch a default and a later branch belong only to its repository.
type orderedValue struct {
	req    *unibox.Request
	branch bool
}

func (v *orderedValue) String() string { return "" }
func (v *orderedValue) Type() string   { return "string" }
func (v *orderedValue) Set(value string) error {
	if v.branch {
		if len(v.req.Repositories) == 0 {
			v.req.Branch = value
		} else {
			v.req.Repositories[len(v.req.Repositories)-1].Branch = value
		}
		return nil
	}
	in := unibox.RepoInput{Source: value}
	if name, source, ok := strings.Cut(value, "="); ok && unibox.ValidName(name) == nil {
		in.Name = name
		in.Source = source
	}
	v.req.Repositories = append(v.req.Repositories, in)
	return nil
}

// shellFlag lets run and resume open a shell in the task's directory
// instead of the agent, or choose the agent; the two exclude each other.
func shellFlag(c *cobra.Command, r *unibox.Request) {
	c.Flags().BoolVar(&r.Shell, "shell", false, "Open a shell in the task's directory instead of running an agent")
	c.Flags().StringVar(&r.Agent, "agent", "", "Agent to run - claude, codex, copilot, opencode - when the sandbox has it (default: the task's last, else the env's)")
	c.MarkFlagsMutuallyExclusive("shell", "agent")
}

func taskFlags(c *cobra.Command, r *unibox.Request) {
	f := c.Flags()
	f.StringVarP(&r.Sandbox, "sandbox", "s", "", "Sandbox to work in (default: UNIBOX_SANDBOX, the context's, or the only one)")
	f.StringVarP(&r.Env, "env", "e", "", "Env to create the sandbox from, named as the env names it unless -s says (default with no sandbox: UNIBOX_ENV, the context's, or the only one)")
	f.StringVar(&r.Task, "task", "", "Name for the task, unique across sandboxes (default: its first repository and a digest)")
	f.StringVar(&r.Author, "author", "", "Commit author, as Name <email>")
	f.BoolVar(&r.Push, "push", false, "Push selected local branches; create one nobody has from the local HEAD, or a URL's default tip (never force)")
	f.BoolVar(&r.IgnoreLocal, "ignore-local", false, "Use the forge version despite uncommitted or unpushed local changes")
	mountFlags(c, r)
	f.Var(&orderedValue{req: r}, "repo", "A repository, directory or HTTPS URL, optionally dirname=source; repeat for several, a --branch after one is its alone")
	f.Var(&orderedValue{req: r, branch: true}, "branch", "Before all repos: shared branch; after a repo: override only that repo")
}

// mountFlags choose between working on the host's checkout and on a clone
// of the forge's; left to unibox, a directory of the host is mounted.
func mountFlags(c *cobra.Command, r *unibox.Request) {
	c.Flags().BoolVar(&r.Mount, "mount", false, "Mount the host's directories into the sandbox: the agent works on your files and .git as they are (the default for directories)")
	c.Flags().BoolVar(&r.Clone, "clone", false, "Clone the repositories from the forge inside the sandbox instead of mounting the host's checkout")
	c.MarkFlagsMutuallyExclusive("mount", "clone")
}

type reportedError struct{ status int }

func (e *reportedError) Error() string { return "preflight needs attention" }
func needsStatus(needs []api.Need) int {
	for _, n := range needs {
		if !n.Warning {
			return 4
		}
	}
	return 0
}

func (o *options) preflight(cmd *cobra.Command, r *unibox.Request) error {
	for attempt := 0; attempt < 4; attempt++ {
		var check api.Check
		err := o.waiting("checking what the task needs", func() (err error) {
			check, err = o.manager.Check(cmd.Context(), *r)
			return err
		})
		if err != nil {
			return err
		}
		if check.Ready {
			return nil
		}
		changed := false
		for _, n := range check.Needs {
			if n.Warning {
				// A warning does not block, but the user hears it: a
				// degraded credential boundary or a check deferred to the
				// sandbox is theirs to weigh before the task runs.
				if !o.json {
					fmt.Fprintf(o.log, "warning: %s\n", n.Message)
				}
				continue
			}
			switch n.Code {
			case "uncommitted", "local_commits":
				if r.Push && n.Code == "local_commits" {
					continue
				}
				if o.ask(n.Message + "; continue using the forge version?") {
					r.IgnoreLocal = true
					changed = true
				} else {
					return unibox.Fail(4, n.Code, "%s: %s", n.Repository, n.Message)
				}
			case "branch_not_pushed", "branch_missing":
				if !r.Push {
					return unibox.Fail(4, n.Code, "%s: %s", n.Repository, n.Message)
				}
			case "arg_missing":
				_, env, err := o.manager.Definition(*r)
				if err != nil {
					return err
				}
				asked, err := o.askArgs(env)
				if err != nil {
					return err
				}
				if !asked {
					return unibox.Fail(4, n.Code, "%s", n.Message)
				}
				changed = true
			case "network_policy":
				if !o.canAsk() {
					return unibox.Fail(4, n.Code, "%s", n.Message)
				}
				if err := o.runStep(cmd, "network", "init"); err != nil {
					return err
				}
				changed = true
			default:
				return unibox.Fail(4, n.Code, "%s", n.Message)
			}
		}
		if !changed {
			return nil
		}
	}
	return fmt.Errorf("preparation inputs keep changing; retry once the repositories are settled")
}

// oneSource takes the one repository an argument may name. Several are
// named with --repo, which also gives each its own branch and directory
// name; mixing the two would leave unclear which a --branch belongs to.
func oneSource(req *unibox.Request, args []string) error {
	switch {
	case len(args) > 1:
		return fmt.Errorf("one repository as an argument; name several with --repo <directory|url> each")
	case len(args) == 1 && len(req.Repositories) > 0:
		return fmt.Errorf("a repository as an argument and --repo do not mix; name them all with --repo")
	case len(args) == 1:
		req.Repositories = []unibox.RepoInput{{Source: args[0]}}
	}
	return nil
}

func (o *options) taskCommand(name string) *cobra.Command {
	var req unibox.Request
	var defaults bool
	c := &cobra.Command{Use: name + " [directory|url] [-- agent-args...]", Short: map[string]string{"check": "Explain what a task needs before it can start", "prepare": "Prepare a task in a sandbox: its directories mounted, or cloned from the forge", "run": "Prepare a task and run its agent, or a shell, in this terminal (unibox resume runs it again)"}[name]}
	taskFlags(c, &req)
	c.Flags().BoolVar(&defaults, "defaults", false, "Take also the env's indexed repositories that say default: on, as task new picks them to begin with")
	if name == "run" {
		shellFlag(c, &req)
	}
	c.RunE = func(cmd *cobra.Command, args []string) error {
		extra := []string{}
		if n := cmd.ArgsLenAtDash(); n >= 0 {
			extra = args[n:]
			args = args[:n]
		}
		if name != "run" && len(extra) > 0 {
			return fmt.Errorf("only run accepts agent arguments after --")
		}
		if err := oneSource(&req, args); err != nil {
			return err
		}
		if len(req.Repositories) == 0 {
			req.Repositories = []unibox.RepoInput{{Source: "."}}
		}
		o.defaultTarget(&req)
		if err := o.withIndexedRepos(&req, true, defaults); err != nil {
			return err
		}
		if name == "check" {
			check, err := o.manager.Check(cmd.Context(), req)
			if err != nil {
				return err
			}
			if o.json {
				if err = o.write(check); err != nil {
					return err
				}
			} else {
				if check.Ready {
					fmt.Fprintln(o.output, "ready (URL-only repositories are checked inside the sandbox)")
				}
				for _, n := range check.Needs {
					fmt.Fprintf(o.output, "%s: %s %s\n", n.Code, n.Repository, n.Message)
				}
			}
			if status := needsStatus(check.Needs); status != 0 {
				return &reportedError{status}
			}
			return nil
		}
		if name == "run" && o.json {
			return fmt.Errorf("run owns the terminal; use check or prepare with --json, then run without it")
		}
		if err := o.preflight(cmd, &req); err != nil {
			return err
		}
		if name == "prepare" {
			result, err := o.manager.Prepare(cmd.Context(), req)
			if err != nil {
				return err
			}
			if o.json {
				return o.write(result)
			}
			fmt.Fprintf(o.output, "task %s ready in sandbox %s (%s)\n", result.Task, result.Sandbox, result.Directory)
			return nil
		}
		return o.manager.Run(cmd.Context(), req, o.input, o.output, o.log, extra)
	}
	return c
}

// addRepoCommand grows a prepared task by more repositories. The agent in a
// running session sees them at once in its task directory.
func (o *options) addRepoCommand() *cobra.Command {
	var req unibox.Request
	c := &cobra.Command{Use: "add-repo task [directory|url]", Args: cobra.RangeArgs(1, 2), Short: "Add more repositories to a prepared task, also while its agent runs", Long: "Add more repositories to a prepared task, also while its agent runs: one directory or URL as run takes it, or several with --repo. They are mounted or cloned as the task's first ones were.", RunE: func(cmd *cobra.Command, args []string) error {
		req.Task = args[0]
		if err := oneSource(&req, args[1:]); err != nil {
			return err
		}
		if len(req.Repositories) == 0 {
			return fmt.Errorf("no repository selected; name a directory or URL, or pass --repo for several")
		}
		if err := o.withIndexedRepos(&req, false, false); err != nil {
			return err
		}
		if err := o.preflight(cmd, &req); err != nil {
			return err
		}
		t, added, err := o.manager.AddRepositories(cmd.Context(), req)
		if err != nil {
			return err
		}
		if o.json {
			return o.write(map[string]any{"api_version": api.Version, "task": t, "added": added})
		}
		if len(added) == 0 {
			fmt.Fprintf(o.output, "task %s already holds these repositories\n", t.ID)
			return nil
		}
		for _, r := range added {
			fmt.Fprintf(o.output, "added %s to task %s at %s/%s\n", r.Name, t.ID, t.Directory, r.Name)
		}
		return nil
	}}
	f := c.Flags()
	f.StringVar(&req.Author, "author", "", "Commit author, as Name <email>")
	f.BoolVar(&req.Push, "push", false, "Push selected local branches; create one nobody has from the local HEAD, or a URL's default tip (never force)")
	f.BoolVar(&req.IgnoreLocal, "ignore-local", false, "Use the forge version despite uncommitted or unpushed local changes")
	f.Var(&orderedValue{req: &req}, "repo", "A repository, directory or HTTPS URL, optionally dirname=source; repeat for several, a --branch after one is its alone")
	f.Var(&orderedValue{req: &req, branch: true}, "branch", "Before all repos: shared branch; after a repo: override only that repo")
	return c
}

func (o *options) createCommand() *cobra.Command {
	var req unibox.Request
	var adopt bool
	var res unibox.Resources
	c := &cobra.Command{Use: "create [name]", Args: cobra.MaximumNArgs(1), Short: "Create a sandbox from an env without a task in it", Long: `Create a sandbox from an env, named as the env names it unless a name is
given. --adopt takes over a sandbox sbx has already, made from the env with
sbx itself: unibox then adds tasks to it, and removes it only when forced.
--cpus and --memory give this sandbox alone its share of the machine,
written to vars.toml as [sandboxes.<name>.resources] and dropped with it.`, Example: "  unibox sandbox create -e dev\n  unibox sandbox create dev-big -e dev --cpus 8 --memory 50%\n  unibox sandbox create --adopt my2n-dev -e my2n-dev", RunE: func(cmd *cobra.Command, args []string) error {
		req.Sandbox = first(args)
		env, err := o.envName(req.Env)
		if err != nil {
			return err
		}
		req.Env = env
		if res != (unibox.Resources{}) {
			if adopt {
				return fmt.Errorf("an adopted sandbox keeps what sbx gave it; leave out --cpus and --memory")
			}
			for _, name := range unibox.ResourceNames {
				if v := res.Get(name); v != "" {
					if err := unibox.ValidResource(name, v); err != nil {
						return err
					}
				}
			}
			b, _, err := o.manager.Definition(req)
			if err != nil {
				return err
			}
			if b.State != "missing" && b.State != "creating" {
				return fmt.Errorf("sandbox %s exists already; set its resources in vars.toml and unibox sandbox recreate %s applies them", b.Name, b.Name)
			}
			if err = o.manager.Paths.SetSandboxResources(b.Name, res); err != nil {
				return err
			}
		}
		if !adopt {
			if _, e, err := o.manager.Definition(req); err == nil {
				if _, err = o.askArgs(e); err != nil {
					return err
				}
			}
			if err := o.ensureNetworkPolicy(cmd); err != nil {
				return err
			}
		}
		b, err := o.manager.Create(cmd.Context(), req, adopt)
		if err != nil {
			return err
		}
		if o.json {
			return o.write(map[string]any{"api_version": api.Version, "sandbox": b})
		}
		verb := "created"
		if adopt {
			verb = "adopted"
		}
		fmt.Fprintf(o.output, "%s sandbox %s from env %s\n", verb, b.Name, b.Env)
		return nil
	}}
	envFlag(c, &req.Env)
	c.Flags().BoolVar(&adopt, "adopt", false, "Take over a sandbox sbx already has instead of creating one")
	c.Flags().StringVar(&res.CPUs, "cpus", "", "CPUs for this sandbox alone: a count, 0 for all, or a share such as 50%")
	c.Flags().StringVar(&res.Memory, "memory", "", "Memory for this sandbox alone: a size such as 16g, or a share such as 75%")
	return c
}

// instructionsCommand shows what a task's agent is told when a session
// starts, so a person can see the env's, the repositories' and the kits'
// instructions as the agent gets them, not as each file says them.
func (o *options) instructionsCommand() *cobra.Command {
	var agent string
	var paths bool
	c := &cobra.Command{Use: "instructions task", Args: cobra.ExactArgs(1), Short: "Show the instructions a task's agent reads when its session starts", Long: `Show the instructions a task's agent reads when its session starts: the
files its own documentation names - its home's, the kits' that sbx writes
there, the session directory's and its repository's - in the order it reads
them, each once. What unibox writes before each session, the task's
AGENTS.md, is shown as the next session gets it. The files are read in the
running sandbox; a sandbox that is not running is not started, and only
what unibox writes is shown.`, RunE: func(cmd *cobra.Command, args []string) error {
		got, err := o.manager.AgentInstructions(cmd.Context(), args[0], agent)
		if err != nil {
			return err
		}
		if o.json {
			return o.write(got)
		}
		switch {
		case !got.Known:
			fmt.Fprintf(o.log, "unibox knows no instruction files of %s; only what unibox writes is shown\n", cmp.Or(got.Agent, "the task's agent"))
		case !got.Read:
			fmt.Fprintf(o.log, "sandbox %s is not running; only what unibox writes is shown - start it with unibox resume %s\n", got.Sandbox, got.Task)
		}
		if len(got.Files) == 0 {
			return o.success(fmt.Sprintf("%s in %s reads no instructions", cmp.Or(got.Agent, "the agent"), got.Directory))
		}
		for i, f := range got.Files {
			head := f.Path
			if f.Link != "" {
				head += " -> " + f.Link
			}
			if f.Unibox {
				head += " (unibox writes it before each session)"
			}
			if paths {
				fmt.Fprintln(o.output, head)
				continue
			}
			if i > 0 {
				fmt.Fprintln(o.output)
			}
			fmt.Fprintln(o.output, listTitle.Render("== "+head))
			fmt.Fprintln(o.output, strings.TrimRight(f.Content, "\n"))
		}
		return nil
	}}
	c.Flags().StringVar(&agent, "agent", "", "The agent to read for, if not the task's")
	c.Flags().BoolVar(&paths, "paths", false, "List the files alone")
	return c
}
