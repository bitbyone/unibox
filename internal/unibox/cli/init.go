package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
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

// initCommand sets unibox up. Until its walk is back, it says what to run.
func (o *options) initCommand() *cobra.Command {
	return &cobra.Command{Use: "init", Args: cobra.NoArgs, Short: "Set unibox up: an env, its args and logins", RunE: func(cmd *cobra.Command, args []string) error {
		if len(o.manager.Config.Envs) == 0 {
			fmt.Fprintln(o.output, "unibox env add <name> <file.sbxenv.yaml> names your team's env; then unibox var set -e <name> for its args, and unibox doctor says what is left")
			return nil
		}
		fmt.Fprintln(o.output, "unibox doctor says what is left to set up")
		return nil
	}}
}
