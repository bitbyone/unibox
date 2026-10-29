// Package cli exposes unibox through Cobra, without importing unagit's commands.
package cli

import (
	"bufio"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
	"github.com/tobola/unibox/internal/version"
	"golang.org/x/term"
)

type options struct {
	paths       unibox.Paths
	json, yes   bool
	input       io.Reader
	output, log io.Writer
	manager     *unibox.Manager
	interactive bool
}

func New(paths unibox.Paths, in io.Reader, out, errout io.Writer) *cobra.Command {
	o := &options{paths: paths, input: in, output: out, log: errout}
	if f, ok := in.(*os.File); ok {
		o.interactive = term.IsTerminal(int(f.Fd()))
	}
	return o.root()
}

func (o *options) root() *cobra.Command {
	paths, in, out, errout := o.paths, o.input, o.output, o.log
	root := &cobra.Command{Use: "unibox", Short: "Run coding agents in independent sandbox environments", SilenceErrors: true, SilenceUsage: true, Version: version.String()}
	// As unagit: the release alone, or dev and the commit while developing.
	root.SetVersionTemplate("{{.Version}}\n")
	root.SetIn(in)
	root.SetOut(out)
	root.SetErr(errout)
	root.PersistentFlags().BoolVar(&o.json, "json", false, "Write versioned JSON without prompting")
	root.PersistentFlags().BoolVarP(&o.yes, "yes", "y", false, "Accept operation prompts (never discards work)")
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		m, err := unibox.Open(paths, errout)
		if err != nil {
			return err
		}
		m.In, m.Out, m.Err = in, out, errout
		o.manager = m
		return m.RefreshPlugin()
	}
	// What a command works on is a word of a group (unibox task rm); only
	// the everyday ones stand alone.
	root.AddCommand(o.taskCommand("run"), o.resumeCommand(), o.initCommand(), o.contextCommand(), o.configCommand(),
		o.taskGroup(), o.sandboxGroup(), o.envCommand(), o.varCommand(), o.secretCommand(), o.portCommand(), o.networkCommand(), o.repoCommand(),
		o.herdrCommand(), o.doctorCommand(), o.pruneCommand(), o.destroyCommand(), o.versionCommand())
	// Alone at a terminal, unibox is its sandboxes, the list one acts in
	// most; anywhere else, or before init, it says how it is used.
	root.Args = cobra.NoArgs
	root.RunE = func(cmd *cobra.Command, args []string) error {
		if o.json || !o.interactive {
			return cmd.Help()
		}
		if len(o.manager.Config.Envs) == 0 {
			if err := cmd.Help(); err != nil {
				return err
			}
			fmt.Fprintln(o.log, "\nunibox is not set up yet; unibox init sets it up from your team's repository")
			return nil
		}
		return o.runStep(cmd, "sandbox")
	}
	return root
}

func Execute(ctx context.Context, args []string, in io.Reader, out, errout io.Writer) int {
	paths, err := unibox.DefaultPaths()
	if err != nil {
		fmt.Fprintln(errout, err)
		return 1
	}
	root := New(paths, in, out, errout)
	root.SetArgs(args)
	err = root.ExecuteContext(ctx)
	if err == nil {
		return 0
	}
	status := 1
	code := "error"
	var failure *unibox.Failure
	if errors.As(err, &failure) {
		status = failure.Status
		code = failure.Code
	}
	var reported *reportedError
	if errors.As(err, &reported) {
		return reported.status
	}
	jsonMode, _ := root.PersistentFlags().GetBool("json")
	if jsonMode {
		_ = json.NewEncoder(out).Encode(api.Error{APIVersion: api.Version, Code: code, Message: err.Error()})
	} else {
		fmt.Fprintln(errout, "unibox:", err)
	}
	return status
}

func (o *options) write(v any) error {
	e := json.NewEncoder(o.output)
	e.SetIndent("", "  ")
	return e.Encode(v)
}
func (o *options) success(message string) error {
	if o.json {
		return o.write(map[string]any{"api_version": api.Version, "ok": true, "message": message})
	}
	fmt.Fprintln(o.output, message)
	return nil
}
func (o *options) ask(question string) bool {
	if o.json || !o.interactive {
		return false
	}
	fmt.Fprintf(o.log, "%s [y/N] ", question)
	line, _ := bufio.NewReader(o.input).ReadString('\n')
	return strings.EqualFold(strings.TrimSpace(line), "y") || strings.EqualFold(strings.TrimSpace(line), "yes")
}
func (o *options) confirm(question string) error {
	if o.yes || o.ask(question) {
		return nil
	}
	return unibox.Fail(4, "confirmation_required", "%s; rerun with --yes to confirm", strings.ToLower(question))
}

func (o *options) configCommand() *cobra.Command {
	c := &cobra.Command{Use: "config", Short: "Read or change unibox's own configuration"}
	c.AddCommand(&cobra.Command{Use: "get [key]", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		cfg := o.manager.Config
		var value any = cfg
		switch {
		case len(args) == 1 && unibox.SBXSettings[args[0]] != "":
			v, err := o.manager.SBX.Setting(cmd.Context(), unibox.SBXSettings[args[0]])
			if err != nil {
				return err
			}
			value = v == "true"
		case len(args) == 1:
			v, err := configValue(cfg, args[0])
			if err != nil {
				return err
			}
			value = v
		}
		if o.json {
			return o.write(map[string]any{"api_version": api.Version, "value": value})
		}
		switch v := value.(type) {
		case string:
			fmt.Fprintln(o.output, v)
		case bool:
			fmt.Fprintln(o.output, v)
		default:
			b, _ := json.MarshalIndent(v, "", "  ")
			fmt.Fprintln(o.output, string(b))
		}
		return nil
	}})
	c.AddCommand(&cobra.Command{Use: "set key value", Args: cobra.ExactArgs(2), Short: "Change a setting", Long: `Change a setting of unibox's own configuration. The keys:

  default_env  the env a sandbox is created from when none is named
  stop_idle    whether a sandbox is stopped when its last session ends
  ssh_agent    whether sandboxes may use your SSH agent (sbx is set to match)
  editor_ssh   whether editors and apps reach sandboxes as ssh <sandbox>.sbx
               (sbx is set to match, its daemon restarted)
  remember_host_commands
               whether approving an env's host commands covers their later
               runs too, rather than being asked each time (sbx's setting)
  claude_remote_control
               whether Claude Code's /remote-control works in sandboxes, so
               claude.ai and its app can drive a session (sbx's setting)
  clipboard_image_paste
               whether agents paste images from this computer's clipboard
               with Ctrl+V, which lets the sandbox read images copied here
               (sbx's setting)`, Example: "  unibox config set default_env dev\n  unibox config set stop_idle false", RunE: func(cmd *cobra.Command, args []string) error {
		cfg := o.manager.Config
		key, value := args[0], args[1]
		switch key {
		case "remember_host_commands", "claude_remote_control", "clipboard_image_paste":
			v, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("%s must be true or false", key)
			}
			if err := o.manager.SBX.SetSetting(cmd.Context(), unibox.SBXSettings[key], strconv.FormatBool(v)); err != nil {
				return err
			}
			*cfg.SBXSetting(key) = &v
		case "default_env":
			if _, ok := cfg.Envs[value]; !ok {
				return fmt.Errorf("there is no env %s; unibox env ls lists them", value)
			}
			cfg.DefaultEnv = value
		case "editor_ssh":
			v, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("%s must be true or false", key)
			}
			if err := o.setEditorSSH(cmd, v); err != nil {
				return err
			}
			if v {
				return o.success("editors reach a sandbox as ssh <sandbox>.sbx")
			}
			return o.success("configuration updated")
		case "stop_idle", "ssh_agent":
			v, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("%s must be true or false", key)
			}
			if key == "ssh_agent" {
				if v {
					fmt.Fprintln(o.log, sshAgentRisk)
				}
				if err := o.setSSHAgent(cmd, v); err != nil {
					return err
				}
				return o.success("configuration updated")
			}
			cfg.StopIdleSandbox = &v
		default:
			return fmt.Errorf("unknown configuration key %q; the keys are %s", key, configKeys)
		}
		if err := o.paths.SaveConfig(cfg); err != nil {
			return err
		}
		return o.success("configuration updated")
	}})
	c.AddCommand(&cobra.Command{Use: "unset key", Args: cobra.ExactArgs(1), Short: "Forget a setting, back to its default", RunE: func(cmd *cobra.Command, args []string) error {
		cfg := o.manager.Config
		switch args[0] {
		case "remember_host_commands", "claude_remote_control", "clipboard_image_paste":
			if err := o.manager.SBX.SetSetting(cmd.Context(), unibox.SBXSettings[args[0]], ""); err != nil {
				return err
			}
			*cfg.SBXSetting(args[0]) = nil
		case "default_env":
			cfg.DefaultEnv = ""
		case "stop_idle":
			cfg.StopIdleSandbox = nil
		case "ssh_agent":
			// Unsaid, a forwarding sbx is refused; so it is turned off.
			if err := o.setSSHAgent(cmd, false); err != nil {
				return err
			}
			cfg = o.manager.Config
			cfg.SSHAgentOn = nil
		case "editor_ssh":
			if err := o.setEditorSSH(cmd, false); err != nil {
				return err
			}
			cfg = o.manager.Config
			cfg.EditorSSHOn = nil
		default:
			return fmt.Errorf("unknown configuration key %q; the keys are %s", args[0], configKeys)
		}
		if err := o.paths.SaveConfig(cfg); err != nil {
			return err
		}
		return o.success("configuration updated")
	}})
	return c
}

const configKeys = "default_env, stop_idle, ssh_agent, editor_ssh, remember_host_commands, claude_remote_control and clipboard_image_paste"

func configValue(c unibox.Config, key string) (any, error) {
	switch key {
	case "default_env":
		return c.DefaultEnv, nil
	case "envs":
		return c.Envs, nil
	case "stop_idle":
		return c.StopIdle(), nil
	case "ssh_agent":
		on, _ := c.SSHAgent()
		return on, nil
	case "editor_ssh":
		on, _ := c.EditorSSH()
		return on, nil
	}
	return nil, fmt.Errorf("unknown configuration key %q", key)
}

func (o *options) listCommand() *cobra.Command {
	return &cobra.Command{Use: "ls", Args: cobra.NoArgs, Short: "List unibox's sandboxes and their state in sbx", RunE: func(cmd *cobra.Command, args []string) error {
		var boxes []api.Sandbox
		err := o.waiting("reading the sandboxes", func() (err error) {
			boxes, err = o.manager.List(cmd.Context())
			return err
		})
		if err != nil {
			return err
		}
		if o.json {
			return o.write(map[string]any{"api_version": api.Version, "sandboxes": boxes})
		}
		w := tabwriter.NewWriter(o.output, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "SANDBOX\tENV\tAGENT\tSTATE\tTASKS\tSESSIONS")
		for _, b := range boxes {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%d\n", b.Name, b.Env, cmp.Or(b.Agent, "-"), b.State, len(b.Tasks), len(b.Runs))
		}
		return w.Flush()
	}}
}
func (o *options) tasksCommand() *cobra.Command {
	var sandbox string
	c := &cobra.Command{Use: "ls", Args: cobra.NoArgs, Short: "List prepared tasks, in every sandbox or with -s in one", RunE: func(cmd *cobra.Command, args []string) error {
		tasks, runs, _, err := o.allTasks(cmd, sandbox)
		if err != nil {
			return err
		}
		if o.json {
			return o.write(map[string]any{"api_version": api.Version, "tasks": tasks, "runs": runs})
		}
		w := tabwriter.NewWriter(o.output, 0, 4, 2, ' ', 0)
		// The alias column shows only once a task has one.
		aliased := slices.ContainsFunc(tasks, func(t api.Task) bool { return t.Alias != "" })
		if aliased {
			fmt.Fprint(w, "TASK\tALIAS\t")
		} else {
			fmt.Fprint(w, "TASK\t")
		}
		fmt.Fprintln(w, "SANDBOX\tAGENT\tSTATE\tSESSIONS\tREPOSITORIES")
		for _, t := range tasks {
			sessions := 0
			for _, r := range runs {
				if r.Sandbox == t.Sandbox && r.Task == t.ID {
					sessions++
				}
			}
			repos := []string{}
			for _, r := range t.Repositories {
				kind, where := repoWhere(r)
				repos = append(repos, r.Name+"@"+cmp.Or(r.Branch, "default")+" "+kind+": "+where)
			}
			fmt.Fprintf(w, "%s\t", t.ID)
			if aliased {
				fmt.Fprintf(w, "%s\t", cmp.Or(t.Alias, "-"))
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n", t.Sandbox, t.Agent, t.State, sessions, strings.Join(repos, ", "))
		}
		if err = w.Flush(); err != nil {
			return err
		}
		if len(tasks) > 0 {
			fmt.Fprintln(o.log, "resume one with: unibox resume <task>")
		}
		return nil
	}}
	// Every sandbox's tasks unless told, whatever the context says.
	c.Flags().StringVarP(&sandbox, "sandbox", "s", "", "Only this sandbox's tasks")
	return c
}

// allTasks reads the tasks of every sandbox, or of one, with their
// sessions and the agents each task's sandbox runs.
func (o *options) allTasks(cmd *cobra.Command, sandbox string) ([]api.Task, []api.Run, []taskRow, error) {
	var boxes []api.Sandbox
	err := o.waiting("reading the tasks", func() (err error) {
		boxes, err = o.manager.List(cmd.Context())
		return err
	})
	if err != nil {
		return nil, nil, nil, err
	}
	tasks, runs, rows := []api.Task{}, []api.Run{}, []taskRow{}
	found := false
	for _, b := range boxes {
		if sandbox != "" && b.Name != sandbox {
			continue
		}
		found = true
		sorted := slices.Clone(b.Tasks)
		slices.SortFunc(sorted, func(x, y api.Task) int { return strings.Compare(x.ID, y.ID) })
		tasks = append(tasks, sorted...)
		runs = append(runs, b.Runs...)
		agents := slices.Sorted(maps.Keys(b.Agents))
		for _, t := range sorted {
			row := taskRow{Task: t, agents: agents}
			for _, r := range b.Runs {
				if r.Task == t.ID {
					row.sessions++
				}
			}
			rows = append(rows, row)
		}
	}
	if sandbox != "" && !found {
		return nil, nil, nil, fmt.Errorf("sandbox %s not found; unibox sandbox ls lists them", sandbox)
	}
	return tasks, runs, rows, nil
}

// tasksUI lets a person pick a task at a terminal and resume it, open a
// shell in it, name or remove it. Naming and removing come back to the
// list; resuming takes the terminal and ends it.
func (o *options) tasksUI(cmd *cobra.Command, sandbox string) error {
	ctx := cmd.Context()
	for {
		_, _, rows, err := o.allTasks(cmd, sandbox)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			fmt.Fprintln(o.log, "no tasks yet; unibox run <directory> prepares one")
			return nil
		}
		lipgloss.HasDarkBackground()
		out, err := tea.NewProgram(tasksModel{rows: rows, context: o.contextCorner()}, tea.WithInput(o.input), tea.WithOutput(o.log), tea.WithContext(ctx)).Run()
		if err != nil {
			return err
		}
		chosen := out.(tasksModel).chosen
		if chosen == nil {
			// Left as it is, it is said as task ls says it, as unibox port
			// leaves its ports.
			args := []string{"task", "ls"}
			if sandbox != "" {
				args = append(args, "-s", sandbox)
			}
			return o.runStep(cmd, args...)
		}
		t := chosen.task
		switch chosen.kind {
		case "resume", "shell":
			req := unibox.Request{Task: t.ID, Agent: chosen.agent, Shell: chosen.kind == "shell"}
			if err := o.preflight(cmd, &req); err != nil {
				return err
			}
			return o.manager.Run(ctx, req, o.input, o.output, o.log, nil)
		case "repos":
			if err := o.addTaskRepos(cmd, t); err != nil && !errors.Is(err, errCancelled) {
				fmt.Fprintln(o.log, err)
			}
		case "new":
			// A new task in the sandbox of the one under the cursor, picked
			// as unibox task new picks it; it takes the terminal.
			err := o.runStep(cmd, "task", "new", "-s", t.Sandbox)
			if errors.Is(err, errCancelled) {
				continue
			}
			return err
		case "rename":
			alias, err := o.askText("Name of task "+t.ID, "Its ID takes the alias away", cmp.Or(t.Alias, t.ID), nil)
			if err != nil {
				return err
			}
			if _, err := o.manager.AliasTask(t.ID, alias); err != nil {
				fmt.Fprintln(o.log, err)
			}
		case "remove":
			if err := o.manager.Remove(ctx, t.Sandbox, t.ID, false); err != nil {
				fmt.Fprintln(o.log, err)
			} else {
				fmt.Fprintf(o.log, "removed task %s\n", t.ID)
			}
		}
	}
}

func (o *options) resumeCommand() *cobra.Command {
	var req unibox.Request
	c := &cobra.Command{Use: "resume [task] [-- agent-args...]", Args: cobra.ArbitraryArgs, Short: "Run a prepared task's agent again, or a shell in it, in the sandbox that holds it", Long: `Run a prepared task's agent again, or a shell in it, in the sandbox that
holds it. Without a task at a terminal, it lists the tasks to pick one.

claude and codex continue the last conversation they had in the task, when
they had one there; --new starts them afresh. Other agents start afresh.`, RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 || cmd.ArgsLenAtDash() == 0 {
			if o.json || !o.interactive {
				return fmt.Errorf("name the task to resume; unibox task ls lists them")
			}
			return o.tasksUI(cmd, "")
		}
		if n := cmd.ArgsLenAtDash(); n > 1 || (n < 0 && len(args) > 1) {
			return fmt.Errorf("resume takes one task; agent arguments go after --")
		}
		if o.json {
			return fmt.Errorf("resume owns the terminal; omit --json")
		}
		req.Task = args[0]
		if err := o.preflight(cmd, &req); err != nil {
			return err
		}
		return o.manager.Run(cmd.Context(), req, o.input, o.output, o.log, args[1:])
	}}
	shellFlag(c, &req)
	rmFlag(c, &req.Remove)
	return c
}

func (o *options) shellCommand() *cobra.Command {
	return &cobra.Command{Use: "shell [sandbox]", Args: cobra.MaximumNArgs(1), Short: "Open an interactive shell (counts as an active session)", RunE: func(cmd *cobra.Command, args []string) error {
		if o.json {
			return fmt.Errorf("shell is interactive; omit --json")
		}
		name, err := o.sandboxName(first(args))
		if err != nil {
			return err
		}
		return o.manager.Shell(cmd.Context(), name, o.input, o.output, o.log)
	}}
}
func (o *options) stopCommand() *cobra.Command {
	return &cobra.Command{Use: "stop [sandbox]", Args: cobra.MaximumNArgs(1), Short: "Stop an idle sandbox without deleting its disk", RunE: func(cmd *cobra.Command, args []string) error {
		name, err := o.sandboxName(first(args))
		if err != nil {
			return err
		}
		if err := o.manager.Stop(cmd.Context(), name); err != nil {
			return err
		}
		return o.success("sandbox stopped")
	}}
}
func (o *options) recreateCommand() *cobra.Command {
	return &cobra.Command{Use: "recreate [sandbox]", Args: cobra.MaximumNArgs(1), Short: "Create a sandbox again from its env as it is now, keeping its tasks", Long: `Remove a sandbox and create it again from its env's files and your args as
they are now, after checking for running sessions and unpushed work. Its
tasks come back: mounted directories are mounted again, clones are made
again from the forge. What the sandbox held outside its tasks is lost.`, RunE: func(cmd *cobra.Command, args []string) error {
		name, err := o.sandboxName(first(args))
		if err != nil {
			return err
		}
		if err := o.confirm("Recreate sandbox " + name + " and discard what it holds outside its tasks?"); err != nil {
			return err
		}
		// Asked for by name with --yes, sbx's plan is approved as well.
		o.manager.ApproveEnv = o.yes
		if err := o.manager.Recreate(cmd.Context(), name); err != nil {
			return err
		}
		return o.success("sandbox recreated")
	}}
}
func (o *options) removeCommand(task bool) *cobra.Command {
	use, short := "rm sandbox", "Remove a sandbox, after checking for running sessions and unpushed work"
	if task {
		use, short = "rm task", "Remove a task from its sandbox, after checking for running sessions and unpushed work"
	}
	var force bool
	// What goes is always named, never taken from the context.
	c := &cobra.Command{Use: use, Args: cobra.ExactArgs(1), Short: short, RunE: func(cmd *cobra.Command, args []string) error {
		sandbox, id := args[0], ""
		if task {
			// A task's name is unique across sandboxes, so it finds its own.
			name, err := o.manager.TaskSandbox(args[0])
			if err != nil {
				return err
			}
			sandbox, id = name, o.manager.TaskID(args[0])
		}
		what := "sandbox " + sandbox
		if id != "" {
			what = "task " + id + " from sandbox " + sandbox
		}
		if err := o.confirm("Remove " + what + "?"); err != nil {
			return err
		}
		if err := o.manager.Remove(cmd.Context(), sandbox, id, force); err != nil {
			return err
		}
		if id == "" {
			if err := o.forgetContext(sandbox); err != nil {
				return err
			}
		}
		return o.success("removed")
	}}
	if !task {
		c.Flags().BoolVar(&force, "force", false, "Remove it with its running sessions and unpushed work, or one unibox adopted")
	}
	return c
}

// renameCommand gives a task a name of the user's. It is an alias: the
// task keeps its ID and directory, so nothing in a running session moves.
func (o *options) renameCommand() *cobra.Command {
	var reset bool
	c := &cobra.Command{Use: "rename task [name]", Args: cobra.RangeArgs(1, 2), Short: "Give a task a name taken wherever its ID is; --reset takes it away", RunE: func(cmd *cobra.Command, args []string) error {
		if reset == (len(args) == 2) {
			return fmt.Errorf("give the task a name, or --reset to take its alias away")
		}
		alias := o.manager.TaskID(args[0])
		if !reset {
			alias = args[1]
		}
		t, err := o.manager.AliasTask(args[0], alias)
		if err != nil {
			return err
		}
		if o.json {
			return o.write(map[string]any{"api_version": api.Version, "task": t})
		}
		if t.Alias == "" {
			return o.success("task " + t.ID + " has no alias")
		}
		return o.success("task " + t.ID + " is also " + t.Alias)
	}}
	c.Flags().BoolVar(&reset, "reset", false, "Take the alias away; the task is its ID alone")
	return c
}

// pruneCommand tidies what was left behind outside unibox: sandboxes
// removed with sbx itself, and directories still mounted for a task that
// is gone.
func (o *options) pruneCommand() *cobra.Command {
	return &cobra.Command{Use: "prune", Args: cobra.NoArgs, Short: "Forget the sandboxes sbx no longer has, and unmount what no task uses", RunE: func(cmd *cobra.Command, args []string) error {
		removed, err := o.manager.Prune(cmd.Context())
		if err != nil {
			return err
		}
		unmounted, busy, err := o.manager.PruneMounts(cmd.Context())
		if err != nil {
			return err
		}
		for _, name := range removed {
			if err := o.forgetContext(name); err != nil {
				return err
			}
		}
		if o.json {
			if unmounted == nil {
				unmounted = []string{}
			}
			if busy == nil {
				busy = []string{}
			}
			return o.write(map[string]any{"api_version": api.Version, "forgotten": removed, "unmounted": unmounted, "busy": busy})
		}
		if len(removed) > 0 {
			fmt.Fprintf(o.output, "forgot %s\n", strings.Join(removed, ", "))
		}
		for _, m := range unmounted {
			fmt.Fprintf(o.output, "unmounted %s\n", m)
		}
		if len(busy) > 0 {
			fmt.Fprintf(o.output, "left %s alone while in use; prune again later\n", strings.Join(busy, ", "))
		}
		if len(removed)+len(unmounted)+len(busy) == 0 {
			fmt.Fprintln(o.output, "nothing to prune")
		}
		return nil
	}}
}

// versionCommand prints the release unibox was built from, as --version
// does; with --json also the JSON contract's version it speaks, for a
// program to check before it drives unibox.
func (o *options) versionCommand() *cobra.Command {
	return &cobra.Command{Use: "version", Args: cobra.NoArgs, Short: "Print the release unibox was built from (with --json, the API version too)", RunE: func(cmd *cobra.Command, args []string) error {
		if o.json {
			return o.write(map[string]any{"api_version": api.Version, "version": version.String()})
		}
		_, err := fmt.Fprintln(o.output, version.String())
		return err
	}}
}
