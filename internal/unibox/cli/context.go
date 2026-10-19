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

// What a command works on is its argument; the profile or sandbox it works
// in is a flag. Left out, it is taken from UNIBOX_PROFILE or
// UNIBOX_SANDBOX, then from unibox context use, then from there being only
// one.

func profileFlag(c *cobra.Command, p *string) {
	c.Flags().StringVarP(p, "profile", "p", "", "Profile (default: UNIBOX_PROFILE, the context's, or the only one)")
}

func sandboxFlag(c *cobra.Command, s *string) {
	c.Flags().StringVarP(s, "sandbox", "s", "", "Sandbox (default: UNIBOX_SANDBOX, the context's, or the only one)")
}

// defaultProfile is the profile a command works in when it is not told,
// or "" when none can be said.
func (o *options) defaultProfile() string {
	if p := os.Getenv("UNIBOX_PROFILE"); p != "" {
		return p
	}
	if p := o.manager.Config.Context.Profile; p != "" {
		return p
	}
	if names, err := unibox.ProfileNames(o.manager.Config.Environments); err == nil && len(names) == 1 {
		return names[0]
	}
	return ""
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

func (o *options) profileName(flag string) (string, error) {
	if p := cmp.Or(flag, o.defaultProfile()); p != "" {
		return p, nil
	}
	names, _ := unibox.ProfileNames(o.manager.Config.Environments)
	return "", fmt.Errorf("no profile chosen; pass -p (%s), or set one with unibox context use -p <profile>", orNoneOf(names, "unibox profile ls lists them"))
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

// contextCommand keeps the sandbox and profile commands work in when they
// are not told, as kubectl keeps its context.
func (o *options) contextCommand() *cobra.Command {
	show := func() error {
		c := o.manager.Config.Context
		if o.json {
			return o.write(map[string]any{"api_version": api.Version, "context": c, "sandbox": o.defaultSandbox(), "profile": o.defaultProfile()})
		}
		fmt.Fprintf(o.output, "sandbox  %s\nprofile  %s\n", describe(c.Sandbox, o.defaultSandbox(), "UNIBOX_SANDBOX"), describe(c.Profile, o.defaultProfile(), "UNIBOX_PROFILE"))
		return nil
	}
	c := &cobra.Command{Use: "context", Args: cobra.NoArgs, Short: "Show or set the sandbox and profile commands work in when not told", Long: `Commands take the sandbox they work in as -s and the profile as -p. Left
out, it comes from UNIBOX_SANDBOX or UNIBOX_PROFILE, then from the context
set here, then from there being only one.`, Example: "  unibox context use -s dev\n  unibox context\n  unibox context clear", RunE: func(cmd *cobra.Command, args []string) error { return show() }}
	var sandbox, profile string
	use := &cobra.Command{Use: "use", Args: cobra.NoArgs, Short: "Set the context's sandbox, profile or both", RunE: func(cmd *cobra.Command, args []string) error {
		if sandbox == "" && profile == "" {
			return fmt.Errorf("say what to use: -s <sandbox>, -p <profile> or both")
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
		if profile != "" {
			if _, err := o.manager.Profile(profile); err != nil {
				return err
			}
			cfg.Context.Profile = profile
		}
		if err := o.paths.SaveConfig(cfg); err != nil {
			return err
		}
		o.manager.Config = cfg
		return show()
	}}
	use.Flags().StringVarP(&sandbox, "sandbox", "s", "", "Sandbox commands work in")
	use.Flags().StringVarP(&profile, "profile", "p", "", "Profile commands work in")
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
// is not told: the default sandbox, else a new one of the default profile.
func (o *options) defaultTarget(r *unibox.Request) {
	if r.Sandbox != "" || r.NewSandbox != "" || r.Profile != "" || r.Task != "" && len(r.Repositories) == 0 {
		return
	}
	if s := o.defaultSandbox(); s != "" {
		r.Sandbox = s
	} else if p := o.defaultProfile(); p != "" {
		r.Profile = p
	}
}
