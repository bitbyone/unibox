package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
)

// networkUI is unibox network at a terminal: a sandbox's own rules and the
// hosts it was refused lately, to allow or deny by a key: any host typed,
// the refused one under the cursor offered.
func (o *options) networkUI(cmd *cobra.Command, sandbox string) error {
	ctx := cmd.Context()
	var rules []api.NetworkRule
	blocked := map[string]bool{}
	build := func() (listModel, error) {
		v, err := o.manager.Network(ctx, sandbox)
		if err != nil {
			return listModel{}, err
		}
		rules = networkRules(v, false)
		m := listModel{title: "network of sandbox " + sandbox + " - " + beyond(v), header: []string{"RULE", "HOST", "PROTOCOL", "FROM"}, actions: []listAction{
			{key: "a", name: "allow", help: "allow", any: true},
			{key: "n", name: "deny", help: "deny", any: true},
			{key: "d", name: "rm", help: "remove", confirm: "take the rule for %s away?"},
		}}
		for _, r := range rules {
			decision := portOn.Render(r.Decision)
			if r.Decision == "deny" {
				decision = portErr.Render(r.Decision)
			}
			m.rows = append(m.rows, []string{decision, r.Host, r.Protocol, fromText(r)})
			m.ids = append(m.ids, r.Host)
		}
		clear(blocked)
		for _, b := range v.Blocked {
			if slices.Contains(m.ids, b.Host) {
				continue
			}
			blocked[b.Host] = true
			m.rows = append(m.rows, []string{portOff.Render("refused"), b.Host, "", portOff.Render(fmt.Sprintf("%d×, %s", b.Count, ago(b.LastSeen)))})
			m.ids = append(m.ids, b.Host)
		}
		return m, nil
	}
	act := func(action, id string) (string, error) {
		switch action {
		case "allow", "deny":
			// A refused host under the cursor is offered; typing replaces it.
			preset := ""
			if blocked[id] {
				preset = id
			}
			hosts, err := o.askText("Hosts sandbox "+sandbox+" may "+map[string]string{"allow": "reach", "deny": "not reach"}[action],
				"Names, *.name for subdomains or name:port, separated by spaces", preset, func(s string) error { return unibox.ValidHosts(strings.Fields(s)) })
			if err != nil || strings.TrimSpace(hosts) == "" {
				return "", err
			}
			protocol, err := o.askChoice("Protocol", "", unibox.Protocols, "tcp", nil)
			if err != nil {
				return "", err
			}
			return o.quietStep(cmd, append(append([]string{"network", action}, strings.Fields(hosts)...), "-s", sandbox, "--protocol", protocol)...)
		case "rm":
			if blocked[id] {
				return "a refused host has no rule to take away; a allows it", nil
			}
			i := slices.IndexFunc(rules, func(r api.NetworkRule) bool { return r.Host == id })
			if i >= 0 && rules[i].From == "sbx" && rules[i].Sandbox == "" {
				// A rule of every sandbox's, added with sbx: unibox network
				// has no command for it, being about one sandbox.
				if err := o.manager.RemoveHostRules(ctx, "", []string{id}); err != nil {
					return "", err
				}
				return "took the rule for " + id + " away from every sandbox", nil
			}
			return o.quietStep(cmd, "network", "rm", id, "-s", sandbox)
		}
		return "", nil
	}
	return o.actList(cmd, build, act, func() error { return nil })
}
