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
	c := &cobra.Command{Use: "destroy", Args: cobra.NoArgs, Short: "Remove every sandbox and task; with --all, everything unibox made on this host", Long: `Remove every sandbox, and so every task, after checking each for running
sessions and unpushed work: one that has either stops it, and nothing is
removed, unless --force discards them. A mounted directory of the host is
never deleted.

--all goes on to what unibox made outside its sandboxes: its images, the
secrets and approvals it gave sbx (what you made with sbx stays), the herdr
plugin, and its state, configuration and your vars.toml. The environments
repository stays where it is; only the configuration naming it goes.
--secrets removes the tokens unibox keeps in the platform's store too, and
--msb the microsandbox runtime in ~/.microsandbox.`, RunE: func(cmd *cobra.Command, args []string) error {
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
			what = append(what, "unibox's images, its sbx secrets and approvals, the herdr plugin, its state, configuration and your vars.toml")
		}
		if opts.Secrets {
			what = append(what, "the tokens in the platform's store")
		}
		if opts.MSB {
			what = append(what, "~/.microsandbox")
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
			return o.success("unibox is as if never used here; the environments repository stays, unibox init --dir <path> takes it up again")
		}
		if err := o.forgetContext(o.manager.Config.Context.Sandbox); err != nil {
			return err
		}
		return o.success("every sandbox and task removed")
	}}
	f := c.Flags()
	f.BoolVar(&opts.All, "all", false, "Also everything else unibox made on this host: images, sbx secrets and approvals, the herdr plugin, its state, configuration and vars.toml")
	f.BoolVar(&opts.Secrets, "secrets", false, "Also the tokens unibox keeps in the platform's store (unibox secret ls)")
	f.BoolVar(&opts.MSB, "msb", false, "Also the microsandbox runtime in ~/.microsandbox")
	f.BoolVar(&opts.Force, "force", false, "Remove sandboxes with running sessions or unpushed work, discarding it")
	return c
}
