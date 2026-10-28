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
// is a flag. Left out, the sandbox is taken from UNIBOX_SANDBOX, then from
// unibox context use, then from there being only one; the env from
// UNIBOX_ENV, then from that sandbox, then from default_env, then from
// there being only one.

func envFlag(c *cobra.Command, e *string) {
	c.Flags().StringVarP(e, "env", "e", "", "Env (default: UNIBOX_ENV, the sandbox's, default_env, or the only one)")
}

func sandboxFlag(c *cobra.Command, s *string) {
	c.Flags().StringVarP(s, "sandbox", "s", "", "Sandbox (default: UNIBOX_SANDBOX, the context's, or the only one)")
}

// defaultSandbox is the sandbox a command works in when it is not told,
// or "" when none can be said: UNIBOX_SANDBOX, the context's, the sandbox
// of the context's env once it is made, or the only one.
func (o *options) defaultSandbox() string {
	if s := o.chosenSandbox(); s != "" {
		return s
	}
	if o.manager.Config.Context.Env != "" {
		// An env chosen before its sandbox was made has none yet; another
		// env's sandbox is not it.
		return ""
	}
	if state, err := o.paths.ReadState(); err == nil && len(state.Sandboxes) == 1 {
		for name := range state.Sandboxes {
			return name
		}
	}
	return ""
}

// chosenSandbox is the sandbox the person chose to work in: UNIBOX_SANDBOX,
// the context's, or that of the context's env once it is made.
func (o *options) chosenSandbox() string {
	if s := os.Getenv("UNIBOX_SANDBOX"); s != "" {
		return s
	}
	ctx := o.manager.Config.Context
	if ctx.Sandbox != "" {
		return ctx.Sandbox
	}
	if ctx.Env != "" {
		return o.madeSandbox(ctx.Env)
	}
	return ""
}

// madeSandbox is an env's sandbox if unibox has made it, else "".
func (o *options) madeSandbox(env string) string {
	ec, ok := o.manager.Config.Envs[env]
	if !ok {
		return ""
	}
	name := ec.Name
	if name == "" {
		sbx, err := unibox.ReadSBXEnv(ec.Files)
		if err != nil {
			return ""
		}
		name = sbx.SandboxName(env)
	}
	if state, err := o.paths.ReadState(); err == nil {
		if _, made := state.Sandboxes[name]; made {
			return name
		}
	}
	return ""
}

// sandboxEnv is the env a sandbox was made from, if it is still an env.
func (o *options) sandboxEnv(sandbox string) string {
	state, err := o.paths.ReadState()
	if err != nil {
		return ""
	}
	if _, known := o.manager.Config.Envs[state.Sandboxes[sandbox].Env]; known {
		return state.Sandboxes[sandbox].Env
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
		if o.json {
			return o.write(map[string]any{"api_version": api.Version, "context": o.manager.Config.Context, "sandbox": o.defaultSandbox(), "env": o.defaultEnv()})
		}
		fmt.Fprintln(o.output, o.contextText())
		return nil
	}
	c := &cobra.Command{Use: "context", Args: cobra.NoArgs, Short: "Show or set the sandbox commands work in when not told", Long: `Commands take the sandbox they work in as -s and the env as -e. Left
out, the sandbox comes from UNIBOX_SANDBOX, then from the context set here,
then from there being only one; the env from UNIBOX_ENV, then from that
sandbox, then from default_env, then from there being only one. The context
is a sandbox, or an env whose sandbox is not made yet, which becomes its
sandbox once it is.`, Example: "  unibox context use -s dev\n  unibox context\n  unibox context clear", RunE: func(cmd *cobra.Command, args []string) error {
		if !o.interactive || o.json {
			return show()
		}
		return o.contextUI(cmd, show)
	}}
	var sandbox, env string
	use := &cobra.Command{Use: "use", Args: cobra.NoArgs, Short: "Set the sandbox commands work in, or an env whose sandbox is to be made", RunE: func(cmd *cobra.Command, args []string) error {
		if sandbox == "" && env == "" {
			return fmt.Errorf("say what to use: -s <sandbox>, or -e <env> for its sandbox")
		}
		if err := o.useContext(sandbox, env); err != nil {
			return err
		}
		return show()
	}}
	use.Flags().StringVarP(&sandbox, "sandbox", "s", "", "Sandbox commands work in")
	use.Flags().StringVarP(&env, "env", "e", "", "Env whose sandbox commands work in, made or to be made")
	use.MarkFlagsMutuallyExclusive("sandbox", "env")
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

// contextText says what commands work in when not told, and why.
func (o *options) contextText() string {
	sandbox, env := o.defaultSandbox(), o.defaultEnv()
	why := ""
	switch {
	case os.Getenv("UNIBOX_SANDBOX") != "":
		why = " (UNIBOX_SANDBOX)"
	case sandbox != "" && o.chosenSandbox() == "":
		why = " (the only one)"
	}
	if sandbox == "" && env == "" {
		return "commands work in what they are told; unibox context use -s <sandbox> sets one"
	}
	if sandbox == "" {
		return "commands work in env " + env + ", whose sandbox is not made yet"
	}
	if env == "" {
		return "commands work in sandbox " + sandbox + why
	}
	return "commands work in sandbox " + sandbox + why + ", of env " + env
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

// useContext sets what commands work in when not told: a sandbox, or an
// env, which is its sandbox once that is made and until then the env
// alone. The env of a sandbox is the one it was made from, so the context
// never names two.
func (o *options) useContext(sandbox, env string) error {
	// Read again: a step before may have saved what this one must keep.
	cfg, err := o.paths.LoadConfig()
	if err != nil {
		return err
	}
	o.manager.Config = cfg
	switch {
	case sandbox != "":
		state, err := o.paths.ReadState()
		if err != nil {
			return err
		}
		if _, ok := state.Sandboxes[sandbox]; !ok {
			return fmt.Errorf("sandbox %s not found; unibox sandbox ls lists them", sandbox)
		}
		cfg.Context = unibox.Context{Sandbox: sandbox}
	case env != "":
		if _, ok := cfg.Envs[env]; !ok {
			return fmt.Errorf("there is no env %s; unibox env ls lists them", env)
		}
		cfg.Context = unibox.Context{Env: env}
		if made := o.madeSandbox(env); made != "" {
			cfg.Context = unibox.Context{Sandbox: made}
		}
	}
	if err := o.paths.SaveConfig(cfg); err != nil {
		return err
	}
	o.manager.Config = cfg
	return nil
}
