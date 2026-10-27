package cli

import (
	"cmp"
	"errors"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
)

// Each group is a noun and its words verbs: unibox task rm.

func (o *options) taskGroup() *cobra.Command {
	var sandbox string
	c := &cobra.Command{Use: "task", Args: cobra.NoArgs, Short: "Prepare, list, name and remove tasks; unibox run and resume work in them", Long: `Prepare, list, name and remove tasks; unibox run and resume work in them.
Alone at a terminal, unibox task lists the tasks to resume one, with its
agent, another or a shell, or to name or remove it, each by a key.`}
	c.Flags().StringVarP(&sandbox, "sandbox", "s", "", "Only this sandbox's tasks")
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if o.json || !o.interactive {
			if sandbox != "" {
				return o.runStep(cmd, "task", "ls", "-s", sandbox)
			}
			return o.runStep(cmd, "task", "ls")
		}
		return o.tasksUI(cmd, sandbox)
	}
	c.AddCommand(o.tasksCommand(), o.taskCommand("prepare"), o.taskCommand("check"), o.addRepoCommand(), o.renameCommand(), o.removeCommand(true))
	return c
}

func (o *options) sandboxGroup() *cobra.Command {
	c := &cobra.Command{Use: "sandbox", Args: cobra.NoArgs, Short: "Create, enter, stop, recreate and remove sandboxes", Long: `Create, enter, stop, recreate and remove sandboxes. Alone at a terminal,
unibox sandbox lists them to open one's tasks, a shell, its ports or what
unibox keeps of it, or to stop, recreate or remove it, each by a key.`}
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if o.json || !o.interactive {
			return o.runStep(cmd, "sandbox", "ls")
		}
		return o.sandboxesUI(cmd)
	}
	c.AddCommand(o.listCommand(), o.createCommand(), o.inspectCommand(), o.resourcesCommand(), o.shellCommand(), o.stopCommand(), o.recreateCommand(), o.removeCommand(false))
	return c
}

func first(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

// sandboxesUI lets a person act on a sandbox at a terminal. Stopping,
// recreating and removing come back to the list; what takes the terminal
// or prints ends it.
func (o *options) sandboxesUI(cmd *cobra.Command) error {
	for {
		var boxes []api.Sandbox
		err := o.waiting("reading the sandboxes", func() (err error) {
			boxes, err = o.manager.List(cmd.Context())
			return err
		})
		if err != nil {
			return err
		}
		if len(boxes) == 0 {
			fmt.Fprintln(o.log, "no sandboxes yet; unibox sandbox create makes one, unibox run one with a task")
			return nil
		}
		lipgloss.HasDarkBackground()
		out, err := tea.NewProgram(sandboxesModel{rows: boxes}, tea.WithInput(o.input), tea.WithOutput(o.log), tea.WithContext(cmd.Context())).Run()
		if err != nil {
			return err
		}
		chosen := out.(sandboxesModel).chosen
		if chosen == nil {
			return o.runStep(cmd, "sandbox", "ls")
		}
		name := chosen.sandbox
		switch chosen.kind {
		case "tasks":
			return o.tasksUI(cmd, name)
		case "ports":
			return o.portsUI(cmd, name)
		case "resources":
			if err = o.askResources(cmd, chosen.box); err != nil && !errors.Is(err, errCancelled) {
				fmt.Fprintln(o.log, err)
			}
			continue
		case "shell", "inspect":
			return o.runStep(cmd, "sandbox", chosen.kind, name)
		case "stop", "recreate", "remove":
			// Asked in the list already; the command still refuses to
			// discard running sessions or unpushed work.
			verb := chosen.kind
			if verb == "remove" {
				verb = "rm"
			}
			yes := o.yes
			o.yes = true
			err = o.runStep(cmd, "sandbox", verb, name)
			o.yes = yes
			if err != nil {
				fmt.Fprintln(o.log, err)
			}
		}
	}
}

// askResources asks a sandbox's own CPUs and memory, sets them as unibox
// sandbox resources does, and offers to recreate it to apply them.
func (o *options) askResources(cmd *cobra.Command, b api.Sandbox) error {
	values, err := o.manager.Paths.LoadEnvValues()
	if err != nil {
		return err
	}
	own := values.SandboxResources[b.Name]
	env := o.envOf(b)
	args := []string{"sandbox", "resources", b.Name}
	for _, r := range unibox.ResourceNames {
		now := cmp.Or(values.ResourcesFor(env, b.Name).Get(r), "as the env's files say") + resourceFrom(values, env, b, r)
		hint := "now " + now + "; empty keeps it"
		if own.Get(r) != "" {
			hint += ", - gives back the env's"
		}
		v, err := o.askText(r+" for "+b.Name, hint, "", func(s string) error {
			if s == "" || s == "-" {
				return nil
			}
			return unibox.ValidResource(r, s)
		})
		if err != nil {
			return err
		}
		if v != "" && v != own.Get(r) && (v != "-" || own.Get(r) != "") {
			args = append(args, "--"+r, v)
		}
	}
	if len(args) == 3 {
		return nil
	}
	if err = o.runStep(cmd, args...); err != nil {
		return err
	}
	if b.State == "missing" {
		return nil
	}
	again, err := o.askYes("Recreate sandbox "+b.Name+" now, discarding what it holds outside its tasks?", false)
	if err != nil || !again {
		return err
	}
	yes := o.yes
	o.yes = true
	defer func() { o.yes = yes }()
	return o.runStep(cmd, "sandbox", "recreate", b.Name)
}
