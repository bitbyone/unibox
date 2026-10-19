package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
)

func (o *options) prompt(reader *bufio.Reader, question, preset string) (string, error) {
	fmt.Fprintf(o.log, "%s [%s]: ", question, preset)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("interactive init was cut short; rerun it or pass the profile on the command line")
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return preset, nil
	}
	return line, nil
}

func (o *options) inspectCommand() *cobra.Command {
	return &cobra.Command{Use: "inspect sandbox", Args: cobra.ExactArgs(1), Short: "Show a sandbox's record and the runtime materials made for it", RunE: func(cmd *cobra.Command, args []string) error {
		boxes, err := o.manager.List(cmd.Context())
		if err != nil {
			return err
		}
		for _, b := range boxes {
			if b.Name != args[0] {
				continue
			}
			kit := ""
			descriptor := ""
			if r, ok := o.manager.Runtimes[b.Runtime].(*unibox.SBX); ok {
				if dir := r.KitDir(b.RuntimeName); dir != "" {
					kit = dir
					data, _ := os.ReadFile(filepath.Join(dir, b.RuntimeName+".yaml"))
					descriptor = string(data)
				}
			}
			if o.json {
				return o.write(map[string]any{"api_version": api.Version, "sandbox": b, "kit_dir": kit, "kit_descriptor": descriptor})
			}
			record, _ := json.MarshalIndent(b, "", "  ")
			fmt.Fprintln(o.output, string(record))
			if kit != "" {
				fmt.Fprintf(o.output, "\ngenerated kit: %s\n\n%s", kit, descriptor)
			}
			return nil
		}
		return fmt.Errorf("sandbox %s not found; list them with unibox sandboxes", args[0])
	}}
}

func (o *options) validateCommand() *cobra.Command {
	return &cobra.Command{Use: "validate", Args: cobra.NoArgs, Short: "Check the environments repository: layout, profiles, instruction texts", RunE: func(cmd *cobra.Command, args []string) error {
		problems, err := unibox.Validate(o.manager.Config.Environments)
		if err != nil {
			return err
		}
		if o.json {
			if err := o.write(map[string]any{"api_version": api.Version, "ok": len(problems) == 0, "problems": problems}); err != nil {
				return err
			}
		} else {
			for _, p := range problems {
				fmt.Fprintf(o.output, "%s: %s\n", p.Subject, p.Message)
			}
			if len(problems) == 0 {
				fmt.Fprintln(o.output, "environments repository is valid")
			}
		}
		if len(problems) > 0 {
			return &reportedError{4}
		}
		return nil
	}}
}

func (o *options) initCommand() *cobra.Command {
	var interactive bool
	var agents []string
	var cpus, memory, dir, from, use, template string
	c := &cobra.Command{Use: "init [profile]", Args: cobra.MaximumNArgs(1), Short: "Lay out the environments repository, and add a profile to it - or copy one from a template", RunE: func(cmd *cobra.Command, args []string) error {
		name := ""
		if len(args) == 1 {
			name = args[0]
		}
		// Where the repository goes: --dir says it and records it; without
		// it the configured one is used, and with none -i asks, defaulting
		// to here. Init always says which it writes into.
		root := o.manager.Config.Environments
		previous := root
		var reader *bufio.Reader
		if interactive {
			if o.json || !o.interactive {
				return fmt.Errorf("interactive init needs a terminal; pass the profile and flags instead")
			}
			reader = bufio.NewReader(o.input)
		}
		switch {
		case dir != "":
			abs, err := unibox.ExpandPath(dir)
			if err != nil {
				return err
			}
			if abs, err = filepath.Abs(abs); err != nil {
				return err
			}
			root = abs
		case root == "" && interactive:
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			fmt.Fprintln(o.log, "No environments repository is configured yet; it holds your profiles and is yours to keep in git.")
			v, err := o.prompt(reader, "Environments repository", wd)
			if err != nil {
				return err
			}
			if root, err = unibox.ExpandPath(v); err != nil {
				return err
			}
			if root, err = filepath.Abs(root); err != nil {
				return err
			}
		case root == "":
			return fmt.Errorf("no environments repository is configured; say where with unibox init --dir <path>, or be asked with unibox init -i")
		}
		if from != "" {
			if _, err := unibox.Init(root, "", nil, "", ""); err != nil {
				return err
			}
			return o.initFrom(cmd, root, previous, name, from, use, template, interactive)
		}
		if use != "" || template != "" {
			return fmt.Errorf("--use and --template go with --from")
		}
		if interactive {
			if name == "" {
				v, err := o.prompt(reader, "Profile name", "dev")
				if err != nil {
					return err
				}
				name = v
			}
			ids := []string{}
			for _, a := range unibox.Agents {
				ids = append(ids, a.ID)
			}
			v, err := o.prompt(reader, "Agents ("+strings.Join(ids, ", ")+")", strings.Join(agents, ","))
			if err != nil {
				return err
			}
			agents = strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' })
			if cpus, err = o.prompt(reader, "CPUs (a number, or a share of the host's: 50%)", cpus); err != nil {
				return err
			}
			if memory, err = o.prompt(reader, "Memory (a size such as 8g, or a share: 50%)", memory); err != nil {
				return err
			}
		}
		if err := os.MkdirAll(root, 0o755); err != nil {
			return err
		}
		// A profile that exists keeps its agents: init completes it, and
		// takes --agents only to check they are the profile's.
		chosen := agents
		if !interactive && !cmd.Flags().Changed("agents") {
			chosen = nil
		}
		report, err := unibox.Init(root, name, chosen, cpus, memory)
		if err != nil {
			return err
		}
		// Each person's values go in their home, laid out for them here.
		varsWritten, err := o.paths.EnsureVarsFile()
		if err != nil {
			return err
		}
		if root != previous {
			cfg := o.manager.Config
			cfg.Environments = root
			if err := o.paths.SaveConfig(cfg); err != nil {
				return err
			}
		}
		if o.json {
			return o.write(map[string]any{"api_version": api.Version, "root": report.Root, "written": report.Written, "kept": report.Kept, "completed": report.Completed, "migrated": report.Migrated, "updated": report.Updated, "removed": report.Removed, "relaid": report.Relaid})
		}
		switch {
		case previous == "":
			fmt.Fprintf(o.output, "environments repository: %s (configured)\n", root)
		case root != previous:
			fmt.Fprintf(o.output, "environments repository: %s (configured; was %s)\n", root, previous)
		default:
			fmt.Fprintf(o.output, "environments repository: %s\n", root)
		}
		if varsWritten {
			fmt.Fprintf(o.output, "wrote %s, for your own values of what profiles ask (unibox var set)\n", o.paths.VarsFile())
		}
		for i, line := range report.Migrated {
			if i == 0 {
				fmt.Fprintln(o.output, "migrated", line)
			} else {
				fmt.Fprintln(o.output, "  not carried over:", line)
			}
		}
		for _, rel := range report.Written {
			fmt.Fprintln(o.output, "wrote", rel)
		}
		for _, line := range report.Relaid {
			fmt.Fprintln(o.output, "layout:", line)
		}
		for _, rel := range report.Updated {
			fmt.Fprintln(o.output, "updated", rel, "to unibox's own")
		}
		for _, rel := range report.Removed {
			fmt.Fprintln(o.output, "removed", rel, "- unibox no longer has it")
		}
		for _, rel := range report.Kept {
			fmt.Fprintln(o.output, "kept", rel)
		}
		for _, rel := range report.Completed {
			fmt.Fprintln(o.output, "completed", rel)
		}
		if name == "" {
			fmt.Fprintln(o.output, "add a profile with unibox init <profile>, or be asked with unibox init -i")
			return nil
		}
		fmt.Fprintf(o.output, "profile %s is ready; sign it in with unibox agent login %s, add forges with unibox service add %s <name>, then unibox run builds and starts it\n", name, name, name)
		return nil
	}}
	c.Flags().StringVar(&dir, "dir", "", "The environments repository to lay out and configure (default: the configured one)")
	c.Flags().StringVar(&from, "from", "", "A template to copy the profile from: a git URL or a directory, an environments repository of someone else's")
	c.Flags().StringVar(&use, "use", "", "The template's profile to copy (default: the profile's name; -i asks)")
	c.Flags().StringVar(&template, "template", "", "The name the template is kept under in .templates (default: its repository's)")
	c.Flags().BoolVarP(&interactive, "interactive", "i", false, "Ask for the repository when none is configured, and the profile's name, agents and resources")
	c.Flags().StringSliceVar(&agents, "agents", []string{"copilot"}, "Agents the profile declares and installs")
	c.Flags().StringVar(&cpus, "cpus", "4", "CPUs for the profile's sandboxes, or a share of the host's (50%)")
	c.Flags().StringVar(&memory, "memory", "8g", "Memory for the profile's sandboxes (8g), or a share of the host's (50%)")
	return c
}
