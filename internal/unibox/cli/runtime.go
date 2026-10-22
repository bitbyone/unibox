package cli

import (
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/tobola/unibox/internal/unibox"
)

const sbxMissing = "Docker Sandboxes (sbx) is not installed. It is the runtime unibox supports and prefers: install it from https://docs.docker.com/ai/sandboxes/, run sbx login, then unibox init again."

// runtimeMissing says what is wrong when the runtime new sandboxes would
// get is not installed, or "" when it is.
func (o *options) runtimeMissing() string {
	name := o.manager.DefaultRuntime()
	r, ok := o.manager.Runtimes[name]
	if ok {
		if p, isPresent := r.(unibox.Present); !isPresent || p.Present() {
			return ""
		}
	}
	if name == "" || name == "sbx" {
		return sbxMissing
	}
	return fmt.Sprintf("runtime %s, the configured default, is not installed; install it, or unibox config unset default_runtime for sbx", name)
}

// checkRuntime is where an interactive init starts: without sbx it says
// so, and offers the experimental microsandbox instead where it can run,
// for the user to choose; without a terminal it warns and goes on.
func (o *options) checkRuntime(cmd *cobra.Command, interactive bool) error {
	missing := o.runtimeMissing()
	if missing == "" {
		return nil
	}
	if !interactive {
		fmt.Fprintln(o.log, "warning:", missing)
		return nil
	}
	msb, ok := o.manager.Runtimes["msb"]
	if missing != sbxMissing || !ok || !unibox.MSBPlatform() {
		return fmt.Errorf("%s", missing)
	}
	choice, err := o.askPick("No sandbox runtime", missing+"\n\nmicrosandbox (msb) can run sandboxes here instead; it is experimental: logins, the network policy and live changes are not what sbx gives.", []pick{
		{"stop", "Stop - I will install sbx first (recommended)"},
		{"msb", "Go on with microsandbox (msb), experimental"},
	}, "stop")
	if err != nil {
		return err
	}
	if choice != "msb" {
		return fmt.Errorf("install sbx, run sbx login, then unibox init again")
	}
	cfg := o.manager.Config
	cfg.DefaultRuntime = "msb"
	if err = o.paths.SaveConfig(cfg); err != nil {
		return err
	}
	o.manager.Config = cfg
	fmt.Fprintln(o.log, "new sandboxes run in msb (unibox config unset default_runtime goes back to sbx once it is installed)")
	if err = msb.Available(cmd.Context()); err != nil {
		fmt.Fprintf(o.log, "before the first run: %v\n", err)
	}
	return nil
}

// runtimeHealth is the runtime checking itself, for the user to see what
// to fix before a sandbox fails on it: the checks that fail, and, where
// the user is not signed in, the sign-in offered at once.
func (o *options) runtimeHealth(cmd *cobra.Command) error {
	d, ok := o.manager.Runtimes[o.manager.DefaultRuntime()].(unibox.Diagnoser)
	if !ok || o.runtimeMissing() != "" {
		return nil
	}
	var checks []unibox.RuntimeCheck
	err := o.working("checking "+o.manager.DefaultRuntime(), func() error {
		var err error
		checks, err = d.Diagnose(cmd.Context())
		return err
	})
	if err != nil {
		fmt.Fprintf(o.log, "could not check the runtime: %v\n", err)
		return nil
	}
	failed := false
	for _, c := range checks {
		if c.Status != "fail" {
			continue
		}
		failed = true
		fmt.Fprintf(o.log, "✗ %s - %s\n", c.Name, c.Message)
		if c.Hint != "" {
			fmt.Fprintf(o.log, "    %s\n", c.Hint)
		}
		if c.Name == "Authentication" && o.manager.DefaultRuntime() == "sbx" {
			login, err := o.askYes("Sign in to Docker now (sbx login)?", true)
			if err != nil {
				return err
			}
			if login {
				l := exec.CommandContext(cmd.Context(), "sbx", "login")
				l.Stdin, l.Stdout, l.Stderr = o.input, o.log, o.log
				if err := l.Run(); err != nil {
					return fmt.Errorf("sbx login did not finish; run it yourself, then unibox init again: %w", err)
				}
			}
		}
	}
	if failed {
		fmt.Fprintln(o.log, "unibox doctor checks again once you have fixed it")
	}
	return nil
}
