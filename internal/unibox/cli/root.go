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
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

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
	// secretsVia are the person's own commands the profile at hand reads
	// its secrets by, by name, so none of them is asked for into the
	// platform's store.
	secretsVia func(name string) string
	// guided says the user asked to be walked through (-i): questions with
	// choices, pages opened in the browser, an agent's own sign-in run.
	guided bool
	// inProfile writes a token's command into the profile, versioned with
	// the repository, rather than into the person's vars.toml.
	inProfile bool
	// wiz is the walk of an interactive init under way.
	wiz *wizard
	// setupForRun says an init was started by a run with nothing set up:
	// it leaves out its own Run step, and setupProfile is what it made,
	// for the run to go on with; setupRepository is where the
	// environments repository goes by default then, as here is the
	// repository the run is for.
	setupForRun     bool
	setupProfile    string
	setupRepository string
}

func New(paths unibox.Paths, in io.Reader, out, errout io.Writer) *cobra.Command {
	o := &options{paths: paths, input: in, output: out, log: errout}
	if f, ok := in.(*os.File); ok {
		o.interactive = term.IsTerminal(int(f.Fd()))
	}
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
		o.manager = m
		return m.RefreshPlugin()
	}
	// What a command works on is a word of a group (unibox task rm); only
	// the everyday ones stand alone.
	root.AddCommand(o.taskCommand("run"), o.resumeCommand(), o.initCommand(), o.contextCommand(), o.configCommand(),
		o.taskGroup(), o.sandboxGroup(), o.profileGroup(), o.runtimeGroup(),
		o.instructionsCommand(), o.agentCommand(), o.serviceCommand(), o.networkCommand(), o.scriptCommand(),
		o.secretCommand(), o.varCommand(), o.templateCommand(), o.herdrCommand(), o.doctorCommand(), o.pruneCommand(), o.destroyCommand(), o.versionCommand())
	// unibox runs itself as this under the shell that fetched a running
	// msb sandbox's tokens, to hand them to msb; it is no command for people.
	root.AddCommand(&cobra.Command{Use: unibox.MSBSecretsCommand + " sandbox spec...", Hidden: true, Args: cobra.MinimumNArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		return unibox.RotateMSBSecrets(cmd.Context(), args[0], args[1:])
	}})
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
		if len(args) == 1 {
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
		default:
			b, _ := json.MarshalIndent(v, "", "  ")
			fmt.Fprintln(o.output, string(b))
		}
		return nil
	}})
	c.AddCommand(&cobra.Command{Use: "set key value", Args: cobra.ExactArgs(2), Short: "Change a setting", Long: `Change a setting of unibox's own configuration. The keys:

  environments              the environments repository's path
  default_runtime           the runtime new sandboxes get: sbx, or the experimental msb
  runtimes.<name>.on        whether a runtime may be used (true, false)
  runtimes.<name>.stop_idle whether a sandbox is stopped when its last session ends
  runtimes.sbx.ssh_agent    whether sandboxes may use your SSH agent (sbx is set to match)

A sandbox keeps the runtime it was made in; --runtime asks for another one
for a new sandbox.`, Example: "  unibox config set default_runtime sbx\n  unibox config set runtimes.sbx.stop_idle false", RunE: func(cmd *cobra.Command, args []string) error {
		cfg := o.manager.Config
		key, value := args[0], args[1]
		switch key {
		case "environments":
			path, err := unibox.ExpandPath(value)
			if err != nil {
				return err
			}
			cfg.Environments = path
		case "default_runtime":
			if _, ok := o.manager.Runtimes[value]; !ok {
				return fmt.Errorf("unknown runtime %s; unibox runtime ls lists them", value)
			}
			cfg.DefaultRuntime = value
		default:
			parts := strings.Split(key, ".")
			if len(parts) != 3 || parts[0] != "runtimes" {
				return fmt.Errorf("unknown configuration key %q", key)
			}
			if _, ok := o.manager.Runtimes[parts[1]]; !ok {
				return fmt.Errorf("unknown runtime %s", parts[1])
			}
			v, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("%s must be true or false", key)
			}
			if parts[2] == "ssh_agent" {
				if parts[1] != "sbx" {
					return fmt.Errorf("ssh_agent is sbx's; %s does not forward an SSH agent", parts[1])
				}
				if v {
					fmt.Fprintln(o.log, sshAgentRisk)
				}
				if err := o.setSSHAgent(cmd, v); err != nil {
					return err
				}
				return o.success("configuration updated")
			}
			r := cfg.Runtimes[parts[1]]
			switch parts[2] {
			case "on":
				r.On = &v
			case "stop_idle":
				r.StopIdle = &v
			default:
				return fmt.Errorf("unknown configuration key %q", key)
			}
			cfg.Runtimes[parts[1]] = r
		}
		if err := o.paths.SaveConfig(cfg); err != nil {
			return err
		}
		return o.success("configuration updated")
	}})
	c.AddCommand(&cobra.Command{Use: "unset key", Args: cobra.ExactArgs(1), Short: "Forget a setting, back to its default", RunE: func(cmd *cobra.Command, args []string) error {
		cfg := o.manager.Config
		key := args[0]
		switch key {
		case "environments":
			// The repository itself stays; unibox only stops using it.
			cfg.Environments = ""
		case "default_runtime":
			cfg.DefaultRuntime = ""
		default:
			parts := strings.Split(key, ".")
			if len(parts) != 3 || parts[0] != "runtimes" {
				return fmt.Errorf("unknown configuration key %q", key)
			}
			if parts[2] == "ssh_agent" && parts[1] == "sbx" {
				// Unsaid, a forwarding sbx is refused; so it is turned off.
				if err := o.setSSHAgent(cmd, false); err != nil {
					return err
				}
				cfg = o.manager.Config
				r := cfg.Runtimes["sbx"]
				r.SSHAgent = nil
				cfg.Runtimes["sbx"] = r
				if err := o.paths.SaveConfig(cfg); err != nil {
					return err
				}
				return o.success("configuration updated")
			}
			r, ok := cfg.Runtimes[parts[1]]
			if !ok {
				return o.success("configuration updated")
			}
			switch parts[2] {
			case "on":
				r.On = nil
			case "stop_idle":
				r.StopIdle = nil
			default:
				return fmt.Errorf("unknown configuration key %q", key)
			}
			cfg.Runtimes[parts[1]] = r
		}
		if err := o.paths.SaveConfig(cfg); err != nil {
			return err
		}
		return o.success("configuration updated")
	}})
	return c
}
func configValue(c unibox.Config, key string) (any, error) {
	switch key {
	case "environments":
		return c.Environments, nil
	case "default_runtime":
		return c.DefaultRuntime, nil
	case "runtimes":
		return c.Runtimes, nil
	}
	p := strings.Split(key, ".")
	if len(p) == 3 && p[0] == "runtimes" {
		switch p[2] {
		case "on":
			return c.Enabled(p[1]), nil
		case "stop_idle":
			return c.StopIdle(p[1]), nil
		case "ssh_agent":
			on, _ := c.SSHAgent(p[1])
			return on, nil
		}
	}
	return nil, fmt.Errorf("unknown configuration key %q", key)
}

func (o *options) profilesCommand() *cobra.Command {
	return &cobra.Command{Use: "ls", Args: cobra.NoArgs, Short: "List profiles, agents and builds", RunE: func(cmd *cobra.Command, args []string) error {
		// Without a repository there is nothing to list; say how to start.
		if _, err := o.manager.Environments(); err != nil {
			if o.json {
				return o.write(map[string]any{"api_version": api.Version, "profiles": []api.Profile{}, "error": err.Error()})
			}
			fmt.Fprintln(o.log, err)
			return nil
		}
		rows, err := o.manager.Profiles(cmd.Context())
		if err != nil {
			return err
		}
		if o.json {
			return o.write(map[string]any{"api_version": api.Version, "profiles": rows})
		}
		w := tabwriter.NewWriter(o.output, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "PROFILE\tAGENTS\tIMAGE\tERROR")
		for _, p := range rows {
			fmt.Fprintf(w, "%s\t%s\t%v\t%s\n", p.Name, strings.Join(p.Agents, ","), p.Images, p.Error)
		}
		return w.Flush()
	}}
}
func (o *options) runtimesCommand() *cobra.Command {
	return &cobra.Command{Use: "ls", Args: cobra.NoArgs, Short: "Show the runtimes, which is the default, and whether each can be used", RunE: func(cmd *cobra.Command, args []string) error {
		rows := []map[string]any{}
		for name, r := range o.manager.Runtimes {
			reason := ""
			if err := r.Available(cmd.Context()); err != nil {
				reason = err.Error()
			}
			rows = append(rows, map[string]any{"name": name, "default": name == o.manager.DefaultRuntime(), "experimental": unibox.IsExperimental(r), "enabled": o.manager.Config.Enabled(name), "available": reason == "", "error": reason, "capabilities": r.Capabilities()})
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i]["name"].(string) > rows[j]["name"].(string) })
		if o.json {
			return o.write(map[string]any{"api_version": api.Version, "runtimes": rows})
		}
		w := tabwriter.NewWriter(o.output, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "RUNTIME\tSTATUS\tAVAILABLE\tISOLATION")
		for _, r := range rows {
			status := "supported"
			if r["experimental"].(bool) {
				status = "experimental"
			}
			if r["default"].(bool) {
				status += ", default"
			}
			if !r["enabled"].(bool) {
				status += ", off"
			}
			available := "yes"
			if e := r["error"].(string); e != "" {
				available = "no: " + e
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r["name"], status, available, r["capabilities"].(unibox.Capabilities).Isolation)
		}
		return w.Flush()
	}}
}
func (o *options) buildCommand() *cobra.Command {
	var rt string
	var rebuild bool
	c := &cobra.Command{Use: "build [profile]", Args: cobra.MaximumNArgs(1), Short: "Build, validate and load a profile image", RunE: func(cmd *cobra.Command, args []string) error {
		name, err := o.profileName(first(args))
		if err != nil {
			return err
		}
		if err := o.manager.Build(cmd.Context(), name, rt, rebuild); err != nil {
			return err
		}
		return o.success("image ready")
	}}
	c.Flags().StringVar(&rt, "runtime", "", "Runtime to build for")
	c.Flags().BoolVar(&rebuild, "rebuild", false, "Build again even when current")
	return c
}
func (o *options) diffCommand() *cobra.Command {
	var rt string
	c := &cobra.Command{Use: "diff [profile]", Args: cobra.MaximumNArgs(1), Short: "Show changes since the last build", RunE: func(cmd *cobra.Command, args []string) error {
		name, err := o.profileName(first(args))
		if err != nil {
			return err
		}
		return o.manager.Diff(cmd.Context(), name, rt, o.output)
	}}
	c.Flags().StringVar(&rt, "runtime", "", "Runtime of the previous build")
	return c
}

func (o *options) instructionsCommand() *cobra.Command {
	var sandbox, profile string
	c := &cobra.Command{Use: "instructions", Args: cobra.NoArgs, Short: "Read assembled instructions or attach named texts to a sandbox", Long: "Print the instructions an agent gets: a profile's, or with -s those of a sandbox - its profile's and the texts attached to it. unibox instructions attach <name>... -s <sandbox> sets what is attached.", RunE: func(cmd *cobra.Command, args []string) error {
		attached := []string{}
		if sandbox != "" && profile != "" {
			return fmt.Errorf("pass -p or -s, not both")
		}
		if sandbox != "" {
			boxes, err := o.manager.List(cmd.Context())
			if err != nil {
				return err
			}
			for _, b := range boxes {
				if b.Name == sandbox {
					profile = b.Profile
					attached = b.Instructions
				}
			}
			if profile == "" {
				return fmt.Errorf("sandbox %s not found; unibox sandbox ls lists them", sandbox)
			}
		} else {
			name, err := o.profileName(profile)
			if err != nil {
				return err
			}
			profile = name
		}
		p, err := unibox.LoadProfile(o.manager.Config.Environments, profile)
		if err != nil {
			return err
		}
		text, err := p.InstructionText(attached)
		if err != nil {
			return err
		}
		if o.json {
			return o.write(map[string]any{"api_version": api.Version, "instructions": text})
		}
		_, err = io.WriteString(o.output, text)
		return err
	}}
	c.Flags().StringVarP(&sandbox, "sandbox", "s", "", "Show the instructions as this sandbox's agents get them, its attached texts included")
	profileFlag(c, &profile)
	var target string
	attach := &cobra.Command{Use: "attach [instruction...]", Args: cobra.ArbitraryArgs, Short: "Replace the sandbox's list of attached instruction names (none clears it)", RunE: func(cmd *cobra.Command, args []string) error {
		name, err := o.sandboxName(target)
		if err != nil {
			return err
		}
		if err := o.manager.AttachInstructions(name, args); err != nil {
			return err
		}
		return o.success("instructions of sandbox " + name + " updated; they apply before the next session")
	}}
	sandboxFlag(attach, &target)
	c.AddCommand(attach)
	return c
}

func (o *options) listCommand() *cobra.Command {
	return &cobra.Command{Use: "ls", Args: cobra.NoArgs, Short: "List unibox's sandboxes and their runtime state", RunE: func(cmd *cobra.Command, args []string) error {
		boxes, err := o.manager.List(cmd.Context())
		if err != nil {
			return err
		}
		if _, err := o.manager.Environments(); err != nil && !o.json {
			fmt.Fprintln(o.log, err)
		}
		if o.json {
			return o.write(map[string]any{"api_version": api.Version, "sandboxes": boxes})
		}
		w := tabwriter.NewWriter(o.output, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "SANDBOX\tRUNTIME\tPROFILE\tSTATE\tTASKS\tSESSIONS")
		for _, b := range boxes {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%d\n", b.Name, b.Runtime, b.Profile, b.State, len(b.Tasks), len(b.Runs))
		}
		return w.Flush()
	}}
}
func (o *options) tasksCommand() *cobra.Command {
	var sandbox string
	c := &cobra.Command{Use: "ls", Args: cobra.NoArgs, Short: "List prepared tasks, in every sandbox or with -s in one", RunE: func(cmd *cobra.Command, args []string) error {
		boxes, err := o.manager.List(cmd.Context())
		if err != nil {
			return err
		}
		tasks, runs := []api.Task{}, []api.Run{}
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
		}
		if sandbox != "" && !found {
			return fmt.Errorf("sandbox %s not found; unibox sandbox ls lists them", sandbox)
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
				repos = append(repos, r.Name+"@"+r.Branch)
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

func (o *options) resumeCommand() *cobra.Command {
	var req unibox.Request
	c := &cobra.Command{Use: "resume task [-- agent-args...]", Args: cobra.MinimumNArgs(1), Short: "Run a prepared task's agent again, or a shell in it, in the sandbox that holds it", RunE: func(cmd *cobra.Command, args []string) error {
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
	c.Flags().StringVar(&req.Agent, "agent", "", "Coding agent to run instead of the task's own")
	scriptFlag(c, &req)
	shellFlag(c, &req)
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
	var rebuild bool
	c := &cobra.Command{Use: "recreate [sandbox]", Args: cobra.MaximumNArgs(1), Short: "Rebuild when needed and recreate an idle sandbox without discarding unpushed work", RunE: func(cmd *cobra.Command, args []string) error {
		name, err := o.sandboxName(first(args))
		if err != nil {
			return err
		}
		args = []string{name}
		if err := o.confirm("Recreate sandbox " + args[0] + " and discard its caches?"); err != nil {
			return err
		}
		// A login stored after the sandbox is made does not reach it (a
		// token's placeholder is set at creation), so ask for it first.
		if err := o.preflight(cmd, &unibox.Request{Sandbox: args[0]}); err != nil {
			return err
		}
		if err := o.manager.Recreate(cmd.Context(), args[0], rebuild); err != nil {
			return err
		}
		return o.success("sandbox recreated")
	}}
	c.Flags().BoolVar(&rebuild, "rebuild", false, "Force a fresh image build")
	return c
}
func (o *options) removeCommand(task bool) *cobra.Command {
	use, short := "rm sandbox", "Remove a sandbox, after checking for running sessions and unpushed work"
	if task {
		use, short = "rm task", "Remove a task from its sandbox, after checking for running sessions and unpushed work"
	}
	// What goes is always named, never taken from the context.
	return &cobra.Command{Use: use, Args: cobra.ExactArgs(1), Short: short, RunE: func(cmd *cobra.Command, args []string) error {
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
		if err := o.manager.Remove(cmd.Context(), sandbox, id); err != nil {
			return err
		}
		if id == "" {
			if err := o.forgetContext(sandbox); err != nil {
				return err
			}
		}
		return o.success("removed")
	}}
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

// pruneCommand removes images unibox no longer needs; each can be built or
// made again, so it asks nothing.
func (o *options) pruneCommand() *cobra.Command {
	return &cobra.Command{Use: "prune", Args: cobra.NoArgs, Short: "Remove the images unibox no longer needs: older builds, and those of profiles and sandboxes that are gone", RunE: func(cmd *cobra.Command, args []string) error {
		removed, err := o.manager.Prune(cmd.Context())
		if err != nil {
			return err
		}
		if o.json {
			if removed == nil {
				removed = []string{}
			}
			return o.write(map[string]any{"api_version": api.Version, "removed": removed})
		}
		if len(removed) == 0 {
			fmt.Fprintln(o.output, "nothing to remove")
			return nil
		}
		fmt.Fprintf(o.output, "removed %d images\n", len(removed))
		return nil
	}}
}

// versionCommand prints the release unibox was built from, as --version
// does; with --json also the profile format and the JSON contract's
// version it speaks, for a program to check before it drives unibox.
func (o *options) versionCommand() *cobra.Command {
	return &cobra.Command{Use: "version", Args: cobra.NoArgs, Short: "Print the release unibox was built from (with --json, the profile format and API version too)", RunE: func(cmd *cobra.Command, args []string) error {
		if o.json {
			return o.write(map[string]any{"api_version": api.Version, "version": version.String(), "profile_format": unibox.ProfileFormat})
		}
		_, err := fmt.Fprintln(o.output, version.String())
		return err
	}}
}
