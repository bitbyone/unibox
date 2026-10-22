package cli

import (
	"fmt"
	"slices"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
)

// doctorCheck is one thing unibox doctor looked at.
type doctorCheck struct {
	Area    string `json:"area"`
	Name    string `json:"name"`
	Status  string `json:"status"` // pass, warn or fail
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// doctorCommand checks what a run needs, before a run fails on it: the
// runtime, the environments repository, the user's values and each
// profile's tokens, logins and image.
func (o *options) doctorCommand() *cobra.Command {
	var profile string
	c := &cobra.Command{Use: "doctor", Args: cobra.NoArgs, Short: "Check the runtime, the environments repository and the profiles, and say what to fix", RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		checks := []doctorCheck{}
		add := func(area, name, status, message, hint string) {
			checks = append(checks, doctorCheck{area, name, status, message, hint})
		}
		rt := o.manager.DefaultRuntime()
		runtime := "Runtime " + rt
		if missing := o.runtimeMissing(); missing != "" {
			add("Runtime", "installed", "fail", missing, "")
		} else {
			r := o.manager.Runtimes[rt]
			add(runtime, "installed", "pass", "found", "")
			if d, ok := r.(unibox.Diagnoser); ok {
				var found []unibox.RuntimeCheck
				err := o.working("checking "+rt, func() error {
					var err error
					found, err = d.Diagnose(ctx)
					return err
				})
				if err != nil {
					add(runtime, "diagnose", "warn", err.Error(), "")
				}
				for _, c := range found {
					if c.Status == "skip" {
						continue
					}
					add(runtime, c.Name, c.Status, c.Message, c.Hint)
				}
			}
			if s, ok := r.(*unibox.SBX); ok {
				allowed, said := o.manager.Config.SSHAgent("sbx")
				forwarding, err := s.SSHForwarding(ctx)
				switch {
				case err != nil:
					add(runtime, "SSH agent", "warn", err.Error(), "")
				case forwarding && !allowed:
					add(runtime, "SSH agent", "fail", "sbx forwards your SSH agent, which you have not allowed", "unibox config set runtimes.sbx.ssh_agent false turns it off, or true allows it")
				case !forwarding && allowed:
					add(runtime, "SSH agent", "warn", "you allowed the SSH agent, but sbx does not forward it", "unibox config set runtimes.sbx.ssh_agent true sets sbx to match")
				case forwarding:
					add(runtime, "SSH agent", "pass", "forwarded, as you allowed", "")
				case said:
					add(runtime, "SSH agent", "pass", "not forwarded, as you chose", "")
				default:
					add(runtime, "SSH agent", "pass", "not forwarded", "")
				}
			}
		}
		root, err := o.manager.Environments()
		if err != nil {
			add("Environments", "repository", "fail", err.Error(), "")
		} else {
			add("Environments", "repository", "pass", root, "")
			problems, err := unibox.Validate(root)
			if err != nil {
				add("Environments", "valid", "fail", err.Error(), "")
			}
			for _, p := range problems {
				add("Environments", p.Subject, "fail", p.Message, "")
			}
		}
		if _, err := o.paths.LoadVars(); err != nil {
			add("Your values", "vars.toml", "fail", err.Error(), "")
		} else {
			add("Your values", "vars.toml", "pass", o.paths.VarsFile(), "")
		}
		names := []string{}
		if profile != "" {
			names = []string{profile}
		} else if root != "" {
			if rows, err := o.manager.Profiles(ctx); err == nil {
				for _, r := range rows {
					names = append(names, r.Name)
				}
			}
		}
		store, storeErr := o.secretStore()
		for _, name := range names {
			area := "Profile " + name
			p, err := o.manager.Profile(name)
			if err != nil {
				add(area, "loads", "fail", err.Error(), "")
				continue
			}
			ok := true
			for _, v := range p.MissingVars() {
				ok = false
				add(area, "var "+v, "fail", "no value: "+p.Vars[v], "unibox var set "+v+" <value>")
			}
			for _, ref := range slices.DeleteFunc(p.SecretRefs(), func(n string) bool { return p.ReadBy(n) != "" }) {
				if storeErr != nil {
					add(area, "secret "+ref, "fail", storeErr.Error(), "")
					ok = false
				} else if has, _ := store.Has(ctx, ref); !has {
					ok = false
					add(area, "secret "+ref, "fail", "not in "+store.Name(), "unibox agent login -i -p "+name+", unibox secret set "+ref+", or unibox secret source")
				}
			}
			if ok {
				add(area, "values and tokens", "pass", "given", "")
			}
			r, rOK := o.manager.Runtimes[rt]
			if !rOK {
				continue
			}
			if err := r.Available(ctx); err != nil {
				add(area, "logins and image", "warn", "not checked: "+err.Error(), "")
				continue
			}
			if a, isAuth := r.(unibox.Authenticator); isAuth {
				// A missing login is asked for by the first run, so it is
				// told, not failed.
				for _, n := range a.AuthNeeds(ctx, "unibox-"+name, p) {
					add(area, "agent "+n.Agent, "warn", n.Message, "unibox agent login -i -p "+name+" (or the first run asks)")
				}
			}
			if _, err := o.manager.Built(ctx, p, r); err != nil {
				add(area, "image", "warn", "not built for "+rt, "unibox profile build "+name+" (or the first run builds it)")
			} else {
				add(area, "image", "pass", "built", "")
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
			area := ""
			w := tabwriter.NewWriter(o.output, 0, 4, 1, ' ', 0)
			for _, c := range checks {
				if c.Area != area {
					area = c.Area
					fmt.Fprintf(w, "%s\n", area)
				}
				mark := map[string]string{"pass": "✓", "warn": "!", "fail": "✗"}[c.Status]
				fmt.Fprintf(w, "  %s %s\t%s\n", mark, c.Name, c.Message)
				if c.Hint != "" && c.Status != "pass" {
					fmt.Fprintf(w, "      \t%s\n", c.Hint)
				}
			}
			if err := w.Flush(); err != nil {
				return err
			}
			if failed == 0 {
				fmt.Fprintln(o.output, "everything a run needs is in place")
			} else {
				fmt.Fprintf(o.output, "%d to fix\n", failed)
			}
		}
		if failed > 0 {
			return &reportedError{4}
		}
		return nil
	}}
	c.Flags().StringVarP(&profile, "profile", "p", "", "Check this profile alone (default: every profile)")
	return c
}
