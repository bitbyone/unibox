package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/tobola/unibox/internal/unibox"
)

const sshAgentRisk = "An agent then signs with every key in your SSH agent: it can push to every repository and reach every server your keys reach, as you, beyond the services the profile names. It never sees the keys, and cannot add or remove them."

// setSSHAgent records whether sandboxes may use the user's SSH agent, and
// has sbx forward it or not to match, restarting its daemon if it must.
func (o *options) setSSHAgent(cmd *cobra.Command, on bool) error {
	cfg := o.manager.Config
	r := cfg.Runtimes["sbx"]
	r.SSHAgent = &on
	if cfg.Runtimes == nil {
		cfg.Runtimes = map[string]unibox.RuntimeConfig{}
	}
	cfg.Runtimes["sbx"] = r
	if err := o.paths.SaveConfig(cfg); err != nil {
		return err
	}
	o.manager.Config = cfg
	sbx, ok := o.manager.Runtimes["sbx"].(*unibox.SBX)
	if !ok || !sbx.Present() {
		return nil
	}
	sbx.SSHAgent = on
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

// askSSHAgent is the question init asks once, where sbx is the runtime:
// whether sandboxes may use the user's SSH agent.
func (o *options) askSSHAgent(cmd *cobra.Command) error {
	if _, said := o.manager.Config.SSHAgent("sbx"); said || o.manager.DefaultRuntime() != "sbx" {
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
