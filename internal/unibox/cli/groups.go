package cli

import "github.com/spf13/cobra"

// Each group is a noun and its words verbs: unibox task rm.

func (o *options) taskGroup() *cobra.Command {
	c := &cobra.Command{Use: "task", Short: "Prepare, list, name and remove tasks; unibox run and resume work in them"}
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
