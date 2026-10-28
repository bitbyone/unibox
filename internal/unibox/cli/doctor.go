package cli

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

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
	// Fix is the unibox command that fixes it, which doctor -i offers to
	// run; empty where the fix needs a value or a choice of the person's.
	Fix []string `json:"-"`
	// Cost is what running Fix gives up, said in the one question asked,
	// as the command runs with --yes once the person agrees.
	Cost string `json:"-"`
}

// doctorCommand checks what a run needs, before a run fails on it: sbx,
// each env's files, the person's args and secrets, and the sandboxes unibox
// keeps.
func (o *options) doctorCommand() *cobra.Command {
	var only string
	var interactive bool
	c := &cobra.Command{Use: "doctor", Args: cobra.NoArgs, Short: "Check sbx, the envs, your args and secrets, and the sandboxes, and say what to fix", RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		checks := []doctorCheck{}
		add := func(area, name, status, message, hint string) {
			checks = append(checks, doctorCheck{area, name, status, message, hint, nil, ""})
		}
		// fix gives the check added last the command that fixes it.
		fix := func(args ...string) { checks[len(checks)-1].Fix = args }
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
				if c.Name == "Global network policy" && c.Status == "fail" {
					hint = "unibox network init chooses it"
				}
				if c.Name == "SSH client config" && c.Status != "pass" {
					hint = "unibox config set editor_ssh true sets it up"
				}
				add("sbx", c.Name, c.Status, c.Message, hint)
				if c.Name == "Global network policy" && c.Status == "fail" {
					fix("network", "init")
				}
			}
			if version, err := sbx.Version(ctx); err != nil {
				add("sbx", "for unibox", "warn", err.Error(), "")
			} else if !unibox.VersionAtLeast(version, unibox.MinSBXVersion) {
				sbxOK = false
				add("sbx", "for unibox", "fail", version+", older than the "+unibox.MinSBXVersion+" unibox needs", "update Docker Sandboxes")
			} else {
				add("sbx", "for unibox", "pass", version+", new enough", "")
			}
			if remember, err := sbx.Setting(ctx, "env.rememberHostCommands"); err == nil {
				if remember == "true" {
					add("sbx", "host commands", "warn", "sbx runs an env's host commands again without asking", "unibox config set remember_host_commands false has it ask each time")
					fix("config", "set", "remember_host_commands", "false")
				} else {
					add("sbx", "host commands", "pass", "sbx asks before an env's host commands run", "")
				}
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
				fix("config", "set", "ssh_agent", "true")
			case forwarding:
				add("sbx", "SSH agent", "pass", "forwarded, as you allowed", "")
			case said:
				add("sbx", "SSH agent", "pass", "not forwarded, as you chose", "")
			default:
				add("sbx", "SSH agent", "pass", "not forwarded", "")
			}
			if on, said := o.manager.Config.EditorSSH(); said {
				serving, err := sbx.EditorSSH(ctx)
				switch {
				case err != nil:
					add("sbx", "editor SSH", "warn", err.Error(), "")
				case serving != on:
					add("sbx", "editor SSH", "warn", fmt.Sprintf("your config.yaml says %t, sbx's feature.ssh %t", on, serving), fmt.Sprintf("unibox config set editor_ssh %t sets sbx to match", on))
					fix("config", "set", "editor_ssh", strconv.FormatBool(on))
				case on:
					add("sbx", "editor SSH", "pass", "editors reach a sandbox as ssh <sandbox>.sbx", "")
				default:
					add("sbx", "editor SSH", "pass", "off, as you chose", "")
				}
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
		behind := map[string]int{}
		needed := map[string][]unibox.NeededSecret{}
		for _, name := range names {
			area := "Env " + name
			env, err := o.manager.Config.LoadEnv(name)
			if err != nil {
				add(area, "files", "fail", err.Error(), "")
				continue
			}
			add(area, "files", "pass", strings.Join(env.SBX.Files, ", "), "")
			values, _ := o.paths.LoadEnvValues()
			mine := values.ArgsFor(env)
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
					fix("var", "rm", "-e", name, u)
				}
				if sbxOK {
					var plan string
					err := o.working("planning env "+name, func() error {
						var err error
						plan, err = o.manager.Plan(ctx, env)
						return err
					})
					if err != nil {
						add(area, "plan", "fail", err.Error(), "fix the env's files; unibox env show "+name+" shows what they declare")
					} else {
						add(area, "plan", "pass", plan, "")
					}
				}
			}
			sandbox := env.SandboxName()
			if !sbxOK {
				add(area, "secrets", "warn", "not checked: sbx is not ready", "")
			} else {
				var secrets []unibox.NeededSecret
				err := o.working("reading the kits of env "+name, func() error {
					var err error
					secrets, err = o.manager.NeededSecrets(ctx, env, sandbox)
					return err
				})
				if err != nil {
					add(area, "secrets", "warn", err.Error(), "")
				}
				needed[name] = secrets
				for _, l := range secrets {
					switch {
					case l.Stored:
						add(area, "secret "+l.Service, "pass", "stored ("+l.Scope+")", "")
					case l.Optional:
						add(area, "secret "+l.Service, "warn", cmp.Or(l.Description, "optional, and not stored"), "unibox secret set -e "+name+" "+l.Service)
					default:
						add(area, "secret "+l.Service, "fail", cmp.Or(l.Description, "not stored"), "unibox secret set -e "+name+" "+l.Service)
						fix("secret", "set", "-e", name, l.Service)
					}
				}
			}
			if changed(state, env, values) {
				add(area, "sandbox "+sandbox, "warn", "the env changed since the sandbox was created", "unibox sandbox recreate "+sandbox)
				fix("sandbox", "recreate", sandbox)
				checks[len(checks)-1].Cost = "it discards what the sandbox holds outside its tasks"
			}
			if repo := env.Config.Repo; env.Config.Source != "" && repo != "" {
				if _, done := behind[repo]; !done {
					var n int
					err := o.working("asking the remote of "+repo, func() (err error) {
						ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
						defer cancel()
						n, err = unibox.Behind(ctx, repo)
						return err
					})
					behind[repo] = n
					switch {
					case err != nil:
						add(area, "clone", "warn", err.Error(), "")
					case n > 0:
						add(area, "clone", "warn", fmt.Sprintf("%d commit(s) behind its remote", n), "unibox env update")
						fix("env", "update", name)
					default:
						add(area, "clone", "pass", "as its remote", "")
					}
				}
			}
		}
		for _, c := range unibox.SecretClashes(needed) {
			said := []string{}
			for _, env := range slices.Sorted(maps.Keys(c.Hosts)) {
				said = append(said, "env "+env+" sends it to "+strings.Join(c.Hosts[env], ", "))
			}
			add("Tokens", "secret "+c.Service, "warn", strings.Join(said, "; ")+"; sbx keeps one token for all of them", "have one team's kit name its service apart, such as "+c.Service+"-<team>")
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
				if b.State == "running" {
					var missing [][2]string
					var stray [][2]string
					err := o.working("reading the mounts of sandbox "+b.Name, func() error {
						var err error
						missing, stray, err = o.manager.MountDrift(ctx, b)
						return err
					})
					if err != nil {
						add("Sandboxes", b.Name, "warn", "mounts not checked: "+err.Error(), "")
					}
					for _, mt := range missing {
						add("Sandboxes", b.Name, "warn", mt[0]+" is not mounted at "+mt[1], "unibox resume of its task mounts it again")
					}
					for _, mt := range stray {
						add("Sandboxes", b.Name, "warn", mt[0]+" is mounted at "+mt[1]+", but no task has it", "unibox prune unmounts it")
						fix("prune")
					}
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
		if interactive {
			return o.offerFixes(cmd, checks)
		}
		if failed > 0 {
			return &reportedError{4}
		}
		return nil
	}}
	c.Flags().StringVarP(&only, "env", "e", "", "Check this env alone (default: every env)")
	c.Flags().BoolVarP(&interactive, "interactive", "i", false, "Offer to run the command that fixes each problem, one at a time")
	return c
}

// offerFixes asks, problem by problem, whether to run the unibox command
// that fixes it, and runs it as its own command would. What needs a value
// or a choice of the person's has no fix and stays a hint.
func (o *options) offerFixes(cmd *cobra.Command, checks []doctorCheck) error {
	if !o.canAsk() {
		return fmt.Errorf("doctor -i asks at a terminal; run it in one, or run the commands it names")
	}
	offered := 0
	for _, c := range checks {
		if c.Status == "pass" || len(c.Fix) == 0 {
			continue
		}
		offered++
		question := fmt.Sprintf("%s, %s: %s. Run unibox %s?", c.Area, c.Name, c.Message, strings.Join(c.Fix, " "))
		if c.Cost != "" {
			question = fmt.Sprintf("%s, %s: %s. Run unibox %s? %s", c.Area, c.Name, c.Message, strings.Join(c.Fix, " "), strings.ToUpper(c.Cost[:1])+c.Cost[1:]+".")
		}
		run, err := o.askYes(question, true)
		if err != nil {
			return err
		}
		if !run {
			continue
		}
		// Agreed once: the command does not ask again for the same.
		yes := o.yes
		o.yes = true
		err = o.runStep(cmd, c.Fix...)
		o.yes = yes
		if err != nil {
			return err
		}
	}
	if offered == 0 {
		return o.success("nothing doctor can fix by itself; the hints above say the rest")
	}
	return o.success("run unibox doctor again to see what is left")
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

// changed says whether the env is no longer what its sandbox was created
// from, as far as unibox made it and the person's values let it be told.
func changed(state unibox.State, env unibox.Env, values unibox.EnvValues) bool {
	mine := values.ArgsFor(env)
	b, ok := state.Sandboxes[env.SandboxName()]
	if !ok || b.Env != env.Name || b.Fingerprint == "" || b.Adopted || len(env.SBX.Missing(mine)) > 0 {
		return false
	}
	r, err := values.ResourcesFor(env, env.SandboxName()).Resolve(unibox.HostResources(), io.Discard)
	if err != nil {
		return false
	}
	fp, err := env.SBX.Fingerprint(env.SBX.Declared(mine), r)
	return err == nil && fp != b.Fingerprint
}
