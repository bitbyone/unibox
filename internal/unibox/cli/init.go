package cli

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
)

// inspectCommand shows what unibox keeps about a sandbox.
func (o *options) inspectCommand() *cobra.Command {
	return &cobra.Command{Use: "inspect [sandbox]", Args: cobra.MaximumNArgs(1), Short: "Show what unibox keeps about a sandbox", RunE: func(cmd *cobra.Command, args []string) error {
		name, err := o.sandboxName(first(args))
		if err != nil {
			return err
		}
		boxes, err := o.manager.List(cmd.Context())
		if err != nil {
			return err
		}
		for _, b := range boxes {
			if b.Name != name {
				continue
			}
			if o.json {
				return o.write(map[string]any{"api_version": api.Version, "sandbox": b})
			}
			record, _ := json.MarshalIndent(b, "", "  ")
			fmt.Fprintln(o.output, string(record))
			return nil
		}
		return fmt.Errorf("sandbox %s not found; unibox sandbox ls lists them", name)
	}}
}

// initCommand walks through setting unibox up at a terminal: sbx, an
// env, its args, its logins, its sandbox. Each step runs the command it
// stands for, so the walk and the commands never differ; what is in place
// already is only said. Away from a terminal it says what is left.
func (o *options) initCommand() *cobra.Command {
	return &cobra.Command{Use: "init", Args: cobra.NoArgs, Short: "Set unibox up step by step: sbx, an env, its args, logins and sandbox", RunE: func(cmd *cobra.Command, args []string) error {
		if !o.canAsk() {
			if len(o.manager.Config.Envs) == 0 {
				fmt.Fprintln(o.output, "run unibox init in a terminal, or name your team's env with unibox env add <name> <file.sbxenv.yaml>; unibox doctor says what is left after it")
				return nil
			}
			fmt.Fprintln(o.output, "run unibox init in a terminal to be walked through it; unibox doctor says what is left")
			return nil
		}
		return o.walk(cmd)
	}}
}

// walkStep heads a step of the walk.
func (o *options) walkStep(n int, title string) {
	fmt.Fprintf(o.log, "\n%d/5 %s\n", n, title)
}

func (o *options) walk(cmd *cobra.Command) error {
	ctx := cmd.Context()
	o.walkStep(1, "sbx")
	if err := o.manager.SBX.Available(ctx); err != nil {
		return fmt.Errorf("%w; init goes on once sbx is installed and signed in (sbx login)", err)
	}
	fmt.Fprintln(o.log, "  installed and signed in")
	if err := o.askSSHAgent(cmd); err != nil {
		return err
	}

	o.walkStep(2, "Env")
	name, err := o.walkEnv(cmd)
	if err != nil {
		return err
	}
	env, err := o.manager.Config.LoadEnv(name)
	if err != nil {
		return err
	}

	o.walkStep(3, "Values")
	asked, err := o.askArgs(env)
	if err != nil {
		return err
	}
	if !asked {
		fmt.Fprintf(o.log, "  env %s has every value it needs\n", name)
	}

	o.walkStep(4, "Logins")
	var logins []unibox.Login
	if err = o.working("reading the kits of env "+name, func() error {
		logins, err = o.manager.Logins(ctx, env, "")
		return err
	}); err != nil {
		return err
	}
	missing := []string{}
	for _, l := range logins {
		if !l.Stored && !l.Optional {
			missing = append(missing, l.Service)
		}
	}
	switch {
	case len(missing) == 0:
		fmt.Fprintf(o.log, "  sbx has every token env %s needs\n", name)
	default:
		fmt.Fprintf(o.log, "  sbx has no token for %s; it asks for each on this terminal, and the value goes to it alone\n", strings.Join(missing, ", "))
		if store, err := o.askYes("Store them now?", true); err != nil {
			return err
		} else if store {
			if err = o.runStep(cmd, "login", "set", "-e", name); err != nil {
				return err
			}
		} else {
			fmt.Fprintf(o.log, "  later: unibox login set -e %s\n", name)
		}
	}

	o.walkStep(5, "Sandbox")
	sandbox := env.SandboxName()
	state, err := o.paths.ReadState()
	if err != nil {
		return err
	}
	if b, ok := state.Sandboxes[sandbox]; ok && b.State != "creating" {
		fmt.Fprintf(o.log, "  sandbox %s is there\n", sandbox)
	} else if create, err := o.askYes(fmt.Sprintf("Create sandbox %s now? sbx shows what it makes and asks to approve it", sandbox), true); err != nil {
		return err
	} else if create {
		if err = o.runStep(cmd, "sandbox", "create", "-e", name); err != nil {
			return err
		}
	} else {
		fmt.Fprintln(o.log, "  unibox run creates it when it first needs it")
	}
	fmt.Fprintf(o.log, "\nset up; start a task with unibox run <directory>, or unibox run --shell for a shell\n")
	return nil
}

// walkEnv picks the env the walk sets up, adding one with env add when
// there is none or another is wanted.
func (o *options) walkEnv(cmd *cobra.Command) (string, error) {
	cfg := o.manager.Config
	const another = "\x00add"
	if len(cfg.Envs) > 0 {
		options := []pick{}
		for _, name := range slices.Sorted(maps.Keys(cfg.Envs)) {
			options = append(options, pick{name, name + " (" + strings.Join(cfg.Envs[name].Files, ", ") + ")"})
		}
		options = append(options, pick{another, "another, from an sbxenv file"})
		choice, err := o.askPick("Env to set up", "An env names your team's sbxenv files.", options, cmp.Or(cfg.DefaultEnv, options[0].value))
		if err != nil || choice != another {
			return choice, err
		}
	}
	file, err := o.askText("sbxenv file", "Your team's file, such as agent-sandboxes/sandboxes/dev.sbxenv.yaml; a local overlay can be added later with unibox env add.", "", func(s string) error {
		path, err := unibox.ExpandPath(s)
		if err != nil {
			return err
		}
		_, err = unibox.ReadSBXEnv([]string{path})
		return err
	})
	if err != nil {
		return "", err
	}
	path, _ := unibox.ExpandPath(file)
	sbxEnv, _ := unibox.ReadSBXEnv([]string{path})
	preset := cmp.Or(sbxEnv.Name, strings.TrimSuffix(filepath.Base(path), ".sbxenv.yaml"))
	name, err := o.askText("Env name", "What commands call it, as in unibox run -e <name>.", preset, func(s string) error {
		if _, taken := o.manager.Config.Envs[s]; taken {
			return fmt.Errorf("env %s exists already", s)
		}
		return unibox.ValidName(s)
	})
	if err != nil {
		return "", err
	}
	return name, o.runStep(cmd, "env", "add", name, path)
}

// runStep runs a command of unibox as the walk's step.
func (o *options) runStep(cmd *cobra.Command, args ...string) error {
	c, rest, err := cmd.Root().Find(args)
	if err != nil {
		return err
	}
	if err = c.ParseFlags(rest); err != nil {
		return err
	}
	c.SetContext(cmd.Context())
	return c.RunE(c, c.Flags().Args())
}
