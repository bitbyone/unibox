package cli

import (
	"strconv"

	"github.com/tobola/unibox/internal/unibox"

	"fmt"
	"os"
	"strings"
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

// settingQuestion is what init asks of one of the person's settings of
// the whole machine: a yes and a no, preset as unibox recommends.
type settingQuestion struct {
	key, title, description string
	picks                   []pick
	preset                  string
}

// askSetting asks one of the person's settings of the whole machine once,
// unless config.yaml says it already. The team's value, from the
// settings: of its unibox.yaml files, is taken without a question while
// the person's is unsaid, and where theirs differs it is asked again, the
// team's first.
func (o *options) askSetting(cmd *cobra.Command, team map[string]bool, q settingQuestion) error {
	said := *o.manager.Config.Setting(q.key)
	recommended, has := team[q.key]
	switch {
	case has && said == nil:
		fmt.Fprintf(o.log, "  %s: %s, as the team says\n", q.title, yesNo(recommended))
		return o.setSetting(cmd, q.key, recommended)
	case has && *said == recommended, !has && said != nil:
		return nil
	case has:
		q.picks = teamFirst(q.picks, yesNo(recommended))
		q.preset = yesNo(recommended)
		q.description += fmt.Sprintf(" You said %s; the team recommends %s.", yesNo(*said), yesNo(recommended))
	case q.key == "ssh_sandbox":
		// sbx's own setting, for every sandbox: one sbx has on already was
		// decided and is only recorded.
		if sbx := o.manager.SBX; sbx.Present() {
			if on, err := sbx.EditorSSH(cmd.Context()); err == nil && on {
				cfg := o.manager.Config
				cfg.EditorSSHOn = &on
				if err := o.paths.SaveConfig(cfg); err != nil {
					return err
				}
				o.manager.Config = cfg
				// sbx serving SSH says nothing of the SSH client's own
				// configuration, which only sbx setup ssh writes.
				return sbx.SetupSSHClient(cmd.Context())
			}
		}
	}
	choice, err := o.askPick(q.title, q.description, q.picks, q.preset)
	if err != nil {
		return err
	}
	v := choice == "yes"
	if said != nil && *said == v {
		return nil
	}
	return o.setSetting(cmd, q.key, v)
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

// teamFirst puts the team's answer first and calls it the recommended
// one, over unibox's own recommendation.
func teamFirst(picks []pick, team string) []pick {
	out := []pick{}
	for _, p := range picks {
		p.label = strings.TrimSuffix(p.label, " (recommended)")
		if p.value == team {
			p.label += " (recommended)"
			out = append([]pick{p}, out...)
		} else {
			out = append(out, p)
		}
	}
	return out
}

// setSetting keeps one of the person's settings of the whole machine in
// config.yaml and sets sbx to match.
func (o *options) setSetting(cmd *cobra.Command, key string, v bool) error {
	switch key {
	case "ssh_agent":
		return o.setSSHAgent(cmd, v)
	case "ssh_sandbox":
		return o.setEditorSSH(cmd, v)
	}
	if err := o.manager.SBX.SetSetting(cmd.Context(), unibox.SBXSettings[key], strconv.FormatBool(v)); err != nil {
		return err
	}
	cfg := o.manager.Config
	*cfg.SBXSetting(key) = &v
	if err := o.paths.SaveConfig(cfg); err != nil {
		return err
	}
	o.manager.Config = cfg
	return nil
}
