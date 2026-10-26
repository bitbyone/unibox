package cli

import (
	"bytes"
	"cmp"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tobola/unibox/internal/unibox"
)

// networkUI is unibox network at a terminal: the rules a sandbox has, or
// every rule with --global, to add to and take away from, and what the
// policy decided lately.
func (o *options) networkUI(cmd *cobra.Command, sandbox string, global bool, leave func() error) error {
	ctx := cmd.Context()
	var rules []unibox.NetworkRule
	title := "network rules of " + who(sandbox)
	if global {
		title = "every network rule"
	}
	scope := func(ruleSandbox string) []string {
		if ruleSandbox == "" {
			return []string{"--global"}
		}
		return []string{"-s", ruleSandbox}
	}
	build := func() (listModel, error) {
		var err error
		if rules, err = o.manager.SBX.NetworkRules(ctx, sandbox); err != nil {
			return listModel{}, err
		}
		m := listModel{title: title, header: []string{"DECISION", "FOR", "FROM", "HOSTS"}, actions: []listAction{
			{key: "a", name: "allow", help: "allow hosts", any: true},
			{key: "n", name: "deny", help: "deny hosts", any: true},
			{key: "d", name: "rm", help: "remove", confirm: "take rule %s away?"},
			{key: "l", name: "log", help: "latest requests", any: true},
		}}
		for _, r := range rules {
			decision := r.Decision
			if !r.Active {
				decision += " (inactive)"
			}
			m.rows = append(m.rows, []string{decision, cmp.Or(r.Sandbox, "all"), r.From, strings.Join(r.Hosts, ", ")})
			m.ids = append(m.ids, r.ID)
		}
		return m, nil
	}
	act := func(action, id string) (string, error) {
		switch action {
		case "allow", "deny":
			hosts, err := o.askText("Hosts "+who(sandbox)+" may "+map[string]string{"allow": "reach", "deny": "not reach"}[action],
				"Names, *.name for subdomains or name:port, separated by spaces", "", func(s string) error { return unibox.ValidHosts(strings.Fields(s)) })
			if err != nil || strings.TrimSpace(hosts) == "" {
				return "", err
			}
			return o.quietStep(cmd, append(append([]string{"network", action}, strings.Fields(hosts)...), scope(sandbox)...)...)
		case "rm":
			for _, r := range rules {
				if r.ID != id {
					continue
				}
				if !r.Editable {
					return "", fmt.Errorf("the rule comes from %s and is not to be removed here", r.From)
				}
				return o.quietStep(cmd, append(append([]string{"network", "rm"}, r.Hosts...), scope(r.Sandbox)...)...)
			}
		case "log":
			var out bytes.Buffer
			if err := o.manager.SBX.NetworkLog(ctx, sandbox, 10, &out); err != nil {
				return "", err
			}
			return strings.TrimRight(out.String(), "\n"), nil
		}
		return "", nil
	}
	return o.actList(cmd, build, act, leave)
}
