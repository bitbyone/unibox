package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tobola/unibox/internal/unibox"
)

// destroyCommand takes unibox back to before it was used, as far as asked.
func (o *options) destroyCommand() *cobra.Command {
	var opts unibox.DestroyOptions
	c := &cobra.Command{Use: "destroy", Args: cobra.NoArgs, Short: "Remove every sandbox and task; with --all, everything unibox keeps on this host", Long: `Remove every sandbox unibox created, and so every task, after checking each
for running sessions and unpushed work: one that has either stops it, and
nothing is removed, unless --force discards them. A mounted directory of
the host is never deleted, and a sandbox unibox adopted is only forgotten.

--all goes on to the herdr plugin and unibox's state, configuration and
your args. The env files, the kits and your logins in sbx stay: they are
the team's and sbx's.`, RunE: func(cmd *cobra.Command, args []string) error {
		s, err := o.paths.ReadState()
		if err != nil {
			return err
		}
		tasks := 0
		for _, b := range s.Sandboxes {
			tasks += len(b.Tasks)
		}
		what := []string{fmt.Sprintf("%d sandboxes and their %d tasks", len(s.Sandboxes), tasks)}
		if opts.All {
			what = append(what, "the herdr plugin, unibox's state, configuration and your args")
		}
		question := "Remove " + strings.Join(what, "; ")
		if opts.Force {
			question += ", discarding running sessions and unpushed work"
		}
		if err := o.confirm(question + "?"); err != nil {
			return err
		}
		if err := o.manager.Destroy(cmd.Context(), opts); err != nil {
			return err
		}
		if opts.All {
			return o.success("unibox is as if never used here")
		}
		if err := o.forgetContext(o.manager.Config.Context.Sandbox); err != nil {
			return err
		}
		return o.success("every sandbox and task removed")
	}}
	f := c.Flags()
	f.BoolVar(&opts.All, "all", false, "Also the herdr plugin and unibox's state, configuration and your args")
	f.BoolVar(&opts.Force, "force", false, "Remove sandboxes with running sessions or unpushed work, discarding it")
	return c
}
