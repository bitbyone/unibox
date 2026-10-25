package cli

import (
	"cmp"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
	"golang.org/x/term"
)

// doctorCheck is one thing unibox doctor looked at.
type doctorCheck struct {
	Area    string `json:"area"`
	Name    string `json:"name"`
	Status  string `json:"status"` // pass, warn or fail
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// doctorCommand checks what a run needs, before a run fails on it: sbx,
// each env's files, the person's args and logins, and the sandboxes unibox
// keeps.
func (o *options) doctorCommand() *cobra.Command {
	var only string
	c := &cobra.Command{Use: "doctor", Args: cobra.NoArgs, Short: "Check sbx, the envs, your args and logins, and the sandboxes, and say what to fix", RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		checks := []doctorCheck{}
		add := func(area, name, status, message, hint string) {
			checks = append(checks, doctorCheck{area, name, status, message, hint})
		}
		sbx := o.manager.SBX
		sbxOK := false
		if !sbx.Present() {
			add("sbx", "installed", "fail", "sbx is not on PATH", "install Docker Sandboxes: https://docs.docker.com/ai/sandboxes/")
		} else {
			add("sbx", "installed", "pass", "found", "")
			var found []unibox.RuntimeCheck
			err := o.working("checking sbx", func() error {
				var err error
				found, err = sbx.Diagnose(ctx)
				return err
			})
			if err != nil {
				add("sbx", "diagnose", "warn", err.Error(), "")
			}
			sbxOK = err == nil
			for _, c := range found {
				if c.Status == "skip" {
					continue
				}
				if c.Status == "fail" {
					sbxOK = false
				}
				hint := c.Hint
				if c.Link != "" {
					hint = strings.TrimSpace(strings.Replace(hint, " from the link above", "", 1)) + ": " + c.Link
				}
				add("sbx", c.Name, c.Status, c.Message, hint)
			}
			allowed, said := o.manager.Config.SSHAgent()
			forwarding, err := sbx.SSHForwarding(ctx)
			switch {
			case err != nil:
				add("sbx", "SSH agent", "warn", err.Error(), "")
			case forwarding && !allowed:
				add("sbx", "SSH agent", "fail", "sbx forwards your SSH agent, which you have not allowed", "unibox config set ssh_agent false turns it off, or true allows it")
			case !forwarding && allowed:
				add("sbx", "SSH agent", "warn", "you allowed the SSH agent, but sbx does not forward it", "unibox config set ssh_agent true sets sbx to match")
			case forwarding:
				add("sbx", "SSH agent", "pass", "forwarded, as you allowed", "")
			case said:
				add("sbx", "SSH agent", "pass", "not forwarded, as you chose", "")
			default:
				add("sbx", "SSH agent", "pass", "not forwarded", "")
			}
		}
		if _, err := o.paths.LoadEnvValues(); err != nil {
			add("Your args", "vars.toml", "fail", err.Error(), "")
		}
		names := sorted(o.manager.Config.Envs)
		if only != "" {
			names = []string{only}
		}
		if len(names) == 0 {
			add("Envs", "configured", "fail", "no env yet", "unibox env add <name> <file.sbxenv.yaml>")
		}
		state, _ := o.paths.ReadState()
		for _, name := range names {
			area := "Env " + name
			env, err := o.manager.Config.LoadEnv(name)
			if err != nil {
				add(area, "files", "fail", err.Error(), "")
				continue
			}
			add(area, "files", "pass", strings.Join(env.SBX.Files, ", "), "")
			values, _ := o.paths.LoadEnvValues()
			mine := values.Envs[name]
			if missing := env.SBX.Missing(mine); len(missing) > 0 {
				for _, a := range missing {
					add(area, "arg "+a, "fail", cmp.Or(env.SBX.Args[a].Description, "required, and not set"), "unibox var set -e "+name+" "+a+" <value>")
				}
			} else if unknown, err := env.SBX.CheckArgs(mine); err != nil {
				add(area, "args", "fail", err.Error(), "")
			} else {
				add(area, "args", "pass", "given", "")
				for _, u := range unknown {
					add(area, "arg "+u, "warn", "set, but the env declares no such arg", "unibox var rm -e "+name+" "+u)
				}
			}
			sandbox := env.SandboxName()
			if !sbxOK {
				add(area, "logins", "warn", "not checked: sbx is not ready", "")
			} else {
				var logins []unibox.Login
				err := o.working("reading the kits of env "+name, func() error {
					var err error
					logins, err = o.manager.Logins(ctx, env, sandbox)
					return err
				})
				if err != nil {
					add(area, "logins", "warn", err.Error(), "")
				}
				for _, l := range logins {
					switch {
					case l.Stored:
						add(area, "login "+l.Service, "pass", "stored ("+l.Scope+")", "")
					case l.Optional:
						add(area, "login "+l.Service, "warn", cmp.Or(l.Description, "optional, and not stored"), "sbx secret set "+l.Service)
					default:
						add(area, "login "+l.Service, "fail", cmp.Or(l.Description, "not stored"), "sbx secret set "+l.Service)
					}
				}
			}
			if b, ok := state.Sandboxes[sandbox]; ok && b.Env == name && b.Fingerprint != "" && !b.Adopted {
				if args := env.SBX.Declared(mine); env.SBX.Missing(mine) == nil {
					if fp, err := env.SBX.Fingerprint(args); err == nil && fp != b.Fingerprint {
						add(area, "sandbox "+b.Name, "warn", "the env changed since the sandbox was created", "unibox sandbox recreate "+b.Name)
					}
				}
			}
		}
		if sbxOK {
			boxes, err := o.manager.List(ctx)
			if err != nil {
				add("Sandboxes", "state", "fail", err.Error(), "")
			}
			for _, b := range boxes {
				if b.State == "missing" {
					add("Sandboxes", b.Name, "warn", "sbx no longer has it", "unibox prune forgets it, or unibox sandbox recreate "+b.Name+" makes it again")
					continue
				}
				for _, t := range b.Tasks {
					for _, r := range t.Repositories {
						if !r.Mounted {
							continue
						}
						if _, err := os.Stat(r.HostPath); err != nil {
							add("Sandboxes", b.Name, "fail", "task "+t.ID+" mounts "+r.HostPath+", which is gone, so the sandbox cannot start", "unibox task rm "+t.ID)
						}
					}
				}
			}
		}
		failed := 0
		for _, c := range checks {
			if c.Status == "fail" {
				failed++
			}
		}
		if o.json {
			if err := o.write(map[string]any{"api_version": api.Version, "ok": failed == 0, "checks": checks}); err != nil {
				return err
			}
		} else {
			o.printChecks(checks, failed)
		}
		if failed > 0 {
			return &reportedError{4}
		}
		return nil
	}}
	c.Flags().StringVarP(&only, "env", "e", "", "Check this env alone (default: every env)")
	return c
}

// printChecks lays the checks out by area, one column for their names
// across all of them, in colour at a terminal.
func (o *options) printChecks(checks []doctorCheck, failed int) {
	paint := o.painter()
	width := 0
	for _, c := range checks {
		width = max(width, len([]rune(c.Name)))
	}
	area := ""
	for _, c := range checks {
		if c.Area != area {
			if area != "" {
				fmt.Fprintln(o.output)
			}
			area = c.Area
			fmt.Fprintln(o.output, paint("bold", area))
		}
		mark := map[string]string{"pass": paint("green", "✓"), "warn": paint("yellow", "!"), "fail": paint("red", "✗")}[c.Status]
		message := c.Message
		if c.Status == "pass" {
			message = paint("dim", message)
		}
		fmt.Fprintf(o.output, "  %s %-*s  %s\n", mark, width, c.Name, message)
		if c.Hint != "" && c.Status != "pass" {
			fmt.Fprintf(o.output, "    %*s  %s\n", width, "", paint("dim", "→ "+c.Hint))
		}
	}
	fmt.Fprintln(o.output)
	if failed == 0 {
		fmt.Fprintln(o.output, paint("green", "everything a run needs is in place"))
	} else {
		fmt.Fprintln(o.output, paint("red", fmt.Sprintf("%d to fix", failed)))
	}
}

// painter colours text at a terminal that takes colour, and leaves it
// plain elsewhere - a pipe, a file, NO_COLOR.
func (o *options) painter() func(style, text string) string {
	f, ok := o.output.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) || os.Getenv("NO_COLOR") != "" {
		return func(_, text string) string { return text }
	}
	codes := map[string]string{"bold": "1", "dim": "2", "green": "32", "yellow": "33", "red": "31"}
	return func(style, text string) string { return "\x1b[" + codes[style] + "m" + text + "\x1b[0m" }
}
