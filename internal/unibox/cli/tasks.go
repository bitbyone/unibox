package cli

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"text/tabwriter"

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
// instead of the agent; the two exclude each other.
func shellFlag(c *cobra.Command, r *unibox.Request) {
	c.Flags().BoolVar(&r.Shell, "shell", false, "Open a shell in the task's directory instead of running an agent")
	c.MarkFlagsMutuallyExclusive("agent", "shell")
}

func taskFlags(c *cobra.Command, r *unibox.Request) {
	f := c.Flags()
	f.StringVar(&r.Sandbox, "sandbox", "", "Sandbox to work in; one that does not exist yet is made from --profile (default: the profile's name)")
	f.StringVar(&r.Profile, "profile", "", "Environment profile for a new sandbox")
	f.StringVar(&r.Runtime, "runtime", "", "Runtime adapter (existing sandboxes keep theirs)")
	f.StringVar(&r.Agent, "agent", "", "Coding agent declared by the profile")
	f.StringVar(&r.Task, "task", "", "Name for the task, unique across sandboxes (default: its first repository and a digest)")
	f.StringVar(&r.Author, "author", "", "Commit author, as Name <email>")
	f.StringSliceVar(&r.Instructions, "instructions", nil, "Named instruction texts for a new sandbox")
	f.BoolVar(&r.Push, "push", false, "Push selected local branches; create one nobody has from the local HEAD, or a URL's default tip (never force)")
	f.BoolVar(&r.IgnoreLocal, "ignore-local", false, "Use the forge version despite uncommitted or unpushed local changes")
	f.BoolVar(&r.Mount, "mount", false, "Mount the selected directories of the host into the sandbox instead of cloning them: the agent works on your files and .git as they are")
	f.Var(&orderedValue{req: r}, "repo", "Repository path or HTTPS URL, optionally name=source (repeatable)")
	f.Var(&orderedValue{req: r, branch: true}, "branch", "Before all repos: shared branch; after a repo: override only that repo")
}

type reportedError struct{ status int }

func (e *reportedError) Error() string { return "preflight needs attention" }
func needsStatus(needs []api.Need) int {
	for _, n := range needs {
		if !n.Warning {
			switch n.Code {
			case "image_required":
				return 3
			default:
				return 4
			}
		}
	}
	return 0
}

func (o *options) preflight(cmd *cobra.Command, r *unibox.Request) error {
	for attempt := 0; attempt < 4; attempt++ {
		check, err := o.manager.Check(cmd.Context(), *r)
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
			case "image_required":
				// The profile is the user's; a change of the image is told
				// and built, not asked about.
				_, rt, p, err := o.manager.Definition(*r)
				if err != nil {
					return err
				}
				fmt.Fprintf(o.log, "profile %s changed what its image is built from; building it (unibox diff %s shows what)\n", p.Name, p.Name)
				if err = o.manager.Build(cmd.Context(), p.Name, rt.Name(), false); err != nil {
					return err
				}
				changed = true
			case "login_required":
				// A token or command sign-in needs nothing typed: the host
				// command is in the profile the user wrote, so the
				// runtime is told its source without asking again. An OAuth
				// login or a key still needs the user at the terminal.
				_, _, p, err := o.manager.Definition(*r)
				if err != nil {
					return err
				}
				if m := p.Auth[n.Agent].Method; m == unibox.AuthToken || m == unibox.AuthCommand {
					// sbx's notes on a stored secret are kept for a failure.
					var said bytes.Buffer
					if err := o.manager.Login(cmd.Context(), *r, n.Agent, strings.NewReader(""), &said, &said); err != nil {
						fmt.Fprint(o.log, said.String())
						return err
					}
					fmt.Fprintf(o.log, "stored %s's %s source in the runtime\n", n.Agent, m)
					changed = true
					continue
				}
				if o.json || !o.ask(n.Message+".\nStore it now?") {
					return unibox.Fail(4, n.Code, "%s", n.Message)
				}
				if err := o.manager.Login(cmd.Context(), *r, n.Agent, o.input, o.log, o.log); err != nil {
					return err
				}
				changed = true
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
			default:
				return unibox.Fail(4, n.Code, "%s", n.Message)
			}
		}
		if !changed {
			return nil
		}
	}
	return fmt.Errorf("preparation inputs keep changing; retry after the profile is stable")
}

func (o *options) taskCommand(name string) *cobra.Command {
	var req unibox.Request
	c := &cobra.Command{Use: name + " [directory...] [-- agent-args...]", Short: map[string]string{"check": "Explain what a task needs before it can start", "prepare": "Prepare independent forge clones inside a sandbox", "run": "Prepare a task and run its agent, or a shell, in this terminal (unibox resume runs it again)"}[name]}
	taskFlags(c, &req)
	if name == "run" {
		scriptFlag(c, &req)
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
		for _, dir := range args {
			req.Repositories = append(req.Repositories, unibox.RepoInput{Source: dir})
		}
		if len(req.Repositories) == 0 {
			req.Repositories = []unibox.RepoInput{{Source: "."}}
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
// running session sees them at once: they are clones in its task directory.
func (o *options) addRepoCommand() *cobra.Command {
	var req unibox.Request
	c := &cobra.Command{Use: "add-repo task [directory...]", Args: cobra.MinimumNArgs(1), Short: "Clone more repositories into a prepared task, also while its agent runs", Long: "Clone more repositories into a prepared task, also while its agent runs: directories as run takes them, or --repo for a URL or name=source.", RunE: func(cmd *cobra.Command, args []string) error {
		req.Task = args[0]
		for _, dir := range args[1:] {
			req.Repositories = append(req.Repositories, unibox.RepoInput{Source: dir})
		}
		if len(req.Repositories) == 0 {
			return fmt.Errorf("no repository selected; name a directory, or pass --repo <path-or-url>")
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
	f.BoolVar(&req.Mount, "mount", false, "Mount the selected directories of the host into the sandbox instead of cloning them: the agent works on your files and .git as they are")
	f.Var(&orderedValue{req: &req}, "repo", "Repository path or HTTPS URL, optionally name=source (repeatable)")
	f.Var(&orderedValue{req: &req, branch: true}, "branch", "Before all repos: shared branch; after a repo: override only that repo")
	return c
}

func (o *options) createCommand() *cobra.Command {
	var req unibox.Request
	c := &cobra.Command{Use: "create sandbox", Args: cobra.ExactArgs(1), Short: "Create a named environment without mounting a host workspace", RunE: func(cmd *cobra.Command, args []string) error {
		req.NewSandbox = args[0]
		if err := o.preflight(cmd, &req); err != nil {
			return err
		}
		b, err := o.manager.Create(cmd.Context(), req)
		if err != nil {
			return err
		}
		if o.json {
			return o.write(map[string]any{"api_version": api.Version, "sandbox": b})
		}
		fmt.Fprintf(o.output, "created sandbox %s (%s: %s)\n", b.Name, b.Runtime, b.RuntimeName)
		return nil
	}}
	c.Flags().StringVar(&req.Profile, "profile", "", "Environment profile")
	c.Flags().StringVar(&req.Runtime, "runtime", "", "Runtime adapter")
	c.Flags().StringSliceVar(&req.Instructions, "instructions", nil, "Named instruction texts to attach")
	return c
}

func scriptFlag(c *cobra.Command, r *unibox.Request) {
	c.Flags().StringArrayVar(&r.Scripts, "script", nil, "Run one of the profile's scripts in the sandbox before the agent starts (repeatable, in order)")
}

// scriptCommand lists a sandbox's scripts, or runs them at once.
func (o *options) scriptCommand() *cobra.Command {
	return &cobra.Command{Use: "script sandbox [name...]", Args: cobra.MinimumNArgs(1), Short: "List the scripts of a sandbox's profile, or run them in it now", Long: "List the scripts a sandbox's profile names under scripts, or run them in the sandbox now, in /work, one after another; the first to fail stops the rest. unibox run and resume take --script to run them before the agent starts instead.", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 1 {
			if o.json {
				return fmt.Errorf("a script writes to the terminal; omit --json")
			}
			return o.manager.RunScripts(cmd.Context(), args[0], args[1:], o.output)
		}
		profile, scripts, err := o.manager.SandboxScripts(args[0])
		if err != nil {
			return err
		}
		if o.json {
			if scripts == nil {
				scripts = map[string]string{}
			}
			return o.write(map[string]any{"api_version": api.Version, "sandbox": args[0], "profile": profile, "scripts": scripts})
		}
		if len(scripts) == 0 {
			fmt.Fprintf(o.output, "profile %s has no scripts; name them under scripts in its sandbox.yaml\n", profile)
			return nil
		}
		w := tabwriter.NewWriter(o.output, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "SCRIPT\tFILE")
		names := []string{}
		for n := range scripts {
			names = append(names, n)
		}
		slices.Sort(names)
		for _, n := range names {
			fmt.Fprintf(w, "%s\t%s\n", n, scripts[n])
		}
		return w.Flush()
	}}
}
