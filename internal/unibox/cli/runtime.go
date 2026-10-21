package cli

import (
	"fmt"

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
