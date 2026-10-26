package cli

import "github.com/spf13/cobra"

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
	c := &cobra.Command{Use: "sandbox", Short: "Create, enter, stop, recreate and remove sandboxes"}
	c.AddCommand(o.listCommand(), o.createCommand(), o.inspectCommand(), o.shellCommand(), o.stopCommand(), o.recreateCommand(), o.removeCommand(false))
	return c
}

func first(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}
