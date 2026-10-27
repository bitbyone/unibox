package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const sshAgentRisk = "An agent then signs with every key in your SSH agent: it can push to every repository and reach every server your keys reach, as you, beyond the services the env names. It never sees the keys, and cannot add or remove them."

// setSSHAgent records whether sandboxes may use the user's SSH agent, and
// has sbx forward it or not to match, restarting its daemon if it must.
func (o *options) setSSHAgent(cmd *cobra.Command, on bool) error {
	cfg := o.manager.Config
	cfg.SSHAgentOn = &on
	if err := o.paths.SaveConfig(cfg); err != nil {
		return err
	}
	o.manager.Config = cfg
	sbx := o.manager.SBX
	if !sbx.Present() {
		return nil
	}
	sbx.AllowSSHAgent(on)
	now, err := sbx.SSHForwarding(cmd.Context())
	if err != nil || now == on {
		return err
	}
	verb := "off"
	if on {
		verb = "on"
	}
	fmt.Fprintf(o.log, "turning sbx's SSH agent forwarding %s; running sandboxes keep their connections until they restart\n", verb)
	return o.working("restarting the sbx daemon", func() error { return sbx.SetSSHForwarding(cmd.Context(), on) })
}

// working runs what takes a while, with a spinner at a terminal and a line
// without one.
func (o *options) working(what string, do func() error) error {
	if !o.interactive || o.json {
		fmt.Fprintln(o.log, what+"...")
		return do()
	}
	done := make(chan error, 1)
	go func() { done <- do() }()
	frames := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for i := 0; ; i++ {
		select {
		case err := <-done:
			mark := "✓"
			if err != nil {
				mark = "✗"
			}
			fmt.Fprintf(o.log, "\r%s %s\n", mark, what)
			return err
		case <-tick.C:
			fmt.Fprintf(o.log, "\r%c %s", frames[i%len(frames)], what)
		}
	}
}

// askSSHAgent is the question init asks once: whether sandboxes may use the user's SSH agent.
func (o *options) askSSHAgent(cmd *cobra.Command) error {
	if _, said := o.manager.Config.SSHAgent(); said {
		return nil
	}
	choice, err := o.askPick("SSH agent in sandboxes", "With it, git over SSH works in a sandbox with no token - a mounted checkout pushes as you do. "+sshAgentRisk, []pick{
		{"no", "No - agents reach git through the services' tokens alone (recommended)"},
		{"yes", "Yes - agents use my SSH agent"},
	}, "no")
	if err != nil {
		return err
	}
	return o.setSSHAgent(cmd, choice == "yes")
}

// setEditorSSH records whether editors may reach sandboxes over SSH, and
// has sbx serve it or not to match. Turning it on always restarts sbx's
// daemon, since one started before the flag was on serves no SSH.
func (o *options) setEditorSSH(cmd *cobra.Command, on bool) error {
	cfg := o.manager.Config
	cfg.EditorSSHOn = &on
	if err := o.paths.SaveConfig(cfg); err != nil {
		return err
	}
	o.manager.Config = cfg
	sbx := o.manager.SBX
	if !sbx.Present() {
		return nil
	}
	if !on {
		if now, err := sbx.EditorSSH(cmd.Context()); err != nil || !now {
			return err
		}
	}
	what := "letting editors reach sandboxes over SSH, restarting the sbx daemon"
	if !on {
		what = "keeping editors out of sandboxes, restarting the sbx daemon"
	}
	return o.working(what, func() error { return sbx.SetEditorSSH(cmd.Context(), on) })
}

// askEditorSSH is the question init asks once: whether editors and desktop
// apps may reach sandboxes over SSH.
func (o *options) askEditorSSH(cmd *cobra.Command) error {
	if _, said := o.manager.Config.EditorSSH(); said {
		return nil
	}
	choice, err := o.askPick("Editors over SSH", "VS Code, Cursor, Orca and other apps reach a sandbox as ssh <sandbox>.sbx. sbx serves it to your user alone, signed in to Docker, and adds the host entry to your SSH client's configuration. An editor's session is not unibox's: unibox config set stop_idle false keeps a sandbox running for it.", []pick{
		{"yes", "Yes - editors connect over SSH"},
		{"no", "No"},
	}, "yes")
	if err != nil {
		return err
	}
	return o.setEditorSSH(cmd, choice == "yes")
}

// waiting shows a spinner while do runs, as long as it takes more than a
// moment, and takes it away after: what follows, a list or a table, is
// what stays. Only on a terminal; anywhere else it would be noise in what
// a program reads.
func (o *options) waiting(what string, do func() error) error {
	f, ok := o.log.(*os.File)
	if !ok || o.json || !term.IsTerminal(int(f.Fd())) {
		return do()
	}
	done := make(chan error, 1)
	go func() { done <- do() }()
	select {
	case err := <-done:
		return err
	case <-time.After(150 * time.Millisecond):
	}
	frames := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for i := 0; ; i++ {
		fmt.Fprintf(f, "\r%c %s", frames[i%len(frames)], what)
		select {
		case err := <-done:
			fmt.Fprint(f, "\r\x1b[K")
			return err
		case <-tick.C:
		}
	}
}
