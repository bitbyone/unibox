package cli

import (
	"cmp"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
)

// What a command works on is its argument; the env or sandbox it works in
// is a flag. Left out, it is taken from UNIBOX_ENV or UNIBOX_SANDBOX, then
// from unibox context use, then from there being only one.

func envFlag(c *cobra.Command, e *string) {
	c.Flags().StringVarP(e, "env", "e", "", "Env a new sandbox is created from (default: UNIBOX_ENV, the context's, or the only one)")
}

func sandboxFlag(c *cobra.Command, s *string) {
	c.Flags().StringVarP(s, "sandbox", "s", "", "Sandbox (default: UNIBOX_SANDBOX, the context's, or the only one)")
}

// defaultSandbox is the sandbox a command works in when it is not told,
// or "" when none can be said.
func (o *options) defaultSandbox() string {
	if s := os.Getenv("UNIBOX_SANDBOX"); s != "" {
		return s
	}
	if s := o.manager.Config.Context.Sandbox; s != "" {
		return s
	}
	if state, err := o.paths.ReadState(); err == nil && len(state.Sandboxes) == 1 {
		for name := range state.Sandboxes {
			return name
		}
	}
	return ""
}

func (o *options) sandboxName(flag string) (string, error) {
	if s := cmp.Or(flag, o.defaultSandbox()); s != "" {
		return s, nil
	}
	names := []string{}
	if state, err := o.paths.ReadState(); err == nil {
		for name := range state.Sandboxes {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return "", fmt.Errorf("no sandbox chosen; pass -s (%s), or set one with unibox context use -s <sandbox>", orNoneOf(names, "unibox sandbox ls lists them"))
}

func orNoneOf(names []string, hint string) string {
	if len(names) == 0 {
		return hint
	}
	return "one of " + strings.Join(names, ", ")
}

// contextCommand keeps the sandbox and env commands work in when they
// are not told, as kubectl keeps its context.
func (o *options) contextCommand() *cobra.Command {
	show := func() error {
		c := o.manager.Config.Context
		if o.json {
			return o.write(map[string]any{"api_version": api.Version, "context": c, "sandbox": o.defaultSandbox(), "env": o.defaultEnv()})
		}
		fmt.Fprintf(o.output, "sandbox  %s\nenv      %s\n", describe(c.Sandbox, o.defaultSandbox(), "UNIBOX_SANDBOX"), describe(c.Env, o.defaultEnv(), "UNIBOX_ENV"))
		return nil
	}
	c := &cobra.Command{Use: "context", Args: cobra.NoArgs, Short: "Show or set the sandbox and env commands work in when not told", Long: `Commands take the sandbox they work in as -s and the env a new one is
created from as -e. Left out, it comes from UNIBOX_SANDBOX or UNIBOX_ENV,
then from the context set here, then from there being only one (for an
env, the configuration's default_env before that).`, Example: "  unibox context use -s dev\n  unibox context\n  unibox context clear", RunE: func(cmd *cobra.Command, args []string) error {
		if !o.interactive || o.json {
			return show()
		}
		return o.contextUI(cmd, show)
	}}
	var sandbox, env string
	use := &cobra.Command{Use: "use", Args: cobra.NoArgs, Short: "Set the context's sandbox, env or both", RunE: func(cmd *cobra.Command, args []string) error {
		if sandbox == "" && env == "" {
			return fmt.Errorf("say what to use: -s <sandbox>, -e <env> or both")
		}
		cfg := o.manager.Config
		if sandbox != "" {
			state, err := o.paths.ReadState()
			if err != nil {
				return err
			}
			if _, ok := state.Sandboxes[sandbox]; !ok {
				return fmt.Errorf("sandbox %s not found; unibox sandbox ls lists them", sandbox)
			}
			cfg.Context.Sandbox = sandbox
		}
		if env != "" {
			if _, ok := cfg.Envs[env]; !ok {
				return fmt.Errorf("there is no env %s; unibox env ls lists them", env)
			}
			cfg.Context.Env = env
		}
		if err := o.paths.SaveConfig(cfg); err != nil {
			return err
		}
		o.manager.Config = cfg
		return show()
	}}
	use.Flags().StringVarP(&sandbox, "sandbox", "s", "", "Sandbox commands work in")
	use.Flags().StringVarP(&env, "env", "e", "", "Env new sandboxes are created from")
	c.AddCommand(use)
	c.AddCommand(&cobra.Command{Use: "clear", Args: cobra.NoArgs, Short: "Forget the context", RunE: func(cmd *cobra.Command, args []string) error {
		cfg := o.manager.Config
		cfg.Context = unibox.Context{}
		if err := o.paths.SaveConfig(cfg); err != nil {
			return err
		}
		o.manager.Config = cfg
		return show()
	}})
	return c
}

// describe says what a command works in when not told, and why.
func describe(set, effective, env string) string {
	switch {
	case effective == "":
		return "-"
	case os.Getenv(env) != "":
		return effective + " (" + env + ")"
	case set != "":
		return effective
	}
	return effective + " (the only one)"
}

// forgetContext takes a removed sandbox out of the context.
func (o *options) forgetContext(sandbox string) error {
	cfg := o.manager.Config
	if cfg.Context.Sandbox != sandbox {
		return nil
	}
	cfg.Context.Sandbox = ""
	o.manager.Config = cfg
	return o.paths.SaveConfig(cfg)
}

// defaultTarget fills in where a new task goes when run, prepare or check
// is not told: the default sandbox, else the default env's.
func (o *options) defaultTarget(r *unibox.Request) {
	if r.Sandbox != "" || r.Env != "" || r.Task != "" && len(r.Repositories) == 0 {
		return
	}
	if s := o.defaultSandbox(); s != "" {
		r.Sandbox = s
	} else if e := o.defaultEnv(); e != "" {
		r.Env = e
	}
}
